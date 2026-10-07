package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

type cancelAfterErrChecksContext struct {
	context.Context
	remaining int
}

func (c *cancelAfterErrChecksContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

type replaySchedulerReader struct {
	name       string
	pathsByID  map[string][]string
	hashes     map[string]string
	hashCalls  map[string]int
	parseCalls map[string]int
}

func (r *replaySchedulerReader) Name() string { return r.name }

func (r *replaySchedulerReader) Discover(_ context.Context, def input_config.InputDefinition) ([]string, error) {
	return append([]string(nil), r.pathsByID[def.ID]...), nil
}

func (r *replaySchedulerReader) Hash(_ context.Context, path string) (string, error) {
	r.hashCalls[path]++
	return r.hashes[path], nil
}

func (r *replaySchedulerReader) Parse(_ context.Context, path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	r.parseCalls[path]++
	return models.ParsedFile{
		Path: path,
		Hash: r.hashes[path],
		Records: []models.Message{{
			Role:        "user",
			Origin:      models.OriginHuman,
			Content:     "scheduler productive fixture",
			UUID:        "scheduler-uuid",
			ContentType: "text",
		}},
	}, nil
}

func mustSelectReplayPaths(t *testing.T, states map[string]*syncPathState, queues replayQueues, limit int) map[string]replayReason {
	t.Helper()
	selected, err := selectReplayPaths(context.Background(), states, queues, limit)
	if err != nil {
		t.Fatalf("select replay paths: %v", err)
	}
	return selected
}

func schedulerState(path, readerName string) *syncPathState {
	reader := &replaySchedulerReader{name: readerName}
	return &syncPathState{
		claim: discoveredPathClaim{
			path:   path,
			def:    input_config.InputDefinition{ID: path, Source: "session"},
			reader: reader,
		},
		existingMeta: storage.FileMetadata{Path: path, Hash: "same"},
		exists:       true,
		hash:         "same",
	}
}

func schedulerPaths(prefix string, count int) []string {
	paths := make([]string, count)
	for i := range paths {
		paths[i] = fmt.Sprintf("/%s-%03d", prefix, i)
	}
	return paths
}

func schedulerStates(groups ...[]string) map[string]*syncPathState {
	states := make(map[string]*syncPathState)
	for _, paths := range groups {
		for _, path := range paths {
			states[path] = schedulerState(path, "claude")
		}
	}
	return states
}

func TestDeduplicatePathClaimsCancellationInterruptsSort(t *testing.T) {
	reader := &replaySchedulerReader{name: "claude"}
	claims := make([]discoveredPathClaim, 128)
	for i := range claims {
		claims[i] = discoveredPathClaim{
			path:   "/same",
			reader: reader,
			def: input_config.InputDefinition{
				ID:     fmt.Sprintf("input-%03d", len(claims)-i),
				Source: "session",
			},
		}
	}
	ctx := &cancelAfterErrChecksContext{Context: context.Background(), remaining: 150}
	unique, err := deduplicatePathClaims(ctx, claims)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context.Canceled", err)
	}
	if unique != nil {
		t.Fatalf("canceled dedup returned partial claims: %#v", unique)
	}
}

func TestSelectReplayPathsCancellationStopsMidQueue(t *testing.T) {
	paths := schedulerPaths("cancel", 100)
	ctx := &cancelAfterErrChecksContext{Context: context.Background(), remaining: 12}
	selected, err := selectReplayPaths(ctx, schedulerStates(paths), replayQueues{origin: paths}, 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context.Canceled", err)
	}
	if selected != nil {
		t.Fatalf("canceled replay selection returned partial result: %v", selected)
	}
}

func TestReplaySchedulerCombinedCapAndStaleCannotBypass(t *testing.T) {
	origin := schedulerPaths("origin", 100)
	echo := schedulerPaths("echo", 100)
	stale := schedulerPaths("stale", 20)
	states := schedulerStates(origin, echo, stale)

	selected := mustSelectReplayPaths(t, states, replayQueues{origin: origin, echo: echo, stale: stale}, 200)
	if len(selected) != 200 {
		t.Fatalf("selected %d immutable replays, want exact global cap 200", len(selected))
	}
	for _, path := range stale {
		if selected[path] != 0 {
			t.Fatalf("stale path %s bypassed the combined origin+echo cap", path)
		}
	}

	// A provenance path beyond the cap must not re-enter through staleSet.
	outside := "/origin-outside-cap"
	originWithTail := append(append([]string(nil), schedulerPaths("full", 200)...), outside)
	states = schedulerStates(originWithTail)
	selected = mustSelectReplayPaths(t, states, replayQueues{origin: originWithTail, stale: []string{outside}}, 200)
	if selected[outside] != 0 {
		t.Fatalf("provenance tail re-entered through stale queue: reason=%v", selected[outside])
	}
}

func TestReplaySchedulerOverlapChargesOnce(t *testing.T) {
	const path = "/all-reasons.jsonl"
	states := schedulerStates([]string{path})
	selected := mustSelectReplayPaths(t, states, replayQueues{
		origin: []string{path},
		echo:   []string{path},
		stale:  []string{path},
	}, 1)
	if len(selected) != 1 {
		t.Fatalf("overlap selected %d paths, want one", len(selected))
	}
	want := replayOrigin | replayEcho | replayStale
	if selected[path] != want {
		t.Fatalf("overlap reason = %v, want %v", selected[path], want)
	}
}

func TestReplaySchedulerSelectionIgnoresDiscoveryOrder(t *testing.T) {
	states := make(map[string]*syncPathState)
	for _, path := range []string{"/c", "/b", "/a"} { // inverse discovery insertion
		states[path] = schedulerState(path, "claude")
	}
	selected := mustSelectReplayPaths(t, states, replayQueues{origin: []string{"/a", "/b", "/c"}}, 2)
	if selected["/a"] == 0 || selected["/b"] == 0 || selected["/c"] != 0 {
		t.Fatalf("selection followed discovery order: %v", selected)
	}
}

func TestReplaySchedulerUndiscoveredAndModifiedDoNotConsume(t *testing.T) {
	modified := schedulerState("/modified", "claude")
	modified.hash = "changed"
	modified.naturalParse = true
	states := map[string]*syncPathState{
		"/modified":  modified,
		"/immutable": schedulerState("/immutable", "claude"),
	}
	selected := mustSelectReplayPaths(t, states, replayQueues{origin: []string{
		"/undiscovered", "/modified", "/immutable",
	}}, 1)
	if len(selected) != 1 || selected["/immutable"] == 0 {
		t.Fatalf("undiscovered or modified work consumed cap: %v", selected)
	}
	if selected["/modified"] != 0 {
		t.Fatalf("modified natural parse was charged as replay: %v", selected)
	}
}

func TestReplaySchedulerDrains200Then1Then0(t *testing.T) {
	remaining := schedulerPaths("drain", 201)
	states := schedulerStates(remaining)
	for run, want := range []int{200, 1, 0} {
		selected := mustSelectReplayPaths(t, states, replayQueues{origin: remaining}, 200)
		if len(selected) != want {
			t.Fatalf("run %d selected %d, want %d", run+1, len(selected), want)
		}
		next := remaining[:0]
		for _, path := range remaining {
			if selected[path] == 0 {
				next = append(next, path)
			}
		}
		remaining = next
	}
}

func TestReplaySchedulerEmptyPiOverlapDoesNotDuplicate(t *testing.T) {
	const path = "/empty-pi.jsonl"
	states := map[string]*syncPathState{path: schedulerState(path, "pi")}
	selected := mustSelectReplayPaths(t, states, replayQueues{
		origin:  []string{path},
		echo:    []string{path},
		stale:   []string{path},
		emptyPi: []string{path},
	}, 200)
	if len(selected) != 1 {
		t.Fatalf("empty Pi overlap selected %d parses, want one", len(selected))
	}
	want := replayOrigin | replayEcho | replayStale | replayEmptyPi
	if selected[path] != want {
		t.Fatalf("empty Pi overlap reason = %v, want %v", selected[path], want)
	}

	states[path].existingMeta.Hash = emptyPiHashPrefix + "same"
	states[path].hash = "same" // racy-clean hashing returns the unmarked content hash
	if got := mustSelectReplayPaths(t, states, replayQueues{emptyPi: []string{path}}, 200); len(got) != 0 {
		t.Fatalf("marked empty Pi path replayed again after forced hash: %v", got)
	}
}

func TestCanonicalContentHashIsScopedToPi(t *testing.T) {
	marked := emptyPiHashPrefix + "same"
	if !contentHashesEqual("pi", marked, "same") {
		t.Fatal("Pi marker changed canonical content identity")
	}
	if contentHashesEqual("claude", marked, "same") {
		t.Fatal("Pi marker normalization leaked into another reader")
	}
}

func TestReplaySchedulerEquivalentAndIncompatibleClaims(t *testing.T) {
	reader := &replaySchedulerReader{name: "claude"}
	equivalent := []discoveredPathClaim{
		{path: "/same", reader: reader, def: input_config.InputDefinition{ID: "z", Source: "session", Decode: input_config.DecodeConfig{Format: "claude"}}},
		{path: "/same", reader: reader, def: input_config.InputDefinition{ID: "a", Source: "session"}},
	}
	unique, err := deduplicatePathClaims(context.Background(), equivalent)
	if err != nil || len(unique) != 1 || unique[0].def.ID != "a" {
		t.Fatalf("equivalent claims = %#v, err=%v; want canonical input a", unique, err)
	}

	incompatible := append([]discoveredPathClaim(nil), equivalent...)
	incompatible[0].def.Decode.IndexReasoning = true
	_, firstErr := deduplicatePathClaims(context.Background(), incompatible)
	for left, right := 0, len(incompatible)-1; left < right; left, right = left+1, right-1 {
		incompatible[left], incompatible[right] = incompatible[right], incompatible[left]
	}
	_, secondErr := deduplicatePathClaims(context.Background(), incompatible)
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatalf("incompatible error is not deterministic: first=%v second=%v", firstErr, secondErr)
	}
	if !strings.Contains(firstErr.Error(), `path "/same" is claimed by incompatible inputs`) ||
		!strings.Contains(firstErr.Error(), `"a"`) || !strings.Contains(firstErr.Error(), `"z"`) {
		t.Fatalf("incompatible error lacks actionable claims: %v", firstErr)
	}
}

func TestReplaySchedulerProductiveDuplicatePathHashesAndParsesOnce(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "duplicate.jsonl")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	reader := &replaySchedulerReader{
		name:       "claude",
		pathsByID:  map[string][]string{"z-input": {path, path}, "a-input": {path}},
		hashes:     map[string]string{path: "hash-1"},
		hashCalls:  make(map[string]int),
		parseCalls: make(map[string]int),
	}
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{
			{ID: "z-input", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: "claude"}},
			{ID: "a-input", Source: "session", Active: true},
		}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	cfg := config.Config{DatabasePath: filepath.Join(tmp, "index.db")}
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("productive sync: %v", err)
	}
	if reader.hashCalls[path] != 1 || reader.parseCalls[path] != 1 {
		t.Fatalf("duplicate path calls = hash %d parse %d, want 1/1", reader.hashCalls[path], reader.parseCalls[path])
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var gotPaths, gotRows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path = ?`, path).Scan(&gotPaths); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ?`, path).Scan(&gotRows); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]int{gotPaths, gotRows}, []int{1, 1}) {
		t.Fatalf("productive persisted counts = paths %d rows %d, want 1/1", gotPaths, gotRows)
	}
}
