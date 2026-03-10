# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Documentation

- Close E10 — all 11 tasks verified as implemented
- Close E11 — all 9 plan indexing tasks completed
- Update CLAUDE.md for E10/E11 and harden pre-push module check

### Features

- Support multi-path session_dirs with backward compat
- Support multiple --path flags in sync command
- Harden repo for public release
- E10 session discovery & resume (#5)
- Port rootline git hook checks
- Add sync-version recipe and document just workflows
- E11 plan indexing — parser, source field, sync pipeline (T062-T066)
- E11 source filter — --source flag for search and resume (T067-T070)
- Add v2→v3 migration with fts5vocab virtual table (T071)
- Add topics subcommand with fts5vocab queries (T072-T074)
- Add list subcommand for session listing with metadata (T077-T080)
- Add per-project breakdown to status output (T081-T083)

### Miscellaneous

- Expand gitignore with WAL and runtime patterns
- Create E12 session analytics planning docs
- Move backscroll skill from praxis to this repo
- Add YAML frontmatter to legacy E01-E05 docs
- Create E13-E15 planning docs

### Testing

- Add multi-path config migration tests
- Add integration tests for topics robot and JSON output (T075-T076)

## [0.1.14] - 2026-03-09

### Documentation

- Align documentation style with rootline project
- Rewrite README with concept-driven sections matching rootline style
- Rewrite README with user-facing tone matching rootline
- Add per-feature reference docs matching rootline pattern
- Add v2 roadmap (E10-E11) and opportunities analysis

### Features

- Auto-sync on query + CWD as default project

### Miscellaneous

- Sync Cargo.lock after thiserror removal

## [0.1.13] - 2026-03-09

### Documentation

- Remove stale errors.rs reference from architecture diagram
- Mark E09 tasks T046-T050 as completed

### Features

- Add 5 noise filter patterns for hook stdout and command tags

### Refactor

- Migrate regex compilation to LazyLock statics
- Remove unused BackscrollError and thiserror dependency

### Testing

- Add unit tests for new noise filter patterns

## [0.1.12] - 2026-03-08

### Documentation

- Rewrite README in English following rootline style
- Refine README tone and structure to match rootline
- Update README with new commands and LLM output modes
- Mark E01-E08 tasks as completed

### Features

- Add output formatting module (text/json/robot)
- Add reader module for direct session reading

### Miscellaneous

- Create E06-E08 planning docs
- Add MIT license
- Add CLAUDE.md project instructions
- Update dependencies (regex, tracing env-filter, rust-version)

### Refactor

- Redesign SearchEngine trait and domain models
- Implement defensive parsing with noise filtering
- Restructure CLI with output formats, read command, and tracing
- Upgrade sqlite schema v2 with external FTS5 content table

### Testing

- Update integration tests for SessionRecord format

## [0.1.11] - 2026-03-07

### Bug Fixes

- Redesign CI pipeline to use atomic gh release create
- Apply rustfmt to main.rs

### Features

- Show application version in status command

## [0.1.10] - 2026-03-07

### Features

- Add automated github release job with binary upload

## [0.1.9] - 2026-03-07

### Bug Fixes

- Resolve lints and formatting issues to fix CI pipeline
- Lazy database opening to prevent locking in CI tests

### CI/CD

- Update and finalize github actions pipeline

### Documentation

- Add backscroll structured investigation
- Confirm Go decision after CASS evaluation
- Add hypothesize frontmatter to research document
- Add project README and finalize roadmap status

### Features

- Initial rust 2024 setup and core infrastructure (E01)
- Defensive parsing and incremental sync (E02)
- Fts5 search engine implementation (E03)
- Static build configuration with zig (E04)
- Support default paths and hierarchical configuration
- Refactor to ports and adapters architecture (S002)
- Implement automated semver tagging in CI based on rootline logic

### Testing

- 96% coverage and cli integration tests (E05)


