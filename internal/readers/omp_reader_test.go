package readers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

func parseOMPTest(t *testing.T, body string, reasoning bool) (models.ParsedFile, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return (&OMPReader{}).Parse(context.Background(), path, input_config.InputDefinition{Decode: input_config.DecodeConfig{IndexReasoning: reasoning}})
}

func TestOMPReaderNativeV3(t *testing.T) {
	raw, err := os.ReadFile("../../tests/fixtures/omp-v3.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, reasoning := range []bool{false, true} {
		parsed, err := parseOMPTest(t, string(raw), reasoning)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Cwd != "/workspace/omp-fixture-project" || parsed.Hash == "" {
			t.Fatalf("metadata: %+v", parsed)
		}
		want := []struct {
			marker, role, kind string
			origin             models.MessageOrigin
		}{
			{"ompuserquartz", "user", "text", models.OriginHuman},
			{"ompoutputquartz", "tool", "tool", models.OriginAutomation},
			{"ompanswerquartz", "assistant", "text", models.OriginAssistant},
			{"omptoolquartz", "assistant", "tool", models.OriginAssistant},
			{"ompdeveloperquartz", "developer", "text", models.OriginSystem},
			{"ompsyntheticquartz", "user", "text", models.OriginAutomation},
			{"ompagentquartz", "user", "text", models.OriginAutomation},
			{"ompunknownquartz", "user", "text", models.OriginUnknown},
			{"ompbranchquartz", "assistant", "text", models.OriginAssistant},
		}
		if reasoning {
			want = append(want[:3], append([]struct {
				marker, role, kind string
				origin             models.MessageOrigin
			}{{"ompreasonquartz", "reasoning", "reasoning", models.OriginAssistant}}, want[3:]...)...)
		}
		if len(parsed.Records) != len(want) {
			t.Fatalf("reasoning=%t got %+v want %d", reasoning, parsed.Records, len(want))
		}
		seen := map[string]bool{}
		for i, msg := range parsed.Records {
			w := want[i]
			if !strings.Contains(msg.Content, w.marker) || msg.Role != w.role || msg.ContentType != w.kind || msg.Origin != w.origin {
				t.Fatalf("row %d: %+v want %+v", i, msg, w)
			}
			if !strings.HasPrefix(msg.UUID, "omp:omp-fixture-session:") || seen[msg.UUID] {
				t.Fatalf("identity: %+v", msg)
			}
			seen[msg.UUID] = true
			if strings.Contains(msg.Content, "ompignored") {
				t.Fatalf("privacy leak: %+v", msg)
			}
			if msg.ToolName == "bash" {
				if msg.CommandHead != "echo" || msg.IsError == nil || !*msg.IsError || msg.ExitCode == nil || *msg.ExitCode != 7 || !msg.WasInterrupted {
					t.Fatalf("paired native metadata: %+v", msg)
				}
			}
			if msg.Role == "assistant" && strings.Contains(msg.Content, "ompbranchquartz") && msg.WasInterrupted {
				t.Fatal("aborted stopReason is not interruption")
			}
		}
	}
}

func TestOMPReaderStrictBoundary(t *testing.T) {
	const header = `{"type":"session","version":3,"id":"s","cwd":"/workspace/project","timestamp":"2026-09-01T12:00:00Z"}` + "\n"
	const good = `{"type":"message","id":"m","timestamp":"2026-09-01T12:00:00Z","message":{"role":"user","content":"survivor","attribution":"user"}}` + "\n"
	for _, body := range []string{good, strings.Replace(header, `"version":3`, `"version":2`, 1) + good, strings.Replace(header, `"id":"s"`, `"id":""`, 1) + good, header + header + good} {
		if _, err := parseOMPTest(t, body, false); err == nil {
			t.Fatalf("unsupported header accepted: %s", body)
		}
	}
	parsed, err := parseOMPTest(t, header+"{bad\n"+`{"type":"message","timestamp":"bad","id":"invalid","message":{"role":"user","content":"leak"}}`+"\n"+good, false)
	if err != nil || len(parsed.Records) != 1 || parsed.Records[0].UUID != "omp:s:m#b0" {
		t.Fatalf("neighbor preservation: %+v %v", parsed, err)
	}
	other, err := parseOMPTest(t, strings.Replace(header, `"id":"s"`, `"id":"other"`, 1)+good, false)
	if err != nil || other.Records[0].UUID == parsed.Records[0].UUID {
		t.Fatal("session-scoped entry IDs collided")
	}
}

func TestOMPReaderRawMetadataAndEcho(t *testing.T) {
	body := `{"type":"session","version":3,"id":"s","cwd":"/workspace/project","timestamp":"2026-09-01T12:00:00Z"}` + "\n" +
		`{"type":"message","id":"r","timestamp":"2026-09-01T12:00:00Z","message":{"role":"toolResult","toolCallId":"call","toolName":"bash","isError":false,"details":{"exitCode":9,"__interrupted":true,"secret":"mustnotleak"},"content":[{"type":"text","text":"` + strings.Repeat("界", 5000) + `"},{"type":"image","text":"mustnotleak"}]}}` + "\n" +
		`{"type":"message","id":"c","timestamp":"2026-09-01T12:00:01Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call","name":"bash","arguments":{"command":"backscroll search quartz"}}]}}` + "\n"
	parsed, err := parseOMPTest(t, body, false)
	if err != nil || len(parsed.Records) != 2 {
		t.Fatalf("parse: %+v %v", parsed, err)
	}
	for _, m := range parsed.Records {
		if !m.SearchEcho || m.ExitCode == nil || *m.ExitCode != 9 || !m.WasInterrupted || m.IsError == nil || *m.IsError || strings.Contains(m.Content, "mustnotleak") {
			t.Fatalf("metadata lost: %+v", m)
		}
	}
	if len([]rune(parsed.Records[0].Content)) != MaxToolTextLen {
		t.Fatal("output cap")
	}
	// Textual exit/interruption markers and non-Bash details never prove an exit code.
	body = strings.ReplaceAll(body, `"bash"`, `"read"`)
	parsed, err = parseOMPTest(t, body, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range parsed.Records {
		if m.ExitCode != nil || m.SearchEcho {
			t.Fatalf("non-Bash signal: %+v", m)
		}
	}
}

func TestOMPReaderContextAndMissingRoot(t *testing.T) {
	r := &OMPReader{}
	if r.Name() != "omp" {
		t.Fatal(r.Name())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Parse(ctx, "missing", input_config.InputDefinition{}); err == nil {
		t.Fatal("cancellation")
	}
	if _, err := r.Hash(ctx, "missing"); err == nil {
		t.Fatal("hash cancellation")
	}
	if _, err := r.Discover(ctx, input_config.InputDefinition{}); err == nil {
		t.Fatal("discovery cancellation")
	}
	def := input_config.InputDefinition{Discover: input_config.DiscoverConfig{Roots: []string{filepath.Join(t.TempDir(), "absent")}, Include: []string{"*/*.jsonl"}}}
	if paths, err := r.Discover(context.Background(), def); err != nil || len(paths) != 0 {
		t.Fatalf("missing root: %v %v", paths, err)
	}
}

func TestOMPReaderContentAllowlistAndStableBlocks(t *testing.T) {
	const header = `{"type":"session","version":3,"id":"s","cwd":"/workspace/project","timestamp":"2026-09-01T12:00:00Z"}` + "\n"
	const entries = `{"type":"message","id":"unknown","timestamp":"2026-09-01T12:00:00Z","message":{"role":"user","attribution":"future","content":[{"type":"text","text":"unknown actor"},{"type":"thinking","thinking":"ignored"},{"type":"toolCall","id":"bad","name":"bash","arguments":{"command":"ignored"}}]}}` + "\n" +
		`{"type":"message","id":"a","timestamp":"2026-09-01T12:00:00Z","message":{"role":"assistant","stopReason":"aborted","content":[{"type":"image","text":"ignored"},{"type":"thinking","thinking":"reasoning"},{"type":"text","text":"answer"},{"type":"toolCall","id":"bad","name":"bash","arguments":"ignored"},{"type":"toolCall","id":"null","name":"bash","arguments":null},{"type":"toolCall","name":"bash","arguments":{}},{"type":"toolCall","id":"noname","arguments":{}},{"type":"future","text":"ignored"}]}}` + "\n" +
		`{"type":"message","id":"r","timestamp":"2026-09-01T12:00:00Z","message":{"role":"toolResult","toolName":"bash","toolCallId":"unmatched","details":{"exitCode":"2","__interrupted":false},"content":[{"type":"text","text":"exit code 2 Request interrupted"},{"type":"text","text":"second block"}]}}` + "\n" +
		`{"type":"message","id":"opaque","timestamp":"2026-09-01T12:00:00Z","message":{"role":"assistant","content":{"text":"ignored"}}}` + "\n" +
		`{"type":"message","id":"plain-result","timestamp":"2026-09-01T12:00:00Z","message":{"role":"toolResult","content":"ignored"}}` + "\n"
	off, err := parseOMPTest(t, header+entries, false)
	if err != nil {
		t.Fatal(err)
	}
	on, err := parseOMPTest(t, header+entries, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(off.Records) != 4 || len(on.Records) != 5 {
		t.Fatalf("allowlist: off=%+v on=%+v", off.Records, on.Records)
	}
	if off.Records[0].Origin != models.OriginUnknown || off.Records[1].UUID != "omp:s:a#b2" || on.Records[2].UUID != off.Records[1].UUID {
		t.Fatal("origin or physical block identity")
	}
	for _, m := range on.Records {
		if strings.Contains(m.Content, "ignored") || m.WasInterrupted || m.ExitCode != nil || m.SearchEcho || m.ToolName != "" || m.IsError != nil {
			t.Fatalf("unproven metadata/content: %+v", m)
		}
	}
	// With a valid file the Hash port agrees exactly with Parse's content hash.
	path := filepath.Join(t.TempDir(), "native.jsonl")
	if err := os.WriteFile(path, []byte(header+entries), 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := (&OMPReader{}).Hash(context.Background(), path)
	if err != nil || hash != off.Hash {
		t.Fatalf("hash=%s parsed=%s err=%v", hash, off.Hash, err)
	}
}

func TestOMPReaderClassifiesFencedCode(t *testing.T) {
	const header = `{"type":"session","version":3,"id":"s","cwd":"/workspace/project","timestamp":"2026-09-01T12:00:00Z"}` + "\n"
	for _, content := range []string{`"before\n` + "```go\\nfmt.Println(1)\\n```" + `"`, `[{"type":"text","text":"before\n` + "```go\\nfmt.Println(1)\\n```" + `"}]`} {
		body := header + `{"type":"message","id":"code","timestamp":"2026-09-01T12:00:00Z","message":{"role":"assistant","content":` + content + `}}` + "\n"
		parsed, err := parseOMPTest(t, body, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Records) != 1 || parsed.Records[0].ContentType != "code" {
			t.Fatalf("fenced code classification: %+v", parsed.Records)
		}
	}
}
