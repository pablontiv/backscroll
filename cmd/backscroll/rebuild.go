package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/storage"
)

type rebuildDependencies struct {
	prepareIndex                         func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error)
	closeIndexDB                         func(*storage.Database, error) error
	rebuildFTSContext                    func(*storage.Database, context.Context) error
	backfillDerivedContext               func(*storage.Database, context.Context, storage.BackfillDerivedOpts) error
	reresolveProjectsContext             func(*storage.Database, context.Context, func(string) string) (int64, error)
	loadRegistry                         func() projects.ProjectRegistry
	reresolveProjectsWithRegistryContext func(*storage.Database, context.Context, projects.ProjectRegistry) (int64, error)
}

type rebuildRunner struct {
	deps rebuildDependencies
}

func newRebuildRunner(deps *rebuildDependencies) *rebuildRunner {
	resolved := rebuildDependencies{
		prepareIndex: prepareIndex,
		closeIndexDB: closeIndexDB,
		rebuildFTSContext: func(db *storage.Database, ctx context.Context) error {
			return db.RebuildFTSContext(ctx)
		},
		backfillDerivedContext: func(db *storage.Database, ctx context.Context, opts storage.BackfillDerivedOpts) error {
			return db.BackfillDerivedContext(ctx, opts)
		},
		reresolveProjectsContext: func(db *storage.Database, ctx context.Context, resolver func(string) string) (int64, error) {
			return db.ReresolveProjectsContext(ctx, resolver)
		},
		loadRegistry: projects.LoadGlobalRegistry,
		reresolveProjectsWithRegistryContext: func(db *storage.Database, ctx context.Context, registry projects.ProjectRegistry) (int64, error) {
			return db.ReresolveProjectsWithRegistryContext(ctx, registry)
		},
	}
	if deps == nil {
		return &rebuildRunner{deps: resolved}
	}
	if deps.prepareIndex != nil {
		resolved.prepareIndex = deps.prepareIndex
	}
	if deps.closeIndexDB != nil {
		resolved.closeIndexDB = deps.closeIndexDB
	}
	if deps.rebuildFTSContext != nil {
		resolved.rebuildFTSContext = deps.rebuildFTSContext
	}
	if deps.backfillDerivedContext != nil {
		resolved.backfillDerivedContext = deps.backfillDerivedContext
	}
	if deps.reresolveProjectsContext != nil {
		resolved.reresolveProjectsContext = deps.reresolveProjectsContext
	}
	if deps.loadRegistry != nil {
		resolved.loadRegistry = deps.loadRegistry
	}
	if deps.reresolveProjectsWithRegistryContext != nil {
		resolved.reresolveProjectsWithRegistryContext = deps.reresolveProjectsWithRegistryContext
	}
	return &rebuildRunner{deps: resolved}
}

func newRebuildCmd(stdout, stderr io.Writer, runner *rebuildRunner) *cobra.Command {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if runner == nil {
		runner = newRebuildRunner(nil)
	}
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
			return runner.runRebuild(cmd.Context(), stdout, stderr, startup.Config)
		},
	}

	return cmd
}

func runRebuild(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config) error {
	return newRebuildRunner(nil).runRebuild(ctx, stdout, stderr, cfg)
}

func (r *rebuildRunner) runRebuild(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config) (retErr error) {
	if r == nil {
		r = newRebuildRunner(nil)
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	db, diag, err := r.deps.prepareIndex(ctx, cfg, indexMutation)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, false, false)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = r.deps.closeIndexDB(db, retErr) }()

	if err := writeRebuildOutput(stdout, "Re-deriving FTS indexes from database...\n"); err != nil {
		return err
	}
	if err := r.deps.rebuildFTSContext(db, ctx); err != nil {
		return fmt.Errorf("rebuild FTS: %w", err)
	}

	// Backfill templates, corrections, and lossy tool_events for expired files.
	if err := writeRebuildOutput(stdout, "Backfilling derived data from stored text...\n"); err != nil {
		return err
	}
	var backfillStats struct {
		filesProcessed  int
		templatesFound  int
		signalsFound    int
		eventsExtracted int
	}
	if err := r.deps.backfillDerivedContext(db, ctx, storage.BackfillDerivedOpts{
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
		if err := writeRebuildOutput(stdout, "Backfill complete: %d files processed, %d templates, %d corrections, %d lossy events.\n",
			backfillStats.filesProcessed, backfillStats.templatesFound, backfillStats.signalsFound, backfillStats.eventsExtracted); err != nil {
			return err
		}
	}

	// Re-resolve project identities from session paths.
	if err := writeRebuildOutput(stdout, "Re-resolving projects from session paths...\n"); err != nil {
		return err
	}
	resolver := func(sourcePath string) string {
		cwd := projects.DecodeCwdFromSessionPath(sourcePath)
		if cwd != "" {
			return projects.DeriveFallbackID(cwd)
		}
		return ""
	}
	resolved, err := r.deps.reresolveProjectsContext(db, ctx, resolver)
	if err != nil {
		return fmt.Errorf("project re-resolution: %w", err)
	} else if resolved > 0 {
		if err := writeRebuildOutput(stdout, "Re-resolved %d sessions with derived project identities.\n", resolved); err != nil {
			return err
		}
	}

	// Registry-aware re-resolution corrects historical fallback labels.
	if err := writeRebuildOutput(stdout, "Checking registry for project label corrections...\n"); err != nil {
		return err
	}
	registry := r.deps.loadRegistry()
	registryMatched, err := r.deps.reresolveProjectsWithRegistryContext(db, ctx, registry)
	if err != nil {
		return fmt.Errorf("registry re-resolution: %w", err)
	} else if registryMatched > 0 {
		if err := writeRebuildOutput(stdout, "Registry matched and corrected %d sessions.\n", registryMatched); err != nil {
			return err
		}
	}

	return writeRebuildOutput(stdout, "Rebuild complete. No indexed data was deleted (perennity contract).\n")
}

func writeRebuildOutput(w io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return fmt.Errorf("write rebuild output: %w", err)
	}
	return nil
}
