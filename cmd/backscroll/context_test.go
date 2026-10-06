package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	picokitoutput "github.com/pablontiv/picokit/output"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestContextFormatsGoldenAndStructuralFields(t *testing.T) {
	records := []storage.ContextRecord{{
		UUID:        nil,
		SourcePath:  "/tmp/chat\none",
		Ordinal:     7,
		Role:        "assistant",
		Origin:      models.OriginAutomation,
		Timestamp:   nil,
		ContentType: "tool",
		Source:      "session",
		Text:        "hola 🌍\nquote \" slash \\ tab\t",
		Anchor:      true,
	}}

	jsonPayload, fits, err := contextPayloadWithinBudget(records, contextJSONFormat, contextMaxMaxTokens)
	if err != nil || !fits {
		t.Fatalf("JSON payload: fits=%t err=%v", fits, err)
	}
	wantJSON := `{"anchor":{"uuid":null,"source_path":"/tmp/chat\none","ordinal":7},"records":[{"uuid":null,"source_path":"/tmp/chat\none","ordinal":7,"role":"assistant","origin":"automation","timestamp":null,"content_type":"tool","source":"session","text":"hola 🌍\nquote \" slash \\ tab\t","is_anchor":true}],"truncated":false,"omitted":0}
`
	if got := string(jsonPayload); got != wantJSON {
		t.Fatalf("JSON golden mismatch\n got: %s\nwant: %s", got, wantJSON)
	}
	var envelope contextEnvelope
	if err := json.Unmarshal(jsonPayload, &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Anchor.UUID != nil || envelope.Records[0].Timestamp != nil {
		t.Fatalf("nullable fields lost: %+v", envelope)
	}
	if got := envelope.Records[0]; got.Origin != "automation" || got.Source != "session" || !got.IsAnchor {
		t.Fatalf("provenance/anchor fields = %+v", got)
	}

	robotPayload, fits, err := contextPayloadWithinBudget(records, contextRobotFormat, contextMaxMaxTokens)
	if err != nil || !fits {
		t.Fatalf("robot payload: fits=%t err=%v", fits, err)
	}
	wantRobot := "" +
		"anchor_uuid=null\n" +
		"anchor_source_path=/tmp/chat\\none\n" +
		"anchor_ordinal=7\n" +
		"records=1\n" +
		"truncated=false\n" +
		"omitted=0\n" +
		"record_0_uuid=null\n" +
		"record_0_source_path=/tmp/chat\\none\n" +
		"record_0_ordinal=7\n" +
		"record_0_role=assistant\n" +
		"record_0_origin=automation\n" +
		"record_0_timestamp=null\n" +
		"record_0_content_type=tool\n" +
		"record_0_source=session\n" +
		"record_0_text=hola 🌍\\nquote \\\" slash \\\\ tab\\t\n" +
		"record_0_is_anchor=true\n"
	if got := string(robotPayload); got != wantRobot {
		t.Fatalf("robot golden mismatch\n got: %q\nwant: %q", got, wantRobot)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(robotPayload), "\n"), "\n") {
		if !strings.Contains(line, "=") {
			t.Fatalf("robot line is not one key/value: %q", line)
		}
	}

	textPayload, fits, err := contextPayloadWithinBudget(records, contextTextFormat, contextMaxMaxTokens)
	if err != nil || !fits {
		t.Fatalf("text payload: fits=%t err=%v", fits, err)
	}
	wantText := "" +
		"Context anchor: uuid=null source_path=\"/tmp/chat\\none\" ordinal=7\n" +
		"Records: 1 | truncated=false | omitted=0\n" +
		"\nRecord 0\n" +
		"  uuid: null\n" +
		"  source_path: \"/tmp/chat\\none\"\n" +
		"  ordinal: 7\n" +
		"  role: \"assistant\"\n" +
		"  origin: \"automation\"\n" +
		"  timestamp: null\n" +
		"  content_type: \"tool\"\n" +
		"  source: \"session\"\n" +
		"  text: \"hola 🌍\\nquote \\\" slash \\\\ tab\\t\"\n" +
		"  is_anchor: true\n"
	if got := string(textPayload); got != wantText {
		t.Fatalf("text golden mismatch\n got: %q\nwant: %q", got, wantText)
	}
}

func TestContextTextLimitUsesUnicodeCodePoints(t *testing.T) {
	input := strings.Repeat("界", contextMaxTextRunes) + "🙂"
	output := contextOutputRecord(storage.ContextRecord{Text: input})
	if got := len([]rune(output.Text)); got != contextMaxTextRunes {
		t.Fatalf("output text code points = %d, want %d", got, contextMaxTextRunes)
	}
	if !strings.HasSuffix(output.Text, "界") || strings.Contains(output.Text, "🙂") {
		t.Fatalf("Unicode truncation boundary is wrong")
	}

	exact := strings.Repeat("e\u0301", contextMaxTextRunes/2)
	if got := contextOutputRecord(storage.ContextRecord{Text: exact}).Text; got != exact {
		t.Fatalf("exactly %d code points were truncated", contextMaxTextRunes)
	}
}

func TestContextBudgetRemovesWholeEdgeRecordsDeterministically(t *testing.T) {
	records := make([]storage.ContextRecord, 5)
	for i := range records {
		uuid := fmt.Sprintf("uuid-%d", i)
		records[i] = storage.ContextRecord{
			UUID:        &uuid,
			SourcePath:  "/session/budget.jsonl",
			Ordinal:     int64(i),
			Role:        "assistant",
			Origin:      models.OriginAssistant,
			ContentType: "text",
			Source:      "session",
			Text:        strings.Repeat(fmt.Sprintf("record-%d ", i), 30),
			Anchor:      i == 2,
		}
	}

	normalized := make([]contextRecordOutput, len(records))
	for i, record := range records {
		normalized[i] = contextOutputRecord(record)
	}
	desired, err := renderContextPayload(contextEnvelope{
		Anchor: contextAnchorOutput{
			UUID:       normalized[2].UUID,
			SourcePath: normalized[2].SourcePath,
			Ordinal:    normalized[2].Ordinal,
		},
		Records: normalized[1:4], Truncated: true, Omitted: 2,
	}, contextJSONFormat)
	if err != nil {
		t.Fatal(err)
	}
	budget := picokitoutput.TokenCount(string(desired))

	payload, fits, err := contextPayloadWithinBudget(records, contextJSONFormat, budget)
	if err != nil || !fits {
		t.Fatalf("budgeted payload: fits=%t err=%v", fits, err)
	}
	if got := picokitoutput.TokenCount(string(payload)); got > budget {
		t.Fatalf("payload tokens = %d, budget = %d", got, budget)
	}
	var got contextEnvelope
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !got.Truncated || got.Omitted != 2 || len(got.Records) != 3 {
		t.Fatalf("truncation metadata = truncated %t omitted %d records %d", got.Truncated, got.Omitted, len(got.Records))
	}
	for i, wantOrdinal := range []int64{1, 2, 3} {
		if got.Records[i].Ordinal != wantOrdinal || got.Records[i].Text != records[wantOrdinal].Text {
			t.Fatalf("record %d was partial or non-deterministic: %+v", i, got.Records[i])
		}
	}
	if !got.Records[1].IsAnchor {
		t.Fatalf("anchor was not preserved: %+v", got.Records)
	}

	full, err := renderContextPayload(contextEnvelope{
		Anchor:  contextAnchorOutput{UUID: normalized[2].UUID, SourcePath: normalized[2].SourcePath, Ordinal: 2},
		Records: normalized,
	}, contextJSONFormat)
	if err != nil {
		t.Fatal(err)
	}
	exactBudget := picokitoutput.TokenCount(string(full))
	exactPayload, fits, err := contextPayloadWithinBudget(records, contextJSONFormat, exactBudget)
	if err != nil || !fits || string(exactPayload) != string(full) {
		t.Fatalf("exact token limit did not retain complete payload: fits=%t err=%v", fits, err)
	}
}

func TestContextValidationLimitsAndSelectors(t *testing.T) {
	validUUID := contextCommandOptions{uuid: "opaque", uuidSet: true, before: 0, after: 50, maxTokens: 64}
	if err := validateContextRequest(validUUID); err != nil {
		t.Fatalf("minimum/exact limits rejected: %v", err)
	}
	validPath := contextCommandOptions{sourcePath: "/session", sourcePathSet: true, ordinalSet: true, ordinal: -9, before: 50, after: 0, maxTokens: 16384, robotFormat: true}
	if err := validateContextRequest(validPath); err != nil {
		t.Fatalf("path/ordinal and maximum limits rejected: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no selector", want: "exactly one anchor selector"},
		{name: "positional", args: []string{"extra", "--uuid", "x"}, want: "unknown command"},
		{name: "both selectors", args: []string{"--uuid", "x", "--source-path", "/x", "--ordinal", "1"}, want: "exactly one anchor selector"},
		{name: "source without ordinal", args: []string{"--source-path", "/x"}, want: "exactly one anchor selector"},
		{name: "ordinal without source", args: []string{"--ordinal", "1"}, want: "exactly one anchor selector"},
		{name: "empty uuid", args: []string{"--uuid="}, want: "--uuid must not be empty"},
		{name: "empty path", args: []string{"--source-path=", "--ordinal", "1"}, want: "--source-path must not be empty"},
		{name: "before low", args: []string{"--uuid", "x", "--before", "-1"}, want: "--before must be between"},
		{name: "before high", args: []string{"--uuid", "x", "--before", "51"}, want: "--before must be between"},
		{name: "after low", args: []string{"--uuid", "x", "--after", "-1"}, want: "--after must be between"},
		{name: "after high", args: []string{"--uuid", "x", "--after", "51"}, want: "--after must be between"},
		{name: "tokens low", args: []string{"--uuid", "x", "--max-tokens", "63"}, want: "--max-tokens must be between"},
		{name: "tokens high", args: []string{"--uuid", "x", "--max-tokens", "16385"}, want: "--max-tokens must be between"},
		{name: "formats", args: []string{"--uuid", "x", "--json", "--robot"}, want: "mutually exclusive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newContextCmd(&bytes.Buffer{}, &bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "startup configuration") || strings.Contains(err.Error(), "database") {
				t.Fatalf("validation reached startup/database: %v", err)
			}
		})
	}

	cmd := newContextCmd(&bytes.Buffer{}, &bytes.Buffer{})
	for name, want := range map[string]string{"before": "5", "after": "5", "max-tokens": "2000"} {
		if got := cmd.Flags().Lookup(name).DefValue; got != want {
			t.Fatalf("--%s default = %q, want %q", name, got, want)
		}
	}
}

func TestContextCommandUsesIndexedAPIAndEmitsDiagnostics(t *testing.T) {
	cfg := newContextTestIndex(t)

	t.Run("successful JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			uuid: "anchor", uuidSet: true, before: 1, after: 1, maxTokens: contextMaxMaxTokens, jsonFormat: true,
		})
		if err != nil {
			t.Fatalf("runContext: %v", err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q", stderr.String())
		}
		var envelope contextEnvelope
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if len(envelope.Records) != 3 || envelope.Anchor.UUID == nil || *envelope.Anchor.UUID != "anchor" {
			t.Fatalf("context envelope = %+v", envelope)
		}
		if !envelope.Records[1].IsAnchor || envelope.Records[1].Origin != "assistant" || envelope.Records[1].Source != "session" {
			t.Fatalf("anchor record = %+v", envelope.Records[1])
		}
	})

	t.Run("not found JSON is stdout-only", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		missing := strings.Repeat("missing value ", 80)
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			uuid: missing, uuidSet: true, before: 5, after: 5, maxTokens: 64, jsonFormat: true,
		})
		assertContextDiagnostic(t, err, stdout.String(), stderr.String(), "context_not_found", true)
		if tokens := picokitoutput.TokenCount(stdout.String()); tokens <= 64 {
			t.Fatalf("diagnostic fixture did not prove budget exemption: %d tokens", tokens)
		}
	})

	t.Run("ambiguous robot is stdout-only", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			sourcePath: "/duplicate", sourcePathSet: true, ordinal: 9, ordinalSet: true,
			before: 5, after: 5, maxTokens: 2000, robotFormat: true,
		})
		if err == nil || !strings.Contains(stdout.String(), "diagnostic_code=context_ambiguous\n") {
			t.Fatalf("err=%v stdout=%q", err, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("robot diagnostic stderr = %q", stderr.String())
		}
	})

	t.Run("not found text is stderr-only", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			uuid: "absent", uuidSet: true, before: 5, after: 5, maxTokens: 2000,
		})
		if err == nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "diagnostic: context_not_found:") {
			t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
	})

	t.Run("valid tiny budget gets structured diagnostic and no partial payload", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			uuid: "anchor", uuidSet: true, before: 0, after: 0, maxTokens: 64, jsonFormat: true,
		})
		assertContextDiagnostic(t, err, stdout.String(), stderr.String(), "context_budget_too_small", true)
		var diagnostic map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &diagnostic); err != nil {
			t.Fatalf("budget diagnostic is not standalone JSON: %v; %q", err, stdout.String())
		}
		if _, exists := diagnostic["records"]; exists {
			t.Fatalf("partial context payload leaked into diagnostic: %v", diagnostic)
		}
	})
}

func newContextTestIndex(t *testing.T) *config.Config {
	t.Helper()
	path := t.TempDir() + "/context.db"
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("open context fixture: %v", err)
	}
	insert := func(id, ordinal int, sourcePath, role, origin, text string, uuid any) {
		t.Helper()
		if _, err := db.DB().Exec(`
			INSERT INTO search_items
				(id, source, source_path, ordinal, role, origin, text, uuid, timestamp, content_type)
			VALUES (?, 'session', ?, ?, ?, ?, ?, ?, NULL, 'text')
		`, id, sourcePath, ordinal, role, origin, text, uuid); err != nil {
			t.Fatalf("insert context fixture %d: %v", id, err)
		}
	}
	insert(1, 1, "/session", "user", "human", "before", "before")
	insert(2, 2, "/session", "assistant", "assistant", strings.Repeat("anchor payload ", 30), "anchor")
	insert(3, 3, "/session", "user", "human", "after", nil)
	insert(4, 9, "/duplicate", "user", "human", "duplicate one", nil)
	insert(5, 9, "/duplicate", "assistant", "assistant", "duplicate two", nil)
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		t.Fatalf("checkpoint context fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close context fixture: %v", err)
	}
	return &config.Config{DatabasePath: path}
}

func assertContextDiagnostic(t *testing.T, err error, stdout, stderr, code string, machine bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("diagnostic %s returned nil error", code)
	}
	if machine && stderr != "" {
		t.Fatalf("machine diagnostic stderr = %q", stderr)
	}
	var diagnostic struct {
		Code string `json:"code"`
	}
	if decodeErr := json.Unmarshal([]byte(stdout), &diagnostic); decodeErr != nil {
		t.Fatalf("decode diagnostic: %v; stdout=%q", decodeErr, stdout)
	}
	if diagnostic.Code != code {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Code, code)
	}
}
