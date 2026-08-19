package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LegacySourcesFile describes one config file containing an unsupported [sources] table.
type LegacySourcesFile struct {
	Path string
	Keys []string
}

// LegacySourcesError reports legacy [sources] tables and deterministic migration guidance.
type LegacySourcesError struct {
	Files     []LegacySourcesFile
	InputsDir string
}

func (e *LegacySourcesError) Error() string {
	var b strings.Builder

	b.WriteString("legacy [sources] configuration is no longer supported; migrate to declarative input files")
	if e.InputsDir != "" {
		b.WriteString(" in ")
		b.WriteString(e.InputsDir)
	}
	b.WriteString(".\n\n")

	b.WriteString("Config files containing [sources]:\n")
	for _, file := range e.Files {
		b.WriteString("- ")
		b.WriteString(file.Path)
		b.WriteString(" (keys: ")
		if len(file.Keys) == 0 {
			b.WriteString("none")
		} else {
			b.WriteString(strings.Join(file.Keys, ", "))
		}
		b.WriteString(")\n")
	}

	b.WriteString("\nCreate *.inputs.toml definitions using decode.format = \"markdown_document\" or \"markdown_sections\".\n")
	b.WriteString("Migration mapping:\n")
	b.WriteString("- ke -> source = \"ke\", decode.format = \"markdown_document\"\n")
	b.WriteString("- memories -> source = \"memory\", decode.format = \"markdown_document\"\n")
	b.WriteString("- decisions -> source = \"decision\", decode.format = \"markdown_sections\"\n")
	b.WriteString("- rules -> source = \"rule\", decode.format = \"markdown_sections\"\n")
	b.WriteString("- specs -> source = \"spec\", decode.format = \"markdown_sections\"\n")
	b.WriteString("- backlog -> source = \"backlog\", decode.format = \"markdown_sections\"")

	return b.String()
}

// ValidateNoLegacySources rejects unsupported legacy [sources] tables in config files.
func ValidateNoLegacySources(inputsDir string) error {
	paths := []string{globalConfigPath(), "./backscroll.toml"}
	var files []LegacySourcesFile

	for _, path := range paths {
		file, err := legacySourcesInFile(path)
		if err != nil {
			return err
		}
		if file != nil {
			files = append(files, *file)
		}
	}

	if len(files) == 0 {
		return nil
	}

	return &LegacySourcesError{Files: files, InputsDir: inputsDir}
}

func legacySourcesInFile(path string) (*LegacySourcesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var envelope struct {
		Sources *SourcesConfig `toml:"sources"`
	}
	if err := toml.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if envelope.Sources == nil {
		return nil, nil
	}

	return &LegacySourcesFile{Path: path, Keys: legacySourcesKeys(envelope.Sources)}, nil
}

func legacySourcesKeys(sources *SourcesConfig) []string {
	var keys []string
	if len(sources.Backlog) > 0 {
		keys = append(keys, "backlog")
	}
	if len(sources.Decisions) > 0 {
		keys = append(keys, "decisions")
	}
	if len(sources.KE) > 0 {
		keys = append(keys, "ke")
	}
	if len(sources.Memories) > 0 {
		keys = append(keys, "memories")
	}
	if len(sources.Rules) > 0 {
		keys = append(keys, "rules")
	}
	if len(sources.Specs) > 0 {
		keys = append(keys, "specs")
	}
	sort.Strings(keys)
	return keys
}
