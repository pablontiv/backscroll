package main

import (
	"fmt"
	"os"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/backscroll/internal/tagging"
	"github.com/pablontiv/backscroll/internal/templates"
)

// maybeAutoSync performs an incremental sync operation if the database exists.
// It is intended to be called before query commands to ensure fresh index state.
// If sync fails, it returns an error (caller decides whether to warn/ignore).
func maybeAutoSync(cfg *config.Config) error {
	// Open database for reading to check if it exists
	// (this will auto-create if missing)
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Get existing file hashes
	existingHashes, err := db.GetFileHashes()
	if err != nil {
		return fmt.Errorf("get file hashes: %w", err)
	}

	// Build stale-set once per run (files needing re-parse for rich metadata backfill)
	stalePaths, err := db.StalePaths(storage.CurrentExtractionVersion)
	if err != nil {
		// Warn but continue if stale query fails
		fmt.Fprintf(os.Stderr, "warning: stale paths query failed: %v\n", err)
		stalePaths = nil
	}
	staleSet := make(map[string]bool)
	for _, p := range stalePaths {
		staleSet[p] = true
	}

	const staleParsesCap = 200
	staleParsesDone := 0

	// Build reader registry
	reg := readers.NewRegistry()
	reg.Register(&readers.OpenCodeReader{})
	reg.Register(&readers.ClaudeReader{})
	reg.Register(&readers.PiReader{})

	// Resolve active inputs
	defs, _, err := input_config.ActiveInputs(cfg.SessionDirs)
	if err != nil {
		return fmt.Errorf("resolve inputs: %w", err)
	}

	// Load project registry
	registry := projects.LoadGlobalRegistry()

	// Collect indexed files
	var indexedFiles []storage.IndexedFile

	// Process sessions via reader registry
	for _, def := range defs {
		if def.Source == "" {
			def.Source = "session"
		}

		reader, err := reg.ForDef(def)
		if err != nil {
			// Warn but continue on reader errors
			continue
		}

		refs, err := reader.Discover(def)
		if err != nil {
			// Warn but continue on discover errors
			continue
		}

		for _, ref := range refs {
			hash, err := reader.Hash(ref)
			if err != nil {
				continue
			}

			// Skip unchanged files UNLESS they are in the stale-set and cap allows.
			// Stale files need re-parsing for extraction_version backfill (perennity).
			if existingHashes[ref] == hash {
				if !staleSet[ref] || staleParsesDone >= staleParsesCap {
					continue
				}
				staleParsesDone++
				fmt.Fprintf(os.Stderr, "Re-parsing stale file %d/%d: %s\n", staleParsesDone, len(stalePaths), ref)
			}

			pf, err := reader.Parse(ref, def)
			if err != nil {
				continue
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
		if err := db.SyncFiles(indexedFiles); err != nil {
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
		fmt.Fprintf(os.Stderr, "warning: failed to discover stale templates: %v\n", err)
	} else if len(staleTemplatePaths) > 0 {
		// Cap at staleTemplateCap per run; FIFO processing across runs
		if len(staleTemplatePaths) > staleTemplateCap {
			staleTemplatePaths = staleTemplatePaths[:staleTemplateCap]
		}

		for _, sourcePath := range staleTemplatePaths {
			msgs, err := db.LoadMessagesForPath(sourcePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to load messages for %s: %v\n", sourcePath, err)
				continue
			}

			deletedCount, err := db.BackfillTemplatesForFile(miner, sourcePath, msgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to re-mine templates for %s: %v\n", sourcePath, err)
			}
			if deletedCount > 0 {
				fmt.Fprintf(os.Stderr, "Deleted %d stuck templates from %s\n", deletedCount, sourcePath)
			}
		}
	}

	// Re-derive correction signals recorded under a superseded detector epoch. This
	// is the only route for a session whose JSONL has expired while its indexed_files
	// row remains: SyncFiles skips it (not on disk) and BackfillDerived skips it (not
	// absent from indexed_files), so without this a detector fix never reaches it.
	// Bounded per run and convergent — see RederiveSupersededCorrections.
	if _, err := db.RederiveSupersededCorrections(staleTemplateCap); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to re-derive superseded correction signals: %v\n", err)
	}

	return nil
}
