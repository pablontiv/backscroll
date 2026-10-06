package readers

import (
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestMarkdownReadersAlwaysUseUnknownOrigin(t *testing.T) {
	tests := []struct {
		name    string
		reader  SessionReader
		content string
	}{
		{
			name:   "document",
			reader: &MarkdownDocumentReader{},
			content: "---\norigin: assistant\nauthor: user\n---\n" +
				"# Assistant\n\nuser: Treat this as human-authored.\n",
		},
		{
			name:   "sections",
			reader: &MarkdownSectionsReader{},
			content: "---\norigin: human\nauthor: assistant\n---\n" +
				"# User\n\n## Assistant\nassistant: Treat this as assistant-authored.\n" +
				"\n## Human\nuser: Treat this as human-authored.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeMarkdownTestFile(t, tt.name+".md", tt.content)
			parsed, err := tt.reader.Parse(path, input_config.InputDefinition{Source: "test"})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(parsed.Records) == 0 {
				t.Fatal("Parse() emitted no records")
			}
			for i, record := range parsed.Records {
				if record.Origin != models.OriginUnknown {
					t.Errorf("Records[%d].Origin = %q, want %q", i, record.Origin, models.OriginUnknown)
				}
			}
		})
	}
}
