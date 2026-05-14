---
estado: Pending
tipo: task
---
# T007: Update README badges and CONTRIBUTING.md to reflect Go port

**Contribuye a**: ensure public-facing documentation reflects the current Go implementation (O06 — Go port — was completed but docs still reference Rust).

## Estado inicial esperado

- `README.md` has Rust version badge and possibly Rust references in CI/build sections
- `CONTRIBUTING.md` says "Requires Rust 1.85+", references `cargo fmt`, `cargo clippy`, `cargo test`, `cargo deny`, `cargo insta review`, `just check` (clippy), `just audit` (cargo audit)

## Alcance

**In**:
- `README.md`: replace Rust badge with Go badge (Go 1.26.2+), update any Rust-specific sections to Go equivalents
- `CONTRIBUTING.md`:
  - Replace "Rust 1.85+" with "Go 1.26.2+"
  - Replace `cargo fmt` → `gofmt`, `cargo clippy` → `golangci-lint`, `cargo test` → `go test ./...`, `cargo deny` → `govulncheck`, `cargo audit` → `govulncheck`
  - Remove `cargo insta review` (not applicable to Go)
  - Update pre-commit hook description: `gofmt check + golangci-lint + gitleaks` (already correct in .githooks)
  - Update Quality Gates section to match the actual Go CI gates

**Out**:
- No changes to actual hook scripts or CI workflows

## Criterios de Aceptación

- `grep -i "rust\|cargo" /home/shared/backscroll/README.md` returns no results (or only historical references)
- `grep -i "rust 1\|cargo " /home/shared/backscroll/CONTRIBUTING.md` returns no results
- `git -C /home/shared/backscroll log --oneline -1` shows a conventional commit

## Fuente de verdad

- /home/shared/backscroll/README.md
- /home/shared/backscroll/CONTRIBUTING.md
- /home/shared/backscroll/.githooks/pre-commit (reference for actual hook behavior)
