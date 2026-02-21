package metrics

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewCollector(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := NewCollector(db)
	if c.db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestCollect_Empty(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := NewCollector(db)
	m, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
}
