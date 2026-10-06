package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/backscroll/internal/tagging"
	"github.com/pablontiv/backscroll/internal/templates"
)

// diagnosticsEnabled checks if startup diagnostics are enabled via env var
func diagnosticsEnabled() bool {
	return os.Getenv("BACKSCROLL_STARTUP_DIAGNOSTICS") == "1"
}

const emptyPiHashPrefix = "pi-empty-v1:"

// canonicalContentHash removes persisted parser-state markers when comparing
// content identity. The marker is Pi-specific and remains intact in storage.
func canonicalContentHash(readerName, hash string) string {
	if readerName == "pi" {
		return strings.TrimPrefix(hash, emptyPiHashPrefix)
	}
	return hash
}

func contentHashesEqual(readerName, persistedHash, observedHash string) bool {
	return canonicalContentHash(readerName, persistedHash) == canonicalContentHash(readerName, observedHash)
}

var (
	maybeAutoSyncOpen               = storage.Open
	maybeAutoSyncActiveInputs       = input_config.ActiveInputs
	maybeAutoSyncLoadGlobalRegistry = projects.LoadGlobalRegistry
	maybeAutoSyncNewRegistry        = newDefaultAutoSyncRegistry
	maybeAutoSyncSyncFiles          = func(db *storage.Database, files []storage.IndexedFile) error { return db.SyncFiles(files) }
	maybeAutoSyncGetFileMetadata    = getFileMetadata // for testability
)

// startupPhaseTiming holds measurements for startup phases that occur before maybeAutoSync.
// Populated by coordinateStartup if diagnosticsEnabled() is true.
type startupPhaseTiming struct {
	LockAcquisitionTime time.Duration
	IndexPrepareTime    time.Duration
}

// startupDiags holds pre-sync phase timings, set by coordinateStartup.
// Access must be guarded by checking diagnosticsEnabled() first.
var startupDiags *startupPhaseTiming

func newDefaultAutoSyncRegistry() *readers.Registry {
	reg := readers.NewRegistry()
	reg.Register(&readers.OpenCodeReader{})
	reg.Register(&readers.ClaudeReader{})
	reg.Register(&readers.PiReader{})
	reg.Register(&readers.CodexReader{})
	reg.Register(&readers.MarkdownDocumentReader{})
	reg.Register(&readers.MarkdownSectionsReader{})
	return reg
}

func usesFileMetadataPrefilter(reader readers.SessionReader) bool {
	// SQLite WAL commits may not change OpenCode's main database file.
	return reader.Name() != "opencode"
}

// getFileMetadata returns the size and mtime of a file in RFC3339 format.
// Returns (size, mtime, error). On error, all values are nil/empty.
// This is used by the v14 metadata prefilter to skip hashing unchanged files.
func getFileMetadata(path string) (*int64, *string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat: %w", err)
	}

	size := stat.Size()
	mtime := stat.ModTime().Format(time.RFC3339)

	return &size, &mtime, nil
}

// The replay scheduler uses one discovered claim and one classification per
// unique source path, independent of input and discovery ordering.
type discoveredPathClaim struct {
	path   string
	def    input_config.InputDefinition
	reader readers.SessionReader
}

type syncInputSemantics struct {
	source         string
	format         string
	indexReasoning bool
}

func semanticsForClaim(claim discoveredPathClaim) syncInputSemantics {
	return syncInputSemantics{
		source:         claim.def.Source,
		format:         claim.reader.Name(),
		indexReasoning: claim.def.Decode.IndexReasoning,
	}
}

func claimDescription(claim discoveredPathClaim) string {
	semantics := semanticsForClaim(claim)
	return fmt.Sprintf("%q (source=%q, format=%q, index_reasoning=%t)",
		claim.def.ID, semantics.source, semantics.format, semantics.indexReasoning)
}

// deduplicatePathClaims establishes one canonical parser contract per path.
// Discovery settings and input IDs do not affect parsing, so equivalent inputs
// may overlap. Source, effective reader format, and parser options must agree.
func deduplicatePathClaims(claims []discoveredPathClaim) ([]discoveredPathClaim, error) {
	byPath := make(map[string][]discoveredPathClaim)
	for _, claim := range claims {
		byPath[claim.path] = append(byPath[claim.path], claim)
	}

	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	unique := make([]discoveredPathClaim, 0, len(paths))
	for _, path := range paths {
		pathClaims := byPath[path]
		sort.Slice(pathClaims, func(i, j int) bool {
			return claimDescription(pathClaims[i]) < claimDescription(pathClaims[j])
		})
		want := semanticsForClaim(pathClaims[0])
		for _, claim := range pathClaims[1:] {
			if semanticsForClaim(claim) != want {
				descriptions := make([]string, 0, len(pathClaims))
				for _, conflicting := range pathClaims {
					descriptions = append(descriptions, claimDescription(conflicting))
				}
				return nil, fmt.Errorf("path %q is claimed by incompatible inputs: %s", path, strings.Join(descriptions, ", "))
			}
		}
		unique = append(unique, pathClaims[0])
	}
	return unique, nil
}

type replayReason uint8

const (
	replayOrigin replayReason = 1 << iota
	replayEcho
	replayStale
	replayEmptyPi
)

type replayQueues struct {
	origin  []string
	echo    []string
	stale   []string
	emptyPi []string
}

type syncPathState struct {
	claim        discoveredPathClaim
	existingMeta storage.FileMetadata
	exists       bool
	hash         string
	naturalParse bool
}

// selectReplayPaths combines all parser-backed maintenance queues. Queue order
// is durable (origin, echo, pure stale, empty Pi), while duplicate paths retain
// all reasons and consume at most one slot. Only discovered, hash-immutable
// paths spend the replay budget; natural parses perform the same maintenance
// for free.
func selectReplayPaths(states map[string]*syncPathState, queues replayQueues, limit int) map[string]replayReason {
	reasons := make(map[string]replayReason)
	ordered := make([]string, 0)
	addQueue := func(paths []string, reason replayReason) {
		for _, path := range paths {
			if reasons[path] == 0 {
				ordered = append(ordered, path)
			}
			reasons[path] |= reason
		}
	}
	addQueue(queues.origin, replayOrigin)
	addQueue(queues.echo, replayEcho)
	addQueue(queues.stale, replayStale)
	addQueue(queues.emptyPi, replayEmptyPi)

	selected := make(map[string]replayReason)
	for _, path := range ordered {
		if len(selected) >= limit {
			break
		}
		state := states[path]
		if state == nil || !state.exists || state.naturalParse ||
			!contentHashesEqual(state.claim.reader.Name(), state.existingMeta.Hash, state.hash) {
			continue
		}
		reason := reasons[path]
		if reason == replayEmptyPi && (state.claim.reader.Name() != "pi" || strings.HasPrefix(state.existingMeta.Hash, emptyPiHashPrefix)) {
			continue
		}
		selected[path] = reason
	}
	return selected
}

// isRacyCleanFile reports whether a file's mtime suggests it could be racy clean.
// A file is racy clean if its mtime is not strictly older than the recorded
// last_indexed time (within a 2-second granularity margin). Such files could have
// been edited in the same timestamp tick as the indexing, leaving size and mtime
// unchanged but content different. See git's racy-git documentation.
func isRacyCleanFile(fileMtime string, lastIndexed string) bool {
	// Parse fileMtime as RFC3339 (written by Go in sync_helpers.go:47)
	fileMt, err := time.Parse(time.RFC3339, fileMtime)
	if err != nil {
		return true // On parse error, assume racy (conservative)
	}

	// Parse lastIndexed. The indexed_files.last_indexed column is DATETIME type,
	// declared as "DEFAULT CURRENT_TIMESTAMP". SQLite stores it as "2006-01-02 15:04:05"
	// in raw text, but the modernc.org/sqlite driver converts DATETIME columns on read,
	// so Go receives RFC3339 format "2026-05-15T15:59:25Z". Inspecting with the sqlite3
	// CLI shows raw format and misleadingly suggests a SQLite-layout parse is required.
	// Verify through the driver (Go), not the CLI. Accept both formats for robustness.
	var indexTime time.Time

	const sqliteTimestampLayout = "2006-01-02 15:04:05"
	if indexTime, err = time.ParseInLocation(sqliteTimestampLayout, lastIndexed, time.UTC); err != nil {
		// Fall back to RFC3339 (actual driver behavior)
		if indexTime, err = time.Parse(time.RFC3339, lastIndexed); err != nil {
			return true // On parse error, assume racy (conservative)
		}
	}

	// File is racy if its mtime is not strictly older than last_indexed.
	// Allow 2 seconds margin to account for filesystem granularity (1-2 second typical).
	// Both times are now proper time.Time values; .After() compares instants correctly
	// across any timezone differences.
	const racyMarginSeconds = 2
	return fileMt.After(indexTime.Add(-time.Duration(racyMarginSeconds) * time.Second))
}

// maybeAutoSync performs an incremental sync operation if the database exists.
// It is intended to be called before query commands to ensure fresh index state.
// If sync fails, it returns an error (caller decides whether to warn/ignore).
func maybeAutoSync(cfg *config.Config, progress io.Writer) (retErr error) {
	diag := diagnosticsEnabled()
	var startTime time.Time
	if diag {
		startTime = time.Now()
	}

	// Open database for reading to check if it exists
	// (this will auto-create if missing)
	db, err := maybeAutoSyncOpen(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	// Get existing file metadata for prefiltering
	existingMetadata, err := db.GetFileMetadata()
	if err != nil {
		return fmt.Errorf("get file metadata: %w", err)
	}

	// Load every durable parser-backed maintenance queue. Selection happens only
	// after natural new/modified work has been classified.
	stalePaths, err := db.StalePaths(storage.CurrentExtractionVersion)
	if err != nil {
		return fmt.Errorf("discover stale paths: %w", err)
	}
	emptyPaths, err := db.EmptyIndexedPaths()
	if err != nil {
		return fmt.Errorf("discover empty indexed paths: %w", err)
	}
	echoPaths, err := db.PendingSearchEchoPaths()
	if err != nil {
		return fmt.Errorf("discover pending search echo paths: %w", err)
	}
	originPaths, err := db.PendingOriginPaths(storage.CurrentOriginVersion, len(existingMetadata))
	if err != nil {
		return fmt.Errorf("discover pending message origins: %w", err)
	}
	const replayParsesCap = 200

	// Build reader registry
	reg := maybeAutoSyncNewRegistry()

	// Resolve active inputs
	defs, _, err := maybeAutoSyncActiveInputs(cfg.SessionDirs)
	if err != nil {
		return fmt.Errorf("resolve inputs: %w", err)
	}

	// Load project registry
	registry := maybeAutoSyncLoadGlobalRegistry()

	// Track diagnostics
	var discoveryTime, metadataTime, hashingTime, parsingTime time.Duration
	var bytesHashed int64
	var filesHashed, filesSkipped int

	// Discover first, then establish a single parser contract for every path.
	// This makes hashing, parsing, and replay selection global rather than input-local.
	var claims []discoveredPathClaim
	for _, def := range defs {
		if def.Source == "" {
			def.Source = "session"
		}
		reader, err := reg.ForDef(def)
		if err != nil {
			return fmt.Errorf("resolve reader for input %q: %w", def.ID, err)
		}

		var discoveryStart time.Time
		if diag {
			discoveryStart = time.Now()
		}
		refs, err := reader.Discover(def)
		if err != nil {
			return fmt.Errorf("discover input %q: %w", def.ID, err)
		}
		if diag {
			discoveryTime += time.Since(discoveryStart)
		}
		for _, ref := range refs {
			claims = append(claims, discoveredPathClaim{path: ref, def: def, reader: reader})
		}
	}
	uniqueClaims, err := deduplicatePathClaims(claims)
	if err != nil {
		return err
	}

	// Classify every unique path before choosing replay work. New and modified
	// files are natural parses and therefore never spend the maintenance cap.
	states := make(map[string]*syncPathState, len(uniqueClaims))
	for _, claim := range uniqueClaims {
		ref, reader := claim.path, claim.reader
		existingMeta, exists := existingMetadata[ref]
		state := &syncPathState{claim: claim, existingMeta: existingMeta, exists: exists}

		var metadataStart time.Time
		if diag {
			metadataStart = time.Now()
		}
		if usesFileMetadataPrefilter(reader) && exists && existingMeta.Size != nil && existingMeta.Mtime != nil &&
			existingMeta.LastIndexed != nil {
			if fileSize, fileMtime, metadataErr := maybeAutoSyncGetFileMetadata(ref); metadataErr == nil &&
				fileSize != nil && fileMtime != nil && *fileSize == *existingMeta.Size && *fileMtime == *existingMeta.Mtime &&
				!isRacyCleanFile(*fileMtime, *existingMeta.LastIndexed) {
				state.hash = existingMeta.Hash
			}
		}
		if diag {
			metadataTime += time.Since(metadataStart)
		}

		var hashingStart time.Time
		if diag {
			hashingStart = time.Now()
		}
		if state.hash == "" {
			state.hash, err = reader.Hash(ref)
			if err != nil {
				return fmt.Errorf("hash %s: %w", ref, err)
			}
			if diag {
				filesHashed++
				if fileSize, _, metadataErr := maybeAutoSyncGetFileMetadata(ref); metadataErr == nil && fileSize != nil {
					bytesHashed += *fileSize
				}
			}
		} else if diag {
			filesSkipped++
		}
		if diag {
			hashingTime += time.Since(hashingStart)
		}

		state.naturalParse = !exists || !contentHashesEqual(reader.Name(), existingMeta.Hash, state.hash)
		states[ref] = state
	}

	replaySet := selectReplayPaths(states, replayQueues{
		origin:  originPaths,
		echo:    echoPaths,
		stale:   stalePaths,
		emptyPi: emptyPaths,
	}, replayParsesCap)

	// Collect indexed files. uniqueClaims is path-sorted, so natural work and
	// selected replays are deterministic even when discovery order changes.
	var indexedFiles []storage.IndexedFile
	staleReplaysDone, emptyPiReplaysDone := 0, 0
	for _, claim := range uniqueClaims {
		ref, def, reader := claim.path, claim.def, claim.reader
		state := states[ref]
		reason, replaySelected := replaySet[ref]
		if !state.naturalParse && !replaySelected {
			continue
		}
		if replaySelected {
			if reason == replayEmptyPi {
				emptyPiReplaysDone++
				_, _ = fmt.Fprintf(progress, "Re-parsing empty Pi file %d: %s\n", emptyPiReplaysDone, ref)
			} else {
				staleReplaysDone++
				_, _ = fmt.Fprintf(progress, "Re-parsing stale file %d/%d: %s\n", staleReplaysDone, replayParsesCap, ref)
			}
		}

		var parsingStart time.Time
		if diag {
			parsingStart = time.Now()
		}
		pf, err := reader.Parse(ref, def)
		if err != nil {
			return fmt.Errorf("parse %s: %w", ref, err)
		}
		if diag {
			parsingTime += time.Since(parsingStart)
		}

		// Use session cwd for project identification; fall back to file path if cwd is empty
		identPath := pf.Cwd
		if identPath == "" {
			identPath = ref
		}
		ident := projects.Identify(identPath, registry)

		var sessionTags tagging.Accumulator
		var indexedMsgs []storage.IndexedMessage
		for ordinal, msg := range pf.Records {
			sessionTags.Add(msg.Content)
			indexedMsgs = append(indexedMsgs, storage.IndexedMessage{
				Ordinal:           ordinal,
				Role:              msg.Role,
				Origin:            msg.Origin,
				Text:              msg.Content,
				UUID:              msg.UUID,
				Timestamp:         msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
				ContentType:       msg.ContentType,
				ToolName:          msg.ToolName,
				CommandHead:       msg.CommandHead,
				IsError:           msg.IsError,
				WasInterrupted:    msg.WasInterrupted,
				ExitCode:          msg.ExitCode,
				SearchEcho:        msg.SearchEcho,
				ExtractionVersion: storage.CurrentExtractionVersion,
			})
		}

		fileSize, fileMtime, _ := maybeAutoSyncGetFileMetadata(ref)
		indexedHash := pf.Hash
		if reader.Name() == "pi" && len(pf.Records) == 0 {
			// Mark a zero-row parse with the Pi parser epoch. Old unmarked hashes
			// replay once. Supported-empty files then converge instead of replaying
			// on every startup. Bump the prefix when zero-row Pi semantics change.
			indexedHash = emptyPiHashPrefix + pf.Hash
		}

		indexedFiles = append(indexedFiles, storage.IndexedFile{
			SourcePath: ref,
			Source:     def.Source,
			Hash:       indexedHash,
			Project:    ident.ProjectID,
			Messages:   indexedMsgs,
			Tags:       sessionTags.Tags(),
			FileSize:   fileSize,
			FileMtime:  fileMtime,
		})
	}

	// Phase 5: Database
	var databaseStart time.Time
	if diag {
		databaseStart = time.Now()
	}

	// Sync all files
	if len(indexedFiles) > 0 {
		if err := maybeAutoSyncSyncFiles(db, indexedFiles); err != nil {
			return fmt.Errorf("sync files: %w", err)
		}
	}

	// Q3: Auto-upgrade stale templates from v1 to v2 after sync completes
	// Run in separate transaction for crash-safety
	miner := templates.NewMiner()
	const staleTemplateCap = 200
	const currentNormalizationVersion = 2
	staleTemplatePaths, err := db.StaleTemplatePaths(currentNormalizationVersion)
	if err != nil {
		return fmt.Errorf("discover stale templates: %w", err)
	} else if len(staleTemplatePaths) > 0 {
		// Cap at staleTemplateCap per run; FIFO processing across runs
		if len(staleTemplatePaths) > staleTemplateCap {
			staleTemplatePaths = staleTemplatePaths[:staleTemplateCap]
		}

		for _, sourcePath := range staleTemplatePaths {
			msgs, err := db.LoadMessagesForPath(sourcePath)
			if err != nil {
				return fmt.Errorf("load messages for stale template path %s: %w", sourcePath, err)
			}

			deletedCount, err := db.BackfillTemplatesForFile(miner, sourcePath, msgs)
			if err != nil {
				return fmt.Errorf("re-mine templates for %s: %w", sourcePath, err)
			}
			if deletedCount > 0 {
				_, _ = fmt.Fprintf(progress, "Deleted %d stuck templates from %s\n", deletedCount, sourcePath)
			}
		}
	}

	// Re-derive correction signals recorded under a superseded detector epoch. This
	// is the only route for a session whose JSONL has expired while its indexed_files
	// row remains: SyncFiles skips it (not on disk) and BackfillDerived skips it (not
	// absent from indexed_files), so without this a detector fix never reaches it.
	// Bounded per run and convergent — see RederiveSupersededCorrections.
	if _, err := db.RederiveSupersededCorrections(staleTemplateCap); err != nil {
		return fmt.Errorf("re-derive superseded correction signals: %w", err)
	}

	if diag {
		databaseTime := time.Since(databaseStart)
		totalTime := time.Since(startTime)

		// Report diagnostics
		_, _ = fmt.Fprintf(progress, "\nStartup diagnostics:\n")
		if startupDiags != nil {
			_, _ = fmt.Fprintf(progress, "  Lock Acquisition:%v\n", startupDiags.LockAcquisitionTime)
			_, _ = fmt.Fprintf(progress, "  Index Prepare:   %v\n", startupDiags.IndexPrepareTime)
		}
		_, _ = fmt.Fprintf(progress, "  Discovery:       %v\n", discoveryTime)
		_, _ = fmt.Fprintf(progress, "  Metadata:        %v (%d files checked)\n", metadataTime, filesHashed+filesSkipped)
		_, _ = fmt.Fprintf(progress, "  Hashing:         %v (%d files hashed, %d files skipped, %.1f MB)\n",
			hashingTime, filesHashed, filesSkipped, float64(bytesHashed)/(1024*1024))
		_, _ = fmt.Fprintf(progress, "  Parsing:         %v\n", parsingTime)
		_, _ = fmt.Fprintf(progress, "  Database:        %v\n", databaseTime)

		// Calculate unattributed time
		measuredTime := discoveryTime + metadataTime + hashingTime + parsingTime + databaseTime
		if startupDiags != nil {
			measuredTime += startupDiags.LockAcquisitionTime + startupDiags.IndexPrepareTime
		}
		unattributedTime := totalTime - measuredTime
		if unattributedTime > 0 {
			_, _ = fmt.Fprintf(progress, "  Unattributed:    %v (I/O, config load, other OS overhead; page-cache sensitive)\n", unattributedTime)
		}

		_, _ = fmt.Fprintf(progress, "  Total:           %v\n", totalTime)
	}

	return nil
}
