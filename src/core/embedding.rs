use miette::Result;

#[allow(dead_code)]
pub(crate) trait EmbeddingProvider: Send + Sync {
    fn embed(&self, texts: &[&str]) -> Result<Vec<Vec<f32>>>;
    fn dimensions(&self) -> usize;
    fn model_name(&self) -> &str;
}

#[allow(dead_code)]
pub(crate) struct MockEmbeddingProvider {
    dims: usize,
}

#[allow(dead_code)]
impl MockEmbeddingProvider {
    pub(crate) fn new(dims: usize) -> Self {
        Self { dims }
    }
}

impl EmbeddingProvider for MockEmbeddingProvider {
    fn embed(&self, texts: &[&str]) -> Result<Vec<Vec<f32>>> {
        Ok(texts
            .iter()
            .map(|text| {
                let hash = text.bytes().fold(0u64, |acc, b| {
                    acc.wrapping_mul(31).wrapping_add(u64::from(b))
                });
                (0..self.dims)
                    .map(|i| {
                        let seed = hash.wrapping_add(i as u64);
                        ((seed % 2000) as f32 / 1000.0) - 1.0
                    })
                    .collect()
            })
            .collect())
    }

    fn dimensions(&self) -> usize {
        self.dims
    }

    fn model_name(&self) -> &str {
        "mock-384"
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mock_provider_dimensions() {
        let provider = MockEmbeddingProvider::new(384);
        assert_eq!(provider.dimensions(), 384);
    }

    #[test]
    fn test_mock_provider_embed_returns_correct_shape() {
        let provider = MockEmbeddingProvider::new(384);
        let result = provider.embed(&["hello world", "foo bar"]).unwrap();
        assert_eq!(result.len(), 2);
        assert_eq!(result[0].len(), 384);
        assert_eq!(result[1].len(), 384);
    }

    #[test]
    fn test_mock_provider_deterministic() {
        let provider = MockEmbeddingProvider::new(384);
        let a = provider.embed(&["same input"]).unwrap();
        let b = provider.embed(&["same input"]).unwrap();
        assert_eq!(a, b);
    }

    #[test]
    fn test_mock_provider_different_texts_different_vectors() {
        let provider = MockEmbeddingProvider::new(384);
        let result = provider.embed(&["text one", "text two"]).unwrap();
        assert_ne!(result[0], result[1]);
    }
}
