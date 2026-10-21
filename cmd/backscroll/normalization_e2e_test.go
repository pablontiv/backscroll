package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestFencedProviderNormalizationCLI(t *testing.T) {
	e := newQueryEchoE2E(t)
	paths := make(map[string]string)
	for _, provider := range []string{"claude", "pi", "codex", "omp", "opencode"} {
		paths[provider] = writeNormalizationFixture(t, e, provider)
	}
	code := e.searchJSON("normalization167", "code", 20)
	if len(code) != 5 {
		t.Fatalf("fenced provider results = %+v, want five", code)
	}
	for _, result := range code {
		if result.Content != "normalization167 beacon ```python value = 167 print(value) ``` finish marker" {
			t.Fatalf("normalized fenced text = %+v", result)
		}
	}
	plain := e.searchJSON("normalization167", "text", 20)
	if len(plain) != 10 {
		t.Fatalf("prose/provider-wrapper controls = %+v, want ten", plain)
	}
	assertRows := func(reasoning bool) {
		t.Helper()
		index, err := storage.OpenReadOnly(e.database)
		if err != nil {
			t.Fatal(err)
		}
		defer index.Close()
		for provider, path := range paths {
			var text, origin, evidence string
			var uuid *string
			if err := index.DB().QueryRow(`SELECT text,origin,uuid,project_evidence FROM search_items WHERE source_path=? AND text LIKE 'normalization167 beacon value%'`, path).Scan(&text, &origin, &uuid, &evidence); err != nil {
				t.Fatal(err)
			}
			if text != "normalization167 beacon value = 167 print(value) finish marker" || origin != "human" || evidence != "native_cwd" || (uuid == nil) != (provider == "codex") {
				t.Fatalf("%s prose/identity/origin/project = %q/%v/%q/%q", provider, text, uuid, origin, evidence)
			}
			var count int
			if err := index.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path=? AND content_type='reasoning'`, path).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 0
			if reasoning && provider != "claude" && provider != "opencode" {
				want = 1
			}
			if count != want {
				t.Fatalf("%s reasoning rows=%d, want %d", provider, count, want)
			}
			if err := index.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path=? AND content_type='tool' AND origin='automation' AND text=?`, path, "normalizationtool167 output\n  kept    tool  spacing").Scan(&count); err != nil || count != 1 {
				t.Fatalf("%s native tool output count=%d err=%v", provider, count, err)
			}
			if err := index.DB().QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path=? AND tool_name IN ('bash','exec_command') AND command_head='printf'`, path).Scan(&count); err != nil || count != 1 {
				t.Fatalf("%s native call count=%d err=%v", provider, count, err)
			}
		}
	}
	assertRows(false)
	for provider := range paths {
		manifest := filepath.Join(e.config, "backscroll", "inputs", "normalization-"+provider+".inputs.toml")
		data, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest, append(data, []byte("index_reasoning = true\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.run("status", "--json")
	assertRows(true)
	t.Log("actual hermetic dev CLI: all five providers classify fences, normalize prose, preserve native identity/origin/project/tool output, and replay reasoning opt-in")
}

func writeNormalizationFixture(t *testing.T, e *queryEchoE2E, provider string) string {
	t.Helper()
	root := e.addReaderManifest("normalization-"+provider, provider)
	texts := []string{
		"  normalization167 beacon\n```python\nvalue =  167\nprint(value)\n```\n  finish\tmarker  ",
		"  normalization167 beacon\nvalue =  167\nprint(value)\n  finish\tmarker  ",
		"<system-reminder>```hidden fence```</system-reminder> normalization167 wrapper\n  visible",
	}
	if provider == "opencode" {
		texts[2] = "normalization167 <system-reminder>native visible</system-reminder>\n  second    line"
	}
	const timestamp = "2026-01-01T00:00:00Z"
	const cwd = "/projects/normalization167"
	const output = "normalizationtool167 output\n  kept    tool  spacing"
	const reasoning = "private167 reasoning\n  normalized"
	if provider == "opencode" {
		path := filepath.Join(root, "opencode.db")
		db := openOpenCodeFixture(t, path)
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('normalization167',?)`, cwd); err != nil {
			t.Fatal(err)
		}
		for i, text := range texts {
			id := fmt.Sprintf("normalization-%d", i)
			if _, err := db.Exec(`INSERT INTO message VALUES(?,?,?,?,?)`, id, "normalization167", i, i, `{"role":"user"}`); err != nil {
				t.Fatal(err)
			}
			parts := []map[string]any{{"type": "text", "text": text}}
			if i == 2 {
				parts = append(parts, map[string]any{"type": "text", "text": "ignored167", "ignored": true}, map[string]any{"type": "reasoning", "text": reasoning}, map[string]any{"type": "tool", "tool": "bash", "state": map[string]any{"input": map[string]any{"command": "printf normalizationtool167"}, "output": output}})
			}
			for j, part := range parts {
				data, _ := json.Marshal(part)
				if _, err := db.Exec(`INSERT INTO part VALUES(?,?,?,?,?,?)`, fmt.Sprintf("%s-%d", id, j), id, "normalization167", i, i, string(data)); err != nil {
					t.Fatal(err)
				}
			}
		}
		return path
	}
	var records []map[string]any
	if provider == "pi" || provider == "omp" {
		records = append(records, map[string]any{"type": "session", "version": 3, "id": "native167", "cwd": cwd, "timestamp": timestamp})
	}
	if provider == "codex" {
		records = append(records, map[string]any{"type": "session_meta", "payload": map[string]any{"id": "native167", "cwd": cwd}})
	}
	for i, text := range texts {
		content := any(text)
		if i != 1 {
			content = []map[string]any{{"type": "text", "text": text}}
		}
		rec := map[string]any{"type": "message", "id": fmt.Sprintf("native-%d", i), "uuid": fmt.Sprintf("native-%d", i), "cwd": cwd, "timestamp": timestamp, "message": map[string]any{"role": "user", "attribution": "user", "content": content}}
		if provider == "claude" {
			rec["type"] = "user"
		}
		if provider == "codex" {
			// Leading injected wrappers are native structural removals, not a
			// reason to classify the remaining ordinary request as fenced code.
			if i == 2 {
				text = "<heartbeat>```native injected```</heartbeat>\n" + text
			}
			rec = map[string]any{"type": "response_item", "timestamp": timestamp, "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": text}}}}
		}
		records = append(records, rec)
	}
	toolType, toolIDKey, argsKey := "toolCall", "id", "arguments"
	if provider == "claude" {
		toolType, argsKey = "tool_use", "input"
	}
	thinking := map[string]any{"type": "thinking", "text": reasoning, "thinking": reasoning}
	call := map[string]any{"type": toolType, toolIDKey: "call167", "name": "bash", argsKey: map[string]any{"command": "printf normalizationtool167"}}
	records = append(records, map[string]any{"type": "message", "id": "call", "uuid": "call", "timestamp": timestamp, "message": map[string]any{"role": "assistant", "content": []map[string]any{call, thinking}}})
	result := map[string]any{"role": "toolResult", "toolCallId": "call167", "toolName": "bash", "content": []map[string]any{{"type": "text", "text": output}}, "isError": false}
	if provider == "claude" {
		records[len(records)-1]["type"] = "assistant"
		result = map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": "call167", "content": output}}}
	}
	records = append(records, map[string]any{"type": "message", "id": "result", "uuid": "result", "timestamp": timestamp, "message": result})
	if provider == "claude" {
		records[len(records)-1]["type"] = "user"
	}
	if provider == "codex" {
		records = records[:len(records)-2]
		for _, item := range []map[string]any{
			{"type": "function_call", "name": "exec_command", "call_id": "call167", "arguments": `{"cmd":"printf normalizationtool167"}`},
			{"type": "function_call_output", "call_id": "call167", "output": output},
			{"type": "reasoning", "summary": []map[string]any{{"type": "summary_text", "text": reasoning}}},
		} {
			records = append(records, map[string]any{"type": "response_item", "timestamp": timestamp, "payload": item})
		}
	}
	var lines []string
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenCodeNormalizationReplayPreservesIdentifiedPayloadCLI(t *testing.T) {
	e := newQueryEchoE2E(t)
	path := writeNormalizationFixture(t, e, "opencode")
	e.run("status", "--json")
	def := input_config.InputDefinition{Source: "session"}
	old := readers.IngestionFingerprint(&semanticReplayReader{name: "opencode", epoch: 2}, def)
	current := readers.IngestionFingerprint(&readers.OpenCodeReader{}, def)
	index, err := storage.Open(e.database)
	if err != nil {
		t.Fatal(err)
	}
	var uuid string
	var id, ordinal int64
	if err := index.DB().QueryRow(`SELECT id,ordinal,uuid FROM search_items WHERE content_type='code'`).Scan(&id, &ordinal, &uuid); err != nil {
		t.Fatal(err)
	}
	// Recreate an epoch-2 retained payload and an available source with a
	// previously unindexed message; source bytes and its native hash stay equal.
	const retained = "normalization167 beacon\n```python\nvalue =  167\n```"
	if _, err := index.DB().Exec(`UPDATE search_items SET text=?,content_type='text' WHERE uuid=?`, retained, uuid); err != nil {
		t.Fatal(err)
	}
	if _, err := index.DB().Exec(`DELETE FROM search_items WHERE text LIKE 'normalization167 beacon value%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := index.DB().Exec(`UPDATE indexed_files SET ingestion_fingerprint=? WHERE path=?`, old, path); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := index.DB().QueryRow(`SELECT hash FROM indexed_files WHERE path=?`, path).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	index.Close()
	e.run("status", "--json")
	index, err = storage.OpenReadOnly(e.database)
	if err != nil {
		t.Fatal(err)
	}
	var gotText, gotKind, gotFingerprint, gotHash string
	var gotID, gotOrdinal int64
	if err := index.DB().QueryRow(`SELECT id,ordinal,text,content_type FROM search_items WHERE uuid=?`, uuid).Scan(&gotID, &gotOrdinal, &gotText, &gotKind); err != nil {
		t.Fatal(err)
	}
	if gotID != id || gotOrdinal != ordinal || gotText != retained || gotKind != "text" {
		t.Fatalf("identified replay replaced retained payload: %d/%d/%q/%q", gotID, gotOrdinal, gotText, gotKind)
	}
	if err := index.DB().QueryRow(`SELECT hash,ingestion_fingerprint FROM indexed_files WHERE path=?`, path).Scan(&gotHash, &gotFingerprint); err != nil {
		t.Fatal(err)
	}
	if gotHash != hash || gotFingerprint != current || old == current {
		t.Fatalf("stable-byte replay hash/fingerprint=%q/%q", gotHash, gotFingerprint)
	}
	var restored string
	if err := index.DB().QueryRow(`SELECT text FROM search_items WHERE text LIKE 'normalization167 beacon value%'`).Scan(&restored); err != nil || restored != "normalization167 beacon value = 167 print(value) finish marker" {
		t.Fatalf("newly indexed replay payload=%q err=%v", restored, err)
	}
	index.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	index, err = storage.Open(e.database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.DB().Exec(`UPDATE indexed_files SET ingestion_fingerprint=? WHERE path=?`, old, path); err != nil {
		t.Fatal(err)
	}
	index.Close()
	e.run("status", "--json")
	index, err = storage.OpenReadOnly(e.database)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	if err := index.DB().QueryRow(`SELECT ingestion_fingerprint FROM indexed_files WHERE path=?`, path).Scan(&gotFingerprint); err != nil || gotFingerprint != old {
		t.Fatalf("expired fingerprint=%q err=%v", gotFingerprint, err)
	}
	if err := index.DB().QueryRow(`SELECT text FROM search_items WHERE uuid=?`, uuid).Scan(&gotText); err != nil || gotText != retained {
		t.Fatalf("expired perennial payload=%q err=%v", gotText, err)
	}
}
