package components

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jorgerojas26/lazysql/models"
)

// sendKey feeds a key event through the primitive the way the tview event
// loop does, so input captures and focus handling are exercised.
func sendKey(p tview.Primitive, event *tcell.EventKey) {
	p.InputHandler()(event, func(next tview.Primitive) {
		App.SetFocus(next)
	})
}

func newTestConnectionSelection(t *testing.T) *ConnectionSelection {
	t.Helper()

	previousPages := mainPages
	t.Cleanup(func() { mainPages = previousPages })
	mainPages = tview.NewPages()

	connectionPages := NewConnectionPages()
	cs := NewConnectionSelection(NewConnectionForm(connectionPages), connectionPages)

	connectionsTable.SetConnections(testConnections())

	return cs
}

func TestSlashFocusesTheSearchInput(t *testing.T) {
	cs := newTestConnectionSelection(t)
	App.SetFocus(connectionsTable)

	sendKey(cs, tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))

	if !cs.SearchInput.HasFocus() {
		t.Fatal("pressing / did not focus the search input")
	}
}

func TestTypingInTheSearchInputFiltersInsteadOfRunningCommands(t *testing.T) {
	cs := newTestConnectionSelection(t)
	App.SetFocus(cs.SearchInput)

	for _, r := range "prod" {
		sendKey(cs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}

	if got := cs.SearchInput.GetText(); got != "prod" {
		t.Fatalf("search input text = %q, want %q", got, "prod")
	}

	if got := connectionsTable.GetRowCount(); got != 2 {
		t.Fatalf("filtered row count = %d, want 2", got)
	}
}

func TestEscapeClearsTheSearchAndReturnsFocusToTheTable(t *testing.T) {
	cs := newTestConnectionSelection(t)
	App.SetFocus(cs.SearchInput)

	for _, r := range "prod" {
		sendKey(cs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	sendKey(cs, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if got := cs.SearchInput.GetText(); got != "" {
		t.Fatalf("search input text after Escape = %q, want empty", got)
	}

	if got := connectionsTable.GetRowCount(); got != len(testConnections()) {
		t.Fatalf("row count after Escape = %d, want %d", got, len(testConnections()))
	}

	if !connectionsTable.HasFocus() {
		t.Fatal("Escape did not return focus to the connections table")
	}
}

func TestEnterKeepsTheFilterAndReturnsFocusToTheTable(t *testing.T) {
	cs := newTestConnectionSelection(t)
	App.SetFocus(cs.SearchInput)

	for _, r := range "prod" {
		sendKey(cs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	sendKey(cs, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if got := connectionsTable.GetRowCount(); got != 2 {
		t.Fatalf("row count after Enter = %d, want 2", got)
	}

	if !connectionsTable.HasFocus() {
		t.Fatal("Enter did not return focus to the connections table")
	}
}

func TestSelectedConnectionFollowsTheFilteredRows(t *testing.T) {
	cs := newTestConnectionSelection(t)
	App.SetFocus(cs.SearchInput)

	for _, r := range "prod" {
		sendKey(cs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	sendKey(cs, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	sendKey(cs, tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))

	want := models.Connection{Name: "PROD-mysql", ReadOnly: true}
	index := connectionsTable.GetSelectedConnectionIndex()

	if index < 0 {
		t.Fatal("no connection selected after filtering")
	}

	if got := connectionsTable.GetConnections()[index]; got.Name != want.Name {
		t.Fatalf("selected connection = %q, want %q", got.Name, want.Name)
	}
}
