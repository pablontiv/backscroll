use crate::core::ParsedMessage;
use crate::core::sync::{discover_candidate_files, parse_input_file_with_definition};
use crate::input_config::{InputConfig, InputDefinition};
use std::collections::HashMap;
use std::path::Path;

fn paths_refer_to_same_file(left: &Path, right: &Path) -> bool {
    match (left.canonicalize(), right.canonicalize()) {
        (Ok(left), Ok(right)) => left == right,
        _ => left == right,
    }
}

fn input_discovers_path(input: &InputDefinition, path: &Path) -> miette::Result<bool> {
    let candidates = discover_candidate_files(&input.discover)?;
    Ok(candidates
        .iter()
        .any(|candidate| paths_refer_to_same_file(candidate, path)))
}

pub fn read_input_file(
    path: &Path,
    input_config: &InputConfig,
) -> miette::Result<Vec<ParsedMessage>> {
    let inputs = input_config.active_inputs();
    if inputs.is_empty() {
        return Err(miette::miette!(
            "No active input manifest found; read requires a matching *.inputs.toml or backscroll.inputs.d/*.toml manifest"
        ));
    }

    for input in &inputs {
        if !input_discovers_path(input, path)? {
            continue;
        }
        let file =
            parse_input_file_with_definition(input, path, &HashMap::new()).ok_or_else(|| {
                miette::miette!(
                    "Failed to read {} with input manifest '{}'",
                    path.display(),
                    input.id
                )
            })?;
        return Ok(file.messages);
    }

    Err(miette::miette!(
        "No active input manifest applies to {}; read will not use an implicit Claude parser fallback",
        path.display()
    ))
}
