package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newReindexCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Clear and rebuild the entire index",
		Long: `Reindex deletes all indexed content and rebuilds the database from scratch.

This command clears the entire index and then performs a full sync of all
session directories. Use this to recover from corruption or reset the database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReindex(stdout, stderr)
		},
	}

	return cmd
}

func runReindex(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open database
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Purge all records (before date far in future)
	_, _ = fmt.Fprintf(stdout, "Clearing index...\n")
	_, err = db.Purge("2099-12-31")
	if err != nil {
		return fmt.Errorf("purge database: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Index cleared. Running full sync...\n")

	// Now run sync (this will re-index everything)
	return runSync(stdout, stderr, "", false, false, true)
}
