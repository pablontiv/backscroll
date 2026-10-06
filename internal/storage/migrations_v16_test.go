package storage

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestV16MigrationAddsConstrainedMessageOrigin(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v15.sql")
	seed, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, uuid, content_type)
		VALUES ('session', '/legacy/session.jsonl', 0, 'user', 'legacy message', 'legacy-origin', 'text')
	`); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diag != nil {
		t.Fatalf("inspect v15 error=%v diagnostic=%+v", err, diag)
	}
	if len(plan.Steps) != 1 || plan.Steps[0] != (compat.MigrationStep{Version: 16, Name: "V16 parser-backed message origin"}) {
		t.Fatalf("v15 migration plan = %+v, want only v16", plan.Steps)
	}
	if err := applyMigrationPlanForTest(context.Background(), db, plan); err != nil {
		t.Fatalf("apply v16: %v", err)
	}

	var origin string
	var originVersion sql.NullInt64
	if err := db.DB().QueryRow(`SELECT origin, origin_version FROM search_items WHERE uuid = 'legacy-origin'`).Scan(&origin, &originVersion); err != nil {
		t.Fatal(err)
	}
	if origin != "unknown" || originVersion.Valid {
		t.Fatalf("historical origin = (%q, %+v), want (unknown, NULL)", origin, originVersion)
	}

	for ordinal, validOrigin := range []string{"human", "assistant", "system", "automation", "unknown"} {
		if _, err := db.DB().Exec(`
			INSERT INTO search_items (source, source_path, ordinal, role, text, content_type, origin, origin_version)
			VALUES ('session', '/current/session.jsonl', ?, 'assistant', 'current message', 'text', ?, 1)
		`, ordinal, validOrigin); err != nil {
			t.Fatalf("insert valid origin %q: %v", validOrigin, err)
		}
	}
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type, origin)
		VALUES ('session', '/invalid/session.jsonl', 0, 'assistant', 'invalid message', 'text', 'operator')
	`); err == nil {
		t.Fatal("origin outside the persisted domain was accepted")
	}
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type, origin)
		VALUES ('session', '/null/session.jsonl', 0, 'assistant', 'null message', 'text', NULL)
	`); err == nil {
		t.Fatal("NULL origin was accepted")
	}

	assertCurrentShape(t, db.DB())
	assertMigrationVersionCount(t, db.DB(), 16, 1)
}
