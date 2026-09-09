package components

import (
	"testing"

	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/models"
)

func testConnections() []models.Connection {
	return []models.Connection{
		{Name: "local-sqlite"},
		{Name: "staging-mysql"},
		{Name: "prod-postgres"},
		{Name: "PROD-mysql", ReadOnly: true},
		{Name: "dev-mssql"},
	}
}

func newTestConnectionsTable(t *testing.T) *ConnectionsTable {
	t.Helper()

	return &ConnectionsTable{
		Table:   tview.NewTable().SetSelectable(true, false),
		Wrapper: tview.NewFlex(),
	}
}

func TestFilterConnectionIndexes(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{
			name:  "empty query matches every connection",
			query: "",
			want:  []int{0, 1, 2, 3, 4},
		},
		{
			name:  "matches are case insensitive",
			query: "prod",
			want:  []int{2, 3},
		},
		{
			name:  "query casing does not matter either",
			query: "PROD",
			want:  []int{2, 3},
		},
		{
			name:  "matches non contiguous characters",
			query: "prpg",
			want:  []int{2},
		},
		{
			name:  "keeps the original connection order",
			query: "m",
			want:  []int{1, 3, 4},
		},
		{
			name:  "no matches yields no indexes",
			query: "nothing-here",
			want:  []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterConnectionIndexes(testConnections(), tt.query)

			if len(got) != len(tt.want) {
				t.Fatalf("filterConnectionIndexes(%q) = %v, want %v", tt.query, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("filterConnectionIndexes(%q) = %v, want %v", tt.query, got, tt.want)
				}
			}
		})
	}
}

func TestFilterConnectionIndexesIgnoresReadOnlyPrefix(t *testing.T) {
	connections := []models.Connection{{Name: "mydb", ReadOnly: true}}

	if got := filterConnectionIndexes(connections, "read"); len(got) != 0 {
		t.Fatalf("filterConnectionIndexes matched the READ display prefix: %v", got)
	}
}

func TestFilterShowsOnlyMatchingRows(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Filter("prod")

	if got := ct.GetRowCount(); got != 2 {
		t.Fatalf("row count after filtering = %d, want 2", got)
	}

	if got := ct.GetCell(0, 0).Text; got != "prod-postgres" {
		t.Fatalf("first filtered row = %q, want %q", got, "prod-postgres")
	}
}

func TestGetSelectedConnectionIndexMapsFilteredRowToConnection(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Filter("prod")
	ct.Select(1, 0)

	if got := ct.GetSelectedConnectionIndex(); got != 3 {
		t.Fatalf("GetSelectedConnectionIndex() = %d, want 3", got)
	}
}

func TestGetSelectedConnectionIndexWithoutFilter(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Select(2, 0)

	if got := ct.GetSelectedConnectionIndex(); got != 2 {
		t.Fatalf("GetSelectedConnectionIndex() = %d, want 2", got)
	}
}

func TestGetSelectedConnectionIndexWhenNothingMatches(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Filter("nothing-here")

	if got := ct.GetSelectedConnectionIndex(); got != -1 {
		t.Fatalf("GetSelectedConnectionIndex() = %d, want -1", got)
	}
}

func TestClearingTheFilterRestoresEveryRow(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Filter("prod")
	ct.Filter("")

	if got := ct.GetRowCount(); got != len(testConnections()) {
		t.Fatalf("row count after clearing the filter = %d, want %d", got, len(testConnections()))
	}
}

func TestSetConnectionsKeepsTheActiveFilter(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())
	ct.Filter("prod")

	ct.SetConnections(testConnections()[:3])

	if got := ct.GetRowCount(); got != 1 {
		t.Fatalf("row count after replacing connections = %d, want 1", got)
	}

	ct.Select(0, 0)

	if got := ct.GetSelectedConnectionIndex(); got != 2 {
		t.Fatalf("GetSelectedConnectionIndex() = %d, want 2", got)
	}
}

func TestConnectedConnectionIsMarked(t *testing.T) {
	previousPages := mainPages
	t.Cleanup(func() { mainPages = previousPages })

	mainPages = tview.NewPages()
	mainPages.AddPage("prod-postgres", tview.NewBox(), true, false)

	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	if got := ct.GetCell(2, 0).Text; got != "[green]* prod-postgres" {
		t.Fatalf("connected row = %q, want %q", got, "[green]* prod-postgres")
	}

	if got := ct.GetCell(0, 0).Text; got != "local-sqlite" {
		t.Fatalf("disconnected row = %q, want %q", got, "local-sqlite")
	}
}

func TestConnectedMarkerSurvivesFiltering(t *testing.T) {
	previousPages := mainPages
	t.Cleanup(func() { mainPages = previousPages })

	mainPages = tview.NewPages()
	mainPages.AddPage("prod-postgres", tview.NewBox(), true, false)

	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())

	ct.Filter("postgres")

	if got := ct.GetCell(0, 0).Text; got != "[green]* prod-postgres" {
		t.Fatalf("connected row after filtering = %q, want %q", got, "[green]* prod-postgres")
	}
}

func TestRefreshKeepsTheSelectedRow(t *testing.T) {
	ct := newTestConnectionsTable(t)
	ct.SetConnections(testConnections())
	ct.Select(3, 0)

	ct.Refresh()

	if got, _ := ct.GetSelection(); got != 3 {
		t.Fatalf("selected row after Refresh() = %d, want 3", got)
	}
}
