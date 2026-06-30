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
