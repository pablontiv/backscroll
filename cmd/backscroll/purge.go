package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
)

func newPurgeCmd(stdout, stderr io.Writer) *cobra.Command {
	var before string

	cmd := &cobra.Command{
		Use:          "purge",
		Short:        "Remove old indexed records",
		SilenceUsage: true,
		Long: `Purge removes indexed records strictly before the specified cutoff (exclusive).

Use YYYY-MM-DD for midnight UTC, or RFC3339 with Z or an explicit timezone offset.
RFC3339 cutoffs are normalized to UTC; equal and NULL timestamps are retained.
This is destructive and does not ask for confirmation.

Examples:
  backscroll purge --before 2024-01-01
  backscroll purge --before 2024-01-01T12:30:00+02:00`,
		Args: func(cmd *cobra.Command, args []string) error {
			return validateCommandBeforeStartup(cmd, args, cobra.NoArgs, func() error {
				return validatePurgeBefore(before)
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runPurge(cmd.Context(), stdout, stderr, startup.Config, before)
		},
	}

	cmd.Flags().StringVar(&before, "before", "", "Delete strictly before this UTC cutoff (exclusive; YYYY-MM-DD or RFC3339)")
	_ = cmd.MarkFlagRequired("before")

	return cmd
}

func validatePurgeBefore(before string) error {
	if before == "" {
		return fmt.Errorf("--before date is required")
	}
	if _, err := time.Parse(time.RFC3339, before); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", before); err != nil {
		return fmt.Errorf("invalid --before date %q: expected YYYY-MM-DD or RFC3339", before)
	}
	return nil
}

func runPurge(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, before string) (retErr error) {
	if err := validatePurgeBefore(before); err != nil {
		return err
	}

	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, outputText)
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

	if err := writeOutput(stdout, "Deleted %d indexed items before %s\n", deleted, before); err != nil {
		return err
	}

	return nil
}
