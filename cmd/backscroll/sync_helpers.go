package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/backscroll/internal/tagging"
	"github.com/pablontiv/backscroll/internal/templates"
)

var (
	maybeAutoSyncOpen               = storage.Open
	maybeAutoSyncActiveInputs       = input_config.ActiveInputs
	maybeAutoSyncLoadGlobalRegistry = projects.LoadGlobalRegistry
	maybeAutoSyncNewRegistry        = newDefaultAutoSyncRegistry
	maybeAutoSyncSyncFiles          = func(db *storage.Database, files []storage.IndexedFile) error { return db.SyncFiles(files) }
	maybeAutoSyncGetFileMetadata    = getFileMetadata // for testability
)

func newDefaultAutoSyncRegistry() *readers.Registry {
	reg := readers.NewRegistry()
	reg.Register(&readers.OpenCodeReader{})
	reg.Register(&readers.ClaudeReader{})
	reg.Register(&readers.PiReader{})
	reg.Register(&readers.MarkdownDocumentReader{})
	reg.Register(&readers.MarkdownSectionsReader{})
	return reg
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

	// Parse lastIndexed. Try both possible formats:
	// - SQLite CURRENT_TIMESTAMP: "2026-05-15 15:59:25" (space-separated, UTC)
	// - RFC3339: "2026-05-15T15:59:25Z" (also UTC)
	var indexTime time.Time

	const sqliteTimestampLayout = "2006-01-02 15:04:05"
	if indexTime, err = time.ParseInLocation(sqliteTimestampLayout, lastIndexed, time.UTC); err != nil {
		// Fall back to RFC3339
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

	// Build stale-set once per run (files needing re-parse for rich metadata backfill)
	stalePaths, err := db.StalePaths(storage.CurrentExtractionVersion)
	if err != nil {
		return fmt.Errorf("discover stale paths: %w", err)
	}
	staleSet := make(map[string]bool)
	for _, p := range stalePaths {
		staleSet[p] = true
	}

	const staleParsesCap = 200
	staleParsesDone := 0

	// Build reader registry
	reg := maybeAutoSyncNewRegistry()

	// Resolve active inputs
	defs, _, err := maybeAutoSyncActiveInputs(cfg.SessionDirs)
	if err != nil {
		return fmt.Errorf("resolve inputs: %w", err)
	}

	// Load project registry
	registry := maybeAutoSyncLoadGlobalRegistry()

	// Collect indexed files
	var indexedFiles []storage.IndexedFile

	// Process sessions via reader registry
	for _, def := range defs {
		if def.Source == "" {
			def.Source = "session"
		}

		reader, err := reg.ForDef(def)
		if err != nil {
			return fmt.Errorf("resolve reader for input %q: %w", def.ID, err)
		}

		refs, err := reader.Discover(def)
		if err != nil {
			return fmt.Errorf("discover input %q: %w", def.ID, err)
		}

		for _, ref := range refs {
			// v14 metadata prefilter with racy-clean guard:
			// Files modified within the same timestamp tick as indexing could have matching
			// size+mtime but different content. Git calls these "racy clean" files.
			// We use last_indexed as the reference point: if file.mtime >= last_indexed,
			// do not trust the metadata (could be racy). Otherwise, skip hashing if both match.
			var hash string
			var shouldParse bool

			existingMeta, exists := existingMetadata[ref]
			if exists && existingMeta.Size != nil && existingMeta.Mtime != nil &&
				existingMeta.LastIndexed != nil {
				// All metadata fields must be non-NULL to use the prefilter
				// Check if current file matches recorded metadata
				if fileSize, fileMtime, err := maybeAutoSyncGetFileMetadata(ref); err == nil {
					if fileSize != nil && fileMtime != nil &&
						*fileSize == *existingMeta.Size &&
						*fileMtime == *existingMeta.Mtime {
						// Metadata matches; check if file is racy-clean
						// (mtime within ~2s of last_indexed means could be in same tick)
						isRacyClean := isRacyCleanFile(*fileMtime, *existingMeta.LastIndexed)
						if !isRacyClean {
							// File metadata unchanged and not racy: use cached hash, skip re-hashing
							hash = existingMeta.Hash
							shouldParse = staleSet[ref] && staleParsesDone < staleParsesCap
						}
						// else: racy-clean file falls through to hash anyway
					}
				}
			}

			// If prefilter didn't match or wasn't available, compute the hash
			if hash == "" {
				var err error
				hash, err = reader.Hash(ref)
				if err != nil {
					return fmt.Errorf("hash %s: %w", ref, err)
				}
				shouldParse = true
			}

			// Skip unchanged files UNLESS they are in the stale-set and cap allows
			if shouldParse || (exists && existingMeta.Hash != hash) {
				if !shouldParse && staleSet[ref] && staleParsesDone < staleParsesCap {
					staleParsesDone++
					_, _ = fmt.Fprintf(progress, "Re-parsing stale file %d/%d: %s\n", staleParsesDone, len(stalePaths), ref)
				}
				// Will parse below
			} else {
				// File unchanged and not stale: skip
				continue
			}

			pf, err := reader.Parse(ref, def)
			if err != nil {
				return fmt.Errorf("parse %s: %w", ref, err)
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
					Text:              msg.Content,
					UUID:              msg.UUID,
					Timestamp:         msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
					ContentType:       msg.ContentType,
					ToolName:          msg.ToolName,
					CommandHead:       msg.CommandHead,
					IsError:           msg.IsError,
					WasInterrupted:    msg.WasInterrupted,
					ExitCode:          msg.ExitCode,
					ExtractionVersion: storage.CurrentExtractionVersion,
				})
			}

			// Get file metadata for v14 prefilter
			fileSize, fileMtime, _ := maybeAutoSyncGetFileMetadata(ref)

			indexedFiles = append(indexedFiles, storage.IndexedFile{
				SourcePath: ref,
				Source:     def.Source,
				Hash:       pf.Hash,
				Project:    ident.ProjectID,
				Messages:   indexedMsgs,
				Tags:       sessionTags.Tags(),
				FileSize:   fileSize,
				FileMtime:  fileMtime,
			})
		}
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

	return nil
}
