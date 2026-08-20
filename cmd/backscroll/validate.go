package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
)

func newValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "Validate the index integrity",
		SilenceUsage: true,
		Long: `Validate checks the integrity of the SQLite index by verifying:
- Required tables exist
- FTS5 virtual table is set up correctly
- No orphaned records exist

Returns an error if validation fails.

Validate is read-only and never auto-syncs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runValidate(cmd.Context(), stdout, stderr, startup.Config, jsonFormat)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runValidate(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, jsonFormat bool) (retErr error) {
	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseDiagnostics(stdout, stderr, []compat.Diagnostic{*diag}, jsonFormat)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	if err := db.Validate(); err != nil {
		activePath, resolveErr := resolveActiveIndexPath(cfg.DatabasePath)
		if resolveErr != nil {
			return fmt.Errorf("resolve active index path: %w", resolveErr)
		}
		diagnostics, inspectErr := recoveryDiagnosticsForIndex(ctx, db, activePath)
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
