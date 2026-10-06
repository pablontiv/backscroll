# Backscroll Context Mode

Use this only for `/skill:backscroll --context`. Produce a recovery brief with: Backscroll evidence, optional Rootline live state, and gaps.

Backscroll retrieval uses active manifests, mandatory startup sync, perennial SQLite, and database-backed query. Use `search` for discovery and `context` once exact UUID or source-path-plus-ordinal identity exists. Raw `cat`, `jq`, Python, or filesystem session hunting is not a fallback.

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

If a result includes exact identity, retrieve its immediate indexed neighborhood. Treat the UUID as opaque; otherwise require the exact stored source path and ordinal:

```bash
backscroll context --uuid "$UUID" --before 5 --after 5 --json --max-tokens 4000
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --json --max-tokens 4000
```

Context defaults to five records on each side, caps each record text at 4000 Unicode code points, and preserves the anchor while removing whole edge records to meet the successful-payload budget. `context_not_found`, `context_ambiguous`, and `context_budget_too_small` are structured diagnostics exempt from that budget. Origin is parser-backed and can remain `unknown`.

For empty results or suspected gaps, follow the main skill's search discipline rather than raw-file fallback. Context reads only perennial or recovered SQLite rows; it never reads provider files.

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
