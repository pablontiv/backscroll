package autoupdate

import (
	"context"
	"fmt"
	"io"
)

// Limits comfortably exceed normal Go CLI releases, but bound heap and archive
// expansion independently of transport timeouts. See README.md.
const (
	maxLatestBytes     int64 = 1 << 20
	maxChecksumBytes   int64 = 4 << 20
	maxArchiveBytes    int64 = 128 << 20
	maxExpandedBytes   int64 = 512 << 20
	maxExecutableBytes int64 = 128 << 20
)

type byteBudget struct {
	r         io.Reader
	remaining int64
	label     string
}

func (b *byteBudget) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, fmt.Errorf("%s exceeds byte limit", b.label)
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.r.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, fmt.Errorf("%s exceeds byte limit", b.label)
	}
	return n, err
}
func readBounded(ctx context.Context, r io.Reader, limit int64, label string) ([]byte, error) {
	return io.ReadAll(contextReader{ctx, &byteBudget{r, limit, label}})
}
