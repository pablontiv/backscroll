package sync

import (
	"regexp"
	"sync"
)

// Noise filter patterns compiled at init time (LazyLock equivalent)
var (
	noisePatternOnce sync.Once
	noisePatterns    *NoisePatterns
)

// NoisePatterns holds compiled regex patterns for noise filtering
type NoisePatterns struct {
	// Type patterns that should be filtered
	FilteredTypes map[string]bool

	// Content patterns for additional filtering
	SystemReminderPattern   *regexp.Regexp
	TaskNotificationPattern *regexp.Regexp
	CaveatPattern           *regexp.Regexp
	CommandPattern          *regexp.Regexp
}

// GetNoisePatterns returns the lazily-initialized noise patterns
func GetNoisePatterns() *NoisePatterns {
	noisePatternOnce.Do(func() {
		noisePatterns = &NoisePatterns{
			FilteredTypes: map[string]bool{
				"system-reminder":      true,
				"task-notification":    true,
				"command":              true,
				"command-result":       true,
				"local-command-caveat": true,
				"progress":             true,
			},
			// These are compiled but not actively used in the current implementation
			// They're here for potential future use
			SystemReminderPattern:   regexp.MustCompile(`<system-reminder>.*?</system-reminder>`),
			TaskNotificationPattern: regexp.MustCompile(`<task-notification>.*?</task-notification>`),
			CaveatPattern:           regexp.MustCompile(`Caveat:.*?(?:\n|$)`),
			CommandPattern:          regexp.MustCompile(`<(?:command|local-command-caveat)>.*?</(?:command|local-command-caveat)>`),
		}
	})
	return noisePatterns
}
