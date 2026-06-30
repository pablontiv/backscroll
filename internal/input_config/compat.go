package input_config

import (
	"fmt"
)

// SessionDirsToManifest generates an implicit input manifest with Decode.Format="claude"
// from a list of session directories, routing to ClaudeReader for parsing.
// This provides backward compatibility for configs that use session_dirs rather than
// declarative *.inputs.toml files. Record/Map/Content/Text fields are retained for
// backward compatibility but are ignored by ClaudeReader (slated for removal in Slice 4).
func SessionDirsToManifest(dirs []string) InputDefinition {
	return InputDefinition{
		ID:     "legacy-session-dirs",
		Source: "session",
		Active: true,
		Discover: DiscoverConfig{
			Roots:          dirs,
			Include:        []string{"**/*.jsonl"},
			Exclude:        []string{"**/subagents/**"},
			FollowSymlinks: false,
		},
		Decode: DecodeConfig{Format: "claude"},
		Record: RecordConfig{
			Selector: "$",
			IncludeWhen: []Predicate{
				{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
			},
			ExcludeWhen: []Predicate{
				{Selector: "$.isMeta", Op: "eq", Value: true},
			},
		},
		Map: MapConfig{
			Role:      "$.message.role",
			UUID:      "$.uuid",
			Timestamp: "$.timestamp",
			SessionID: "$.sessionId",
		},
		Content: ContentConfig{
			Selector:           "$.message.content",
			String:             "$",
			Blocks:             "$.message.content[*]",
			BlockText:          "$.text",
			ContentType:        "$.type",
			DefaultContentType: "text",
			IncludeWhen: []Predicate{
				{Selector: "$.type", Op: "eq", Value: "text"},
			},
		},
		Text: TextConfig{
			Join:      "\n",
			Trim:      true,
			DropEmpty: true,
		},
	}
}

// ActiveInputs returns the active inputs to use, applying the following priority:
//  1. If declarative inputs are available in InputsDir, use them.
//  2. If sessionDirs is non-empty, generate a legacy compat manifest.
//  3. If neither is available, return an error.
//
// mode is set to indicate which source was used.
func ActiveInputs(sessionDirs []string) ([]InputDefinition, InputMode, error) {
	defs, err := LoadInputs()
	if err != nil {
		return nil, ModeUnknown, fmt.Errorf("load inputs: %w", err)
	}
	if len(defs) > 0 {
		return defs, ModeDeclarative, nil
	}

	if len(sessionDirs) > 0 {
		return []InputDefinition{SessionDirsToManifest(sessionDirs)}, ModeLegacy, nil
	}

	return nil, ModeUnknown, nil
}

// InputMode indicates how inputs were resolved.
type InputMode int

const (
	ModeUnknown     InputMode = 0
	ModeDeclarative InputMode = 1
	ModeLegacy      InputMode = 2
)

func (m InputMode) String() string {
	switch m {
	case ModeDeclarative:
		return "declarative"
	case ModeLegacy:
		return "legacy (session_dirs)"
	default:
		return "unknown"
	}
}
