package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type preparedEnvironment struct {
	WorkDir, FixtureRoot, FixtureDigest string
	Env                                 []string
	Cleanup                             func()
}

func prepareSyntheticEnvironment(root string) (preparedEnvironment, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return preparedEnvironment{}, err
	}
	digest, err := fixtureTreeDigest(absRoot)
	if err != nil {
		return preparedEnvironment{}, err
	}
	work, err := os.MkdirTemp("", "backscroll-recall-eval-")
	if err != nil {
		return preparedEnvironment{}, err
	}
	cleanup := func() { _ = os.RemoveAll(work) }
	configRoot := filepath.Join(work, "config")
	inputsDir := filepath.Join(configRoot, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		cleanup()
		return preparedEnvironment{}, err
	}
	manifest := fmt.Sprintf("version = 1\n\n[[inputs]]\nid = %q\nsource = %q\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\nexclude = []\n[inputs.decode]\nformat = \"claude\"\n", "recall-eval", "session", absRoot)
	if err := os.WriteFile(filepath.Join(inputsDir, "recall-eval.inputs.toml"), []byte(manifest), 0o600); err != nil {
		cleanup()
		return preparedEnvironment{}, err
	}
	env := overrideEnv(os.Environ(), map[string]string{
		"HOME": work, "XDG_CONFIG_HOME": configRoot, "BACKSCROLL_CONFIG_DIR": configRoot,
		"BACKSCROLL_DATABASE_PATH": filepath.Join(work, "backscroll.db"), "BACKSCROLL_SESSION_DIRS": "",
	})
	return preparedEnvironment{WorkDir: work, FixtureRoot: absRoot, FixtureDigest: digest, Env: env, Cleanup: cleanup}, nil
}

func overrideEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[key]; !replaced {
			out = append(out, entry)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func fixtureTreeDigest(root string) (string, error) {
	h := sha256.New()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, path := range paths {
		rel, _ := filepath.Rel(root, path)
		_, _ = io.WriteString(h, filepath.ToSlash(rel)+"\x00")
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func selectQueries(queries []evalQuery, dataset string, limit int) []evalQuery {
	var selected []evalQuery
	for _, q := range queries {
		if q.Dataset == dataset {
			selected = append(selected, q)
		}
	}
	if limit > 0 && limit < len(selected) {
		selected = selected[:limit]
	}
	return selected
}

func validateQueries(queries []evalQuery, dataset string) error {
	seen := make(map[string]bool)
	for _, q := range queries {
		if q.ID == "" || q.Text == "" || seen[q.ID] {
			return fmt.Errorf("invalid query id/text or duplicate id %q", q.ID)
		}
		seen[q.ID] = true
		if q.ExpectError && q.ExpectAbsent {
			return fmt.Errorf("query %q cannot expect error and absence", q.ID)
		}
		if dataset == "synthetic" && !q.ExpectError && q.ExpectedFile == "" {
			return fmt.Errorf("synthetic query %q has no expected_file", q.ID)
		}
		clean := filepath.Clean(q.ExpectedFile)
		if q.ExpectedFile != "" && (filepath.IsAbs(q.ExpectedFile) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))) {
			return fmt.Errorf("query %q expected_file escapes fixture root", q.ID)
		}
	}
	return nil
}
