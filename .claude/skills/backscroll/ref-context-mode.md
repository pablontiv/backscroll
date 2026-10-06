# Backscroll Context Mode

Use this only for `/skill:backscroll --context`. Produce a recovery brief with: Backscroll evidence, optional Rootline live state, and gaps.

Backscroll retrieval uses active manifests, mandatory startup sync, perennial SQLite, and database-backed query. Use `search` for discovery. For one selected result, use `context` with its published UUID when non-null, or with its exact published source path plus ordinal only when UUID is null. Raw `cat`, `jq`, Python, or filesystem session hunting is not a fallback.

## Required Backscroll Retrieval

```bash
backscroll validate
backscroll status
backscroll list --limit 10 --all-projects --json
```

If the user supplied a query, assign it to `QUERY`. Otherwise use the directory name plus context terms:

```bash
PROJECT_SLUG="$(basename "$PWD")"
QUERY="${QUERY:-$PROJECT_SLUG context decisions handoff blockers}"
backscroll search --text "$QUERY" --all-projects --json --fields minimal --max-tokens 4000
```

If this returns no useful results, run one broader session search:

```bash
backscroll search --text "$PROJECT_SLUG" --source session --all-projects --json --fields minimal --max-tokens 4000
```

Minimal JSON publishes `uuid`, `source_path`, and `ordinal` for each result. Select one result and keep those values together; do not use rank as ordinal.

Retrieve the selected result's immediate indexed neighborhood. Assign its published values to the shell variables before invoking one of these forms:

```bash
# Preferred when the selected result's UUID is non-null.
backscroll context --uuid "$UUID" --before 5 --after 5 --json --max-tokens 4000

# Fallback only when that result's UUID is null.
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --json --max-tokens 4000
```

Treat UUID as opaque and source path plus ordinal as an indivisible fallback. If `context_ambiguous` is returned, do not guess or select a row by role; report the ambiguity, refine search, and prefer a non-null UUID from the intended result when available. Context defaults to five positional DB records on each side, ordered within the stored source path by ordinal and then row ID. It does not select neighbors by conversational role. It caps each record text at 4000 Unicode code points and preserves the anchor while removing whole edge records to meet the successful-payload budget. `context_not_found`, `context_ambiguous`, and `context_budget_too_small` are structured diagnostics exempt from that budget. Origin is parser-backed and can remain `unknown`.

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
