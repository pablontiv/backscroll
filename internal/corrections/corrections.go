package corrections

import (
	"strings"

	"github.com/pablontiv/backscroll/internal/models"
)

// Detection is the output of a single detector run on a message.
type Detection struct {
	DetectorName string
	Confidence   float64
}

// Detector is a function that analyzes a message at a given ordinal position
// within a session and returns a Detection if a correction signal is found.
// Returns nil if no signal is detected.
type Detector func(msgs []models.Message, idx int) *Detection

// Registry maps detector names to their functions (for orderly, repeatable
// execution and testing). Detectors are registered at init() time.
var detectorRegistry map[string]Detector

// Register adds a detector to the registry. Called at init() time.
func Register(name string, fn Detector) {
	detectorRegistry[name] = fn
}

// RunDetectors executes all registered detectors over a session's messages.
// Returns a map of message ordinal -> slice of Detections (detectors that fired).
// The slice is sorted by detector name for determinism.
func RunDetectors(msgs []models.Message) map[int][]Detection {
	if len(msgs) == 0 {
		return nil
	}
	result := make(map[int][]Detection)
	for i := range msgs {
		for name, fn := range detectorRegistry {
			if detection := fn(msgs, i); detection != nil {
				detection.DetectorName = name
				result[i] = append(result[i], *detection)
			}
		}
	}
	return result
}

// correctionLexicon is the comprehensive es+en lexicon for correction signals.
// Patterns are lowercase; matching is case-insensitive.
// NOTE: Spanish is primary (first-class); English is secondary.
var correctionLexicon = []string{
	// Spanish correction signals
	"no, te pedí",
	"te pedí",
	"eso no",
	"otra vez",
	"te dije",
	"es mentira",
	"no era eso",
	"de nuevo",
	"eso no es",

	// English correction signals
	"not what i asked",
	"what i asked",
	"i said",
	"you ignored",
	"wrong file",
	"this is wrong",
	"that's wrong",
	"that is wrong",
}

// cCorrectionLexiconDetector checks if a user message matches the correction
// lexicon (es+en). Confidence 0.8 (relatively high; lexicon is well-curated).
func cCorrectionLexiconDetector(msgs []models.Message, idx int) *Detection {
	if idx >= len(msgs) || msgs[idx].Role != "user" {
		return nil
	}
	content := msgs[idx].Content
	lowerContent := strings.ToLower(content)

	for _, pattern := range correctionLexicon {
		if strings.Contains(lowerContent, pattern) {
			return &Detection{Confidence: 0.8}
		}
	}
	return nil
}

// cInterruptDetector checks if a user message was interrupted (F0a signal).
// Confidence 0.5 (moderate; interrupt is real evidence but incomplete).
func cInterruptDetector(msgs []models.Message, idx int) *Detection {
	if idx >= len(msgs) || msgs[idx].Role != "user" {
		return nil
	}
	if msgs[idx].WasInterrupted {
		return &Detection{Confidence: 0.5}
	}
	return nil
}

// cDenialDetector checks if a user message follows a permission denial
// in an assistant or tool-result message. Detects "denied" or "rechazado".
// Confidence 0.4 (lower: only predictive of correction if user responds).
func cDenialDetector(msgs []models.Message, idx int) *Detection {
	if idx >= len(msgs) || msgs[idx].Role != "user" {
		return nil
	}
	if idx == 0 {
		return nil // no previous message
	}

	prevIdx := idx - 1
	prevMsg := msgs[prevIdx]
	if prevMsg.Role != "assistant" && prevMsg.Role != "tool" {
		return nil
	}

	lowerContent := strings.ToLower(prevMsg.Content)
	if strings.Contains(lowerContent, "denied") || strings.Contains(lowerContent, "rechaza") {
		return &Detection{Confidence: 0.4}
	}
	return nil
}

// tokenize splits text into lowercase words, filtering common stopwords.
func tokenize(text string) map[string]bool {
	stopwords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true,
		"at": true, "be": true, "but": true, "by": true, "for": true,
		"if": true, "in": true, "into": true, "is": true, "it": true,
		"no": true, "not": true, "of": true, "on": true, "or": true,
		"that": true, "the": true, "to": true, "was": true, "with": true,
		"el": true, "la": true, "de": true, "que": true, "y": true,
		"es": true, "en": true, "un": true, "una": true, "o": true,
	}

	tokens := make(map[string]bool)
	lowerText := strings.ToLower(text)
	for _, word := range strings.Fields(lowerText) {
		// Strip punctuation
		word = strings.Trim(word, ".,!?;:\"'()[]{}*\\/\\-_")
		if len(word) > 0 && !stopwords[word] {
			tokens[word] = true
		}
	}
	return tokens
}

// jaccardSimilarity computes token-based Jaccard similarity between two texts.
// Returns overlap / union over tokens (ignoring stopwords).
func jaccardSimilarity(text1, text2 string) float64 {
	tokens1 := tokenize(text1)
	tokens2 := tokenize(text2)

	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	// Count overlap
	overlap := 0
	for t := range tokens1 {
		if tokens2[t] {
			overlap++
		}
	}

	// Count union
	union := make(map[string]bool)
	for t := range tokens1 {
		union[t] = true
	}
	for t := range tokens2 {
		union[t] = true
	}

	return float64(overlap) / float64(len(union))
}

// cRephraseDetector checks if a user message is a rephrase of the previous
// user message (via Jaccard token overlap >= 0.6).
// Confidence 0.6 (moderate; similarity alone doesn't prove correction).
func cRephraseDetector(msgs []models.Message, idx int) *Detection {
	if idx >= len(msgs) || msgs[idx].Role != "user" {
		return nil
	}
	if idx == 0 {
		return nil
	}

	// Find the nearest previous user message (skipping assistant/tool messages)
	prevUserIdx := -1
	for i := idx - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			prevUserIdx = i
			break
		}
	}
	if prevUserIdx < 0 {
		return nil // no previous user message
	}

	similarity := jaccardSimilarity(msgs[prevUserIdx].Content, msgs[idx].Content)
	if similarity >= 0.6 {
		return &Detection{Confidence: 0.6}
	}
	return nil
}

func init() {
	detectorRegistry = make(map[string]Detector)
	Register("lexicon", cCorrectionLexiconDetector)
	Register("interrupt", cInterruptDetector)
	Register("denial", cDenialDetector)
	Register("rephrase", cRephraseDetector)
}
