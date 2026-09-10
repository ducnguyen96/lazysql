package components

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Schema context — the connection/schema block handed to the external editor
// ---------------------------------------------------------------------------

// The rendered block is delimited by these markers so it can be removed again
// when the editor closes. They are plain SQL comments, so a query that still
// carries the block is harmless if the stripping ever misses it.
const (
	schemaContextStartMarker = "-- >>> lazysql:schema >>>"
	schemaContextEndMarker   = "-- <<< lazysql:schema <<<"
)

// SchemaColumn is a single column of a table as described by the driver.
type SchemaColumn struct {
	Name       string
	Type       string
	References string // "users.id" when the column is a foreign key
	NotNull    bool
	PrimaryKey bool
}

// SchemaTable is a table and its columns.
type SchemaTable struct {
	Name    string
	Columns []SchemaColumn
}

// SchemaSnapshot is everything an assistant needs to write a query against the
// current connection: which database it is, which dialect, and what it holds.
type SchemaSnapshot struct {
	Connection string
	Provider   string
	Database   string
	Tables     []SchemaTable
	// Complete is set once every table has been described. Until then the
	// snapshot may hold table names without their columns.
	Complete bool
}

// foreignKeyEdge is one relationship, always expressed child → parent.
type foreignKeyEdge struct {
	ChildTable   string
	ChildColumn  string
	ParentTable  string
	ParentColumn string
}

// ---------------------------------------------------------------------------
// Parsing driver results
// ---------------------------------------------------------------------------

// Drivers return their own column layout for GetTableColumns and
// GetForeignKeys, so the fields are located by header name instead of by index.
var (
	columnNameHeaders     = []string{"field", "column_name", "name"}
	columnTypeHeaders     = []string{"type", "data_type", "column_type"}
	columnNullableHeaders = []string{"null", "is_nullable"}
	columnNotNullHeaders  = []string{"notnull", "not_null"}
	columnKeyHeaders      = []string{"key", "column_key"}
	columnPKHeaders       = []string{"pk", "is_primary_key"}

	fkParentTableHeaders  = []string{"referenced_table_name", "referenced_table", "foreign_table_name", "parent_table", "table"}
	fkParentColumnHeaders = []string{"referenced_column_name", "referenced_column", "foreign_column_name", "parent_column", "to"}
	fkChildColumnHeaders  = []string{"column_name", "child_column", "from", "field"}
	fkChildTableHeaders   = []string{"table_name", "child_table"}
)

// headerIndex returns the index of the first header matching one of the given
// names, or -1. Header names are compared case-insensitively.
func headerIndex(headers []string, names ...string) int {
	for _, name := range names {
		for i, header := range headers {
			if strings.EqualFold(strings.TrimSpace(header), name) {
				return i
			}
		}
	}

	return -1
}

// cellAt returns the trimmed value at index, or "" when out of range.
func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}

	return strings.TrimSpace(row[index])
}

// isTruthy reports whether a driver value means "yes". Drivers spell booleans
// as 1/0, true/false or YES/NO depending on the backend.
func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "t":
		return true
	default:
		return false
	}
}

// parseSchemaColumns converts a driver's GetTableColumns result — a header row
// followed by one row per column — into SchemaColumns.
func parseSchemaColumns(rows [][]string) []SchemaColumn {
	if len(rows) < 2 {
		return nil
	}

	headers := rows[0]

	nameIdx := headerIndex(headers, columnNameHeaders...)
	if nameIdx == -1 {
		nameIdx = 0 // unknown layout: assume the name comes first
	}

	typeIdx := headerIndex(headers, columnTypeHeaders...)
	nullableIdx := headerIndex(headers, columnNullableHeaders...)
	notNullIdx := headerIndex(headers, columnNotNullHeaders...)
	keyIdx := headerIndex(headers, columnKeyHeaders...)
	pkIdx := headerIndex(headers, columnPKHeaders...)

	columns := make([]SchemaColumn, 0, len(rows)-1)

	for _, row := range rows[1:] {
		name := cellAt(row, nameIdx)
		if name == "" {
			continue
		}

		column := SchemaColumn{
			Name: name,
			Type: cellAt(row, typeIdx),
		}

		switch {
		case notNullIdx != -1:
			column.NotNull = isTruthy(cellAt(row, notNullIdx))
		case nullableIdx != -1:
			// "NO"/"0"/"false" all mean the column is NOT NULL.
			column.NotNull = !isTruthy(cellAt(row, nullableIdx))
		}

		switch {
		case keyIdx != -1:
			column.PrimaryKey = strings.EqualFold(cellAt(row, keyIdx), "PRI")
		case pkIdx != -1:
			column.PrimaryKey = isTruthy(cellAt(row, pkIdx))
		}

		columns = append(columns, column)
	}

	return columns
}

// parseForeignKeys converts a driver's GetForeignKeys result for table into
// child → parent edges. Drivers disagree on direction: MySQL returns the keys
// pointing AT the queried table (so the row names the child), while SQLite,
// Postgres and MSSQL return the keys held BY it (so the child is the table
// that was asked about).
func parseForeignKeys(table string, rows [][]string) []foreignKeyEdge {
	if len(rows) < 2 {
		return nil
	}

	headers := rows[0]

	parentTableIdx := headerIndex(headers, fkParentTableHeaders...)
	parentColumnIdx := headerIndex(headers, fkParentColumnHeaders...)
	childColumnIdx := headerIndex(headers, fkChildColumnHeaders...)
	childTableIdx := headerIndex(headers, fkChildTableHeaders...)

	if parentTableIdx == -1 || childColumnIdx == -1 {
		return nil
	}

	edges := make([]foreignKeyEdge, 0, len(rows)-1)

	for _, row := range rows[1:] {
		edge := foreignKeyEdge{
			ChildTable:   table,
			ChildColumn:  cellAt(row, childColumnIdx),
			ParentTable:  cellAt(row, parentTableIdx),
			ParentColumn: cellAt(row, parentColumnIdx),
		}

		if childTableIdx != -1 {
			if childTable := cellAt(row, childTableIdx); childTable != "" {
				edge.ChildTable = childTable
			}
		}

		if edge.ParentTable == "" || edge.ChildColumn == "" {
			continue
		}

		edges = append(edges, edge)
	}

	return edges
}

// ---------------------------------------------------------------------------
// Snapshot assembly
// ---------------------------------------------------------------------------

// bareTableName drops a schema qualifier, so "public.orders" matches "orders".
func bareTableName(name string) string {
	if index := strings.LastIndex(name, "."); index != -1 {
		return name[index+1:]
	}

	return name
}

// ApplyForeignKeys annotates the child column of every edge with the table and
// column it points at.
func (s *SchemaSnapshot) ApplyForeignKeys(edges []foreignKeyEdge) {
	for _, edge := range edges {
		tableIdx := s.indexOfTable(edge.ChildTable)
		if tableIdx == -1 {
			continue
		}

		columns := s.Tables[tableIdx].Columns

		for i := range columns {
			if !strings.EqualFold(columns[i].Name, edge.ChildColumn) {
				continue
			}

			reference := edge.ParentTable
			if edge.ParentColumn != "" {
				reference += "." + edge.ParentColumn
			}

			columns[i].References = reference

			break
		}
	}
}

// indexOfTable finds a table by exact name, falling back to its bare name.
func (s *SchemaSnapshot) indexOfTable(name string) int {
	for i, table := range s.Tables {
		if strings.EqualFold(table.Name, name) {
			return i
		}
	}

	bare := bareTableName(name)

	for i, table := range s.Tables {
		if strings.EqualFold(bareTableName(table.Name), bare) {
			return i
		}
	}

	return -1
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// Render writes the snapshot as a block of SQL comments, one line per table.
// The result always ends with a newline.
func (s *SchemaSnapshot) Render() string {
	var builder strings.Builder

	builder.WriteString(schemaContextStartMarker + "\n")
	builder.WriteString("-- Everything between these markers is discarded when you close the\n")
	builder.WriteString("-- editor. Ask your assistant for a query, leave the SQL below the\n")
	builder.WriteString("-- block, then save and quit to run it in lazysql.\n")
	builder.WriteString("--\n")

	if info := s.infoLine(); info != "" {
		builder.WriteString("-- " + info + "\n")
		builder.WriteString("--\n")
	}

	switch {
	case len(s.Tables) == 0:
		builder.WriteString("-- (no schema loaded yet — it may still be loading)\n")
	case !s.Complete:
		builder.WriteString("-- (the columns of some tables are still loading)\n")
	}

	for _, table := range s.Tables {
		builder.WriteString("-- " + renderTable(table) + "\n")
	}

	builder.WriteString(schemaContextEndMarker + "\n")

	return builder.String()
}

// infoLine describes the connection the query will run against.
func (s *SchemaSnapshot) infoLine() string {
	parts := make([]string, 0, 3)

	if s.Connection != "" {
		parts = append(parts, "connection: "+s.Connection)
	}

	if s.Provider != "" {
		parts = append(parts, "provider: "+s.Provider)
	}

	if s.Database != "" {
		parts = append(parts, "database: "+s.Database)
	}

	return strings.Join(parts, " | ")
}

// renderTable formats a table as "name(column type NOT NULL PK, ...)".
func renderTable(table SchemaTable) string {
	if len(table.Columns) == 0 {
		return table.Name
	}

	columns := make([]string, 0, len(table.Columns))

	for _, column := range table.Columns {
		parts := []string{column.Name}

		if column.Type != "" {
			parts = append(parts, column.Type)
		}

		if column.NotNull {
			parts = append(parts, "NOT NULL")
		}

		if column.PrimaryKey {
			parts = append(parts, "PK")
		}

		if column.References != "" {
			parts = append(parts, "-> "+column.References)
		}

		columns = append(columns, strings.Join(parts, " "))
	}

	return table.Name + "(" + strings.Join(columns, ", ") + ")"
}

// BuildExternalEditorContent returns the file the external editor opens on:
// the schema block, then whatever the SQL editor currently holds.
func BuildExternalEditorContent(snapshot *SchemaSnapshot, currentQuery string) string {
	if snapshot == nil {
		snapshot = &SchemaSnapshot{}
	}

	content := snapshot.Render() + "\n"

	if query := strings.TrimSpace(currentQuery); query != "" {
		content += query + "\n"
	}

	return content
}

// StripSchemaContext removes the schema block from text, leaving the query.
// Text without the start marker is returned untouched.
func StripSchemaContext(text string) string {
	lines := strings.Split(text, "\n")

	start := -1

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), schemaContextStartMarker) {
			start = i
			break
		}
	}

	if start == -1 {
		return text
	}

	end := -1

	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), schemaContextEndMarker) {
			end = i
			break
		}
	}

	if end == -1 {
		// The end marker was edited away: drop the marker line and the run of
		// comments below it rather than everything that follows.
		end = start

		for i := start + 1; i < len(lines); i++ {
			if !strings.HasPrefix(strings.TrimSpace(lines[i]), "--") {
				break
			}

			end = i
		}
	}

	remaining := append(append([]string{}, lines[:start]...), lines[end+1:]...)

	return strings.TrimSpace(strings.Join(remaining, "\n"))
}

// ---------------------------------------------------------------------------
// System schema / table filtering
// ---------------------------------------------------------------------------

// systemSchemas are the schemas a database engine keeps for itself. They add
// hundreds of tables that nobody writes queries against, so the schema context
// (and the autocomplete built alongside it) leaves them out.
var systemSchemas = map[string]bool{
	"pg_catalog":         true,
	"information_schema": true,
	"sys":                true, // MSSQL
}

// systemSchemaPrefixes cover the schemas Postgres numbers per session.
var systemSchemaPrefixes = []string{"pg_toast", "pg_temp"}

// isSystemSchema reports whether a schema belongs to the database engine.
func isSystemSchema(schema string) bool {
	lower := strings.ToLower(strings.TrimSpace(schema))

	if systemSchemas[lower] {
		return true
	}

	for _, prefix := range systemSchemaPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

// isSystemTable reports whether a table belongs to the database engine.
// SQLite keeps its bookkeeping in tables prefixed with "sqlite_".
func isSystemTable(table string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(table)), "sqlite_")
}

// skipSchema reports whether a schema should be left out of the schema context.
// MySQL and MSSQL report the database itself as the schema, so a database that
// happens to be named like a system schema is kept when it is the one open.
func skipSchema(schema, database string) bool {
	if strings.EqualFold(schema, database) {
		return false
	}

	return isSystemSchema(schema)
}
