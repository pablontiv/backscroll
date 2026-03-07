# F01: Parser y Schema

**Epic**: [E06 Robustez del Motor](../README.md)
**Objetivo**: Reescribir el parser para el formato wrapper real de Claude Code JSONL y migrar el schema SQLite a content table extensible.
**Satisface**: P1 (parser real), P2 (schema content table), P4 (re-sync sin duplicados)
**Milestone**: `cargo test` pasa con fixtures JSONL reales; schema tiene search_items + messages_fts external content.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E06)
- INV2: `just check` pasa (heredado de E06)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S021](S021-modelo-jsonl-real/) | Reescribir modelo para JSONL real |
| [S022](S022-schema-versioning/) | Schema versioning + content table |
| [S023](S023-sync-metadata/) | Sync correcto con metadata |
