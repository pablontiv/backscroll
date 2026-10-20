package readers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestPiNativeToolMetadataBeforeTruncationAndEcho(t *testing.T) {
	padding, _ := json.Marshal(strings.Repeat("x", MaxToolTextLen+1))
	fixture := `{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"direct","name":"bash","arguments":{"aaa":` + string(padding) + `,"command":"backscroll search --text quartz"}},{"type":"toolCall","id":"wrapper","name":"bash","arguments":{"command":"sh -c 'backscroll search --text quartz'"}}]}}` + "\n" +
		`{"type":"message","message":{"role":"toolResult","toolCallId":"wrapper","content":"quartz wrapped output","isError":false}}` + "\n" +
		`{"type":"message","message":{"role":"toolResult","toolCallId":"direct","content":"quartz direct output","isError":true}}` + "\n"
	parsed, err := (&PiReader{}).Parse(context.Background(), writePiFixture(t, fixture), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Records) != 4 {
		t.Fatalf("records: %+v", parsed.Records)
	}
	direct, wrapper := parsed.Records[0], parsed.Records[1]
	if direct.ToolUseID != "direct" || direct.ToolName != "bash" || direct.CommandHead != "backscroll" || direct.IsError == nil || !*direct.IsError || !direct.SearchEcho || len([]rune(direct.Content)) != MaxToolTextLen {
		t.Fatalf("raw native metadata: %+v", direct)
	}
	if wrapper.IsError == nil || *wrapper.IsError || wrapper.SearchEcho || parsed.Records[2].SearchEcho || !parsed.Records[3].SearchEcho {
		t.Fatalf("native linkage/direct boundary: %+v", parsed.Records)
	}
	for _, result := range parsed.Records[2:] {
		if result.ToolName != "" || result.Role != "tool" {
			t.Fatalf("result became call: %+v", result)
		}
	}
}
