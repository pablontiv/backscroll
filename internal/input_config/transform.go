package input_config

import (
	"errors"
	"regexp"
	"strings"
)

// ErrDropped is returned by ApplyTransforms when drop_empty drops the text.
var ErrDropped = errors.New("content dropped (drop_empty)")

// ApplyTransforms applies the transforms from TextConfig to the input text in order.
// Returns ("", ErrDropped) when drop_empty is set and the result is blank.
func ApplyTransforms(cfg TextConfig, text string) (string, error) {
	for _, r := range cfg.Remove {
		var err error
		text, err = applyRemove(r, text)
		if err != nil {
			return "", err
		}
	}

	if cfg.Trim {
		text = strings.TrimSpace(text)
	}

	if cfg.Join != "" {
		text = collapseWhitespace(text, cfg.Join)
	}

	if cfg.DropEmpty && strings.TrimSpace(text) == "" {
		return "", ErrDropped
	}

	return text, nil
}

func applyRemove(r RemoveConfig, text string) (string, error) {
	switch r.Kind {
	case "regex":
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return "", &InvalidPatternError{Pattern: r.Pattern, Err: err}
		}
		return re.ReplaceAllString(text, ""), nil
	case "substring":
		return strings.ReplaceAll(text, r.Pattern, ""), nil
	default:
		// Treat unknown kind as substring for forward compat
		return strings.ReplaceAll(text, r.Pattern, ""), nil
	}
}

// collapseWhitespace replaces runs of whitespace (including newlines) with sep.
func collapseWhitespace(text, sep string) string {
	// Collapse sequences of whitespace-only lines into the separator
	lines := strings.Split(text, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		out = append(out, l)
	}
	if sep == "\n" {
		return strings.Join(out, "\n")
	}
	// For non-newline separators, collapse all whitespace
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(strings.Join(out, "\n"), sep)
}

// InvalidPatternError is returned when a regex pattern is invalid.
type InvalidPatternError struct {
	Pattern string
	Err     error
}

func (e *InvalidPatternError) Error() string {
	return "invalid regex pattern " + e.Pattern + ": " + e.Err.Error()
}

func (e *InvalidPatternError) Unwrap() error {
	return e.Err
}
