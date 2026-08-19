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
	"github.com/pablontiv/backscroll/internal/storage"
)

func newStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonFormat  bool
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show index status and configuration",
		Long: `Status displays information about the backscroll index, including:
- Database path and size
- Number of indexed files and messages
- Last indexing timestamp
- Configuration

Use --json to output as JSON.
Status is read-only and never auto-syncs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(stdout, stderr, jsonFormat, indexedOnly)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Deprecated: status is always read-only")

	return cmd
}

func runStatus(stdout, stderr io.Writer, jsonFormat, indexedOnly bool) error {
	_ = indexedOnly
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check if database exists without creating it. Status is diagnostic/read-only
	// and must not auto-sync or open the index through a writer.
	_, err = os.Stat(cfg.DatabasePath)
	dbExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat database: %w", err)
	}

	var stats storage.Stats
	if dbExists {
		db, diag, err := prepareIndex(context.Background(), cfg, indexDiagnostic, false)
		if diag != nil {
			return refuseDiagnostics(stdout, stderr, []compat.Diagnostic{*diag}, jsonFormat)
		}
		if err != nil {
			return fmt.Errorf("open database read-only: %w", err)
		}
		defer func() { _ = db.Close() }()

		stats, err = db.GetStats()
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}
	}

	// Resolve active inputs for status display
	activeInputNames, usingDeclarative := resolveInputsForStatus(cfg.SessionDirs)

	// Get database file size
	var dbSize int64
	if dbExists {
		fileInfo, _ := os.Stat(cfg.DatabasePath)
		if fileInfo != nil {
			dbSize = fileInfo.Size()
		}
	}

	// Format output
	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"database": map[string]interface{}{
				"path":   cfg.DatabasePath,
				"exists": dbExists,
				"size":   dbSize,
			},
			"index": map[string]interface{}{
				"usable":           dbExists && stats.TotalFiles > 0,
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
		_, _ = fmt.Fprintf(stdout, "Backscroll Status\n")
		_, _ = fmt.Fprintf(stdout, "=================\n\n")

		_, _ = fmt.Fprintf(stdout, "Database:\n")
		if dbExists {
			_, _ = fmt.Fprintf(stdout, "  Path: %s\n", cfg.DatabasePath)
			_, _ = fmt.Fprintf(stdout, "  Size: %.2f MB\n", float64(dbSize)/1024/1024)
		} else {
			_, _ = fmt.Fprintf(stdout, "  Path: %s (not yet created)\n", cfg.DatabasePath)
		}

		if dbExists {
			_, _ = fmt.Fprintf(stdout, "\nIndex:\n")
			_, _ = fmt.Fprintf(stdout, "  Files indexed:    %d\n", stats.TotalFiles)
			_, _ = fmt.Fprintf(stdout, "  Messages indexed: %d\n", stats.TotalMessages)
			_, _ = fmt.Fprintf(stdout, "  Chunks stored:    %d\n", stats.TotalChunks)
			_, _ = fmt.Fprintf(stdout, "  Embeddings:       %d\n", stats.TotalEmbeddings)
			_, _ = fmt.Fprintf(stdout, "  Vectors stored:   %d\n", stats.TotalVectors)
			if !stats.IndexedAt.IsZero() {
				_, _ = fmt.Fprintf(stdout, "  Last indexed:     %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
			}
		} else {
			_, _ = fmt.Fprintf(stdout, "\nIndex: Not yet created\n")
			_, _ = fmt.Fprintf(stdout, "  Files indexed:    0\n")
			_, _ = fmt.Fprintf(stdout, "  Messages indexed: 0\n")
		}

		_, _ = fmt.Fprintf(stdout, "\nConfiguration:\n")
		if usingDeclarative {
			_, _ = fmt.Fprintf(stdout, "  Inputs: %d active (declarative)\n", len(activeInputNames))
			for _, name := range activeInputNames {
				_, _ = fmt.Fprintf(stdout, "    - %s\n", name)
			}
		} else {
			_, _ = fmt.Fprintf(stdout, "  Inputs: legacy (session_dirs)\n")
			for _, dir := range cfg.SessionDirs {
				_, _ = fmt.Fprintf(stdout, "    - %s\n", dir)
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

func recoveryDiagnosticsForIndex(db *storage.Database, activePath string) ([]compat.Diagnostic, error) {
	input, diag, err := storage.ReadRecoveryInput(context.Background(), db)
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
