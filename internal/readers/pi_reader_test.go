package readers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

func writePiFixture(t *testing.T, lines string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pi.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPiReader_Name(t *testing.T) {
	if (&PiReader{}).Name() != "pi" {
		t.Error("Name != pi")
	}
}

func TestPiReader_TextAndCwd(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","cwd":"/home/shared/proj","message":{"role":"user","content":"hello pi"}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: false},
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Cwd != "/home/shared/proj" {
		t.Errorf("Cwd = %q, want /home/shared/proj", pf.Cwd)
	}
	if len(pf.Records) != 1 || pf.Records[0].Content != "hello pi" || pf.Records[0].ContentType != "text" {
		t.Fatalf("records = %+v", pf.Records)
	}
}

func TestPiReader_RecordTypeMatchesLegacyEnvelope(t *testing.T) {
	const timestamp = "2026-05-10T22:19:34.694Z"
	legacy := `{"type":"message","timestamp":"` + timestamp + `","cwd":"/home/shared/proj","message":{"role":"assistant","content":[{"type":"text","text":"pion text token"},{"type":"toolCall","name":"web_search","arguments":{"queries":["pion tool token"]}},{"type":"thinking","text":"pion reasoning token"}]}}` + "\n"
	pion := `{"recordType":"message","timestamp":"` + timestamp + `","cwd":"/home/shared/proj","message":{"role":"assistant","content":[{"type":"text","text":"pion text token"},{"type":"toolCall","name":"web_search","arguments":{"queries":["pion tool token"]}},{"type":"thinking","text":"pion reasoning token"}]}}` + "\n"
	def := input_config.InputDefinition{Decode: input_config.DecodeConfig{IndexReasoning: true}}

	legacyFile, err := (&PiReader{}).Parse(writePiFixture(t, legacy), def)
	if err != nil {
		t.Fatalf("parse legacy envelope: %v", err)
	}
	pionFile, err := (&PiReader{}).Parse(writePiFixture(t, pion), def)
	if err != nil {
		t.Fatalf("parse Pion envelope: %v", err)
	}

	if legacyFile.Cwd != pionFile.Cwd {
		t.Fatalf("cwd differs: legacy %q, Pion %q", legacyFile.Cwd, pionFile.Cwd)
	}
	if !reflect.DeepEqual(legacyFile.Records, pionFile.Records) {
		t.Fatalf("normalized records differ:\nlegacy: %+v\nPion:  %+v", legacyFile.Records, pionFile.Records)
	}
	if len(pionFile.Records) != 3 {
		t.Fatalf("Pion records = %d, want text, tool, and reasoning", len(pionFile.Records))
	}
	wantTimestamp, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range pionFile.Records {
		if !record.Timestamp.Equal(wantTimestamp) {
			t.Errorf("timestamp = %s, want %s", record.Timestamp, wantTimestamp)
		}
	}
}

func TestPiReader_EnvelopePolicy(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantLen int
	}{
		{name: "legacy type", line: `{"type":"message","message":{"role":"user","content":"legacy"}}`, wantLen: 1},
		{name: "Pion recordType", line: `{"recordType":"message","message":{"role":"user","content":"pion"}}`, wantLen: 1},
		{name: "dual agreement", line: `{"type":"message","recordType":"message","message":{"role":"user","content":"agreed"}}`, wantLen: 1},
		{name: "dual conflict", line: `{"type":"message","recordType":"tool_start","message":{"role":"user","content":"conflict"}}`},
		{name: "null type is present", line: `{"type":null,"recordType":"message","message":{"role":"user","content":"null legacy"}}`},
		{name: "null recordType is present", line: `{"type":"message","recordType":null,"message":{"role":"user","content":"null Pion"}}`},
		{name: "both discriminators null", line: `{"type":null,"recordType":null,"message":{"role":"user","content":"both null"}}`},
		{name: "conflict cannot become custom", line: `{"type":"custom","recordType":"tool_end","customType":"result","data":{"text":"conflict"}}`},
		{name: "unknown recordType", line: `{"recordType":"tool_start","message":{"role":"user","content":"unknown"}}`},
		{name: "unknown dual agreement", line: `{"type":"tool_end","recordType":"tool_end","message":{"role":"user","content":"unknown"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := (&PiReader{}).Parse(writePiFixture(t, tt.line+"\n"), input_config.InputDefinition{})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(parsed.Records) != tt.wantLen {
				t.Fatalf("records = %+v, want length %d", parsed.Records, tt.wantLen)
			}
		})
	}
}

func TestPiReader_RecordTypeSkipsMalformedAndUnknownNeighbors(t *testing.T) {
	lines := "{not-json}\n" +
		`{"recordType":"tool_start","data":{"secret":"must not be indexed"}}` + "\n" +
		`{"recordType":"message","message":{"role":"user","content":"searchable Pion neighbor"}}` + "\n"
	parsed, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(parsed.Records) != 1 || parsed.Records[0].Content != "searchable Pion neighbor" {
		t.Fatalf("records = %+v, want only the supported neighbor", parsed.Records)
	}
}

func TestPiReader_CapturesToolCall(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"text","text":"searching"},{"type":"toolCall","name":"web_search","arguments":{"queries":["pizzqx_query"]}}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotText, gotTool bool
	for _, m := range pf.Records {
		if m.ContentType == "text" && m.Content == "searching" {
			gotText = true
		}
		if m.ContentType == "tool" && contains(m.Content, "web_search") && contains(m.Content, "pizzqx_query") {
			gotTool = true
		}
	}
	if !gotText {
		t.Error("missing text message")
	}
	if !gotTool {
		t.Error("missing toolCall message")
	}
}

func TestPiReader_SkipsNonMessageNonCustomTypes(t *testing.T) {
	lines := `{"type":"session","timestamp":"2026-05-10T22:19:34.694Z"}` + "\n" +
		`{"type":"model_change","timestamp":"2026-05-10T22:19:34.694Z"}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("records = %d, want 0", len(pf.Records))
	}
}

func TestPiReader_MarksDirectBashSearchCalls(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"bash","arguments":{"command":"backscroll search --text orchard"}},{"type":"toolCall","name":"bash","arguments":{"command":"rg orchard"}}]}}` + "\n" +
		`{"type":"message","timestamp":"2026-05-10T22:19:35.694Z","message":{"role":"toolResult","content":[{"type":"text","text":"result_0_snippet=orchard"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	var sawSearch, sawRg, sawResult bool
	for _, m := range pf.Records {
		switch {
		case m.SearchEcho && contains(m.Content, "backscroll search"):
			sawSearch = true
		case m.ContentType == "tool" && contains(m.Content, "rg orchard"):
			sawRg = true
			if m.SearchEcho {
				t.Fatal("rg call marked as search echo")
			}
		case contains(m.Content, "result_0_snippet"):
			sawResult = true
		}
	}
	if !sawSearch {
		t.Fatalf("direct bash search was not marked; records=%+v", pf.Records)
	}
	if !sawRg {
		t.Fatal("missing rg toolCall")
	}
	if sawResult {
		t.Fatal("toolResult rows must stay unindexed")
	}
}

func TestPiReader_CapturesCustomResult(t *testing.T) {
	lines := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"web_search","arguments":{"queries":["q"]}}]}}` + "\n" +
		`{"type":"custom","customType":"web-search-results","timestamp":"2026-05-10T22:19:44.292Z","data":{"queries":[{"query":"q","answer":"pizzqx_answer_token"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotResult bool
	for _, m := range pf.Records {
		if m.ContentType == "tool" && contains(m.Content, "pizzqx_answer_token") && contains(m.Content, "web-search-results") {
			gotResult = true
		}
	}
	if !gotResult {
		t.Errorf("custom result not captured; records = %+v", pf.Records)
	}
}

func TestPiReader_SkipsEmptyCustomData(t *testing.T) {
	line := `{"type":"custom","customType":"x","timestamp":"2026-05-10T22:19:44.292Z","data":{}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("empty custom data should yield no message; got %+v", pf.Records)
	}
}

func TestPiReader_CapturesReasoningWhenEnabled(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"let me analyze this problem"},{"type":"text","text":"here is the solution"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotReasoning, gotText bool
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" && contains(m.Content, "analyze") {
			gotReasoning = true
		}
		if m.ContentType == "text" && contains(m.Content, "solution") {
			gotText = true
		}
	}
	if !gotReasoning {
		t.Error("reasoning block not captured when index_reasoning=true")
	}
	if !gotText {
		t.Error("text block missing")
	}
}

func TestPiReader_SkipsReasoningWhenDisabled(t *testing.T) {
	line := `{"recordType":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"internal reasoning"},{"type":"text","text":"visible text"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotReasoning bool
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" {
			gotReasoning = true
		}
	}
	if gotReasoning {
		t.Error("reasoning block captured when index_reasoning=false (should be skipped)")
	}
}

func TestPiReader_CapturesMessageOrigins(t *testing.T) {
	lines := `{"type":"message","message":{"role":"user","content":"human origin token"}}` + "\n" +
		`{"recordType":"message","message":{"role":"assistant","content":[{"type":"text","text":"assistant text token"},{"type":"toolCall","name":"assistant_tool","arguments":{"value":"assistant tool token"}},{"type":"thinking","text":"assistant thinking token"}]}}` + "\n" +
		`{"type":"message","message":{"role":"user","content":[{"type":"toolCall","name":"user_supplied_tool","arguments":{"value":"user tool token"}}]}}` + "\n" +
		`{"type":"custom","customType":"result","data":{"value":"automation origin token"}}` + "\n" +
		`{"recordType":"custom","customType":"result","data":{"value":"unsupported Pion custom"}}` + "\n" +
		`{"recordType":"tool_start","message":{"role":"user","content":"unsupported tool start"}}` + "\n" +
		`{"recordType":"tool_end","message":{"role":"assistant","content":"unsupported tool end"}}` + "\n" +
		`{"recordType":"text","message":{"role":"user","content":"unsupported text envelope"}}` + "\n"

	parsed, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{IndexReasoning: true},
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(parsed.Records) != 6 {
		t.Fatalf("records = %+v, want six supported records", parsed.Records)
	}

	for _, record := range parsed.Records {
		var want models.MessageOrigin
		switch {
		case contains(record.Content, "human origin token"):
			want = models.OriginHuman
		case contains(record.Content, "assistant text token"),
			contains(record.Content, "assistant tool token"),
			contains(record.Content, "assistant thinking token"),
			contains(record.Content, "user tool token"):
			want = models.OriginAssistant
		case contains(record.Content, "automation origin token"):
			want = models.OriginAutomation
		default:
			t.Fatalf("unexpected record from unsupported envelope: %+v", record)
		}
		if record.Origin != want {
			t.Errorf("origin for %q = %q, want %q", record.Content, record.Origin, want)
		}
	}
}

func TestPiReader_SkipsEmptyReasoning(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":""},{"type":"text","text":"ok"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" {
			t.Error("empty reasoning block should not create a message")
		}
	}
}
