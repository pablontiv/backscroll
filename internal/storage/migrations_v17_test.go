package storage

import (
	"context"
	"database/sql"
	"github.com/pablontiv/backscroll/internal/compat"
	"testing"
)

func TestV17DefaultsHistoricalProjectEvidenceToUnknown(t *testing.T) {
	path := createFixtureDatabase(t, "v16.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec("INSERT INTO search_items (source,source_path,ordinal,role,text,project,content_type) VALUES ('session','/expired/native.jsonl',0,'user','retained','native-looking-label','text')"); err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diag != nil {
		t.Fatalf("%v %+v", err, diag)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Version != 17 || plan.Steps[1].Version != 18 {
		t.Fatal(plan.Steps)
	}
	if err := applyMigrationPlanForTest(context.Background(), db, plan); err != nil {
		t.Fatal(err)
	}
	var label string
	var proof sql.NullString
	if err := db.DB().QueryRow("SELECT project,project_evidence FROM search_items").Scan(&label, &proof); err != nil {
		t.Fatal(err)
	}
	if label != "native-looking-label" || proof.Valid {
		t.Fatalf("migration guessed provenance: %s %+v", label, proof)
	}
	if _, err := db.DB().Exec("UPDATE search_items SET project_evidence='invented'"); err == nil {
		t.Fatal("invalid proof accepted")
	}
}
