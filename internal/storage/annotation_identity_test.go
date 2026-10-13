package storage

import (
	"errors"
	"testing"
)

func TestAnnotationExactIdentity(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	for _, u := range []string{"opaque one", "opaque two"} {
		if _, err := db.db.Exec(`INSERT INTO search_items(source, source_path, ordinal, role, text, content_type, uuid) VALUES ('session','/duplicate',7,'user','hello','text',?)`, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertAnnotation("opaque one", "", -1, "correction", "original"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, u, p string
		n          int
		want       error
	}{
		{"ambiguous", "", "/duplicate", 7, ErrContextRecordAmbiguous},
		{"missing uuid", "absent", "", -1, ErrContextRecordNotFound},
		{"missing coordinate", "", "/absent", 7, ErrContextRecordNotFound},
		{"conflicting path", "opaque one", "/wrong", -1, nil},
		{"conflicting ordinal", "opaque one", "", 8, nil},
		{"conflicting pair", "opaque one", "/wrong", 8, nil},
		{"missing identity", "", "", -1, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := db.UpsertAnnotation(c.u, c.p, c.n, "correction", "rejected")
			if err == nil || c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			var count int
			var label string
			if err := db.db.QueryRow(`SELECT COUNT(*),label FROM annotations`).Scan(&count, &label); err != nil {
				t.Fatal(err)
			}
			if count != 1 || label != "original" {
				t.Fatalf("annotations changed: %d %s", count, label)
			}
		})
	}
	for _, c := range []struct {
		p string
		n int
	}{{"", -1}, {"/duplicate", -1}, {"", 7}, {"/duplicate", 7}} {
		if err := db.UpsertAnnotation("opaque two", c.p, c.n, "correction", "updated"); err != nil {
			t.Fatal(err)
		}
	}
	var uuid, path, label string
	var n int
	if err := db.db.QueryRow(`SELECT item_uuid,source_path,ordinal,label FROM annotations`).Scan(&uuid, &path, &n, &label); err != nil {
		t.Fatal(err)
	}
	if uuid != "opaque two" || path != "/duplicate" || n != 7 || label != "updated" {
		t.Fatalf("noncanonical annotation: %s %s %d %s", uuid, path, n, label)
	}
}
