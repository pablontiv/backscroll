package storage

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestV18AddsNullableIngestionFingerprintWithoutGuessingHistory(t *testing.T) {
	path := createFixtureDatabase(t, "v17.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`INSERT INTO indexed_files (path, hash) VALUES ('/expired/source.jsonl', 'stable-hash')`); err != nil {
		t.Fatal(err)
	}

	plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diag != nil {
		t.Fatalf("inspect v17 error=%v diagnostic=%+v", err, diag)
	}
	if len(plan.Steps) != 1 || plan.Steps[0] != (compat.MigrationStep{Version: 18, Name: "V18 ingestion semantics fingerprint"}) {
		t.Fatalf("v17 migration plan = %+v, want v18", plan.Steps)
	}
	if err := applyMigrationPlanForTest(context.Background(), db, plan); err != nil {
		t.Fatal(err)
	}

	var fingerprint sql.NullString
	if err := db.DB().QueryRow(`SELECT ingestion_fingerprint FROM indexed_files WHERE path = '/expired/source.jsonl'`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if fingerprint.Valid {
		t.Fatalf("migration guessed historical ingestion semantics: %q", fingerprint.String)
	}
	assertCurrentShape(t, db.DB())
	assertMigrationVersionCount(t, db.DB(), 18, 1)
}
