package main

import (
	"fmt"
	"io"
)

// writeOutput checks required formatted payload writes. Advisory stderr
// progress and hints intentionally do not use this helper.
func writeOutput(w io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// requiredOutputWriter retains the first failure for output APIs (notably
// Cobra help) that cannot propagate it. It never wraps advisory stderr.
type requiredOutputWriter struct {
	io.Writer
	err error
}

func (w *requiredOutputWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if w.err == nil {
		w.err = err
	}
	return n, err
}
