package input_config

// InputFile represents a parsed *.inputs.toml file.
type InputFile struct {
	Version int               `toml:"version"`
	Inputs  []InputDefinition `toml:"inputs"`
}

// InputDefinition is a single [[inputs]] entry.
type InputDefinition struct {
	ID       string         `toml:"id"`
	Source   string         `toml:"source"`
	Active   bool           `toml:"active"`
	Discover DiscoverConfig `toml:"discover"`
	Decode   DecodeConfig   `toml:"decode"`
	Record   RecordConfig   `toml:"record"`
	Map      MapConfig      `toml:"map"`
	Content  ContentConfig  `toml:"content"`
	Text     TextConfig     `toml:"text"`
}

// DiscoverConfig controls which files are discovered for this input.
type DiscoverConfig struct {
	Roots          []string `toml:"roots"`
	Include        []string `toml:"include"`
	Exclude        []string `toml:"exclude"`
	FollowSymlinks bool     `toml:"follow_symlinks"`
}

// DecodeConfig specifies how to decode discovered files.
type DecodeConfig struct {
	Format string `toml:"format"` // "jsonl", "json", "markdown", "sqlite"
}

// RecordConfig controls which records within a decoded file are accepted.
type RecordConfig struct {
	Selector    string      `toml:"selector"`
	IncludeWhen []Predicate `toml:"include_when"`
	ExcludeWhen []Predicate `toml:"exclude_when"`
}

// Predicate filters records based on a field value.
type Predicate struct {
	Selector string `toml:"selector"`
	Op       string `toml:"op"`    // "eq", "ne", "in", "exists", "missing"
	Value    any    `toml:"value"` // string | bool | float64 | []any
}

// MapConfig extracts metadata fields from a record using JSONPath selectors.
type MapConfig struct {
	Role      string `toml:"role"`
	UUID      string `toml:"uuid"`
	Timestamp string `toml:"timestamp"`
	SessionID string `toml:"session_id"`
	Project   string `toml:"project"`
}

// ContentConfig extracts content blocks from a record.
type ContentConfig struct {
	Selector           string      `toml:"selector"`
	String             string      `toml:"string"`
	Blocks             string      `toml:"blocks"`
	BlockText          string      `toml:"block_text"`
	ContentType        string      `toml:"content_type"`
	IncludeWhen        []Predicate `toml:"include_when"`
	DefaultContentType string      `toml:"default_content_type"`
}

// TextConfig post-processes the extracted text content.
type TextConfig struct {
	Join      string         `toml:"join"`
	Trim      bool           `toml:"trim"`
	DropEmpty bool           `toml:"drop_empty"`
	Remove    []RemoveConfig `toml:"remove"`
}

// RemoveConfig removes text matching a pattern.
type RemoveConfig struct {
	Kind    string `toml:"kind"` // "regex" or "substring"
	Pattern string `toml:"pattern"`
}
