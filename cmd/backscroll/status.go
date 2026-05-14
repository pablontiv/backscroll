package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonFormat bool
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show index status and configuration",
		Long: `Status displays information about the backscroll index, including:
- Database path and size
- Number of indexed files and messages
- Last indexing timestamp
- Configuration

Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(stdout, stderr, jsonFormat)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runStatus(stdout, stderr io.Writer, jsonFormat bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check if database exists
	_, err = os.Stat(cfg.DatabasePath)
	dbExists := err == nil

	var stats storage.Stats
	if dbExists {
		// Open database
		db, err := storage.OpenReadOnly(cfg.DatabasePath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = db.Close() }()

		stats, err = db.GetStats()
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}
	}

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
				"total_files":    stats.TotalFiles,
				"total_messages": stats.TotalMessages,
				"indexed_at":     stats.IndexedAt,
			},
			"config": map[string]interface{}{
				"session_dirs": cfg.SessionDirs,
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
			if !stats.IndexedAt.IsZero() {
				_, _ = fmt.Fprintf(stdout, "  Last indexed:     %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
			}
		} else {
			_, _ = fmt.Fprintf(stdout, "\nIndex: Not yet created\n")
		}

		_, _ = fmt.Fprintf(stdout, "\nConfiguration:\n")
		_, _ = fmt.Fprintf(stdout, "  Session directories:\n")
		for _, dir := range cfg.SessionDirs {
			_, _ = fmt.Fprintf(stdout, "    - %s\n", dir)
		}
	}

	return nil
}
