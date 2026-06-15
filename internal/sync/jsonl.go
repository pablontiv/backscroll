package sync

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// IterateJSONLFile calls fn once for each non-empty JSONL line in path.
// It uses bufio.Reader instead of bufio.Scanner so large JSON records are not
// constrained by Scanner's default token limit.
func IterateJSONLFile(path string, fn func(lineNumber int, line []byte) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)
	lineNumber := 0
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNumber++
			line = bytes.TrimSuffix(line, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) > 0 {
				if fnErr := fn(lineNumber, line); fnErr != nil {
					return fnErr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read line %d from %s: %w", lineNumber+1, path, err)
		}
	}
}
