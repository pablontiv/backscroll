package compat

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
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

func TestInspectIndexRecognizesPartialV6LineageAndPlansV7(t *testing.T) {
	db := openFixtureCopy(t, "v6.sql")
	defer db.Close()

	plan, diag, err := InspectIndex(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("plan error=%v diagnostic=%+v", err, diag)
	}
	if plan.From.AppliedVersion != 6 {
		t.Fatalf("applied version = %d, want 6", plan.From.AppliedVersion)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Name != "V7 reasoning triggers" {
		t.Fatalf("steps=%+v, want V7+", plan.Steps)
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

func TestJoinRowsCloseErrorJoinsPrimaryAndCloseErrors(t *testing.T) {
	primaryErr := errors.New("primary schema rows failure")
	closeErr := errors.New("close schema rows failure")
	err := primaryErr

	joinRowsCloseError(fakeSchemaRowsCloser{err: closeErr}, &err)

	if err == nil || !errors.Is(err, primaryErr) || !errors.Is(err, closeErr) {
		t.Fatalf("joined error = %v, want primary and close", err)
	}
}

func TestJoinRowsCloseErrorLeavesSuccessfulScansUnchanged(t *testing.T) {
	var err error
	joinRowsCloseError(fakeSchemaRowsCloser{}, &err)
	if err != nil {
		t.Fatalf("successful rows close error = %v, want nil", err)
	}

	primaryErr := errors.New("primary schema rows failure")
	err = primaryErr
	joinRowsCloseError(fakeSchemaRowsCloser{}, &err)
	if err != primaryErr {
		t.Fatalf("primary error changed to %v", err)
	}
}

type fakeSchemaRowsCloser struct {
	err error
}

func (r fakeSchemaRowsCloser) Close() error { return r.err }

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

func TestRegularTableSignatureIsConservativeForUnsupportedDDL(t *testing.T) {
	for _, tt := range []struct {
		name  string
		left  string
		right string
	}{
		{
			name:  "generated column",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT NOT NULL);`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT NOT NULL, body_len INTEGER GENERATED ALWAYS AS (length(body)) VIRTUAL);`,
		},
		{
			name:  "autoincrement",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT NOT NULL);`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT NOT NULL);`,
		},
		{
			name:  "collation",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT NOT NULL);`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT COLLATE NOCASE NOT NULL);`,
		},
		{
			name:  "on conflict",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT UNIQUE NOT NULL);`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT UNIQUE ON CONFLICT REPLACE NOT NULL);`,
		},
		{
			name: "deferrable foreign key",
			left: `
				CREATE TABLE parents (id INTEGER PRIMARY KEY);
				CREATE TABLE items (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parents(id));
			`,
			right: `
				CREATE TABLE parents (id INTEGER PRIMARY KEY);
				CREATE TABLE items (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parents(id) DEFERRABLE INITIALLY DEFERRED);
			`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			left := openSchema(t, tt.left)
			defer left.Close()
			right := openSchema(t, tt.right)
			defer right.Close()

			leftShape, err := inspectShape(context.Background(), left)
			if err != nil {
				t.Fatal(err)
			}
			rightShape, err := inspectShape(context.Background(), right)
			if err != nil {
				t.Fatal(err)
			}
			if leftShape.Signature == rightShape.Signature {
				t.Fatalf("%s DDL difference collided at signature %s", tt.name, leftShape.Signature)
			}
		})
	}
}

func TestConservativeSignaturePreservesWhitespaceInsideQuotedSQL(t *testing.T) {
	for _, tt := range []struct {
		name  string
		left  string
		right string
	}{
		{
			name:  "default quoted literal",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT DEFAULT 'alpha  beta');`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT DEFAULT 'alpha beta');`,
		},
		{
			name:  "check quoted literal",
			left:  `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT CHECK (body <> 'alpha  beta'));`,
			right: `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT CHECK (body <> 'alpha beta'));`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			left := openSchema(t, tt.left)
			defer left.Close()
			right := openSchema(t, tt.right)
			defer right.Close()

			leftShape, err := inspectShape(context.Background(), left)
			if err != nil {
				t.Fatal(err)
			}
			rightShape, err := inspectShape(context.Background(), right)
			if err != nil {
				t.Fatal(err)
			}
			if leftShape.Signature == rightShape.Signature {
				t.Fatalf("quoted whitespace difference collided at signature %s", leftShape.Signature)
			}
		})
	}
}

func TestConservativeSignatureStableForExactSameSQL(t *testing.T) {
	schemaSQL := `CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT DEFAULT 'alpha  beta' CHECK (body <> 'gamma  delta'));`
	left := openSchema(t, schemaSQL)
	defer left.Close()
	right := openSchema(t, schemaSQL)
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
		t.Fatalf("exact same SQL signature differs: %s != %s", leftShape.Signature, rightShape.Signature)
	}
}

func TestInspectIndexRecognizesObservedDevelopmentV13Shape(t *testing.T) {
	db := openFixtureCopy(t, "v13-development-alter-built.sql")
	defer db.Close()

	shape, err := inspectShape(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	// Signature recomputed after normalizeSQL made whitespace-insensitive (issue #52)
	const observedSignature = "sha256:4d04377754986f0da2f61f2ab73889168f596eb84e1f02df96e23072072e9375"
	if shape.Signature != observedSignature {
		t.Fatalf("development V13 signature = %s, want %s", shape.Signature, observedSignature)
	}

	plan, diag, err := InspectIndex(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}
	if plan.From.AppliedVersion != 13 || len(plan.Steps) != 0 {
		t.Fatalf("development V13 plan = %+v, want current with no steps", plan)
	}
}

func TestObservedDevelopmentV13StillRejectsUnknownAlteration(t *testing.T) {
	db := openFixtureCopy(t, "v13-development-alter-built.sql")
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE unexpected_development_shape (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	_, diag, err := InspectIndex(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if diag == nil || diag.Code != CodeUnsupportedLineage {
		t.Fatalf("diagnostic = %+v, want %s", diag, CodeUnsupportedLineage)
	}
}

func TestInspectIndexRecognizesCanonicalAndExplicitLegacyV13Shapes(t *testing.T) {
	for _, fixture := range []string{"v13.sql", "v13-legacy-alter-built.sql", "v13-legacy-existing-schema-migrations.sql"} {
		t.Run(fixture, func(t *testing.T) {
			db := openFixtureCopy(t, fixture)
			defer db.Close()

			plan, diag, err := InspectIndex(context.Background(), db)
			if err != nil || diag != nil {
				t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
			}
			if plan.From.AppliedVersion != 13 {
				t.Fatalf("applied version = %d, want 13", plan.From.AppliedVersion)
			}
			if len(plan.Steps) != 0 {
				t.Fatalf("V13-compatible shape has pending steps: %+v", plan.Steps)
			}
		})
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

func TestNormalizeSQLIsWhitespaceInsensitiveOutsideStringLiterals(t *testing.T) {
	tests := []struct {
		name string
		sql1 string
		sql2 string
	}{
		{
			name: "multiple spaces collapsed to single space",
			sql1: "CREATE   TABLE  foo  (id  INTEGER)",
			sql2: "CREATE TABLE foo (id INTEGER)",
		},
		{
			name: "newlines and tabs collapsed to single space",
			sql1: "CREATE\n  TABLE\tfoo\n(\nid\nINTEGER\n)",
			sql2: "CREATE TABLE foo ( id INTEGER )",
		},
		{
			name: "alter-built schema: inline vs multiline columns normalize the same",
			sql1: "CREATE TABLE search_items (\n  content_type TEXT DEFAULT 'text',\n  extraction_version INTEGER,\n  was_interrupted INTEGER\n);",
			sql2: "CREATE TABLE search_items ( content_type TEXT DEFAULT 'text', extraction_version INTEGER, was_interrupted INTEGER );",
		},
		{
			name: "preserve space inside single-quoted literal",
			sql1: "CREATE TABLE foo (body TEXT DEFAULT 'alpha  beta')",
			sql2: "CREATE TABLE foo (body TEXT DEFAULT 'alpha  beta')",
		},
		{
			name: "double-single-quote escape inside literal",
			sql1: "CREATE TABLE foo (body TEXT DEFAULT 'O''Brien')",
			sql2: "CREATE TABLE foo (body TEXT DEFAULT 'O''Brien')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			norm1 := normalizeSQL(tt.sql1)
			norm2 := normalizeSQL(tt.sql2)
			if norm1 != norm2 {
				t.Fatalf("normalization differs: %q != %q", norm1, norm2)
			}
		})
	}
}

func TestNormalizeSQLPreservesWhitespaceInStringLiterals(t *testing.T) {
	// This is critical: whitespace inside string literals must be preserved exactly
	sql := "CREATE TABLE foo (body TEXT DEFAULT 'alpha  beta', CHECK (body <> 'gamma  delta'))"
	norm := normalizeSQL(sql)
	if !strings.Contains(norm, "'alpha  beta'") {
		t.Fatalf("whitespace in first literal not preserved: %q", norm)
	}
	if !strings.Contains(norm, "'gamma  delta'") {
		t.Fatalf("whitespace in second literal not preserved: %q", norm)
	}
}
