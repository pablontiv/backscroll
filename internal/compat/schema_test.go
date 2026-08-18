package compat

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"
)

func TestInspectIndexUsesObservedShapeNotVersionAlone(t *testing.T) {
	tests := []struct {
		fixture       string
		wantFirstStep string
	}{
		{"v5-with-source-metadata.sql", "V6 drop source_metadata when present"},
		{"v5-without-source-metadata.sql", "V7 reasoning triggers"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			db := openFixtureCopy(t, tt.fixture)
			defer db.Close()

			plan, diag, err := InspectIndex(context.Background(), db)
			if err != nil || diag != nil {
				t.Fatalf("plan error=%v diagnostic=%+v", err, diag)
			}
			if len(plan.Steps) == 0 || plan.Steps[0].Name != tt.wantFirstStep {
				t.Fatalf("steps=%+v", plan.Steps)
			}
		})
	}
}

func TestInspectIndexCurrentShapeIsIdempotent(t *testing.T) {
	db := openFixtureCopy(t, "v13.sql")
	defer db.Close()

	plan, diag, err := InspectIndex(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("plan error=%v diagnostic=%+v", err, diag)
	}
	if plan.From.AppliedVersion != 13 {
		t.Fatalf("applied version = %d, want 13", plan.From.AppliedVersion)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("current shape has pending steps: %+v", plan.Steps)
	}
	if err := VerifyCurrentShape(context.Background(), db); err != nil {
		t.Fatalf("verify current shape: %v", err)
	}
}

func TestInspectIndexUnsupportedShapeReturnsInternalDiagnostic(t *testing.T) {
	db := openFixtureCopy(t, "v13.sql")
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE unexpected_shape_marker (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	plan, diag, err := InspectIndex(context.Background(), db)
	if err != nil {
		t.Fatalf("inspect error = %v", err)
	}
	if diag == nil {
		t.Fatalf("diagnostic is nil; plan=%+v", plan)
	}
	if diag.Code != CodeUnsupportedLineage {
		t.Fatalf("diagnostic code = %s, want %s", diag.Code, CodeUnsupportedLineage)
	}
	if !strings.Contains(diag.Summary, plan.From.Signature) {
		t.Fatalf("summary %q does not include signature %q", diag.Summary, plan.From.Signature)
	}
	if len(diag.Continuation) != 0 {
		t.Fatalf("continuation = %+v, want empty", diag.Continuation)
	}
}

func TestVerifyCurrentShapeRejectsPendingMigrations(t *testing.T) {
	db := openFixtureCopy(t, "v5-without-source-metadata.sql")
	defer db.Close()

	if err := VerifyCurrentShape(context.Background(), db); err == nil {
		t.Fatal("verify current shape succeeded for a shape with pending migrations")
	}
}

func TestInspectIndexMalformedMigrationMetadataReturnsError(t *testing.T) {
	db := openSchema(t, `
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
	`)
	defer db.Close()

	_, diag, err := InspectIndex(context.Background(), db)
	if err == nil {
		t.Fatalf("inspect succeeded with diagnostic %+v", diag)
	}
	if diag != nil {
		t.Fatalf("diagnostic = %+v, want nil Go error path", diag)
	}
	if !strings.Contains(err.Error(), "inspect schema:") || !strings.Contains(err.Error(), "schema_migrations") {
		t.Fatalf("error = %v, want wrapped schema_migrations error", err)
	}
}

func TestInspectShapeSignatureIsStableAcrossMetadataOrder(t *testing.T) {
	left := openSchema(t, `
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_on TEXT NOT NULL, checksum TEXT NOT NULL);
		INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (1, 'one', 'first clock', 'checksum');
		CREATE TABLE alpha (id INTEGER PRIMARY KEY, body TEXT NOT NULL);
		CREATE INDEX idx_alpha_body ON alpha(body);
		CREATE TABLE beta (id INTEGER PRIMARY KEY, alpha_id INTEGER NOT NULL);
		CREATE INDEX idx_beta_alpha ON beta(alpha_id);
	`)
	defer left.Close()
	right := openSchema(t, `
		CREATE TABLE beta (id INTEGER PRIMARY KEY, alpha_id INTEGER NOT NULL);
		CREATE INDEX idx_beta_alpha ON beta(alpha_id);
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_on TEXT NOT NULL, checksum TEXT NOT NULL);
		INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (1, 'one', 'second clock', 'checksum');
		CREATE TABLE alpha (id INTEGER PRIMARY KEY, body TEXT NOT NULL);
		CREATE INDEX idx_alpha_body ON alpha(body);
	`)
	defer right.Close()

	leftShape, err := inspectShape(context.Background(), left)
	if err != nil {
		t.Fatal(err)
	}
	rightShape, err := inspectShape(context.Background(), right)
	if err != nil {
		t.Fatal(err)
	}
	if leftShape.Signature != rightShape.Signature {
		t.Fatalf("signatures differ after metadata reordering: %s != %s", leftShape.Signature, rightShape.Signature)
	}
}

func openFixtureCopy(t *testing.T, fixture string) *sql.DB {
	t.Helper()

	data, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/"+fixture)
	if err != nil {
		t.Fatal(err)
	}
	return openSchema(t, string(data))
}

func openSchema(t *testing.T, schemaSQL string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}
