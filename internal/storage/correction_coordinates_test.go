package storage

import (
	"path/filepath"
	"testing"
)

// Exercise the production maintenance APIs against a real temporary database.
// Detector keys are slice positions; only loaded messages own persisted coordinates.
func TestCorrectionMaintenancePreservesPersistedCoordinates(t *testing.T) {
	for _, mode := range []string{"backfill", "rederive"} {
		for _, identity := range []string{"uuid", "uuid-less", "ambiguous-uuid-less"} {
			t.Run(mode+"/"+identity, func(t *testing.T) {
				db, err := Open(filepath.Join(t.TempDir(), "index.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				const path = "/expired/correction.jsonl"
				var uuid any
				wantUUID := ""
				if identity == "uuid" {
					uuid = "canonical-message"
					wantUUID = uuid.(string)
				}
				if _, err := db.db.Exec(`INSERT INTO search_items
     (source, source_path, ordinal, role, text, uuid, content_type)
     VALUES ('session', ?, 12, 'user', 'no, te pedí otra cosa', ?, 'text')`, path, uuid); err != nil {
					t.Fatal(err)
				}
				// A later tool row must remain filtered out of detector input.
				if _, err := db.db.Exec(`INSERT INTO search_items
     (source, source_path, ordinal, role, text, uuid, content_type)
     VALUES ('session', ?, 40, 'user', 'no, te pedí otra cosa', 'tool-row', 'tool')`, path); err != nil {
					t.Fatal(err)
				}
				if identity == "ambiguous-uuid-less" || identity == "uuid" {
					if _, err := db.db.Exec(`INSERT INTO search_items
      (source, source_path, ordinal, role, text, uuid, content_type)
      VALUES ('session', ?, 12, 'assistant', 'Understood', 'other-message', 'text')`, path); err != nil {
						t.Fatal(err)
					}
				}
				loaded, err := db.LoadMessagesForPath(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(loaded) == 0 || loaded[0].Ordinal != 12 || loaded[0].UUID != wantUUID {
					t.Fatalf("slice index zero must hold the canonical candidate: %+v", loaded)
				}
				if mode == "rederive" {
					if _, err := db.db.Exec(`INSERT INTO correction_signals
      (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
      VALUES (?, ?, 12, 'lexicon', 0.8, ?)`, uuid, path, CurrentExtractionVersion-1); err != nil {
						t.Fatal(err)
					}
				}
				run := func() int {
					t.Helper()
					if mode == "backfill" {
						if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
							t.Fatal(err)
						}
						return 0
					}
					n, err := db.RederiveSupersededCorrections(10)
					if err != nil {
						t.Fatal(err)
					}
					return n
				}
				if n := run(); mode == "rederive" && n != 1 {
					t.Fatalf("processed %d paths, want 1", n)
				}
				check := func() int {
					t.Helper()
					var total, invalid int
					if err := db.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE
      WHEN ordinal != 12 OR COALESCE(item_uuid, '') != ? OR extraction_version != ?
      THEN 1 ELSE 0 END), 0) FROM correction_signals WHERE source_path = ?`,
						wantUUID, CurrentExtractionVersion, path).Scan(&total, &invalid); err != nil {
						t.Fatal(err)
					}
					if total == 0 || invalid != 0 {
						t.Fatalf("signals=%d invalid coordinates/identity/epoch=%d; want ordinal 12, UUID %q", total, invalid, wantUUID)
					}
					candidates, err := db.AggregateCorrections(CorrectionAggOpts{})
					if err != nil {
						t.Fatal(err)
					}
					if identity == "ambiguous-uuid-less" {
						if len(candidates) != 0 {
							t.Fatalf("ambiguous UUID-less join leaked candidates: %+v", candidates)
						}
					} else if len(candidates) != 1 || candidates[0].Ordinal != 12 || candidates[0].UUID != wantUUID {
						t.Fatalf("canonical join lost or misattributed candidate: %+v", candidates)
					}
					return total
				}
				before := check()
				if n := run(); mode == "rederive" && n != 0 {
					t.Fatalf("rerun processed %d paths, want convergence", n)
				}
				if after := check(); after != before {
					t.Fatalf("rerun changed signals: %d -> %d", before, after)
				}
			})
		}
	}
}
