package readers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const hashBufferSize = 32 * 1024

func hashFile(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hashReader(ctx, file)
}

func hashReader(ctx context.Context, reader io.Reader) (string, error) {
	h := sha256.New()
	buffer := make([]byte, hashBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := reader.Read(buffer)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if err == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if err != nil {
			return "", err
		}
	}
}
