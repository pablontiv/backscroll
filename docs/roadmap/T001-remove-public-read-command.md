---
estado: Pending
tipo: task
---
# T001: Remove public read command

## Preserva

- all existing search/list/topics/resume workflows unchanged

## Contexto

backscroll read (src/main.rs Commands::Read -> src/core/reader.rs read_input_file) parses files directly via discover_candidate_files and parse_input_file_with_definition without consulting the DB or triggering auto-sync. Other commands (search, resume, list, topics, insights, export, status, reindex) do auto-sync. This asymmetry breaks the DB-as-source-of-truth invariant and causes stale reads.

## Alcance

**In**:
1. remove or gate Commands::Read from public CLI
2. update backscroll skill to not use backscroll read PATH
3. provide alternative: backscroll search with source_path filter for path-based lookup

**Out**:
1. no changes to sync or search internals

## Estado inicial esperado

backscroll read is public and parses files without DB/sync

## Criterios de Aceptación

- backscroll --help no longer shows read as a public command, or read forces auto-sync before parsing
- backscroll skill uses backscroll search instead of backscroll read PATH
- path-based session lookup works via DB query

## Fuente de verdad

- src/main.rs
- src/core/reader.rs
