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

	_, _ = fmt.Fprintf(stdout, "Running incremental sync...\n")
	if err := maybeAutoSync(cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: sync failed: %v\n", err)
	}
	_, _ = fmt.Fprintf(stdout, "Rebuild complete. No indexed data was deleted (perennity contract).\n")
	return nil
}
