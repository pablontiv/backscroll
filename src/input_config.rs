use figment::{
    Figment,
    providers::{Format, Toml},
};
use globset::Glob;
use serde::{Deserialize, Deserializer, Serialize};
use serde_json_path::JsonPath;
use std::fs;
use std::path::{Component, Path, PathBuf};

#[derive(Deserialize)]
#[serde(untagged)]
enum StringOrVec {
    String(String),
    Vec(Vec<String>),
}

fn string_or_vec<'de, D>(deserializer: D) -> std::result::Result<Vec<String>, D::Error>
where
    D: Deserializer<'de>,
{
    match StringOrVec::deserialize(deserializer)? {
        StringOrVec::String(s) => Ok(vec![s]),
        StringOrVec::Vec(v) => Ok(v),
    }
}

fn default_true() -> bool {
    true
}

fn default_record_selector() -> String {
    "$".to_string()
}

fn default_string_selector() -> String {
    "$".to_string()
}

fn default_encoding() -> String {
    "utf-8".to_string()
}

fn default_content_type() -> String {
    "text".to_string()
}

fn default_join() -> String {
    "\n".to_string()
}

pub fn default_discover_include() -> Vec<String> {
    vec!["**/*.{json,jsonl}".to_string()]
}

pub fn default_discover_exclude() -> Vec<String> {
    vec!["**/subagents/**".to_string()]
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(default, deny_unknown_fields)]
pub struct SessionInput {
    pub source: String,
    #[serde(default = "SessionInput::default_parser")]
    pub parser: String,
    #[serde(default, deserialize_with = "string_or_vec")]
    pub paths: Vec<String>,
    #[serde(default)]
    pub glob: Option<String>,
    #[serde(default)]
    pub include_agents: bool,
    #[serde(
        default = "default_discover_include",
        deserialize_with = "string_or_vec"
    )]
    pub include: Vec<String>,
    #[serde(
        default = "default_discover_exclude",
        deserialize_with = "string_or_vec"
    )]
    pub exclude: Vec<String>,
    #[serde(default)]
    pub follow_symlinks: bool,
    #[serde(default = "default_true")]
    pub active: bool,
}

impl SessionInput {
    fn default_parser() -> String {
        "claude".to_string()
    }

    pub fn is_active(&self) -> bool {
        self.active
    }

    pub fn parser(&self) -> &str {
        if self.parser.is_empty() {
            "claude"
        } else {
            self.parser.as_str()
        }
    }

    pub fn discover_config(&self) -> DiscoverConfig {
        let include = self
            .glob
            .as_ref()
            .map_or_else(|| self.include.clone(), |glob| vec![glob.clone()]);
        let exclude = self.exclude.clone();

        DiscoverConfig {
            roots: self.paths.clone(),
            include,
            exclude,
            follow_symlinks: self.follow_symlinks,
        }
    }
}

impl Default for SessionInput {
    fn default() -> Self {
        Self {
            source: "session".to_string(),
            parser: Self::default_parser(),
            paths: Vec::new(),
            glob: None,
            include_agents: false,
            include: default_discover_include(),
            exclude: default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct InputManifest {
    pub version: u16,
    pub inputs: Vec<InputDefinition>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct InputDefinition {
    pub id: String,
    pub source: String,
    #[serde(default = "default_true")]
    pub active: bool,
    pub discover: DiscoverConfig,
    pub decode: DecodeConfig,
    #[serde(default)]
    pub record: RecordConfig,
    #[serde(default, rename = "map")]
    pub mapping: Option<MapConfig>,
    #[serde(default)]
    pub content: Option<ContentConfig>,
    #[serde(default)]
    pub text: TextConfig,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct DiscoverConfig {
    pub roots: Vec<String>,
    pub include: Vec<String>,
    #[serde(default)]
    pub exclude: Vec<String>,
    #[serde(default)]
    pub follow_symlinks: bool,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct DecodeConfig {
    pub format: DecodeFormat,
    #[serde(default = "default_encoding")]
    pub encoding: String,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum DecodeFormat {
    Jsonl,
    Json,
    Markdown,
    MarkdownSections,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct RecordConfig {
    #[serde(default = "default_record_selector")]
    pub selector: String,
    #[serde(default)]
    pub include_when: Vec<Predicate>,
    #[serde(default)]
    pub exclude_when: Vec<Predicate>,
}

impl Default for RecordConfig {
    fn default() -> Self {
        Self {
            selector: default_record_selector(),
            include_when: Vec::new(),
            exclude_when: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct MapConfig {
    pub role: String,
    #[serde(default)]
    pub uuid: Option<String>,
    #[serde(default)]
    pub timestamp: Option<String>,
    #[serde(default)]
    pub session_id: Option<String>,
    #[serde(default)]
    pub project: Option<String>,
    #[serde(default)]
    pub role_aliases: std::collections::BTreeMap<String, String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ContentConfig {
    pub selector: String,
    #[serde(default = "default_string_selector")]
    pub string: String,
    #[serde(default)]
    pub blocks: Option<String>,
    #[serde(default)]
    pub block_text: Option<String>,
    #[serde(default)]
    pub content_type: Option<String>,
    #[serde(default)]
    pub include_when: Vec<Predicate>,
    #[serde(default)]
    pub exclude_when: Vec<Predicate>,
    #[serde(default = "default_content_type")]
    pub default_content_type: String,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct TextConfig {
    #[serde(default = "default_join")]
    pub join: String,
    #[serde(default = "default_true")]
    pub trim: bool,
    #[serde(default = "default_true")]
    pub drop_empty: bool,
    #[serde(default)]
    pub remove: Vec<RemoveRule>,
}

impl Default for TextConfig {
    fn default() -> Self {
        Self {
            join: default_join(),
            trim: true,
            drop_empty: true,
            remove: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Predicate {
    pub selector: String,
    pub op: PredicateOp,
    #[serde(default)]
    pub value: Option<PredicateValue>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum PredicateOp {
    Eq,
    Ne,
    In,
    Exists,
    Missing,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(untagged)]
pub enum PredicateValue {
    String(String),
    Bool(bool),
    Integer(i64),
    Float(f64),
    Array(Vec<PredicateValue>),
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct RemoveRule {
    pub kind: RemoveKind,
    pub pattern: String,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum RemoveKind {
    Regex,
    Prefix,
    Suffix,
}

#[derive(Debug, Clone)]
pub struct LoadedInputManifest {
    pub path: PathBuf,
    pub manifest: InputManifest,
}

#[derive(Debug, Clone, Default)]
pub struct InputConfig {
    pub manifests: Vec<LoadedInputManifest>,
    pub inputs: Vec<InputDefinition>,
}

fn normalize_path_lexically(path: &Path) -> PathBuf {
    let mut normalized = PathBuf::new();

    for component in path.components() {
        match component {
            Component::CurDir => {}
            Component::ParentDir => {
                if !normalized.pop() {
                    normalized.push(component.as_os_str());
                }
            }
            Component::Prefix(_) | Component::RootDir | Component::Normal(_) => {
                normalized.push(component.as_os_str());
            }
        }
    }

    if normalized.as_os_str().is_empty() {
        PathBuf::from(".")
    } else {
        normalized
    }
}

fn expand_tilde(raw: &str) -> miette::Result<PathBuf> {
    if raw == "~" || raw.starts_with("~/") || raw.starts_with("~\\") {
        let home = dirs::home_dir().ok_or_else(|| {
            miette::miette!("Cannot expand '~' in discover.roots: home directory is unknown")
        })?;
        if raw == "~" {
            Ok(home)
        } else {
            Ok(home.join(&raw[2..]))
        }
    } else {
        Ok(PathBuf::from(raw))
    }
}

fn resolve_discovery_root(raw: &str, base_dir: &Path) -> miette::Result<PathBuf> {
    let expanded = expand_tilde(raw)?;
    let resolved = if expanded.is_absolute() {
        expanded
    } else {
        base_dir.join(expanded)
    };
    Ok(normalize_path_lexically(&resolved))
}

fn validate_glob_patterns(
    manifest_path: &Path,
    input_id: &str,
    field: &str,
    patterns: &[String],
) -> miette::Result<()> {
    for pattern in patterns {
        Glob::new(pattern).map_err(|err| {
            miette::miette!(
                "Active input '{}' in {} has invalid {} glob '{}': {}",
                input_id,
                manifest_path.display(),
                field,
                pattern,
                err
            )
        })?;
    }
    Ok(())
}

fn validate_jsonpath_selector(
    manifest_path: &Path,
    input_id: &str,
    field: &str,
    selector: &str,
) -> miette::Result<()> {
    JsonPath::parse(selector).map_err(|err| {
        miette::miette!(
            "Active input '{}' in {} has invalid {} JSONPath selector '{}': {}",
            input_id,
            manifest_path.display(),
            field,
            selector,
            err
        )
    })?;
    Ok(())
}

fn validate_predicate_selectors(
    manifest_path: &Path,
    input_id: &str,
    field: &str,
    predicates: &[Predicate],
) -> miette::Result<()> {
    for (index, predicate) in predicates.iter().enumerate() {
        validate_jsonpath_selector(
            manifest_path,
            input_id,
            &format!("{field}[{index}].selector"),
            &predicate.selector,
        )?;
    }
    Ok(())
}

impl InputConfig {
    pub fn load() -> miette::Result<Self> {
        let config_dir = Self::global_config_dir()?;
        let inputs_dir = config_dir.join("backscroll").join("inputs");
        Self::load_from_inputs_dir(&inputs_dir)
    }

    fn global_config_dir() -> miette::Result<PathBuf> {
        if let Some(dir) = std::env::var_os("BACKSCROLL_CONFIG_DIR") {
            if dir.is_empty() {
                return Err(miette::miette!(
                    "BACKSCROLL_CONFIG_DIR is set but empty; unset it or point it at a config directory"
                ));
            }
            return Ok(PathBuf::from(dir));
        }

        dirs::config_dir().ok_or_else(|| {
            miette::miette!(
                "Could not determine OS config directory for input manifests; set BACKSCROLL_CONFIG_DIR to override"
            )
        })
    }

    #[cfg(test)]
    pub(crate) fn load_from_inputs_dir_for_tests(inputs_dir: &Path) -> miette::Result<Self> {
        Self::load_from_inputs_dir(inputs_dir)
    }

    fn load_from_inputs_dir(inputs_dir: &Path) -> miette::Result<Self> {
        let mut manifests = Vec::new();
        let mut inputs = Vec::new();

        for path in Self::manifest_paths_from_inputs_dir(inputs_dir)? {
            let mut manifest = Self::read_manifest(&path)?;
            Self::resolve_manifest_roots(&path, &mut manifest)?;
            Self::validate_manifest(&path, &manifest)?;
            inputs.extend(manifest.inputs.iter().cloned());
            manifests.push(LoadedInputManifest { path, manifest });
        }

        Ok(Self { manifests, inputs })
    }

    pub fn active_inputs(&self) -> Vec<InputDefinition> {
        self.inputs
            .iter()
            .filter(|input| input.active)
            .cloned()
            .collect()
    }

    pub fn active_session_inputs(&self) -> Vec<SessionInput> {
        self.inputs
            .iter()
            .filter(|input| input.active && input.source == "session")
            .map(InputDefinition::to_legacy_session_input)
            .collect()
    }

    fn manifest_paths_from_inputs_dir(inputs_dir: &Path) -> miette::Result<Vec<PathBuf>> {
        if !inputs_dir.exists() {
            return Ok(Vec::new());
        }
        if !inputs_dir.is_dir() {
            return Err(miette::miette!(
                "Input manifest path {} is not a directory; expected manifests in <config_dir>/backscroll/inputs/*.inputs.toml (set BACKSCROLL_CONFIG_DIR to override)",
                inputs_dir.display()
            ));
        }

        let mut entries = fs::read_dir(inputs_dir)
            .map_err(|err| {
                miette::miette!(
                    "Failed to read input manifest directory {}: {}",
                    inputs_dir.display(),
                    err
                )
            })?
            .collect::<std::result::Result<Vec<_>, _>>()
            .map_err(|err| {
                miette::miette!(
                    "Failed to read input manifest directory {}: {}",
                    inputs_dir.display(),
                    err
                )
            })?;
        entries.sort_by_key(|entry| entry.path());

        Ok(entries
            .into_iter()
            .map(|entry| entry.path())
            .filter(|path| {
                path.is_file()
                    && path
                        .file_name()
                        .is_some_and(|name| name.to_string_lossy().ends_with(".inputs.toml"))
            })
            .collect())
    }

    fn read_manifest(path: &Path) -> miette::Result<InputManifest> {
        let content = fs::read_to_string(path).map_err(|err| {
            miette::miette!("Failed to read input manifest {}: {}", path.display(), err)
        })?;
        Figment::new()
            .merge(Toml::string(&content))
            .extract::<InputManifest>()
            .map_err(|err| {
                miette::miette!("Failed to parse input manifest {}: {}", path.display(), err)
            })
    }

    fn resolve_manifest_roots(path: &Path, manifest: &mut InputManifest) -> miette::Result<()> {
        let base_dir = path.parent().unwrap_or_else(|| Path::new("."));
        for input in &mut manifest.inputs {
            if !input.active {
                continue;
            }
            for root in &mut input.discover.roots {
                let resolved = resolve_discovery_root(root, base_dir)?;
                *root = resolved.to_string_lossy().into_owned();
            }
        }
        Ok(())
    }

    fn validate_manifest(path: &Path, manifest: &InputManifest) -> miette::Result<()> {
        if manifest.version != 1 {
            return Err(miette::miette!(
                "Unsupported input manifest version {} in {}; expected version = 1",
                manifest.version,
                path.display()
            ));
        }
        if manifest.inputs.is_empty() {
            return Err(miette::miette!(
                "Input manifest {} must define at least one [[inputs]] entry",
                path.display()
            ));
        }
        for input in &manifest.inputs {
            if input.active {
                input.validate_active(path)?;
            }
        }
        Ok(())
    }
}

impl InputDefinition {
    fn validate_active(&self, path: &Path) -> miette::Result<()> {
        if self.id.trim().is_empty() {
            return Err(miette::miette!(
                "Active input manifest {} has an input with an empty id",
                path.display()
            ));
        }
        if self.source.trim().is_empty() {
            return Err(miette::miette!(
                "Active input '{}' in {} has an empty source",
                self.id,
                path.display()
            ));
        }
        if self.discover.roots.is_empty() {
            return Err(miette::miette!(
                "Active input '{}' in {} must set discover.roots",
                self.id,
                path.display()
            ));
        }
        if self.discover.include.is_empty() {
            return Err(miette::miette!(
                "Active input '{}' in {} must set discover.include",
                self.id,
                path.display()
            ));
        }
        for root in &self.discover.roots {
            let root_path = Path::new(root);
            if !root_path.exists() {
                return Err(miette::miette!(
                    "Active input '{}' in {} has missing discover.roots path '{}'",
                    self.id,
                    path.display(),
                    root
                ));
            }
            if !root_path.is_file() && !root_path.is_dir() {
                return Err(miette::miette!(
                    "Active input '{}' in {} has discover.roots path '{}' that is neither a file nor a directory",
                    self.id,
                    path.display(),
                    root
                ));
            }
        }
        validate_glob_patterns(path, &self.id, "discover.include", &self.discover.include)?;
        validate_glob_patterns(path, &self.id, "discover.exclude", &self.discover.exclude)?;
        if !matches!(
            self.decode.encoding.to_ascii_lowercase().as_str(),
            "utf-8" | "utf8"
        ) {
            return Err(miette::miette!(
                "Active input '{}' in {} has unsupported decode.encoding '{}'; only utf-8 is supported",
                self.id,
                path.display(),
                self.decode.encoding
            ));
        }
        self.validate_selectors(path)?;
        self.validate_text_rules(path)?;
        Ok(())
    }

    fn validate_text_rules(&self, path: &Path) -> miette::Result<()> {
        for (index, rule) in self.text.remove.iter().enumerate() {
            if matches!(rule.kind, RemoveKind::Regex) {
                regex::Regex::new(&rule.pattern).map_err(|err| {
                    miette::miette!(
                        "Active input '{}' in {} has invalid text.remove[{}].pattern regex '{}': {}",
                        self.id,
                        path.display(),
                        index,
                        rule.pattern,
                        err
                    )
                })?;
            }
        }
        Ok(())
    }

    fn validate_selectors(&self, path: &Path) -> miette::Result<()> {
        if matches!(
            self.decode.format,
            DecodeFormat::Markdown | DecodeFormat::MarkdownSections
        ) {
            return Ok(());
        }

        let Some(mapping) = &self.mapping else {
            return Err(miette::miette!(
                "Active input '{}' in {} must define [inputs.map] for decode.format = {:?}",
                self.id,
                path.display(),
                self.decode.format
            ));
        };
        let Some(content) = &self.content else {
            return Err(miette::miette!(
                "Active input '{}' in {} must define [inputs.content] for decode.format = {:?}",
                self.id,
                path.display(),
                self.decode.format
            ));
        };

        validate_jsonpath_selector(path, &self.id, "record.selector", &self.record.selector)?;
        validate_predicate_selectors(
            path,
            &self.id,
            "record.include_when",
            &self.record.include_when,
        )?;
        validate_predicate_selectors(
            path,
            &self.id,
            "record.exclude_when",
            &self.record.exclude_when,
        )?;

        validate_jsonpath_selector(path, &self.id, "map.role", &mapping.role)?;
        if let Some(selector) = &mapping.uuid {
            validate_jsonpath_selector(path, &self.id, "map.uuid", selector)?;
        }
        if let Some(selector) = &mapping.timestamp {
            validate_jsonpath_selector(path, &self.id, "map.timestamp", selector)?;
        }
        if let Some(selector) = &mapping.session_id {
            validate_jsonpath_selector(path, &self.id, "map.session_id", selector)?;
        }
        if let Some(selector) = &mapping.project {
            validate_jsonpath_selector(path, &self.id, "map.project", selector)?;
        }

        validate_jsonpath_selector(path, &self.id, "content.selector", &content.selector)?;
        validate_jsonpath_selector(path, &self.id, "content.string", &content.string)?;
        if let Some(selector) = &content.blocks {
            validate_jsonpath_selector(path, &self.id, "content.blocks", selector)?;
        }
        if let Some(selector) = &content.block_text {
            validate_jsonpath_selector(path, &self.id, "content.block_text", selector)?;
        }
        if let Some(selector) = &content.content_type {
            validate_jsonpath_selector(path, &self.id, "content.content_type", selector)?;
        }
        validate_predicate_selectors(
            path,
            &self.id,
            "content.include_when",
            &content.include_when,
        )?;
        validate_predicate_selectors(
            path,
            &self.id,
            "content.exclude_when",
            &content.exclude_when,
        )?;
        Ok(())
    }

    fn to_legacy_session_input(&self) -> SessionInput {
        SessionInput {
            source: self.source.clone(),
            parser: self.id.clone(),
            paths: self.discover.roots.clone(),
            glob: None,
            include_agents: true,
            include: self.discover.include.clone(),
            exclude: self.discover.exclude.clone(),
            follow_symlinks: self.discover.follow_symlinks,
            active: self.active,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use miette::IntoDiagnostic;

    #[test]
    fn rejects_unknown_predicate_operator_with_clear_error() -> miette::Result<()> {
        let dir = tempfile::tempdir().into_diagnostic()?;
        let root = dir.path().join("data");
        fs::create_dir(&root).into_diagnostic()?;
        fs::write(root.join("session.jsonl"), "{}").into_diagnostic()?;
        let root_toml = root.to_string_lossy().replace('\\', "\\\\");
        fs::write(
            dir.path().join("bad.inputs.toml"),
            format!(
                r#"version = 1

[[inputs]]
id = "bad"
source = "session"

[inputs.discover]
roots = ["{}"]
include = ["**/*.jsonl"]

[inputs.decode]
format = "jsonl"

[inputs.record]
include_when = [{{ selector = "$.type", op = "contains", value = "user" }}]

[inputs.map]
role = "$.role"

[inputs.content]
selector = "$.content"
"#,
                root_toml
            ),
        )
        .into_diagnostic()?;

        let error =
            InputConfig::load_from_inputs_dir(dir.path()).expect_err("unknown op should fail");
        let message = error.to_string();
        assert!(message.contains("contains"), "{message}");
        assert!(message.contains("eq"), "{message}");
        assert!(message.contains("missing"), "{message}");
        Ok(())
    }

    #[test]
    fn expands_tilde_roots_against_home_directory() -> miette::Result<()> {
        let Some(home) = dirs::home_dir() else {
            return Ok(());
        };

        let resolved = resolve_discovery_root("~/backscroll-test", Path::new("/base"))?;

        assert_eq!(resolved, home.join("backscroll-test"));
        Ok(())
    }
}
