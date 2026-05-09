# T013 scout findings: evaluate-jmespath-future

## 1. Executive finding

**Finding: probably no immediate material benefit from taking T013 off Hold now** (confidence: **medium-high**).

T013 would be useful as a **small decision note** if Backscroll is about to support inputs that current JSONPath + closed operators cannot express. I found no local or session-history evidence of such a concrete blocker today. The stronger evidence says the opposite: Backscroll intentionally shipped the MVP with JSONPath (`serde_json_path`) plus limited predicates/transforms, and prior sessions explicitly recommended **not** adopting JMESPath as the first option because it is more power and complexity than Backscroll currently needs.

So: keep **no implementation/no dependency**. If work is desired, scope it strictly to T013’s existing acceptance criteria: compare, document pros/cons, and either reject or defer with examples.

## 2. Local artifacts

### T013 itself

- `docs/roadmap/O02-generic-agnostic-input-engine/T013-evaluate-jmespath-future.md:1-47`
  - Status is `On Hold`.
  - It explicitly preserves “JMESPath future, not MVP dependency” and says it must not block T001-T012 or add a dependency.
  - Scope is evaluation only: compare JMESPath vs JSONPath + declarative operators, evaluate Rust crates/maintenance, identify cases MVP does not cover, and document adopt/reject/postpone.
  - Out of scope: implementation, adding `jmespath` to `Cargo.toml`, blocking MVP.

### Current contract and roadmap posture

- `docs/roadmap/O02-generic-agnostic-input-engine/README.md:24-33`, `:35-52`, `:54-71`
  - O02 is completed and its invariant says JMESPath stays future-only unless a later explicit decision adds it.
  - MVP in-scope is JSON/JSONL with JSONPath plus minimal declarative filters/transforms; JMESPath is out of scope.
- `docs/input-contract.md:1-30`, `:139-183`, `:359-370`
  - Contract is provider-neutral and maps to `ParsedFile`/`ParsedMessage` via `discover -> decode -> record -> map -> content -> text -> emit`.
  - Record/content selectors are JSONPath; operators are limited to `eq`, `ne`, `in`, `exists`, `missing`.
  - Validation policy says all selectors are JSONPath in the MVP and JMESPath is reserved for T013.
- `docs/intention-agentic-input-definitions.md:13-20`, `:58-64`
  - Current MVP scope says manifests are data-only: discovery, decode, selectors, predicates, mapping, text normalization.
  - It has an “absolute rule” / no-goal: no executable adapters, scripts, plugins, or JMESPath in this MVP.
- `docs/roadmap/O03-global-user-scoped-inputs/README.md:24-33`, `:47-52`
  - Follow-on O03 repeats the invariant: no JMESPath/plugins as dependency for this MVP.

### Code/config integration points

- `Cargo.toml:31-51`
  - Contains `serde_json_path = "0.7.2"`; no `jmespath` dependency.
- `src/input_config.rs:171-212`, `:296-330`, `:474-626`, `:631-679`
  - Defines closed predicate and text-transform types.
  - Validates JSONPath selectors with `serde_json_path::JsonPath::parse`.
  - Validates globs, UTF-8, regex rules, required mapping/content sections, and rejects unknown predicate ops (test uses `contains` as invalid).
- `src/core/sync.rs:114-260`, `:263-410`, `:440-560`, `:966-1050`
  - Runtime parser compiles/evaluates JSONPath, applies predicate operators, extracts content blocks, normalizes text, supports dry-run, and emits `ParsedFile`/`ParsedMessage`.
- `src/core/sync.rs:1220-1325`
  - Tests cover object/array content, all MVP predicate operators, and excluding Pi `think` blocks without JMESPath.
- `src/main.rs:24-90`, `:208-250`, `:390-540`
  - CLI exposes `search --source-path`, JSON/robot outputs, and `backscroll inputs list/validate/test`; search autosyncs manifest inputs before querying.
- `inputs/claude.inputs.toml:1-64` and `inputs/pi.inputs.toml:1-47`
  - Shipped presets express current Claude/Pi extraction with JSONPath selectors and closed operators only.

### External/research notes already in repo

- `research-external.md:6-29`, `:31-63`
  - External evidence supports declarative ingestion pipelines and validation/dry-run.
  - It says small mapping DSLs are common, but also that expression-language choice remains open and needs separate evaluation.
  - This is relevant but not a concrete Backscroll blocker.

## 3. Session-history evidence

Backscroll searches used before raw session inspection included:

- `backscroll search "T013" --all-projects --source session --before 2026-05-09 --robot --max-tokens 1200 --limit 10`
- `backscroll search "evaluate-jmespath-future" --all-projects --source session --before 2026-05-09 --robot --max-tokens 1200 --limit 10`
- `backscroll search "jmespath rust crate maintenance" --all-projects --source session --before 2026-05-09 --robot --max-tokens 1500 --limit 10`
- `backscroll search "JMESPath JSONPath operadores basta" --all-projects --source session --before 2026-05-09 --robot --max-tokens 1500 --limit 10`
- Plus broad searches for `json query`, `filter fields`, `robot`, `json output`, `inputs`, and `source-path`.

High-signal session findings:

- `/home/pones/.pi/agent/sessions/--home-shared-backscroll--/2026-05-07T20-38-01-652Z_019e0429-62af-7422-b8cf-293606e81c6e/8fc48765/run-0/session.jsonl:62`
  - External research concluded JMESPath is a mature JSON projection option with ABNF/compliance suite, but confidence on JSONPath/JMESPath/CEL library choice in Rust is only medium because conformance, maintenance, and performance require project-specific testing.
  - It recommended MVP manifests with selectors/predicates and “Do not default to arbitrary jq/CEL transforms”; keep transformations declarative/bounded and advanced engines explicit opt-in.
- `/home/pones/.pi/agent/sessions/--home-shared-backscroll--/2026-05-07T23-33-06-978Z_019e04c9-af22-7698-b1b4-b24005cc2b03.jsonl:746`
  - Prior assistant recommendation: “No adoptaría JMESPath como primera opción”; JMESPath is viable but more than needed; prefer JSONPath selectors + small own transforms + simple declarative filters; leave JMESPath future if more expressive mapping is needed.
- Same session, line `747`
  - User direction in Spanish: “plan todo esto, deja JMESPath como pendiente” — direct intent to materialize the roadmap while leaving JMESPath pending.
- Same session, lines `769`, `797`, `808`
  - T013 was created and then reported as `On Hold` as part of O02 planning.
- `/home/pones/.pi/agent/sessions/--home-shared-backscroll--/2026-05-08T14-08-21-438Z_019e07ea-fdbd-735e-93ba-334975b798da/...`
  - O02 implementation task prompts repeatedly carried hard constraints: “No JMESPath, no plugins/scripts/adapters” and validation must use JSONPath, not JMESPath. Search hits around T008/T010/T014 reflect enforcement of the roadmap invariant, not new demand for JMESPath.

What session history **does not** show:

- No prior session identified a real Claude/Pi/document-source mapping that JSONPath + MVP operators failed to express.
- No user preference surfaced for JMESPath specifically; the only direct user preference I found was to leave it pending.
- No rejected approach says “never JMESPath”; the rejection was “not first option / not MVP / overkill until needed.”

## 4. Constraints, risks, dependencies

- **No implementation/dependency change under T013.** T013 itself excludes adding `jmespath` or touching `Cargo.toml`.
- **Current user-facing contract promises JSONPath-only MVP.** Adding JMESPath would expand docs, validation, dry-run diagnostics, examples, and support burden.
- **Crate/library risk remains unresolved.** Prior evidence says Rust choice needs conformance, maintenance, and performance testing. T013 could evaluate this, but that is not the same as adopting it.
- **Security/complexity risk.** Prior notes consistently separate “agnostic declarative” from arbitrary expression/plugin execution. A more expressive language increases error surface and documentation load.
- **Dependency trigger should be concrete unmet cases.** The best trigger would be a new manifest/source whose extraction needs projections/conditionals/coalescing not expressible by current JSONPath + closed operators.

## 5. Remaining clarification questions for implementation confidence

1. Are there upcoming non-Claude/Pi input formats with concrete examples that current JSONPath selectors and `eq/ne/in/exists/missing` cannot express?
2. If a future language is needed, should the target be projection-only JMESPath, predicate-oriented CEL, jq/jaq-like transforms, or a small Backscroll-specific operator extension?
3. Would maintainers accept a “decision note only” T013 that rejects/defers JMESPath unless a real fixture demonstrates a gap?
4. What threshold should justify adding a dependency: one shipped preset need, multiple user manifests, performance/maintainability data, or external standardization?
