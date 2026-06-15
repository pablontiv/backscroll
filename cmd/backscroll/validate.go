package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	var indexedOnly bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the index integrity",
		Long: `Validate checks the integrity of the SQLite index by verifying:
- Required tables exist
- FTS5 virtual table is set up correctly
- No orphaned records exist

Returns an error if validation fails.

Use --indexed-only to skip auto-sync (validate existing index only).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(stdout, stderr, indexedOnly)
		},
	}

	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")

	return cmd
}

func runValidate(stdout, stderr io.Writer, indexedOnly bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Auto-sync before validate unless --indexed-only is set
	if !indexedOnly {
		if err := maybeAutoSync(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: auto-sync failed: %v; validating cached index\n", err)
		}
	}

	// Open database
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Validate
	if err := db.Validate(); err != nil {
		_, _ = fmt.Fprintf(stdout, "❌ Validation failed: %v\n", err)
		return err
	}

	_, _ = fmt.Fprintf(stdout, "✓ Index validation passed\n")
	_, _ = fmt.Fprintf(stdout, "✓ All required tables exist\n")
	_, _ = fmt.Fprintf(stdout, "✓ FTS5 virtual table is set up correctly\n")
	_, _ = fmt.Fprintf(stdout, "✓ No orphaned records found\n")

	return nil
}
