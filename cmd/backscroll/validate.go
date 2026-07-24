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
		Args:         cobra.NoArgs,
		Short:        "Validate the index integrity",
		SilenceUsage: true,
		Long: `This command accepts no positional operands.

Validate checks the integrity of the SQLite index by verifying:
- Required tables exist
- FTS5 virtual table is set up correctly
- No orphaned records exist

Returns an error if validation fails.

Command startup may synchronize active inputs before this handler runs. The
validate handler then checks the prepared SQLite index without performing a
second sync.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := startupResultFrom(cmd).requireConfig()
			if err != nil {
				return err
			}
			return runValidate(cmd.Context(), stdout, stderr, cfg, invocationOutputMode(cmd))
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runValidate(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, output outputMode) (retErr error) {
	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseDiagnostics(stdout, stderr, []compat.Diagnostic{*diag}, output == outputJSON)
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
			return refuseDiagnostics(stdout, stderr, diagnostics, output == outputJSON)
		}
		if output == outputJSON {
			if encodeErr := json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "error": err.Error()}); encodeErr != nil {
				return encodeErr
			}
			return err
		}
		if err := writeOutput(stdout, "❌ Validation failed: %v\n", err); err != nil {
			return err
		}
		return err
	}

	if output == outputJSON {
		return json.NewEncoder(stdout).Encode(map[string]any{"valid": true, "database_exists": true})
	}
	if err := writeOutput(stdout, "✓ Index validation passed\n"); err != nil {
		return err
	}
	if err := writeOutput(stdout, "✓ All required tables exist\n"); err != nil {
		return err
	}
	if err := writeOutput(stdout, "✓ FTS5 virtual table is set up correctly\n"); err != nil {
		return err
	}
	if err := writeOutput(stdout, "✓ No orphaned records found\n"); err != nil {
		return err
	}

	return nil
}
