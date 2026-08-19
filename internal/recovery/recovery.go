package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// Execute resolves active and stranded database paths, reads distinct inputs in
// immutable read-only mode, and runs the deterministic compatibility recovery planner.
func Execute(ctx context.Context, opts Options) (report Report, err error) {
	if opts.ActivePath == "" {
		return Report{}, fmt.Errorf("active database path is required")
	}
	if opts.FromPath == "" {
		return Report{}, fmt.Errorf("--from database path is required")
	}

	active, err := resolvePath(opts.ActivePath)
	if err != nil {
		return Report{}, err
	}
	from, err := resolvePath(opts.FromPath)
	if err != nil {
		return Report{}, err
	}

	report.ActivePath = active.path
	report.BackupPath = active.path + ".recovery-backup"

	if !opts.DryRun {
		return report, fmt.Errorf("recovery apply is not available yet; rerun with --dry-run to inspect the plan")
	}

	paths := []resolvedPath{active}
	if !sameFile(active, from) {
		paths = append(paths, from)
	}

	handles := make([]*storage.Database, 0, len(paths))
	defer func() {
		for i := len(handles) - 1; i >= 0; i-- {
			if closeErr := handles[i].Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
	}()

	inputs := make([]compat.RecoveryInput, 0, len(paths))
	for _, path := range paths {
		db, openErr := storage.OpenImmutableReadOnly(path.path)
		if openErr != nil {
			return report, recoveryOpenError(path.path, openErr)
		}
		handles = append(handles, db)

		input, diag, readErr := storage.ReadRecoveryInput(ctx, db)
		if readErr != nil {
			return report, readErr
		}
		if diag != nil {
			return report, diagnosticError(*diag)
		}
		inputs = append(inputs, input)
		report.InputCounts = append(report.InputCounts, input.RowCount)
	}

	plan, diagnostics, planErr := compat.PlanRecovery(inputs)
	if planErr != nil {
		return report, planErr
	}
	report.Shapes = append(report.Shapes, plan.InputShapes...)
	report.ExactDuplicates = plan.ExactDuplicates
	report.FinalCount = len(plan.Records)
	report.Conflicts = append(report.Conflicts, diagnostics...)
	if len(diagnostics) != 0 {
		return report, fmt.Errorf("recovery plan has %d diagnostic(s)", len(diagnostics))
	}
	return report, nil
}

func resolvePath(path string) (resolvedPath, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve database path %s: %w", path, err)
	}
	resolved := abs
	if realPath, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = realPath
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return resolvedPath{path: resolved}, nil
		}
		return resolvedPath{}, fmt.Errorf("stat database path %s: %w", resolved, err)
	}
	return resolvedPath{path: resolved, info: info}, nil
}

func sameFile(left, right resolvedPath) bool {
	if left.info == nil || right.info == nil {
		return false
	}
	return os.SameFile(left.info, right.info)
}

func recoveryOpenError(path string, err error) error {
	if errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
		return fmt.Errorf("recovery dry-run cannot inspect %s without side effects while its WAL has uncheckpointed frames; close the writer or checkpoint the database, then retry: %w", path, err)
	}
	return fmt.Errorf("open %s for recovery dry-run without side effects: %w", path, err)
}

func diagnosticError(diag compat.Diagnostic) error {
	return fmt.Errorf("%s: %s", diag.Code, diag.Summary)
}
