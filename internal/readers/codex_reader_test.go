package readers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func codexTestFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexReaderFixture(t *testing.T) {
	r := &CodexReader{}
	path := filepath.Join("..", "..", "tests", "fixtures", "codex-rollout-v1.jsonl")
	def := input_config.InputDefinition{Decode: input_config.DecodeConfig{IndexReasoning: true}}
	parsed, err := r.Parse(path, def)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Records) != 8 {
		t.Fatalf("records: %+v", parsed.Records)
	}
	if parsed.Cwd != "/synthetic/codex-fixture-project" || parsed.Path != path {
		t.Fatalf("metadata: %+v", parsed)
	}
	hash, err := r.Hash(path)
	if err != nil || hash != parsed.Hash || len(hash) != 64 {
		t.Fatalf("hash %q %v", hash, err)
	}
	again, err := r.Parse(path, def)
	if err != nil || !reflect.DeepEqual(parsed, again) {
		t.Fatalf("nondeterministic parse: %v", err)
	}
	if got := parsed.Records[1]; got.Content != "exec_command cmd=echo codextoolquartz" || got.Role != "assistant" || got.ContentType != "tool" {
		t.Fatalf("call: %+v", got)
	}
	if got := parsed.Records[4]; got.Content != "codexblockquartz" || got.Role != "tool" {
		t.Fatalf("output: %+v", got)
	}
	if got := parsed.Records[5]; got.Content != "codexreasonquartz" || got.Role != "reasoning" {
		t.Fatalf("reasoning: %+v", got)
	}
	if got := parsed.Records[0].Timestamp; !got.Equal(time.Date(2026, 9, 1, 12, 0, 2, 0, time.UTC)) {
		t.Fatalf("timestamp %s", got)
	}
	reg := NewRegistry()
	reg.Register(r)
	reader, err := reg.ForDef(input_config.InputDefinition{Decode: input_config.DecodeConfig{Format: "codex"}})
	if err != nil || reader != r {
		t.Fatalf("registry: %v %v", reader, err)
	}
}

func TestCodexReaderMalformedAndVariantRecords(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"plain user", `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`, "hello"},
		{"mixed blocks", `{"type":"message","role":"assistant","content":[null,7,{"type":"output_text","text":9},{"type":"input_image","text":"secret"},{"type":"output_text","text":"safe"}]}`, "safe"},
		{"function empty args", `{"type":"function_call","name":"noop","arguments":"{}"}`, "noop"},
		{"function invalid JSON", `{"type":"function_call","name":"exec","arguments":"{"}`, ""},
		{"function wrong argument type", `{"type":"function_call","name":"exec","arguments":{"cmd":"secret"}}`, ""},
		{"function null args", `{"type":"function_call","name":"exec","arguments":"null"}`, ""},
		{"function array args", `{"type":"function_call","name":"exec","arguments":"[]"}`, ""},
		{"function missing name", `{"type":"function_call","arguments":"{}"}`, ""},
		{"custom text", `{"type":"custom_tool_call","name":"apply_patch","input":"synthetic diff"}`, "apply_patch synthetic diff"},
		{"custom empty", `{"type":"custom_tool_call","name":"apply_patch","input":" "}`, ""},
		{"custom missing name", `{"type":"custom_tool_call","input":"secret"}`, ""},
		{"output text", `{"type":"function_call_output","output":" result "}`, "result"},
		{"output array", `{"type":"function_call_output","output":[{"type":"input_text","text":"first"},{"type":"input_text","text":"second"}]}`, "first\nsecond"},
		{"custom output text", `{"type":"custom_tool_call_output","output":"custom result"}`, "custom result"},
		{"output object", `{"type":"function_call_output","output":{"text":"secret"}}`, ""},
		{"output image only", `{"type":"custom_tool_call_output","output":[{"type":"input_image","image_url":"secret"}]}`, ""},
		{"output null", `{"type":"function_call_output","output":null}`, ""},
		{"output missing", `{"type":"function_call_output"}`, ""},
		{"output wrong scalar", `{"type":"function_call_output","output":42}`, ""},
		{"encrypted only", `{"type":"reasoning","encrypted_content":"secret"}`, ""},
		{"plaintext reasoning", `{"type":"reasoning","content":[{"type":"reasoning_text","text":"plain"},{"type":"text","text":"reason"}]}`, "plain reason"},
		{"wrong role", `{"type":"message","role":"system","content":[{"type":"input_text","text":"secret"}]}`, ""},
		{"empty text", `{"type":"message","role":"user","content":[{"type":"input_text","text":" "}]}`, ""},
		{"unknown type", `{"type":"tool_search_output","tools":[{"text":"secret"}]}`, ""},
		{"null payload", `null`, ""},
		{"array payload", `[]`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := codexTestFile(t, fmt.Sprintf(`{"type":"response_item","timestamp":"2026-09-01T12:00:00.123456789Z","payload":%s}`, tc.payload))
			parsed, err := (&CodexReader{}).Parse(path, input_config.InputDefinition{Decode: input_config.DecodeConfig{IndexReasoning: true}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(parsed.Records) != 0 {
					t.Fatalf("malformed/unsupported content leaked: %+v", parsed.Records)
				}
				return
			}
			if len(parsed.Records) != 1 || parsed.Records[0].Content != tc.want {
				t.Fatalf("records: %+v want %q", parsed.Records, tc.want)
			}
			if parsed.Records[0].Timestamp.Nanosecond() != 123456789 {
				t.Fatal("lost timestamp precision")
			}
		})
	}
}

func TestCodexReaderMetadataAndTimestampSafety(t *testing.T) {
	path := codexTestFile(t, `{"type":"session_meta","payload":{"cwd":42}}
{"type":"session_meta","payload":{"cwd":"/first"}}
{"type":"session_meta","payload":{"cwd":"/second"}}
{"type":"turn_context","payload":{"cwd":"/ignored"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"missing timestamp"}]}}
{"type":"response_item","timestamp":"bad","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"bad timestamp"}]}}
{"type":"response_item","timestamp":42,"payload":{}}
null
[]
{"truncated":
`)
	parsed, err := (&CodexReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Cwd != "/first" || len(parsed.Records) != 0 {
		t.Fatalf("unsafe parse: %+v", parsed)
	}
}

func TestCodexReaderToolTruncation(t *testing.T) {
	item := codexItem{Type: "custom_tool_call", Name: "patch", Input: strings.Repeat("界", MaxToolTextLen+1)}
	msg, ok := codexMessage(item, time.Time{}, false)
	if !ok || len([]rune(msg.Content)) != MaxToolTextLen {
		t.Fatalf("cap: %d", len([]rune(msg.Content)))
	}
}

func TestCodexReaderFileErrorsAndDiscovery(t *testing.T) {
	r := &CodexReader{}
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	if _, err := r.Parse(missing, input_config.InputDefinition{}); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := r.Hash(missing); err == nil {
		t.Fatal("missing hash accepted")
	}
	if _, err := r.Parse(t.TempDir(), input_config.InputDefinition{}); err == nil {
		t.Fatal("directory accepted")
	}
	path := codexTestFile(t, "")
	def := input_config.InputDefinition{Discover: input_config.DiscoverConfig{Roots: []string{filepath.Dir(path)}, Include: []string{"**/*.jsonl"}}}
	files, err := r.Discover(def)
	if err != nil || len(files) != 1 || files[0] != path {
		t.Fatalf("discover: %v %v", files, err)
	}
	def.Discover.Exclude = []string{"**/*.jsonl"}
	files, err = r.Discover(def)
	if err != nil || len(files) != 0 {
		t.Fatalf("exclude: %v %v", files, err)
	}
}

func FuzzCodexReaderRecord(f *testing.F) {
	f.Add([]byte(`{"type":"message","role":"user","content":[{"type":"input_text","text":"seed"}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Exercise the same typed item decoder and selector without filesystem churn.
		var item codexItem
		if json.Unmarshal(data, &item) == nil {
			_, _ = codexMessage(item, time.Time{}, true)
		}
	})
}
