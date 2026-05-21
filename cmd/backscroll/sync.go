package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/chunking"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/embedding"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/plans"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/sources"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/backscroll/internal/tagging"
	"github.com/pablontiv/picokit/hashfile"
)

func newSyncCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		path          string
		includeAgents bool
		noPlans       bool
		optimize      bool
		noEmbed       bool
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Index session files into SQLite",
		Long: `Sync walks your session directories, hashes each file, and indexes new/changed
files into the SQLite database for search. Extracts tags from session content
and indexes plans from ~/.claude/plans/.

Use --path to override session directories.
Use --include-agents to include agent-generated sessions (filtered by default).
Use --no-plans to skip indexing markdown plans.
Use --optimize to rebuild the FTS5 index after syncing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(stdout, stderr, path, includeAgents, noPlans, optimize, noEmbed)
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Override session directory")
	cmd.Flags().BoolVar(&includeAgents, "include-agents", false, "Include agent-generated sessions")
	cmd.Flags().BoolVar(&noPlans, "no-plans", false, "Skip indexing markdown plans")
	cmd.Flags().BoolVar(&optimize, "optimize", false, "Optimize FTS5 index after sync")
	cmd.Flags().BoolVar(&noEmbed, "no-embeddings", false, "Skip embedding generation this run")

	return cmd
}

func runSync(stdout, stderr io.Writer, path string, includeAgents, noPlans, optimize, noEmbed bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Override session dirs if --path given
	if path != "" {
		cfg.SessionDirs = []string{path}
	}

	// Open database
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

	// Resolve active inputs.
	// When --path is given, bypass declarative manifests and use a legacy
	// manifest pointing only to the override path.
	var defs []input_config.InputDefinition
	var mode input_config.InputMode
	if path != "" {
		defs = []input_config.InputDefinition{input_config.SessionDirsToManifest(cfg.SessionDirs)}
		mode = input_config.ModeLegacy
	} else {
		defs, mode, err = input_config.ActiveInputs(cfg.SessionDirs)
		if err != nil {
			return fmt.Errorf("resolve inputs: %w", err)
		}
	}

	// Load project registry
	registry := projects.LoadGlobalRegistry()

	// Collect indexed files
	var indexedFiles []storage.IndexedFile
	var syncedCount int

	// Process sessions via reader registry
	for _, def := range defs {
		if def.Source == "" {
			def.Source = "session"
		}
		// --include-agents: remove subagents exclude from legacy manifests
		if includeAgents && mode == input_config.ModeLegacy {
			def.Discover.Exclude = nil
		}

		reader, err := reg.ForDef(def)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: no reader for input %s: %v\n", def.ID, err)
			continue
		}

		refs, err := reader.Discover(def)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: discover input %s: %v\n", def.ID, err)
			continue
		}

		for _, ref := range refs {
			hash, err := reader.Hash(ref)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: hash %s: %v\n", ref, err)
				continue
			}

			if existingHashes[ref] == hash {
				continue
			}

			pf, err := reader.Parse(ref, def)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: parse %s: %v\n", ref, err)
				continue
			}

			ident := projects.Identify(ref, registry)

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
			syncedCount++
		}
	}

	// Process external sources
	for _, sourceType := range []string{"ke", "decision", "memory", "rule", "spec", "backlog"} {
		var sourcePaths []string
		switch sourceType {
		case "ke":
			sourcePaths = cfg.Sources.KE
		case "decision":
			sourcePaths = cfg.Sources.Decisions
		case "memory":
			sourcePaths = cfg.Sources.Memories
		case "rule":
			sourcePaths = cfg.Sources.Rules
		case "spec":
			sourcePaths = cfg.Sources.Specs
		case "backlog":
			sourcePaths = cfg.Sources.Backlog
		}

		for _, sourcePath := range sourcePaths {
			hash, err := hashfile.HashFile(sourcePath)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: hash source file %s: %v\n", sourcePath, err)
				continue
			}

			if existingHashes[sourcePath] == hash {
				continue
			}

			// Parse source file
			sourceItems, err := sources.ParseSectioned(sourcePath, sourceType)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: parse source file %s: %v\n", sourcePath, err)
				continue
			}

			// Identify project
			ident := projects.Identify(sourcePath, registry)

			// Convert source items to IndexedMessage
			var indexedMsgs []storage.IndexedMessage
			for ordinal, item := range sourceItems {
				indexedMsgs = append(indexedMsgs, storage.IndexedMessage{
					Ordinal:     ordinal,
					Role:        "system",
					Text:        item.Content,
					Timestamp:   "",
					ContentType: "text",
				})
			}

			indexedFiles = append(indexedFiles, storage.IndexedFile{
				SourcePath: sourcePath,
				Source:     sourceType,
				Hash:       hash,
				Project:    ident.ProjectID,
				Messages:   indexedMsgs,
				Tags:       []string{},
			})
			syncedCount++
		}
	}

	// Process plans if not skipped
	if !noPlans {
		planDir := filepath.Join(homeDir(), ".claude", "plans")
		planFiles, err := plans.DiscoverPlanFiles(planDir)
		if err == nil {
			for _, planPath := range planFiles {
				hash, err := hashfile.HashFile(planPath)
				if err != nil {
					_, _ = fmt.Fprintf(stderr, "warning: hash plan file %s: %v\n", planPath, err)
					continue
				}

				if existingHashes[planPath] == hash {
					continue
				}

				// Parse plan
				sections, err := plans.ParsePlan(planPath)
				if err != nil {
					_, _ = fmt.Fprintf(stderr, "warning: parse plan file %s: %v\n", planPath, err)
					continue
				}

				// Identify project
				ident := projects.Identify(planPath, registry)

				// Convert sections to IndexedMessage
				var indexedMsgs []storage.IndexedMessage
				for ordinal, section := range sections {
					indexedMsgs = append(indexedMsgs, storage.IndexedMessage{
						Ordinal:     ordinal,
						Role:        "plan",
						Text:        section.Content,
						Timestamp:   "",
						ContentType: "text",
					})
				}

				indexedFiles = append(indexedFiles, storage.IndexedFile{
					SourcePath: planPath,
					Source:     "plan",
					Hash:       hash,
					Project:    ident.ProjectID,
					Messages:   indexedMsgs,
					Tags:       []string{},
				})
				syncedCount++
			}
		}
	}

	// Sync all files
	if len(indexedFiles) > 0 {
		if err := db.SyncFiles(indexedFiles); err != nil {
			return fmt.Errorf("sync files: %w", err)
		}
	}

	// Chunk and embed newly synced content when embedding is enabled
	if cfg.Embedding.Enabled && !noEmbed && len(indexedFiles) > 0 {
		runEmbedPipeline(stdout, stderr, db, cfg.Embedding, indexedFiles)
	}

	// Optimize if requested
	if optimize {
		if err := db.OptimizeFTS(); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: optimize FTS5: %v\n", err)
		}
	}

	// Print stats
	stats, err := db.GetStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Synced %d files\n", syncedCount)
	_, _ = fmt.Fprintf(stdout, "Total indexed: %d files, %d messages\n", stats.TotalFiles, stats.TotalMessages)
	if !stats.IndexedAt.IsZero() {
		_, _ = fmt.Fprintf(stdout, "Last indexed: %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
	}

	return nil
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// runEmbedPipeline chunks and embeds each newly synced file.
// Provider errors are logged as warnings and do not abort sync.
func runEmbedPipeline(stdout, stderr io.Writer, db *storage.Database, cfg config.EmbeddingConfig, files []storage.IndexedFile) {
	provider, err := embedding.NewOnnxProvider(cfg.ModelPath)
	if err != nil {
		if errors.Is(err, embedding.ErrOnnxNotAvailable) {
			_, _ = fmt.Fprintf(stderr, "warning: embedding provider not available (%v); storing chunks only\n", err)
		} else {
			_, _ = fmt.Fprintf(stderr, "warning: embedding provider init failed: %v; storing chunks only\n", err)
		}
		provider = nil
	}

	now := time.Now().Unix()
	var totalChunks int

	for _, f := range files {
		// Build full text from all messages in this file
		var full string
		for _, msg := range f.Messages {
			if msg.Text != "" {
				full += msg.Text + "\n"
			}
		}
		if full == "" {
			continue
		}

		texts := chunking.ChunkText(full, 512, 50)
		records := make([]storage.ChunkRecord, len(texts))
		for i, t := range texts {
			records[i] = storage.ChunkRecord{
				ChunkIdx:   i,
				Content:    t,
				TokenCount: chunking.TokenCount(t),
			}
		}

		chunkIDs, err := db.InsertChunks(f.SourcePath, records, now)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: store chunks for %s: %v\n", f.SourcePath, err)
			continue
		}
		totalChunks += len(chunkIDs)

		if provider == nil {
			continue
		}

		for i, id := range chunkIDs {
			if err := db.InsertEmbeddingMetadata(id, cfg.ModelName, "", provider.Dimensions(), now); err != nil {
				_, _ = fmt.Fprintf(stderr, "warning: store embedding metadata chunk %d: %v\n", i, err)
			}
		}
	}

	if totalChunks > 0 {
		_, _ = fmt.Fprintf(stdout, "Stored %d chunks\n", totalChunks)
	}
}
