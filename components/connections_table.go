package components

import (
	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/app"
	"github.com/jorgerojas26/lazysql/models"
)

type ConnectionsTable struct {
	*tview.Table
	Wrapper       *tview.Flex
	errorTextView *tview.TextView
	error         string
	filter        string
	connections   []models.Connection
	// visible maps a table row to its index in connections.
	visible []int
}

var connectionsTable *ConnectionsTable

func NewConnectionsTable() *ConnectionsTable {
	wrapper := tview.NewFlex()

	errorTextView := tview.NewTextView()
	errorTextView.SetTextStyle(tcell.StyleDefault.Foreground(tcell.ColorRed))

	table := &ConnectionsTable{
		Table:         tview.NewTable().SetSelectable(true, false),
		Wrapper:       wrapper,
		errorTextView: errorTextView,
	}

	table.SetOffset(5, 0)
	table.SetSelectedStyle(tcell.StyleDefault.Foreground(app.Styles.SecondaryTextColor).Background(tview.Styles.PrimitiveBackgroundColor))

	wrapper.AddItem(table, 0, 1, true)
	table.SetConnections(app.App.Connections())

	connectionsTable = table

	return connectionsTable
}

// filterConnectionIndexes returns the indexes of the connections whose name
// fuzzy matches query, in their original order. An empty query matches all.
func filterConnectionIndexes(connections []models.Connection, query string) []int {
	indexes := make([]int, 0, len(connections))

	for i, connection := range connections {
		if fuzzy.MatchFold(query, connection.Name) {
			indexes = append(indexes, i)
		}
	}

	return indexes
}

func connectionDisplayName(connection models.Connection) string {
	displayName := connection.Name

	if connection.ReadOnly {
		displayName = "[lightblue]READ[-] " + displayName
	}

	if mainPages != nil && mainPages.HasPage(connection.Name) {
		displayName = "[green]* " + displayName
	}

	return displayName
}

// render redraws the rows that pass the current filter.
func (ct *ConnectionsTable) render() {
	ct.Clear()

	ct.visible = filterConnectionIndexes(ct.connections, ct.filter)

	for row, index := range ct.visible {
		ct.SetCellSimple(row, 0, connectionDisplayName(ct.connections[index]))
	}

	ct.Select(0, 0)
}

func (ct *ConnectionsTable) AddConnection(connection models.Connection) {
	ct.connections = append(ct.connections, connection)
	ct.render()
}

func (ct *ConnectionsTable) GetConnections() []models.Connection {
	return ct.connections
}

// GetSelectedConnectionIndex returns the index into GetConnections of the
// highlighted row, or -1 when no connection is selected.
func (ct *ConnectionsTable) GetSelectedConnectionIndex() int {
	row, _ := ct.GetSelection()

	if row < 0 || row >= len(ct.visible) {
		return -1
	}

	return ct.visible[row]
}

// Refresh redraws the table, keeping the highlighted row where it is.
func (ct *ConnectionsTable) Refresh() {
	row, column := ct.GetSelection()

	ct.render()

	if row < ct.GetRowCount() {
		ct.Select(row, column)
	}
}

// Filter narrows the table down to the connections matching query.
func (ct *ConnectionsTable) Filter(query string) {
	ct.filter = query
	ct.render()
}

// FilteredCount returns how many connections the current filter shows.
func (ct *ConnectionsTable) FilteredCount() int {
	return len(ct.visible)
}

func (ct *ConnectionsTable) GetError() string {
	return ct.error
}

func (ct *ConnectionsTable) SetConnections(connections []models.Connection) {
	ct.connections = append([]models.Connection(nil), connections...)
	ct.render()
	App.ForceDraw()
}

func (ct *ConnectionsTable) SetError(err error) {
	ct.error = err.Error()
	ct.errorTextView.SetText(ct.error)
}
