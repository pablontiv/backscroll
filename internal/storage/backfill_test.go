package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/templates"
)

func TestBackfillDerivedMinesTemplatesFromExpiredFile(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Simulate an expired file: insert search_items row (tool output with error)
	// but do NOT insert indexed_files entry.
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('session', '/expired/s.jsonl', 0, 'assistant', 'error: undefined variable x', '2026-01-01T00:00:00Z',
		        'u1#t0', 'proj', 'tool', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// BackfillDerived should discover this row and mine templates
	var templatesBefore int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM message_templates").Scan(&templatesBefore); err != nil {
		t.Fatal(err)
	}
	if templatesBefore != 0 {
		t.Fatalf("expected 0 templates before backfill, got %d", templatesBefore)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var templatesAfter int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM message_templates").Scan(&templatesAfter); err != nil {
		t.Fatal(err)
	}
	if templatesAfter == 0 {
		t.Error("backfill must discover and mine templates from expired rows")
	}
}

func TestBackfillDerivedIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert expired-file row
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('session', '/expired/s.jsonl', 0, 'user', 'no, eso no', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Run backfill twice
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	var signalsAfterFirst int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&signalsAfterFirst); err != nil {
		t.Fatal(err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var signalsAfterSecond int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&signalsAfterSecond); err != nil {
		t.Fatal(err)
	}

	if signalsAfterFirst != signalsAfterSecond {
		t.Errorf("backfill must be idempotent; signals: %d -> %d", signalsAfterFirst, signalsAfterSecond)
	}
}

func TestBackfillDerivedExtractsLossyToolEventsFromInputs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Expired file with tool input message (recoverable)
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('session', '/expired/s.jsonl', 0, 'assistant', 'Bash command=go test', '2026-01-01T00:00:00Z', NULL, 'proj', 'tool', 0)
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var toolName, cmdHead string
	var extractVersion int
	if err := db.db.QueryRow(`
		SELECT tool_name, command_head, extraction_version FROM tool_events WHERE source_path = '/expired/s.jsonl' AND ordinal = 0
	`).Scan(&toolName, &cmdHead, &extractVersion); err != nil {
		t.Fatalf("query tool_events: %v", err)
	}
	if toolName != "Bash" {
		t.Errorf("tool_name = %q, want Bash", toolName)
	}
	if cmdHead != "go" {
		t.Errorf("command_head = %q, want go", cmdHead)
	}
	if extractVersion != 0 {
		t.Errorf("extraction_version = %d, want 0 (lossy marker)", extractVersion)
	}
}

func TestBackfillDerivedSkipsOutputTexts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Expired file with tool OUTPUT messages (not recoverable as tool_events).
	// Outputs cannot be attributed to a tool without tool_use_id linkage.
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES
		('session', '/expired/output.jsonl', 0, 'assistant', 'error: exit code 1', '2026-01-01T00:00:00Z', NULL, 'proj', 'tool', 0),
		('session', '/expired/output.jsonl', 1, 'assistant', 'PASS: all tests', '2026-01-01T00:00:05Z', NULL, 'proj', 'tool', 0)
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Output-only file should yield zero tool_events (cannot attribute without tool_use_id)
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE source_path = '/expired/output.jsonl'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("output-only file must yield 0 tool_events; got %d (lossy: outputs unattributable)", count)
	}
}

func TestBackfillDerivedWithNonToolMessages(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Mix of tool and non-tool messages
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES
		('session', '/expired/mixed.jsonl', 0, 'user', 'run tests', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 1),
		('session', '/expired/mixed.jsonl', 1, 'assistant', 'Bash command=go test', '2026-01-01T00:00:01Z', 'u2', 'proj', 'tool', 1),
		('session', '/expired/mixed.jsonl', 2, 'user', 'fix error eso no', '2026-01-01T00:00:02Z', 'u3', 'proj', 'text', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Verify corrections detected in text messages
	var signalCount int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals WHERE source_path = '/expired/mixed.jsonl'").Scan(&signalCount); err != nil {
		t.Fatal(err)
	}
	if signalCount == 0 {
		t.Error("backfill should detect corrections in prose messages")
	}
}

func TestBackfillDerivedNoFilesToBackfill(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Database with no expired files should return immediately
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill empty: %v", err)
	}
}

func TestBackfillDerivedProgressCallback(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert two expired files
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES
		('session', '/expired/s1.jsonl', 0, 'user', 'no eso no', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 1),
		('session', '/expired/s2.jsonl', 0, 'user', 'no eso no', '2026-01-01T00:00:00Z', 'u2', 'proj', 'text', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Track progress calls
	var progressCalls []struct {
		processed int
		templates int
		signals   int
		events    int
	}
	if err := db.BackfillDerived(BackfillDerivedOpts{
		OnProgress: func(processed, templateCount, signalCount, eventCount int) {
			progressCalls = append(progressCalls, struct {
				processed int
				templates int
				signals   int
				events    int
			}{processed, templateCount, signalCount, eventCount})
		},
	}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Progress callback should have been called
	if len(progressCalls) == 0 {
		t.Error("OnProgress callback was not called")
	}
}

func TestBackfillDerivedSkipsOnDiskFiles(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// On-disk file: insert search_items row AND indexed_files entry.
	// This file should NOT be touched by backfill (belongs to B1 rich re-parse path).
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('session', '/disk/s.jsonl', 0, 'user', 'no eso no', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.db.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/disk/s.jsonl', 'hash123', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Run backfill
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Verify no lossy tool_events were created (file was skipped)
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE source_path = '/disk/s.jsonl' AND extraction_version = 0").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("on-disk file must not be backfilled; got %d lossy events (expected 0)", count)
	}
}

func TestBackfillDerivedPartialDerivationsCheck(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Expired file with templates+corrections but NO lossy tool_events.
	// This file should be processed to add lossy tool_events.
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('session', '/expired/partial.jsonl', 0, 'assistant', 'Bash command=go test', '2026-01-01T00:00:00Z', NULL, 'proj', 'tool', 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Manually insert template_matches and correction_signals (simulating earlier mining)
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('sig1', 1, 'error: <*>', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}
	var tmplID int64
	if err := db.db.QueryRow("SELECT id FROM message_templates WHERE signature = 'sig1'").Scan(&tmplID); err != nil {
		t.Fatal(err)
	}
	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES (?, NULL, '/expired/partial.jsonl', 0)
	`, tmplID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.db.Exec(`
		INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES (NULL, '/expired/partial.jsonl', 0, 'lexicon', 0.8, 1)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify: no lossy tool_events yet
	var eventsBefore int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE source_path = '/expired/partial.jsonl' AND extraction_version = 0").Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if eventsBefore != 0 {
		t.Fatalf("expected 0 lossy events before backfill, got %d", eventsBefore)
	}

	// Run backfill: should add lossy tool_events (missing derivation)
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Verify: lossy tool_events added
	var eventsAfter int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE source_path = '/expired/partial.jsonl' AND extraction_version = 0").Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter == 0 {
		t.Error("backfill must add lossy tool_events when missing from expired file")
	}
}

// TestBackfillTemplatesFilterInputs verifies that input serializations are excluded
// and only error-bearing rows are mined.
func TestBackfillTemplatesFilterInputs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	sourcePath := "test_session.jsonl"

	// Note: Do NOT insert into indexed_files since backfill processes EXPIRED files
	// (those absent from indexed_files but present in search_items).

	// Insert tool_events: ordinal 3 has is_error=1
	_, err = db.db.Exec(`
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, extraction_version)
		VALUES ('uuid-3', ?, 3, 'Bash', 'echo', 1, 1)
	`, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Insert search_items with mixed content:
	// - ordinal 1: input serialization (Bash command=...) — should be EXCLUDED
	// - ordinal 2: error prefix — should be INCLUDED
	// - ordinal 3: is_error=1 in tool_events — should be INCLUDED
	// - ordinal 4: normal output, no error — should be EXCLUDED
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source_path, ordinal, role, text, timestamp, uuid, content_type, extraction_version)
		VALUES
		(?, 1, 'assistant', 'Bash command=go test', '2026-01-01T00:00:00Z', 'uuid-1', 'tool', 1),
		(?, 2, 'tool', 'error: file not found', '2026-01-01T00:00:01Z', 'uuid-2', 'tool', 1),
		(?, 3, 'tool', 'ok output', '2026-01-01T00:00:02Z', 'uuid-3', 'tool', 1),
		(?, 4, 'tool', 'normal output no error', '2026-01-01T00:00:03Z', 'uuid-4', 'tool', 1)
	`, sourcePath, sourcePath, sourcePath, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Run backfill: should mine from ordinal 2 and 3 only (exclude 1 and 4)
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatal(err)
	}

	// Verify: check template_matches to see which ordinals were mined
	rows, err := db.db.Query(`
		SELECT ordinal FROM template_matches WHERE source_path = ? ORDER BY ordinal
	`, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var minedOrdinals []int
	for rows.Next() {
		var ord int
		if err := rows.Scan(&ord); err != nil {
			t.Fatal(err)
		}
		minedOrdinals = append(minedOrdinals, ord)
	}

	// Expected: ordinals 2 and 3 only (ordinal 1 is input, ordinal 4 has no error)
	if len(minedOrdinals) == 0 {
		t.Errorf("expected templates from ordinals 2 and 3, got none")
		return
	}

	// Check that input serialization (ordinal 1) was NOT mined
	for _, ord := range minedOrdinals {
		if ord == 1 {
			t.Errorf("ordinal 1 (input serialization) must not be mined; found in templates")
		}
		if ord == 4 {
			t.Errorf("ordinal 4 (no error) must not be mined; found in templates")
		}
	}

	// Check that at least one of 2 or 3 was mined
	foundErrorOrSignal := false
	for _, ord := range minedOrdinals {
		if ord == 2 || ord == 3 {
			foundErrorOrSignal = true
		}
	}
	if !foundErrorOrSignal {
		t.Errorf("expected ordinals 2 or 3 to be mined (error-bearing); got %v", minedOrdinals)
	}
}

// TestStaleTemplatePaths discovers templates older than currentVersion (RED test - task 3.1)
func TestStaleTemplatePaths(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert templates with mixed normalization versions
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES
		('sig1', 1, 'error: v1 template', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z'),
		('sig2', 1, 'error: another v1', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z'),
		('sig3', 2, 'error: v2 template', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert templates: %v", err)
	}

	// Insert template_matches linking templates to source paths
	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		SELECT id, 'uuid1', '/path/s1.jsonl', 0 FROM message_templates WHERE signature = 'sig1'
		UNION ALL
		SELECT id, 'uuid2', '/path/s2.jsonl', 0 FROM message_templates WHERE signature = 'sig2'
		UNION ALL
		SELECT id, 'uuid3', '/path/s3.jsonl', 0 FROM message_templates WHERE signature = 'sig3'
	`)
	if err != nil {
		t.Fatalf("insert matches: %v", err)
	}

	// Call StaleTemplatePaths with currentVersion=2 - should return paths for v1 templates
	stalePaths, err := db.StaleTemplatePaths(2)
	if err != nil {
		t.Fatalf("StaleTemplatePaths: %v", err)
	}

	// Should find 2 stale paths (sig1 and sig2 are v1)
	if len(stalePaths) != 2 {
		t.Errorf("expected 2 stale paths, got %d: %v", len(stalePaths), stalePaths)
	}

	// Verify paths are returned
	pathSet := make(map[string]bool)
	for _, p := range stalePaths {
		pathSet[p] = true
	}
	if !pathSet["/path/s1.jsonl"] || !pathSet["/path/s2.jsonl"] {
		t.Errorf("expected /path/s1.jsonl and /path/s2.jsonl in stale paths, got %v", stalePaths)
	}
}

// TestStaleTemplatePathsEmpty returns empty when all templates at current version
func TestStaleTemplatePathsEmpty(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert only v2 templates (current version)
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('sig1', 2, 'error: v2 template', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Call StaleTemplatePaths with currentVersion=2 - should return empty
	stalePaths, err := db.StaleTemplatePaths(2)
	if err != nil {
		t.Fatalf("StaleTemplatePaths: %v", err)
	}

	if len(stalePaths) != 0 {
		t.Errorf("expected 0 stale paths when all templates at current version, got %d", len(stalePaths))
	}
}

// TestBackfillDerivedReminesToUpsertSemantics verifies upsert path behavior (RED test - task 3.3)
func TestBackfillDerivedReminesToUpsertSemantics(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// First, sync one message to naturally generate a v2 template
	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ContentType: "tool", ToolName: "Bash", IsError: boolPtr(true), Text: "error: database locked", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/s.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Get the template that was created (should have normalization_version=2 from miner)
	var tmplID int64
	var sig string
	var normVersion int
	if err := db.db.QueryRow("SELECT id, signature, normalization_version FROM message_templates ORDER BY id DESC LIMIT 1").Scan(&tmplID, &sig, &normVersion); err != nil {
		t.Fatal(err)
	}

	// Manually downgrade template to v1 to simulate historical template
	_, err = db.db.Exec(`UPDATE message_templates SET normalization_version = 1 WHERE id = ?`, tmplID)
	if err != nil {
		t.Fatalf("downgrade template: %v", err)
	}

	// Add another error message that will mine to the SAME template (use numeric suffix to force variable matching)
	// The Drain miner with depth=2 keeps first 2 tokens, so:
	// "error: database 999" → normalized to "error: database <*>" (third token is numeric)
	// This will generate the same template signature as "error: database locked" only if
	// ExtractErrorLines makes the change. Let's use the same message to ensure same signature.
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, uuid, project, content_type, extraction_version)
		VALUES ('session', '/s.jsonl', 1, 'assistant', 'error: database locked', 'u2#t0', 'proj', 'tool', 1)
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	// Call BackfillDerived - should update normalization_version back to 2 via upsert
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Verify template normalization_version was updated to 2
	var updatedNormVersion int
	if err := db.db.QueryRow("SELECT normalization_version FROM message_templates WHERE id = ?", tmplID).Scan(&updatedNormVersion); err != nil {
		t.Fatal(err)
	}
	if updatedNormVersion != 2 {
		t.Errorf("expected normalization_version=2 after upsert, got %d", updatedNormVersion)
	}
}

// TestBackfillDerivedRemineIdempotency verifies re-run doesn't duplicate rows
func TestBackfillDerivedRemineIdempotency(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert v1 template
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('sig1', 1, 'error: <*> timeout', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Insert search_items
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, uuid, project, content_type, extraction_version)
		VALUES ('session', '/s.jsonl', 0, 'assistant', 'error: timeout', 'u1#t0', 'proj', 'tool', 1)
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Run BackfillDerived twice
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("first backfill: %v", err)
	}

	var matchesAfterFirst int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM template_matches").Scan(&matchesAfterFirst); err != nil {
		t.Fatal(err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	var matchesAfterSecond int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM template_matches").Scan(&matchesAfterSecond); err != nil {
		t.Fatal(err)
	}

	// Counts should be the same (idempotent)
	if matchesAfterFirst != matchesAfterSecond {
		t.Errorf("expected idempotent count, got %d then %d", matchesAfterFirst, matchesAfterSecond)
	}
}

// TestLoadMessagesForPath verifies that LoadMessagesForPath correctly retrieves
// messages from the database for a given source path (Q3 helper).
func TestLoadMessagesForPath(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	sourcePath := "path/to/session.json"

	// Insert sample search_items rows
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES
		('session', ?, 0, 'user', 'hello', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 3),
		('session', ?, 1, 'assistant', 'hi there', '2026-01-01T00:00:01Z', 'u2', 'proj', 'text', 3)
	`, sourcePath, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	// Test: load messages for the path
	loaded, err := db.LoadMessagesForPath(sourcePath)
	if err != nil {
		t.Fatalf("LoadMessagesForPath failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded))
	}

	if loaded[0].Ordinal != 0 || loaded[0].UUID != "u1" {
		t.Errorf("message 0 mismatch: got %+v", loaded[0])
	}

	if loaded[1].Ordinal != 1 || loaded[1].UUID != "u2" {
		t.Errorf("message 1 mismatch: got %+v", loaded[1])
	}
}

// TestBackfillRederivesSupersededSignalsForExpiredSession covers the only route a
// detector fix has to an EXPIRED session: its JSONL is gone from disk, so SyncFiles
// will never run for it again and the sync-time epoch clear can never fire. Without
// backfill re-derivation a false positive recorded under older detector rules would
// be permanent for exactly the sessions the perennial store exists to preserve.
func TestBackfillRederivesSupersededSignalsForExpiredSession(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/expired/chat.jsonl"

	// A perennial row whose source file is gone: present in search_items, absent
	// from indexed_files. Its text is a pasted transcript current rules reject.
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES (?, 'session', 0, 'user', ?, '2026-01-01T00:00:00Z', 'x#0', 'proj', 'text', ?)
	`, path, "[3:06 p.m., 2/7/2026] Pedro Chan: eso no es un bug [3:07 p.m., 2/7/2026] Pablo: te dije que no [3:08 p.m., 2/7/2026] Ana: arreglado", CurrentExtractionVersion); err != nil {
		t.Fatalf("seed expired search_item: %v", err)
	}
	// A genuine single-message correction on the same expired path. It must survive
	// re-derivation, and its stamp is what the convergence check below reads — a
	// fixture where every message is filtered would make that check vacuous.
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES (?, 'session', 1, 'user', 'no, eso no es un bug', '2026-01-01T00:00:01Z', 'x#1', 'proj', 'text', ?)
	`, path, CurrentExtractionVersion); err != nil {
		t.Fatalf("seed genuine correction: %v", err)
	}
	// A stale false positive from an older detector epoch.
	if _, err := db.db.Exec(`
		INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('x#0', ?, 0, 'lexicon', 0.8, 0)
	`, path); err != nil {
		t.Fatalf("seed stale signal: %v", err)
	}

	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("BackfillDerived: %v", err)
	}

	var transcriptSignals int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals WHERE source_path = ? AND ordinal = 0 AND detector = 'lexicon'
	`, path).Scan(&transcriptSignals); err != nil {
		t.Fatalf("count transcript signals: %v", err)
	}
	if transcriptSignals != 0 {
		t.Errorf("stale lexicon signal on the pasted transcript survived backfill: got %d, want 0", transcriptSignals)
	}

	var genuineSignals int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals WHERE source_path = ? AND ordinal = 1 AND detector = 'lexicon'
	`, path).Scan(&genuineSignals); err != nil {
		t.Fatalf("count genuine signals: %v", err)
	}
	if genuineSignals != 1 {
		t.Fatalf("genuine correction was not re-derived: got %d lexicon signals, want 1", genuineSignals)
	}

	// Convergence: re-derived signals are stamped current, so a second pass must not
	// keep rediscovering this path forever.
	var stale int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals
		WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
	`, path, CurrentExtractionVersion).Scan(&stale); err != nil {
		t.Fatalf("count stale-epoch signals: %v", err)
	}
	if stale != 0 {
		t.Errorf("backfill left %d signal(s) below the current epoch; discovery would never converge", stale)
	}
}

// TestRederiveSupersededCorrectionsReachesDeadZone covers the session state neither
// existing path can reach: the JSONL has expired from disk (so SyncFiles never runs
// for it) while its indexed_files row remains (so BackfillDerived does not consider
// it expired). Nothing prunes indexed_files when a file disappears, so this is the
// common state, not an edge case — a detector fix that misses it is inert in practice.
//
// It also pins convergence for the empty case: a path that re-derives to NO signals
// must not be rediscovered forever.
func TestRederiveSupersededCorrectionsReachesDeadZone(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/gone/from/disk.jsonl"

	// indexed_files row survives the file's disappearance: this is the dead zone.
	if _, err := db.db.Exec(`INSERT INTO indexed_files (path, hash, last_indexed) VALUES (?, 'h', '2026-01-01T00:00:00Z')`, path); err != nil {
		t.Fatalf("seed indexed_files: %v", err)
	}
	// Only content is a pasted transcript, so current rules yield NO signals at all.
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES (?, 'session', 0, 'user', ?, '2026-01-01T00:00:00Z', 'd#0', 'proj', 'text', ?)
	`, path, "[3:06 p.m., 2/7/2026] Pedro Chan: eso no es un bug [3:07 p.m., 2/7/2026] Pablo: te dije que no [3:08 p.m., 2/7/2026] Ana: arreglado", CurrentExtractionVersion); err != nil {
		t.Fatalf("seed search_item: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('d#0', ?, 0, 'lexicon', 0.8, 0)
	`, path); err != nil {
		t.Fatalf("seed stale signal: %v", err)
	}

	// BackfillDerived must NOT be what fixes this — the path is not "expired" by its
	// definition. Running it first proves the dead zone is real.
	if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
		t.Fatalf("BackfillDerived: %v", err)
	}
	var afterBackfill int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path = ?`, path).Scan(&afterBackfill); err != nil {
		t.Fatalf("count after backfill: %v", err)
	}
	if afterBackfill == 0 {
		t.Fatal("precondition void: BackfillDerived already cleared this path, so the dead zone this test guards no longer exists")
	}

	processed, err := db.RederiveSupersededCorrections(200)
	if err != nil {
		t.Fatalf("RederiveSupersededCorrections: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed %d paths, want 1: the dead-zone path was not discovered", processed)
	}

	var remaining int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path = ?`, path).Scan(&remaining); err != nil {
		t.Fatalf("count after re-derivation: %v", err)
	}
	if remaining != 0 {
		t.Errorf("stale signal survived re-derivation: got %d, want 0", remaining)
	}

	// Convergence for the empty case: nothing left below the current epoch, so a
	// second pass finds nothing and the path stops being reprocessed.
	again, err := db.RederiveSupersededCorrections(200)
	if err != nil {
		t.Fatalf("second RederiveSupersededCorrections: %v", err)
	}
	if again != 0 {
		t.Errorf("second pass reprocessed %d path(s); empty re-derivation never converges", again)
	}
}

// TestRederiveSupersededCorrectionsIsBoundedAndDrains pins the per-run bound: a large
// backlog must not be re-derived in one pass (auto-sync runs on ordinary commands and
// cannot stall on it), and successive runs must drain the rest in deterministic order
// rather than revisiting the same head forever.
func TestRederiveSupersededCorrectionsIsBoundedAndDrains(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const total = 5
	for i := 0; i < total; i++ {
		path := fmt.Sprintf("/p/s%02d.jsonl", i)
		if _, err := db.db.Exec(`
			INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
			VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', ?, 'proj', 'text', ?)
		`, path, fmt.Sprintf("u%02d#0", i), CurrentExtractionVersion); err != nil {
			t.Fatalf("seed search_item %d: %v", i, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, 0, 'lexicon', 0.8, 0)
		`, fmt.Sprintf("u%02d#0", i), path); err != nil {
			t.Fatalf("seed stale signal %d: %v", i, err)
		}
	}

	const limit = 2
	first, err := db.RederiveSupersededCorrections(limit)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first != limit {
		t.Errorf("first pass processed %d paths, want %d: the per-run bound is not applied", first, limit)
	}

	// The head of the ordering must be done, and the tail untouched.
	var headStale, tailStale int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals
		WHERE source_path = '/p/s00.jsonl' AND (extraction_version IS NULL OR extraction_version < ?)
	`, CurrentExtractionVersion).Scan(&headStale); err != nil {
		t.Fatalf("count head: %v", err)
	}
	if headStale != 0 {
		t.Errorf("first path still carries %d superseded signal(s) after being processed", headStale)
	}
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals
		WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
	`, fmt.Sprintf("/p/s%02d.jsonl", total-1), CurrentExtractionVersion).Scan(&tailStale); err != nil {
		t.Fatalf("count tail: %v", err)
	}
	if tailStale == 0 {
		t.Error("last path was processed despite the per-run bound")
	}

	// Successive passes drain the backlog and then stop.
	runs := 0
	for {
		n, err := db.RederiveSupersededCorrections(limit)
		if err != nil {
			t.Fatalf("drain pass: %v", err)
		}
		if n == 0 {
			break
		}
		runs++
		if runs > total {
			t.Fatal("draining did not terminate: the same paths are being reprocessed")
		}
	}

	var remaining int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals WHERE extraction_version IS NULL OR extraction_version < ?
	`, CurrentExtractionVersion).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d superseded signal(s) left after draining", remaining)
	}
}

// TestRederiveSupersededCorrectionsPropagatesErrors pins failure behaviour: this runs
// inside ordinary auto-sync, where a database problem must surface as an error the
// caller can warn about rather than a panic that kills a routine command.
func TestRederiveSupersededCorrectionsPropagatesErrors(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	processed, err := db.RederiveSupersededCorrections(10)
	if err == nil {
		t.Error("expected an error from a closed database, got nil")
	}
	if processed != 0 {
		t.Errorf("processed = %d on a failed run, want 0", processed)
	}
}

// TestLoadMessagesForPathHandlesNullColumns pins the failure that made re-derivation
// inert on the real corpus: legacy and Pi/OpenCode rows carry NULL uuid, and a plain
// string Scan aborts on them. Because discovery order is deterministic, one such path
// sat at the head of every batch and no path was ever re-derived.
func TestLoadMessagesForPathHandlesNullColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.db.Exec(`
		INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES ('/legacy/s.jsonl', 'session', 0, 'user', 'no, eso no es un bug', NULL, NULL, 'proj', 'text', NULL)
	`); err != nil {
		t.Fatalf("seed NULL-uuid row: %v", err)
	}

	msgs, err := db.LoadMessagesForPath("/legacy/s.jsonl")
	if err != nil {
		t.Fatalf("LoadMessagesForPath must tolerate NULL columns: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("loaded %d messages, want 1", len(msgs))
	}
	if msgs[0].UUID != "" {
		t.Errorf("UUID = %q, want empty string for a NULL uuid", msgs[0].UUID)
	}
	if msgs[0].Timestamp != "" {
		t.Errorf("Timestamp = %q, want empty string for a NULL timestamp", msgs[0].Timestamp)
	}
	if msgs[0].ExtractionVersion != 0 {
		t.Errorf("ExtractionVersion = %d, want 0 for a NULL value", msgs[0].ExtractionVersion)
	}
}

// TestRederiveSupersededCorrectionsSkipsUnreadablePaths pins batch resilience: a path
// that cannot be read must not abort the run, or the deterministic ordering would park
// the same bad path at the head of every batch forever.
func TestRederiveSupersededCorrectionsSkipsUnreadablePaths(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Two paths with superseded signals; the first sorts ahead of the second.
	for _, p := range []string{"/a/first.jsonl", "/b/second.jsonl"} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
			VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', NULL, 'proj', 'text', 1)
		`, p); err != nil {
			t.Fatalf("seed search_item %s: %v", p, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES ('x', ?, 0, 'lexicon', 0.8, 0)
		`, p); err != nil {
			t.Fatalf("seed signal %s: %v", p, err)
		}
	}

	if _, err := db.RederiveSupersededCorrections(200); err != nil {
		t.Fatalf("re-derive: %v", err)
	}

	var stale int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals WHERE extraction_version IS NULL OR extraction_version < ?
	`, CurrentExtractionVersion).Scan(&stale); err != nil {
		t.Fatalf("count stale: %v", err)
	}
	if stale != 0 {
		t.Errorf("%d superseded signal(s) remain; NULL-bearing paths were not processed", stale)
	}
}

// TestBackfillTemplatesForFileUpgradesAndIsIdempotent covers the storage-level entry
// point the incremental Q3 re-mining calls for each stale path: it must lift a
// template to the current normalization version and stay a no-op when re-run.
func TestBackfillTemplatesForFileUpgradesAndIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/p/errors.jsonl"
	msgs := []IndexedMessage{
		{Ordinal: 0, Role: "assistant", Text: "error: cannot find package /tmp/a/b.go", UUID: "e#0",
			Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion},
	}

	miner := templates.NewMiner()
	if _, err := db.BackfillTemplatesForFile(miner, path, msgs); err != nil {
		t.Fatalf("first re-mine: %v", err)
	}

	var mined int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates WHERE normalization_version = ?`, CurrentNormalizationVersion).Scan(&mined); err != nil {
		t.Fatalf("count current-version templates: %v", err)
	}
	if mined == 0 {
		t.Fatal("re-mining produced no template at the current normalization version")
	}

	// Age it, then re-mine: the row must be lifted back to the current version.
	if _, err := db.db.Exec(`UPDATE message_templates SET normalization_version = 1`); err != nil {
		t.Fatalf("age template: %v", err)
	}
	if _, err := db.BackfillTemplatesForFile(templates.NewMiner(), path, msgs); err != nil {
		t.Fatalf("re-mine after aging: %v", err)
	}
	var stale int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates WHERE normalization_version < ?`, CurrentNormalizationVersion).Scan(&stale); err != nil {
		t.Fatalf("count stale: %v", err)
	}
	if stale != 0 {
		t.Errorf("%d template(s) left below the current normalization version", stale)
	}

	// Idempotence: occurrence counts and matches must not grow on a repeat run.
	var occBefore, matchBefore int
	_ = db.db.QueryRow(`SELECT COALESCE(SUM(occurrence_count),0) FROM message_templates`).Scan(&occBefore)
	_ = db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchBefore)
	if _, err := db.BackfillTemplatesForFile(templates.NewMiner(), path, msgs); err != nil {
		t.Fatalf("third re-mine: %v", err)
	}
	var occAfter, matchAfter int
	_ = db.db.QueryRow(`SELECT COALESCE(SUM(occurrence_count),0) FROM message_templates`).Scan(&occAfter)
	_ = db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchAfter)
	if occAfter != occBefore || matchAfter != matchBefore {
		t.Errorf("re-mining is not idempotent: occurrences %d->%d, matches %d->%d", occBefore, occAfter, matchBefore, matchAfter)
	}

	// An empty message set must be a clean no-op, not an error.
	if _, err := db.BackfillTemplatesForFile(templates.NewMiner(), "/p/empty.jsonl", nil); err != nil {
		t.Errorf("empty re-mine should be a no-op: %v", err)
	}
}

// TestRederiveSupersededCorrectionsRollsBackFailedPath pins the atomicity of a skipped
// path. Re-deriving deletes a path's superseded signals before writing the new ones,
// so a failure between the two must roll back: otherwise the path is left with its old
// signals destroyed and its new ones missing — strictly worse than not touching it,
// and reported as a skip. A trigger makes the INSERT fail after the DELETE has run.
func TestRederiveSupersededCorrectionsRollsBackFailedPath(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const poison = "/a/poison.jsonl"
	const healthy = "/b/healthy.jsonl"

	for i, p := range []string{poison, healthy} {
		uuid := fmt.Sprintf("p%d#0", i)
		if _, err := db.db.Exec(`
			INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
			VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', ?, 'proj', 'text', 1)
		`, p, uuid); err != nil {
			t.Fatalf("seed search_item %s: %v", p, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, 0, 'lexicon', 0.8, 0)
		`, uuid, p); err != nil {
			t.Fatalf("seed stale signal %s: %v", p, err)
		}
	}

	// Fail every INSERT for the poison path, leaving its DELETE already executed.
	if _, err := db.db.Exec(`
		CREATE TRIGGER poison_insert BEFORE INSERT ON correction_signals
		WHEN NEW.source_path = '` + poison + `'
		BEGIN SELECT RAISE(ABORT, 'poisoned'); END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	processed, err := db.RederiveSupersededCorrections(200)
	if err == nil {
		t.Error("expected the failing path to be reported, got nil error")
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1 (the healthy path must still be re-derived)", processed)
	}

	// The failing path must be untouched: its original signal survives.
	var poisonSignals int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path = ?`, poison).Scan(&poisonSignals); err != nil {
		t.Fatalf("count poison signals: %v", err)
	}
	if poisonSignals != 1 {
		t.Errorf("failing path has %d signal(s), want 1: its DELETE was committed despite the failure", poisonSignals)
	}

	// The healthy path must be fully re-derived and stamped current.
	var healthyStale int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_signals
		WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
	`, healthy, CurrentExtractionVersion).Scan(&healthyStale); err != nil {
		t.Fatalf("count healthy stale: %v", err)
	}
	if healthyStale != 0 {
		t.Errorf("healthy path still carries %d superseded signal(s)", healthyStale)
	}
}

func TestBackfillTemplatesForFileDeletesStuckTemplates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a v1 template from input serialization + a matching row
	// This simulates a stuck template that was mined before isInputSerialization guard
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('v1_input_sig', 1, 'Bash command=<*>', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}

	var tmplID int64
	err = db.db.QueryRow(`SELECT id FROM message_templates WHERE signature = ?`, "v1_input_sig").Scan(&tmplID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u1', '/test/s.jsonl', 0)
	`, tmplID)
	if err != nil {
		t.Fatal(err)
	}

	// Count templates before backfill
	var countBefore int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 template before backfill, got %d", countBefore)
	}

	// Load messages for backfill (empty set — no error lines, so re-mining finds nothing)
	msgs := []IndexedMessage{}

	miner := templates.NewMiner()
	deletedCount, err := db.BackfillTemplatesForFile(miner, "/test/s.jsonl", msgs)
	if err != nil {
		t.Fatalf("BackfillTemplatesForFile: %v", err)
	}

	// The stuck template should have been deleted
	var countAfter int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != 0 {
		t.Errorf("stuck template should be deleted; expected 0, got %d", countAfter)
	}
	if deletedCount != 1 {
		t.Errorf("deletedCount should be 1, got %d", deletedCount)
	}
}

func TestBackfillTemplatesForFilePreservesMultiPathTemplates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a v1 template with matches on TWO different paths
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('multi_path_sig', 1, 'error: <*>', 2, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}

	var tmplID int64
	err = db.db.QueryRow(`SELECT id FROM message_templates WHERE signature = ?`, "multi_path_sig").Scan(&tmplID)
	if err != nil {
		t.Fatal(err)
	}

	// Matches on path A and B
	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u1', '/test/a.jsonl', 0), (?, 'u2', '/test/b.jsonl', 0)
	`, tmplID, tmplID)
	if err != nil {
		t.Fatal(err)
	}

	// Re-mine only path A with empty messages (no error lines → re-mining finds nothing)
	miner := templates.NewMiner()
	msgs := []IndexedMessage{}

	deletedCount, err := db.BackfillTemplatesForFile(miner, "/test/a.jsonl", msgs)
	if err != nil {
		t.Fatalf("BackfillTemplatesForFile: %v", err)
	}

	// Template should NOT be deleted (still has matches on B)
	var countAfter int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates WHERE signature = ?`, "multi_path_sig").Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countAfter != 1 {
		t.Errorf("multi-path template should be preserved; expected 1, got %d", countAfter)
	}

	// But matches on path A should be deleted
	var matchesOnA int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches WHERE template_id = ? AND source_path = ?`, tmplID, "/test/a.jsonl").Scan(&matchesOnA); err != nil {
		t.Fatal(err)
	}
	if matchesOnA != 0 {
		t.Errorf("matches on path A should be deleted; expected 0, got %d", matchesOnA)
	}

	// Matches on path B should remain
	var matchesOnB int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches WHERE template_id = ? AND source_path = ?`, tmplID, "/test/b.jsonl").Scan(&matchesOnB); err != nil {
		t.Fatal(err)
	}
	if matchesOnB != 1 {
		t.Errorf("matches on path B should be preserved; expected 1, got %d", matchesOnB)
	}

	if deletedCount != 0 {
		t.Errorf("no templates should be deleted (still reachable via path B); got %d", deletedCount)
	}
}
