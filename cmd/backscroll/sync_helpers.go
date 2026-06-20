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

	// Build reader registry
	reg := readers.NewRegistry()
	reg.Register(&readers.JsonlReader{})
	reg.Register(&readers.OpenCodeReader{})

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

			if existingHashes[ref] == hash {
				continue
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
					Ordinal:     ordinal,
					Role:        msg.Role,
					Text:        msg.Content,
					Timestamp:   msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
					ContentType: msg.ContentType,
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

	return nil
}

// runSync is called by rebuild to perform a full sync.
func runSync(stdout, stderr io.Writer, path string, includePlans, includeSources, optimize, noEmbed bool) error {
	// For now, this is a stub that prints a message
	// In v2, rebuild calls this but the actual sync logic is in maybeAutoSync
	_, _ = fmt.Fprintf(stdout, "Sync: no-op in v2 (auto-sync handles incremental updates)\n")
	return nil
}
