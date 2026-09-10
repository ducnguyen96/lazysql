package components

import (
	"sync"

	"github.com/jorgerojas26/lazysql/drivers"
)

// schemaFetchWorkers bounds how many tables are described at once. Each table
// costs two round trips, so a serial loop over a remote database takes seconds;
// a small pool hides that latency without opening an unreasonable number of
// connections.
const schemaFetchWorkers = 8

// schemaTableRef names a table twice: as the driver wants it (schema-qualified
// for Postgres) and as the editor refers to it.
type schemaTableRef struct {
	BareName      string
	QualifiedName string
}

// schemaFetchResult is everything one pass over the tables produced.
type schemaFetchResult struct {
	// Tables is in the order the refs were given, minus the ones the driver
	// could not describe.
	Tables []SchemaTable
	// ForeignKeys holds every relationship found, still to be applied.
	ForeignKeys []foreignKeyEdge
	// Columns maps a bare table name to its column names, for autocomplete.
	Columns map[string][]string
}

// fetchSchema describes every table, running up to workers queries at a time.
// It performs no UI work, so it is safe to call from a background goroutine.
func fetchSchema(driver drivers.Driver, database string, refs []schemaTableRef, workers int) schemaFetchResult {
	result := schemaFetchResult{Columns: make(map[string][]string, len(refs))}

	if driver == nil || len(refs) == 0 {
		return result
	}

	if workers < 1 {
		workers = 1
	}

	// Results are collected by index so the output order does not depend on
	// which query happens to finish first.
	results := make([]describedTable, len(refs))

	var wg sync.WaitGroup

	queue := make(chan int)

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for index := range queue {
				results[index] = describeTable(driver, database, refs[index])
			}
		}()
	}

	for index := range refs {
		queue <- index
	}
	close(queue)

	wg.Wait()

	for _, res := range results {
		if !res.ok {
			continue
		}

		result.Tables = append(result.Tables, res.table)
		result.Columns[res.table.Name] = res.columnNames
		result.ForeignKeys = append(result.ForeignKeys, res.foreignKeys...)
	}

	return result
}

// describedTable is one table's worth of schema, or ok=false when the driver
// could not describe it.
type describedTable struct {
	table       SchemaTable
	columnNames []string
	foreignKeys []foreignKeyEdge
	ok          bool
}

// describeTable reads one table's columns and foreign keys. A table the driver
// cannot describe is left out of the schema.
func describeTable(driver drivers.Driver, database string, ref schemaTableRef) (res describedTable) {
	rows, err := driver.GetTableColumns(database, ref.QualifiedName)
	if err != nil || len(rows) < 2 {
		return res
	}

	// rows[0] = headers, rows[1:] = one row per column, name first.
	columnNames := make([]string, 0, len(rows)-1)

	for _, row := range rows[1:] {
		if len(row) > 0 && row[0] != "" {
			columnNames = append(columnNames, row[0])
		}
	}

	if len(columnNames) == 0 {
		return res
	}

	res.table = SchemaTable{Name: ref.BareName, Columns: parseSchemaColumns(rows)}
	res.columnNames = columnNames
	res.ok = true

	if fks, err := driver.GetForeignKeys(database, ref.QualifiedName); err == nil {
		res.foreignKeys = parseForeignKeys(ref.BareName, fks)
	}

	return res
}
