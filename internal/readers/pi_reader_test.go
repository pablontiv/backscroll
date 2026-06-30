package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
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
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
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

func TestPiReader_CapturesToolCall(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"text","text":"searching"},{"type":"toolCall","name":"web_search","arguments":{"queries":["pizzqx_query"]}}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
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
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("records = %d, want 0", len(pf.Records))
	}
}

func TestPiReader_CapturesCustomResult(t *testing.T) {
	lines := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"web_search","arguments":{"queries":["q"]}}]}}` + "\n" +
		`{"type":"custom","customType":"web-search-results","timestamp":"2026-05-10T22:19:44.292Z","data":{"queries":[{"query":"q","answer":"pizzqx_answer_token"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{})
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
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("empty custom data should yield no message; got %+v", pf.Records)
	}
}
