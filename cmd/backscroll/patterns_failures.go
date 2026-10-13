package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pablontiv/backscroll/internal/storage"
)

type failureTrendBucket struct {
	Week string `json:"week"`
	storage.FailureCensus
	Patterns []storage.FailurePatternWeekly `json:"patterns"`
}

func writeFailureCensus(w io.Writer, prefix string, c storage.FailureCensus, robot bool) {
	if robot {
		_, _ = fmt.Fprintf(w, "%stotal_events=%d\n%ssignalled_events=%d\n%sunsignalled_events=%d\n", prefix, c.TotalEvents, prefix, c.SignalledEvents, prefix, c.UnsignalledEvents)
	} else {
		_, _ = fmt.Fprintf(w, "Total events: %d\nSignalled events (with error signal): %d\nUnsignalled events: %d\n\n", c.TotalEvents, c.SignalledEvents, c.UnsignalledEvents)
	}
}

func writeFailurePattern(w io.Writer, prefix string, index int, tool string, signal bool, exit *int, count, signalled int, robot bool) {
	ec := "?"
	if exit != nil {
		ec = fmt.Sprint(*exit)
	}
	if robot {
		_, _ = fmt.Fprintf(w, "%stool_name=%s\n%sis_error=%v\n%sexit_code=%s\n%scount=%d\n%scoverage=%d/%d\n", prefix, tool, prefix, signal, prefix, ec, prefix, count, prefix, count, signalled)
	} else {
		_, _ = fmt.Fprintf(w, "%d. %s (is_error=%v, exit_code=%s) — %d occurrences\n", index+1, tool, signal, ec, count)
	}
}

func renderFailurePatterns(stdout, stderr io.Writer, db *storage.Database, opts storage.AggregateOptions, trend, jsonFormat, robotFormat, allProjects bool) (retErr error) {
	// Preserve required-output failures across the extracted renderer.
	w := &requiredOutputWriter{Writer: stdout}
	stdout = w
	defer func() {
		if retErr == nil && w.err != nil {
			retErr = fmt.Errorf("write output: %w", w.err)
		}
	}()
	if trend {
		var excluded int
		rows, err := db.AggregateFailuresTrend(opts, &excluded)
		if err != nil {
			return err
		}
		census, err := db.FailureCensusTrend(opts)
		if err != nil {
			return err
		}
		if excluded > 0 {
			_, _ = fmt.Fprintf(stderr, "trend: %d events excluded (no timestamp)\n", excluded)
		}
		byWeek := map[string][]storage.FailurePatternWeekly{}
		for _, r := range rows {
			byWeek[r.Week] = append(byWeek[r.Week], r)
		}
		buckets := []failureTrendBucket{}
		total := storage.FailureCensus{}
		for _, c := range census {
			patterns := byWeek[c.Week]
			if patterns == nil {
				patterns = []storage.FailurePatternWeekly{}
			}
			buckets = append(buckets, failureTrendBucket{c.Week, c.FailureCensus, patterns})
			total.TotalEvents += c.TotalEvents
			total.SignalledEvents += c.SignalledEvents
			total.UnsignalledEvents += c.UnsignalledEvents
		}
		if jsonFormat {
			if len(rows) == 0 {
				writeSearchHints(stderr, allProjects, true)
			}
			return json.NewEncoder(stdout).Encode(struct {
				Count int    `json:"count"`
				Kind  string `json:"kind"`
				storage.FailureCensus
				Trends []failureTrendBucket `json:"trends"`
			}{len(rows), "failures", total, buckets})
		}
		if robotFormat {
			_, _ = fmt.Fprintln(stdout, "*** Trends ***")
		} else {
			_, _ = fmt.Fprint(stdout, "Trends in Failures (Week-over-Week)\n===================================\n\n")
		}
		writeFailureCensus(stdout, "", total, robotFormat)
		for wi, b := range buckets {
			if robotFormat {
				_, _ = fmt.Fprintf(stdout, "week_%d=%s\n", wi, b.Week)
			} else {
				_, _ = fmt.Fprintln(stdout, b.Week)
			}
			writeFailureCensus(stdout, fmt.Sprintf("week_%d_", wi), b.FailureCensus, robotFormat)
			for i, p := range b.Patterns {
				writeFailurePattern(stdout, fmt.Sprintf("week_%d_result_%d_", wi, i), i, p.ToolName, p.IsError, p.ExitCode, p.Count, p.SignalledEvents, robotFormat)
			}
		}
		if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(rows))
		}
		if len(rows) == 0 {
			if !robotFormat {
				_, _ = fmt.Fprintln(stdout, "No failure patterns found.")
			}
			writeSearchHints(stderr, allProjects, true)
		}
		return nil
	}
	rows, err := db.AggregateFailures(opts)
	if err != nil {
		return err
	}
	census, err := db.FailureCensus(opts)
	if err != nil {
		return err
	}
	if rows == nil {
		rows = []storage.FailurePattern{}
	}
	if jsonFormat {
		if len(rows) == 0 {
			writeSearchHints(stderr, allProjects, true)
		}
		return json.NewEncoder(stdout).Encode(struct {
			Count int    `json:"count"`
			Kind  string `json:"kind"`
			storage.FailureCensus
			Patterns []storage.FailurePattern `json:"patterns"`
		}{len(rows), "failures", census, rows})
	}
	if robotFormat {
		_, _ = fmt.Fprintln(stdout, "*** Failures ***")
	} else {
		_, _ = fmt.Fprint(stdout, "Failure Patterns\n================\n\n")
	}
	writeFailureCensus(stdout, "", census, robotFormat)
	for i, p := range rows {
		writeFailurePattern(stdout, fmt.Sprintf("result_%d_", i), i, p.ToolName, p.IsError, p.ExitCode, p.Count, p.SignalledEvents, robotFormat)
	}
	if robotFormat {
		_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(rows))
	}
	if len(rows) == 0 {
		if !robotFormat {
			_, _ = fmt.Fprintln(stdout, "No failure patterns found.")
		}
		writeSearchHints(stderr, allProjects, true)
	}
	return nil
}
