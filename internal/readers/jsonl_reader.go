package readers

import (
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sync"
	"github.com/pablontiv/picokit/hashfile"
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
	return hashfile.HashFile(path)
}

// Parse reads a JSONL session file and returns its messages as a ParsedFile.
// When the InputDefinition has MapConfig selectors set, it uses the declarative pipeline.
// Otherwise it falls back to the legacy ParseSessions parser.
func (r *JsonlReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
	if def.Map.Role != "" {
		msgs, cwd, err = input_config.ParseDeclarativeWithCwd(path, def)
	} else {
		msgs, err = sync.ParseSessions(path)
	}
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{
		Path:    path,
		Hash:    hash,
		Records: msgs,
		Cwd:     cwd,
	}, nil
}
