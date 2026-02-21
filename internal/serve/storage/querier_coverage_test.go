package storage

import (
	"context"
	"testing"
)

func TestNewSkillQuerier(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sq := NewSkillQuerier(s)
	if sq == nil {
		t.Fatal("expected non-nil querier")
	}

	// Query with no skills should return nil.
	result, err := sq.QueryNearest(context.Background(), make([]float32, 8), "local")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil result for empty store")
	}
}
