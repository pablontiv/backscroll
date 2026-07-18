package categories

import (
	"regexp"
)

type Rule struct {
	Pattern  *regexp.Regexp
	Tool     string
	Category string
}

type Mapper struct {
	version int
	rules   []Rule
}

func (m *Mapper) Categorize(toolName, commandHead string) string {
	// Single pass: check each rule in order
	// A rule matches if:
	//   - Tool is set AND equals toolName AND (Pattern is not set OR Pattern matches input)
	//   - OR Tool is not set AND Pattern matches input
	input := toolName
	if commandHead != "" {
		input = toolName + " " + commandHead
	}

	for _, r := range m.rules {
		if r.Tool != "" {
			// Exact tool match required, pattern optional
			if r.Tool == toolName {
				if r.Pattern == nil || r.Pattern.MatchString(input) {
					return r.Category
				}
			}
		} else if r.Pattern != nil {
			// Pattern-only match
			if r.Pattern.MatchString(input) {
				return r.Category
			}
		}
	}

	// Fallthrough: passthrough tool name as category
	return toolName
}

func (m *Mapper) Version() int {
	return m.version
}
