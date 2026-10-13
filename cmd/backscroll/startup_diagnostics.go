package main

import (
	"fmt"
	"io"
	"time"
)

// All phase intervals are sequential and measured with this invocation's clock.
// The outer elapsed interval also includes uninstrumented work and lease handoff.
type startupDiagnostics struct {
	start                                           time.Time
	now                                             func() time.Time
	lock, prepare                                   time.Duration
	hasLock, hasPrepare, sync                       bool
	discovery, metadata, hashing, parsing, database time.Duration
	bytesHashed                                     int64
	filesHashed, filesSkipped                       int
}

type startupDiagnosticsContextKey struct{}
type startupDiagnosticsWriterKey struct{}

func (s *startupSyncService) newDiagnostics() *startupDiagnostics {
	now := s.now
	if now == nil {
		now = time.Now
	}
	return &startupDiagnostics{start: now(), now: now}
}

func (d *startupDiagnostics) render(w io.Writer) {
	total := d.now().Sub(d.start)
	measured := d.lock + d.prepare + d.discovery + d.metadata + d.hashing + d.parsing + d.database
	remainder := total - measured
	_, _ = fmt.Fprint(w, "\nStartup diagnostics:\n")
	if d.hasLock {
		_, _ = fmt.Fprintf(w, "  Lock Acquisition:%v\n", d.lock)
	}
	if d.hasPrepare {
		_, _ = fmt.Fprintf(w, "  Index Prepare:   %v\n", d.prepare)
	}
	if d.sync {
		_, _ = fmt.Fprintf(w, "  Discovery:       %v\n", d.discovery)
		_, _ = fmt.Fprintf(w, "  Metadata:        %v (%d files checked)\n", d.metadata, d.filesHashed+d.filesSkipped)
		_, _ = fmt.Fprintf(w, "  Hashing:         %v (%d files hashed, %d files skipped, %.1f MB)\n", d.hashing, d.filesHashed, d.filesSkipped, float64(d.bytesHashed)/(1024*1024))
		_, _ = fmt.Fprintf(w, "  Parsing:         %v\n", d.parsing)
		_, _ = fmt.Fprintf(w, "  Database:        %v\n", d.database)
	}
	// Never hide accounting inconsistencies (including legacy independently supplied
	// phase timings). Overlap is explicitly deducted: phases - overlap + remainder = total.
	if remainder < 0 {
		_, _ = fmt.Fprintf(w, "  Overlap:         %v (deduct from measured phase sum)\n", -remainder)
		remainder = 0
	}
	_, _ = fmt.Fprintf(w, "  Unattributed:    %v (uninstrumented work within this startup interval)\n", remainder)
	_, _ = fmt.Fprintf(w, "  Total:           %v\n", total)
}
