package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestSearchModelConversionPreservesExactIdentity(t *testing.T) {
	uuid := "conversion-uuid"
	empty := ""
	got := searchModelResults([]storage.SearchResult{
		{UUID: &uuid, Ordinal: 41, SourcePath: "/present"},
		{UUID: nil, Ordinal: 42, SourcePath: "/nil"},
		{UUID: &empty, Ordinal: 43, SourcePath: "/empty"},
	})

	if len(got) != 3 || got[0].UUID == nil || *got[0].UUID != uuid || got[0].Ordinal != 41 {
		t.Fatalf("present identity was not copied: %+v", got)
	}
	if got[1].UUID != nil || got[1].Ordinal != 42 {
		t.Fatalf("nil identity was not preserved: %+v", got[1])
	}
	if got[2].UUID != nil || got[2].Ordinal != 43 {
		t.Fatalf("empty UUID must be published as null without losing ordinal: %+v", got[2])
	}
}

func TestSearchRobotIdentityEscapesUUIDInMinimalAndFull(t *testing.T) {
	uuid := "uuid\\part\nnext"
	for _, fields := range []string{"minimal", "full"} {
		payload := strings.Join(searchRobotLines([]storage.SearchResult{{
			UUID: &uuid, Ordinal: 51, SourcePath: "/identity", Text: "full", Snippet: "snippet",
		}}, fields, 0), "\n")
		if !strings.Contains(payload, `result_0_uuid=uuid\\part\nnext`) || !strings.Contains(payload, "result_0_ordinal=51") {
			t.Fatalf("%s robot identity was not escaped as an indivisible result: %q", fields, payload)
		}
		for _, line := range strings.Split(payload, "\n") {
			if !strings.HasPrefix(line, "result_0_") {
				t.Fatalf("%s robot UUID split an output line: %q", fields, payload)
			}
		}
	}
}

func TestSearchIdentityOutputsAndContextSelectors(t *testing.T) {
	cfg := newSearchIdentityContractIndex(t)

	t.Run("JSON minimal", func(t *testing.T) {
		stdout := runSearchContract(t, cfg, "identitycontract", "minimal", true, false)
		var results []minimalSearchResult
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("decode minimal JSON: %v\n%s", err, stdout)
		}
		byPath := make(map[string]minimalSearchResult, len(results))
		for _, result := range results {
			byPath[result.SourcePath] = result
		}
		present := byPath["/identity/present.jsonl"]
		if present.UUID == nil || *present.UUID != "contract-uuid" || present.Ordinal != 4 {
			t.Fatalf("minimal present identity = %+v", present)
		}
		for _, path := range []string{"/identity/fallback.jsonl", "/identity/empty.jsonl"} {
			result, ok := byPath[path]
			if !ok || result.UUID != nil {
				t.Fatalf("minimal nullable identity for %s = %+v, present=%t", path, result, ok)
			}
		}
		if !strings.Contains(stdout, `"source_path": "/identity/present.jsonl"`) {
			t.Fatalf("minimal source_path casing changed: %s", stdout)
		}
	})

	t.Run("JSON full", func(t *testing.T) {
		stdout := runSearchContract(t, cfg, "identitycontract", "full", true, false)
		var results []models.SearchResult
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("decode full JSON: %v\n%s", err, stdout)
		}
		byPath := make(map[string]models.SearchResult, len(results))
		for _, result := range results {
			byPath[result.FilePath] = result
		}
		present := byPath["/identity/present.jsonl"]
		if present.UUID == nil || *present.UUID != "contract-uuid" || present.Ordinal != 4 {
			t.Fatalf("full present identity = %+v", present)
		}
		if byPath["/identity/fallback.jsonl"].UUID != nil || byPath["/identity/empty.jsonl"].UUID != nil {
			t.Fatalf("full nullable UUIDs were invented: %+v", byPath)
		}
		var raw []map[string]any
		if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
			t.Fatal(err)
		}
		if len(raw) == 0 {
			t.Fatal("full JSON returned no results")
		}
		for _, key := range []string{"UUID", "Ordinal", "FilePath"} {
			if _, ok := raw[0][key]; !ok {
				t.Fatalf("full JSON missing model-cased key %q: %s", key, stdout)
			}
		}
	})

	t.Run("text", func(t *testing.T) {
		stdout := runSearchContract(t, cfg, "presentselector", "minimal", false, false)
		for _, want := range []string{"Path: /identity/present.jsonl", "UUID: contract-uuid", "Ordinal: 4", "identitycontract presentselector"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("text output missing %q: %s", want, stdout)
			}
		}
		if strings.Index(stdout, "UUID: contract-uuid") > strings.Index(stdout, "identitycontract presentselector") ||
			strings.Index(stdout, "Ordinal: 4") > strings.Index(stdout, "identitycontract presentselector") {
			t.Fatalf("text identity must precede content: %s", stdout)
		}

		nullable := runSearchContract(t, cfg, "fallbackselector", "minimal", false, false)
		if !strings.Contains(nullable, "UUID: null") || !strings.Contains(nullable, "Ordinal: 7") {
			t.Fatalf("text nullable identity = %s", nullable)
		}
	})

	t.Run("search to context by UUID", func(t *testing.T) {
		result := singleMinimalSearchResult(t, runSearchContract(t, cfg, "presentselector", "minimal", true, false))
		if result.UUID == nil {
			t.Fatal("search result did not expose UUID selector")
		}
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			uuid: *result.UUID, uuidSet: true, before: 0, after: 0,
			maxTokens: contextMaxMaxTokens, jsonFormat: true,
		})
		if err != nil || stderr.Len() != 0 {
			t.Fatalf("context by UUID: err=%v stderr=%q", err, stderr.String())
		}
		var envelope contextEnvelope
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Anchor.UUID == nil || *envelope.Anchor.UUID != *result.UUID || envelope.Anchor.Ordinal != int64(result.Ordinal) {
			t.Fatalf("UUID context anchor = %+v, search = %+v", envelope.Anchor, result)
		}
	})

	t.Run("search to context by source path and ordinal", func(t *testing.T) {
		result := singleMinimalSearchResult(t, runSearchContract(t, cfg, "fallbackselector", "minimal", true, false))
		if result.UUID != nil {
			t.Fatalf("fallback result unexpectedly has UUID: %+v", result)
		}
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			sourcePath: result.SourcePath, sourcePathSet: true,
			ordinal: int64(result.Ordinal), ordinalSet: true,
			before: 0, after: 0, maxTokens: contextMaxMaxTokens, jsonFormat: true,
		})
		if err != nil || stderr.Len() != 0 {
			t.Fatalf("context by fallback: err=%v stderr=%q", err, stderr.String())
		}
		var envelope contextEnvelope
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Anchor.UUID != nil || envelope.Anchor.SourcePath != result.SourcePath || envelope.Anchor.Ordinal != int64(result.Ordinal) {
			t.Fatalf("fallback context anchor = %+v, search = %+v", envelope.Anchor, result)
		}
	})

	t.Run("ambiguous fallback remains ambiguous", func(t *testing.T) {
		result := singleMinimalSearchResult(t, runSearchContract(t, cfg, "ambiguousselector", "minimal", true, false))
		var stdout, stderr bytes.Buffer
		err := runContext(context.Background(), &stdout, &stderr, cfg, contextCommandOptions{
			sourcePath: result.SourcePath, sourcePathSet: true,
			ordinal: int64(result.Ordinal), ordinalSet: true,
			before: 0, after: 0, maxTokens: contextMaxMaxTokens, robotFormat: true,
		})
		if err == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "diagnostic_code=context_ambiguous\n") {
			t.Fatalf("ambiguous fallback: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
	})
}

func newSearchIdentityContractIndex(t *testing.T) *config.Config {
	t.Helper()
	path := t.TempDir() + "/search-identity.db"
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, ordinal int, sourcePath, text string, uuid any) {
		t.Helper()
		if _, err := db.DB().Exec(`
			INSERT INTO search_items
				(id, source, source_path, ordinal, role, origin, text, uuid, timestamp, project, content_type)
			VALUES (?, 'session', ?, ?, 'assistant', 'assistant', ?, ?, '2026-09-07T00:00:00Z', 'contract', 'text')
		`, id, sourcePath, ordinal, text, uuid); err != nil {
			t.Fatalf("insert search identity fixture %d: %v", id, err)
		}
	}
	insert(1, 4, "/identity/present.jsonl", "identitycontract presentselector", "contract-uuid")
	insert(2, 7, "/identity/fallback.jsonl", "identitycontract fallbackselector", nil)
	insert(3, 8, "/identity/empty.jsonl", "identitycontract emptyselector", "")
	insert(4, 9, "/identity/ambiguous.jsonl", "ambiguousselector first", nil)
	insert(5, 9, "/identity/ambiguous.jsonl", "duplicate context peer", nil)
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return &config.Config{DatabasePath: path}
}

func runSearchContract(t *testing.T, cfg *config.Config, query, fields string, jsonFormat, robotFormat bool) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runSearch(context.Background(), &stdout, &stderr, cfg,
		query, "", true, jsonFormat, robotFormat,
		"", "", "", "", "", 20, 0, "text", "",
		fields, 0, true, 0.3, false)
	if err != nil {
		t.Fatalf("run search %q: %v\nstderr: %s", query, err, stderr.String())
	}
	return stdout.String()
}

func singleMinimalSearchResult(t *testing.T, payload string) minimalSearchResult {
	t.Helper()
	var results []minimalSearchResult
	if err := json.Unmarshal([]byte(payload), &results); err != nil {
		t.Fatalf("decode search result: %v\n%s", err, payload)
	}
	if len(results) != 1 {
		t.Fatalf("search result count = %d, want 1: %s", len(results), payload)
	}
	return results[0]
}
