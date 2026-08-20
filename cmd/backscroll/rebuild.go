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
		Use:          "rebuild",
		Short:        "Rebuild the FTS search indexes from the database",
		SilenceUsage: true,
		Long: `Rebuild re-derives the FTS search indexes from the database itself and
operates on the index synchronized at command startup. It never deletes indexed
content: sessions whose source files have expired from disk are preserved (the
database is the perennial event store). Use 'purge' to delete data explicitly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runRebuild(cmd.Context(), stdout, stderr, startup.Config)
		},
	}

	return cmd
}

var (
	rebuildBackfillDerived = func(db *storage.Database, opts storage.BackfillDerivedOpts) error {
		return db.BackfillDerived(opts)
	}
	rebuildReresolveProjects = func(db *storage.Database, ctx context.Context, resolver func(string) string) (int64, error) {
		return db.ReresolveProjects(ctx, resolver)
	}
	rebuildReresolveProjectsWithRegistry = func(db *storage.Database, ctx context.Context, registry projects.ProjectRegistry) (int64, error) {
		return db.ReresolveProjectsWithRegistry(ctx, registry)
	}
)

func runRebuild(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config) (retErr error) {
	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, false, false)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	_, _ = fmt.Fprintf(stdout, "Re-deriving FTS indexes from database...\n")
	if err := db.RebuildFTS(); err != nil {
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
	if err := rebuildBackfillDerived(db, storage.BackfillDerivedOpts{
		OnProgress: func(processed, templateCount, signalCount, eventCount int) {
			backfillStats.filesProcessed = processed
			backfillStats.templatesFound = templateCount
			backfillStats.signalsFound = signalCount
			backfillStats.eventsExtracted = eventCount
		},
	}); err != nil {
		return fmt.Errorf("backfill derived: %w", err)
	}
	if backfillStats.filesProcessed > 0 {
		_, _ = fmt.Fprintf(stdout, "Backfill complete: %d files processed, %d templates, %d corrections, %d lossy events.\n",
			backfillStats.filesProcessed, backfillStats.templatesFound, backfillStats.signalsFound, backfillStats.eventsExtracted)
	}

	// Re-resolve project identities from session paths
	_, _ = fmt.Fprintf(stdout, "Re-resolving projects from session paths...\n")
	resolver := func(sourcePath string) string {
		cwd := projects.DecodeCwdFromSessionPath(sourcePath)
		if cwd != "" {
			return projects.DeriveFallbackID(cwd)
		}
		return ""
	}
	resolved, err := rebuildReresolveProjects(db, ctx, resolver)
	if err != nil {
		return fmt.Errorf("project re-resolution: %w", err)
	} else if resolved > 0 {
		_, _ = fmt.Fprintf(stdout, "Re-resolved %d sessions with derived project identities.\n", resolved)
	}

	// Registry-aware re-resolution: correct historical fallback labels
	_, _ = fmt.Fprintf(stdout, "Checking registry for project label corrections...\n")
	registry := projects.LoadGlobalRegistry()
	registryMatched, err := rebuildReresolveProjectsWithRegistry(db, ctx, registry)
	if err != nil {
		return fmt.Errorf("registry re-resolution: %w", err)
	} else if registryMatched > 0 {
		_, _ = fmt.Fprintf(stdout, "Registry matched and corrected %d sessions.\n", registryMatched)
	}

	_, _ = fmt.Fprintf(stdout, "Rebuild complete. No indexed data was deleted (perennity contract).\n")
	return nil
}
