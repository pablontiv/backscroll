use figment::{
    Figment,
    providers::{Format, Toml},
};
use serde::{Deserialize, Deserializer, Serialize};
use std::fs;
use std::path::{Path, PathBuf};

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
}

impl Default for SessionInput {
    fn default() -> Self {
        Self {
            source: "session".to_string(),
            parser: Self::default_parser(),
            paths: Vec::new(),
            glob: None,
            include_agents: false,
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
    #[serde(rename = "map")]
    pub mapping: MapConfig,
    pub content: ContentConfig,
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

impl InputConfig {
    pub fn load() -> miette::Result<Self> {
        Self::load_from_dir(Path::new("."))
    }

    pub fn load_from_dir(root: &Path) -> miette::Result<Self> {
        let mut manifests = Vec::new();
        let mut inputs = Vec::new();

        for path in Self::manifest_paths_from_dir(root)? {
            let manifest = Self::read_manifest(&path)?;
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

    fn manifest_paths_from_dir(root: &Path) -> miette::Result<Vec<PathBuf>> {
        let mut paths = Vec::new();

        if root.is_dir() {
            let mut entries = fs::read_dir(root)
                .map_err(|err| miette::miette!("Failed to read {}: {}", root.display(), err))?
                .collect::<std::result::Result<Vec<_>, _>>()
                .map_err(|err| miette::miette!("Failed to read {}: {}", root.display(), err))?;
            entries.sort_by_key(|entry| entry.path());
            paths.extend(
                entries
                    .into_iter()
                    .map(|entry| entry.path())
                    .filter(|path| {
                        path.is_file()
                            && path.file_name().is_some_and(|name| {
                                name.to_string_lossy().ends_with(".inputs.toml")
                            })
                    }),
            );
        }

        let dir = root.join("backscroll.inputs.d");
        if dir.is_dir() {
            let mut entries = fs::read_dir(&dir)
                .map_err(|err| miette::miette!("Failed to read {}: {}", dir.display(), err))?
                .collect::<std::result::Result<Vec<_>, _>>()
                .map_err(|err| miette::miette!("Failed to read {}: {}", dir.display(), err))?;
            entries.sort_by_key(|entry| entry.path());
            paths.extend(
                entries
                    .into_iter()
                    .map(|entry| entry.path())
                    .filter(|path| {
                        path.is_file() && path.extension().is_some_and(|ext| ext == "toml")
                    }),
            );
        }

        Ok(paths)
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
        Ok(())
    }

    fn to_legacy_session_input(&self) -> SessionInput {
        SessionInput {
            source: self.source.clone(),
            parser: self.id.clone(),
            paths: self.discover.roots.clone(),
            glob: None,
            include_agents: !self
                .discover
                .exclude
                .iter()
                .any(|pattern| pattern.contains("subagents")),
            active: self.active,
        }
    }
}
