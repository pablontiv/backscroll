---
estado: Completed
---
# Read

`backscroll read` displays one file through the active input-manifest pipeline, showing normalized human ↔ assistant dialogue with configured noise removed.

## CLI Usage

```bash
backscroll inputs validate
backscroll read ~/.claude/projects/example/session.jsonl
```

The file must match at least one active input manifest under `<config_dir>/backscroll/inputs/*.inputs.toml`. Set `BACKSCROLL_CONFIG_DIR` to override `<config_dir>` for tests or custom installations.

## How It Works

Read applies the same manifest-driven extraction and text normalization as `sync`, but outputs directly to the terminal instead of indexing. This is useful for reviewing a specific session without searching.

The output shows each message with its role:

```text
[user]
How should we structure the database schema?

[assistant]
I'd recommend starting with three tables...
```

Messages appear in file order. Records, content blocks, and text fragments are included or dropped according to the matching manifest's `record`, `content`, and `text` sections.

## Noise Filtering

Read uses the same `[inputs.text].remove` rules as sync. See [Sync & Indexing](sync.md) for more detail.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | File read successfully |
| `1` | Error (file not found, no matching active input, parse failure) |
