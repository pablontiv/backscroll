package main

import (
	"fmt"
	"io"

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

	// Get existing file hashes
	existingHashes, err := db.GetFileHashes()
	if err != nil {
		return fmt.Errorf("get file hashes: %w", err)
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
			hash, err := reader.Hash(ref)
			if err != nil {
				return fmt.Errorf("hash %s: %w", ref, err)
			}

			// Skip unchanged files UNLESS they are in the stale-set and cap allows.
			// Stale files need re-parsing for extraction_version backfill (perennity).
			if existingHashes[ref] == hash {
				if !staleSet[ref] || staleParsesDone >= staleParsesCap {
					continue
				}
				staleParsesDone++
				_, _ = fmt.Fprintf(progress, "Re-parsing stale file %d/%d: %s\n", staleParsesDone, len(stalePaths), ref)
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

			var sessionText string
			var indexedMsgs []storage.IndexedMessage
			for ordinal, msg := range pf.Records {
				sessionText += msg.Content + "\n"
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

			sessionTags := tagging.Tag(sessionText)

			indexedFiles = append(indexedFiles, storage.IndexedFile{
				SourcePath: ref,
				Source:     def.Source,
				Hash:       pf.Hash,
				Project:    ident.ProjectID,
				Messages:   indexedMsgs,
				Tags:       sessionTags,
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
