---
estado: Completed
tipo: task
---
# T010: Fix README Rust remnants

**Contribuye a**: complete the Go port documentation — T007 updated badges only; the Development section, From Source, docs table, and License line still reference Rust.

## Alcance

**In**:
- Fix "From Source" install command: `cargo install` → `go install`
- Replace Development section: remove rustfmt/clippy/LLVM/Zig, use Go equivalents from CLAUDE.md
- Remove "Rust Architecture" row from Documentation table
- Fix License line: `[MIT](Cargo.toml)` → `[PolyForm Noncommercial 1.0.0](LICENSE)`

**Out**:
- No changes to Core Idea, Quick Start, AI-Native, or Configuration sections (already correct)

## Criterios de Aceptación

- `grep -n "cargo\|rustfmt\|clippy\|Cargo.toml\|LLVM\|Zig\|static-build" /home/shared/backscroll/README.md` returns empty
- `grep "PolyForm" /home/shared/backscroll/README.md` exits 0
- `grep "go install" /home/shared/backscroll/README.md` exits 0
- `git -C /home/shared/backscroll log --oneline -1` shows a conventional commit

## Fuente de verdad

- /home/shared/backscroll/README.md
- /home/shared/backscroll/CLAUDE.md (Go commands reference)
