package components

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseSchemaColumns — driver result rows → SchemaColumn
// ---------------------------------------------------------------------------

func TestParseSchemaColumns_MySQLHeaders(t *testing.T) {
	rows := [][]string{
		{"Field", "Type", "Collation", "Null", "Key", "Default", "Extra", "Privileges", "Comment"},
		{"id", "int unsigned", "NULL", "NO", "PRI", "NULL", "auto_increment", "select", ""},
		{"email", "varchar(255)", "utf8mb4", "NO", "UNI", "NULL", "", "select", ""},
		{"deleted_at", "timestamp", "NULL", "YES", "", "NULL", "", "select", ""},
	}

	cols := parseSchemaColumns(rows)

	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	if cols[0].Name != "id" || cols[0].Type != "int unsigned" {
		t.Errorf("expected id/int unsigned, got %s/%s", cols[0].Name, cols[0].Type)
	}
	if !cols[0].PrimaryKey {
		t.Error("expected id to be a primary key")
	}
	if !cols[1].NotNull {
		t.Error("expected email to be NOT NULL")
	}
	if cols[2].NotNull {
		t.Error("expected deleted_at to be nullable")
	}
	if cols[1].PrimaryKey {
		t.Error("expected UNI key to not count as a primary key")
	}
}

func TestParseSchemaColumns_PostgresHeaders(t *testing.T) {
	rows := [][]string{
		{"column_name", "data_type", "is_nullable", "column_default", "comment"},
		{"id", "integer", "NO", "nextval('users_id_seq')", ""},
		{"bio", "text", "YES", "NULL", ""},
	}

	cols := parseSchemaColumns(rows)

	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	if cols[0].Name != "id" || cols[0].Type != "integer" || !cols[0].NotNull {
		t.Errorf("unexpected first column: %+v", cols[0])
	}
	if cols[1].NotNull {
		t.Error("expected bio to be nullable")
	}
}

func TestParseSchemaColumns_SQLiteHeaders(t *testing.T) {
	// SQLite's PRAGMA table_info, with the leading cid column already stripped
	// by the driver.
	rows := [][]string{
		{"name", "type", "notnull", "dflt_value", "pk"},
		{"id", "INTEGER", "1", "NULL", "1"},
		{"title", "TEXT", "0", "NULL", "0"},
	}

	cols := parseSchemaColumns(rows)

	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	if !cols[0].NotNull || !cols[0].PrimaryKey {
		t.Errorf("expected id to be NOT NULL and PK, got %+v", cols[0])
	}
	if cols[1].NotNull || cols[1].PrimaryKey {
		t.Errorf("expected title to be nullable non-PK, got %+v", cols[1])
	}
}

func TestParseSchemaColumns_UnknownHeadersFallBackToFirstColumn(t *testing.T) {
	rows := [][]string{
		{"weird", "stuff"},
		{"id", "whatever"},
	}

	cols := parseSchemaColumns(rows)

	if len(cols) != 1 || cols[0].Name != "id" {
		t.Fatalf("expected fallback to first column as name, got %+v", cols)
	}
	if cols[0].Type != "" {
		t.Errorf("expected no type for unknown headers, got %q", cols[0].Type)
	}
}

func TestParseSchemaColumns_HeaderOnlyReturnsNothing(t *testing.T) {
	rows := [][]string{{"Field", "Type"}}

	if cols := parseSchemaColumns(rows); len(cols) != 0 {
		t.Fatalf("expected no columns, got %+v", cols)
	}
}

// ---------------------------------------------------------------------------
// parseForeignKeys — driver result rows → edges
// ---------------------------------------------------------------------------

func TestParseForeignKeys_MySQLListsReferencingTables(t *testing.T) {
	// MySQL returns the keys pointing AT the queried table, so the child table
	// comes from the row itself.
	rows := [][]string{
		{"TABLE_NAME", "COLUMN_NAME", "CONSTRAINT_NAME", "REFERENCED_COLUMN_NAME", "REFERENCED_TABLE_NAME"},
		{"orders", "user_id", "fk_orders_user", "id", "users"},
	}

	edges := parseForeignKeys("users", rows)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	want := foreignKeyEdge{ChildTable: "orders", ChildColumn: "user_id", ParentTable: "users", ParentColumn: "id"}
	if edges[0] != want {
		t.Errorf("expected %+v, got %+v", want, edges[0])
	}
}

func TestParseForeignKeys_SQLiteListsOutgoingKeys(t *testing.T) {
	// PRAGMA foreign_key_list returns the keys held BY the queried table, and
	// its "table" column is the referenced table.
	rows := [][]string{
		{"id", "seq", "table", "from", "to", "on_update", "on_delete", "match"},
		{"0", "0", "users", "user_id", "id", "NO ACTION", "NO ACTION", "NONE"},
	}

	edges := parseForeignKeys("orders", rows)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	want := foreignKeyEdge{ChildTable: "orders", ChildColumn: "user_id", ParentTable: "users", ParentColumn: "id"}
	if edges[0] != want {
		t.Errorf("expected %+v, got %+v", want, edges[0])
	}
}

func TestParseForeignKeys_PostgresHeaders(t *testing.T) {
	rows := [][]string{
		{"constraint_name", "column_name", "foreign_table_schema", "foreign_table_name", "foreign_column_name"},
		{"orders_user_id_fkey", "user_id", "public", "users", "id"},
	}

	edges := parseForeignKeys("public.orders", rows)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	want := foreignKeyEdge{ChildTable: "public.orders", ChildColumn: "user_id", ParentTable: "users", ParentColumn: "id"}
	if edges[0] != want {
		t.Errorf("expected %+v, got %+v", want, edges[0])
	}
}

func TestParseForeignKeys_MSSQLHeaders(t *testing.T) {
	rows := [][]string{
		{"constraint_name", "column_name", "current_database", "referenced_table", "referenced_column", "delete_rule", "update_rule"},
		{"FK_orders_users", "user_id", "shopdb", "dbo.users", "id", "NO_ACTION", "NO_ACTION"},
	}

	edges := parseForeignKeys("orders", rows)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	want := foreignKeyEdge{ChildTable: "orders", ChildColumn: "user_id", ParentTable: "dbo.users", ParentColumn: "id"}
	if edges[0] != want {
		t.Errorf("expected %+v, got %+v", want, edges[0])
	}
}

func TestParseForeignKeys_IncompleteRowIsSkipped(t *testing.T) {
	rows := [][]string{
		{"TABLE_NAME", "COLUMN_NAME", "REFERENCED_COLUMN_NAME", "REFERENCED_TABLE_NAME"},
		{"orders", "user_id", "id", ""},
	}

	if edges := parseForeignKeys("users", rows); len(edges) != 0 {
		t.Fatalf("expected no edges for a row without a referenced table, got %+v", edges)
	}
}

// ---------------------------------------------------------------------------
// SchemaSnapshot.ApplyForeignKeys
// ---------------------------------------------------------------------------

func TestApplyForeignKeys_AnnotatesChildColumn(t *testing.T) {
	snapshot := &SchemaSnapshot{
		Tables: []SchemaTable{
			{Name: "users", Columns: []SchemaColumn{{Name: "id"}}},
			{Name: "orders", Columns: []SchemaColumn{{Name: "id"}, {Name: "user_id"}}},
		},
	}

	snapshot.ApplyForeignKeys([]foreignKeyEdge{
		{ChildTable: "orders", ChildColumn: "user_id", ParentTable: "users", ParentColumn: "id"},
	})

	if got := snapshot.Tables[1].Columns[1].References; got != "users.id" {
		t.Errorf("expected orders.user_id to reference users.id, got %q", got)
	}
	if got := snapshot.Tables[0].Columns[0].References; got != "" {
		t.Errorf("expected users.id to reference nothing, got %q", got)
	}
}

func TestApplyForeignKeys_MatchesQualifiedTableNames(t *testing.T) {
	snapshot := &SchemaSnapshot{
		Tables: []SchemaTable{
			{Name: "orders", Columns: []SchemaColumn{{Name: "user_id"}}},
		},
	}

	snapshot.ApplyForeignKeys([]foreignKeyEdge{
		{ChildTable: "public.orders", ChildColumn: "user_id", ParentTable: "public.users", ParentColumn: "id"},
	})

	if got := snapshot.Tables[0].Columns[0].References; got != "public.users.id" {
		t.Errorf("expected qualified child table to match bare table name, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// SchemaSnapshot.Render
// ---------------------------------------------------------------------------

func newTestSnapshot() *SchemaSnapshot {
	return &SchemaSnapshot{
		Connection: "local-mysql",
		Provider:   "mysql",
		Database:   "shopdb",
		Tables: []SchemaTable{
			{Name: "users", Columns: []SchemaColumn{
				{Name: "id", Type: "int", NotNull: true, PrimaryKey: true},
				{Name: "email", Type: "varchar(255)", NotNull: true},
				{Name: "deleted_at", Type: "timestamp"},
			}},
			{Name: "orders", Columns: []SchemaColumn{
				{Name: "id", Type: "int", NotNull: true, PrimaryKey: true},
				{Name: "user_id", Type: "int", NotNull: true, References: "users.id"},
			}},
		},
	}
}

func TestRender_EveryLineIsASQLComment(t *testing.T) {
	for _, line := range strings.Split(strings.TrimRight(newTestSnapshot().Render(), "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "--") {
			t.Fatalf("expected every rendered line to be a SQL comment, got %q", line)
		}
	}
}

func TestRender_IncludesConnectionInfo(t *testing.T) {
	rendered := newTestSnapshot().Render()

	for _, want := range []string{"local-mysql", "mysql", "shopdb"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("expected rendered context to mention %q\n%s", want, rendered)
		}
	}
}

func TestRender_DescribesTableColumns(t *testing.T) {
	rendered := newTestSnapshot().Render()

	want := "-- users(id int NOT NULL PK, email varchar(255) NOT NULL, deleted_at timestamp)"
	if !strings.Contains(rendered, want) {
		t.Errorf("expected line %q\n%s", want, rendered)
	}
}

func TestRender_ShowsForeignKeyArrow(t *testing.T) {
	rendered := newTestSnapshot().Render()

	want := "user_id int NOT NULL -> users.id"
	if !strings.Contains(rendered, want) {
		t.Errorf("expected foreign key arrow %q\n%s", want, rendered)
	}
}

func TestRender_EmptySnapshotSaysSchemaIsNotLoaded(t *testing.T) {
	rendered := (&SchemaSnapshot{Provider: "mysql", Database: "shopdb"}).Render()

	if !strings.Contains(rendered, "no schema loaded") {
		t.Errorf("expected a note about the missing schema\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// StripSchemaContext
// ---------------------------------------------------------------------------

func TestStripSchemaContext_RemovesRenderedBlock(t *testing.T) {
	text := newTestSnapshot().Render() + "\nSELECT * FROM users;\n"

	if got := StripSchemaContext(text); got != "SELECT * FROM users;" {
		t.Errorf("expected only the query to survive, got %q", got)
	}
}

func TestStripSchemaContext_KeepsSQLAboveTheBlock(t *testing.T) {
	text := "SELECT 1;\n" + newTestSnapshot().Render() + "\nSELECT 2;\n"

	want := "SELECT 1;\n\nSELECT 2;"
	if got := StripSchemaContext(text); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripSchemaContext_LeavesTextWithoutMarkersAlone(t *testing.T) {
	text := "-- a normal comment\nSELECT * FROM users;"

	if got := StripSchemaContext(text); got != text {
		t.Errorf("expected the text to be untouched, got %q", got)
	}
}

func TestStripSchemaContext_DropsCommentRunWhenEndMarkerIsMissing(t *testing.T) {
	text := strings.Join([]string{
		schemaContextStartMarker,
		"-- connection: local-mysql",
		"-- users(id int PK)",
		"",
		"SELECT * FROM users;",
	}, "\n")

	if got := StripSchemaContext(text); got != "SELECT * FROM users;" {
		t.Errorf("expected the orphaned block to be dropped, got %q", got)
	}
}

func TestStripSchemaContext_EmptyWhenNothingButTheBlockRemains(t *testing.T) {
	if got := StripSchemaContext(newTestSnapshot().Render()); got != "" {
		t.Errorf("expected an empty result, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// BuildExternalEditorContent
// ---------------------------------------------------------------------------

func TestBuildExternalEditorContent_PutsCurrentQueryBelowTheBlock(t *testing.T) {
	content := BuildExternalEditorContent(newTestSnapshot(), "SELECT * FROM users;")

	blockEnd := strings.Index(content, schemaContextEndMarker)
	query := strings.Index(content, "SELECT * FROM users;")
	if blockEnd == -1 || query == -1 {
		t.Fatalf("expected both the block and the query in:\n%s", content)
	}
	if query < blockEnd {
		t.Errorf("expected the query below the schema block:\n%s", content)
	}
}

func TestBuildExternalEditorContent_RoundTripsThroughStrip(t *testing.T) {
	query := "SELECT u.email FROM users u JOIN orders o ON o.user_id = u.id;"

	if got := StripSchemaContext(BuildExternalEditorContent(newTestSnapshot(), query)); got != query {
		t.Errorf("expected the query to round trip, got %q", got)
	}
}

func TestRender_IncompleteSnapshotSaysColumnsAreStillLoading(t *testing.T) {
	snapshot := &SchemaSnapshot{
		Database: "shopdb",
		Tables:   []SchemaTable{{Name: "users"}},
	}

	rendered := snapshot.Render()

	if !strings.Contains(rendered, "still loading") {
		t.Errorf("expected a note that the schema is incomplete\n%s", rendered)
	}
	if !strings.Contains(rendered, "-- users") {
		t.Errorf("expected the known table names to be listed\n%s", rendered)
	}
}

func TestRender_CompleteSnapshotHasNoLoadingNote(t *testing.T) {
	snapshot := newTestSnapshot()
	snapshot.Complete = true

	if rendered := snapshot.Render(); strings.Contains(rendered, "still loading") {
		t.Errorf("expected no loading note on a complete snapshot\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// System schema / table filtering
// ---------------------------------------------------------------------------

func TestIsSystemSchema(t *testing.T) {
	system := []string{
		"pg_catalog", "PG_CATALOG", "information_schema", "INFORMATION_SCHEMA",
		"pg_toast", "pg_toast_temp_1", "pg_temp_1", "sys",
	}
	for _, name := range system {
		if !isSystemSchema(name) {
			t.Errorf("expected %q to be a system schema", name)
		}
	}

	userDefined := []string{"public", "pgboss", "shopdb", "audit", "systems", "pgcrypto"}
	for _, name := range userDefined {
		if isSystemSchema(name) {
			t.Errorf("expected %q to be kept", name)
		}
	}
}

func TestIsSystemTable(t *testing.T) {
	if !isSystemTable("sqlite_sequence") {
		t.Error("expected sqlite_sequence to be a system table")
	}
	if isSystemTable("users") || isSystemTable("sqlitedb_rows") {
		t.Error("expected user tables to be kept")
	}
}

func TestSkipSchema_KeepsTheDatabaseYouOpened(t *testing.T) {
	// MySQL and MSSQL report the database itself as the schema, so someone who
	// deliberately opened the "sys" database must still see its tables.
	if skipSchema("sys", "sys") {
		t.Error("expected the opened database to be kept even when it is named like a system schema")
	}
	if !skipSchema("sys", "shopdb") {
		t.Error("expected the sys schema of another database to be skipped")
	}
	if skipSchema("pgboss", "shopdb") {
		t.Error("expected a tool schema to be kept")
	}
}
