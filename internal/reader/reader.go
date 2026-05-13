package reader

import (
	"github.com/pablontiv/backscroll/internal/models"
	internalsync "github.com/pablontiv/backscroll/internal/sync"
)

// ReadFile reads a session JSONL file and returns its messages.
// Applies the same noise filtering and content extraction as the sync pipeline.
func ReadFile(path string) ([]models.Message, error) {
	return internalsync.ParseSessions(path)
}
