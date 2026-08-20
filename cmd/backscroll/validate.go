package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
)

func newValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the index integrity",
		Long: `Validate checks the integrity of the SQLite index by verifying:
- Required tables exist
- FTS5 virtual table is set up correctly
- No orphaned records exist

Returns an error if validation fails.

Validate is read-only and never auto-syncs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(stdout, stderr, jsonFormat)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runValidate(stdout, stderr io.Writer, jsonFormat bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if _, err := os.Stat(cfg.DatabasePath); os.IsNotExist(err) {
		if jsonFormat {
			return json.NewEncoder(stdout).Encode(map[string]any{"valid": true, "database_exists": false})
		}
		_, _ = fmt.Fprintf(stdout, "✓ Index validation skipped: database not found\n")
		return nil
	} else if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}

	db, diag, err := prepareIndex(context.Background(), cfg, indexDiagnostic)
	if diag != nil {
		return refuseDiagnostics(stdout, stderr, []compat.Diagnostic{*diag}, jsonFormat)
	}
	if err != nil {
		return fmt.Errorf("open database read-only: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Validate(); err != nil {
		activePath, resolveErr := resolveActiveIndexPath(cfg.DatabasePath)
		if resolveErr != nil {
			return fmt.Errorf("resolve active index path: %w", resolveErr)
		}
		diagnostics, inspectErr := recoveryDiagnosticsForIndex(db, activePath)
		if inspectErr != nil {
			return fmt.Errorf("inspect recovery diagnostics: %w", inspectErr)
		}
		if len(diagnostics) > 0 {
			return refuseDiagnostics(stdout, stderr, diagnostics, jsonFormat)
		}
		if jsonFormat {
			if encodeErr := json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "error": err.Error()}); encodeErr != nil {
				return encodeErr
			}
			return err
		}
		_, _ = fmt.Fprintf(stdout, "❌ Validation failed: %v\n", err)
		return err
	}

	if jsonFormat {
		return json.NewEncoder(stdout).Encode(map[string]any{"valid": true, "database_exists": true})
	}
	_, _ = fmt.Fprintf(stdout, "✓ Index validation passed\n")
	_, _ = fmt.Fprintf(stdout, "✓ All required tables exist\n")
	_, _ = fmt.Fprintf(stdout, "✓ FTS5 virtual table is set up correctly\n")
	_, _ = fmt.Fprintf(stdout, "✓ No orphaned records found\n")

	return nil
}
