package readers

import (
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	bsync "github.com/pablontiv/backscroll/internal/sync"
)

// JsonlReader implements SessionReader for JSONL session files.
type JsonlReader struct{}

func (r *JsonlReader) Name() string { return "jsonl" }

// Discover returns the paths of JSONL files matching the input definition's discover config.
func (r *JsonlReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

// Hash returns the SHA-256 hex hash of the file at the given path.
func (r *JsonlReader) Hash(path string) (string, error) {
	return bsync.HashFile(path)
}

// Parse reads a JSONL session file and returns its messages as a ParsedFile.
// When the InputDefinition has MapConfig selectors set, it uses the declarative pipeline.
// Otherwise it falls back to the legacy ParseSessions parser.
func (r *JsonlReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := bsync.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	if def.Map.Role != "" {
		msgs, err = input_config.ParseDeclarative(path, def)
	} else {
		msgs, err = bsync.ParseSessions(path)
	}
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{
		Path:    path,
		Hash:    hash,
		Records: msgs,
	}, nil
}
