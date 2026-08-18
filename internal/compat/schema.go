package compat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	defaultCatalog    Catalog
	defaultCatalogErr error
)

func init() {
	defaultCatalog, defaultCatalogErr = LoadCatalog()
}

func InspectIndex(ctx context.Context, q Queryer) (MigrationPlan, *Diagnostic, error) {
	shape, err := inspectShape(ctx, q)
	if err != nil {
		return MigrationPlan{}, nil, fmt.Errorf("inspect schema: %w", err)
	}
	plan := MigrationPlan{From: shape.SchemaShape}
	if defaultCatalogErr != nil {
		return plan, nil, fmt.Errorf("load schema catalog: %w", defaultCatalogErr)
	}
	lineage, ok := defaultCatalog.BySignature(shape.Signature)
	if !ok {
		return plan, &Diagnostic{
			Code:    CodeUnsupportedLineage,
			Summary: fmt.Sprintf("unsupported index schema %s", shape.Signature),
		}, nil
	}
	plan.Steps = lineage.RemainingSteps()
	return plan, nil, nil
}

func VerifyCurrentShape(ctx context.Context, q Queryer) error {
	plan, diag, err := InspectIndex(ctx, q)
	if err != nil {
		return err
	}
	if diag != nil {
		return fmt.Errorf("%s: %s", diag.Code, diag.Summary)
	}
	if len(plan.Steps) != 0 {
		return fmt.Errorf("index schema %s has %d pending migration step(s)", plan.From.Signature, len(plan.Steps))
	}
	return nil
}

type inspectedShape struct {
	SchemaShape
	columnsByTable map[string]map[string]bool
}

func inspectShape(ctx context.Context, q Queryer) (inspectedShape, error) {
	objects, err := loadSQLiteObjects(ctx, q)
	if err != nil {
		return inspectedShape{}, err
	}

	columnsByTable := map[string]map[string]bool{}
	var records []string
	appliedVersion := 0

	if hasObject(objects, "table", "schema_migrations") {
		migrationRows, maxVersion, err := loadSchemaMigrationRecords(ctx, q)
		if err != nil {
			return inspectedShape{}, err
		}
		records = append(records, migrationRows...)
		appliedVersion = maxVersion
	}

	virtualTables := map[string]bool{}
	for _, object := range objects {
		if object.typ == "table" && isVirtualTableSQL(object.sql) {
			virtualTables[object.name] = true
		}
	}

	for _, object := range objects {
		if isVolatileSQLiteObject(object.name) || isFTSShadowObject(object.name, virtualTables) {
			continue
		}
		records = append(records, schemaRecord(object.typ, object.table, object.name, "", normalizeSQL(object.sql)))
		if object.typ != "table" {
			continue
		}
		columns, err := loadTableColumns(ctx, q, object.name)
		if err != nil {
			return inspectedShape{}, err
		}
		for _, column := range columns {
			records = append(records, schemaRecord("column", object.name, column.name, column.signature(), ""))
			if columnsByTable[object.name] == nil {
				columnsByTable[object.name] = map[string]bool{}
			}
			columnsByTable[object.name][column.name] = true
		}
		indexRecords, err := loadIndexRecords(ctx, q, object.name)
		if err != nil {
			return inspectedShape{}, err
		}
		records = append(records, indexRecords...)
	}

	sort.Strings(records)
	signatureBytes := sha256.Sum256([]byte(strings.Join(records, "\n")))
	return inspectedShape{
		SchemaShape: SchemaShape{
			AppliedVersion: appliedVersion,
			Signature:      fmt.Sprintf("sha256:%x", signatureBytes),
		},
		columnsByTable: columnsByTable,
	}, nil
}

type sqliteObject struct {
	typ   string
	name  string
	table string
	sql   string
}

func loadSQLiteObjects(ctx context.Context, q Queryer) ([]sqliteObject, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT type, name, tbl_name, COALESCE(sql, '')
		FROM sqlite_master
		ORDER BY type, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query sqlite_master: %w", err)
	}
	defer rows.Close()

	var objects []sqliteObject
	for rows.Next() {
		var object sqliteObject
		if err := rows.Scan(&object.typ, &object.name, &object.table, &object.sql); err != nil {
			return nil, fmt.Errorf("scan sqlite_master: %w", err)
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sqlite_master: %w", err)
	}
	return objects, nil
}

func loadSchemaMigrationRecords(ctx context.Context, q Queryer) ([]string, int, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT version, name, checksum
		FROM schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, 0, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	var records []string
	maxVersion := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return nil, 0, fmt.Errorf("scan schema_migrations: %w", err)
		}
		if version > maxVersion {
			maxVersion = version
		}
		records = append(records, schemaRecord("migration", "schema_migrations", fmt.Sprintf("%013d", version), name+"|"+checksum, ""))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read schema_migrations: %w", err)
	}
	return records, maxVersion, nil
}

type tableColumn struct {
	cid       int
	name      string
	typ       string
	notNull   int
	defaultTo sql.NullString
	pk        int
}

func (c tableColumn) signature() string {
	defaultValue := "<null>"
	if c.defaultTo.Valid {
		defaultValue = normalizeSQL(c.defaultTo.String)
	}
	return fmt.Sprintf("%013d:%s:%d:%s:%d", c.cid, c.typ, c.notNull, defaultValue, c.pk)
}

func loadTableColumns(ctx context.Context, q Queryer, table string) ([]tableColumn, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("query table_info %s: %w", table, err)
	}
	defer rows.Close()

	var columns []tableColumn
	for rows.Next() {
		var column tableColumn
		if err := rows.Scan(&column.cid, &column.name, &column.typ, &column.notNull, &column.defaultTo, &column.pk); err != nil {
			return nil, fmt.Errorf("scan table_info %s: %w", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table_info %s: %w", table, err)
	}
	return columns, nil
}

type sqliteIndex struct {
	seq     int
	name    string
	unique  int
	origin  string
	partial int
}

func loadIndexRecords(ctx context.Context, q Queryer, table string) ([]string, error) {
	indexes, err := loadIndexes(ctx, q, table)
	if err != nil {
		return nil, err
	}
	var records []string
	for _, index := range indexes {
		columns, err := loadIndexColumns(ctx, q, index.name)
		if err != nil {
			return nil, err
		}
		metadata := fmt.Sprintf("unique=%d origin=%s partial=%d columns=%s", index.unique, index.origin, index.partial, strings.Join(columns, ","))
		records = append(records, schemaRecord("index", table, index.name, metadata, ""))
	}
	return records, nil
}

func loadIndexes(ctx context.Context, q Queryer, table string) ([]sqliteIndex, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA index_list("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("query index_list %s: %w", table, err)
	}
	defer rows.Close()

	var indexes []sqliteIndex
	for rows.Next() {
		var index sqliteIndex
		if err := rows.Scan(&index.seq, &index.name, &index.unique, &index.origin, &index.partial); err != nil {
			return nil, fmt.Errorf("scan index_list %s: %w", table, err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read index_list %s: %w", table, err)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].name < indexes[j].name })
	return indexes, nil
}

func loadIndexColumns(ctx context.Context, q Queryer, index string) ([]string, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA index_info("+quoteIdent(index)+")")
	if err != nil {
		return nil, fmt.Errorf("query index_info %s: %w", index, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, fmt.Errorf("scan index_info %s: %w", index, err)
		}
		columnName := "<expr>"
		if name.Valid {
			columnName = name.String
		}
		columns = append(columns, fmt.Sprintf("%013d:%013d:%s", seqno, cid, columnName))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read index_info %s: %w", index, err)
	}
	sort.Strings(columns)
	return columns, nil
}

func (c *Catalog) attachLineages(fsys fs.FS) error {
	fixturePaths, err := fs.Glob(fsys, "testdata/release-schemas/*.sql")
	if err != nil {
		return fmt.Errorf("glob release schema fixtures: %w", err)
	}
	lineages := map[string]Lineage{}
	for _, fixturePath := range fixturePaths {
		data, err := fs.ReadFile(fsys, fixturePath)
		if err != nil {
			return fmt.Errorf("read release schema fixture %q: %w", fixturePath, err)
		}
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return fmt.Errorf("open fixture database %q: %w", fixturePath, err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			db.Close()
			return fmt.Errorf("load release schema fixture %q: %w", fixturePath, err)
		}
		shape, err := inspectShape(context.Background(), db)
		closeErr := db.Close()
		if err != nil {
			return fmt.Errorf("inspect release schema fixture %q: %w", fixturePath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close release schema fixture %q: %w", fixturePath, closeErr)
		}
		lineages[shape.Signature] = Lineage{
			shape:          shape.SchemaShape,
			remainingSteps: remainingSteps(shape),
		}
		if filepath.Base(fixturePath) == "v13.sql" {
			c.currentSignature = shape.Signature
		}
	}
	c.lineages = lineages
	return nil
}

func remainingSteps(shape inspectedShape) []MigrationStep {
	hasSourceMetadata := shape.columnsByTable["search_items"]["source_metadata"]
	var steps []MigrationStep
	for _, step := range allMigrationSteps {
		if step.Version <= shape.AppliedVersion {
			continue
		}
		if step.Version == 6 && !hasSourceMetadata {
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

var allMigrationSteps = []MigrationStep{
	{Version: 1, Name: "V1 core schema"},
	{Version: 2, Name: "V2 embedding tables"},
	{Version: 3, Name: "V3 embedding blob column"},
	{Version: 4, Name: "V4 tool_fts trigram index"},
	{Version: 5, Name: "V5 drop phantom session_events"},
	{Version: 6, Name: "V6 drop source_metadata when present"},
	{Version: 7, Name: "V7 reasoning triggers"},
	{Version: 8, Name: "V8 perennity: extraction_version, was_interrupted, tool_events"},
	{Version: 9, Name: "V9 tool_events uuid uniqueness index"},
	{Version: 10, Name: "V10 template mining: message_templates, template_matches"},
	{Version: 11, Name: "V11 correction detection: correction_signals"},
	{Version: 12, Name: "V12 agent classification: annotations"},
	{Version: 13, Name: "V13 backfill discovery indexes"},
}

func hasObject(objects []sqliteObject, typ, name string) bool {
	for _, object := range objects {
		if object.typ == typ && object.name == name {
			return true
		}
	}
	return false
}

func isVolatileSQLiteObject(name string) bool {
	return strings.HasPrefix(name, "sqlite_")
}

func isVirtualTableSQL(sqlText string) bool {
	return strings.Contains(strings.ToLower(sqlText), " using fts5")
}

func isFTSShadowObject(name string, virtualTables map[string]bool) bool {
	for virtualTable := range virtualTables {
		if name == virtualTable {
			continue
		}
		for _, suffix := range []string{"_data", "_idx", "_content", "_docsize", "_config"} {
			if name == virtualTable+suffix {
				return true
			}
		}
	}
	return false
}

func normalizeSQL(sqlText string) string {
	return strings.Join(strings.Fields(sqlText), " ")
}

func schemaRecord(kind, table, name, columns, sqlText string) string {
	return strings.Join([]string{kind, table, name, columns, sqlText}, "|")
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
