// Package skillinstall installs the Backscroll skill discovery shim.
package skillinstall

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const statePath = ".local/state/backscroll/skill-install"

//go:embed assets/*
var officialShimFiles embed.FS

// OfficialShim returns a copy of the skill discovery shim shipped with Backscroll.
func OfficialShim() (map[string][]byte, error) {
	entries, err := officialShimFiles.ReadDir("assets")
	if err != nil {
		return nil, fmt.Errorf("read embedded shim: %w", err)
	}
	shim := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("embedded shim contains an unsupported directory: %s", entry.Name())
		}
		data, err := officialShimFiles.ReadFile("assets/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded shim file %s: %w", entry.Name(), err)
		}
		shim[entry.Name()] = data
	}
	return shim, nil
}

// Options defines one skill installation.
type Options struct {
	Home       string
	ConfigHome string
	Shim       map[string][]byte
}

// Snapshot records a filesystem object without following symbolic links.
type Snapshot struct {
	Kind    string              `json:"kind"`
	Target  string              `json:"target,omitempty"`
	Mode    uint32              `json:"mode,omitempty"`
	SHA256  string              `json:"sha256,omitempty"`
	Entries map[string]Snapshot `json:"entries,omitempty"`
}

// MarshalJSON preserves the Python v1 snapshot shape.
func (snapshot Snapshot) MarshalJSON() ([]byte, error) {
	var value any
	switch snapshot.Kind {
	case "absent":
		value = struct {
			Kind string `json:"kind"`
		}{Kind: snapshot.Kind}
	case "link":
		value = struct {
			Kind   string `json:"kind"`
			Target string `json:"target"`
		}{Kind: snapshot.Kind, Target: snapshot.Target}
	case "file":
		value = struct {
			Kind   string `json:"kind"`
			Mode   uint32 `json:"mode"`
			SHA256 string `json:"sha256"`
		}{Kind: snapshot.Kind, Mode: snapshot.Mode, SHA256: snapshot.SHA256}
	case "directory":
		value = struct {
			Kind    string              `json:"kind"`
			Mode    uint32              `json:"mode"`
			Entries map[string]Snapshot `json:"entries"`
		}{Kind: snapshot.Kind, Mode: snapshot.Mode, Entries: snapshot.Entries}
	default:
		return nil, fmt.Errorf("invalid snapshot kind: %q", snapshot.Kind)
	}
	return json.Marshal(value)
}

// Source identifies the immutable managed shim or a Python v1 source clone.
type Source struct {
	Root       string `json:"root,omitempty"`
	Path       string `json:"path"`
	Commit     string `json:"commit,omitempty"`
	TreeSHA256 string `json:"tree_sha256"`
}

// Target records one discovery destination and its approved preimage.
type Target struct {
	Path   string   `json:"path"`
	Before Snapshot `json:"before"`
	Change bool     `json:"change"`
}

// Plan is a read-only installation inventory.
type Plan struct {
	Version          int      `json:"version"`
	Action           string   `json:"action"`
	Home             string   `json:"home"`
	ConfigHome       string   `json:"config_home"`
	Source           Source   `json:"source"`
	Targets          []Target `json:"targets"`
	ManagedChange    bool     `json:"managed_change,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
	Approval         string   `json:"approval"`
}

// ApplyResult reports a completed installation.
type ApplyResult struct {
	Changed         int    `json:"changed"`
	Receipt         string `json:"receipt,omitempty"`
	RestoreApproval string `json:"restore_approval,omitempty"`
}

// RestoreResult reports a completed restoration.
type RestoreResult struct {
	Restored int    `json:"restored"`
	Receipt  string `json:"receipt"`
}

type receipt struct {
	Version int    `json:"version"`
	Plan    Plan   `json:"plan"`
	Run     string `json:"run"`
}

type dependencies struct {
	exchange      func(string, string) error
	moveNoReplace func(string, string) error
	syncDir       func(string) error
}

// Installer holds validated paths and immutable shim bytes.
type Installer struct {
	home       string
	configHome string
	state      string
	shim       map[string][]byte
	desired    Snapshot
	deps       dependencies
}

// New validates an installation request. It does not modify the filesystem.
func New(options Options) (*Installer, error) {
	return newInstaller(options, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
}

func newInstaller(options Options, deps dependencies) (*Installer, error) {
	home, err := canonicalHome(options.Home)
	if err != nil {
		return nil, err
	}
	configHome := options.ConfigHome
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	configHome, err = absolutePath(configHome)
	if err != nil {
		return nil, fmt.Errorf("validate config home: %w", err)
	}
	shim, err := validateShim(options.Shim)
	if err != nil {
		return nil, err
	}
	desired := desiredSnapshot(shim)
	return &Installer{
		home:       home,
		configHome: configHome,
		state:      filepath.Join(home, statePath),
		shim:       shim,
		desired:    desired,
		deps:       deps,
	}, nil
}

// Plan inventories all supported discovery destinations.
func (i *Installer) Plan() (Plan, error) {
	treeDigest, err := digestValue(i.desired)
	if err != nil {
		return Plan{}, err
	}
	managed := filepath.Join(i.state, "managed", treeDigest, "backscroll")
	if err := validateLayout(i.home, i.configHome, i.state, managed, targetPaths(i.home, i.configHome)); err != nil {
		return Plan{}, err
	}
	managedSnapshot, err := snapshot(managed)
	if err != nil {
		return Plan{}, fmt.Errorf("inspect managed shim: %w", err)
	}
	if managedSnapshot.Kind != "absent" && !reflect.DeepEqual(managedSnapshot, i.desired) {
		return Plan{}, fmt.Errorf("managed shim content is invalid: %s", managed)
	}

	plan := Plan{
		Version:       2,
		Action:        "install",
		Home:          i.home,
		ConfigHome:    i.configHome,
		ManagedChange: managedSnapshot.Kind == "absent",
		Source: Source{
			Path:       managed,
			TreeSHA256: treeDigest,
		},
	}
	for _, path := range targetPaths(i.home, i.configHome) {
		before, err := snapshot(path)
		if err != nil {
			return Plan{}, fmt.Errorf("inspect destination %s: %w", path, err)
		}
		change := !reflect.DeepEqual(before, Snapshot{Kind: "link", Target: managed})
		if change && before.Kind != "absent" {
			plan.RequiresApproval = true
		}
		plan.Targets = append(plan.Targets, Target{Path: path, Before: before, Change: change})
	}
	plan.Approval, err = planDigest(plan)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// Apply installs the managed shim. Approval is mandatory when a destination is not absent.
func (i *Installer) Apply(approval string) (result ApplyResult, err error) {
	plan, err := i.Plan()
	if err != nil {
		return ApplyResult{}, err
	}
	if plan.RequiresApproval && approval != plan.Approval {
		return ApplyResult{}, errors.New("approval mismatch; inspect a fresh plan before authorizing")
	}
	if !plan.RequiresApproval && approval != "" && approval != plan.Approval {
		return ApplyResult{}, errors.New("approval mismatch; inspect a fresh plan before authorizing")
	}
	if countChanges(plan.Targets) == 0 && !plan.ManagedChange {
		return ApplyResult{}, nil
	}

	lock, err := acquireLock(i.state)
	if err != nil {
		return ApplyResult{}, err
	}
	defer func() { err = errors.Join(err, lock.close()) }()

	lockedPlan, err := i.Plan()
	if err != nil {
		return ApplyResult{}, err
	}
	if !reflect.DeepEqual(lockedPlan, plan) {
		return ApplyResult{}, errors.New("inventory changed before installation")
	}

	runName := uuid.NewString()
	run := filepath.Join(i.state, runName)
	if err := makePrivateDir(run); err != nil {
		return ApplyResult{}, err
	}
	receiptValue := receipt{Version: 2, Plan: plan, Run: runName}
	receiptPath := filepath.Join(run, "receipt.json")
	if err := writeJSONExclusive(receiptPath, receiptValue); err != nil {
		return ApplyResult{}, err
	}
	if err := i.deps.syncDir(run); err != nil {
		return ApplyResult{}, err
	}
	if err := i.deps.syncDir(i.state); err != nil {
		return ApplyResult{}, err
	}
	restoreApproval, err := digestValue(receiptValue)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := appendEvent(run, map[string]any{"event": "prepared", "restore_approval": restoreApproval}); err != nil {
		return ApplyResult{}, err
	}

	rollback := func(cause error) error {
		_ = appendEvent(run, map[string]any{"event": "failed", "receipt": receiptPath})
		_, restoreErr := restoreRows(run, receiptValue, i.deps)
		if restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("automatic restore failed: %w", restoreErr))
		}
		return cause
	}
	if err := i.ensureManaged(run, plan.Source.Path); err != nil {
		return ApplyResult{}, rollback(err)
	}

	changed := 0
	for index, row := range plan.Targets {
		if !row.Change {
			continue
		}
		if err := i.installTarget(run, receiptValue.Run, index, row, plan.Source.Path); err != nil {
			return ApplyResult{}, rollback(err)
		}
		changed++
	}
	managedSnapshot, err := snapshot(plan.Source.Path)
	if err != nil || !reflect.DeepEqual(managedSnapshot, i.desired) {
		if err == nil {
			err = errors.New("managed shim changed during installation")
		}
		return ApplyResult{}, rollback(err)
	}
	if err := appendEvent(run, map[string]any{"event": "complete", "changed": changed}); err != nil {
		return ApplyResult{}, rollback(err)
	}
	return ApplyResult{Changed: changed, Receipt: receiptPath, RestoreApproval: restoreApproval}, nil
}

// Restore restores one validated Go v2 or Python v1 receipt.
func Restore(home, receiptPath, approval string) (result RestoreResult, err error) {
	canonical, err := canonicalHome(home)
	if err != nil {
		return RestoreResult{}, err
	}
	state := filepath.Join(canonical, statePath)
	lock, err := acquireLock(state)
	if err != nil {
		return RestoreResult{}, err
	}
	defer func() { err = errors.Join(err, lock.close()) }()

	path, err := absolutePath(receiptPath)
	if err != nil {
		return RestoreResult{}, err
	}
	if filepath.Base(path) != "receipt.json" || filepath.Dir(filepath.Dir(path)) != state {
		return RestoreResult{}, errors.New("receipt must belong to this home's installation state")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("inspect receipt: %w", err)
	}
	links, linkErr := linkCount(info)
	owner, ownerErr := ownerID(info)
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || linkErr != nil || ownerErr != nil || links != 1 || owner != currentUserID() || info.Mode().Perm() != 0o600 {
		return RestoreResult{}, errors.New("receipt must be a secure regular file")
	}
	if err := makePrivateDir(filepath.Dir(path)); err != nil {
		return RestoreResult{}, err
	}
	receiptValue, actualApproval, err := readReceipt(path)
	if err != nil {
		return RestoreResult{}, err
	}
	if actualApproval != approval {
		return RestoreResult{}, errors.New("restore approval mismatch")
	}
	if err := validateReceipt(receiptValue, canonical, path); err != nil {
		return RestoreResult{}, err
	}
	deps := dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory}
	count, err := restoreRows(filepath.Dir(path), receiptValue, deps)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := appendEvent(filepath.Dir(path), map[string]any{"event": "restore-complete", "restored": count}); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Restored: count, Receipt: path}, nil
}

func (i *Installer) ensureManaged(run, managed string) error {
	current, err := snapshot(managed)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current, i.desired) {
		return nil
	}
	if current.Kind != "absent" {
		return fmt.Errorf("managed shim content is invalid: %s", managed)
	}
	parent := filepath.Dir(managed)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create managed shim parent: %w", err)
	}
	for _, directory := range []string{parent, filepath.Dir(parent), i.state} {
		if err := i.deps.syncDir(directory); err != nil {
			return err
		}
	}
	if err := safeParents(managed); err != nil {
		return err
	}
	staging := filepath.Join(run, "managed-backscroll")
	if err := writeShimTree(staging, i.shim); err != nil {
		return err
	}
	if err := i.deps.syncDir(run); err != nil {
		return err
	}
	if err := i.deps.moveNoReplace(staging, managed); err != nil {
		return fmt.Errorf("publish managed shim: %w", err)
	}
	if err := i.deps.syncDir(parent); err != nil {
		return err
	}
	return nil
}

func (i *Installer) installTarget(run, runName string, index int, row Target, managed string) error {
	if err := safeParents(row.Path); err != nil {
		return err
	}
	current, err := snapshot(row.Path)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, row.Before) {
		return fmt.Errorf("destination changed during installation: %s", row.Path)
	}
	parent := filepath.Dir(row.Path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}
	if err := safeParents(row.Path); err != nil {
		return err
	}
	if err := sameFilesystem(parent, run); err != nil {
		return err
	}
	if err := appendEvent(run, map[string]any{"event": "install-intent", "path": row.Path}); err != nil {
		return err
	}
	temp := swapPath(row.Path, runName, index)
	if _, err := os.Lstat(temp); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("temporary replacement already exists: %s", temp)
		}
		return err
	}
	if err := os.Symlink(managed, temp); err != nil {
		return fmt.Errorf("create replacement link: %w", err)
	}
	if err := i.deps.syncDir(parent); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if row.Before.Kind == "absent" {
		if err := i.deps.moveNoReplace(temp, row.Path); err != nil {
			_ = os.Remove(temp)
			return fmt.Errorf("install destination: %w", err)
		}
	} else {
		if err := i.deps.exchange(temp, row.Path); err != nil {
			_ = os.Remove(temp)
			return fmt.Errorf("atomically replace destination: %w", err)
		}
		if err := i.deps.syncDir(parent); err != nil {
			return err
		}
		backup := filepath.Join(run, fmt.Sprintf("backup-%d", index))
		if err := i.deps.moveNoReplace(temp, backup); err != nil {
			return fmt.Errorf("move destination preimage to backup: %w", err)
		}
		if err := i.deps.syncDir(run); err != nil {
			return err
		}
	}
	if err := i.deps.syncDir(parent); err != nil {
		return err
	}
	return appendEvent(run, map[string]any{"event": "installed", "path": row.Path})
}

func restoreRows(run string, value receipt, deps dependencies) (int, error) {
	after := Snapshot{Kind: "absent"}
	if value.Plan.Action == "install" {
		after = Snapshot{Kind: "link", Target: value.Plan.Source.Path}
	}
	type pendingRow struct {
		target      string
		saved       string
		before      Snapshot
		cleanupOnly bool
	}
	var pending []pendingRow
	for index, row := range value.Plan.Targets {
		if !row.Change {
			continue
		}
		if err := safeParents(row.Path); err != nil {
			return 0, err
		}
		current, err := snapshot(row.Path)
		if err != nil {
			return 0, err
		}
		backup := filepath.Join(run, fmt.Sprintf("backup-%d", index))
		backupSnapshot, err := snapshot(backup)
		if err != nil {
			return 0, err
		}
		temp := ""
		tempSnapshot := Snapshot{Kind: "absent"}
		if value.Version == 2 {
			temp = swapPath(row.Path, value.Run, index)
			tempSnapshot, err = snapshot(temp)
			if err != nil {
				return 0, err
			}
		}
		if reflect.DeepEqual(current, row.Before) && backupSnapshot.Kind == "absent" {
			if tempSnapshot.Kind == "absent" {
				continue
			}
			if reflect.DeepEqual(tempSnapshot, after) {
				pending = append(pending, pendingRow{target: row.Path, saved: temp, before: row.Before, cleanupOnly: true})
				continue
			}
		}
		if !reflect.DeepEqual(current, after) && current.Kind != "absent" {
			return 0, fmt.Errorf("restore refuses changed destination or backup: %s", row.Path)
		}
		saved := ""
		if row.Before.Kind == "absent" {
			if backupSnapshot.Kind != "absent" || tempSnapshot.Kind != "absent" {
				return 0, fmt.Errorf("restore refuses changed destination or backup: %s", row.Path)
			}
		} else {
			switch {
			case reflect.DeepEqual(backupSnapshot, row.Before) && tempSnapshot.Kind == "absent":
				saved = backup
			case backupSnapshot.Kind == "absent" && reflect.DeepEqual(tempSnapshot, row.Before):
				saved = temp
			default:
				return 0, fmt.Errorf("restore refuses changed destination or backup: %s", row.Path)
			}
		}
		pending = append(pending, pendingRow{target: row.Path, saved: saved, before: row.Before})
	}
	for _, row := range pending {
		if err := appendEvent(run, map[string]any{"event": "restore-intent", "path": row.target}); err != nil {
			return 0, err
		}
		if row.cleanupOnly {
			if err := os.Remove(row.saved); err != nil {
				return 0, fmt.Errorf("remove unused replacement: %w", err)
			}
			if err := deps.syncDir(filepath.Dir(row.target)); err != nil {
				return 0, err
			}
			if err := appendEvent(run, map[string]any{"event": "restored", "path": row.target}); err != nil {
				return 0, err
			}
			continue
		}
		current, err := snapshot(row.target)
		if err != nil {
			return 0, err
		}
		if current.Kind != "absent" {
			if err := os.Remove(row.target); err != nil {
				return 0, fmt.Errorf("remove installed destination: %w", err)
			}
		}
		if row.before.Kind != "absent" {
			if err := deps.moveNoReplace(row.saved, row.target); err != nil {
				return 0, fmt.Errorf("restore destination: %w", err)
			}
		}
		if err := deps.syncDir(filepath.Dir(row.target)); err != nil {
			return 0, err
		}
		if err := deps.syncDir(run); err != nil {
			return 0, err
		}
		if err := appendEvent(run, map[string]any{"event": "restored", "path": row.target}); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}

func validateReceipt(value receipt, home, path string) error {
	if value.Version != 1 && value.Version != 2 {
		return errors.New("unsupported receipt version")
	}
	if value.Run != filepath.Base(filepath.Dir(path)) || value.Plan.Home != home || value.Plan.Version != value.Version {
		return errors.New("receipt identity mismatch")
	}
	if value.Plan.Action != "install" && (value.Version != 1 || value.Plan.Action != "uninstall") {
		return errors.New("receipt action mismatch")
	}
	configHome, err := absolutePath(value.Plan.ConfigHome)
	if err != nil {
		return fmt.Errorf("receipt config home: %w", err)
	}
	expected := targetPaths(home, configHome)
	if len(value.Plan.Targets) != len(expected) {
		return errors.New("receipt destinations mismatch")
	}
	for index, target := range value.Plan.Targets {
		if target.Path != expected[index] {
			return errors.New("receipt destinations mismatch")
		}
		if err := validateSnapshot(target.Before); err != nil {
			return fmt.Errorf("receipt destination snapshot: %w", err)
		}
	}
	if !filepath.IsAbs(value.Plan.Source.Path) {
		return errors.New("receipt source path must be absolute")
	}
	approval, err := planDigest(value.Plan)
	if err != nil {
		return err
	}
	if approval != value.Plan.Approval {
		return errors.New("receipt plan digest mismatch")
	}
	if value.Version == 1 {
		if value.Plan.ManagedChange || value.Plan.Source.Root == "" || value.Plan.Source.Commit == "" || value.Plan.Source.TreeSHA256 == "" {
			return errors.New("Python v1 receipt source is incomplete")
		}
		root, err := absolutePath(value.Plan.Source.Root)
		if err != nil || value.Plan.Source.Path != filepath.Join(root, ".claude/skills/backscroll") {
			return errors.New("Python v1 receipt source path is invalid")
		}
	} else {
		if value.Plan.Source.Root != "" || value.Plan.Source.Commit != "" || !isSHA256(value.Plan.Source.TreeSHA256) {
			return errors.New("Go v2 receipt source is invalid")
		}
		expectedSource := filepath.Join(home, statePath, "managed", value.Plan.Source.TreeSHA256, "backscroll")
		if value.Plan.Source.Path != expectedSource {
			return errors.New("Go v2 receipt source path is invalid")
		}
	}
	return nil
}

func readReceipt(path string) (receipt, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return receipt{}, "", fmt.Errorf("read receipt: %w", err)
	}
	var raw any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return receipt{}, "", fmt.Errorf("decode receipt: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return receipt{}, "", err
	}
	approval, err := digestValue(raw)
	if err != nil {
		return receipt{}, "", err
	}
	var value receipt
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return receipt{}, "", fmt.Errorf("decode receipt fields: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return receipt{}, "", err
	}
	return value, approval, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("receipt contains trailing JSON data")
		}
		return fmt.Errorf("decode receipt trailing data: %w", err)
	}
	return nil
}

func planDigest(plan Plan) (string, error) {
	plan.Approval = ""
	type planWithoutApproval struct {
		Version          int      `json:"version"`
		Action           string   `json:"action"`
		Home             string   `json:"home"`
		ConfigHome       string   `json:"config_home"`
		Source           Source   `json:"source"`
		Targets          []Target `json:"targets"`
		ManagedChange    bool     `json:"managed_change,omitempty"`
		RequiresApproval bool     `json:"requires_approval,omitempty"`
	}
	return digestValue(planWithoutApproval{
		Version: plan.Version, Action: plan.Action, Home: plan.Home, ConfigHome: plan.ConfigHome,
		Source: plan.Source, Targets: plan.Targets, ManagedChange: plan.ManagedChange,
		RequiresApproval: plan.RequiresApproval,
	})
}

func digestValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode digest input: %w", err)
	}
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return "", fmt.Errorf("canonicalize digest input: %w", err)
	}
	data, err = json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode canonical digest input: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateSnapshot(value Snapshot) error {
	emptyEntries := len(value.Entries) == 0
	switch value.Kind {
	case "absent":
		if value.Target != "" || value.Mode != 0 || value.SHA256 != "" || !emptyEntries {
			return errors.New("invalid absent snapshot")
		}
	case "link":
		if value.Target == "" || value.Mode != 0 || value.SHA256 != "" || !emptyEntries {
			return errors.New("invalid link snapshot")
		}
	case "file":
		if value.Target != "" || value.Mode > 0o7777 || !isSHA256(value.SHA256) || !emptyEntries {
			return errors.New("invalid file snapshot")
		}
	case "directory":
		if value.Target != "" || value.Mode > 0o7777 || value.SHA256 != "" || value.Entries == nil {
			return errors.New("invalid directory snapshot")
		}
		for name, child := range value.Entries {
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				return errors.New("invalid snapshot entry name")
			}
			if err := validateSnapshot(child); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid snapshot kind")
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func snapshot(path string) (Snapshot, error) {
	if err := safeParents(path); err != nil {
		return Snapshot{}, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Snapshot{Kind: "absent"}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	mode := uint32(info.Mode().Perm())
	if info.Mode()&os.ModeSetuid != 0 {
		mode |= 0o4000
	}
	if info.Mode()&os.ModeSetgid != 0 {
		mode |= 0o2000
	}
	if info.Mode()&os.ModeSticky != 0 {
		mode |= 0o1000
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Kind: "link", Target: target}, nil
	case info.Mode().IsRegular():
		links, err := linkCount(info)
		if err != nil {
			return Snapshot{}, err
		}
		if links != 1 {
			return Snapshot{}, fmt.Errorf("hard-linked file not supported: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, err
		}
		sum := sha256.Sum256(data)
		return Snapshot{Kind: "file", Mode: mode, SHA256: hex.EncodeToString(sum[:])}, nil
	case info.IsDir():
		entries, err := os.ReadDir(path)
		if err != nil {
			return Snapshot{}, err
		}
		result := Snapshot{Kind: "directory", Mode: mode, Entries: make(map[string]Snapshot, len(entries))}
		for _, entry := range entries {
			child, err := snapshot(filepath.Join(path, entry.Name()))
			if err != nil {
				return Snapshot{}, err
			}
			result.Entries[entry.Name()] = child
		}
		return result, nil
	default:
		return Snapshot{}, fmt.Errorf("special file not supported: %s", path)
	}
}

func desiredSnapshot(shim map[string][]byte) Snapshot {
	root := Snapshot{Kind: "directory", Mode: 0o755, Entries: map[string]Snapshot{}}
	for name, data := range shim {
		parts := strings.Split(filepath.ToSlash(name), "/")
		insertDesired(&root, parts, data)
	}
	return root
}

func insertDesired(root *Snapshot, parts []string, data []byte) {
	if len(parts) == 1 {
		sum := sha256.Sum256(data)
		root.Entries[parts[0]] = Snapshot{Kind: "file", Mode: 0o444, SHA256: hex.EncodeToString(sum[:])}
		return
	}
	child, ok := root.Entries[parts[0]]
	if !ok {
		child = Snapshot{Kind: "directory", Mode: 0o755, Entries: map[string]Snapshot{}}
	}
	insertDesired(&child, parts[1:], data)
	root.Entries[parts[0]] = child
}

func validateShim(input map[string][]byte) (map[string][]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("shim is empty")
	}
	result := make(map[string][]byte, len(input))
	for name, data := range input {
		clean := filepath.Clean(name)
		if name == "" || filepath.IsAbs(name) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("unsafe shim path: %q", name)
		}
		result[clean] = bytes.Clone(data)
	}
	if _, ok := result["SKILL.md"]; !ok {
		return nil, errors.New("shim must contain SKILL.md")
	}
	return result, nil
}

func writeShimTree(root string, shim map[string][]byte) error {
	if err := os.Mkdir(root, 0o700); err != nil {
		return fmt.Errorf("create managed shim staging directory: %w", err)
	}
	names := make([]string, 0, len(shim))
	for name := range shim {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(shim[name])
		if writeErr == nil {
			writeErr = file.Chmod(0o444)
		}
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return err
		}
	}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(a, b int) bool { return len(dirs[a]) > len(dirs[b]) })
	for _, dir := range dirs {
		if err := os.Chmod(dir, 0o755); err != nil {
			return err
		}
		if err := syncDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONExclusive(path string, value any) error {
	file, err := openExclusiveNoFollow(path, 0o600)
	if err != nil {
		return fmt.Errorf("create receipt: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(value)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

func appendEvent(run string, event any) error {
	path := filepath.Join(run, "events.jsonl")
	file, err := openAppendNoFollow(path, 0o600)
	if err != nil {
		return fmt.Errorf("open event journal: %w", err)
	}
	data, err := json.Marshal(event)
	if err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func targetPaths(home, configHome string) []string {
	return []string{
		filepath.Join(home, ".claude/skills/backscroll"),
		filepath.Join(home, ".agents/skills/backscroll"),
		filepath.Join(configHome, "opencode/skills/backscroll"),
	}
}

func validateLayout(home, configHome, state, managed string, targets []string) error {
	paths := append([]string{managed, state}, targets...)
	for _, path := range paths {
		if err := safeParents(path); err != nil {
			return err
		}
	}
	for left := 0; left < len(targets); left++ {
		for right := left + 1; right < len(targets); right++ {
			if overlaps(targets[left], targets[right]) {
				return fmt.Errorf("overlapping destinations: %s, %s", targets[left], targets[right])
			}
		}
		if overlaps(state, targets[left]) || overlaps(managed, targets[left]) {
			return fmt.Errorf("overlapping state or destination: %s", targets[left])
		}
	}
	if overlaps(home, configHome) && home != configHome && !isWithin(configHome, home) {
		return errors.New("invalid home and config home layout")
	}
	return nil
}

func overlaps(left, right string) bool {
	return left == right || isWithin(left, right) || isWithin(right, left)
}

func isWithin(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func canonicalHome(path string) (string, error) {
	if path == "" {
		return "", errors.New("home is required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("home must be an existing directory")
	}
	return resolved, nil
}

func absolutePath(path string) (string, error) {
	result, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	result = filepath.Clean(result)
	if err := safeParents(result); err != nil {
		return "", err
	}
	return result, nil
}

func safeParents(path string) error {
	parent := filepath.Dir(path)
	for {
		info, err := os.Lstat(parent)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("unsafe ancestor: %s", parent)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return nil
		}
		parent = next
	}
}

func makePrivateDir(path string) error {
	if err := safeParents(path); err != nil {
		return err
	}
	_, statErr := os.Lstat(path)
	created := os.IsNotExist(statErr)
	if statErr != nil && !created {
		return statErr
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if created {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	uid, err := ownerID(info)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 || uid != currentUserID() {
		return fmt.Errorf("state directory must be owned by you and mode 0700: %s", path)
	}
	return nil
}

func sameFilesystem(left, right string) error {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return err
	}
	leftDevice, err := deviceID(leftInfo)
	if err != nil {
		return err
	}
	rightDevice, err := deviceID(rightInfo)
	if err != nil {
		return err
	}
	if leftDevice != rightDevice {
		return errors.New("backup and destination must be on the same filesystem")
	}
	return nil
}

func countChanges(targets []Target) int {
	count := 0
	for _, target := range targets {
		if target.Change {
			count++
		}
	}
	return count
}

func swapPath(target, run string, index int) string {
	return filepath.Join(filepath.Dir(target), fmt.Sprintf(".backscroll-skill-%s-%d", run, index))
}

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for fsync %s: %w", path, err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("fsync directory %s: %w", path, err)
	}
	return nil
}
