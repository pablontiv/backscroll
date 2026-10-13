package storage

import (
	"fmt"
	"testing"
)

func TestFailureCensusEligibilityAndPagination(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	for i, stamp := range []string{"2026-01-05T00:00:00Z", "2026-01-05T00:00:00Z", "2026-01-05T00:00:00Z", "2026-01-12T00:00:00Z", "2026-01-12T00:00:00Z", "2026-01-19T00:00:00Z", ""} {
		uuid, path := fmt.Sprint("c-", i), fmt.Sprint("/c-", i)
		eligibilityItem(t, db, uuid, path, "p", stamp, 0)
		eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,is_error,extraction_version) VALUES (?,?,0,?,?,1)`, uuid, path, fmt.Sprint("Tool", i), []any{1, 0, nil, 1, nil, 0, 0}[i])
		eligibilityExec(t, db, `INSERT INTO session_tags VALUES (?,'keep'),(?,'other')`, path, path)
	}
	// A coordinate peer must not fan out or lend scope.
	eligibilityItem(t, db, "peer", "/c-0", "wrong", "2026-01-05T00:00:00Z", 0)
	for _, tc := range []struct {
		opts          AggregateOptions
		total, signal int
	}{
		{AggregateOptions{}, 7, 5},
		{AggregateOptions{Project: "p", Tag: "keep", Limit: 1, Offset: 1}, 7, 5},
		{AggregateOptions{Project: "wrong"}, 0, 0},
		{AggregateOptions{Tag: "missing"}, 0, 0},
		{AggregateOptions{StartDate: "2026-01-05T00:00:00Z", EndDate: "2026-01-12T00:00:00Z"}, 3, 2},
	} {
		c, err := db.FailureCensus(tc.opts)
		if err != nil {
			t.Fatal(err)
		}
		if c.TotalEvents != tc.total || c.SignalledEvents != tc.signal || c.UnsignalledEvents != tc.total-tc.signal {
			t.Errorf("%+v: %+v", tc.opts, c)
		}
	}
	opts := AggregateOptions{Project: "p", Tag: "keep", Limit: 1, Offset: 1}
	var excluded int
	rows, err := db.AggregateFailuresTrend(opts, &excluded)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Week != "2026-W01" || rows[0].SignalledEvents != 2 || excluded != 1 {
		t.Errorf("paged trend=%+v excluded=%d", rows, excluded)
	}
	weeks, err := db.FailureCensusTrend(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 3 || weeks[0].TotalEvents != 1 || weeks[1].SignalledEvents != 1 || weeks[2].UnsignalledEvents != 1 {
		t.Errorf("weekly census=%+v", weeks)
	}
	opts.StartDate = "2026-01-12T00:00:00Z"
	opts.EndDate = "2026-01-19T00:00:00Z"
	weeks, err = db.FailureCensusTrend(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 1 || weeks[0].Week != "2026-W02" || weeks[0].TotalEvents != 2 {
		t.Errorf("bounded weeks=%+v", weeks)
	}
	_, err = db.AggregateFailuresTrend(opts, &excluded)
	if err != nil {
		t.Fatal(err)
	}
	if excluded != 1 {
		t.Errorf("bounded undated diagnostic=%d", excluded)
	}
}
