// Package chunking provides text splitting for embedding generation.
package chunking

import "strings"

// tokenCount estimates the number of tokens in text.
// Approximation: 1 token ≈ 0.75 words (GPT/BERT-family models).
func tokenCount(text string) int {
	words := len(strings.Fields(text))
	if words == 0 {
		return 0
	}
	// ceil(words / 0.75) = ceil(words * 4 / 3)
	return (words*4 + 2) / 3
}

// ChunkText splits text into chunks of at most maxTokens tokens each, with
// optional token overlap between consecutive chunks.
// Split preference order: paragraph boundary > sentence boundary > word boundary.
// Words are never split.
func ChunkText(text string, maxTokens, overlap int) []string {
	if maxTokens <= 0 {
		return []string{text}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if tokenCount(text) <= maxTokens {
		return []string{text}
	}

	// Split into paragraphs first
	paragraphs := splitParagraphs(text)
	chunks := chunkSegments(paragraphs, maxTokens, overlap, splitSentences)
	return chunks
}

// splitParagraphs splits text on blank lines.
func splitParagraphs(text string) []string {
	var paras []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			paras = append(paras, p)
		}
	}
	return paras
}

// splitSentences splits a paragraph into sentences on '. ', '! ', '? ' or newlines.
func splitSentences(para string) []string {
	var sentences []string
	// Use naive sentence splitting on sentence-ending punctuation
	var buf strings.Builder
	runes := []rune(para)
	for i, r := range runes {
		buf.WriteRune(r)
		if (r == '.' || r == '!' || r == '?') && i+1 < len(runes) {
			next := runes[i+1]
			if next == ' ' || next == '\n' {
				s := strings.TrimSpace(buf.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				buf.Reset()
			}
		} else if r == '\n' {
			s := strings.TrimSpace(buf.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

// chunkSegments groups segments (paragraphs or sentences) into chunks ≤ maxTokens.
// If a single segment exceeds maxTokens, it is split by words.
// overlap is the number of tokens to repeat from the previous chunk.
func chunkSegments(segments []string, maxTokens, overlap int, subSplit func(string) []string) []string {
	var chunks []string
	var current []string
	currentTokens := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunk := strings.Join(current, " ")
		chunks = append(chunks, chunk)

		// Compute overlap: keep enough trailing words from current
		if overlap > 0 {
			words := strings.Fields(chunk)
			// overlap tokens → approx overlap * 3/4 words (inverse of tokenCount)
			keepWords := (overlap * 3) / 4
			if keepWords >= len(words) {
				keepWords = len(words) / 2
			}
			if keepWords > 0 {
				overlapText := strings.Join(words[len(words)-keepWords:], " ")
				current = []string{overlapText}
				currentTokens = tokenCount(overlapText)
				return
			}
		}
		current = nil
		currentTokens = 0
	}

	for _, seg := range segments {
		segTokens := tokenCount(seg)

		if segTokens > maxTokens {
			// Segment too large: flush current, then split segment further
			flush()
			if subSplit != nil {
				sub := subSplit(seg)
				subChunks := chunkSegments(sub, maxTokens, overlap, splitWords)
				chunks = append(chunks, subChunks...)
			} else {
				// Word-level split
				subChunks := chunkSegments(splitWords(seg), maxTokens, overlap, nil)
				chunks = append(chunks, subChunks...)
			}
			continue
		}

		if currentTokens+segTokens > maxTokens {
			flush()
		}

		current = append(current, seg)
		currentTokens += segTokens
	}
	flush()
	return chunks
}

// splitWords splits text into individual words.
func splitWords(text string) []string {
	return strings.Fields(text)
}
