use miette::{IntoDiagnostic, Result};

use crate::config::EmbeddingConfig;
use ort::value::Tensor;

#[allow(dead_code)]
pub trait EmbeddingProvider: Send + Sync {
    fn embed(&self, texts: &[&str]) -> Result<Vec<Vec<f32>>>;
    fn dimensions(&self) -> usize;
    fn model_name(&self) -> &str;
}

#[allow(dead_code)]
pub struct MockEmbeddingProvider {
    dims: usize,
}

#[allow(dead_code)]
impl MockEmbeddingProvider {
    pub fn new(dims: usize) -> Self {
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

#[allow(dead_code)]
pub(crate) struct OnnxProvider {
    session: std::sync::Mutex<ort::session::Session>,
    tokenizer: tokenizers::Tokenizer,
    dims: usize,
    name: String,
}

#[allow(dead_code)]
impl OnnxProvider {
    pub(crate) fn new(config: &EmbeddingConfig) -> miette::Result<Self> {
        let model_dir = Self::resolve_model_dir(config)?;
        let model_path = model_dir.join("model.onnx");
        let tokenizer_path = model_dir.join("tokenizer.json");

        if !model_path.exists() || !tokenizer_path.exists() {
            Self::download_model(&config.model_name, &model_dir)?;
        }

        let session = ort::session::Session::builder()
            .into_diagnostic()?
            .commit_from_file(&model_path)
            .into_diagnostic()?;

        let tokenizer = tokenizers::Tokenizer::from_file(&tokenizer_path)
            .map_err(|e| miette::miette!("Failed to load tokenizer: {e}"))?;

        Ok(Self {
            session: std::sync::Mutex::new(session),
            tokenizer,
            dims: 384,
            name: config.model_name.clone(),
        })
    }

    fn resolve_model_dir(config: &EmbeddingConfig) -> miette::Result<std::path::PathBuf> {
        if !config.model_path.is_empty() {
            return Ok(std::path::PathBuf::from(&config.model_path));
        }
        let base = dirs::home_dir()
            .ok_or_else(|| miette::miette!("Cannot determine home directory"))?
            .join(".backscroll")
            .join("models")
            .join(&config.model_name);
        Ok(base)
    }

    fn download_model(model_name: &str, target_dir: &std::path::PathBuf) -> miette::Result<()> {
        std::fs::create_dir_all(target_dir).into_diagnostic()?;
        let base_url =
            format!("https://huggingface.co/sentence-transformers/{model_name}/resolve/main");
        tracing::info!(
            model = model_name,
            "Downloading embedding model (first use)..."
        );

        for filename in &["model.onnx", "tokenizer.json"] {
            let url = format!("{base_url}/{filename}");
            let target = target_dir.join(filename);
            let output = std::process::Command::new("curl")
                .args(["-sL", "-o", target.to_str().unwrap(), &url])
                .output()
                .into_diagnostic()?;
            if !output.status.success() {
                return Err(miette::miette!(
                    "Failed to download {filename}: {}",
                    String::from_utf8_lossy(&output.stderr)
                ));
            }
        }
        tracing::info!(model = model_name, "Model downloaded");
        Ok(())
    }

    fn mean_pooling(
        token_embeddings: &ndarray::ArrayView2<f32>,
        attention_mask: &[u32],
    ) -> Vec<f32> {
        let seq_len = token_embeddings.shape()[0];
        let dims = token_embeddings.shape()[1];
        let mut pooled = vec![0.0f32; dims];
        let mut mask_sum = 0.0f32;
        for i in 0..seq_len {
            let mask = attention_mask[i] as f32;
            mask_sum += mask;
            for j in 0..dims {
                pooled[j] += token_embeddings[[i, j]] * mask;
            }
        }
        if mask_sum > 0.0 {
            for val in &mut pooled {
                *val /= mask_sum;
            }
        }
        // L2 normalize
        let norm: f32 = pooled.iter().map(|x| x * x).sum::<f32>().sqrt();
        if norm > 0.0 {
            for val in &mut pooled {
                *val /= norm;
            }
        }
        pooled
    }
}

impl EmbeddingProvider for OnnxProvider {
    fn embed(&self, texts: &[&str]) -> miette::Result<Vec<Vec<f32>>> {
        let mut results = Vec::with_capacity(texts.len());
        for text in texts {
            let encoding = self
                .tokenizer
                .encode(*text, true)
                .map_err(|e| miette::miette!("Tokenization failed: {e}"))?;
            let ids = encoding.get_ids();
            let attention_mask = encoding.get_attention_mask();
            let input_ids: Vec<i64> = ids.iter().map(|&id| i64::from(id)).collect();
            let attn_mask: Vec<i64> = attention_mask.iter().map(|&m| i64::from(m)).collect();
            let token_type_ids: Vec<i64> = vec![0i64; ids.len()];
            let seq_len = ids.len();

            let input_ids_array =
                ndarray::Array2::from_shape_vec((1, seq_len), input_ids).into_diagnostic()?;
            let attn_mask_array =
                ndarray::Array2::from_shape_vec((1, seq_len), attn_mask).into_diagnostic()?;
            let token_type_array =
                ndarray::Array2::from_shape_vec((1, seq_len), token_type_ids).into_diagnostic()?;

            let input_ids_tensor = Tensor::from_array(input_ids_array).into_diagnostic()?;
            let attn_mask_tensor = Tensor::from_array(attn_mask_array).into_diagnostic()?;
            let token_type_tensor = Tensor::from_array(token_type_array).into_diagnostic()?;

            let mut session = self.session.lock().unwrap();
            let outputs = session
                .run(ort::inputs![
                    "input_ids" => input_ids_tensor,
                    "attention_mask" => attn_mask_tensor,
                    "token_type_ids" => token_type_tensor,
                ])
                .into_diagnostic()?;

            // try_extract_tensor returns (&[i64], &[f32]) — shape is [1, seq_len, dims]
            let (shape, data) = outputs[0].try_extract_tensor::<f32>().into_diagnostic()?;
            let hidden_dim = if shape.len() >= 3 {
                shape[2] as usize
            } else {
                self.dims
            };
            let token_embs = ndarray::ArrayView2::from_shape(
                (seq_len, hidden_dim),
                &data[..seq_len * hidden_dim],
            )
            .into_diagnostic()?;
            let pooled = Self::mean_pooling(&token_embs, attention_mask);
            results.push(pooled);
        }
        Ok(results)
    }

    fn dimensions(&self) -> usize {
        self.dims
    }
    fn model_name(&self) -> &str {
        &self.name
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
