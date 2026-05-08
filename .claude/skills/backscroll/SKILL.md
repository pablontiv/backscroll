---
name: backscroll
description: |
  Use when the user asks about previous Claude/Pi sessions, prior decisions, old context, forgotten discussions, recurring bugs, project history, indexed notes/plans/knowledge, or wants to search/list/read/export Backscroll data. Also use proactively before re-investigating topics that may already exist in prior sessions.
user-invocable: true
allowed-tools:
  - Bash
---

# Skill: Backscroll

Backscroll is the local indexed memory/search tool for sessions and declared knowledge inputs. It indexes only active TOML manifests (`*.inputs.toml` or `backscroll.inputs.d/*.toml`); there is no implicit Claude/Pi fallback.

## Gate Check

```bash
command -v backscroll >/dev/null 2>&1
```

If missing:
```bash
cargo install --git https://github.com/pablontiv/backscroll.git
```

## First Check in Any Project

```bash
backscroll inputs validate
backscroll inputs list
backscroll status
```

If validation fails or no manifests are listed, explain that current ingestion requires manifests such as `claude.inputs.toml`, `pi.inputs.toml`, or files under `backscroll.inputs.d/*.toml`.

## Main Uses

| Need | Command |
|---|---|
| Search current project | `backscroll search "QUERY" --robot --max-tokens 4000` |
| Search all projects | `backscroll search "QUERY" --all-projects --robot --max-tokens 4000` |
| Recent sessions | `backscroll list --recent 10 --robot` |
| Read one indexed input file | `backscroll read PATH` |
| Resume target | `backscroll resume "QUERY" --all-projects --robot` |
| Topics | `backscroll topics --all-projects --robot` |
| Insights | `backscroll insights --all-projects --robot` |
| Export results | `backscroll export "QUERY" --format markdown --all-projects` |
| Validate DB | `backscroll validate` |

Prefer `--robot` for LLM-readable tab-separated output. Use `--json` when machine-readable output is needed.

## Inputs / Dry Run

```bash
backscroll inputs validate
backscroll inputs list --json
backscroll inputs test --input INPUT_ID --file PATH --json
```

Use `inputs test` before blaming search: it shows normalized messages, dropped records/blocks, and drop reasons without writing SQLite.

## Sync / Reindex

```bash
backscroll sync
backscroll reindex
```

`sync` is incremental by file hash. `reindex` clears hashes and reprocesses manifest-declared inputs. `--no-plans` is legacy/no-op for canonical ingestion; plans are indexed only when declared as inputs.

## Search Filters

Backscroll now supports generic sources and filters:

```bash
backscroll search "QUERY" --source sessions --role human --content-type text
backscroll search "QUERY" --source ke --all-projects
backscroll search "QUERY" --source decision --after 2026-03-01 --before 2026-04-01
backscroll search "QUERY" --lexical-only
```

Notes:
- `--source sessions` maps to `source = "session"`; `plans` maps to `plan`.
- Other sources are exact: `plan`, `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`, etc.
- `--role human` is a query alias for `user`; other roles pass through exactly.
- `--content-type` is exact and generic, not Claude-only.
- BM25 and hybrid/vector paths apply filters consistently.

## Canonical Input Model

Current canonical ingestion is TOML-only:

- Claude/Pi conversations emit `source = "session"`.
- Claude subagents are excluded by TOML discovery globs.
- Claude noise removal lives in `[inputs.text].remove`.
- Pi `think` blocks are excluded by TOML `content.exclude_when`.
- Plans and markdown knowledge sources use `decode.format = "markdown"` or `"markdown_sections"`.
- App config (`backscroll.toml`) does not provide canonical ingestion paths.

## Slash Command Modes

| Invocation | Action |
|---|---|
| `/backscroll` | `inputs validate`, `status`, recent sessions |
| `/backscroll QUERY` | Search current project, retry all-projects if empty |
| `/backscroll --topics` | Topic distribution, then optionally search a topic |
| `/backscroll --recent N` | Recent session list |
| `/backscroll --inputs` | Validate/list manifests |
| `/backscroll --context` | Use `ref-context-mode.md` rootline/context-save workflow |

## Context Mode

For `/backscroll --context`, read [ref-context-mode.md](ref-context-mode.md). It combines Backscroll with Rootline/context-save session-state queries.

## Common Mistakes

- Do not assume Claude/Pi sessions are indexed without an active manifest.
- Do not use `session_dirs`, `BACKSCROLL_SESSION_DIR`, or `--path` as canonical ingestion.
- If search is empty, check `backscroll inputs validate`, then run `inputs test` on a sample file.
- Use `--all-projects` when looking for cross-project history.
