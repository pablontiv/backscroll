package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
)

func newPurgeCmd(stdout, stderr io.Writer) *cobra.Command {
	var before string

	cmd := &cobra.Command{
		Use:          "purge",
		Short:        "Remove old indexed records",
		SilenceUsage: true,
		Long: `Purge removes all indexed records with timestamps before the specified date.

The date should be in YYYY-MM-DD format (e.g., 2024-01-15).

Example: backscroll purge --before 2024-01-01`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPurge(stdout, stderr, before)
		},
	}

	cmd.Flags().StringVar(&before, "before", "", "Delete records before this date (YYYY-MM-DD)")
	_ = cmd.MarkFlagRequired("before")

	return cmd
}

func runPurge(stdout, stderr io.Writer, before string) (retErr error) {
	if before == "" {
		return fmt.Errorf("--before date is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, diag, err := prepareIndex(context.Background(), cfg, indexMutation)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, false, false)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	// Purge records
	deleted, err := db.Purge(before)
	if err != nil {
		return fmt.Errorf("purge records: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Deleted %d indexed items before %s\n", deleted, before)

	return nil
}
