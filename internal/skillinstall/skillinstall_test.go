package skillinstall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOfficialShimMatchesRepositoryTree(t *testing.T) {
	shim, err := OfficialShim()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "ref-context-mode.md"} {
		got, ok := shim[name]
		if !ok {
			t.Fatalf("official shim does not contain %s", name)
		}
		want, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "backscroll", name))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("embedded %s differs from the repository shim", name)
		}
	}
	if len(shim) != 2 {
		t.Fatalf("official shim contains %d files, want 2", len(shim))
	}
}

func TestInstallSuccessAndIdempotence(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})

	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("clean installation unexpectedly requires approval")
	}
	if _, err := os.Stat(installer.state); !os.IsNotExist(err) {
		t.Fatalf("Plan created installation state: %v", err)
	}
	if _, err := installer.Apply(strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "approval mismatch") {
		t.Fatalf("unneeded incorrect approval error = %v", err)
	}

	result, err := installer.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 3 || result.Receipt == "" || result.RestoreApproval == "" {
		t.Fatalf("Apply result = %+v", result)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		assertLinkTarget(t, target, plan.Source.Path)
	}
	managed, err := snapshot(plan.Source.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(managed, installer.desired) {
		t.Fatalf("managed snapshot = %#v, want %#v", managed, installer.desired)
	}
	before := targetMetadata(t, targetPaths(installer.home, installer.configHome))
	runsBefore := receiptPaths(t, installer.state)

	second, err := installer.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	if second != (ApplyResult{}) {
		t.Fatalf("idempotent Apply result = %+v", second)
	}
	if got := targetMetadata(t, targetPaths(installer.home, installer.configHome)); !reflect.DeepEqual(got, before) {
		t.Fatalf("idempotent Apply modified target metadata: before=%v after=%v", before, got)
	}
	if got := receiptPaths(t, installer.state); !reflect.DeepEqual(got, runsBefore) {
		t.Fatalf("idempotent Apply created a receipt: before=%v after=%v", runsBefore, got)
	}
}

func TestUnknownContentNeedsApprovalAndRestores(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("unknown content did not require approval")
	}

	if _, err := installer.Apply(strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "approval mismatch") {
		t.Fatalf("Apply with incorrect approval error = %v", err)
	}
	if _, err := os.Stat(installer.state); !os.IsNotExist(err) {
		t.Fatalf("rejected approval created installation state: %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
			t.Fatalf("rejected approval changed %s", target)
		}
	}

	result, err := installer.Apply(plan.Approval)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		assertLinkTarget(t, target, plan.Source.Path)
	}
	restored, err := Restore(home, result.Receipt, result.RestoreApproval)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 3 {
		t.Fatalf("Restore restored = %d, want 3", restored.Restored)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
			t.Fatalf("Restore content at %s = %q", target, got)
		}
	}
	again, err := Restore(home, result.Receipt, result.RestoreApproval)
	if err != nil {
		t.Fatal(err)
	}
	if again.Restored != 0 {
		t.Fatalf("second Restore restored = %d, want 0", again.Restored)
	}
}

func TestApplyRejectsObjectChangedAfterValidation(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	target := plan.Targets[0].Path
	approved := target + ".approved"
	injected := false
	installer.deps.beforeExchange = func(_, right string) error {
		if injected || right != target {
			return nil
		}
		injected = true
		if err := os.Rename(target, approved); err != nil {
			return err
		}
		return os.WriteFile(target, []byte("concurrent work"), 0o600)
	}

	_, err = installer.Apply(plan.Approval)
	if err == nil || !strings.Contains(err.Error(), "destination changed during atomic installation") {
		t.Fatalf("Apply race error = %v", err)
	}
	if !strings.Contains(err.Error(), "receipt:") || !strings.Contains(err.Error(), "restore_approval:") {
		t.Fatalf("Apply rollback error omits recovery credentials: %v", err)
	}
	if got := mustRead(t, target); got != "concurrent work" {
		t.Fatalf("Apply race changed concurrent work: %q", got)
	}
	if got := mustRead(t, filepath.Join(approved, "local-note")); got != "operator data" {
		t.Fatalf("Apply race lost approved preimage: %q", got)
	}
}

func TestRestoreRejectsObjectChangedAfterValidation(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	result, err := installer.Apply(plan.Approval)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := readReceipt(result.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	targets := targetPaths(installer.home, installer.configHome)
	injected := false
	deps := dependencies{}
	deps.syncDir = func(path string) error {
		if !injected && path == filepath.Dir(targets[0]) {
			injected = true
			if err := os.Remove(targets[1]); err != nil {
				return err
			}
			if err := os.WriteFile(targets[1], []byte("concurrent work"), 0o600); err != nil {
				return err
			}
		}
		return syncDirectory(path)
	}

	_, err = restoreRows(filepath.Dir(result.Receipt), value, deps)
	if err == nil || !strings.Contains(err.Error(), "concurrent destination change") {
		t.Fatalf("Restore race error = %v", err)
	}
	if got := mustRead(t, targets[1]); got != "concurrent work" {
		t.Fatalf("Restore race changed concurrent work: %q", got)
	}
	if got := mustRead(t, filepath.Join(filepath.Dir(result.Receipt), "backup-1", "local-note")); got != "operator data" {
		t.Fatalf("Restore race changed the backup: %q", got)
	}
}

func TestPythonV1LiteralFixture(t *testing.T) {
	path := filepath.Join("testdata", "python-v1-receipt.json")
	value, approval, err := readReceipt(path)
	if err != nil {
		t.Fatal(err)
	}
	if value.Version != 1 || value.Plan.Version != 1 || value.Run != "37bb4c83751f4c7a8886290c38d34a79" {
		t.Fatalf("Python v1 fixture identity = %+v", value)
	}
	if approval != "82520cefa00d2fd71e6eb91c7a414101cb2cb39c598b610cf39863d035e65195" {
		t.Fatalf("Python v1 fixture restore approval = %s", approval)
	}
	if value.Plan.Approval != "0c085da8bed41b13e2581fa17c5b082c2e8ffefde50890cef892a4f85bc0fc9f" {
		t.Fatalf("Python v1 fixture plan approval = %s", value.Plan.Approval)
	}
}

func TestPartialFailureRestoresAllDestinations(t *testing.T) {
	home := testHome(t)
	calls := 0
	deps := dependencies{syncDir: syncDirectory}
	deps.beforeExchange = func(left, right string) error {
		calls++
		if calls == 2 {
			return errors.New("injected second destination failure")
		}
		return nil
	}
	installer := testInstaller(t, home, deps)
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Apply(plan.Approval); err == nil || !strings.Contains(err.Error(), "injected second destination failure") {
		t.Fatalf("Apply error = %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		info, err := os.Lstat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("partial failure left installed link: %s", target)
		}
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
			t.Fatalf("partial failure changed %s", target)
		}
	}
	if got := len(receiptPaths(t, installer.state)); got != 1 {
		t.Fatalf("partial failure receipts = %d, want 1", got)
	}
}

func TestRestoreRejectsManipulatedReceipt(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	result, err := installer.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	var value receipt
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value.Plan.Targets[0].Path = filepath.Join(home, "unrelated")
	if err := os.Chmod(result.Receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result.Receipt, data, 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedApproval, err := digestValue(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, tamperedApproval); err == nil || !strings.Contains(err.Error(), "destinations mismatch") {
		t.Fatalf("Restore manipulated receipt error = %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		assertLinkTarget(t, target, value.Plan.Source.Path)
	}
}

func TestRestoreCleansPreparedAtomicReplacement(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := makePrivateDir(installer.state); err != nil {
		t.Fatal(err)
	}
	runName := "prepared-replacement"
	run := filepath.Join(installer.state, runName)
	if err := makePrivateDir(run); err != nil {
		t.Fatal(err)
	}
	value := receipt{Version: 2, Plan: plan, Run: runName}
	receiptPath := filepath.Join(run, "receipt.json")
	if err := writeJSONExclusive(receiptPath, value); err != nil {
		t.Fatal(err)
	}
	first := plan.Targets[0].Path
	if err := os.MkdirAll(filepath.Dir(first), 0o700); err != nil {
		t.Fatal(err)
	}
	temp := swapPath(first, runName, 0)
	if err := os.Symlink(plan.Source.Path, temp); err != nil {
		t.Fatal(err)
	}
	approval, err := digestValue(value)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Restore(home, receiptPath, approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 1 {
		t.Fatalf("Restore prepared replacement count = %d, want 1", result.Restored)
	}
	if _, err := os.Lstat(temp); !os.IsNotExist(err) {
		t.Fatalf("Restore left prepared replacement: %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("Restore changed absent destination %s: %v", target, err)
		}
	}
}

func TestRestoreAcceptsPythonV1Receipt(t *testing.T) {
	home := testHome(t)
	configHome := filepath.Join(home, ".config")
	state := filepath.Join(home, statePath)
	if err := makePrivateDir(state); err != nil {
		t.Fatal(err)
	}
	runName := "python-v1-fixture"
	run := filepath.Join(state, runName)
	if err := makePrivateDir(run); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(home, "stable-product")
	source := filepath.Join(sourceRoot, ".claude/skills/backscroll")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("python source"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		Version:    1,
		Action:     "install",
		Home:       home,
		ConfigHome: configHome,
		Source: Source{Root: sourceRoot, Path: source, Commit: strings.Repeat("a", 40),
			TreeSHA256: strings.Repeat("b", 64)},
	}
	for index, target := range targetPaths(home, configHome) {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		beforeDir := filepath.Join(home, "preimage", string(rune('a'+index)))
		if err := os.MkdirAll(beforeDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(beforeDir, "local-note"), []byte("v1 data"), 0o640); err != nil {
			t.Fatal(err)
		}
		before, err := snapshot(beforeDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(beforeDir, filepath.Join(run, "backup-"+string(rune('0'+index)))); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(source, target); err != nil {
			t.Fatal(err)
		}
		plan.Targets = append(plan.Targets, Target{Path: target, Before: before, Change: true})
	}
	planApproval, err := planDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Approval = planApproval
	value := receipt{Version: 1, Plan: plan, Run: runName}
	receiptPath := filepath.Join(run, "receipt.json")
	if err := writeJSONExclusive(receiptPath, value); err != nil {
		t.Fatal(err)
	}
	approval, err := digestValue(value)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Restore(home, receiptPath, approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 3 {
		t.Fatalf("Restore Python v1 restored = %d, want 3", result.Restored)
	}
	for _, target := range targetPaths(home, configHome) {
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "v1 data" {
			t.Fatalf("Python v1 Restore content at %s = %q", target, got)
		}
	}
}

func TestFileAndBrokenLinkRestoreExactPreimages(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	targets := targetPaths(installer.home, installer.configHome)
	for _, target := range targets[:2] {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(targets[0], []byte("file preimage"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-relative-target", targets[1]); err != nil {
		t.Fatal(err)
	}
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	result, err := installer.Apply(plan.Approval)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(targets[0])
	if err != nil || string(content) != "file preimage" {
		t.Fatalf("restored file = %q, error = %v", content, err)
	}
	info, err := os.Stat(targets[0])
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored file mode = %v, error = %v", info.Mode().Perm(), err)
	}
	link, err := os.Readlink(targets[1])
	if err != nil || link != "missing-relative-target" {
		t.Fatalf("restored broken link = %q, error = %v", link, err)
	}
	if _, err := os.Lstat(targets[2]); !os.IsNotExist(err) {
		t.Fatalf("restored absent target exists: %v", err)
	}
}

func TestRestoreRefusesDestinationAndBackupDrift(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	result, err := installer.Apply(plan.Approval)
	if err != nil {
		t.Fatal(err)
	}
	targets := targetPaths(installer.home, installer.configHome)
	if err := os.Remove(targets[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targets[0], []byte("operator new work"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err == nil || !strings.Contains(err.Error(), "restore refuses") {
		t.Fatalf("destination drift restore error = %v", err)
	}
	assertLinkTarget(t, targets[1], plan.Source.Path)
	if err := os.Remove(targets[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(plan.Source.Path, targets[0]); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(result.Receipt), "backup-1", "local-note")
	if err := os.WriteFile(backup, []byte("backup drift"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err == nil || !strings.Contains(err.Error(), "restore refuses") {
		t.Fatalf("backup drift restore error = %v", err)
	}
	for _, target := range targets {
		assertLinkTarget(t, target, plan.Source.Path)
	}
}

func TestReceiptAndJournalRemainAppendOnly(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	result, err := installer.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(result.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(filepath.Dir(result.Receipt), "events.jsonl")
	eventsBefore, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "restore approval mismatch") {
		t.Fatalf("wrong restore approval error = %v", err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err != nil {
		t.Fatal(err)
	}
	receiptAfter, err := os.ReadFile(result.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receiptAfter, receiptBefore) {
		t.Fatal("restore modified the immutable receipt")
	}
	if !bytes.HasPrefix(eventsAfter, eventsBefore) || len(eventsAfter) <= len(eventsBefore) {
		t.Fatal("restore did not append to the event journal")
	}
	for _, path := range []string{result.Receipt, eventsPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private file %s mode = %v, error = %v", path, info.Mode().Perm(), err)
		}
	}
}

func TestManagedPreviousVersionUpdatesWithoutApproval(t *testing.T) {
	home := testHome(t)
	old := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	oldDigest, err := digestValue(old.desired)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(home, managedPath, oldDigest, "backscroll")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeShimTree(oldPath, old.shim); err != nil {
		t.Fatal(err)
	}
	for _, target := range targetPaths(old.home, old.configHome) {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(oldPath, target); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := New(Options{Home: home, ConfigHome: filepath.Join(home, ".config"), Shim: map[string][]byte{
		"SKILL.md": []byte("---\nname: backscroll\ndescription: updated\n---\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := updated.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("a valid previous managed version requires approval")
	}
	result, err := updated.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 3 {
		t.Fatalf("managed update changed = %d, want 3", result.Changed)
	}
	for _, target := range targetPaths(updated.home, updated.configHome) {
		assertLinkTarget(t, target, plan.Source.Path)
	}
}

func TestConcurrentInstallerLockIsRejected(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	lock, err := acquireLock(installer.state)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()
	if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "lock skill installation") {
		t.Fatalf("concurrent Apply error = %v", err)
	}
}

func TestFilesystemPreflightRunsBeforeReceiptOrDestinationMutation(t *testing.T) {
	home := testHome(t)
	deps := dependencies{
		syncDir:        syncDirectory,
		sameFilesystem: func(string, string) error { return errors.New("injected cross-filesystem target") },
	}
	installer := testInstaller(t, home, deps)
	if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "cross-filesystem") {
		t.Fatalf("Apply filesystem error = %v", err)
	}
	if receipts := receiptPaths(t, installer.state); len(receipts) != 0 {
		t.Fatalf("filesystem preflight created receipts: %v", receipts)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("filesystem preflight changed %s: %v", target, err)
		}
	}
}

func TestDigestUsesPythonASCIIAndHTMLRules(t *testing.T) {
	value := map[string]any{"path": "/tmp/café/<shim>", "symbol": "\U0001f642"}
	canonical, err := pythonCanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"path":"/tmp/caf\u00e9/<shim>","symbol":"\ud83d\ude42"}`
	if string(canonical) != want {
		t.Fatalf("canonical JSON = %s, want %s", canonical, want)
	}
}

func TestSnapshotJSONAndValidation(t *testing.T) {
	valid := []Snapshot{
		{Kind: "absent"},
		{Kind: "link", Target: "/managed"},
		{Kind: "file", Mode: 0o600, SHA256: strings.Repeat("a", 64)},
		{Kind: "directory", Mode: 0o700, Entries: map[string]Snapshot{}},
	}
	for _, snapshot := range valid {
		if _, err := json.Marshal(snapshot); err != nil {
			t.Fatalf("MarshalJSON(%s): %v", snapshot.Kind, err)
		}
		if err := validateSnapshot(snapshot); err != nil {
			t.Fatalf("validateSnapshot(%s): %v", snapshot.Kind, err)
		}
	}
	if _, err := json.Marshal(Snapshot{Kind: "invalid"}); err == nil {
		t.Fatal("MarshalJSON accepted an invalid kind")
	}
	invalid := []Snapshot{
		{Kind: "absent", Target: "unexpected"},
		{Kind: "link"},
		{Kind: "file", SHA256: "short"},
		{Kind: "directory"},
		{Kind: "directory", Entries: map[string]Snapshot{"..": {Kind: "absent"}}},
		{Kind: "directory", Entries: map[string]Snapshot{"child": {Kind: "invalid"}}},
		{Kind: "invalid"},
	}
	for _, snapshot := range invalid {
		if err := validateSnapshot(snapshot); err == nil {
			t.Fatalf("validateSnapshot accepted %#v", snapshot)
		}
	}
}

func TestInstallerInputValidation(t *testing.T) {
	home := testHome(t)
	fileHome := filepath.Join(filepath.Dir(home), "home-file")
	if err := os.WriteFile(fileHome, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	validShim := map[string][]byte{"SKILL.md": []byte("shim")}
	tests := []struct {
		name    string
		options Options
	}{
		{name: "empty home", options: Options{Shim: validShim}},
		{name: "missing home", options: Options{Home: filepath.Join(home, "missing"), Shim: validShim}},
		{name: "file home", options: Options{Home: fileHome, Shim: validShim}},
		{name: "empty shim", options: Options{Home: home}},
		{name: "missing skill", options: Options{Home: home, Shim: map[string][]byte{"reference.md": []byte("x")}}},
		{name: "unsafe shim", options: Options{Home: home, Shim: map[string][]byte{"../SKILL.md": []byte("x")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.options); err == nil {
				t.Fatalf("New accepted %+v", test.options)
			}
		})
	}
}

func TestReceiptValidationRejectsInvalidShapes(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	run := "validation-run"
	path := filepath.Join(installer.state, run, "receipt.json")
	base := receipt{Version: 2, Plan: plan, Run: run}
	clone := func() receipt {
		data, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		var result receipt
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	tests := []struct {
		name      string
		mutate    func(*receipt)
		recompute bool
	}{
		{name: "unsupported version", mutate: func(value *receipt) { value.Version = 3 }},
		{name: "run mismatch", mutate: func(value *receipt) { value.Run = "other" }},
		{name: "action mismatch", mutate: func(value *receipt) { value.Plan.Action = "remove" }},
		{name: "target count", mutate: func(value *receipt) { value.Plan.Targets = value.Plan.Targets[:2] }},
		{name: "target path", mutate: func(value *receipt) { value.Plan.Targets[0].Path = filepath.Join(home, "other") }},
		{name: "target snapshot", mutate: func(value *receipt) { value.Plan.Targets[0].Before = Snapshot{Kind: "invalid"} }},
		{name: "relative source", mutate: func(value *receipt) { value.Plan.Source.Path = "relative" }},
		{name: "plan digest", mutate: func(value *receipt) { value.Plan.Approval = strings.Repeat("0", 64) }},
		{name: "v2 source fields", recompute: true, mutate: func(value *receipt) { value.Plan.Source.Root = home }},
		{name: "v2 source path", recompute: true, mutate: func(value *receipt) { value.Plan.Source.Path = filepath.Join(home, "wrong") }},
		{name: "v1 incomplete", recompute: true, mutate: func(value *receipt) {
			value.Version = 1
			value.Plan.Version = 1
			value.Plan.ManagedChange = false
			value.Plan.Source = Source{}
		}},
		{name: "v1 source path", recompute: true, mutate: func(value *receipt) {
			value.Version = 1
			value.Plan.Version = 1
			value.Plan.ManagedChange = false
			value.Plan.Source = Source{Root: home, Path: filepath.Join(home, "wrong"), Commit: "commit", TreeSHA256: strings.Repeat("a", 64)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := clone()
			test.mutate(&value)
			if test.recompute {
				value.Plan.Approval, err = planDigest(value.Plan)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := validateReceipt(value, home, path); err == nil {
				t.Fatalf("validateReceipt accepted %+v", value)
			}
		})
	}
}

func TestReadReceiptRejectsMalformedData(t *testing.T) {
	directory := t.TempDir()
	tests := map[string]string{
		"invalid":          `{`,
		"trailing":         `{}` + "\n{}",
		"malformed-second": `{}` + "\n{",
		"unknown":          `{"version":2,"plan":{},"run":"x","extra":true}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(directory, name+".json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readReceipt(path); err == nil {
				t.Fatalf("readReceipt accepted %s", content)
			}
		})
	}
}

func TestRestoreAcceptsPythonV1UninstallReceipt(t *testing.T) {
	home := testHome(t)
	configHome := filepath.Join(home, ".config")
	state := filepath.Join(home, statePath)
	if err := makePrivateDir(state); err != nil {
		t.Fatal(err)
	}
	runName := "python-v1-uninstall"
	run := filepath.Join(state, runName)
	if err := makePrivateDir(run); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(home, "stable-product")
	source := filepath.Join(sourceRoot, ".claude/skills/backscroll")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		Version: 1, Action: "uninstall", Home: home, ConfigHome: configHome,
		Source: Source{Root: sourceRoot, Path: source, Commit: "commit", TreeSHA256: strings.Repeat("a", 64)},
	}
	for index, target := range targetPaths(home, configHome) {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		before := Snapshot{Kind: "link", Target: source}
		backup := filepath.Join(run, fmt.Sprintf("backup-%d", index))
		if err := os.Symlink(source, backup); err != nil {
			t.Fatal(err)
		}
		plan.Targets = append(plan.Targets, Target{Path: target, Before: before, Change: true})
	}
	var err error
	plan.Approval, err = planDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	value := receipt{Version: 1, Plan: plan, Run: runName}
	receiptPath := filepath.Join(run, "receipt.json")
	if err := writeJSONExclusive(receiptPath, value); err != nil {
		t.Fatal(err)
	}
	approval, err := digestValue(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Restore(home, receiptPath, approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 3 {
		t.Fatalf("uninstall receipt restored = %d, want 3", result.Restored)
	}
	for _, target := range targetPaths(home, configHome) {
		assertLinkTarget(t, target, source)
	}
}

func TestRestoreRejectsConcurrentLockAndInsecureJournal(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	result, err := installer.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := acquireLock(installer.state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err == nil || !strings.Contains(err.Error(), "lock skill installation") {
		t.Fatalf("concurrent Restore error = %v", err)
	}
	if err := lock.close(); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(filepath.Dir(result.Receipt), "events.jsonl")
	if err := os.Chmod(events, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, result.Receipt, result.RestoreApproval); err == nil || !strings.Contains(err.Error(), "event journal") {
		t.Fatalf("insecure journal Restore error = %v", err)
	}
	if err := os.Chmod(events, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManagedTargetRecognitionRejectsUnownedShapes(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	digest := strings.Repeat("a", 64)
	managed := filepath.Join(home, managedPath, digest, "backscroll")
	cases := []Snapshot{
		{Kind: "absent"},
		{Kind: "file", SHA256: digest},
		{Kind: "link", Target: "relative"},
		{Kind: "link", Target: filepath.Join(home, "outside")},
		{Kind: "link", Target: filepath.Join(home, managedPath, "invalid", "backscroll")},
		{Kind: "link", Target: filepath.Join(home, managedPath, digest, "extra", "backscroll")},
		{Kind: "link", Target: managed},
	}
	for _, before := range cases {
		owned, err := installer.isManagedTarget(before)
		if err != nil {
			t.Fatalf("isManagedTarget(%#v): %v", before, err)
		}
		if owned {
			t.Fatalf("isManagedTarget accepted %#v", before)
		}
	}
	if err := os.MkdirAll(filepath.Dir(managed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("not a tree"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned, err := installer.isManagedTarget(Snapshot{Kind: "link", Target: managed})
	if err != nil || owned {
		t.Fatalf("isManagedTarget regular file = %v, %v", owned, err)
	}
}

func TestNestedShimTreeIsInstalled(t *testing.T) {
	home := testHome(t)
	installer, err := New(Options{
		Home: home,
		Shim: map[string][]byte{
			"SKILL.md":       []byte("shim"),
			"refs/nested.md": []byte("nested"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Apply(""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(plan.Source.Path, "refs", "nested.md"))
	if err != nil || string(content) != "nested" {
		t.Fatalf("nested shim content = %q, error = %v", content, err)
	}
}

func TestRestoreRejectsUnsafeRequests(t *testing.T) {
	home := testHome(t)
	if _, err := Restore("", "/missing/receipt.json", "approval"); err == nil {
		t.Fatal("Restore accepted an empty home")
	}
	outside := filepath.Join(filepath.Dir(home), "receipt.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, outside, "approval"); err == nil || !strings.Contains(err.Error(), "must belong") {
		t.Fatalf("outside receipt error = %v", err)
	}
	state := filepath.Join(home, statePath)
	run := filepath.Join(state, "unsafe")
	if err := makePrivateDir(run); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(run, "receipt.json")
	if _, err := Restore(home, missing, "approval"); err == nil || !strings.Contains(err.Error(), "inspect receipt") {
		t.Fatalf("missing receipt error = %v", err)
	}
	if err := os.WriteFile(missing, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(home, missing, "approval"); err == nil || !strings.Contains(err.Error(), "secure regular file") {
		t.Fatalf("insecure receipt error = %v", err)
	}
}

func TestTargetPublicationFailuresRollback(t *testing.T) {
	t.Run("absent destination publish", func(t *testing.T) {
		home := testHome(t)
		moves := 0
		deps := dependencies{syncDir: syncDirectory}
		deps.beforeMove = func(oldPath, newPath string) error {
			moves++
			if moves == 2 {
				return errors.New("injected destination publish failure")
			}
			return nil
		}
		installer := testInstaller(t, home, deps)
		if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "destination publish failure") {
			t.Fatalf("Apply error = %v", err)
		}
		for _, target := range targetPaths(installer.home, installer.configHome) {
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("failed publish changed %s: %v", target, err)
			}
		}
	})

	t.Run("backup publish", func(t *testing.T) {
		home := testHome(t)
		moves := 0
		deps := dependencies{syncDir: syncDirectory}
		deps.beforeMove = func(oldPath, newPath string) error {
			moves++
			if moves == 2 {
				return errors.New("injected backup publish failure")
			}
			return nil
		}
		installer := testInstaller(t, home, deps)
		seedUnknown(t, installer)
		plan, err := installer.Plan()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := installer.Apply(plan.Approval); err == nil || !strings.Contains(err.Error(), "backup publish failure") {
			t.Fatalf("Apply error = %v", err)
		}
		for _, target := range targetPaths(installer.home, installer.configHome) {
			if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
				t.Fatalf("backup failure changed %s", target)
			}
		}
	})

	t.Run("replacement sync", func(t *testing.T) {
		home := testHome(t)
		installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
		firstParent := filepath.Dir(targetPaths(installer.home, installer.configHome)[0])
		failed := false
		installer.deps.syncDir = func(path string) error {
			if path == firstParent && !failed {
				failed = true
				return errors.New("injected replacement sync failure")
			}
			return syncDirectory(path)
		}
		if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "replacement sync failure") {
			t.Fatalf("Apply error = %v", err)
		}
		for _, target := range targetPaths(installer.home, installer.configHome) {
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("sync failure changed %s: %v", target, err)
			}
		}
	})
}

func TestSerializationAndPrivateStateErrors(t *testing.T) {
	if _, err := digestValue(make(chan int)); err == nil {
		t.Fatal("digestValue accepted an unsupported value")
	}
	directory := t.TempDir()
	if err := appendEvent(directory, make(chan int)); err == nil {
		t.Fatal("appendEvent accepted an unsupported value")
	}
	path := filepath.Join(directory, "receipt.json")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONExclusive(path, map[string]any{}); err == nil {
		t.Fatal("writeJSONExclusive replaced an existing receipt")
	}
	insecure := filepath.Join(directory, "insecure")
	if err := os.Mkdir(insecure, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := makePrivateDir(insecure); err == nil {
		t.Fatal("makePrivateDir accepted an insecure mode")
	}
	if err := sameFilesystemWithMissing(directory, "/dev"); err == nil {
		left, leftErr := statExistingAncestor(directory)
		right, rightErr := statExistingAncestor("/dev")
		if leftErr == nil && rightErr == nil {
			leftDevice, _ := deviceID(left)
			rightDevice, _ := deviceID(right)
			if leftDevice != rightDevice {
				t.Fatal("sameFilesystemWithMissing accepted different devices")
			}
		}
	}
}

func TestPlanRejectsChangedManagedTree(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.Source.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.Source.Path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Plan(); err == nil || !strings.Contains(err.Error(), "managed shim content is invalid") {
		t.Fatalf("Plan changed managed tree error = %v", err)
	}
}

func TestApplyChecksEveryDurabilitySync(t *testing.T) {
	countCalls := func(kind string) int {
		home := testHome(t)
		calls := 0
		deps := dependencies{}
		deps.syncDir = func(path string) error {
			if kind == "directory" {
				calls++
			}
			return syncDirectory(path)
		}
		deps.syncFile = func(file *os.File) error {
			if kind == "file" {
				calls++
			}
			return syncFile(file)
		}
		installer := testInstaller(t, home, deps)
		if _, err := installer.Apply(""); err != nil {
			t.Fatal(err)
		}
		return calls
	}
	for _, kind := range []string{"file", "directory"} {
		total := countCalls(kind)
		if total == 0 {
			t.Fatalf("Apply performed no %s fsync", kind)
		}
		for failAt := 1; failAt <= total; failAt++ {
			t.Run(fmt.Sprintf("%s-%d", kind, failAt), func(t *testing.T) {
				home := testHome(t)
				calls := 0
				deps := dependencies{}
				deps.syncDir = func(path string) error {
					if kind == "directory" {
						calls++
						if calls == failAt {
							return fmt.Errorf("injected directory fsync failure %d", failAt)
						}
					}
					return syncDirectory(path)
				}
				deps.syncFile = func(file *os.File) error {
					if kind == "file" {
						calls++
						if calls == failAt {
							return fmt.Errorf("injected file fsync failure %d", failAt)
						}
					}
					return syncFile(file)
				}
				installer := testInstaller(t, home, deps)
				if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "injected "+kind+" fsync failure") {
					t.Fatalf("Apply error = %v", err)
				}
				for _, target := range targetPaths(installer.home, installer.configHome) {
					if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatalf("%s fsync failure %d changed %s: %v", kind, failAt, target, err)
					}
				}
			})
		}
	}
}

func TestRollbackFsyncFailureReportsRecoveryCredentials(t *testing.T) {
	home := testHome(t)
	deps := dependencies{}
	rollback := false
	exchanges := 0
	deps.beforeExchange = func(string, string) error {
		exchanges++
		if exchanges == 2 {
			rollback = true
			return errors.New("injected apply failure")
		}
		return nil
	}
	deps.syncDir = func(path string) error {
		if rollback {
			rollback = false
			return errors.New("injected rollback directory fsync failure")
		}
		return syncDirectory(path)
	}
	installer := testInstaller(t, home, deps)
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.Apply(plan.Approval)
	if err == nil || !strings.Contains(err.Error(), "injected rollback directory fsync failure") {
		t.Fatalf("Apply rollback error = %v", err)
	}
	if !strings.Contains(err.Error(), "receipt:") || !strings.Contains(err.Error(), "restore_approval:") {
		t.Fatalf("Apply rollback error omits recovery credentials: %v", err)
	}
}

func TestRollbackJournalFsyncFailureReturnsError(t *testing.T) {
	home := testHome(t)
	deps := dependencies{}
	rollback := false
	exchanges := 0
	deps.beforeExchange = func(string, string) error {
		exchanges++
		if exchanges == 2 {
			rollback = true
			return errors.New("injected apply failure")
		}
		return nil
	}
	deps.syncFile = func(file *os.File) error {
		if rollback {
			rollback = false
			return errors.New("injected rollback journal fsync failure")
		}
		return syncFile(file)
	}
	installer := testInstaller(t, home, deps)
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.Apply(plan.Approval)
	if err == nil || !strings.Contains(err.Error(), "injected rollback journal fsync failure") {
		t.Fatalf("Apply rollback journal error = %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
			t.Fatalf("Rollback journal failure changed %s", target)
		}
	}
}

func TestRestoreChecksEveryDurabilitySync(t *testing.T) {
	setup := func(t *testing.T) (string, ApplyResult) {
		home := testHome(t)
		installer := testInstaller(t, home, dependencies{})
		seedUnknown(t, installer)
		plan, err := installer.Plan()
		if err != nil {
			t.Fatal(err)
		}
		result, err := installer.Apply(plan.Approval)
		if err != nil {
			t.Fatal(err)
		}
		return home, result
	}
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			home, result := setup(t)
			calls := 0
			deps := dependencies{}
			deps.syncDir = func(path string) error {
				if kind == "directory" {
					calls++
				}
				return syncDirectory(path)
			}
			deps.syncFile = func(file *os.File) error {
				if kind == "file" {
					calls++
				}
				return syncFile(file)
			}
			if _, err := restoreWithDependencies(home, result.Receipt, result.RestoreApproval, deps); err != nil {
				t.Fatal(err)
			}
			if calls == 0 {
				t.Fatalf("Restore performed no %s fsync", kind)
			}
			for failAt := 1; failAt <= calls; failAt++ {
				t.Run(fmt.Sprintf("%d", failAt), func(t *testing.T) {
					home, result := setup(t)
					current := 0
					deps := dependencies{}
					deps.syncDir = func(path string) error {
						if kind == "directory" {
							current++
							if current == failAt {
								return fmt.Errorf("injected restore directory fsync failure %d", failAt)
							}
						}
						return syncDirectory(path)
					}
					deps.syncFile = func(file *os.File) error {
						if kind == "file" {
							current++
							if current == failAt {
								return fmt.Errorf("injected restore file fsync failure %d", failAt)
							}
						}
						return syncFile(file)
					}
					if _, err := restoreWithDependencies(home, result.Receipt, result.RestoreApproval, deps); err == nil || !strings.Contains(err.Error(), "injected restore "+kind+" fsync failure") {
						t.Fatalf("Restore error = %v", err)
					}
				})
			}
		})
	}
}

func TestWriteShimTreeRejectsConflictingOrExistingPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shim")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeShimTree(root, map[string][]byte{"SKILL.md": []byte("shim")}); err == nil {
		t.Fatal("writeShimTree accepted an existing root")
	}
	conflict := filepath.Join(t.TempDir(), "conflict")
	err := writeShimTree(conflict, map[string][]byte{
		"SKILL.md": []byte("shim"),
		"ref":      []byte("file"),
		"ref/doc":  []byte("nested"),
	})
	if err == nil {
		t.Fatal("writeShimTree accepted conflicting file and directory paths")
	}
	_ = removeImmutableTree(conflict)
}

func TestManagedPublishFailureLeavesDestinationsUntouched(t *testing.T) {
	home := testHome(t)
	deps := dependencies{
		beforeMove: func(string, string) error {
			return errors.New("injected managed publish failure")
		},
		syncDir: syncDirectory,
	}
	installer := testInstaller(t, home, deps)
	if _, err := installer.Apply(""); err == nil || !strings.Contains(err.Error(), "managed publish failure") {
		t.Fatalf("Apply error = %v", err)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("managed publish failure changed %s: %v", target, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(home, managedPath, "*", ".backscroll-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("managed staging paths = %v, error = %v", matches, err)
	}
}

func TestInterruptedInstallCanRestore(t *testing.T) {
	if os.Getenv("BACKSCROLL_SKILL_CRASH_HELPER") == "1" {
		home := os.Getenv("BACKSCROLL_SKILL_CRASH_HOME")
		calls := 0
		deps := dependencies{syncDir: syncDirectory}
		deps.beforeExchange = func(left, right string) error {
			calls++
			if calls == 2 {
				os.Exit(71)
			}
			return nil
		}
		installer := testInstaller(t, home, deps)
		_, _ = installer.Apply(os.Getenv("BACKSCROLL_SKILL_CRASH_APPROVAL"))
		os.Exit(72)
	}

	home := testHome(t)
	installer := testInstaller(t, home, dependencies{syncDir: syncDirectory})
	seedUnknown(t, installer)
	plan, err := installer.Plan()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestInterruptedInstallCanRestore$")
	command.Env = append(os.Environ(),
		"BACKSCROLL_SKILL_CRASH_HELPER=1",
		"BACKSCROLL_SKILL_CRASH_HOME="+home,
		"BACKSCROLL_SKILL_CRASH_APPROVAL="+plan.Approval,
	)
	if err := command.Run(); err == nil {
		t.Fatal("crash helper succeeded")
	} else if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 71 {
		t.Fatalf("crash helper error = %v", err)
	}
	receipts := receiptPaths(t, installer.state)
	if len(receipts) != 1 {
		t.Fatalf("crash receipts = %v", receipts)
	}
	events, err := os.ReadFile(filepath.Join(filepath.Dir(receipts[0]), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var prepared map[string]any
	if err := json.Unmarshal([]byte(strings.Split(string(events), "\n")[0]), &prepared); err != nil {
		t.Fatal(err)
	}
	approval, ok := prepared["restore_approval"].(string)
	if !ok {
		t.Fatalf("prepared event = %v", prepared)
	}
	result, err := Restore(home, receipts[0], approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored != 2 {
		t.Fatalf("interrupted restore count = %d, want 2", result.Restored)
	}
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if got := mustRead(t, filepath.Join(target, "local-note")); got != "operator data" {
			t.Fatalf("interrupted restore changed %s", target)
		}
	}
}

func testHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	home = resolved
	t.Cleanup(func() {
		_ = filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(home, ".config/backscroll"))
	return home
}

func testInstaller(t *testing.T, home string, deps dependencies) *Installer {
	t.Helper()
	installer, err := newInstaller(Options{
		Home:       home,
		ConfigHome: filepath.Join(home, ".config"),
		Shim: map[string][]byte{
			"SKILL.md":            []byte("---\nname: backscroll\n---\n"),
			"ref-context-mode.md": []byte("reference\n"),
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	return installer
}

func seedUnknown(t *testing.T, installer *Installer) {
	t.Helper()
	for _, target := range targetPaths(installer.home, installer.configHome) {
		if err := os.MkdirAll(target, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "local-note"), []byte("operator data"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func assertLinkTarget(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", path, err)
	}
	if got != want {
		t.Fatalf("Readlink(%s) = %s, want %s", path, got, want)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func receiptPaths(t *testing.T, state string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(state, "*", "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func targetMetadata(t *testing.T, paths []string) map[string]os.FileInfo {
	t.Helper()
	result := make(map[string]os.FileInfo, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		result[path] = info
	}
	return result
}
