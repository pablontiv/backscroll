package main

import (
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestDefaultAutoSyncRegistryIncludesMarkdownReaders(t *testing.T) {
	reg := newDefaultAutoSyncRegistry()

	for _, format := range []string{"markdown_document", "markdown_sections"} {
		t.Run(format, func(t *testing.T) {
			reader, err := reg.ForDef(input_config.InputDefinition{Decode: input_config.DecodeConfig{Format: format}})
			if err != nil {
				t.Fatalf("ForDef(%q) error = %v", format, err)
			}
			if reader.Name() != format {
				t.Fatalf("reader.Name() = %q, want %q", reader.Name(), format)
			}
		})
	}
}
