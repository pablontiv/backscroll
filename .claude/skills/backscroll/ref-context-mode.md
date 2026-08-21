# Backscroll Context Mode

Use this only for `/skill:backscroll --context`. Produce a recovery brief with: Backscroll evidence, optional Rootline live state, and gaps.

Backscroll retrieval uses active manifests, mandatory startup sync, perennial SQLite, and database-backed query. Raw `cat`, `jq`, Python, or filesystem session hunting is not a normal retrieval fallback; drill into known paths with `backscroll search --source-path`.

## Required Backscroll Retrieval

```bash
backscroll validate
backscroll status
backscroll list --limit 10 --all-projects --json
```

If the user supplied a query, search for it. Otherwise use the directory name plus context terms:

```bash
PROJECT_SLUG="$(basename "$PWD")"
backscroll search "$PROJECT_SLUG context decisions handoff blockers" --all-projects --max-tokens 4000
```

If this returns no useful results, run one broader session search:

```bash
backscroll search "$PROJECT_SLUG" --source session --all-projects --max-tokens 4000
```

If a result includes a useful `source_path`, drill into it before leaving the indexed boundary:

```bash
SOURCE_PATH="<result_N_source_path>"
backscroll search --source-path "$SOURCE_PATH" --all-projects --max-tokens 4000
```

For empty results or suspected gaps, follow the main skill's search discipline rather than raw-file fallback.

## Optional Rootline State

Run Rootline commands only when `rootline` exists and the target directory exists. Do not assume field names; inspect the schema first.

### Session-state records

```bash
if command -v rootline >/dev/null 2>&1 && [ -d .claude/session-state ] && find .claude/session-state -name .stem -print -quit | grep -q .; then
  rootline validate --all .claude/session-state -o json
  rootline describe .claude/session-state -o json
  rootline query .claude/session-state -o table --limit 10
fi
```

If validation fails, report the validation output and do not rely on session-state query results.

### Roadmap state

```bash
if command -v rootline >/dev/null 2>&1 && [ -f .claude/roadmap.local.md ]; then
  ROADMAP_ROOT="$(awk -F': *' '/^roadmap-root:/ {print $2; exit}' .claude/roadmap.local.md)"
  if [ -n "$ROADMAP_ROOT" ] && [ -d "$ROADMAP_ROOT" ]; then
    rootline stats "$ROADMAP_ROOT" -o table
    rootline tree "$ROADMAP_ROOT" -o table
  fi
fi
```

### Other Rootline directories

```bash
if command -v rootline >/dev/null 2>&1; then
  for dir in lines theories; do
    if [ -d "$dir" ]; then
      rootline query "$dir" -o table --limit 10
    fi
  done
fi
```

## Output

Report exactly three sections:

1. `Backscroll`: relevant sessions/documents and paths.
2. `Rootline`: live records found, or `not available` with the skipped gate.
3. `Gaps`: missing manifests, empty index, absent session-state, or schema/validation errors.
