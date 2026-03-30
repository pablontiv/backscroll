#[allow(dead_code)]
const CHARS_PER_TOKEN: usize = 4;

#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct Chunk {
    pub text: String,
    pub chunk_index: usize,
}

#[allow(dead_code)]
pub(crate) fn chunk_text(text: &str, max_tokens: usize) -> Vec<Chunk> {
    let text = text.trim();
    if text.is_empty() {
        return vec![];
    }

    let max_chars = max_tokens * CHARS_PER_TOKEN;

    if text.len() <= max_chars {
        return vec![Chunk {
            text: text.to_owned(),
            chunk_index: 0,
        }];
    }

    let paragraphs: Vec<&str> = text
        .split("\n\n")
        .map(str::trim)
        .filter(|p| !p.is_empty())
        .collect();

    let mut chunks: Vec<Chunk> = vec![];
    let mut current = String::new();

    let flush = |current: &mut String, chunks: &mut Vec<Chunk>| {
        let trimmed = current.trim().to_owned();
        if !trimmed.is_empty() {
            let idx = chunks.len();
            chunks.push(Chunk {
                text: trimmed,
                chunk_index: idx,
            });
        }
        current.clear();
    };

    for paragraph in paragraphs {
        if paragraph.len() > max_chars {
            // Flush whatever we have first
            flush(&mut current, &mut chunks);

            // Split paragraph by sentence boundaries
            let sentences = split_sentences(paragraph);
            for sentence in sentences {
                if current.len() + sentence.len() + 1 > max_chars && !current.is_empty() {
                    flush(&mut current, &mut chunks);
                }
                if !current.is_empty() {
                    current.push(' ');
                }
                current.push_str(sentence.trim());
            }
            flush(&mut current, &mut chunks);
        } else if current.len() + paragraph.len() + 2 > max_chars {
            flush(&mut current, &mut chunks);
            current.push_str(paragraph);
        } else {
            if !current.is_empty() {
                current.push_str("\n\n");
            }
            current.push_str(paragraph);
        }
    }

    flush(&mut current, &mut chunks);
    chunks
}

fn split_sentences(text: &str) -> Vec<&str> {
    let mut sentences = vec![];
    let mut start = 0;
    let chars: Vec<char> = text.chars().collect();
    let len = chars.len();

    let mut i = 0;
    while i < len {
        if matches!(chars[i], '.' | '?' | '!') && i + 1 < len && chars[i + 1] == ' ' {
            sentences.push(&text[start..i + 1]);
            start = i + 2;
            i += 2;
        } else {
            i += chars[i].len_utf8().max(1);
        }
    }

    if start < text.len() {
        sentences.push(&text[start..]);
    }

    sentences
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_short_text_single_chunk() {
        let result = chunk_text("Hello world", 512);
        assert_eq!(result.len(), 1);
        assert_eq!(result[0].text, "Hello world");
        assert_eq!(result[0].chunk_index, 0);
    }

    #[test]
    fn test_empty_text_no_chunks() {
        let result = chunk_text("", 512);
        assert!(result.is_empty());
    }

    #[test]
    fn test_whitespace_only_no_chunks() {
        let result = chunk_text("   \n\n   ", 512);
        assert!(result.is_empty());
    }

    #[test]
    fn test_long_text_splits_on_paragraphs() {
        // 3 paragraphs of ~900 chars each, max_tokens=512 (2048 chars)
        // Each paragraph fits alone but not all three together
        let para = "x".repeat(900);
        let text = format!("{para}\n\n{para}\n\n{para}");
        let result = chunk_text(&text, 512);
        // Total ~2700 chars, max 2048 — should produce at least 2 chunks
        assert!(
            result.len() >= 2,
            "expected >= 2 chunks, got {}",
            result.len()
        );
    }

    #[test]
    fn test_chunk_indices_sequential() {
        let para = "y".repeat(900);
        let text = format!("{para}\n\n{para}\n\n{para}");
        let result = chunk_text(&text, 512);
        for (i, chunk) in result.iter().enumerate() {
            assert_eq!(chunk.chunk_index, i);
        }
    }
}
