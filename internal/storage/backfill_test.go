package storage

import (
	"path/filepath"
	"testing"
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
