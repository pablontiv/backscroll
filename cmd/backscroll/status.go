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
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/services"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Show index status and configuration",
		SilenceUsage: true,
		Long: `Status displays information about the backscroll index, including:
- Database path and size
- Number of indexed files and messages
- Last indexing timestamp
- Configuration

Use --json to output as JSON.
Startup preflight may sync before status runs; status itself only reads current index state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runStatus(cmd.Context(), stdout, stderr, startup.Config, jsonFormat)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runStatus(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, jsonFormat bool) (retErr error) {
	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseDiagnostics(stdout, stderr, []compat.Diagnostic{*diag}, jsonFormat)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	queryService := services.QueryService{DB: db}
	response, err := queryService.Status(ctx, services.StatusRequest{})
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}
	stats := response.Stats

	// Resolve active inputs for status display
	activeInputNames, usingDeclarative := resolveInputsForStatus(cfg.SessionDirs)

	// Get database file size after startup has prepared the canonical database.
	fileInfo, err := os.Stat(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}
	dbSize := fileInfo.Size()

	// Format output
	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"database": map[string]interface{}{
				"path":   cfg.DatabasePath,
				"exists": true,
				"size":   dbSize,
			},
			"index": map[string]interface{}{
				"usable":           stats.TotalFiles > 0,
				"total_files":      stats.TotalFiles,
				"total_messages":   stats.TotalMessages,
				"indexed_at":       stats.IndexedAt,
				"total_chunks":     stats.TotalChunks,
				"total_embeddings": stats.TotalEmbeddings,
				"total_vectors":    stats.TotalVectors,
			},
			"config": map[string]interface{}{
				"session_dirs":             cfg.SessionDirs,
				"active_inputs":            activeInputNames,
				"using_declarative_inputs": usingDeclarative,
			},
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		// Text output
		if err := writeOutput(stdout, "Backscroll Status\n"); err != nil {
			return err
		}
		if err := writeOutput(stdout, "=================\n\n"); err != nil {
			return err
		}

		if err := writeOutput(stdout, "Database:\n"); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Path: %s\n", cfg.DatabasePath); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Size: %.2f MB\n", float64(dbSize)/1024/1024); err != nil {
			return err
		}

		if err := writeOutput(stdout, "\nIndex:\n"); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Files indexed:    %d\n", stats.TotalFiles); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Messages indexed: %d\n", stats.TotalMessages); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Chunks stored:    %d\n", stats.TotalChunks); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Embeddings:       %d\n", stats.TotalEmbeddings); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Vectors stored:   %d\n", stats.TotalVectors); err != nil {
			return err
		}
		if !stats.IndexedAt.IsZero() {
			if err := writeOutput(stdout, "  Last indexed:     %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST")); err != nil {
				return err
			}
		}

		if err := writeOutput(stdout, "\nConfiguration:\n"); err != nil {
			return err
		}
		if usingDeclarative {
			if err := writeOutput(stdout, "  Inputs: %d active (declarative)\n", len(activeInputNames)); err != nil {
				return err
			}
			for _, name := range activeInputNames {
				if err := writeOutput(stdout, "    - %s\n", name); err != nil {
					return err
				}
			}
		} else {
			if err := writeOutput(stdout, "  Inputs: legacy (session_dirs)\n"); err != nil {
				return err
			}
			for _, dir := range cfg.SessionDirs {
				if err := writeOutput(stdout, "    - %s\n", dir); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// resolveInputsForStatus returns the list of active input IDs and whether
// declarative inputs (*.inputs.toml) are in use.
func resolveInputsForStatus(sessionDirs []string) ([]string, bool) {
	defs, mode, err := input_config.ActiveInputs(sessionDirs)
	if err != nil || mode != input_config.ModeDeclarative {
		return []string{}, false
	}
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		if d.ID != "" {
			names = append(names, d.ID)
		}
	}
	return names, true
}

func recoveryDiagnosticsForIndex(ctx context.Context, db *storage.Database, activePath string) ([]compat.Diagnostic, error) {
	input, diag, err := storage.ReadRecoveryInput(ctx, db)
	if diag != nil {
		d := continuationFor(*diag, activePath)
		return []compat.Diagnostic{d}, err
	}
	if err != nil {
		return nil, err
	}
	_, diagnostics, err := compat.PlanRecovery([]compat.RecoveryInput{input})
	if err != nil {
		return nil, err
	}
	for i := range diagnostics {
		diagnostics[i] = continuationFor(diagnostics[i], activePath)
	}
	return diagnostics, nil
}

func refuseDiagnostics(stdout, stderr io.Writer, diagnostics []compat.Diagnostic, jsonMode bool) error {
	if len(diagnostics) == 0 {
		return nil
	}
	if jsonMode || len(diagnostics) == 1 {
		if err := writeDiagnostic(stdout, stderr, diagnostics[0], jsonMode); err != nil {
			return err
		}
		return indexDiagnosticError{diagnostic: diagnostics[0]}
	}
	for _, diagnostic := range diagnostics {
		if err := writeDiagnostic(stdout, stderr, diagnostic, false); err != nil {
			return err
		}
	}
	return indexDiagnosticError{diagnostic: diagnostics[0]}
}
