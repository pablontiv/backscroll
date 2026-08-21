package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

type indexCommandClass uint8

const (
	indexDataRead indexCommandClass = iota
	indexMutation
)

func prepareIndex(ctx context.Context, cfg *config.Config, class indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
	if cfg == nil {
		return nil, &compat.Diagnostic{Code: compat.CodeIndexStale, Summary: "index configuration is unavailable"}, fmt.Errorf("index configuration is unavailable")
	}
	activePath, err := resolveActiveIndexPath(cfg.DatabasePath)
	if err != nil {
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("resolve active index path: %v", err)}, activePath)
		return nil, &d, err
	}

	var db *storage.Database
	var diag *compat.Diagnostic
	switch class {
	case indexDataRead, indexMutation:
		db, diag, err = storage.OpenCompatible(ctx, cfg.DatabasePath)
	default:
		return nil, nil, fmt.Errorf("unknown index command class %d", class)
	}
	if diag != nil {
		d := continuationFor(*diag, activePath)
		return nil, &d, nil
	}
	if err != nil {
		if errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
			d := compat.Diagnostic{
				Code:    compat.CodeIndexStale,
				Summary: fmt.Sprintf("current index snapshot cannot be inspected without side effects while its WAL has uncheckpointed frames; close the writer or checkpoint the database, then retry: %v", err),
			}
			return nil, &d, err
		}
		d := continuationFor(compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: fmt.Sprintf("prepare index failed: %v", err)}, activePath)
		return nil, &d, err
	}

	return db, nil, nil
}

func closeIndexDB(db *storage.Database, err error) error {
	if db == nil {
		return err
	}
	if closeErr := db.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close index database: %w", closeErr))
	}
	return err
}

func continuationFor(d compat.Diagnostic, activePath string) compat.Diagnostic {
	if strings.TrimSpace(activePath) == "" {
		d.Continuation = nil
		if !strings.Contains(d.Summary, "recovery continuation unavailable") {
			d.Summary = strings.TrimSpace(d.Summary) + " (recovery continuation unavailable: active path is empty)"
		}
		return d
	}
	d.Continuation = []string{"recover", "--from", activePath, "--dry-run"}
	return d
}

func writeDiagnostic(stdout, stderr io.Writer, d compat.Diagnostic, jsonMode bool) error {
	if jsonMode {
		payload := struct {
			Code         string   `json:"code"`
			Summary      string   `json:"summary"`
			Continuation []string `json:"continuation_argv,omitempty"`
		}{
			Code:         string(d.Code),
			Summary:      d.Summary,
			Continuation: d.Continuation,
		}
		return json.NewEncoder(stdout).Encode(payload)
	}
	_, err := fmt.Fprintf(stderr, "diagnostic: %s: %s\n", d.Code, strings.TrimSpace(d.Summary))
	if err != nil {
		return err
	}
	if len(d.Continuation) > 0 {
		_, err = fmt.Fprintf(stderr, "continuation: %s\n", strings.Join(d.Continuation, " "))
	}
	return err
}

func writeRobotDiagnostic(stdout io.Writer, d compat.Diagnostic) error {
	if _, err := fmt.Fprintf(stdout, "diagnostic_code=%s\n", robotEscape(string(d.Code))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "diagnostic_summary=%s\n", robotEscape(strings.TrimSpace(d.Summary))); err != nil {
		return err
	}
	if len(d.Continuation) > 0 {
		encoded, err := json.Marshal(d.Continuation)
		if err != nil {
			return fmt.Errorf("encode diagnostic continuation argv: %w", err)
		}
		if _, err := fmt.Fprintf(stdout, "diagnostic_continuation_argv=%s\n", robotEscape(string(encoded))); err != nil {
			return err
		}
	}
	return nil
}

func robotEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func refuseIndex(stdout, stderr io.Writer, d compat.Diagnostic, jsonMode, robotMode bool) error {
	return refuseIndexWithCause(stdout, stderr, d, nil, jsonMode, robotMode)
}

func refuseIndexWithCause(stdout, stderr io.Writer, d compat.Diagnostic, cause error, jsonMode, robotMode bool) error {
	var err error
	if robotMode {
		err = writeRobotDiagnostic(stdout, d)
	} else {
		err = writeDiagnostic(stdout, stderr, d, jsonMode)
	}
	if err != nil {
		return err
	}
	return indexDiagnosticError{diagnostic: d, cause: cause}
}

type indexDiagnosticError struct {
	diagnostic compat.Diagnostic
	cause      error
}

func (e indexDiagnosticError) Error() string {
	return fmt.Sprintf("%s: %s", e.diagnostic.Code, strings.TrimSpace(e.diagnostic.Summary))
}

func (e indexDiagnosticError) Unwrap() error {
	return e.cause
}

func indexPolicyMachineArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "--robot" || strings.HasPrefix(arg, "--json=") || strings.HasPrefix(arg, "--robot=") {
			return true
		}
	}
	return false
}

func resolveActiveIndexPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("database path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return abs, nil
		}
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}
