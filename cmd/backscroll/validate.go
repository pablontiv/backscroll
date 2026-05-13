package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the index integrity",
		Long: `Validate checks the integrity of the SQLite index by verifying:
- Required tables exist
- FTS5 virtual table is set up correctly
- No orphaned records exist

Returns an error if validation fails.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(stdout, stderr)
		},
	}

	return cmd
}

func runValidate(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open database
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Validate
	if err := db.Validate(); err != nil {
		fmt.Fprintf(stdout, "❌ Validation failed: %v\n", err)
		return err
	}

	fmt.Fprintf(stdout, "✓ Index validation passed\n")
	fmt.Fprintf(stdout, "✓ All required tables exist\n")
	fmt.Fprintf(stdout, "✓ FTS5 virtual table is set up correctly\n")
	fmt.Fprintf(stdout, "✓ No orphaned records found\n")

	return nil
}
