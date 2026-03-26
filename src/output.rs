use backscroll::core::SearchResult;
use std::io::{self, Write};

pub enum OutputFormat {
    Text,
    Json,
    Robot,
}

pub struct OutputOptions {
    pub format: OutputFormat,
    pub fields: String,
    pub max_tokens: Option<usize>,
}

pub fn format_results(results: &[SearchResult], options: &OutputOptions) {
    let mut out = io::stdout();
    let mut tokens_used = 0;

    for res in results {
        // Simple heuristic for tokens
        let approx_tokens = res.text.len() / 4;
        if let Some(max) = options.max_tokens {
            if tokens_used + approx_tokens > max && tokens_used > 0 {
                break; // Limit reached
            }
        }
        tokens_used += approx_tokens;

        match options.format {
            OutputFormat::Json => {
                if options.fields == "minimal" {
                    let min_res = serde_json::json!({
                        "source_path": res.source_path,
                        "snippet": res.match_snippet.as_deref().unwrap_or(&res.text),
                        "score": res.score,
                        "role": res.role,
                        "timestamp": res.timestamp
                    });
                    writeln!(out, "{}", min_res).unwrap();
                } else {
                    if let Ok(json) = serde_json::to_string(res) {
                        writeln!(out, "{}", json).unwrap();
                    }
                }
            }
            OutputFormat::Robot => {
                let snippet = res.match_snippet.as_deref().unwrap_or(&res.text);
                let ts = res.timestamp.as_deref().unwrap_or("-");
                writeln!(
                    out,
                    "{}\t{}\t{}\t{}\t{}",
                    res.source_path, res.score, res.role, ts, snippet
                )
                .unwrap();
            }
            OutputFormat::Text => {
                println!("---");
                let ts = res.timestamp.as_deref().unwrap_or("");
                if ts.is_empty() {
                    println!(
                        "[{}] {} (Score: {:.2})",
                        res.role, res.source_path, res.score
                    );
                } else {
                    println!(
                        "[{}] {} (Score: {:.2}) @ {}",
                        res.role, res.source_path, res.score, ts
                    );
                }
                if let Some(snippet) = &res.match_snippet {
                    // Could add terminal bold for >>> and <<< here
                    let formatted = snippet.replace(">>>", "\x1b[1m").replace("<<<", "\x1b[0m");
                    println!("{}", formatted);
                } else {
                    println!("{}", res.text);
                }
            }
        }
    }
}
