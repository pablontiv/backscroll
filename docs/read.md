---
estado: Completed
---
# Read

`backscroll read` displays a single session file with all noise stripped, showing only the human ↔ assistant dialogue.

## CLI Usage

```bash
backscroll read ~/.claude/projects/abcd/sessions/session.jsonl
```

## How It Works

Read applies the same extraction and noise filtering as `sync`, but outputs directly to the terminal instead of indexing. This is useful for reviewing a specific session without searching.

The output shows each message with its role:

```
[user]
How should we structure the database schema?

[assistant]
I'd recommend starting with three tables...
```

Messages appear in chronological order (by position in the file). Tool calls, tool results, system metadata, and all noise patterns are stripped — only the conversation remains.

## Noise Filtering

Read uses the same noise filter engine as sync. See [Sync & Indexing](sync.md) for the complete list of filtered patterns.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | File read successfully |
| `1` | Error (file not found, parse failure) |
