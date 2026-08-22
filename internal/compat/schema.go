package compat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
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
		if object.typ == "table" && !virtualTables[object.name] {
			tableRecords, columns, err := loadRegularTableRecords(ctx, q, object)
			if err != nil {
				return inspectedShape{}, err
			}
			records = append(records, tableRecords...)
			for _, column := range columns {
				if columnsByTable[object.name] == nil {
					columnsByTable[object.name] = map[string]bool{}
				}
				columnsByTable[object.name][column.name] = true
			}
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

type schemaRowsCloser interface {
	Close() error
}

func joinRowsCloseError(rows schemaRowsCloser, err *error) {
	if rows == nil || err == nil {
		return
	}
	if closeErr := rows.Close(); closeErr != nil {
		*err = errors.Join(*err, closeErr)
	}
}

func loadSQLiteObjects(ctx context.Context, q Queryer) (objects []sqliteObject, err error) {
	rows, err := q.QueryContext(ctx, `
		SELECT type, name, tbl_name, COALESCE(sql, '')
		FROM sqlite_master
		ORDER BY type, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query sqlite_master: %w", err)
	}
	defer joinRowsCloseError(rows, &err)

	for rows.Next() {
		var object sqliteObject
		if scanErr := rows.Scan(&object.typ, &object.name, &object.table, &object.sql); scanErr != nil {
			err = fmt.Errorf("scan sqlite_master: %w", scanErr)
			return nil, err
		}
		objects = append(objects, object)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("read sqlite_master: %w", rowsErr)
		return nil, err
	}
	return objects, nil
}

func loadRegularTableRecords(ctx context.Context, q Queryer, object sqliteObject) ([]string, []tableColumn, error) {
	columns, err := loadTableColumns(ctx, q, object.name)
	if err != nil {
		return nil, nil, err
	}

	// Regular tables are intentionally signed with their full SQLite DDL evidence.
	// Earlier semantic canonicalization accepted unsupported altered tables by
	// collision because PRAGMA-derived fields omit DDL semantics such as
	// AUTOINCREMENT, COLLATE, ON CONFLICT, and DEFERRABLE. The only non-canonical
	// altered shape we support is an explicit checked-in fixture/signature.
	records := []string{schemaRecord("table", object.table, object.name, "", normalizeSQL(object.sql))}
	for _, column := range columns {
		records = append(records, schemaRecord("column", object.name, column.name, column.signature(), ""))
	}
	indexRecords, err := loadIndexRecords(ctx, q, object.name)
	if err != nil {
		return nil, nil, err
	}
	records = append(records, indexRecords...)
	return records, columns, nil
}

func loadSchemaMigrationRecords(ctx context.Context, q Queryer) (records []string, maxVersion int, err error) {
	rows, err := q.QueryContext(ctx, `
		SELECT version, name, checksum
		FROM schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, 0, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer joinRowsCloseError(rows, &err)

	for rows.Next() {
		var version int
		var name, checksum string
		if scanErr := rows.Scan(&version, &name, &checksum); scanErr != nil {
			err = fmt.Errorf("scan schema_migrations: %w", scanErr)
			return nil, 0, err
		}
		if version > maxVersion {
			maxVersion = version
		}
		records = append(records, schemaRecord("migration", "schema_migrations", fmt.Sprintf("%013d", version), name+"|"+checksum, ""))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("read schema_migrations: %w", rowsErr)
		return nil, 0, err
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
	hidden    int
}

func (c tableColumn) signature() string {
	defaultValue := "<null>"
	if c.defaultTo.Valid {
		defaultValue = normalizeSQL(c.defaultTo.String)
	}
	return fmt.Sprintf("%013d:%s:%d:%s:%d:%d", c.cid, c.typ, c.notNull, defaultValue, c.pk, c.hidden)
}

func loadTableColumns(ctx context.Context, q Queryer, table string) (columns []tableColumn, err error) {
	rows, err := q.QueryContext(ctx, "PRAGMA table_xinfo("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("query table_info %s: %w", table, err)
	}
	defer joinRowsCloseError(rows, &err)

	for rows.Next() {
		var column tableColumn
		if scanErr := rows.Scan(&column.cid, &column.name, &column.typ, &column.notNull, &column.defaultTo, &column.pk, &column.hidden); scanErr != nil {
			err = fmt.Errorf("scan table_info %s: %w", table, scanErr)
			return nil, err
		}
		columns = append(columns, column)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("read table_info %s: %w", table, rowsErr)
		return nil, err
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

func loadIndexes(ctx context.Context, q Queryer, table string) (indexes []sqliteIndex, err error) {
	rows, err := q.QueryContext(ctx, "PRAGMA index_list("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("query index_list %s: %w", table, err)
	}
	defer joinRowsCloseError(rows, &err)

	for rows.Next() {
		var index sqliteIndex
		if scanErr := rows.Scan(&index.seq, &index.name, &index.unique, &index.origin, &index.partial); scanErr != nil {
			err = fmt.Errorf("scan index_list %s: %w", table, scanErr)
			return nil, err
		}
		indexes = append(indexes, index)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("read index_list %s: %w", table, rowsErr)
		return nil, err
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].name < indexes[j].name })
	return indexes, nil
}

func loadIndexColumns(ctx context.Context, q Queryer, index string) (columns []string, err error) {
	rows, err := q.QueryContext(ctx, "PRAGMA index_info("+quoteIdent(index)+")")
	if err != nil {
		return nil, fmt.Errorf("query index_info %s: %w", index, err)
	}
	defer joinRowsCloseError(rows, &err)

	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if scanErr := rows.Scan(&seqno, &cid, &name); scanErr != nil {
			err = fmt.Errorf("scan index_info %s: %w", index, scanErr)
			return nil, err
		}
		columnName := "<expr>"
		if name.Valid {
			columnName = name.String
		}
		columns = append(columns, fmt.Sprintf("%013d:%013d:%s", seqno, cid, columnName))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("read index_info %s: %w", index, rowsErr)
		return nil, err
	}
	sort.Strings(columns)
	return columns, nil
}

func remainingStepsFor(appliedVersion int, hasSourceMetadata bool) []MigrationStep {
	var steps []MigrationStep
	for _, step := range allMigrationSteps {
		if step.Version <= appliedVersion {
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
	// Collapse runs of whitespace outside SQL lexical constructs (comments, string literals, identifiers),
	// preserving whitespace within:
	// - Single-quoted string literals (including '' escapes)
	// - Double-quoted identifiers (including "" escapes)
	// - Backtick-quoted identifiers
	// - Bracket-quoted identifiers [...]
	// - Line comments (-- until end-of-line)
	// - Block comments (/* ... */ non-nesting)
	//
	// Defect fix #1: Apostrophes and quotes within comments do not affect lexer state.
	// Defect fix #2: Double-quoted identifiers preserve inner whitespace (e.g., "my  table" != "my table").

	var result strings.Builder
	lastWasSpace := false

	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]

		// Handle line comments (-- until end-of-line)
		if ch == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-' {
			// Write the comment as-is, preserving spaces/newlines until EOL
			result.WriteByte(ch)
			i++
			result.WriteByte(sqlText[i])
			lastWasSpace = false
			i++
			// Copy everything until end of line (newline or EOF), skipping CR (normalize CRLF to LF)
			for i < len(sqlText) && sqlText[i] != '\n' {
				if sqlText[i] != '\r' {
					result.WriteByte(sqlText[i])
				}
				i++
			}
			// If we found a newline, write it as a space (for collapsing purposes)
			if i < len(sqlText) && sqlText[i] == '\n' {
				result.WriteByte(' ')
				lastWasSpace = true
				i++
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Handle block comments (/* ... */ non-nesting)
		if ch == '/' && i+1 < len(sqlText) && sqlText[i+1] == '*' {
			// Write the comment marker and everything until */
			result.WriteByte(ch)
			i++
			result.WriteByte(sqlText[i]) // '*'
			lastWasSpace = false
			i++
			// Copy everything until we find */
			for i < len(sqlText) {
				if sqlText[i] == '*' && i+1 < len(sqlText) && sqlText[i+1] == '/' {
					result.WriteByte('*')
					i++
					result.WriteByte('/')
					i++
					lastWasSpace = false
					break
				}
				result.WriteByte(sqlText[i])
				i++
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Handle single-quoted string literals (preserve whitespace inside)
		if ch == '\'' {
			result.WriteByte(ch)
			lastWasSpace = false
			i++
			// Copy everything until closing single quote, handling '' escape
			for i < len(sqlText) {
				if sqlText[i] == '\'' {
					result.WriteByte('\'')
					// Check if this is an escape (followed by another single quote)
					if i+1 < len(sqlText) && sqlText[i+1] == '\'' {
						result.WriteByte('\'')
						i += 2
					} else {
						// End of string literal
						i++
						lastWasSpace = false
						break
					}
				} else {
					result.WriteByte(sqlText[i])
					i++
				}
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Handle double-quoted identifiers (preserve whitespace inside)
		if ch == '"' {
			result.WriteByte(ch)
			lastWasSpace = false
			i++
			// Copy everything until closing double quote, handling "" escape
			for i < len(sqlText) {
				if sqlText[i] == '"' {
					result.WriteByte('"')
					// Check if this is an escape (followed by another double quote)
					if i+1 < len(sqlText) && sqlText[i+1] == '"' {
						result.WriteByte('"')
						i += 2
					} else {
						// End of identifier
						i++
						lastWasSpace = false
						break
					}
				} else {
					result.WriteByte(sqlText[i])
					i++
				}
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Handle backtick-quoted identifiers (preserve whitespace inside)
		if ch == '`' {
			result.WriteByte(ch)
			lastWasSpace = false
			i++
			// Copy everything until closing backtick
			for i < len(sqlText) && sqlText[i] != '`' {
				result.WriteByte(sqlText[i])
				i++
			}
			if i < len(sqlText) && sqlText[i] == '`' {
				result.WriteByte('`')
				i++
				lastWasSpace = false
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Handle bracket-quoted identifiers [...] (preserve whitespace inside)
		if ch == '[' {
			result.WriteByte(ch)
			lastWasSpace = false
			i++
			// Copy everything until closing bracket
			for i < len(sqlText) && sqlText[i] != ']' {
				result.WriteByte(sqlText[i])
				i++
			}
			if i < len(sqlText) && sqlText[i] == ']' {
				result.WriteByte(']')
				i++
				lastWasSpace = false
			}
			i-- // Adjust for the outer loop's i++
			continue
		}

		// Outside all special contexts: collapse whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if !lastWasSpace {
				result.WriteByte(' ')
				lastWasSpace = true
			}
		} else {
			result.WriteByte(ch)
			lastWasSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}

func schemaRecord(kind, table, name, columns, sqlText string) string {
	return strings.Join([]string{kind, table, name, columns, sqlText}, "|")
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
