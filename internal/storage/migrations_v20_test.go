package storage

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestV20RetainsCanonicalLegacyPayloadAndFingerprint(t *testing.T) {
	path := createFixtureDatabase(t, "v19.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`INSERT INTO indexed_files(path,hash,ingestion_fingerprint) VALUES('/expired.db','retained-hash','retained-fingerprint');
 INSERT INTO search_items(source_path,ordinal,role,text,uuid,project,content_type,extraction_version) VALUES('/expired.db',9,'user','retained payload','legacy-native-unknown','legacy','text',1);
 INSERT INTO session_tags VALUES('/expired.db','testing');`); err != nil {
		t.Fatal(err)
	}
	plan, diagnostic, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diagnostic != nil || len(plan.Steps) != 2 || plan.Steps[0].Version != 20 || plan.Steps[1].Version != 21 {
		t.Fatalf("V19 plan=%+v diagnostic=%+v err=%v", plan, diagnostic, err)
	}
	if err := applyMigrationPlanForTest(context.Background(), db, plan); err != nil {
		t.Fatal(err)
	}
	var session sql.NullString
	var text, uuid, fingerprint, hash string
	var ordinal, epoch int
	if err := db.DB().QueryRow(`SELECT si.session_id,si.text,si.uuid,si.ordinal,si.extraction_version,f.hash,f.ingestion_fingerprint FROM search_items si JOIN indexed_files f ON f.path=si.source_path WHERE si.uuid='legacy-native-unknown'`).Scan(&session, &text, &uuid, &ordinal, &epoch, &hash, &fingerprint); err != nil {
		t.Fatal(err)
	}
	if session.Valid || text != "retained payload" || uuid != "legacy-native-unknown" || ordinal != 9 || epoch != 1 || hash != "retained-hash" || fingerprint != "retained-fingerprint" {
		t.Fatalf("retained canonical evidence: session=%+v text=%s uuid=%s ordinal=%d epoch=%d hash=%s fingerprint=%s", session, text, uuid, ordinal, epoch, hash, fingerprint)
	}
	var tag string
	if err := db.DB().QueryRow("SELECT tag FROM scoped_session_tags WHERE source_path='/expired.db' AND session_id IS NULL").Scan(&tag); err != nil || tag != "testing" {
		t.Fatalf("legacy scoped tag=%q err=%v", tag, err)
	}
	assertCurrentShape(t, db.DB())
	assertMigrationVersionCount(t, db.DB(), 20, 1)
}
