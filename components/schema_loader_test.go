package components

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jorgerojas26/lazysql/drivers"
)

// fetchMock serves canned column and foreign key rows, and can hold every call
// at a barrier so a test can observe how many run at once.
type fetchMock struct {
	*schemaProgrammingMock

	columns map[string][][]string
	fks     map[string][][]string

	barrier chan struct{} // when non-nil,each call waits here

	mu       sync.Mutex
	inFlight int
	maxSeen  int
}

func (m *fetchMock) GetTableColumns(_, table string) ([][]string, error) {
	m.enter()
	defer m.leave()

	rows, ok := m.columns[table]
	if !ok {
		return nil, errors.New("no such table")
	}

	return rows, nil
}

func (m *fetchMock) GetForeignKeys(_, table string) ([][]string, error) {
	rows, ok := m.fks[table]
	if !ok {
		return nil, errors.New("no such table")
	}

	return rows, nil
}

func (m *fetchMock) enter() {
	m.mu.Lock()
	m.inFlight++
	if m.inFlight > m.maxSeen {
		m.maxSeen = m.inFlight
	}
	m.mu.Unlock()

	if m.barrier != nil {
		<-m.barrier
	}
}

func (m *fetchMock) leave() {
	m.mu.Lock()
	m.inFlight--
	m.mu.Unlock()
}

func (m *fetchMock) concurrencyPeak() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.maxSeen
}

func columnRows(names ...string) [][]string {
	rows := [][]string{{"Field", "Type", "Null", "Key"}}
	for _, name := range names {
		rows = append(rows, []string{name, "text", "YES", ""})
	}

	return rows
}

func newFetchMock() *fetchMock {
	return &fetchMock{
		schemaProgrammingMock: &schemaProgrammingMock{},
		columns: map[string][][]string{
			"users":  columnRows("id", "email"),
			"orders": columnRows("id", "user_id"),
			"notes":  columnRows("id", "body"),
		},
		fks: map[string][][]string{
			"orders": {
				{"TABLE_NAME", "COLUMN_NAME", "REFERENCED_COLUMN_NAME", "REFERENCED_TABLE_NAME"},
				{"orders", "user_id", "id", "users"},
			},
		},
	}
}

func refs(names ...string) []schemaTableRef {
	list := make([]schemaTableRef, 0, len(names))
	for _, name := range names {
		list = append(list, schemaTableRef{BareName: name, QualifiedName: name})
	}

	return list
}

func TestFetchSchema_KeepsTheRequestedOrder(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", refs("users", "orders", "notes"), 4)

	if len(result.Tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(result.Tables))
	}
	for i, want := range []string{"users", "orders", "notes"} {
		if result.Tables[i].Name != want {
			t.Errorf("table %d = %s, want %s", i, result.Tables[i].Name, want)
		}
	}
}

func TestFetchSchema_ReadsColumns(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", refs("users"), 4)

	if len(result.Tables) != 1 || len(result.Tables[0].Columns) != 2 {
		t.Fatalf("expected 2 columns for users, got %+v", result.Tables)
	}
	if names := result.Columns["users"]; len(names) != 2 || names[0] != "id" || names[1] != "email" {
		t.Errorf("expected autocomplete columns id,email, got %v", names)
	}
}

func TestFetchSchema_CollectsForeignKeys(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", refs("users", "orders"), 4)

	if len(result.ForeignKeys) != 1 {
		t.Fatalf("expected 1 foreign key, got %+v", result.ForeignKeys)
	}
	want := foreignKeyEdge{ChildTable: "orders", ChildColumn: "user_id", ParentTable: "users", ParentColumn: "id"}
	if result.ForeignKeys[0] != want {
		t.Errorf("expected %+v, got %+v", want, result.ForeignKeys[0])
	}
}

func TestFetchSchema_SkipsTablesTheDriverCannotDescribe(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", refs("users", "ghost"), 4)

	if len(result.Tables) != 1 || result.Tables[0].Name != "users" {
		t.Errorf("expected only users, got %+v", result.Tables)
	}
}

func TestFetchSchema_QueriesTablesConcurrently(t *testing.T) {
	mock := newFetchMock()
	mock.barrier = make(chan struct{})

	done := make(chan schemaFetchResult, 1)
	go func() {
		done <- fetchSchema(mock, "shopdb", refs("users", "orders", "notes"), 3)
	}()

	// Release the three calls only once all three are in flight; a serial loader
	// would never get past the first and the test would time out.
	deadline := time.After(2 * time.Second)
	for mock.concurrencyPeak() < 3 {
		select {
		case <-deadline:
			t.Fatalf("expected 3 concurrent queries, peaked at %d", mock.concurrencyPeak())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(mock.barrier)

	select {
	case result := <-done:
		if len(result.Tables) != 3 {
			t.Errorf("expected 3 tables, got %d", len(result.Tables))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fetchSchema did not finish")
	}
}

func TestFetchSchema_NonPositiveWorkerCountStillWorks(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", refs("users", "orders"), 0)

	if len(result.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(result.Tables))
	}
}

func TestFetchSchema_NoTables(t *testing.T) {
	result := fetchSchema(newFetchMock(), "shopdb", nil, 4)

	if len(result.Tables) != 0 || len(result.ForeignKeys) != 0 {
		t.Errorf("expected an empty result, got %+v", result)
	}
}

var _ drivers.Driver = (*fetchMock)(nil)
