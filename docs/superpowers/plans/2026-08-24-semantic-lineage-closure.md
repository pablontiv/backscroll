# Semantic Lineage Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace textual/checksum lineage identity with conservative semantic schema identity and prove every catalog fixture from V1 onward reaches current head without skipped migration paths.

**Architecture:** Keep `internal/compat` stateless and read-only. Derive a semantic signature from SQLite structural metadata plus token-canonicalized DDL, keep migration rows as separate provenance, and key catalog lookup by `(AppliedVersion, Signature)`. Drive one lossless production-path migration test from every distinct physical fixture in the catalog.

**Tech Stack:** Go 1.26.2, `database/sql`, `modernc.org/sqlite`, SHA-256, embedded JSON/SQL fixtures, stdlib testing, Just

**Spec:** `docs/superpowers/specs/2026-08-24-semantic-lineage-closure-design.md`

## Global Constraints

- Unknown semantic shapes remain fail-closed with `unsupported_lineage`.
- Do not add V14 signature exceptions, compatibility flags, fallback catalogs, repair modes, or persistent state.
- Preserve exact contents of string literals and quoted identifiers.
- Preserve behaviorally meaningful DDL distinctions: constraints, expressions, triggers, partial indexes, foreign keys, generated columns, and virtual-table arguments.
- Migration checksum history is provenance: it is inspected and preserved but does not multiply equivalent semantic identities.
- Every distinct physical fixture remains a closure input even when signatures converge.
- The closure test contains no fixture allowlist and no conditional `t.Skip` path.
- Do not modify migration V1–V14 SQL bodies.
- All tests are hermetic and use `t.TempDir()` or in-memory SQLite.
- Follow RED → GREEN → REFACTOR, focused tests before package and repository gates.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/compat/sql_canonical.go` | Token-canonicalize SQLite DDL while preserving literals and quoted identifiers. |
| `internal/compat/sql_canonical_test.go` | Equivalent and behaviorally distinct SQL evidence. |
| `internal/compat/schema.go` | Inspect structural shape; load migration provenance separately; produce semantic signature. |
| `internal/compat/schema_test.go` | Semantic signature, provenance separation, malformed-ledger, and unsupported-shape tests. |
| `internal/compat/types.go` | Keep `SchemaShape` stable; no new public compatibility state. |
| `internal/compat/catalog.go` | Composite shape lookup and collision-safe lineage attachment. |
| `internal/compat/catalog_test.go` | Composite-key, collision, fixture retention, and checked-in signature tests. |
| `internal/compat/regenerate_manifest.go` | Deterministically regenerate semantic signatures while preserving every fixture mapping. |
| `internal/compat/testdata/release-schemas/manifest.json` | Regenerated semantic signatures for all physical fixtures. |
| `internal/storage/recovery_records.go` | Validate recovery inputs by complete `SchemaShape`, not signature alone. |
| `internal/storage/recovery_records_test.go` | Recovery lookup rejects wrong version even if semantic signature matches. |
| `internal/storage/migration_plan_test.go` | Catalog-derived V1→head closure matrix with losslessness assertions and no skips. |
| `CLAUDE.md` | Record semantic identity and mandatory historical closure invariant. |

### Task 1: Introduce conservative DDL token canonicalization

**Files:**
- Create: `internal/compat/sql_canonical.go`
- Create: `internal/compat/sql_canonical_test.go`

**Interfaces:**
- Consumes: SQLite DDL strings already loaded through `loadSQLiteObjects`.
- Produces: `func canonicalSQL(string) string`; `normalizeSQL` is removed after all internal callers migrate.

- [ ] **Step 1: Write equivalence tests that fail on current normalization**

Create `internal/compat/sql_canonical_test.go` with table-driven tests:

```go
package compat

import "testing"

func TestCanonicalSQLEquivalentRepresentationsMatch(t *testing.T) {
    tests := []struct {
        name        string
        left, right string
    }{
        {
            name:  "punctuation whitespace",
            left:  "CREATE TABLE s ( a TEXT, b INTEGER )",
            right: "create table s(a text,b integer)",
        },
        {
            name:  "comments are not semantics",
            left:  "CREATE TABLE s (a TEXT /* historical layout */, b INTEGER)",
            right: "CREATE TABLE s(a TEXT,b INTEGER)",
        },
        {
            name:  "alter appended layout",
            left:  "CREATE TABLE s (a TEXT, b INTEGER\n)",
            right: "CREATE TABLE s (a TEXT, b INTEGER)",
        },
        {
            name:  "comment token boundary",
            left:  "CREATE TABLE s (a/**/TEXT)",
            right: "CREATE TABLE s (a TEXT)",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got, want := canonicalSQL(tt.left), canonicalSQL(tt.right); got != want {
                t.Fatalf("canonical SQL differs\nleft:  %q\nright: %q", got, want)
            }
        })
    }
}
```

- [ ] **Step 2: Write non-equivalence tests before implementation**

Add exact distinctions required by the spec:

```go
func TestCanonicalSQLBehavioralDifferencesRemainDistinct(t *testing.T) {
    tests := []struct {
        name        string
        left, right string
    }{
        {"literal", "CREATE TABLE s(a TEXT DEFAULT 'x  y')", "CREATE TABLE s(a TEXT DEFAULT 'x y')"},
        {"quoted identifier", `CREATE TABLE "a  b"(id INTEGER)`, `CREATE TABLE "a b"(id INTEGER)`},
        {"check", "CREATE TABLE s(a INTEGER CHECK(a > 0))", "CREATE TABLE s(a INTEGER CHECK(a >= 0))"},
        {"conflict", "CREATE TABLE s(a TEXT UNIQUE)", "CREATE TABLE s(a TEXT UNIQUE ON CONFLICT REPLACE)"},
        {"deferrable", "CREATE TABLE s(a INTEGER REFERENCES p(id))", "CREATE TABLE s(a INTEGER REFERENCES p(id) DEFERRABLE)"},
        {"partial index", "CREATE INDEX i ON s(a) WHERE a > 0", "CREATE INDEX i ON s(a) WHERE a >= 0"},
        {"trigger", "CREATE TRIGGER t AFTER INSERT ON s BEGIN SELECT 1; END", "CREATE TRIGGER t AFTER INSERT ON s BEGIN SELECT 2; END"},
        {"fts tokenizer", "CREATE VIRTUAL TABLE f USING fts5(body, tokenize='porter')", "CREATE VIRTUAL TABLE f USING fts5(body, tokenize='trigram')"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if canonicalSQL(tt.left) == canonicalSQL(tt.right) {
                t.Fatalf("%s unexpectedly collided", tt.name)
            }
        })
    }
}
```

- [ ] **Step 3: Run RED and verify the missing function is the reason**

Run:

```bash
go test ./internal/compat -run '^TestCanonicalSQL'
```

Expected: FAIL to compile with `undefined: canonicalSQL`.

- [ ] **Step 4: Implement token canonicalization in a focused file**

Create `internal/compat/sql_canonical.go` with these exact private interfaces:

```go
package compat

import (
    "strconv"
    "strings"
    "unicode"
)

type sqlTokenKind byte

const (
    sqlBare sqlTokenKind = iota
    sqlLiteral
    sqlQuotedIdentifier
    sqlPunctuation
)

type sqlToken struct {
    kind sqlTokenKind
    text string
}

func canonicalSQL(input string) string {
    tokens := scanSQLTokens(input)
    var encoded strings.Builder
    for _, token := range tokens {
        encoded.WriteByte(byte('0' + token.kind))
        encoded.WriteByte(':')
        encoded.WriteString(strconv.Itoa(len(token.text)))
        encoded.WriteByte(':')
        encoded.WriteString(token.text)
    }
    return encoded.String()
}
```

Implement `scanSQLTokens(input string) []sqlToken` as a single forward scanner with these rules, in this order:

1. Skip Unicode/ASCII whitespace.
2. On `--`, consume through CR/LF and emit no token.
3. On `/*`, consume through the first `*/` and emit no token.
4. On `'`, copy through the closing quote, preserving `''` escapes, and emit `sqlLiteral` with exact bytes.
5. On `"`, `` ` ``, or `[`, copy through the matching delimiter, preserving doubled `""`, and emit `sqlQuotedIdentifier` with exact bytes.
6. Match the longest punctuation/operator from `->>`, `||`, `->`, `<<`, `>>`, `<=`, `>=`, `==`, `!=`, `<>`, then single-byte `(),;.+-*/%<>=&|~` and emit `sqlPunctuation`.
7. Consume a bare token until whitespace, quote, comment opener, or punctuation; emit `sqlBare` with `strings.ToLower`.
8. If a quote or block comment is unterminated, preserve the remaining bytes in its token. `sqlite_master` should not contain malformed DDL, but deterministic output is required.

The length-prefixed token encoding is mandatory. Do not join raw tokens with a sentinel byte: SQL literals can contain arbitrary control bytes and would make delimiter-based serialization ambiguous.

Use one helper per quoted form:

```go
func scanSingleQuoted(input string, start int) (string, int)
func scanDelimitedIdentifier(input string, start int, open, close byte, doubledClose bool) (string, int)
func longestSQLOperator(input string, offset int) (string, bool)
func isSQLPunctuation(ch byte) bool
```

`scanSQLTokens` must always advance at least one byte. Use `unicode.IsSpace` only when decoding a valid rune; SQL punctuation and quotes remain byte-oriented so exact literal bytes survive.

- [ ] **Step 5: Run canonicalization tests and confirm GREEN**

Run:

```bash
go test ./internal/compat -run '^TestCanonicalSQL'
```

Expected: PASS.

- [ ] **Step 6: Keep the new canonicalizer isolated until identity changes atomically**

Do not change `schema.go` in this task. The existing `normalizeSQL` remains the production path until Task 2 can switch canonicalization, provenance, catalog keys, and manifest signatures together without leaving the repository red.

- [ ] **Step 7: Run the complete canonicalizer test file**

Run:

```bash
go test ./internal/compat -run '^TestCanonicalSQL'
```

Expected: PASS. Existing compat and storage behavior remains unchanged because `canonicalSQL` is not wired yet.

- [ ] **Step 8: Commit the independently reviewed canonicalizer**

```bash
git add internal/compat/sql_canonical.go internal/compat/sql_canonical_test.go
git commit -m "feat(compat): canonicalize semantic ddl tokens"
```

### Task 2: Separate migration provenance and key catalog lookup by full shape

**Files:**
- Modify: `internal/compat/schema.go:22-135,184-235,247-253,415-632`
- Modify: `internal/compat/schema_test.go:1-655`
- Modify: `internal/compat/catalog.go:17-199`
- Modify: `internal/compat/catalog_test.go:70-150,379-435`
- Modify: `internal/compat/regenerate_manifest.go:17-139`
- Modify: `internal/compat/testdata/release-schemas/manifest.json`
- Modify: `internal/storage/recovery_records.go:19-52`
- Modify: `internal/storage/recovery_records_test.go`

**Interfaces:**
- Consumes: `SchemaShape{AppliedVersion, Signature}` and `canonicalSQL` from Task 1.
- Produces:

```go
type lineageKey struct {
    appliedVersion int
    signature      string
}

func (c Catalog) ByShape(shape SchemaShape) (Lineage, bool)
func (c Catalog) IsKnownShape(shape SchemaShape) bool
func (c Catalog) CurrentShape() SchemaShape
```

`BySignature`, `IsKnownSignature`, and `CurrentSignature` are removed after all callers migrate.

- [ ] **Step 1: Write failing provenance-separation tests**

Add to `schema_test.go`:

```go
func TestSemanticSignatureIgnoresMigrationChecksumHistory(t *testing.T) {
    const schema = `
        CREATE TABLE schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_on TEXT NOT NULL,
            checksum TEXT NOT NULL
        );
        CREATE TABLE items (id INTEGER PRIMARY KEY, body TEXT NOT NULL);
    `
    left := openSchema(t, schema+`INSERT INTO schema_migrations VALUES (1, 'v1', 'clock-a', 'published');`)
    defer left.Close()
    right := openSchema(t, schema+`INSERT INTO schema_migrations VALUES (1, 'v1', 'clock-b', 'development');`)
    defer right.Close()

    leftShape, err := inspectShape(context.Background(), left)
    if err != nil { t.Fatal(err) }
    rightShape, err := inspectShape(context.Background(), right)
    if err != nil { t.Fatal(err) }
    if leftShape.AppliedVersion != rightShape.AppliedVersion || leftShape.Signature != rightShape.Signature {
        t.Fatalf("equivalent current shapes differ: left=%+v right=%+v", leftShape.SchemaShape, rightShape.SchemaShape)
    }
}
```

Add a structural-name test because PRAGMA returns preserved spelling even for case-insensitive unquoted identifiers:

```go
func TestSemanticSignatureNormalizesUnquotedStructuralNames(t *testing.T) {
    left := openSchema(t, `CREATE TABLE Items (Body TEXT); CREATE INDEX ItemIndex ON Items(Body);`)
    defer left.Close()
    right := openSchema(t, `create table items (body text); create index itemindex on items(body);`)
    defer right.Close()
    leftShape, err := inspectShape(context.Background(), left)
    if err != nil { t.Fatal(err) }
    rightShape, err := inspectShape(context.Background(), right)
    if err != nil { t.Fatal(err) }
    if leftShape.Signature != rightShape.Signature {
        t.Fatalf("unquoted case changed semantic signature: left=%s right=%s", leftShape.Signature, rightShape.Signature)
    }
}
```

Add a second test proving version remains outside the hash but inside identity:

```go
func TestCatalogIdentityIncludesAppliedVersion(t *testing.T) {
    catalog := Catalog{lineages: map[lineageKey]Lineage{
        {appliedVersion: 1, signature: "sha256:same"}: {shape: SchemaShape{AppliedVersion: 1, Signature: "sha256:same"}},
        {appliedVersion: 2, signature: "sha256:same"}: {shape: SchemaShape{AppliedVersion: 2, Signature: "sha256:same"}},
    }}
    if _, ok := catalog.ByShape(SchemaShape{AppliedVersion: 1, Signature: "sha256:same"}); !ok { t.Fatal("v1 missing") }
    if _, ok := catalog.ByShape(SchemaShape{AppliedVersion: 2, Signature: "sha256:same"}); !ok { t.Fatal("v2 missing") }
    if _, ok := catalog.ByShape(SchemaShape{AppliedVersion: 3, Signature: "sha256:same"}); ok { t.Fatal("unknown v3 accepted") }
}
```

- [ ] **Step 2: Run RED**

Run:

```bash
go test ./internal/compat -run '^(TestSemanticSignatureIgnoresMigrationChecksumHistory|TestCatalogIdentityIncludesAppliedVersion)$'
```

Expected: checksum test FAILS because migration rows affect the hash; catalog test FAILS to compile because `lineageKey` and `ByShape` do not exist.

- [ ] **Step 3: Wire canonical DDL and load migration provenance outside semantic records**

Replace every `normalizeSQL` call in `schema.go` with `canonicalSQL`, delete the old `normalizeSQL` implementation, and move any still-relevant quoted-literal/identifier tests to `sql_canonical_test.go`.

Add:

```go
func canonicalStructuralName(name string) string { return strings.ToLower(name) }
```

Use it only in records that contribute to the signature: `sqliteObject.table/name`, column record names, index record table/name, and PRAGMA-returned index column names. Lowercase trimmed column type affinity in `tableColumn.signature`. Keep `columnsByTable` keys at their actual SQLite spelling for migration planning. Full canonical DDL remains in the signature, so quoted identifier contents still remain distinct.

Replace `loadSchemaMigrationRecords` with:

```go
type migrationProvenance struct {
    version  int
    name     string
    checksum string
}

func loadMigrationProvenance(ctx context.Context, q Queryer) (rows []migrationProvenance, appliedVersion int, err error)
```

Use the existing ordered SQL query and scan logic. Validate during iteration:

- version must be positive;
- versions must increase strictly;
- name and checksum must be non-empty;
- `appliedVersion` is the final version.

Return wrapped errors naming `schema_migrations`. Do not append migration rows to the `records` slice hashed by `inspectShape`.

In `inspectShape`, preserve support for an empty database with no ledger:

```go
var provenance []migrationProvenance
appliedVersion := 0
if hasObject(objects, "table", "schema_migrations") {
    provenance, appliedVersion, err = loadMigrationProvenance(ctx, q)
    if err != nil { return inspectedShape{}, err }
}
```

Extend the private shape only:

```go
type inspectedShape struct {
    SchemaShape
    columnsByTable      map[string]map[string]bool
    migrationProvenance []migrationProvenance
}
```

Set `migrationProvenance: provenance` in the result. Do not expose provenance in `SchemaShape` or migration plans.

- [ ] **Step 4: Implement composite catalog identity**

Change `Catalog.lineages` to `map[lineageKey]Lineage`; add `currentShape SchemaShape`; implement:

```go
func keyForShape(shape SchemaShape) lineageKey {
    return lineageKey{appliedVersion: shape.AppliedVersion, signature: shape.Signature}
}

func (c Catalog) ByShape(shape SchemaShape) (Lineage, bool) {
    lineage, ok := c.lineages[keyForShape(shape)]
    return lineage, ok
}

func (c Catalog) IsKnownShape(shape SchemaShape) bool {
    _, ok := c.ByShape(shape)
    return ok
}

func (c Catalog) CurrentShape() SchemaShape { return c.currentShape }
```

Update `attachLineages` to key each fixture by `keyForShape(shape)`. Set `currentShape` from the latest release's `AppliedVersion` and `Signature`.

Update `InspectIndex`:

```go
lineage, ok := defaultCatalog.ByShape(shape.SchemaShape)
```

Update catalog tests to assert `CurrentShape()` rather than `CurrentSignature()`.

- [ ] **Step 5: Make recovery validate complete shape identity**

Rename the private helper:

```go
func readRecordsForShape(ctx context.Context, q compat.Queryer, shape compat.SchemaShape) ([]models.IndexedRecord, *compat.Diagnostic, error)
```

Call `catalog.IsKnownShape(shape)` and include both version and signature in unsupported summaries. Update `ReadRecoveryInputFromQueryer` to pass `plan.From`.

Add to `recovery_records_test.go`:

```go
func TestReadRecordsForShapeRejectsKnownSignatureWithUnknownVersion(t *testing.T) {
    dbPath := createFixtureDatabase(t, "v14.sql")
    db, err := OpenReadOnly(dbPath)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    catalog, err := compat.LoadCatalog()
    if err != nil { t.Fatal(err) }
    shape := catalog.CurrentShape()
    shape.AppliedVersion++
    records, diag, err := readRecordsForShape(context.Background(), db.DB(), shape)
    if err != nil { t.Fatal(err) }
    if records != nil || diag == nil || diag.Code != compat.CodeUnsupportedLineage {
        t.Fatalf("records=%v diagnostic=%+v", records, diag)
    }
}
```

- [ ] **Step 6: Regenerate semantic signatures exactly once**

Run from repository root:

```bash
REGEN_MANIFEST=1 go test ./internal/compat -run '^TestRegenerateManifestOnNormalizationChange$' -v
```

Expected: PASS and `manifest.json` updated. Fixture provenance hashes remain unchanged because fixture bytes were not edited. Multiple physical fixtures may now share a semantic signature.

- [ ] **Step 7: Run the affected packages to GREEN**

```bash
go test ./internal/compat ./internal/storage
```

Expected: PASS. No test observes stale checked-in signatures or calls the removed signature-only catalog API.

- [ ] **Step 8: Commit the atomic semantic identity switch**

```bash
git add internal/compat/schema.go internal/compat/schema_test.go internal/compat/catalog.go \
  internal/compat/catalog_test.go internal/compat/regenerate_manifest.go \
  internal/compat/testdata/release-schemas/manifest.json \
  internal/storage/recovery_records.go internal/storage/recovery_records_test.go
git commit -m "fix(compat): identify semantic schema lineages"
```

### Task 3: Harden semantic collision and fixture-retention guarantees

**Files:**
- Modify: `internal/compat/catalog.go:139-199`
- Modify: `internal/compat/catalog_test.go:120-150,297-435`
- Modify: `internal/compat/regenerate_manifest.go:17-139`
- Modify: `internal/compat/testdata/release-schemas/manifest.json`

**Interfaces:**
- Consumes: composite identity and semantic signatures from Tasks 1–2.
- Produces: deterministic checked-in semantic signatures and collision-safe `attachLineages`.

- [ ] **Step 1: Write collision-consistency tests before tightening attachment**

Replace reflection-based collision testing with tests through `attachLineages`. Add a helper fixture catalog and these cases:

```go
func TestAttachLineagesAcceptsEquivalentPhysicalHistories(t *testing.T) {
    catalog := Catalog{UnmanifestedFixtures: []catalogFixture{
        {Fixture: "fresh.sql", Signature: "sha256:same", AppliedVersion: 13, HasSourceMetadata: false, Provenance: "fresh"},
        {Fixture: "alter.sql", Signature: "sha256:same", AppliedVersion: 13, HasSourceMetadata: false, Provenance: "alter"},
    }, LatestGoRelease: "v3.2.5", Releases: []catalogRelease{{Tag: "v3.2.5", Fixture: "fresh.sql", Signature: "sha256:same", AppliedVersion: 13}}}
    if err := catalog.attachLineages(); err != nil { t.Fatal(err) }
}

func TestAttachLineagesRejectsAmbiguousSemanticCollision(t *testing.T) {
    catalog := Catalog{UnmanifestedFixtures: []catalogFixture{
        {Fixture: "with.sql", Signature: "sha256:same", AppliedVersion: 5, HasSourceMetadata: true, Provenance: "with"},
        {Fixture: "without.sql", Signature: "sha256:same", AppliedVersion: 5, HasSourceMetadata: false, Provenance: "without"},
    }}
    if err := catalog.attachLineages(); err == nil || !strings.Contains(err.Error(), "ambiguous semantic collision") {
        t.Fatalf("collision error = %v", err)
    }
}
```

- [ ] **Step 2: Run RED**

Run:

```bash
go test ./internal/compat -run '^TestAttachLineages'
```

Expected: ambiguous collision test FAILS because the later map entry silently overwrites the first.

- [ ] **Step 3: Reject differing plans for one composite key**

In `attachLineages`, before assignment:

```go
key := keyForShape(shape)
lineage := Lineage{shape: shape, remainingSteps: remainingStepsFor(fixture.AppliedVersion, fixture.HasSourceMetadata)}
if existing, ok := lineages[key]; ok {
    if !reflect.DeepEqual(existing.remainingSteps, lineage.remainingSteps) {
        return fmt.Errorf("ambiguous semantic collision version=%d signature=%s: fixtures disagree on remaining migration plan", shape.AppliedVersion, shape.Signature)
    }
    continue
}
lineages[key] = lineage
```

Prefer a small `sameMigrationSteps(left, right []MigrationStep) bool` helper instead of adding `reflect` to production code. Comparing remaining plans captures `HasSourceMetadata` differences that alter V6 planning.

- [ ] **Step 4: Add a regenerator retention test**

Add a temp-directory test using two physical fixture files that differ only in DDL formatting and migration checksum:

```go
func TestRegenerateManifestRetainsConvergedPhysicalFixtures(t *testing.T) {
    dir := t.TempDir()
    manifestPath := filepath.Join(dir, "manifest.json")
    base := `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_on TEXT NOT NULL, checksum TEXT NOT NULL);`
    fresh := base + `INSERT INTO schema_migrations VALUES (1,'v1','clock','published'); CREATE TABLE items (id INTEGER, body TEXT);`
    altered := base + `INSERT INTO schema_migrations VALUES (1,'v1','clock','development'); CREATE TABLE items(id INTEGER,body TEXT);`
    if err := os.WriteFile(filepath.Join(dir, "fresh.sql"), []byte(fresh), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(dir, "altered.sql"), []byte(altered), 0o644); err != nil { t.Fatal(err) }
    manifest := `{
      "FirstGoRelease":"v0.3.7","LatestGoRelease":"v3.2.5",
      "Releases":[
        {"Tag":"v0.3.7","Fixture":"fresh.sql","ProvenanceSHA256":"old","Signature":"sha256:old-a","AppliedVersion":1},
        {"Tag":"v3.2.5","Fixture":"altered.sql","ProvenanceSHA256":"old","Signature":"sha256:old-b","AppliedVersion":1}
      ]
    }`
    if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil { t.Fatal(err) }
    if err := RegenerateManifestJSON(manifestPath); err != nil { t.Fatal(err) }
    regenerated, err := loadCatalogFromPath(manifestPath)
    if err != nil { t.Fatal(err) }
    if len(regenerated.Releases) != 2 { t.Fatalf("release mappings=%d want 2", len(regenerated.Releases)) }
    if regenerated.Releases[0].Fixture == regenerated.Releases[1].Fixture {
        t.Fatalf("physical histories collapsed: %+v", regenerated.Releases)
    }
    if regenerated.Releases[0].Signature != regenerated.Releases[1].Signature {
        t.Fatalf("equivalent semantic signatures differ: %+v", regenerated.Releases)
    }
}
```

The test changes no embedded fixture and proves regeneration updates mappings rather than deduplicating them.

- [ ] **Step 5: Verify catalog signatures, collision groups, and fixture retention**

Run:

```bash
go test ./internal/compat -run '^(TestReleaseSchemaFixtureSignaturesMatchCheckedInSQL|TestAttachLineages|TestRegenerateManifest|TestCheckedInReleaseSchemaManifestIsComplete)$' -v
```

Expected: PASS.

- [ ] **Step 6: Commit collision and regeneration hardening**

```bash
git add internal/compat/catalog.go internal/compat/catalog_test.go internal/compat/regenerate_manifest.go
git commit -m "test(compat): guard semantic lineage collisions"
```

### Task 4: Enforce lossless migration closure from every physical fixture

**Files:**
- Modify: `internal/storage/migration_plan_test.go:17-124,1480-1580`

**Interfaces:**
- Consumes: `compat.LoadCatalog`, composite semantic identities, production `OpenCompatible`, and existing sentinel helpers.
- Produces: `TestEveryCatalogFixtureReachesCurrentSemanticHead` as the mandatory historical closure gate.

- [ ] **Step 1: Replace the misleading closure test and delete all exclusions**

Rename `TestCatalogGoLineagesUpgradeLosslessly` to:

```go
func TestEveryCatalogFixtureReachesCurrentSemanticHead(t *testing.T)
```

Keep catalog-derived distinct fixture collection. Delete the V13 name check and both `t.Skip` lines completely. Do not add a replacement condition.

At the start, assert corpus coverage:

```go
required := []string{
    "v1.sql", "v2.sql", "v3.sql", "v3-no-source-metadata.sql",
    "v4.sql", "v5-with-source-metadata.sql", "v5-without-source-metadata.sql",
    "v6.sql", "v7.sql", "v8.sql", "v9.sql", "v10.sql", "v11.sql", "v12.sql",
    "v13.sql", "v13-legacy-existing-schema-migrations.sql",
    "v13-legacy-alter-built.sql", "v13-development-alter-built.sql", "v14.sql",
}
for _, name := range required {
    if !seen[name] { t.Fatalf("catalog closure corpus lacks %s", name) }
}
```

This list is an assertion that known historical evidence remains inventoried, not an allowlist controlling execution. The loop still executes every catalog fixture, including future ones.

- [ ] **Step 2: Strengthen final-shape and ledger assertions**

After `OpenCompatible`, require:

```go
plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
if err != nil || diag != nil { t.Fatalf("inspect current head error=%v diagnostic=%+v", err, diag) }
if len(plan.Steps) != 0 { t.Fatalf("fixture %s retained steps: %+v", fixture.name, plan.Steps) }
if plan.From != catalog.CurrentShape() {
    t.Fatalf("fixture %s reached shape %+v, want %+v", fixture.name, plan.From, catalog.CurrentShape())
}
```

Keep these losslessness checks inside every fixture subtest:

```go
assertSearchItems(t, db.DB(), want.SearchItems)
assertSearchItemsByUUID(t, db.DB(), want.ToolSearchItems)
assertTableSentinels(t, db.DB(), want)
assertFTSQueryable(t, db.DB(), "sentinelterm", len(want.SearchItems))
assertToolFTSQueryable(t, db.DB(), "sentinelcmd", len(want.ToolSearchItems))
```

Retain the snapshot assertions already keyed by `fixture.expectSnapshot`. Keep the documented historical V9 checksum adjustment for `v13-development-alter-built.sql`; provenance is preserved even though it no longer affects semantic identity.

- [ ] **Step 3: Run the exact issue regression**

Run:

```bash
go test ./internal/storage -run '^TestEveryCatalogFixtureReachesCurrentSemanticHead/(v13-legacy-alter-built.sql|v13-development-alter-built.sql)$' -v
```

Expected: both subtests PASS; no `SKIP`, and no `9cdad03b…` or `f6a081b9…` final verification failure.

- [ ] **Step 4: Run the complete V1→head matrix**

Run:

```bash
go test ./internal/storage -run '^TestEveryCatalogFixtureReachesCurrentSemanticHead$' -v
```

Expected: every physical fixture PASS, zero skipped subtests.

- [ ] **Step 5: Remove the obsolete single-path #52 regression**

Delete `TestMigratedFixtureSignatureIsInCatalog`; the complete matrix strictly supersedes its V1-only assertion. Do not keep two differently named closure guarantees.

- [ ] **Step 6: Commit the enforceable closure boundary**

```bash
git add internal/storage/migration_plan_test.go
git commit -m "test(storage): require every lineage to reach head"
```

### Task 5: Document the invariant and verify the affected real lineage safely

**Files:**
- Modify: `CLAUDE.md` — replace the whitespace-only #52 decision paragraph with semantic identity and closure requirements.

**Interfaces:**
- Consumes: completed semantic identity and closure matrix.
- Produces: maintainer guidance and release evidence for #58.

- [ ] **Step 1: Update living architecture guidance**

Document these exact rules in `CLAUDE.md`:

- schema identity is `(AppliedVersion, semantic Signature)`;
- migration rows are provenance, not signature input;
- canonical SQL discards comments/formatting but preserves literals, quoted identifiers, constraints, expressions, triggers, indexes, and virtual-table configuration;
- every physical catalog fixture is an upgrade target;
- no recognized fixture may be skipped;
- every new migration must pass `TestEveryCatalogFixtureReachesCurrentSemanticHead`.

Keep the observed #52/#58 history concise; do not retain statements claiming full DDL text and migration rows remain part of identity.

- [ ] **Step 2: Run focused and package verification**

```bash
go test ./internal/compat ./internal/storage -run 'TestCanonicalSQL|TestSemanticSignature|TestAttachLineages|TestEveryCatalogFixtureReachesCurrentSemanticHead|TestReadRecoveryInput' -v
go test ./internal/compat ./internal/storage ./internal/recovery
```

Expected: PASS with no skipped closure fixtures.

- [ ] **Step 3: Run repository gates**

```bash
just check
just test
just ci
```

Expected: PASS; aggregate statement coverage at least 85 percent.

- [ ] **Step 4: Build a dev binary and create an online backup of the affected database**

```bash
go build -o /tmp/backscroll-issue58 ./cmd/backscroll
smoke_dir="$(mktemp -d)"
sqlite3 "$HOME/.backscroll.db" ".backup '$smoke_dir/affected.db'"
```

Expected: dev binary builds; backup command exits 0. Never point the development binary at the original database.

- [ ] **Step 5: Verify the copied real `f6a081b9…` lineage reaches head**

```bash
BACKSCROLL_DATABASE_PATH="$smoke_dir/affected.db" /tmp/backscroll-issue58 status
BACKSCROLL_DATABASE_PATH="$smoke_dir/affected.db" /tmp/backscroll-issue58 validate --json
```

Expected: both commands exit 0; neither output contains `unsupported_lineage`, `f6a081b9`, or `migration_failed`. The original `$HOME/.backscroll.db` remains untouched.

If `sqlite3` is unavailable, report the smoke as pending instead of copying live SQLite files with `cp`; fixture closure remains the automated gate.

- [ ] **Step 6: Commit guidance**

```bash
git add CLAUDE.md
git commit -m "docs(compat): require semantic lineage closure"
```

- [ ] **Step 7: Record final evidence for review**

Include in the PR description:

- exact issue #58 subtests and complete fixture count;
- explicit zero-skip statement;
- regenerated signature collision count and validation result;
- focused/package/CI commands;
- copied-real-database smoke result or explicit pending reason;
- confirmation that no V14 signature-specific runtime branch was added.
