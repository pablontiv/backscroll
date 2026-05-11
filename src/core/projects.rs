use figment::{
    Figment,
    providers::{Format, Toml},
};
use globset::{Glob, GlobSetBuilder};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use toml;

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq)]
pub struct ProjectConfig {
    pub id: String,
    #[serde(default)]
    pub roots: Vec<String>,
    #[serde(default)]
    pub worktree_patterns: Vec<String>,
    #[serde(default)]
    pub aliases: Vec<String>,
}

#[derive(Debug, Default, Deserialize, Serialize)]
pub struct ProjectRegistry {
    #[serde(default)]
    pub projects: Vec<ProjectConfig>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ProjectHint {
    pub project_id: String,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum IdentificationConfidence {
    Exact,
    Pattern,
    Hint,
    Truncated,
    Unknown,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectIdentification {
    pub project_id: String,
    pub confidence: IdentificationConfidence,
}

pub fn load_global_registry() -> ProjectRegistry {
    let config_path = dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".config/backscroll/projects.toml");
    load_registry_from(&config_path)
}

pub fn load_registry_from(path: &Path) -> ProjectRegistry {
    Figment::new()
        .merge(Toml::file(path))
        .extract::<ProjectRegistry>()
        .unwrap_or_default()
}

/// Search upward from `cwd` for a `.backscroll/project.toml` hint file.
pub fn load_local_hint(cwd: &Path) -> Option<ProjectHint> {
    let mut current = cwd;
    loop {
        let hint_path = current.join(".backscroll/project.toml");
        if let Ok(content) = std::fs::read_to_string(&hint_path) {
            if let Ok(hint) = toml::from_str::<ProjectHint>(&content) {
                return Some(hint);
            }
        }
        match current.parent() {
            Some(parent) if parent != current => current = parent,
            _ => return None,
        }
    }
}

/// Identify the canonical project for `cwd` using `registry` and any local hint.
/// Resolution order: local hint → exact root → worktree pattern → truncated suffix → unknown.
pub fn identify(cwd: &Path, registry: &ProjectRegistry) -> ProjectIdentification {
    let hint = load_local_hint(cwd);

    if let Some(h) = hint {
        return ProjectIdentification {
            project_id: h.project_id,
            confidence: IdentificationConfidence::Hint,
        };
    }

    let cwd_str = cwd.to_string_lossy();

    // 1. Exact root match (cwd == root itself)
    for project in &registry.projects {
        for root in &project.roots {
            let root_path = Path::new(root);
            if cwd == root_path {
                return ProjectIdentification {
                    project_id: project.id.clone(),
                    confidence: IdentificationConfidence::Exact,
                };
            }
        }
    }

    // 2. Worktree pattern match — checked before subpath so worktrees get "pattern" confidence
    for project in &registry.projects {
        if project.worktree_patterns.is_empty() {
            continue;
        }
        let mut builder = GlobSetBuilder::new();
        for pattern in &project.worktree_patterns {
            if let Ok(g) = Glob::new(pattern) {
                builder.add(g);
            }
        }
        if let Ok(globset) = builder.build() {
            if globset.is_match(cwd) {
                return ProjectIdentification {
                    project_id: project.id.clone(),
                    confidence: IdentificationConfidence::Pattern,
                };
            }
        }
    }

    // 3. Subpath under a known root
    for project in &registry.projects {
        for root in &project.roots {
            let root_path = Path::new(root);
            if cwd.starts_with(root_path) {
                return ProjectIdentification {
                    project_id: project.id.clone(),
                    confidence: IdentificationConfidence::Exact,
                };
            }
        }
    }

    // 3. Truncated path: cwd_str is a suffix of a known root (leading chars stripped)
    for project in &registry.projects {
        for root in &project.roots {
            let root_no_leading = root.trim_start_matches('/');
            let cwd_no_leading = cwd_str.trim_start_matches('/');
            if root_no_leading.ends_with(cwd_no_leading)
                || cwd_no_leading.ends_with(root_no_leading)
            {
                return ProjectIdentification {
                    project_id: project.id.clone(),
                    confidence: IdentificationConfidence::Truncated,
                };
            }
        }
    }

    ProjectIdentification {
        project_id: "unknown".to_string(),
        confidence: IdentificationConfidence::Unknown,
    }
}

/// List all projects in the registry.
pub fn list_projects(registry: &ProjectRegistry) -> &[ProjectConfig] {
    &registry.projects
}

/// Get aliases for a project by ID.
pub fn get_aliases<'a>(registry: &'a ProjectRegistry, project_id: &str) -> Vec<&'a str> {
    registry
        .projects
        .iter()
        .find(|p| p.id == project_id)
        .map(|p| p.aliases.iter().map(|a| a.as_str()).collect())
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    fn registry_with_pinata() -> ProjectRegistry {
        ProjectRegistry {
            projects: vec![ProjectConfig {
                id: "pinata".to_string(),
                roots: vec!["/home/shared/pinata".to_string()],
                worktree_patterns: vec![
                    "/home/shared/pinata/.worktrees/*".to_string(),
                    "/tmp/pi-worktree-*".to_string(),
                ],
                aliases: vec!["pi rootline extensions".to_string(), "pinata".to_string()],
            }],
        }
    }

    #[test]
    fn test_identify_exact_root_match() {
        let registry = registry_with_pinata();
        let result = identify(Path::new("/home/shared/pinata"), &registry);
        assert_eq!(result.project_id, "pinata");
        assert_eq!(result.confidence, IdentificationConfidence::Exact);
    }

    #[test]
    fn test_identify_subpath_under_root() {
        let registry = registry_with_pinata();
        let result = identify(
            Path::new("/home/shared/pinata/src/decision-guard"),
            &registry,
        );
        assert_eq!(result.project_id, "pinata");
        assert_eq!(result.confidence, IdentificationConfidence::Exact);
    }

    #[test]
    fn test_identify_worktree_dot_worktrees() {
        let registry = registry_with_pinata();
        let result = identify(
            Path::new("/home/shared/pinata/.worktrees/feature-x"),
            &registry,
        );
        assert_eq!(result.project_id, "pinata");
        assert_eq!(result.confidence, IdentificationConfidence::Pattern);
    }

    #[test]
    fn test_identify_worktree_tmp() {
        let registry = registry_with_pinata();
        let result = identify(Path::new("/tmp/pi-worktree-abc123"), &registry);
        assert_eq!(result.project_id, "pinata");
        assert_eq!(result.confidence, IdentificationConfidence::Pattern);
    }

    #[test]
    fn test_identify_truncated_path() {
        let registry = registry_with_pinata();
        // "ome/shared/pinata" = "/home/shared/pinata" with leading "h" stripped
        let result = identify(Path::new("ome/shared/pinata"), &registry);
        assert_eq!(result.project_id, "pinata");
        assert_eq!(result.confidence, IdentificationConfidence::Truncated);
    }

    #[test]
    fn test_identify_unknown_session() {
        let registry = registry_with_pinata();
        let result = identify(Path::new("/home/user/other-project"), &registry);
        assert_eq!(result.project_id, "unknown");
        assert_eq!(result.confidence, IdentificationConfidence::Unknown);
    }

    #[test]
    fn test_identify_local_hint_takes_priority() {
        let dir = tempdir().unwrap();
        let hint_dir = dir.path().join(".backscroll");
        fs::create_dir_all(&hint_dir).unwrap();
        fs::write(
            hint_dir.join("project.toml"),
            "project_id = \"my-project\"\n",
        )
        .unwrap();

        let registry = registry_with_pinata();
        let result = identify(dir.path(), &registry);
        assert_eq!(result.project_id, "my-project");
        assert_eq!(result.confidence, IdentificationConfidence::Hint);
    }

    #[test]
    fn test_aliases() {
        let registry = registry_with_pinata();
        let aliases = get_aliases(&registry, "pinata");
        assert!(aliases.contains(&"pi rootline extensions"));
        assert!(aliases.contains(&"pinata"));
    }

    #[test]
    fn test_load_registry_from_toml() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("projects.toml");
        fs::write(
            &path,
            r#"
[[projects]]
id = "my-repo"
roots = ["/home/user/my-repo"]
worktree_patterns = ["/home/user/my-repo/.worktrees/*"]
aliases = ["my repo"]
"#,
        )
        .unwrap();

        let registry = load_registry_from(&path);
        assert_eq!(registry.projects.len(), 1);
        assert_eq!(registry.projects[0].id, "my-repo");
        assert_eq!(registry.projects[0].roots, vec!["/home/user/my-repo"]);
    }

    #[test]
    fn test_load_registry_missing_file() {
        let registry = load_registry_from(Path::new("/nonexistent/projects.toml"));
        assert!(registry.projects.is_empty());
    }
}
