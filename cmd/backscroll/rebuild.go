package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newRebuildCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the FTS search indexes from the database",
		Long: `Rebuild re-derives the FTS search indexes from the database itself and
runs an incremental sync. It never deletes indexed content: sessions whose
source files have expired from disk are preserved (the database is the
perennial event store). Use 'purge' to delete data explicitly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebuild(stdout, stderr)
		},
	}

	return cmd
}

func runRebuild(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "Re-deriving FTS indexes from database...\n")
	err = db.RebuildFTS()
	if closeErr := db.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("rebuild FTS: %w", err)
	}

	// Backfill templates, corrections, and lossy tool_events for expired files
	_, _ = fmt.Fprintf(stdout, "Backfilling derived data from stored text...\n")
	var backfillStats struct {
		filesProcessed  int
		templatesFound  int
		signalsFound    int
		eventsExtracted int
	}
	if err := db.BackfillDerived(storage.BackfillDerivedOpts{
		OnProgress: func(processed, templateCount, signalCount, eventCount int) {
			backfillStats.filesProcessed = processed
			backfillStats.templatesFound = templateCount
			backfillStats.signalsFound = signalCount
			backfillStats.eventsExtracted = eventCount
		},
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: backfill derived failed: %v\n", err)
	}
	if backfillStats.filesProcessed > 0 {
		_, _ = fmt.Fprintf(stdout, "Backfill complete: %d files processed, %d templates, %d corrections, %d lossy events.\n",
			backfillStats.filesProcessed, backfillStats.templatesFound, backfillStats.signalsFound, backfillStats.eventsExtracted)
	}

	// Re-open for project resolution
	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database for re-resolution: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Re-resolve project identities from session paths
	_, _ = fmt.Fprintf(stdout, "Re-resolving projects from session paths...\n")
	resolver := func(sourcePath string) string {
		cwd := projects.DecodeCwdFromSessionPath(sourcePath)
		if cwd != "" {
			return projects.DeriveFallbackID(cwd)
		}
		return ""
	}
	resolved, err := db.ReresolveProjects(context.Background(), resolver)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: project re-resolution failed: %v\n", err)
	} else if resolved > 0 {
		_, _ = fmt.Fprintf(stdout, "Re-resolved %d sessions with derived project identities.\n", resolved)
	}

	// Registry-aware re-resolution: correct historical fallback labels
	_, _ = fmt.Fprintf(stdout, "Checking registry for project label corrections...\n")
	registry := projects.LoadGlobalRegistry()
	registryMatched, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: registry re-resolution failed: %v\n", err)
	} else if registryMatched > 0 {
		_, _ = fmt.Fprintf(stdout, "Registry matched and corrected %d sessions.\n", registryMatched)
	}

	_, _ = fmt.Fprintf(stdout, "Running incremental sync...\n")
	if err := maybeAutoSync(cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: sync failed: %v\n", err)
	}
	_, _ = fmt.Fprintf(stdout, "Rebuild complete. No indexed data was deleted (perennity contract).\n")
	return nil
}
