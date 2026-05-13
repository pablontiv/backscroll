package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/plans"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/sources"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/backscroll/internal/sync"
	"github.com/pablontiv/backscroll/internal/tagging"
)

func newSyncCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		path          string
		includeAgents bool
		noPlans       bool
		optimize      bool
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
			return runSync(stdout, stderr, path, includeAgents, noPlans, optimize)
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Override session directory")
	cmd.Flags().BoolVar(&includeAgents, "include-agents", false, "Include agent-generated sessions")
	cmd.Flags().BoolVar(&noPlans, "no-plans", false, "Skip indexing markdown plans")
	cmd.Flags().BoolVar(&optimize, "optimize", false, "Optimize FTS5 index after sync")

	return cmd
}

func runSync(stdout, stderr io.Writer, path string, includeAgents, noPlans, optimize bool) error {
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
	defer db.Close()

	// Get existing file hashes
	existingHashes, err := db.GetFileHashes()
	if err != nil {
		return fmt.Errorf("get file hashes: %w", err)
	}

	// Walk session directories
	sessionFiles, err := sync.WalkSessionDirs(cfg.SessionDirs, includeAgents)
	if err != nil {
		return fmt.Errorf("walk session dirs: %w", err)
	}

	// Load project registry
	registry := projects.LoadGlobalRegistry()

	// Collect indexed files
	var indexedFiles []storage.IndexedFile
	var syncedCount int

	// Process session files
	for _, sessionPath := range sessionFiles {
		hash, err := sync.HashFile(sessionPath)
		if err != nil {
			fmt.Fprintf(stderr, "warning: hash file %s: %v\n", sessionPath, err)
			continue
		}

		// Skip if hash matches
		if existingHashes[sessionPath] == hash {
			continue
		}

		// Parse session
		messages, err := sync.ParseSessions(sessionPath)
		if err != nil {
			fmt.Fprintf(stderr, "warning: parse session %s: %v\n", sessionPath, err)
			continue
		}

		// Identify project
		ident := projects.Identify(sessionPath, registry)

		// Collect session text for tagging
		var sessionText string
		for _, msg := range messages {
			sessionText += msg.Content + "\n"
		}

		// Tag the session
		sessionTags := tagging.Tag(sessionText)

		// Convert messages to IndexedMessage
		var indexedMsgs []storage.IndexedMessage
		for ordinal, msg := range messages {
			indexedMsgs = append(indexedMsgs, storage.IndexedMessage{
				Ordinal:     ordinal,
				Role:        msg.Role,
				Text:        msg.Content,
				Timestamp:   msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
				ContentType: msg.ContentType,
			})
		}

		indexedFiles = append(indexedFiles, storage.IndexedFile{
			SourcePath: sessionPath,
			Source:     "session",
			Hash:       hash,
			Project:    ident.ProjectID,
			Messages:   indexedMsgs,
			Tags:       sessionTags,
		})
		syncedCount++
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
			hash, err := sync.HashFile(sourcePath)
			if err != nil {
				fmt.Fprintf(stderr, "warning: hash source file %s: %v\n", sourcePath, err)
				continue
			}

			if existingHashes[sourcePath] == hash {
				continue
			}

			// Parse source file
			sourceItems, err := sources.ParseSectioned(sourcePath, sourceType)
			if err != nil {
				fmt.Fprintf(stderr, "warning: parse source file %s: %v\n", sourcePath, err)
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
				hash, err := sync.HashFile(planPath)
				if err != nil {
					fmt.Fprintf(stderr, "warning: hash plan file %s: %v\n", planPath, err)
					continue
				}

				if existingHashes[planPath] == hash {
					continue
				}

				// Parse plan
				sections, err := plans.ParsePlan(planPath)
				if err != nil {
					fmt.Fprintf(stderr, "warning: parse plan file %s: %v\n", planPath, err)
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

	// Optimize if requested
	if optimize {
		if err := db.OptimizeFTS(); err != nil {
			fmt.Fprintf(stderr, "warning: optimize FTS5: %v\n", err)
		}
	}

	// Print stats
	stats, err := db.GetStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	fmt.Fprintf(stdout, "Synced %d files\n", syncedCount)
	fmt.Fprintf(stdout, "Total indexed: %d files, %d messages\n", stats.TotalFiles, stats.TotalMessages)
	if !stats.IndexedAt.IsZero() {
		fmt.Fprintf(stdout, "Last indexed: %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
	}

	return nil
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}
