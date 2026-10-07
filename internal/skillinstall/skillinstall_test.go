package skillinstall

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstallSuccessAndIdempotence(t *testing.T) {
	home := testHome(t)
	installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})

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
	installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
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

func TestPartialFailureRestoresAllDestinations(t *testing.T) {
	home := testHome(t)
	calls := 0
	deps := dependencies{moveNoReplace: moveNoReplace, syncDir: syncDirectory}
	deps.exchange = func(left, right string) error {
		calls++
		if calls == 2 {
			return errors.New("injected second destination failure")
		}
		return exchangePaths(left, right)
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
	installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
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
	installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
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
