# Semantic Lineage Closure for Backscroll Issue #58

**Status:** Approved design

**Date:** 2026-08-24

**Issue:** #58

## Decision summary

Backscroll will replace textual migration-lineage identity with a hybrid semantic schema identity and make migration closure from every recognized historical fixture to the current schema a mandatory, no-skip test boundary.

A schema's current meaning will select its migration plan. Historical `schema_migrations` rows remain inspectable provenance, but formatting differences and known historical checksum variants will not multiply otherwise equivalent schema identities. Unknown semantic shapes continue to fail closed.

The `recover` command also becomes a true remediation path: it retains startup-lock ownership but bypasses ordinary compatible-open and startup-sync preparation, because recovery must remain reachable when normal preparation cannot open the index.

## Context

Issue #58 reproduces a compatibility defect previously observed in issues #41 and #52. Backscroll recognizes multiple V13 schemas created through historical migration paths. Migration V14 appends `file_size` and `file_mtime` with `ALTER TABLE`, preserving textual differences inherited from each V13 input. Final verification compares the result against a catalog containing only one fresh-built V14 signature.

Two recognized V13 inputs therefore produce unknown V14 signatures:

- `v13-legacy-alter-built.sql` produces `sha256:9cdad03b571fd70df9c1045eae6469864b0e2a66498d7ec9f6dc7ad0af2b2122`.
- `v13-development-alter-built.sql` produces `sha256:f6a081b9df13b30fdac598103d558325c2636e4cec6625f77d1294d26bd47f89`.

`compat.VerifyCurrentShape` rejects both results before commit. The transaction rolls back to V13 and every operational command remains blocked. The local Backscroll database reproduces the `f6a081b9…` failure.

The existing systemic compatibility design already requires every supported Go lineage to migrate losslessly to head with no skipped closure evidence. The implementation violated that contract by excluding the two ALTER-built fixtures in `TestCatalogGoLineagesUpgradeLosslessly`:

```go
if fixture.name == "v13-development-alter-built.sql" || fixture.name == "v13-legacy-alter-built.sql" {
    t.Skip("legacy v13 ALTER-built fixtures are v13 compatibility cases, not v14 upgrade targets")
}
```

Those fixtures are upgrade targets by definition: Backscroll recognizes them and returns a V14 migration plan. The suite encountered the release blocker and hid it.

The failure is not limited to V13. Any recognized shape from V1 onward can preserve construction-path differences through later migrations. A V13-only correction would repeat the same incomplete boundary.

The advertised `recover --from … --dry-run` continuation is also circular. Root startup attempts the same incompatible migration before recovery and returns the same `unsupported_lineage` diagnostic instead of completing useful remediation.

## Goals

- Make equivalent SQLite schema shapes share one compatibility identity despite irrelevant DDL formatting or construction history.
- Preserve distinctions that change SQLite behavior: constraints, expressions, quoted identifiers, literals, triggers, partial indexes, foreign keys, and virtual-table configuration.
- Make every distinct checked-in historical fixture from V1 onward migrate through the real production path to head without skips.
- Make every future migration automatically exercise every supported historical fixture.
- Preserve indexed rows, UUID identities, satellite tables, FTS queryability, migration ledgers, and required snapshots.
- Keep unknown semantic shapes fail-closed.
- Make `recover --dry-run` reachable when ordinary startup preparation fails, without mutating input data.
- Remove the current ALTER-built lockout without adding V14-specific runtime branches.

## Non-goals

- Accept arbitrary third-party or manually corrupted SQLite schemas.
- Implement a complete SQLite parser or prove every possible SQL equivalence.
- Rewrite historical `schema_migrations` rows in user databases.
- Add a compatibility flag, fallback catalog, repair mode, or persistent compatibility state.
- Change migration V1–V14 SQL or data behavior.
- Change recovery union, conflict, or atomic replacement semantics beyond reachability.
- Treat release tags as a substitute for checked-in fixture evidence.

## System-reduction boundary

The design removes two accidental compatibility dimensions:

1. DDL formatting is no longer an independent lineage dimension.
2. Migration checksum history is no longer mixed into current semantic shape identity.

It does not add an exception table for V14 outcomes. One semantic catalog and one catalog-derived closure matrix replace per-migration signature multiplication.

The only startup-policy extension is the already-established conceptual remediation class for `recover`. It adds no command or persisted state; it separates ordinary preparation from the command intended to operate when ordinary preparation fails.

## Architecture

### Hybrid semantic identity

`internal/compat` inspects three independent evidence categories:

1. **Current structural shape**
   - applied migration version;
   - regular and virtual tables;
   - columns, affinity, nullability, defaults, primary-key order, and generated/hidden state where exposed;
   - indexes, uniqueness, indexed-column order, expressions, and partial-index state;
   - foreign-key actions;
   - triggers and views.

2. **Canonical auxiliary SQL**
   - constraints and expressions not fully exposed by PRAGMA;
   - trigger bodies;
   - partial-index predicates and expression indexes;
   - virtual-table module arguments, including FTS tokenizer and content configuration.

3. **Migration provenance**
   - version, name, and checksum rows from `schema_migrations`;
   - fixture and release provenance in the manifest.

The first two categories produce the semantic signature used for compatibility lookup. Provenance remains available for diagnostics, inventory, and losslessness assertions, but does not create a distinct current identity when semantic shape and applied version agree.

### Conservative SQL canonicalization

Canonical auxiliary SQL reuses a focused lexer rather than introducing a parser framework. Outside literals and quoted identifiers it:

- removes comments while retaining token separation where required;
- ignores whitespace and whitespace adjacency to punctuation;
- tokenizes punctuation and operators deterministically;
- normalizes unquoted SQLite keywords and identifiers according to case-insensitive SQLite rules;
- preserves token order.

It preserves exact contents and escapes inside single-quoted literals and double-quoted, backtick, or bracket identifiers.

It does not normalize numeric expressions, reorder expressions, rewrite constraints, or infer deeper equivalence. Conservative false negatives are acceptable; false acceptance of behaviorally distinct schemas is not.

### Catalog model

The checked-in manifest remains the hermetic inventory of releases and observed historical shapes. Each physical fixture retains:

- fixture bytes and provenance hash;
- applied version;
- release or observed-shape provenance;
- semantic signature;
- migration-ledger provenance.

Multiple physical fixtures may share a semantic signature. A collision is valid only when they share applied version, structural evidence, canonical auxiliary semantics, and remaining migration plan.

Lookup becomes conceptually `(applied_version, semantic_signature) → Lineage`. Existing `SchemaShape.Signature` and catalog method names may remain to limit API churn, but documentation changes their meaning from full textual/ledger identity to semantic identity.

Manifest regeneration recomputes semantic signatures for every fixture. It never deletes a fixture because two histories converge: every physical fixture remains an independent migration-closure input.

### Migration closure matrix

A table-driven test enumerates every distinct fixture path from the catalog, including release fixtures and explicitly observed unmanifested shapes. It has no handwritten fixture allowlist and no conditional skip path.

For each fixture it:

1. Copies the fixture into `t.TempDir()`.
2. Seeds version-appropriate sentinels in every perennial table available at that version.
3. Records expected historical ledger rows.
4. Invokes production `OpenCompatible`.
5. Applies all remaining real migrations to current head.
6. Verifies current semantic shape and an empty remaining plan.
7. Verifies search rows, UUIDs, tool rows, satellites, and available derived records.
8. Verifies message and tool FTS queryability.
9. Verifies historical ledger rows remain intact and new authoritative rows are appended.
10. Verifies snapshots exist only for applicable destructive migrations.

The input corpus begins with every supported shape, not V13 alone:

- V1 and V2;
- V3 with and without `source_metadata`;
- V4;
- V5 with and without `source_metadata`;
- V6 through V12;
- all fresh-built, release-built, development, and ALTER-built V13 forms;
- V14 and each future head fixture;
- every newly documented observed shape.

Because inputs come from the catalog and remaining steps come from the current migration catalog, adding V15 automatically exercises every historical input through V15. No recognized fixture may be relabeled as a non-upgrade target.

### Recovery startup boundary

`recover` currently uses general mutation startup. That forces ordinary compatible-open and startup sync before its handler, creating a loop when migration itself fails.

Root startup will classify `recover` as remediation while retaining the canonical startup lock and exclusive ownership. A remediation invocation:

1. Validates CLI and configuration inputs.
2. Acquires the existing startup lock under the mutation timeout contract.
3. Skips ordinary `OpenCompatible` preparation and mandatory pre-handler sync.
4. Enters the recovery handler, whose adapters open active and `--from` inputs read-only.
5. In apply mode, retains the lock through replacement, verification, and post-install sync.
6. In `--dry-run`, performs no input-data writes.

A continuation is valid only if it reaches the remediation planner rather than reproducing the same startup diagnostic.

## Data flow

```text
historical SQLite
    ├── structural PRAGMA evidence ─────┐
    ├── canonical auxiliary DDL ────────┼── semantic shape
    └── migration ledger ─ provenance ──┘         │
                                                  ▼
                                      semantic catalog lookup
                                                  │
                            ┌─────────────────────┴─────────────────────┐
                            │                                           │
                    supported semantic shape                     unknown shape
                            │                                           │
                    real migration plan                       unsupported_lineage
                            │
               transaction + semantic verification
                            │
                  recognized semantic head
```

Recovery uses a separate startup edge:

```text
recover → startup lock → recovery planner → dry-run report
                       └→ verified apply → replacement → post-install sync
```

## Error handling

### Unknown semantics

A readable database whose semantic signature is absent remains `unsupported_lineage`. Equivalent formatting, comments, punctuation spacing, and known ledger variants must not trigger it.

### Migration provenance

Historical checksums are provenance, not current semantic identity. Unknown provenance does not override a fully recognized semantic shape. Contradictory, duplicated, non-monotonic, or structurally invalid ledger rows still fail because they make the applied-version claim unreliable.

### Failed migration

Execution and final semantic verification remain within one transaction. Any failure rolls back schema and data changes. Diagnostics identify the starting fixture/shape, failed step, and produced semantic signature.

### Semantic collision

If two fixtures share a signature but disagree on applied version, structural evidence, auxiliary semantics, or remaining steps, catalog loading fails. No arbitrary map winner is accepted.

### Recovery loop

A recovery continuation that re-enters ordinary preparation is a test failure. `recover --dry-run` must produce a recovery plan or recovery-specific diagnostic after inspecting its source; it cannot repeat startup `migration_failed` as the primary result.

## Testing strategy

### Equivalent shapes must collide

- whitespace around `(`, `)`, and `,`;
- inline versus multiline declarations;
- comments and comment-contained quotes;
- fresh-created versus ALTER-appended equivalent columns;
- case differences in unquoted SQL tokens.

### Behaviorally distinct shapes must not collide

- different literals or quoted identifiers;
- `CHECK` expressions;
- `ON CONFLICT` behavior;
- foreign-key actions or deferrability;
- generated-column expressions;
- partial-index predicates;
- trigger bodies;
- FTS tokenizer or virtual-table arguments.

### Catalog evidence

- Every release maps to an existing fixture with verified bytes.
- Every unmanifested fixture has observed-shape provenance.
- Collision groups agree on version, evidence, and remaining plan.
- Regeneration is deterministic.
- Converging semantic identities never remove fixture histories.

### Migration closure

`TestEveryCatalogFixtureReachesCurrentSemanticHead` exercises every physical fixture without a skip branch. Failures report fixture, starting version/signature, failed migration, and produced signature.

Focused #58 tests retain the exact ALTER-built signatures as evidence but add no runtime signature exceptions.

### Recovery

- `recover --dry-run` against the development ALTER-built fixture reaches planning.
- Dry-run preserves database bytes, WAL/SHM state, and row counts.
- Apply retains startup-lock ownership through post-install sync.
- Concurrent mutation behavior retains the existing five-second bound.

### Repository gates

```bash
go test ./internal/compat ./internal/storage ./internal/recovery ./cmd/backscroll
just check
just test
just ci
```

The closure matrix must report zero skipped fixture cases. Aggregate coverage remains at least 85 percent.

## File boundaries

| Path | Responsibility |
|---|---|
| `internal/compat/schema.go` | Hybrid semantic inspection and auxiliary-SQL canonicalization. |
| `internal/compat/schema_test.go` | Equivalence and non-equivalence evidence. |
| `internal/compat/catalog.go` | Semantic lookup and collision validation. |
| `internal/compat/catalog_test.go` | Manifest accountability and deterministic regeneration. |
| `internal/compat/regenerate_manifest.go` | Recompute signatures without deleting converged histories. |
| `internal/compat/testdata/release-schemas/manifest.json` | Fixture provenance and semantic signatures. |
| `internal/storage/migration_plan_test.go` | Catalog-derived closure matrix and losslessness assertions. |
| `cmd/backscroll/startup_commands.go` | Explicit remediation classification for `recover`. |
| `cmd/backscroll/startup_coordination.go` | Lock retention with remediation preparation bypass. |
| `cmd/backscroll/recover_test.go` and startup tests | Dry-run reachability, no diagnostic loop, and lock behavior. |
| `docs/adr/0004-identidad-semantica-y-cierre-de-linajes.md` | Versioned architectural decision. |

No new production package is required. The lexer may move to one focused file inside `internal/compat` if needed for reviewability; no public parser abstraction is introduced.

## Alternatives rejected

### Add missing V14 signatures

This unblocks current databases but preserves signature multiplication. V15 could produce one new head signature per historical formatting/checksum combination.

### Normalize punctuation only

This fixes #58's immediate textual difference but leaves migration history and other accidental DDL differences mixed into current identity.

### Build a complete SQLite parser

An AST could model deeper equivalence but introduces a major subsystem for SQLite dialect details, triggers, virtual tables, and future syntax. The hybrid model relies on SQLite metadata first.

### Trust version and checksum alone

Equal version labels have represented divergent shapes, while different checksum histories can reach equivalent shapes. This ignores current database semantics.

### Keep recovery in ordinary mutation startup

This preserves one startup branch but makes remediation depend on the condition it must remediate, producing a circular continuation.

## Rollout and reversibility

No user-database migration is required. Semantic signatures are computed at runtime and stored only in the checked-in catalog.

Implementation, regenerated manifest, and tests can be reverted together without changing database bytes or historical ledger rows. Recovery startup classification is separately revertible, although reverting it restores the continuation loop.

A development build must verify the real `~/.backscroll.db` affected by `f6a081b9…`. Release evidence includes successful `status`, a zero-skip closure matrix, and `recover --dry-run` reaching recovery planning.

## Acceptance criteria

- [ ] Every distinct catalog fixture from V1 onward reaches current head through `OpenCompatible`.
- [ ] The closure matrix contains no fixture exclusions or conditional skips.
- [ ] Both #58 ALTER-built V13 fixtures migrate to the recognized semantic head.
- [ ] Equivalent fresh-built and ALTER-built schemas share a semantic signature.
- [ ] Behaviorally distinct DDL remains distinct.
- [ ] Ledger provenance no longer multiplies equivalent identities.
- [ ] Invalid or contradictory ledgers remain fail-closed.
- [ ] Collision groups agree on version, evidence, and remaining plan.
- [ ] Rows, UUIDs, satellites, FTS, ledger rows, and required snapshots survive every path.
- [ ] A future migration automatically extends every historical fixture path.
- [ ] `recover --dry-run` bypasses ordinary compatible-open, reaches planning, and performs no input-data mutation.
- [ ] No runtime branch special-cases the V14 failure signatures.
- [ ] Focused packages, `just check`, `just test`, and `just ci` pass.
