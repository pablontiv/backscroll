package sync

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// IterateJSONLFile calls fn once for each non-empty JSONL line in path.
// It uses bufio.Reader instead of bufio.Scanner so large JSON records are not
// constrained by Scanner's default token limit.
func IterateJSONLFile(path string, fn func(lineNumber int, line []byte) error) error {
	return IterateJSONLFileContext(context.Background(), path, fn)
}

// IterateJSONLFileContext is IterateJSONLFile with cancellation checks around
// file I/O and between records.
func IterateJSONLFileContext(ctx context.Context, path string, fn func(lineNumber int, line []byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return iterateJSONL(ctx, file, path, fn)
}

func iterateJSONL(ctx context.Context, stream io.Reader, source string, fn func(lineNumber int, line []byte) error) error {
	reader := bufio.NewReader(stream)
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := reader.ReadBytes('\n')
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if len(line) > 0 {
			lineNumber++
			line = bytes.TrimSuffix(line, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) > 0 {
				if fnErr := fn(lineNumber, line); fnErr != nil {
					return fnErr
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read line %d from %s: %w", lineNumber+1, source, err)
		}
	}
}
