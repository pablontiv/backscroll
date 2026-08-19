package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestRecoverAtomicallyReplacesAndPreservesActiveBackup(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "original active row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "rescued stranded row",
		UUID:        "22222222-2222-4222-8222-222222222222",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})

	activeBefore := snapshotRecoveryDBFile(t, activePath)
	strandedBefore := snapshotRecoveryDBFile(t, fromPath)

	dryRun, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath, DryRun: true})
	if err != nil {
		t.Fatalf("Execute dry run: %v", err)
	}
	if dryRun.FinalCount != 2 || !reflect.DeepEqual(dryRun.InputCounts, []int{1, 1}) {
		t.Fatalf("dry-run report = %+v, want final count 2 and one row per input", dryRun)
	}
	assertRecoveryDBFileSnapshot(t, "active after dry-run", activePath, activeBefore)
	assertRecoveryDBFileSnapshot(t, "stranded after dry-run", fromPath, strandedBefore)

	report, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
	if err != nil {
		t.Fatalf("Execute apply: %v", err)
	}
	if report.ActivePath != canonicalRecoveryTestPath(t, activePath) {
		t.Fatalf("ActivePath = %q, want resolved active path", report.ActivePath)
	}
	if report.BackupPath == "" || report.BackupPath == activePath || filepath.Dir(report.BackupPath) != filepath.Dir(report.ActivePath) {
		t.Fatalf("BackupPath = %q, want unique sibling of active %q", report.BackupPath, report.ActivePath)
	}
	if !strings.Contains(filepath.Base(report.BackupPath), filepath.Base(report.ActivePath)+".backup-") {
		t.Fatalf("BackupPath = %q, want preserved active basename with backup suffix", report.BackupPath)
	}
	if report.FinalCount != 2 || !reflect.DeepEqual(report.InputCounts, []int{1, 1}) {
		t.Fatalf("apply report = %+v, want final count 2 and one row per input", report)
	}
	backup := snapshotRecoveryDBFile(t, report.BackupPath)
	if !bytes.Equal(backup.main.data, activeBefore.main.data) {
		t.Fatalf("backup bytes differ from original active bytes")
	}
	assertRecoveryDBFileSnapshot(t, "stranded after apply", fromPath, strandedBefore)
	assertRecoveryTexts(t, activePath, []string{"original active row", "rescued stranded row"})
	if _, err := os.Stat(report.BackupPath); err != nil {
		t.Fatalf("backup disappeared after successful recovery: %v", err)
	}
}

func TestRecoverStrandedSourceIsReadOnly(t *testing.T) {
	t.Run("conflict failure", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "active conflicting payload",
			UUID:        "33333333-3333-4333-8333-333333333333",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		}})
		createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "stranded conflicting payload",
			UUID:        "33333333-3333-4333-8333-333333333333",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		}})
		activeBefore := snapshotRecoveryDBFile(t, activePath)
		strandedBefore := snapshotRecoveryDBFile(t, fromPath)

		_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
		if err == nil {
			t.Fatal("Execute conflicting apply succeeded; want complete-plan failure before replacement")
		}
		assertRecoveryDBFileSnapshot(t, "active after conflict", activePath, activeBefore)
		assertRecoveryDBFileSnapshot(t, "stranded after conflict", fromPath, strandedBefore)
	})

	t.Run("replacement failure", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "active survives replacement failure",
			UUID:        "44444444-4444-4444-8444-444444444444",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		}})
		createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
			Ordinal:     0,
			Role:        "assistant",
			Text:        "stranded remains immutable",
			UUID:        "55555555-5555-4555-8555-555555555555",
			Timestamp:   "2026-08-18T00:01:00Z",
			ContentType: "text",
		}})
		activeBefore := snapshotRecoveryDBFile(t, activePath)
		strandedBefore := snapshotRecoveryDBFile(t, fromPath)

		resolvedActivePath := canonicalRecoveryTestPath(t, activePath)
		deps := defaultRecoveryOperationDeps()
		originalMove := deps.installMove
		deps.installMove = func(oldPath, newPath string) error {
			if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActivePath {
				return fmt.Errorf("injected replacement rename failure")
			}
			return originalMove(oldPath, newPath)
		}

		_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
		if err == nil {
			t.Fatal("Execute with replacement rename failure succeeded; want explicit failure")
		}
		if !strings.Contains(err.Error(), "restored active") {
			t.Fatalf("Execute error = %v, want restored active diagnostic", err)
		}
		assertRecoveryDBFileSnapshot(t, "active after replacement failure", activePath, activeBefore)
		assertRecoveryDBFileSnapshot(t, "stranded after replacement failure", fromPath, strandedBefore)
	})
}

func TestRecoverReplacementFailureRestoresActive(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "active before final rename failure",
		UUID:        "66666666-6666-4666-8666-666666666666",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "not installed because final rename fails",
		UUID:        "77777777-7777-4777-8777-777777777777",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})
	activeBefore := snapshotRecoveryDBFile(t, activePath)

	resolvedActivePath := canonicalRecoveryTestPath(t, activePath)
	deps := defaultRecoveryOperationDeps()
	originalMove := deps.installMove
	deps.installMove = func(oldPath, newPath string) error {
		if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActivePath {
			return fmt.Errorf("injected final rename failure")
		}
		return originalMove(oldPath, newPath)
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil {
		t.Fatal("Execute with final rename failure succeeded; want failure")
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.FromPath != canonicalRecoveryTestPath(t, fromPath) {
		t.Fatalf("failure FromPath = %q, want canonical stranded path", failure.FromPath)
	}
	if !strings.Contains(err.Error(), "restored active") {
		t.Fatalf("Execute error = %v, want restored active diagnostic", err)
	}
	assertRecoveryDBFileSnapshot(t, "active after final rename failure", activePath, activeBefore)
}

func TestRecoverNeverDeletesBackup(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "active backed up once",
		UUID:        "88888888-8888-4888-8888-888888888888",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "stranded installed once",
		UUID:        "99999999-9999-4999-8999-999999999999",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})

	first, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
	if err != nil {
		t.Fatalf("first recovery: %v", err)
	}
	second, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: activePath})
	if err != nil {
		t.Fatalf("same-path recovery after first backup: %v", err)
	}
	if first.BackupPath == second.BackupPath {
		t.Fatalf("backup paths reused: %q", first.BackupPath)
	}
	for _, path := range []string{first.BackupPath, second.BackupPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("backup %s missing after later recovery: %v", path, err)
		}
	}
}

func TestRecoverReplacementFaultsAreOperationLocalAndTyped(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(t *testing.T, activePath string, deps *recoveryOperationDeps)
		wantPhase replacementPhase
	}{
		{
			name: "first backup operation fails before consuming active",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.noClobberMove = func(oldPath, newPath string) error {
					if oldPath == resolvedActive {
						return fmt.Errorf("injected backup main failure")
					}
					return noClobberMove(oldPath, newPath)
				}
			},
			wantPhase: phaseBackupMain,
		},
		{
			name: "destination fsync failure happens before install",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				deps.syncFile = func(path string) error { return fmt.Errorf("injected destination fsync failure") }
			},
			wantPhase: phaseDestinationSync,
		},
		{
			name: "first backup directory fsync restores active",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				deps.syncDir = func(path string) error { return fmt.Errorf("injected first directory fsync failure") }
			},
			wantPhase: phaseBackupDirSync,
		},
		{
			name: "install failure restores active",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				originalMove := deps.installMove
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.installMove = func(oldPath, newPath string) error {
					if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
						return fmt.Errorf("injected install failure")
					}
					return originalMove(oldPath, newPath)
				}
			},
			wantPhase: phaseInstall,
		},
		{
			name: "second directory fsync leaves durable backup",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				calls := 0
				originalSyncDir := deps.syncDir
				deps.syncDir = func(path string) error {
					calls++
					if calls == 2 {
						return fmt.Errorf("injected second directory fsync failure")
					}
					return originalSyncDir(path)
				}
			},
			wantPhase: phaseInstallSync,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "active.db")
			fromPath := filepath.Join(dir, "stranded.db")
			createRecoveryDB(t, activePath, []storage.IndexedMessage{{
				Ordinal: 0, Role: "user", Text: "active typed fault", UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Timestamp: "2026-08-18T00:00:00Z", ContentType: "text",
			}})
			createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
				Ordinal: 0, Role: "assistant", Text: "stranded typed fault", UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Timestamp: "2026-08-18T00:01:00Z", ContentType: "text",
			}})
			activeBefore := snapshotRecoveryDBFile(t, activePath)
			deps := defaultRecoveryOperationDeps()
			tt.configure(t, activePath, &deps)

			report, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
			if err == nil {
				t.Fatal("executeWithDeps succeeded; want injected typed replacement failure")
			}
			var replacementErr *replacementError
			if !errors.As(err, &replacementErr) {
				t.Fatalf("error %T %[1]v, want *replacementError", err)
			}
			if replacementErr.Phase != tt.wantPhase {
				t.Fatalf("phase = %s, want %s", replacementErr.Phase, tt.wantPhase)
			}
			if tt.wantPhase == phaseInstallSync {
				if report.BackupPath == "" || !replacementErr.RestorableBackup {
					t.Fatalf("second fsync failure consumed backup: report=%+v err=%+v", report, replacementErr)
				}
				backup := snapshotRecoveryDBFile(t, report.BackupPath)
				if !bytes.Equal(backup.main.data, activeBefore.main.data) {
					t.Fatalf("durable backup after second fsync failure differs from original active")
				}
			} else if tt.wantPhase == phaseBackupMain || tt.wantPhase == phaseBackupFileSync || tt.wantPhase == phaseBackupDirSync || tt.wantPhase == phaseInstall {
				assertRecoveryDBFileSnapshot(t, "active after restorative failure", activePath, activeBefore)
			}
		})
	}
}

func TestRecoverReplacementStatesAreStructurallyTruthful(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		configure              func(t *testing.T, activePath string, deps *recoveryOperationDeps)
		wantPhase              replacementPhase
		wantState              replacementState
		wantRestorable         bool
		wantBackupVisible      bool
		wantBackupVerified     bool
		wantBackupUnknown      bool
		wantRestoredVerified   bool
		wantRestoredVisible    bool
		wantReplacementVisible bool
		wantReplacementDurable bool
		wantNoActiveMutation   bool
		wantInventory          string
		wantCleanupErr         bool
		wantMilestones         ApplyFailureMilestones
	}{
		{
			name: "first backup failure does not advertise candidate as visible",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.noClobberMove = func(oldPath, newPath string) error {
					if oldPath == resolvedActive {
						return fmt.Errorf("injected first backup failure")
					}
					return noClobberMove(oldPath, newPath)
				}
			},
			wantPhase:            phaseBackupMain,
			wantState:            replacementStateNoActiveMutation,
			wantNoActiveMutation: true,
			wantInventory:        "unchanged",
		},
		{
			name: "backup file fsync failure restores active only after verification",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				deps.syncFile = func(path string) error {
					if strings.Contains(filepath.Base(path), ".backup-") {
						return fmt.Errorf("injected backup file fsync failure")
					}
					return syncFile(path)
				}
			},
			wantPhase:            phaseBackupFileSync,
			wantState:            replacementStateActiveRestoredDurableVerified,
			wantRestoredVerified: true,
			wantRestoredVisible:  true,
			wantInventory:        "unchanged",
			wantMilestones:       ApplyFailureMilestones{BackupCreated: true, RestoredActiveVisible: true, RestoredActiveDurableVerified: true},
		},
		{
			name: "first directory fsync failure restores active only after verification",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				calls := 0
				deps.syncDir = func(path string) error {
					calls++
					if calls == 1 {
						return fmt.Errorf("injected first directory fsync failure")
					}
					return syncDir(path)
				}
			},
			wantPhase:            phaseBackupDirSync,
			wantState:            replacementStateActiveRestoredDurableVerified,
			wantRestoredVerified: true,
			wantRestoredVisible:  true,
			wantInventory:        "unchanged",
			wantMilestones:       ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, RestoredActiveVisible: true, RestoredActiveDurableVerified: true},
		},
		{
			name: "install failure restores active only after verification",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.installMove = func(oldPath, newPath string) error {
					if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
						return fmt.Errorf("injected install failure")
					}
					return noClobberMove(oldPath, newPath)
				}
			},
			wantPhase:            phaseInstall,
			wantState:            replacementStateActiveRestoredDurableVerified,
			wantRestoredVerified: true,
			wantRestoredVisible:  true,
			wantInventory:        "unchanged",
			wantMilestones:       ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, RestoredActiveVisible: true, RestoredActiveDurableVerified: true},
		},
		{
			name: "second directory fsync failure reports visible replacement and restorable verified backup",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				calls := 0
				deps.syncDir = func(path string) error {
					calls++
					if calls == 2 {
						return fmt.Errorf("injected second directory fsync failure")
					}
					return syncDir(path)
				}
			},
			wantPhase:              phaseInstallSync,
			wantState:              replacementStateReplacementVisible,
			wantRestorable:         true,
			wantBackupVisible:      true,
			wantBackupVerified:     true,
			wantReplacementVisible: true,
			wantInventory:          "replacement-and-backup",
			wantMilestones:         ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, ReplacementVisible: true},
		},
		{
			name: "pre-install final backup mismatch restores active; backup uncertainty remains historical only",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				original := deps.fingerprintSet
				backupFingerprints := 0
				deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
					snapshot, err := original(path)
					if err == nil && strings.Contains(filepath.Base(path), ".backup-") {
						backupFingerprints++
						if backupFingerprints == 2 {
							snapshot.main.sha256 = "sha256:injected-mismatch"
						}
					}
					return snapshot, err
				}
			},
			wantPhase:            phasePostBackupVerify,
			wantState:            replacementStateActiveRestoredDurableVerified,
			wantRestoredVerified: true,
			wantRestoredVisible:  true,
			wantInventory:        "unchanged",
			wantMilestones:       ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, RestoredActiveVisible: true, RestoredActiveDurableVerified: true},
		},
		{
			name: "post-install backup mismatch emits replacement durable and not restorable verified",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				original := deps.fingerprintSet
				backupFingerprints := 0
				deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
					snapshot, err := original(path)
					if err == nil && strings.Contains(filepath.Base(path), ".backup-") {
						backupFingerprints++
						if backupFingerprints == 3 {
							snapshot.main.sha256 = "sha256:post-install-backup-mismatch"
						}
					}
					return snapshot, err
				}
			},
			wantPhase:              phaseInstallSync,
			wantState:              replacementStateReplacementDurable,
			wantBackupVisible:      true,
			wantBackupUnknown:      true,
			wantReplacementVisible: true,
			wantReplacementDurable: true,
			wantInventory:          "replacement-and-backup",
			wantMilestones:         ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, ReplacementVisible: true, ReplacementDurable: true},
		},
		{
			name: "restore move failure is split unknown and preserves verified backup path",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.installMove = func(oldPath, newPath string) error {
					if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
						return fmt.Errorf("injected install before restore move failure")
					}
					return noClobberMove(oldPath, newPath)
				}
				deps.restoreMove = func(oldPath, newPath string) error {
					if newPath == resolvedActive {
						return fmt.Errorf("injected restore move failure")
					}
					return noClobberMove(oldPath, newPath)
				}
			},
			wantPhase:          phaseInstall,
			wantState:          replacementStateSplitUnknown,
			wantRestorable:     true,
			wantBackupVisible:  true,
			wantBackupVerified: true,
			wantInventory:      "backup-only",
			wantMilestones:     ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true},
		},
		{
			name: "restore fsync failure reports visible restored active with unknown integrity",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				restoredMain := false
				deps.installMove = func(oldPath, newPath string) error {
					if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
						return fmt.Errorf("injected install before restore fsync failure")
					}
					return noClobberMove(oldPath, newPath)
				}
				deps.restoreMove = func(oldPath, newPath string) error {
					if err := noClobberMove(oldPath, newPath); err != nil {
						return err
					}
					if newPath == resolvedActive {
						restoredMain = true
					}
					return nil
				}
				deps.syncFile = func(path string) error {
					if restoredMain && path == resolvedActive {
						return fmt.Errorf("injected restored active fsync failure")
					}
					return syncFile(path)
				}
			},
			wantPhase:            phaseInstall,
			wantState:            replacementStateActiveRestoredVisibleUnknown,
			wantRestoredVisible:  true,
			wantRestoredVerified: false,
			wantInventory:        "unchanged",
			wantMilestones:       ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, RestoredActiveVisible: true},
		},
		{
			name: "restore fingerprint failure reports visible restored active with unknown integrity",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				restoredMain := false
				deps.installMove = func(oldPath, newPath string) error {
					if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
						return fmt.Errorf("injected install before restore fingerprint failure")
					}
					return noClobberMove(oldPath, newPath)
				}
				deps.restoreMove = func(oldPath, newPath string) error {
					if err := noClobberMove(oldPath, newPath); err != nil {
						return err
					}
					if newPath == resolvedActive {
						restoredMain = true
					}
					return nil
				}
				original := deps.fingerprintSet
				deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
					snapshot, err := original(path)
					if err == nil && restoredMain && path == resolvedActive {
						snapshot.main.sha256 = "sha256:restored-fingerprint-mismatch"
					}
					return snapshot, err
				}
			},
			wantPhase:           phaseInstall,
			wantState:           replacementStateActiveRestoredVisibleUnknown,
			wantRestoredVisible: true,
			wantInventory:       "unchanged",
			wantMilestones:      ApplyFailureMilestones{BackupCreated: true, BackupFileSynced: true, BackupDirectoryDurable: true, BackupIntegrityVerified: true, RestoredActiveVisible: true},
		},
		{
			name: "unsupported no-clobber reports no active mutation and cleanup failure without losing primary error",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				deps.noClobberMove = func(oldPath, newPath string) error {
					if oldPath == resolvedActive {
						return noClobberUnsupportedError{oldPath: oldPath, newPath: newPath}
					}
					return noClobberMove(oldPath, newPath)
				}
				originalCleanup := deps.cleanupFileSet
				deps.cleanupFileSet = func(path string) error {
					if err := originalCleanup(path); err != nil {
						return err
					}
					return fmt.Errorf("injected cleanup visibility failure")
				}
			},
			wantPhase:            phaseBackupMain,
			wantState:            replacementStateNoActiveMutation,
			wantNoActiveMutation: true,
			wantCleanupErr:       true,
			wantInventory:        "unchanged",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "active.db")
			fromPath := filepath.Join(dir, "stranded.db")
			createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active truthful state", UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Timestamp: "2026-08-18T00:00:00Z", ContentType: "text"}})
			createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded truthful state", UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Timestamp: "2026-08-18T00:01:00Z", ContentType: "text"}})
			activeBefore := snapshotRecoveryDBFile(t, activePath)
			beforeInventory := recoveryDirectoryInventory(t, dir)
			deps := defaultRecoveryOperationDeps()
			deps.randomHex = deterministicRecoveryHex()
			tt.configure(t, activePath, &deps)

			report, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
			if err == nil {
				t.Fatal("executeWithDeps succeeded; want injected replacement failure")
			}
			var replacementErr *replacementError
			if !errors.As(err, &replacementErr) {
				t.Fatalf("error %T %[1]v, want *replacementError", err)
			}
			if replacementErr.Phase != tt.wantPhase || replacementErr.State != tt.wantState {
				t.Fatalf("phase/state = %s/%s, want %s/%s; err=%v", replacementErr.Phase, replacementErr.State, tt.wantPhase, tt.wantState, err)
			}
			assertReplacementErrorPaths(t, replacementErr, activePath)
			if replacementErr.RestorableBackup != tt.wantRestorable {
				t.Fatalf("RestorableBackup = %v, want %v; err=%+v", replacementErr.RestorableBackup, tt.wantRestorable, replacementErr)
			}
			if tt.wantRestorable {
				if replacementErr.CurrentRestorablePath == "" || replacementErr.CurrentRestorablePath != replacementErr.RestorableBackupPath {
					t.Fatalf("CurrentRestorablePath/RestorableBackupPath = %q/%q, want matching current path", replacementErr.CurrentRestorablePath, replacementErr.RestorableBackupPath)
				}
			} else if replacementErr.CurrentRestorablePath != "" || replacementErr.RestorableBackupPath != "" {
				t.Fatalf("restorable paths = %q/%q, want empty when not currently restorable", replacementErr.CurrentRestorablePath, replacementErr.RestorableBackupPath)
			}
			if replacementErr.BackupVisible != tt.wantBackupVisible || replacementErr.BackupIntegrityVerified != tt.wantBackupVerified || replacementErr.BackupIntegrityUnknown != tt.wantBackupUnknown {
				t.Fatalf("backup fields visible/verified/unknown = %v/%v/%v, want %v/%v/%v; err=%+v", replacementErr.BackupVisible, replacementErr.BackupIntegrityVerified, replacementErr.BackupIntegrityUnknown, tt.wantBackupVisible, tt.wantBackupVerified, tt.wantBackupUnknown, replacementErr)
			}
			if replacementErr.RestoredActiveVisible != tt.wantRestoredVisible || replacementErr.RestoredActiveDurableVerified != tt.wantRestoredVerified {
				t.Fatalf("restore fields visible/durableVerified = %v/%v, want %v/%v; err=%+v", replacementErr.RestoredActiveVisible, replacementErr.RestoredActiveDurableVerified, tt.wantRestoredVisible, tt.wantRestoredVerified, replacementErr)
			}
			if replacementErr.ReplacementVisible != tt.wantReplacementVisible || replacementErr.ReplacementDurable != tt.wantReplacementDurable {
				t.Fatalf("replacement fields visible/durable = %v/%v, want %v/%v; err=%+v", replacementErr.ReplacementVisible, replacementErr.ReplacementDurable, tt.wantReplacementVisible, tt.wantReplacementDurable, replacementErr)
			}
			if replacementErr.NoActiveMutation != tt.wantNoActiveMutation {
				t.Fatalf("NoActiveMutation = %v, want %v", replacementErr.NoActiveMutation, tt.wantNoActiveMutation)
			}
			if (replacementErr.CleanupErr != nil) != tt.wantCleanupErr {
				t.Fatalf("CleanupErr = %v, want present=%v", replacementErr.CleanupErr, tt.wantCleanupErr)
			}
			if !reflect.DeepEqual(replacementErr.Milestones, tt.wantMilestones) {
				t.Fatalf("Milestones = %+v, want %+v", replacementErr.Milestones, tt.wantMilestones)
			}
			assertNoRecoveryTemps(t, dir)
			assertRecoveryFaultInventory(t, tt.wantInventory, dir, activePath, fromPath, replacementErr.BackupPath, activeBefore, beforeInventory, report.BackupPath)
		})
	}
}

func TestRecoverPreReplacementCloseAndCleanupErrorsAreTyped(t *testing.T) {
	for _, tt := range []struct {
		name        string
		configure   func(t *testing.T, activePath string, deps *recoveryOperationDeps)
		wantPhase   replacementPhase
		wantClose   bool
		wantCleanup bool
	}{
		{
			name: "reservation close failure closes before active mutation and removes temp",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				originalClose := deps.closeReservation
				deps.closeReservation = func(r *activeReservation) error {
					if err := originalClose(r); err != nil {
						return err
					}
					return fmt.Errorf("injected reservation close failure")
				}
			},
			wantPhase: phaseReservationClose,
			wantClose: true,
		},
		{
			name: "pre replacement cleanup failure is joined without overwriting primary error",
			configure: func(t *testing.T, activePath string, deps *recoveryOperationDeps) {
				resolvedActive := canonicalRecoveryTestPath(t, activePath)
				originalFingerprint := deps.fingerprintSet
				activeFingerprints := 0
				deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
					snapshot, err := originalFingerprint(path)
					if err == nil && path == resolvedActive {
						activeFingerprints++
						if activeFingerprints == 3 {
							snapshot.main.sha256 = "sha256:pre-replacement-mismatch"
						}
					}
					return snapshot, err
				}
				originalCleanup := deps.cleanupFileSet
				deps.cleanupFileSet = func(path string) error {
					_ = originalCleanup(path)
					return fmt.Errorf("injected pre-replacement cleanup failure")
				}
			},
			wantPhase:   phasePreBackupValidate,
			wantCleanup: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "active.db")
			fromPath := filepath.Join(dir, "stranded.db")
			createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active pre replacement", UUID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", ContentType: "text"}})
			createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded pre replacement", UUID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", ContentType: "text"}})
			before := recoveryDirectoryInventory(t, dir)
			deps := defaultRecoveryOperationDeps()
			deps.randomHex = deterministicRecoveryHex()
			tt.configure(t, activePath, &deps)

			_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
			if err == nil {
				t.Fatal("executeWithDeps succeeded; want pre-replacement failure")
			}
			var preErr *preReplacementError
			if !errors.As(err, &preErr) {
				t.Fatalf("error %T %[1]v, want *preReplacementError", err)
			}
			if preErr.Phase != tt.wantPhase || preErr.State != replacementStateNoActiveMutation {
				t.Fatalf("phase/state = %s/%s, want %s/%s", preErr.Phase, preErr.State, tt.wantPhase, replacementStateNoActiveMutation)
			}
			if preErr.ActivePath != canonicalRecoveryTestPath(t, activePath) || preErr.TempPath == "" {
				t.Fatalf("pre-replacement paths = active %q temp %q", preErr.ActivePath, preErr.TempPath)
			}
			if (preErr.CloseErr != nil) != tt.wantClose || (preErr.CleanupErr != nil) != tt.wantCleanup {
				t.Fatalf("close/cleanup errs = %v/%v, want present %v/%v", preErr.CloseErr, preErr.CleanupErr, tt.wantClose, tt.wantCleanup)
			}
			assertNoRecoveryTemps(t, dir)
			if after := recoveryDirectoryInventory(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("pre-replacement failure mutated inventory\nbefore: %s\nafter:  %s", describeRecoveryInventory(before), describeRecoveryInventory(after))
			}
		})
	}
}

func TestRecoverPreReplacementFailureJoinsPrimaryCleanupAndClose(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active joined errors", UUID: "90909090-9090-4090-8090-909090909090", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded joined errors", UUID: "91919191-9191-4191-8191-919191919191", ContentType: "text"}})
	primaryErr := errors.New("injected primary pre-replacement validation failure")
	cleanupErr := errors.New("injected cleanup failure")
	closeErr := errors.New("injected reservation close failure")
	resolvedActive := canonicalRecoveryTestPath(t, activePath)
	deps := defaultRecoveryOperationDeps()
	deps.randomHex = deterministicRecoveryHex()
	originalFingerprint := deps.fingerprintSet
	activeFingerprints := 0
	deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
		if path == resolvedActive {
			activeFingerprints++
			if activeFingerprints == 3 {
				return sqliteFileSetSnapshot{}, primaryErr
			}
		}
		return originalFingerprint(path)
	}
	originalCleanup := deps.cleanupFileSet
	deps.cleanupFileSet = func(path string) error {
		_ = originalCleanup(path)
		return cleanupErr
	}
	originalClose := deps.closeReservation
	deps.closeReservation = func(r *activeReservation) error {
		return errors.Join(originalClose(r), closeErr)
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil {
		t.Fatal("executeWithDeps succeeded; want joined primary cleanup and close errors")
	}
	for _, want := range []error{primaryErr, cleanupErr, closeErr} {
		if !errors.Is(err, want) {
			t.Fatalf("error %v does not contain %v", err, want)
		}
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) || failure.CleanupErr == nil {
		t.Fatalf("error %T %[1]v, want ApplyFailure with cleanup", err)
	}
}

func TestRecoverNonDryRunPlanningDiagnosticsAreStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	sharedUUID := "01010101-0101-4101-8101-010101010101"
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active conflict", UUID: sharedUUID, ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "stranded conflict", UUID: sharedUUID, ContentType: "text"}})

	report, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
	if err == nil {
		t.Fatal("Execute apply conflict succeeded; want structured planning failure")
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.Phase != phasePlanning || failure.State != replacementStateNoActiveMutation || !failure.NoActiveMutation {
		t.Fatalf("failure phase/state/no-active-mutation = %s/%s/%v", failure.Phase, failure.State, failure.NoActiveMutation)
	}
	if failure.ActivePath != canonicalRecoveryTestPath(t, activePath) || failure.FromPath != canonicalRecoveryTestPath(t, fromPath) || failure.TempPath != "" || failure.BackupPath != "" {
		t.Fatalf("failure paths active=%q from=%q backup=%q temp=%q", failure.ActivePath, failure.FromPath, failure.BackupPath, failure.TempPath)
	}
	if len(report.Conflicts) == 0 || report.FinalCount != 0 {
		t.Fatalf("report conflicts/final = %d/%d, want diagnostics and no final records", len(report.Conflicts), report.FinalCount)
	}
}

func TestRecoverNonDryRunMissingPathsAreStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")

	for _, tt := range []struct {
		name       string
		opts       Options
		wantActive string
		wantFrom   string
	}{
		{
			name:     "missing active",
			opts:     Options{FromPath: fromPath},
			wantFrom: knownPathDescription(fromPath),
		},
		{
			name:       "missing from",
			opts:       Options{ActivePath: activePath},
			wantActive: knownPathDescription(activePath),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Execute(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("Execute succeeded; want structured missing path failure")
			}
			var failure *ApplyFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
			}
			if failure.Phase != phaseSourceRead || failure.State != replacementStateNoActiveMutation || !failure.NoActiveMutation {
				t.Fatalf("failure phase/state/no-active-mutation = %s/%s/%v", failure.Phase, failure.State, failure.NoActiveMutation)
			}
			if failure.ActivePath != tt.wantActive || failure.FromPath != tt.wantFrom {
				t.Fatalf("failure paths active=%q from=%q, want active=%q from=%q", failure.ActivePath, failure.FromPath, tt.wantActive, tt.wantFrom)
			}
		})
	}
}

func TestRecoverDryRunMissingPathsRemainValidationErrors(t *testing.T) {
	_, err := Execute(context.Background(), Options{DryRun: true})
	if err == nil {
		t.Fatal("dry-run Execute succeeded; want raw validation error")
	}
	var failure *ApplyFailure
	if errors.As(err, &failure) {
		t.Fatalf("dry-run validation error %T %[1]v unexpectedly matches *ApplyFailure", err)
	}
}

func TestRecoverDryRunInputValidationErrorsStayPlain(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "from.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active dry-run validation", UUID: "11111111-1111-4111-8111-111111111111", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "from dry-run validation", UUID: "22222222-2222-4222-8222-222222222222", ContentType: "text"}})
	brokenLink := filepath.Join(dir, "broken-link.db")
	if err := os.Symlink(filepath.Join(dir, "missing-target.db"), brokenLink); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	cases := []struct {
		name string
		opts Options
	}{
		{name: "missing_from", opts: Options{ActivePath: activePath, FromPath: "", DryRun: true}},
		{name: "broken_active", opts: Options{ActivePath: brokenLink, FromPath: fromPath, DryRun: true}},
		{name: "broken_from", opts: Options{ActivePath: activePath, FromPath: brokenLink, DryRun: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Execute(ctx, tc.opts)
			if err == nil {
				t.Fatalf("Execute(%+v) succeeded; want dry-run validation error", tc.opts)
			}
			var failure *ApplyFailure
			if errors.As(err, &failure) {
				t.Fatalf("dry-run error %v was ApplyFailure %+v; want plain validation error", err, failure)
			}
		})
	}
}

func TestRecoverDryRunConflictReportsDiagnosticsWithoutApplyFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "from.db")
	sharedUUID := "33333333-3333-4333-8333-333333333333"
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active conflicting dry run", UUID: sharedUUID, ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "from conflicting dry run", UUID: sharedUUID, ContentType: "text"}})

	report, err := Execute(ctx, Options{ActivePath: activePath, FromPath: fromPath, DryRun: true})
	if err == nil {
		t.Fatal("dry-run conflict succeeded; want diagnostic error")
	}
	var failure *ApplyFailure
	if errors.As(err, &failure) {
		t.Fatalf("dry-run conflict returned ApplyFailure %+v; want plain diagnostic error", failure)
	}
	if len(report.Conflicts) == 0 || report.Conflicts[0].Code != compat.CodeRecoveryConflict {
		t.Fatalf("dry-run conflict report diagnostics = %+v, want recovery conflict", report.Conflicts)
	}
	if report.FinalCount != 0 {
		t.Fatalf("dry-run conflict final count = %d, want no applicable final count", report.FinalCount)
	}
}

func TestApplyFailurePublicErrorDetailsExposeRestorableBackup(t *testing.T) {
	cause := errors.New("primary failure")
	restoreErr := errors.New("restore failed")
	cleanupErr := errors.New("cleanup failed")
	closeErr := errors.New("close failed")
	failure := &ApplyFailure{
		ActivePath:           "active.db",
		FromPath:             "from.db",
		BackupPath:           "active.db.backup",
		TempPath:             "temp.db",
		RestorableBackup:     true,
		RestorableBackupPath: "active.db.backup",
		ActiveRestored:       true,
		Cause:                cause,
		RestoreErr:           restoreErr,
		CleanupErr:           cleanupErr,
		CloseErr:             closeErr,
	}

	text := failure.Error()
	for _, want := range []string{"primary failure", "active=active.db", "from=from.db", "backup=active.db.backup", "temp=temp.db", "restorable backup remains at active.db.backup", "restored active", "restore error: restore failed", "cleanup error: cleanup failed", "close error: close failed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ApplyFailure.Error() missing %q in %q", want, text)
		}
	}
	if got := failure.Unwrap(); len(got) != 4 || got[0] != cause || got[1] != restoreErr || got[2] != cleanupErr || got[3] != closeErr {
		t.Fatalf("ApplyFailure.Unwrap() = %#v, want cause/restore/cleanup/close", got)
	}
	if path, ok := RestorableBackupPath(fmt.Errorf("outer: %w", failure)); !ok || path != "active.db.backup" {
		t.Fatalf("RestorableBackupPath wrapped failure = %q, %v", path, ok)
	}
	if path, ok := RestorableBackupPath(errors.New("unrelated")); ok || path != "" {
		t.Fatalf("RestorableBackupPath unrelated = %q, %v", path, ok)
	}
}

func TestRecoverPathCanonicalizationFailureIsStructured(t *testing.T) {
	dir := t.TempDir()

	for _, tt := range []struct {
		name       string
		setup      func(t *testing.T) (activePath, fromPath string)
		wantActive func(t *testing.T, path string) string
		wantFrom   func(t *testing.T, path string) string
	}{
		{
			name: "active symlink failure",
			setup: func(t *testing.T) (string, string) {
				activePath := filepath.Join(dir, "broken-active-link.db")
				fromPath := filepath.Join(dir, "stranded.db")
				createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded", UUID: "07070707-0707-4707-8707-070707070707", ContentType: "text"}})
				if err := os.Symlink(filepath.Join(dir, "missing-active-target.db"), activePath); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
				return activePath, fromPath
			},
			wantActive: func(t *testing.T, path string) string { return knownPathDescription(path) },
			wantFrom:   func(t *testing.T, path string) string { return knownPathDescription(path) },
		},
		{
			name: "from symlink failure",
			setup: func(t *testing.T) (string, string) {
				activePath := filepath.Join(dir, "active.db")
				fromPath := filepath.Join(dir, "broken-from-link.db")
				createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active", UUID: "08080808-0808-4808-8808-080808080808", ContentType: "text"}})
				if err := os.Symlink(filepath.Join(dir, "missing-from-target.db"), fromPath); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
				return activePath, fromPath
			},
			wantActive: canonicalRecoveryTestPath,
			wantFrom:   func(t *testing.T, path string) string { return knownPathDescription(path) },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			activePath, fromPath := tt.setup(t)
			_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
			if err == nil {
				t.Fatal("Execute succeeded; want structured path canonicalization failure")
			}
			var failure *ApplyFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
			}
			if failure.Phase != phaseSourceRead || failure.State != replacementStateNoActiveMutation {
				t.Fatalf("failure phase/state = %s/%s", failure.Phase, failure.State)
			}
			if failure.ActivePath != tt.wantActive(t, activePath) || failure.FromPath != tt.wantFrom(t, fromPath) {
				t.Fatalf("failure paths active=%q from=%q", failure.ActivePath, failure.FromPath)
			}
			if !strings.Contains(err.Error(), "resolve database symlink") {
				t.Fatalf("error = %v, want symlink path details", err)
			}
		})
	}
}

func TestRecoverInitialApplyFailurePreservesCauseAs(t *testing.T) {
	type sentinelError struct{ error }
	cause := &sentinelError{error: errors.New("sentinel initial failure")}
	err := initialApplyFailure(phaseSourceRead, "/active.db", "/from.db", cause)
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	var gotCause *sentinelError
	if !errors.As(err, &gotCause) || gotCause != cause {
		t.Fatalf("errors.As did not preserve cause: %T %[1]v", err)
	}
}

func TestRecoverStrandedOpenFailureIsStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "missing-stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active missing stranded", UUID: "06060606-0606-4606-8606-060606060606", ContentType: "text"}})

	_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
	if err == nil {
		t.Fatal("Execute missing stranded source succeeded; want structured source-open failure")
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.Phase != phaseSourceRead || failure.ActivePath != canonicalRecoveryTestPath(t, activePath) || failure.FromPath != knownPathDescription(fromPath) || failure.TempPath != "" {
		t.Fatalf("failure phase/paths = %s active=%q from=%q temp=%q", failure.Phase, failure.ActivePath, failure.FromPath, failure.TempPath)
	}
}

func TestRecoverStrandedReadFailureCarriesCanonicalFromPath(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active before stranded read fail", UUID: "0a0a0a0a-0a0a-4a0a-8a0a-0a0a0a0a0a0a", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded read fail", UUID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b", ContentType: "text"}})
	wantErr := errors.New("injected stranded source read failure")
	wantFromPath := canonicalRecoveryTestPath(t, fromPath)
	deps := defaultRecoveryOperationDeps()
	originalFingerprintSet := deps.fingerprintSet
	deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
		if path == wantFromPath {
			return sqliteFileSetSnapshot{}, wantErr
		}
		return originalFingerprintSet(path)
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("executeWithDeps error = %v, want stranded read sentinel", err)
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.Phase != phaseSourceRead || failure.FromPath != wantFromPath {
		t.Fatalf("failure phase/from = %s/%q, want source-read with canonical from %q", failure.Phase, failure.FromPath, wantFromPath)
	}
}

func TestRecoverActiveReadFailureIsStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active read fail", UUID: "02020202-0202-4202-8202-020202020202", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded read fail", UUID: "03030303-0303-4303-8303-030303030303", ContentType: "text"}})
	wantErr := errors.New("injected active source read failure")
	deps := defaultRecoveryOperationDeps()
	deps.readReservedInput = func(context.Context, compat.Queryer) (compat.RecoveryInput, *compat.Diagnostic, error) {
		return compat.RecoveryInput{}, nil, wantErr
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("executeWithDeps error = %v, want source read sentinel", err)
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) || failure.Phase != phaseSourceRead || failure.ActivePath != canonicalRecoveryTestPath(t, activePath) || failure.FromPath != canonicalRecoveryTestPath(t, fromPath) {
		t.Fatalf("error = %T %[1]v, want source-read ApplyFailure for active with canonical paths", err)
	}
}

func TestRecoverDestinationConstructionCleanupLeakIsStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active destination leak", UUID: "04040404-0404-4404-8404-040404040404", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded destination leak", UUID: "05050505-0505-4505-8505-050505050505", ContentType: "text"}})
	leakedPath := filepath.Join(dir, ".backscroll-recover-leaked.db")
	if err := os.WriteFile(leakedPath, []byte("leaked temp sentinel"), 0o600); err != nil {
		t.Fatalf("write leaked temp: %v", err)
	}
	causeErr := errors.New("injected destination construction failure")
	cleanupErr := errors.New("injected destination cleanup failure")
	deps := defaultRecoveryOperationDeps()
	deps.createDestination = func(context.Context, string, compat.RecoveryPlan) (string, error) {
		return leakedPath, &storage.RecoveryDestinationError{Path: leakedPath, Cause: causeErr, CleanupErr: cleanupErr}
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil || !errors.Is(err, causeErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("executeWithDeps error = %v, want cause and cleanup", err)
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.FromPath != canonicalRecoveryTestPath(t, fromPath) || failure.TempPath != leakedPath || failure.CleanupErr == nil || len(failure.CleanupErrors) != 1 {
		t.Fatalf("FromPath/TempPath/CleanupErr/CleanupErrors = %q/%q/%v/%d, want canonical from path, leaked path, and structured cleanup", failure.FromPath, failure.TempPath, failure.CleanupErr, len(failure.CleanupErrors))
	}
	if _, statErr := os.Lstat(leakedPath); statErr != nil {
		t.Fatalf("leaked path %s missing: %v", leakedPath, statErr)
	}
}

func TestRecoverSameDirectoryFailureIsStructured(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	tempPath := filepath.Join(t.TempDir(), ".backscroll-recover-other.db")
	_, err := replaceActiveWithBackup(context.Background(), activePath, tempPath, compat.RecoveryPlan{}, sqliteFileSetSnapshot{}, defaultRecoveryOperationDeps())
	if err == nil {
		t.Fatal("replaceActiveWithBackup succeeded; want same-directory failure")
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if failure.Phase != phasePreBackupValidate || failure.State != replacementStateNoActiveMutation || failure.ActivePath != activePath || failure.TempPath != tempPath {
		t.Fatalf("failure phase/state/paths = %s/%s/%q/%q", failure.Phase, failure.State, failure.ActivePath, failure.TempPath)
	}
}

func TestRecoverFingerprintJoinsReadAndCloseErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fingerprint.db")
	if err := os.WriteFile(path, []byte("fingerprint bytes"), 0o600); err != nil {
		t.Fatalf("write fingerprint source: %v", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat fingerprint source: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fingerprint source: %v", err)
	}
	readErr := errors.New("injected fingerprint read failure")
	closeErr := errors.New("injected fingerprint close failure")
	_, err = snapshotSQLiteOpenFile(path, pathInfo, &erroringSnapshotFile{file: file, readErr: readErr, closeErr: closeErr})
	if err == nil || !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("snapshotSQLiteOpenFile error = %v, want read and close", err)
	}
}

type erroringSnapshotFile struct {
	file     *os.File
	readErr  error
	closeErr error
}

func (f *erroringSnapshotFile) Read([]byte) (int, error) { return 0, f.readErr }

func (f *erroringSnapshotFile) Stat() (os.FileInfo, error) { return f.file.Stat() }

func (f *erroringSnapshotFile) Close() error {
	_ = f.file.Close()
	return f.closeErr
}

func TestRecoverRevalidatesConcurrentMutationBeforeInstall(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal: 0, Role: "user", Text: "active before concurrent mutation", UUID: "11111111-1111-4111-8111-111111111111", Timestamp: "2026-08-18T00:00:00Z", ContentType: "text",
	}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal: 0, Role: "assistant", Text: "stranded not installed after mutation", UUID: "22222222-2222-4222-8222-222222222222", Timestamp: "2026-08-18T00:01:00Z", ContentType: "text",
	}})
	resolvedActivePath := canonicalRecoveryTestPath(t, activePath)
	deps := defaultRecoveryOperationDeps()
	originalMove := deps.noClobberMove
	mutated := false
	deps.noClobberMove = func(oldPath, newPath string) error {
		if oldPath == resolvedActivePath && !mutated {
			mutated = true
			f, err := os.OpenFile(oldPath, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				return err
			}
			_, writeErr := f.WriteString("concurrent mutation")
			closeErr := f.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return originalMove(oldPath, newPath)
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil {
		t.Fatal("recovery succeeded after concurrent mutation during backup; want fail closed")
	}
	var replacementErr *replacementError
	if !errors.As(err, &replacementErr) || replacementErr.Phase != phasePostBackupVerify {
		t.Fatalf("error = %T %[1]v, want post-backup verification replacement error", err)
	}
}

func TestRecoverActiveFingerprintRevalidationGaps(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(t *testing.T, activePath, fromPath string, deps *recoveryOperationDeps)
		want      string
	}{
		{
			name: "in-place mutation after reserved SQL read",
			configure: func(t *testing.T, activePath, fromPath string, deps *recoveryOperationDeps) {
				original := deps.readReservedInput
				deps.readReservedInput = func(ctx context.Context, q compat.Queryer) (compat.RecoveryInput, *compat.Diagnostic, error) {
					input, diag, err := original(ctx, q)
					appendRecoveryTestBytes(t, activePath, "mutation after reserved read")
					return input, diag, err
				}
			},
			want: "during recovery SQL read",
		},
		{
			name: "inode swap after reservation close",
			configure: func(t *testing.T, activePath, fromPath string, deps *recoveryOperationDeps) {
				original := deps.closeReservation
				deps.closeReservation = func(r *activeReservation) error {
					if err := original(r); err != nil {
						return err
					}
					replaceRecoveryTestFile(t, activePath, []byte("not the planned sqlite main"))
					return nil
				}
			},
			want: "after closing recovery reservation",
		},
		{
			name: "stranded in-place mutation during immutable read",
			configure: func(t *testing.T, activePath, fromPath string, deps *recoveryOperationDeps) {
				original := deps.fingerprintSet
				mutated := false
				resolvedFrom := canonicalRecoveryTestPath(t, fromPath)
				deps.fingerprintSet = func(path string) (sqliteFileSetSnapshot, error) {
					snapshot, err := original(path)
					if err == nil && path == resolvedFrom && !mutated {
						mutated = true
						appendRecoveryTestBytes(t, fromPath, "stranded mutated after pre-read fingerprint")
					}
					return snapshot, err
				}
			},
			want: "immutable recovery source changed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "active.db")
			fromPath := filepath.Join(dir, "stranded.db")
			createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active gap", UUID: "10101010-1010-4010-8010-101010101010", ContentType: "text"}})
			createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded gap", UUID: "20202020-2020-4020-8020-202020202020", ContentType: "text"}})
			deps := defaultRecoveryOperationDeps()
			tt.configure(t, activePath, fromPath, &deps)

			_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("executeWithDeps error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRecoverRejectsLateActiveSidecarAfterBackup(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active late sidecar", UUID: "30303030-3030-4030-8030-303030303030", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded late sidecar", UUID: "40404040-4040-4040-8040-404040404040", ContentType: "text"}})
	deps := defaultRecoveryOperationDeps()
	original := deps.noClobberMove
	resolvedActive := canonicalRecoveryTestPath(t, activePath)
	deps.noClobberMove = func(oldPath, newPath string) error {
		err := original(oldPath, newPath)
		if err == nil && oldPath == resolvedActive {
			if writeErr := os.WriteFile(resolvedActive+"-journal", []byte("late sidecar"), 0o600); writeErr != nil {
				return writeErr
			}
		}
		return err
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil || !strings.Contains(err.Error(), "unexpected active sidecar") || !strings.Contains(err.Error(), "-journal") {
		t.Fatalf("executeWithDeps error = %v, want late sidecar rejection", err)
	}
	if _, statErr := os.Lstat(activePath); !os.IsNotExist(statErr) {
		t.Fatalf("active main restored into unsafe namespace: stat err=%v", statErr)
	}
}

func TestRecoverSQLiteSidecarPolicy(t *testing.T) {
	t.Run("stranded non-empty WAL rejected before immutable planning", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active", UUID: "33333333-3333-4333-8333-333333333333", ContentType: "text"}})
		stranded := createRecoveryLiveWALDB(t, fromPath)
		defer func() { _ = stranded.Close() }()

		_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
		if err == nil || !errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
			t.Fatalf("Execute error = %v, want immutable WAL unsafe", err)
		}
	})

	t.Run("rollback journal rejected", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active", UUID: "44444444-4444-4444-8444-444444444444", ContentType: "text"}})
		createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded", UUID: "55555555-5555-4555-8555-555555555555", ContentType: "text"}})
		if err := os.WriteFile(activePath+"-journal", []byte("hot journal sentinel"), 0o600); err != nil {
			t.Fatalf("write rollback journal: %v", err)
		}

		_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
		if err == nil || !strings.Contains(err.Error(), "sidecar namespace") || !strings.Contains(err.Error(), "-journal") {
			t.Fatalf("Execute error = %v, want rollback journal namespace rejection", err)
		}
	})

	t.Run("empty active shm rejected before replacement", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active with stale shm", UUID: "66666666-6666-4666-8666-666666666666", ContentType: "text"}})
		createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded install", UUID: "77777777-7777-4777-8777-777777777777", ContentType: "text"}})
		if err := os.WriteFile(activePath+"-shm", nil, 0o600); err != nil {
			t.Fatalf("write empty stale shm: %v", err)
		}

		_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath})
		if err == nil || !strings.Contains(err.Error(), "-shm") || !strings.Contains(err.Error(), "sidecar namespace") {
			t.Fatalf("Execute error = %v, want empty shm namespace rejection", err)
		}
	})

	t.Run("dangling stranded wal symlink rejected", func(t *testing.T) {
		dir := t.TempDir()
		activePath := filepath.Join(dir, "active.db")
		fromPath := filepath.Join(dir, "stranded.db")
		createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active", UUID: "88888888-8888-4888-8888-888888888888", ContentType: "text"}})
		createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded", UUID: "99999999-9999-4999-8999-999999999999", ContentType: "text"}})
		if err := os.Symlink(filepath.Join(dir, "missing-wal-target"), fromPath+"-wal"); err != nil {
			t.Fatalf("symlink dangling wal: %v", err)
		}

		_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath, DryRun: true})
		if err == nil || !errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) || !strings.Contains(err.Error(), fromPath+"-wal") {
			t.Fatalf("Execute error = %v, want dangling WAL symlink rejection", err)
		}
	})
}

func TestRecoverBackupNameCollisionRetriesWithoutClobber(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active collision", UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded collision", UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ContentType: "text"}})
	deps := defaultRecoveryOperationDeps()
	originalMove := deps.noClobberMove
	suffixes := []string{"000001", "000002"}
	deps.randomHex = func(n int) (string, error) {
		if len(suffixes) == 0 {
			return "000002", nil
		}
		next := suffixes[0]
		suffixes = suffixes[1:]
		return next, nil
	}
	collided := false
	resolvedActive := canonicalRecoveryTestPath(t, activePath)
	deps.noClobberMove = func(oldPath, newPath string) error {
		if oldPath == resolvedActive && !collided {
			collided = true
			return os.ErrExist
		}
		return originalMove(oldPath, newPath)
	}

	report, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err != nil {
		t.Fatalf("Execute after backup collision retry: %v", err)
	}
	if !collided || !strings.Contains(report.BackupPath, "000002") {
		t.Fatalf("backup collision retry report=%+v collided=%v, want second suffix", report, collided)
	}
}

func TestRecoverBackupNameFailuresAreStructuredAndOperationLocal(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(*recoveryOperationDeps)
	}{
		{
			name: "random suffix failure",
			configure: func(deps *recoveryOperationDeps) {
				wantErr := errors.New("injected random suffix failure")
				deps.randomHex = func(int) (string, error) { return "", wantErr }
			},
		},
		{
			name: "collision exhaustion",
			configure: func(deps *recoveryOperationDeps) {
				deps.randomHex = func(int) (string, error) { return "abcdef", nil }
				deps.noClobberMove = func(string, string) error { return os.ErrExist }
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			activePath := filepath.Join(dir, "active.db")
			fromPath := filepath.Join(dir, "stranded.db")
			createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active backup naming", UUID: "12121212-1212-4212-8212-121212121212", ContentType: "text"}})
			createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded backup naming", UUID: "34343434-3434-4434-8434-343434343434", ContentType: "text"}})
			before := recoveryDirectoryInventory(t, dir)
			deps := defaultRecoveryOperationDeps()
			if tt.configure != nil {
				tt.configure(&deps)
			}

			_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
			if err == nil {
				t.Fatal("executeWithDeps succeeded; want structured backup naming failure")
			}
			var failure *ApplyFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
			}
			if failure.Phase != phaseBackupMain || failure.State != replacementStateNoActiveMutation || !failure.NoActiveMutation {
				t.Fatalf("failure phase/state/no-mutation = %s/%s/%v", failure.Phase, failure.State, failure.NoActiveMutation)
			}
			if after := recoveryDirectoryInventory(t, dir); !reflect.DeepEqual(after, before) {
				t.Fatalf("naming failure mutated inventory\nbefore: %s\nafter:  %s", describeRecoveryInventory(before), describeRecoveryInventory(after))
			}
		})
	}
}

func TestRecoverLeakedTempPathRemainsStructuredWhenCleanupFails(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active leaked temp", UUID: "56565656-5656-4656-8656-565656565656", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded leaked temp", UUID: "78787878-7878-4878-8878-787878787878", ContentType: "text"}})
	deps := defaultRecoveryOperationDeps()
	deps.randomHex = deterministicRecoveryHex()
	resolvedActive := canonicalRecoveryTestPath(t, activePath)
	deps.installMove = func(oldPath, newPath string) error {
		if strings.Contains(filepath.Base(oldPath), ".backscroll-recover-") && newPath == resolvedActive {
			return fmt.Errorf("injected install failure before temp cleanup leak")
		}
		return noClobberMove(oldPath, newPath)
	}
	leakedTemp := ""
	deps.cleanupFileSet = func(path string) error {
		if strings.Contains(filepath.Base(path), ".backscroll-recover-") {
			leakedTemp = path
			return fmt.Errorf("injected cleanup failure for %s", path)
		}
		return removeSQLiteFileSet(path)
	}

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil {
		t.Fatal("executeWithDeps succeeded; want cleanup leak failure")
	}
	var failure *ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *ApplyFailure", err)
	}
	if leakedTemp == "" || failure.TempPath != leakedTemp {
		t.Fatalf("TempPath = %q, leakedTemp = %q", failure.TempPath, leakedTemp)
	}
	if failure.CleanupErr == nil || len(failure.CleanupErrors) == 0 {
		t.Fatalf("CleanupErr/CleanupErrors = %v/%d, want structured cleanup failure", failure.CleanupErr, len(failure.CleanupErrors))
	}
	if _, statErr := os.Lstat(leakedTemp); statErr != nil {
		t.Fatalf("leaked temp %s missing after cleanup failure: %v", leakedTemp, statErr)
	}
}

func TestNoClobberMoveRejectsSymlinkCollision(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	target := filepath.Join(dir, "target.db")
	elsewhere := filepath.Join(dir, "elsewhere.db")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(elsewhere, []byte("elsewhere"), 0o600); err != nil {
		t.Fatalf("write elsewhere: %v", err)
	}
	if err := os.Symlink(elsewhere, target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	err := noClobberMove(source, target)
	if err == nil || !os.IsExist(err) {
		t.Fatalf("noClobberMove error = %v, want existing-target failure", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "source" {
		t.Fatalf("source after collision = %q err=%v", got, err)
	}
	linkTarget, err := os.Readlink(target)
	if err != nil || linkTarget != elsewhere {
		t.Fatalf("target symlink = %q err=%v, want unchanged symlink", linkTarget, err)
	}
}

func TestRecoverCleanupErrorsAreSurfaced(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "active cleanup", UUID: "88888888-8888-4888-8888-888888888888", ContentType: "text"}})
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "assistant", Text: "stranded cleanup", UUID: "99999999-9999-4999-8999-999999999999", ContentType: "text"}})
	badTempDir := filepath.Join(dir, ".backscroll-recover-bad.db")
	if err := os.Mkdir(badTempDir, 0o700); err != nil {
		t.Fatalf("mkdir bad temp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badTempDir, "child"), []byte("prevents cleanup"), 0o600); err != nil {
		t.Fatalf("write bad temp child: %v", err)
	}
	deps := defaultRecoveryOperationDeps()
	deps.createDestination = func(context.Context, string, compat.RecoveryPlan) (string, error) { return badTempDir, nil }
	deps.syncFile = func(string) error { return fmt.Errorf("injected preinstall failure") }

	_, err := executeWithDeps(context.Background(), Options{ActivePath: activePath, FromPath: fromPath}, deps)
	if err == nil || !strings.Contains(err.Error(), "cleanup verified recovery destination") {
		t.Fatalf("Execute error = %v, want surfaced cleanup failure", err)
	}
}

func TestRecoverSameResolvedPathIsOneInput(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "same database row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})

	aliasPath := filepath.Join(dir, "alias.db")
	if err := os.Symlink(activePath, aliasPath); err != nil {
		t.Fatalf("symlink active database: %v", err)
	}

	report, err := Execute(context.Background(), Options{
		ActivePath: activePath,
		FromPath:   aliasPath,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Execute dry run: %v", err)
	}

	if !reflect.DeepEqual(report.InputCounts, []int{1}) {
		t.Fatalf("InputCounts = %v, want one active input with one row", report.InputCounts)
	}
	if len(report.Shapes) != 1 {
		t.Fatalf("Shapes = %d, want 1", len(report.Shapes))
	}
	if report.FinalCount != 1 || report.ExactDuplicates != 0 {
		t.Fatalf("FinalCount=%d ExactDuplicates=%d, want 1 and 0", report.FinalCount, report.ExactDuplicates)
	}
}

func TestExecuteWrapsImmutableLiveWALErrorWithRecoveryGuidance(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	active := createRecoveryLiveWALDB(t, activePath)
	defer func() { _ = active.Close() }()
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "stranded row",
		UUID:        "22222222-2222-4222-8222-222222222222",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})

	_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath, DryRun: true})
	if err == nil {
		t.Fatal("Execute with active live WAL succeeded; want immutable recovery planning guidance")
	}
	if !errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
		t.Fatalf("Execute error = %v, want ErrImmutableReadOnlyWALUnsafe", err)
	}
	message := err.Error()
	resolvedActive, resolveErr := filepath.EvalSymlinks(activePath)
	if resolveErr != nil {
		t.Fatalf("resolve active path: %v", resolveErr)
	}
	for _, want := range []string{"recovery dry-run", "checkpoint", resolvedActive} {
		if !strings.Contains(message, want) {
			t.Fatalf("Execute error %q missing %q", message, want)
		}
	}
}

func TestResolvePathSurfacesStatErrors(t *testing.T) {
	dir := t.TempDir()
	loop := filepath.Join(dir, "loop.db")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatalf("create symlink loop: %v", err)
	}

	_, err := resolvePath(loop)
	if err == nil {
		t.Fatal("resolvePath symlink loop succeeded; want stat error")
	}
	if os.IsNotExist(err) {
		t.Fatalf("resolvePath error = %v, want non-NotExist stat error", err)
	}
	if !strings.Contains(err.Error(), "stat database path") {
		t.Fatalf("resolvePath error = %v, want stat database path context", err)
	}
}

func TestResolvePathAllowsMissingPathForOpenError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	resolved, err := resolvePath(missing)
	if err != nil {
		t.Fatalf("resolvePath missing path: %v", err)
	}
	if resolved.path == "" || resolved.info != nil {
		t.Fatalf("resolvePath missing = %+v, want path with nil info", resolved)
	}
}

type recoveryInventoryEntry struct {
	Kind   string
	Size   int64
	Mode   os.FileMode
	SHA256 string
}

func deterministicRecoveryHex() func(int) (string, error) {
	next := 1
	return func(int) (string, error) {
		value := fmt.Sprintf("%06d", next)
		next++
		return value, nil
	}
}

func recoveryDirectoryInventory(t *testing.T, dir string) map[string]recoveryInventoryEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir inventory %s: %v", dir, err)
	}
	result := map[string]recoveryInventoryEntry{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat inventory entry %s: %v", path, err)
		}
		item := recoveryInventoryEntry{Kind: "other", Size: info.Size(), Mode: info.Mode()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read inventory entry %s: %v", path, err)
			}
			item.Kind = "regular"
			item.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		} else if info.IsDir() {
			item.Kind = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			item.Kind = "symlink"
		}
		result[entry.Name()] = item
	}
	return result
}

func describeRecoveryInventory(inventory map[string]recoveryInventoryEntry) string {
	keys := make([]string, 0, len(inventory))
	for key := range inventory {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		entry := inventory[key]
		parts = append(parts, fmt.Sprintf("%s:%s:%d:%s", key, entry.Kind, entry.Size, entry.SHA256))
	}
	return strings.Join(parts, ",")
}

func assertReplacementErrorPaths(t *testing.T, replacementErr *replacementError, activePath string) {
	t.Helper()
	if replacementErr.ActivePath != canonicalRecoveryTestPath(t, activePath) {
		t.Fatalf("replacement active path = %q, want canonical active", replacementErr.ActivePath)
	}
	if replacementErr.TempPath == "" {
		t.Fatalf("replacement temp path is empty: %+v", replacementErr)
	}
	if replacementErr.BackupPath == "" && replacementErr.BackupVisible {
		t.Fatalf("backup is visible without exact backup path: %+v", replacementErr)
	}
}

func assertNoRecoveryTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir for temp assertion: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backscroll-recover-") {
			t.Fatalf("temporary recovery destination still visible: %s in %s", entry.Name(), describeRecoveryInventory(recoveryDirectoryInventory(t, dir)))
		}
	}
}

func assertRecoveryFaultInventory(t *testing.T, want, dir, activePath, fromPath, backupPath string, activeBefore recoveryDBFileSnapshot, before map[string]recoveryInventoryEntry, reportedBackup string) {
	t.Helper()
	after := recoveryDirectoryInventory(t, dir)
	switch want {
	case "unchanged":
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("failure inventory changed\nbefore: %s\nafter:  %s", describeRecoveryInventory(before), describeRecoveryInventory(after))
		}
	case "backup-only":
		if backupPath == "" {
			t.Fatal("backup-only inventory requested without backup path")
		}
		wantInventory := cloneRecoveryInventory(before)
		delete(wantInventory, filepath.Base(activePath))
		wantInventory[filepath.Base(backupPath)] = recoveryInventoryEntry{Kind: "regular", Size: int64(len(activeBefore.main.data)), Mode: activeBefore.main.mode, SHA256: strings.TrimPrefix(activeBefore.main.fingerprint, "sha256:")}
		if !reflect.DeepEqual(after, wantInventory) {
			t.Fatalf("backup-only inventory mismatch\nwant: %s\ngot:  %s", describeRecoveryInventory(wantInventory), describeRecoveryInventory(after))
		}
	case "replacement-and-backup":
		if backupPath == "" || reportedBackup != backupPath {
			t.Fatalf("replacement failure backup path = err %q report %q, want same non-empty", backupPath, reportedBackup)
		}
		if _, err := os.Stat(backupPath); err != nil {
			t.Fatalf("stat visible backup %s: %v", backupPath, err)
		}
		if _, err := os.Stat(activePath); err != nil {
			t.Fatalf("stat visible replacement active %s: %v", activePath, err)
		}
		if _, ok := after[filepath.Base(backupPath)]; !ok {
			t.Fatalf("replacement inventory lacks backup %s: %s", filepath.Base(backupPath), describeRecoveryInventory(after))
		}
		if _, ok := after[filepath.Base(activePath)]; !ok {
			t.Fatalf("replacement inventory lacks active: %s", describeRecoveryInventory(after))
		}
		if _, ok := after[filepath.Base(fromPath)]; !ok {
			t.Fatalf("replacement inventory lacks stranded source: %s", describeRecoveryInventory(after))
		}
		if len(after) != 3 {
			t.Fatalf("replacement inventory has extra entries: %s", describeRecoveryInventory(after))
		}
	default:
		t.Fatalf("unknown inventory expectation %q", want)
	}
}

func cloneRecoveryInventory(in map[string]recoveryInventoryEntry) map[string]recoveryInventoryEntry {
	out := make(map[string]recoveryInventoryEntry, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func appendRecoveryTestBytes(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("open %s for append mutation: %v", path, err)
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		t.Fatalf("append mutation to %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close appended mutation %s: %v", path, err)
	}
}

func replaceRecoveryTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	temp := path + ".swap-test"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		t.Fatalf("write swap file %s: %v", temp, err)
	}
	if err := os.Rename(temp, path); err != nil {
		t.Fatalf("rename swap file over %s: %v", path, err)
	}
}

func createRecoveryDB(t *testing.T, path string, messages []storage.IndexedMessage) {
	t.Helper()
	db := openRecoveryDB(t, path, messages)
	if err := db.Close(); err != nil {
		t.Fatalf("close test database %s: %v", path, err)
	}
}

func createRecoveryLiveWALDB(t *testing.T, path string) *storage.Database {
	t.Helper()
	db := openRecoveryDB(t, path, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "active live WAL row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	wal, err := os.Stat(path + "-wal")
	if err != nil {
		_ = db.Close()
		t.Fatalf("stat live WAL sidecar: %v", err)
	}
	if wal.Size() == 0 {
		_ = db.Close()
		t.Fatal("live WAL sidecar is empty; test fixture did not create committed WAL frames")
	}
	return db
}

type recoveryDBFileSnapshot struct {
	main     recoveryFileSnapshot
	sidecars map[string]recoveryFileSnapshot
}

type recoveryFileSnapshot struct {
	data        []byte
	mode        os.FileMode
	mtime       time.Time
	fingerprint string
}

func snapshotRecoveryDBFile(t *testing.T, dbPath string) recoveryDBFileSnapshot {
	t.Helper()
	return recoveryDBFileSnapshot{
		main:     snapshotRecoveryFile(t, dbPath),
		sidecars: snapshotRecoverySidecars(t, dbPath),
	}
}

func snapshotRecoverySidecars(t *testing.T, dbPath string) map[string]recoveryFileSnapshot {
	t.Helper()
	result := map[string]recoveryFileSnapshot{}
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		result[suffix] = snapshotRecoveryFile(t, path)
	}
	return result
}

func snapshotRecoveryFile(t *testing.T, path string) recoveryFileSnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return recoveryFileSnapshot{data: data, mode: info.Mode(), mtime: info.ModTime(), fingerprint: fmt.Sprintf("sha256:%x", sha256.Sum256(data))}
}

func assertRecoveryDBFileSnapshot(t *testing.T, label, dbPath string, want recoveryDBFileSnapshot) {
	t.Helper()
	got := snapshotRecoveryDBFile(t, dbPath)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s changed\nwant: %s\ngot:  %s", label, describeRecoveryDBFileSnapshot(want), describeRecoveryDBFileSnapshot(got))
	}
}

func describeRecoveryDBFileSnapshot(snapshot recoveryDBFileSnapshot) string {
	return fmt.Sprintf("main=%s sidecars=%v", describeRecoveryFileSnapshot(snapshot.main), recoverySnapshotSidecarKeys(snapshot.sidecars))
}

func describeRecoveryFileSnapshot(snapshot recoveryFileSnapshot) string {
	return fmt.Sprintf("bytes=%d mode=%s mtime=%s fingerprint=%s", len(snapshot.data), snapshot.mode, snapshot.mtime.Format(time.RFC3339Nano), snapshot.fingerprint)
}

func recoverySnapshotSidecarKeys(snapshot map[string]recoveryFileSnapshot) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	return keys
}

func canonicalRecoveryTestPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("absolute path for %s: %v", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved
	}
	parent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs))
	if parentErr != nil {
		t.Fatalf("resolve path for %s: %v", abs, err)
	}
	return filepath.Join(parent, filepath.Base(abs))
}

func assertRecoveryTexts(t *testing.T, dbPath string, want []string) {
	t.Helper()
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open recovered database read-only: %v", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.DB().Query(`SELECT text FROM search_items ORDER BY text`)
	if err != nil {
		t.Fatalf("query recovered texts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatalf("scan recovered text: %v", err)
		}
		got = append(got, text)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read recovered texts: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recovered texts = %v, want %v", got, want)
	}
}

func openRecoveryDB(t *testing.T, path string, messages []storage.IndexedMessage) *storage.Database {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("open test database %s: %v", path, err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/sessions/shared.jsonl",
		Source:     "session",
		Hash:       "hash-" + filepath.Base(path),
		Project:    "project",
		Messages:   messages,
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("sync test database %s: %v", path, err)
	}
	return db
}
