package recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/storage"
)

// Options configures stranded database recovery planning.
type Options struct {
	ActivePath string
	FromPath   string
	DryRun     bool
}

// Report describes the recovery plan without embedding writable state.
type Report struct {
	ActivePath      string
	BackupPath      string
	InputCounts     []int
	ExactDuplicates int
	FinalCount      int
	Shapes          []compat.SchemaShape
	Conflicts       []compat.Diagnostic
}

type resolvedPath struct {
	path string
	info os.FileInfo
}

type recoveryOperationDeps struct {
	createDestination func(context.Context, string, compat.RecoveryPlan) (string, error)
	verifyDestination func(context.Context, string, compat.RecoveryPlan) error
	noClobberMove     func(string, string) error
	installMove       func(string, string) error
	restoreMove       func(string, string) error
	syncDir           func(string) error
	syncFile          func(string) error
	randomHex         func(int) (string, error)
	fingerprintSet    func(string) (sqliteFileSetSnapshot, error)
	ensureSidecars    func(string) error
	openReservation   func(context.Context, string) (*activeReservation, error)
	readReservedInput func(context.Context, compat.Queryer) (compat.RecoveryInput, *compat.Diagnostic, error)
	closeReservation  func(*activeReservation) error
	cleanupFileSet    func(string) error
}

func defaultRecoveryOperationDeps() recoveryOperationDeps {
	return recoveryOperationDeps{
		createDestination: storage.CreateRecoveryDestination,
		verifyDestination: storage.VerifyRecoveryDestination,
		noClobberMove:     noClobberMove,
		installMove:       noClobberMove,
		restoreMove:       noClobberMove,
		syncDir:           syncDir,
		syncFile:          syncFile,
		randomHex:         randomHex,
		fingerprintSet:    snapshotSQLiteFileSet,
		ensureSidecars:    ensureSourceSidecarsSafe,
		openReservation:   openActiveReservation,
		readReservedInput: storage.ReadRecoveryInputFromQueryer,
		closeReservation:  func(r *activeReservation) error { return r.Close() },
		cleanupFileSet:    removeSQLiteFileSet,
	}
}

// Execute resolves active and stranded database paths, reads distinct inputs in
// immutable read-only mode, runs the deterministic compatibility recovery planner,
// and optionally installs a verified fresh destination with a preserved backup.
func Execute(ctx context.Context, opts Options) (report Report, err error) {
	return executeWithDeps(ctx, opts, defaultRecoveryOperationDeps())
}

func executeWithDeps(ctx context.Context, opts Options, deps recoveryOperationDeps) (report Report, err error) {
	if opts.ActivePath == "" {
		err := fmt.Errorf("active database path is required")
		if opts.DryRun {
			return Report{}, err
		}
		return Report{}, initialApplyFailure(phaseSourceRead, "", knownPathDescription(opts.FromPath), err)
	}
	if opts.FromPath == "" {
		err := fmt.Errorf("--from database path is required")
		if opts.DryRun {
			return Report{}, err
		}
		return Report{}, initialApplyFailure(phaseSourceRead, knownPathDescription(opts.ActivePath), "", err)
	}

	active, err := resolvePath(opts.ActivePath)
	if err != nil {
		if opts.DryRun {
			return Report{}, err
		}
		return Report{}, initialApplyFailure(phaseSourceRead, knownPathDescription(opts.ActivePath), knownPathDescription(opts.FromPath), err)
	}
	from, err := resolvePath(opts.FromPath)
	if err != nil {
		if opts.DryRun {
			return Report{}, err
		}
		return Report{}, initialApplyFailure(phaseSourceRead, active.path, knownPathDescription(opts.FromPath), err)
	}

	report.ActivePath = active.path
	report.BackupPath = intendedBackupDescription(active.path)

	paths := []resolvedPath{active}
	if !sameFile(active, from) {
		paths = append(paths, from)
	}

	if opts.DryRun {
		plan, diagnostics, err := planRecoveryFromImmutablePaths(ctx, paths, &report, true, deps)
		if err != nil {
			return report, err
		}
		fillReportFromPlan(&report, plan, diagnostics)
		if len(diagnostics) != 0 {
			return report, fmt.Errorf("recovery plan has %d diagnostic(s)", len(diagnostics))
		}
		return report, nil
	}

	plan, plannedActive, verifiedTempPath, diagnostics, err := planRecoveryWithActiveReservation(ctx, active, from, &report, deps)
	if err != nil {
		if len(diagnostics) != 0 || len(plan.InputShapes) != 0 {
			fillReportFromPlan(&report, plan, diagnostics)
		}
		return report, ensureApplyFailureFromPath(err, phasePlanning, active.path, verifiedTempPath, from.path)
	}
	fillReportFromPlan(&report, plan, diagnostics)
	if len(diagnostics) != 0 {
		return report, enrichApplyFailuresFromPath(applyDiagnosticFailure(active.path, diagnostics), from.path)
	}
	backupPath, err := replaceActiveWithBackup(ctx, active.path, verifiedTempPath, plan, plannedActive, deps)
	if err != nil {
		err = ensureApplyFailureFromPath(err, phaseInstall, active.path, verifiedTempPath, from.path)
		if backupPath != "" {
			report.BackupPath = backupPath
		}
		if cleanupErr := deps.cleanupFileSet(verifiedTempPath); cleanupErr != nil {
			var applyErr *ApplyFailure
			if errors.As(err, &applyErr) {
				applyErr.addCleanupError(fmt.Errorf("cleanup verified recovery destination %s after failed replacement: %w", verifiedTempPath, cleanupErr))
			}
		}
		return report, enrichApplyFailuresFromPath(err, from.path)
	}
	report.BackupPath = backupPath
	return report, nil
}

func fillReportFromPlan(report *Report, plan compat.RecoveryPlan, diagnostics []compat.Diagnostic) {
	report.Shapes = append(report.Shapes, plan.InputShapes...)
	report.ExactDuplicates = plan.ExactDuplicates
	report.FinalCount = len(plan.Records)
	report.Conflicts = append(report.Conflicts, diagnostics...)
}

func planRecoveryFromImmutablePaths(ctx context.Context, paths []resolvedPath, report *Report, dryRun bool, deps recoveryOperationDeps) (compat.RecoveryPlan, []compat.Diagnostic, error) {
	inputs := make([]compat.RecoveryInput, 0, len(paths))
	for _, path := range paths {
		input, err := readImmutableRecoveryInput(ctx, path.path, dryRun, deps)
		if err != nil {
			return compat.RecoveryPlan{}, nil, err
		}
		inputs = append(inputs, input)
		report.InputCounts = append(report.InputCounts, input.RowCount)
	}

	plan, diagnostics, planErr := compat.PlanRecovery(inputs)
	if planErr != nil {
		return compat.RecoveryPlan{}, nil, planErr
	}
	return plan, diagnostics, nil
}

func readImmutableRecoveryInput(ctx context.Context, path string, dryRun bool, deps recoveryOperationDeps) (compat.RecoveryInput, error) {
	if err := deps.ensureSidecars(path); err != nil {
		return compat.RecoveryInput{}, recoveryOpenError(path, err, dryRun)
	}
	planned, err := deps.fingerprintSet(path)
	if err != nil {
		return compat.RecoveryInput{}, recoveryOpenError(path, err, dryRun)
	}
	db, openErr := storage.OpenImmutableReadOnly(path)
	if openErr != nil {
		return compat.RecoveryInput{}, recoveryOpenError(path, openErr, dryRun)
	}
	input, diag, err := storage.ReadRecoveryInput(ctx, db)
	closeErr := db.Close()
	if err != nil {
		return compat.RecoveryInput{}, errors.Join(err, closeErr)
	}
	if diag != nil {
		return compat.RecoveryInput{}, errors.Join(diagnosticError(*diag), closeErr)
	}
	if closeErr != nil {
		return compat.RecoveryInput{}, closeErr
	}
	if err := deps.ensureSidecars(path); err != nil {
		return compat.RecoveryInput{}, recoveryOpenError(path, err, dryRun)
	}
	got, err := deps.fingerprintSet(path)
	if err != nil {
		return compat.RecoveryInput{}, recoveryOpenError(path, err, dryRun)
	}
	if !sameSQLiteFileSetSnapshot(got, planned) {
		return compat.RecoveryInput{}, fmt.Errorf("immutable recovery source changed while reading %s", path)
	}
	return input, nil
}

func planRecoveryWithActiveReservation(ctx context.Context, active, from resolvedPath, report *Report, deps recoveryOperationDeps) (planOut compat.RecoveryPlan, plannedOut sqliteFileSetSnapshot, tempOut string, diagnosticsOut []compat.Diagnostic, errOut error) {
	if err := deps.ensureSidecars(active.path); err != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", fmt.Errorf("active database sidecars are unsafe for recovery apply: %w", err), nil, nil)
	}
	reservation, err := deps.openReservation(ctx, active.path)
	if err != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseReservationOpen, active.path, "", err, nil, nil)
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := deps.closeReservation(reservation); closeErr != nil {
				closeFailure := preReplacementFailure(phaseReservationClose, active.path, tempOut, closeErr, nil, closeErr)
				if errOut != nil {
					errOut = errors.Join(errOut, closeFailure)
				} else {
					errOut = closeFailure
				}
			}
		}
	}()

	plannedSnapshot, err := deps.fingerprintSet(active.path)
	if err != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", fmt.Errorf("fingerprint locked active database before recovery SQL read: %w", err), nil, nil)
	}
	plannedSnapshot.sidecars = map[string]sqliteFileSnapshot{}
	activeInput, diag, err := deps.readReservedInput(ctx, reservation)
	if err != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", fmt.Errorf("read active recovery source under reservation: %w", err), nil, nil)
	}
	if diag != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", diagnosticError(*diag), nil, nil)
	}
	if err := revalidateMainSnapshot(active.path, plannedSnapshot, deps); err != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", fmt.Errorf("active database changed during recovery SQL read: %w", err), nil, nil)
	}
	inputs := []compat.RecoveryInput{activeInput}
	report.InputCounts = append(report.InputCounts, activeInput.RowCount)

	if !sameFile(active, from) {
		strandedInput, err := readImmutableRecoveryInput(ctx, from.path, false, deps)
		if err != nil {
			return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseSourceRead, active.path, "", fmt.Errorf("read stranded recovery source %s: %w", from.path, err), nil, nil)
		}
		inputs = append(inputs, strandedInput)
		report.InputCounts = append(report.InputCounts, strandedInput.RowCount)
	}

	plan, diagnostics, planErr := compat.PlanRecovery(inputs)
	if planErr != nil {
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phasePlanning, active.path, "", planErr, nil, nil)
	}
	if len(diagnostics) != 0 {
		if err := deps.closeReservation(reservation); err != nil {
			closed = true
			return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseReservationClose, active.path, "", err, nil, err)
		}
		closed = true
		return plan, sqliteFileSetSnapshot{}, "", diagnostics, applyDiagnosticFailure(active.path, diagnostics)
	}

	verifiedTempPath, err := deps.createDestination(ctx, filepath.Dir(active.path), plan)
	tempOut = verifiedTempPath
	if err != nil {
		cleanupErr := destinationConstructionCleanupErr(err)
		if leakedPath := destinationConstructionTempPath(err); leakedPath != "" {
			verifiedTempPath = leakedPath
			tempOut = leakedPath
		}
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseDestinationVerify, active.path, verifiedTempPath, err, cleanupErr, nil)
	}

	if err := revalidateMainSnapshot(active.path, plannedSnapshot, deps); err != nil {
		cleanupErr := deps.cleanupFileSet(verifiedTempPath)
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phasePreBackupValidate, active.path, verifiedTempPath, fmt.Errorf("active database changed while planning recovery apply: %w", err), cleanupErr, nil)
	}
	if err := deps.closeReservation(reservation); err != nil {
		cleanupErr := deps.cleanupFileSet(verifiedTempPath)
		closed = true
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phaseReservationClose, active.path, verifiedTempPath, err, cleanupErr, err)
	}
	closed = true
	if err := deps.ensureSidecars(active.path); err != nil {
		cleanupErr := deps.cleanupFileSet(verifiedTempPath)
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phasePreBackupValidate, active.path, verifiedTempPath, fmt.Errorf("active database sidecars changed after closing recovery reservation: %w", err), cleanupErr, nil)
	}
	if err := revalidateMainSnapshot(active.path, plannedSnapshot, deps); err != nil {
		cleanupErr := deps.cleanupFileSet(verifiedTempPath)
		return compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, "", nil, preReplacementFailure(phasePreBackupValidate, active.path, verifiedTempPath, fmt.Errorf("active database changed after closing recovery reservation: %w", err), cleanupErr, nil)
	}
	return plan, plannedSnapshot, verifiedTempPath, diagnostics, nil
}

type activeReservation struct {
	db   *sql.DB
	conn *sql.Conn
}

func openActiveReservation(ctx context.Context, activePath string) (*activeReservation, error) {
	db, err := sql.Open("sqlite", "file:"+activePath+"?mode=rw&_pragma=busy_timeout(1)")
	if err != nil {
		return nil, fmt.Errorf("open active database for recovery apply reservation: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		closeErr := db.Close()
		return nil, fmt.Errorf("reserve active database for recovery apply: %w", errors.Join(err, closeErr))
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		closeErr := errors.Join(conn.Close(), db.Close())
		return nil, fmt.Errorf("reserve active database writer slot for final recovery plan: %w", errors.Join(err, closeErr))
	}
	return &activeReservation{db: db, conn: conn}, nil
}

func (r *activeReservation) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.conn.QueryContext(ctx, query, args...)
}

func (r *activeReservation) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return r.conn.QueryRowContext(ctx, query, args...)
}

func (r *activeReservation) Close() error {
	var errs []error
	if r.conn != nil {
		if _, rollbackErr := r.conn.ExecContext(context.Background(), `ROLLBACK`); rollbackErr != nil {
			errs = append(errs, fmt.Errorf("rollback active recovery reservation: %w", rollbackErr))
		}
		if closeErr := r.conn.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("close active recovery reservation connection: %w", closeErr))
		}
		r.conn = nil
	}
	if r.db != nil {
		if closeErr := r.db.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("close active recovery reservation database: %w", closeErr))
		}
		r.db = nil
	}
	return errors.Join(errs...)
}

// ApplyFailurePhase is the recovery apply stage that failed.
type ApplyFailurePhase string

type replacementPhase = ApplyFailurePhase

const (
	phaseDestinationSidecar replacementPhase = "destination-sidecar-cleanup"
	phaseDestinationSync    replacementPhase = "destination-fsync"
	phaseDestinationVerify  replacementPhase = "destination-verify"
	phasePreBackupValidate  replacementPhase = "active-pre-backup-revalidation"
	phaseBackupMain         replacementPhase = "backup-main"
	phaseBackupSidecar      replacementPhase = "backup-sidecar"
	phaseBackupFileSync     replacementPhase = "backup-file-fsync"
	phaseBackupDirSync      replacementPhase = "backup-directory-fsync"
	phasePostBackupVerify   replacementPhase = "backup-snapshot-verify"
	phaseInstall            replacementPhase = "install-active"
	phaseInstallSync        replacementPhase = "install-directory-fsync"
	phaseRestore            replacementPhase = "restore-active"
	phaseSourceRead         replacementPhase = "source-read"
	phasePlanning           replacementPhase = "planning"
	phaseReservationOpen    replacementPhase = "reservation-open"
	phaseReservationClose   replacementPhase = "reservation-close"
)

// ApplyFailureState describes the current filesystem state after a failed apply.
type ApplyFailureState string

type replacementState = ApplyFailureState

const (
	replacementStateNoMutation                    replacementState = "no-mutation"
	replacementStateNoActiveMutation              replacementState = "no-active-mutation"
	replacementStateBackupVisible                 replacementState = "backup-visible"
	replacementStateBackupFileSynced              replacementState = "backup-file-synced"
	replacementStateBackupDirectoryDurable        replacementState = "backup-directory-durable"
	replacementStateBackupIntegrityVerified       replacementState = "backup-integrity-verified"
	replacementStateBackupDirectoryUnknown        replacementState = "backup-directory-durable-integrity-unknown"
	replacementStateReplacementVisible            replacementState = "replacement-visible"
	replacementStateReplacementDurable            replacementState = "replacement-durable"
	replacementStateActiveRestoredVisibleUnknown  replacementState = "active-restored-visible-integrity-unknown"
	replacementStateActiveRestoredDurableVerified replacementState = "active-restored-durable-verified"
	replacementStateSplitUnknown                  replacementState = "split-or-unknown"
)

type replacementStatus struct {
	BackupPath                    string
	NoActiveMutation              bool
	BackupVisible                 bool
	BackupFileSynced              bool
	BackupDirectoryDurable        bool
	BackupIntegrityVerified       bool
	BackupIntegrityUnknown        bool
	ReplacementVisible            bool
	ReplacementDurable            bool
	RestoredActiveVisible         bool
	RestoredActiveDurableVerified bool
	SplitUnknown                  bool
	Milestones                    ApplyFailureMilestones
}

// ApplyFailureMilestones records historical milestones reached before a failed
// apply. These are intentionally separate from the current filesystem-state
// booleans on ApplyFailure.
type ApplyFailureMilestones struct {
	BackupCreated                 bool
	BackupFileSynced              bool
	BackupDirectoryDurable        bool
	BackupIntegrityVerified       bool
	ReplacementVisible            bool
	ReplacementDurable            bool
	RestoredActiveVisible         bool
	RestoredActiveDurableVerified bool
}

// ApplyFailure is the structured error returned by failed recovery apply
// operations after recovery planning starts. Callers should use errors.As rather
// than parse Error text.
type ApplyFailure struct {
	Phase                         replacementPhase
	State                         replacementState
	ActivePath                    string
	FromPath                      string
	BackupPath                    string
	TempPath                      string
	CurrentRestorablePath         string
	Milestones                    ApplyFailureMilestones
	NoActiveMutation              bool
	BackupVisible                 bool
	BackupFileSynced              bool
	BackupDirectoryDurable        bool
	BackupIntegrityVerified       bool
	BackupIntegrityUnknown        bool
	ReplacementVisible            bool
	ReplacementDurable            bool
	RestoredActiveVisible         bool
	RestoredActiveDurableVerified bool
	ActiveRestored                bool
	ActiveInstalled               bool
	RestorableBackup              bool
	RestorableBackupPath          string
	Cause                         error
	RestoreErr                    error
	CleanupErr                    error
	CleanupErrors                 []error
	CloseErr                      error
}

type replacementError = ApplyFailure
type preReplacementError = ApplyFailure

func (e *ApplyFailure) Error() string {
	msg := fmt.Sprintf("recovery replacement failed during %s: %v; state=%s active=%s from=%s backup=%s temp=%s", e.Phase, e.Cause, e.State, e.ActivePath, e.FromPath, e.BackupPath, e.TempPath)
	if e.RestorableBackup {
		msg += "; restorable backup remains at " + e.RestorableBackupPath
	}
	if e.ActiveRestored {
		msg += "; restored active"
	}
	if e.RestoreErr != nil {
		msg += "; restore error: " + e.RestoreErr.Error()
	}
	if e.CleanupErr != nil {
		msg += "; cleanup error: " + e.CleanupErr.Error()
	}
	if e.CloseErr != nil {
		msg += "; close error: " + e.CloseErr.Error()
	}
	return msg
}

func (e *ApplyFailure) Unwrap() []error {
	errs := []error{}
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	if e.RestoreErr != nil {
		errs = append(errs, e.RestoreErr)
	}
	if e.CleanupErr != nil {
		errs = append(errs, e.CleanupErr)
	}
	if e.CloseErr != nil {
		errs = append(errs, e.CloseErr)
	}
	return errs
}

func (e *ApplyFailure) addCleanupError(err error) {
	if err == nil {
		return
	}
	e.CleanupErrors = append(e.CleanupErrors, err)
	e.CleanupErr = errors.Join(e.CleanupErr, err)
}

// RestorableBackupPath reports the only known manual recovery path after a
// failed apply, when the replacement sequence proved the backup durable and
// integrity-verified and left it visible.
func RestorableBackupPath(err error) (string, bool) {
	var replacementErr *replacementError
	if errors.As(err, &replacementErr) && replacementErr.RestorableBackupPath != "" {
		return replacementErr.RestorableBackupPath, true
	}
	return "", false
}

func preReplacementFailure(phase replacementPhase, activePath, tempPath string, cause, cleanupErr, closeErr error) error {
	if cause == nil {
		cause = errors.Join(cleanupErr, closeErr)
	}
	failure := &ApplyFailure{Phase: phase, State: replacementStateNoActiveMutation, ActivePath: activePath, TempPath: tempPath, NoActiveMutation: true, Cause: cause, CloseErr: closeErr}
	failure.addCleanupError(cleanupErr)
	return failure
}

func initialApplyFailure(phase replacementPhase, activePath, fromPath string, cause error) error {
	return &ApplyFailure{Phase: phase, State: replacementStateNoActiveMutation, ActivePath: activePath, FromPath: fromPath, NoActiveMutation: true, Cause: cause}
}

func ensureApplyFailure(err error, phase replacementPhase, activePath, tempPath string) error {
	if err == nil {
		return nil
	}
	var failure *ApplyFailure
	if errors.As(err, &failure) {
		if failure.ActivePath == "" {
			failure.ActivePath = activePath
		}
		if failure.TempPath == "" {
			failure.TempPath = tempPath
		}
		return err
	}
	return preReplacementFailure(phase, activePath, tempPath, err, nil, nil)
}

func ensureApplyFailureFromPath(err error, phase replacementPhase, activePath, tempPath, fromPath string) error {
	return enrichApplyFailuresFromPath(ensureApplyFailure(err, phase, activePath, tempPath), fromPath)
}

func enrichApplyFailuresFromPath(err error, fromPath string) error {
	if err == nil || fromPath == "" {
		return err
	}
	enrichApplyFailureTreeFromPath(err, fromPath)
	return err
}

func enrichApplyFailureTreeFromPath(err error, fromPath string) {
	if err == nil {
		return
	}
	if failure, ok := err.(*ApplyFailure); ok && failure.FromPath == "" {
		failure.FromPath = fromPath
	}
	if wrapped, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range wrapped.Unwrap() {
			enrichApplyFailureTreeFromPath(child, fromPath)
		}
		return
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		enrichApplyFailureTreeFromPath(wrapped.Unwrap(), fromPath)
	}
}

func applyDiagnosticFailure(activePath string, diagnostics []compat.Diagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	summary := fmt.Errorf("recovery plan has %d diagnostic(s)", len(diagnostics))
	for _, diag := range diagnostics {
		summary = errors.Join(summary, diagnosticError(diag))
	}
	return preReplacementFailure(phasePlanning, activePath, "", summary, nil, nil)
}

func destinationConstructionTempPath(err error) string {
	var destinationErr *storage.RecoveryDestinationError
	if errors.As(err, &destinationErr) {
		return destinationErr.Path
	}
	return ""
}

func destinationConstructionCleanupErr(err error) error {
	var destinationErr *storage.RecoveryDestinationError
	if errors.As(err, &destinationErr) {
		return destinationErr.CleanupErr
	}
	return nil
}

func replaceActiveWithBackup(ctx context.Context, activePath, verifiedTempPath string, plan compat.RecoveryPlan, plannedActive sqliteFileSetSnapshot, deps recoveryOperationDeps) (backupPath string, err error) {
	defer func() {
		var re *replacementError
		if errors.As(err, &re) {
			re.ActivePath = activePath
			re.TempPath = verifiedTempPath
		}
	}()
	activeDir := filepath.Dir(activePath)
	if activeDir != filepath.Dir(verifiedTempPath) {
		return "", replacementFailure(phasePreBackupValidate, replacementStatus{NoActiveMutation: true}, fmt.Errorf("recovery replacement requires same directory: active=%s temp=%s", activePath, verifiedTempPath), nil)
	}
	verifiedDestination, err := prepareVerifiedDestination(ctx, verifiedTempPath, plan, deps)
	if err != nil {
		var re *replacementError
		if errors.As(err, &re) {
			return re.RestorableBackupPath, re
		}
		return "", err
	}
	if err := revalidatePlannedActive(activePath, plannedActive, deps); err != nil {
		return "", replacementFailure(phasePreBackupValidate, replacementStatus{NoActiveMutation: true}, fmt.Errorf("active database changed after locked recovery planning: %w", err), nil)
	}
	if err := deps.syncFile(activePath); err != nil {
		return "", replacementFailure(phasePreBackupValidate, replacementStatus{NoActiveMutation: true}, fmt.Errorf("fsync active database before backup: %w", err), nil)
	}
	if err := revalidatePlannedActive(activePath, plannedActive, deps); err != nil {
		return "", replacementFailure(phasePreBackupValidate, replacementStatus{NoActiveMutation: true}, fmt.Errorf("active database changed after pre-backup fsync: %w", err), nil)
	}

	backupPath, movedSidecars, err := installBackup(activePath, plannedActive, deps)
	if err != nil {
		var re *replacementError
		if errors.As(err, &re) && re.RestorableBackup {
			return re.RestorableBackupPath, err
		}
		return "", err
	}
	backupStatus := backupVisibleStatus(backupPath)
	if err := ensureNoActiveSQLiteSidecars(activePath); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phasePostBackupVerify, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	if err := verifyBackupMatchesPlannedSnapshot(backupPath, plannedActive, deps); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phasePostBackupVerify, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	if err := deps.syncFile(backupPath); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phaseBackupFileSync, withRestoreOutcome(backupStatus, outcome), fmt.Errorf("fsync recovery backup file: %w", err), outcome.err)
	}
	backupStatus = backupFileSyncedStatus(backupPath)
	if err := deps.syncDir(activeDir); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phaseBackupDirSync, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	backupStatus = backupDurableStatus(backupPath)
	if err := verifyBackupMatchesPlannedSnapshot(backupPath, plannedActive, deps); err != nil {
		unknownBackup := backupUnknownStatus(backupPath)
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phasePostBackupVerify, withRestoreOutcome(unknownBackup, outcome), err, outcome.err)
	}
	backupStatus = backupVerifiedStatus(backupPath)

	if err := deps.ensureSidecars(activePath); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phaseInstall, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	if _, err := os.Lstat(activePath); err == nil || !os.IsNotExist(err) {
		if err == nil {
			err = fmt.Errorf("active path unexpectedly exists before installing replacement: %s", activePath)
		}
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phaseInstall, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	if err := deps.installMove(verifiedTempPath, activePath); err != nil {
		outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
		return replacementBackupPath(outcome, backupPath), replacementFailure(phaseInstall, withRestoreOutcome(backupStatus, outcome), err, outcome.err)
	}
	if err := deps.syncDir(activeDir); err != nil {
		// The replacement rename succeeded, but the final directory fsync did not.
		// Keep the last known durable backup instead of consuming it for a restore
		// that could make the crash-recovery state less truthful.
		installed := backupStatus
		installed.ReplacementVisible = true
		installed.Milestones.ReplacementVisible = true
		return backupPath, replacementFailure(phaseInstallSync, installed, err, nil)
	}
	installed := backupStatus
	installed.ReplacementVisible = true
	installed.Milestones.ReplacementVisible = true
	if err := revalidateSnapshot(activePath, verifiedDestination, deps); err != nil {
		return backupPath, replacementFailure(phaseInstallSync, installed, fmt.Errorf("installed active database differs from verified recovery destination: %w", err), nil)
	}
	installed.ReplacementDurable = true
	installed.Milestones.ReplacementDurable = true
	if err := verifyBackupMatchesPlannedSnapshot(backupPath, plannedActive, deps); err != nil {
		unknownBackup := installed
		unknownBackup.BackupIntegrityVerified = false
		unknownBackup.BackupIntegrityUnknown = true
		return backupPath, replacementFailure(phaseInstallSync, unknownBackup, fmt.Errorf("durable backup changed after installing replacement: %w", err), nil)
	}
	return backupPath, nil
}

func prepareVerifiedDestination(ctx context.Context, verifiedTempPath string, plan compat.RecoveryPlan, deps recoveryOperationDeps) (sqliteFileSetSnapshot, error) {
	status := replacementStatus{NoActiveMutation: true}
	if err := deps.ensureSidecars(verifiedTempPath); err != nil {
		return sqliteFileSetSnapshot{}, replacementFailure(phaseDestinationSidecar, status, err, nil)
	}
	if err := deps.verifyDestination(ctx, verifiedTempPath, plan); err != nil {
		return sqliteFileSetSnapshot{}, replacementFailure(phaseDestinationVerify, status, err, nil)
	}
	if err := deps.syncFile(verifiedTempPath); err != nil {
		return sqliteFileSetSnapshot{}, replacementFailure(phaseDestinationSync, status, err, nil)
	}
	if err := deps.ensureSidecars(verifiedTempPath); err != nil {
		return sqliteFileSetSnapshot{}, replacementFailure(phaseDestinationSidecar, status, err, nil)
	}
	fingerprint, err := deps.fingerprintSet(verifiedTempPath)
	if err != nil {
		return sqliteFileSetSnapshot{}, replacementFailure(phaseDestinationVerify, status, fmt.Errorf("fingerprint verified recovery destination: %w", err), nil)
	}
	return fingerprint, nil
}

func moveFailureStatus(candidate string, err error) replacementStatus {
	status := replacementStatus{BackupPath: candidate, NoActiveMutation: true}
	var unsupported noClobberUnsupportedError
	if errors.As(err, &unsupported) {
		status.NoActiveMutation = true
	}
	return status
}

func installBackup(activePath string, plannedActive sqliteFileSetSnapshot, deps recoveryOperationDeps) (backupPath string, movedSidecars []string, err error) {
	if err := deps.ensureSidecars(activePath); err != nil {
		return "", nil, replacementFailure(phaseBackupMain, replacementStatus{NoActiveMutation: true}, err, nil)
	}
	for i := 0; i < 100; i++ {
		candidate, candidateErr := backupPathCandidate(activePath, deps)
		if candidateErr != nil {
			return "", nil, replacementFailure(phaseBackupMain, replacementStatus{NoActiveMutation: true}, candidateErr, nil)
		}
		moveErr := deps.noClobberMove(activePath, candidate)
		if moveErr == nil {
			backupPath = candidate
			break
		}
		if os.IsExist(moveErr) {
			continue
		}
		status := moveFailureStatus(candidate, moveErr)
		return "", nil, replacementFailure(phaseBackupMain, status, fmt.Errorf("move active database to backup candidate %s: %w", candidate, moveErr), nil)
	}
	if backupPath == "" {
		return "", nil, replacementFailure(phaseBackupMain, replacementStatus{NoActiveMutation: true}, fmt.Errorf("could not choose a unique recovery backup path for %s", activePath), nil)
	}

	for _, suffix := range sortedSnapshotSidecars(plannedActive) {
		if err := deps.noClobberMove(activePath+suffix, backupPath+suffix); err != nil {
			outcome := restoreBackup(activePath, backupPath, movedSidecars, plannedActive, deps)
			return backupPath, movedSidecars, replacementFailure(phaseBackupSidecar, withRestoreOutcome(backupVisibleStatus(backupPath), outcome), fmt.Errorf("move sidecar %s: %w", suffix, err), outcome.err)
		}
		movedSidecars = append(movedSidecars, suffix)
	}
	return backupPath, movedSidecars, nil
}

func replacementFailure(phase replacementPhase, status replacementStatus, cause, restoreErr error) *replacementError {
	state := replacementStateNoMutation
	switch {
	case status.RestoredActiveDurableVerified:
		state = replacementStateActiveRestoredDurableVerified
	case status.RestoredActiveVisible:
		state = replacementStateActiveRestoredVisibleUnknown
	case status.ReplacementDurable:
		state = replacementStateReplacementDurable
	case status.ReplacementVisible:
		state = replacementStateReplacementVisible
	case status.SplitUnknown || restoreErr != nil:
		state = replacementStateSplitUnknown
	case status.BackupIntegrityUnknown:
		state = replacementStateBackupDirectoryUnknown
	case status.BackupIntegrityVerified:
		state = replacementStateBackupIntegrityVerified
	case status.BackupDirectoryDurable:
		state = replacementStateBackupDirectoryDurable
	case status.BackupFileSynced:
		state = replacementStateBackupFileSynced
	case status.BackupVisible:
		state = replacementStateBackupVisible
	case status.NoActiveMutation:
		state = replacementStateNoActiveMutation
	}
	restorableBackup := status.BackupPath != "" && status.BackupVisible && status.BackupDirectoryDurable && status.BackupIntegrityVerified && !status.BackupIntegrityUnknown && !status.RestoredActiveVisible && !status.RestoredActiveDurableVerified
	restorablePath := mapRestorableBackupPath(restorableBackup, status.BackupPath)
	return &replacementError{
		Phase:                         phase,
		State:                         state,
		BackupPath:                    status.BackupPath,
		CurrentRestorablePath:         restorablePath,
		Milestones:                    status.Milestones,
		NoActiveMutation:              status.NoActiveMutation,
		BackupVisible:                 status.BackupVisible,
		BackupFileSynced:              status.BackupFileSynced,
		BackupDirectoryDurable:        status.BackupDirectoryDurable,
		BackupIntegrityVerified:       status.BackupIntegrityVerified && !status.BackupIntegrityUnknown,
		BackupIntegrityUnknown:        status.BackupIntegrityUnknown,
		ReplacementVisible:            status.ReplacementVisible || status.ReplacementDurable,
		ReplacementDurable:            status.ReplacementDurable,
		RestoredActiveVisible:         status.RestoredActiveVisible || status.RestoredActiveDurableVerified,
		RestoredActiveDurableVerified: status.RestoredActiveDurableVerified,
		ActiveRestored:                status.RestoredActiveDurableVerified,
		ActiveInstalled:               status.ReplacementVisible || status.ReplacementDurable,
		RestorableBackup:              restorableBackup,
		RestorableBackupPath:          restorablePath,
		Cause:                         cause,
		RestoreErr:                    restoreErr,
	}
}

func mapRestorableBackupPath(restorable bool, backupPath string) string {
	if restorable {
		return backupPath
	}
	return ""
}

type restoreOutcome struct {
	activeVisible         bool
	activeDurableVerified bool
	err                   error
}

func replacementBackupPath(outcome restoreOutcome, backupPath string) string {
	if outcome.activeVisible {
		return ""
	}
	return backupPath
}

func backupVisibleStatus(backupPath string) replacementStatus {
	return replacementStatus{BackupPath: backupPath, BackupVisible: backupPath != "", Milestones: ApplyFailureMilestones{BackupCreated: backupPath != ""}}
}

func backupFileSyncedStatus(backupPath string) replacementStatus {
	return replacementStatus{BackupPath: backupPath, BackupVisible: true, BackupFileSynced: true, Milestones: ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true}}
}

func backupDurableStatus(backupPath string) replacementStatus {
	return replacementStatus{BackupPath: backupPath, BackupVisible: true, BackupFileSynced: true, BackupDirectoryDurable: true, Milestones: ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true}}
}

func backupVerifiedStatus(backupPath string) replacementStatus {
	return replacementStatus{BackupPath: backupPath, BackupVisible: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, Milestones: ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true}}
}

func backupUnknownStatus(backupPath string) replacementStatus {
	return replacementStatus{BackupPath: backupPath, BackupVisible: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityUnknown: true, Milestones: ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true}}
}

func withRestoreOutcome(status replacementStatus, outcome restoreOutcome) replacementStatus {
	if outcome.activeVisible {
		status.BackupVisible = false
		status.BackupFileSynced = false
		status.BackupDirectoryDurable = false
		status.BackupIntegrityVerified = false
		status.BackupIntegrityUnknown = false
		status.RestoredActiveVisible = true
		status.Milestones.RestoredActiveVisible = true
	}
	if outcome.activeDurableVerified {
		status.RestoredActiveDurableVerified = true
		status.Milestones.RestoredActiveDurableVerified = true
		return status
	}
	if outcome.err != nil && !outcome.activeVisible {
		status.SplitUnknown = true
	}
	return status
}

func restoreBackup(activePath, backupPath string, backupSidecars []string, plannedActive sqliteFileSetSnapshot, deps recoveryOperationDeps) restoreOutcome {
	if backupPath == "" {
		return restoreOutcome{err: fmt.Errorf("no backup path available to restore")}
	}
	if err := deps.ensureSidecars(activePath); err != nil {
		return restoreOutcome{err: fmt.Errorf("active namespace has unexpected sidecars before restore: %w", err)}
	}
	for i := len(backupSidecars) - 1; i >= 0; i-- {
		suffix := backupSidecars[i]
		if err := deps.restoreMove(backupPath+suffix, activePath+suffix); err != nil {
			return restoreOutcome{err: fmt.Errorf("restore active sidecar %s from backup: %w", suffix, err)}
		}
	}
	if err := deps.restoreMove(backupPath, activePath); err != nil {
		return restoreOutcome{err: fmt.Errorf("restore active database from backup: %w", err)}
	}
	if err := deps.syncFile(activePath); err != nil {
		return restoreOutcome{activeVisible: true, err: fmt.Errorf("fsync restored active file: %w", err)}
	}
	if err := deps.syncDir(filepath.Dir(activePath)); err != nil {
		return restoreOutcome{activeVisible: true, err: fmt.Errorf("fsync restored active directory: %w", err)}
	}
	if err := revalidatePlannedActive(activePath, plannedActive, deps); err != nil {
		return restoreOutcome{activeVisible: true, err: fmt.Errorf("verify restored active database: %w", err)}
	}
	return restoreOutcome{activeVisible: true, activeDurableVerified: true}
}

func ensureSourceSidecarsSafe(dbPath string) error {
	for _, suffix := range sqliteSidecarSuffixes() {
		path := dbPath + suffix
		info, err := os.Lstat(path)
		if err == nil {
			if suffix == "-wal" {
				if info.Mode().IsRegular() && info.Size() == 0 {
					continue
				}
				return fmt.Errorf("%w: unexpected SQLite sidecar namespace entry %s", storage.ErrImmutableReadOnlyWALUnsafe, path)
			}
			return fmt.Errorf("unexpected SQLite sidecar namespace entry %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("lstat SQLite sidecar %s: %w", path, err)
		}
	}
	return nil
}

func ensureNoActiveSQLiteSidecars(dbPath string) error {
	if err := ensureSourceSidecarsSafe(dbPath); err != nil {
		return fmt.Errorf("unexpected active sidecar appeared after backup: %w", err)
	}
	return nil
}

func verifyBackupMatchesPlannedSnapshot(backupPath string, planned sqliteFileSetSnapshot, deps recoveryOperationDeps) error {
	got, err := deps.fingerprintSet(backupPath)
	if err != nil {
		return fmt.Errorf("snapshot recovery backup %s: %w", backupPath, err)
	}
	if !sameSQLiteFileSetSnapshot(got, planned) {
		return fmt.Errorf("recovery backup %s does not match locked active snapshot", backupPath)
	}
	return nil
}

func revalidatePlannedActive(activePath string, planned sqliteFileSetSnapshot, deps recoveryOperationDeps) error {
	if err := deps.ensureSidecars(activePath); err != nil {
		return err
	}
	return revalidateSnapshot(activePath, planned, deps)
}

func revalidateSnapshot(path string, planned sqliteFileSetSnapshot, deps recoveryOperationDeps) error {
	got, err := deps.fingerprintSet(path)
	if err != nil {
		return err
	}
	if !sameSQLiteFileSetSnapshot(got, planned) {
		return fmt.Errorf("file set no longer matches planned snapshot")
	}
	return nil
}

func revalidateMainSnapshot(path string, planned sqliteFileSetSnapshot, deps recoveryOperationDeps) error {
	got, err := deps.fingerprintSet(path)
	if err != nil {
		return err
	}
	if !sameSQLiteFileSnapshot(got.main, planned.main) {
		return fmt.Errorf("main file no longer matches planned snapshot")
	}
	return nil
}

type sqliteFileSetSnapshot struct {
	main     sqliteFileSnapshot
	sidecars map[string]sqliteFileSnapshot
}

type sqliteFileSnapshot struct {
	sha256  string
	size    int64
	mode    os.FileMode
	modTime time.Time
	info    os.FileInfo
}

func snapshotSQLiteFileSet(dbPath string) (sqliteFileSetSnapshot, error) {
	main, err := snapshotSQLiteFile(dbPath)
	if err != nil {
		return sqliteFileSetSnapshot{}, err
	}
	sidecars := map[string]sqliteFileSnapshot{}
	for _, suffix := range sqliteSidecarSuffixes() {
		path := dbPath + suffix
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return sqliteFileSetSnapshot{}, fmt.Errorf("lstat sidecar %s: %w", path, err)
		}
		snap, err := snapshotSQLiteFile(path)
		if err != nil {
			return sqliteFileSetSnapshot{}, fmt.Errorf("snapshot sidecar %s: %w", path, err)
		}
		sidecars[suffix] = snap
	}
	return sqliteFileSetSnapshot{main: main, sidecars: sidecars}, nil
}

type snapshotFile interface {
	io.Reader
	Stat() (os.FileInfo, error)
	Close() error
}

func snapshotSQLiteFile(path string) (sqliteFileSnapshot, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return sqliteFileSnapshot{}, err
	}
	if !pathInfo.Mode().IsRegular() {
		return sqliteFileSnapshot{}, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return sqliteFileSnapshot{}, err
	}
	return snapshotSQLiteOpenFile(path, pathInfo, file)
}

func snapshotSQLiteOpenFile(path string, pathInfo os.FileInfo, file snapshotFile) (snapshot sqliteFileSnapshot, err error) {
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close fingerprint file %s: %w", path, closeErr))
		}
	}()
	openInfo, err := file.Stat()
	if err != nil {
		return sqliteFileSnapshot{}, err
	}
	if !os.SameFile(pathInfo, openInfo) {
		return sqliteFileSnapshot{}, fmt.Errorf("%s changed between lstat and open", path)
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return sqliteFileSnapshot{}, err
	}
	closeReadInfo, err := file.Stat()
	if err != nil {
		return sqliteFileSnapshot{}, err
	}
	if !os.SameFile(openInfo, closeReadInfo) || closeReadInfo.Size() != openInfo.Size() || !closeReadInfo.ModTime().Equal(openInfo.ModTime()) {
		return sqliteFileSnapshot{}, fmt.Errorf("%s changed while fingerprinting", path)
	}
	pathInfoAfter, err := os.Lstat(path)
	if err != nil {
		return sqliteFileSnapshot{}, err
	}
	if !os.SameFile(openInfo, pathInfoAfter) {
		return sqliteFileSnapshot{}, fmt.Errorf("%s changed after fingerprinting", path)
	}
	return sqliteFileSnapshot{
		sha256:  hex.EncodeToString(h.Sum(nil)),
		size:    openInfo.Size(),
		mode:    openInfo.Mode(),
		modTime: openInfo.ModTime(),
		info:    openInfo,
	}, nil
}

func sameSQLiteFileSetSnapshot(left, right sqliteFileSetSnapshot) bool {
	if !sameSQLiteFileSnapshot(left.main, right.main) {
		return false
	}
	if len(left.sidecars) != len(right.sidecars) {
		return false
	}
	for suffix, leftSnap := range left.sidecars {
		rightSnap, ok := right.sidecars[suffix]
		if !ok || !sameSQLiteFileSnapshot(leftSnap, rightSnap) {
			return false
		}
	}
	return true
}

func sameSQLiteFileSnapshot(left, right sqliteFileSnapshot) bool {
	if left.info == nil || right.info == nil {
		return false
	}
	return left.sha256 == right.sha256 && left.size == right.size && os.SameFile(left.info, right.info)
}

func sortedSnapshotSidecars(snapshot sqliteFileSetSnapshot) []string {
	keys := make([]string, 0, len(snapshot.sidecars))
	for key := range snapshot.sidecars {
		keys = append(keys, key)
	}
	if len(keys) == 2 && keys[0] > keys[1] {
		keys[0], keys[1] = keys[1], keys[0]
	}
	return keys
}

func backupPathCandidate(activePath string, deps recoveryOperationDeps) (string, error) {
	suffix, err := deps.randomHex(3)
	if err != nil {
		return "", err
	}
	base := filepath.Base(activePath)
	if !strings.HasPrefix(base, ".") {
		base = "." + base
	}
	return filepath.Join(filepath.Dir(activePath), fmt.Sprintf("%s.backup-%s-%s", base, time.Now().UTC().Format("20060102T150405Z"), suffix)), nil
}

func intendedBackupDescription(activePath string) string {
	base := filepath.Base(activePath)
	if !strings.HasPrefix(base, ".") {
		base = "." + base
	}
	return filepath.Join(filepath.Dir(activePath), base+".backup-<UTC>-<randomhex>")
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random recovery backup suffix: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

type noClobberUnsupportedError struct {
	oldPath string
	newPath string
	cause   error
}

func (e noClobberUnsupportedError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("atomic no-clobber move unsupported before mutation: old=%s new=%s: %v", e.oldPath, e.newPath, e.cause)
	}
	return fmt.Sprintf("atomic no-clobber move unsupported before mutation: old=%s new=%s", e.oldPath, e.newPath)
}

func (e noClobberUnsupportedError) Unwrap() error { return e.cause }

func removeSQLiteFileSet(dbPath string) error {
	var errs []error
	for _, path := range append([]string{dbPath}, sqliteSidecarPaths(dbPath)...) {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			errs = append(errs, fmt.Errorf("remove SQLite recovery file %s: %w", path, removeErr))
		}
	}
	return errors.Join(errs...)
}

func sqliteSidecarPaths(dbPath string) []string {
	suffixes := sqliteSidecarSuffixes()
	paths := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		paths = append(paths, dbPath+suffix)
	}
	return paths
}

func sqliteSidecarSuffixes() []string {
	return []string{"-wal", "-shm", "-journal"}
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}

func resolvePath(path string) (resolvedPath, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve database path %s: %w", path, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return resolvedPath{path: abs}, nil
		}
		return resolvedPath{}, fmt.Errorf("stat database path %s: %w", abs, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if strings.Contains(err.Error(), "too many links") {
			resolved = abs
		} else {
			return resolvedPath{}, fmt.Errorf("resolve database symlink %s: %w", abs, err)
		}
	}
	info, err = os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return resolvedPath{path: resolved}, nil
		}
		return resolvedPath{}, fmt.Errorf("stat database path %s: %w", resolved, err)
	}
	return resolvedPath{path: resolved, info: info}, nil
}

func knownPathDescription(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func sameFile(left, right resolvedPath) bool {
	if left.info == nil || right.info == nil {
		return false
	}
	return os.SameFile(left.info, right.info)
}

func recoveryOpenError(path string, err error, dryRun bool) error {
	mode := "recovery apply"
	if dryRun {
		mode = "recovery dry-run"
	}
	if errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
		return fmt.Errorf("%s cannot inspect %s without side effects while its WAL has uncheckpointed frames; close the writer or checkpoint the database, then retry: %w", mode, path, err)
	}
	return fmt.Errorf("open %s for %s without side effects: %w", path, mode, err)
}

func diagnosticError(diag compat.Diagnostic) error {
	return fmt.Errorf("%s: %s", diag.Code, diag.Summary)
}
