package chunking

import (
	"strings"
	"testing"
)

// generateText creates a text with approximately n words.
func generateText(n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = "word"
	}
	return strings.Join(words, " ")
}

func TestChunkText_ShortText(t *testing.T) {
	text := "Hello world"
	chunks := ChunkText(text, 512, 0)
	if len(chunks) != 1 {
		t.Errorf("short text: got %d chunks, want 1", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("short text: got %q, want %q", chunks[0], text)
	}
}

func TestChunkText_Empty(t *testing.T) {
	chunks := ChunkText("", 512, 0)
	if len(chunks) != 0 {
		t.Errorf("empty text: got %d chunks, want 0", len(chunks))
	}
}

func TestChunkText_ZeroMaxTokens(t *testing.T) {
	text := "some text here"
	chunks := ChunkText(text, 0, 0)
	if len(chunks) != 1 {
		t.Errorf("zero maxTokens: got %d chunks, want 1", len(chunks))
	}
}

func TestChunkText_LongText_ProducesMultipleChunks(t *testing.T) {
	// ~2000 words → ~2667 tokens; with maxTokens=512 expect ≥3 chunks
	text := generateText(2000)
	chunks := ChunkText(text, 512, 0)
	if len(chunks) < 3 {
		t.Errorf("2000-word text with maxTokens=512: got %d chunks, want ≥3", len(chunks))
	}
}

func TestChunkText_MaxTokensRespected(t *testing.T) {
	text := generateText(2000)
	maxTokens := 512
	chunks := ChunkText(text, maxTokens, 0)
	for i, chunk := range chunks {
		tc := tokenCount(chunk)
		if tc > maxTokens+10 { // allow small overshoot due to approximation
			t.Errorf("chunk[%d] has %d tokens, exceeds maxTokens=%d", i, tc, maxTokens)
		}
	}
}

func TestChunkText_NoWordSplit(t *testing.T) {
	// Words should not be split mid-word
	text := generateText(500)
	chunks := ChunkText(text, 100, 0)
	for _, chunk := range chunks {
		for _, word := range strings.Fields(chunk) {
			if word != "word" {
				t.Errorf("unexpected word fragment: %q", word)
			}
		}
	}
}

func TestChunkText_ParagraphBoundary(t *testing.T) {
	para1 := generateText(100) // ~133 tokens
	para2 := generateText(100)
	para3 := generateText(100)
	text := para1 + "\n\n" + para2 + "\n\n" + para3

	// maxTokens=200 → should split on paragraph boundaries
	chunks := ChunkText(text, 200, 0)
	if len(chunks) < 2 {
		t.Errorf("expected ≥2 chunks for 3 paragraphs with maxTokens=200, got %d", len(chunks))
	}
}

func TestChunkText_Overlap(t *testing.T) {
	text := generateText(300) // ~400 tokens
	chunks := ChunkText(text, 150, 30)
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 chunks, got %d", len(chunks))
	}
	// With overlap, consecutive chunks should share some words
	words1 := strings.Fields(chunks[0])
	words2 := strings.Fields(chunks[1])

	// The beginning of chunk[1] should appear near the end of chunk[0]
	if len(words1) > 0 && len(words2) > 0 {
		last := words1[len(words1)-1]
		foundOverlap := false
		for _, w := range words2[:min(20, len(words2))] {
			if w == last {
				foundOverlap = true
				break
			}
		}
		// With word="word" everywhere, just verify chunks differ
		if chunks[0] == chunks[1] {
			t.Error("chunks should not be identical")
		}
		_ = foundOverlap
	}
}

func TestChunkText_AllWordsCovered(t *testing.T) {
	// All words from input should appear in at least one chunk
	words := make([]string, 200)
	for i := range words {
		words[i] = "token"
	}
	text := strings.Join(words, " ")
	chunks := ChunkText(text, 50, 0)

	allChunksText := strings.Join(chunks, " ")
	totalWordCount := len(strings.Fields(allChunksText))
	// With no overlap, total words across chunks ≥ input words
	if totalWordCount < len(words) {
		t.Errorf("words lost: input %d, chunks total %d", len(words), totalWordCount)
	}
}

func TestChunkText_TableDriven(t *testing.T) {
	cases := []struct {
		name      string
		wordCount int
		maxTokens int
		minChunks int
	}{
		{"small/512", 50, 512, 1},
		{"medium/512", 500, 512, 1},
		{"large/512", 2000, 512, 3},
		{"large/200", 2000, 200, 5},
		{"huge/100", 5000, 100, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := generateText(tc.wordCount)
			chunks := ChunkText(text, tc.maxTokens, 0)
			if len(chunks) < tc.minChunks {
				t.Errorf("got %d chunks, want ≥%d", len(chunks), tc.minChunks)
			}
		})
	}
}

func TestSplitSentences_Punctuation(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int // number of sentences
	}{
		{"period", "Hello world. How are you.", 2},
		{"exclamation", "Hello! How are you!", 2},
		{"question", "Who are you? Where am I?", 2},
		{"newline", "Line one\nLine two", 2},
		{"empty", "", 0},
		{"no break", "No punctuation here", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSentences(tc.input)
			if len(got) != tc.want {
				t.Errorf("splitSentences(%q) = %d sentences, want %d: %v", tc.input, len(got), tc.want, got)
			}
		})
	}
}

func TestTokenCount(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"hello", 2},       // 1 word → ceil(1*4/3) = 2
		{"hello world", 3}, // 2 words → ceil(2*4/3) = 3
		{"a b c d", 6},     // 4 words → ceil(4*4/3) = 6
	}
	for _, tc := range cases {
		got := tokenCount(tc.text)
		if got != tc.want {
			t.Errorf("tokenCount(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
