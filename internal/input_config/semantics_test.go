package input_config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidatedDefaultsAndRequiredFields(t *testing.T) {
	const valid = `version = 1
[[inputs]]
id = "defaulted"
active = true
[inputs.discover]
roots = ["/does/not/exist"]
include = ["**/*.jsonl"]
`
	for _, tc := range []struct{ name, text, field string }{
		{"defaults", valid, ""},
		{"missing version", strings.ReplaceAll(valid, "version = 1\n", ""), "version"},
		{"unsupported version", strings.ReplaceAll(valid, "version = 1", "version = 99"), "version"},
		{"empty root", strings.ReplaceAll(valid, "/does/not/exist", " "), "discover.roots[0]"},
		{"late include segment", strings.ReplaceAll(valid, "**/*.jsonl", "no-match/["), "discover.include[0]"},
		{"empty pattern", strings.ReplaceAll(valid, "**/*.jsonl", ""), "discover.include[0]"},
		{"inactive", "version = 1\n[[inputs]]\nactive = false\n[inputs.decode]\nformat = \"not-installed\"\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.inputs.toml")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			defs, err := LoadInputsFromDir(dir)
			if tc.field != "" {
				if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.field) {
					t.Fatalf("error = %v, want %s and %s", err, path, tc.field)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "inactive" {
				if len(defs) != 0 {
					t.Fatalf("inactive definitions returned: %+v", defs)
				}
				return
			}
			if len(defs) != 1 || defs[0].Source != "session" || defs[0].Decode.Format != "claude" {
				t.Fatalf("defaults not resolved: %+v", defs)
			}
			files, err := DiscoverFiles(defs[0].Discover)
			if err != nil || len(files) != 0 {
				t.Fatalf("missing-root tolerance: %v, %v", files, err)
			}
		})
	}
}
