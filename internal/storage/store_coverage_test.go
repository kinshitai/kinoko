package storage

import (
	"testing"
)

func TestNewSQLiteStore_DefaultEmbeddingModel(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.embeddingModel != "text-embedding-3-small" {
		t.Fatalf("expected default embedding model, got %q", s.embeddingModel)
	}
}

func TestNewSQLiteStore_CustomEmbeddingModel(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "custom-model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.embeddingModel != "custom-model" {
		t.Fatalf("expected custom-model, got %q", s.embeddingModel)
	}
}

func TestNewSQLiteStore_DBAccessible(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	db := s.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}

	// Verify tables were created.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='skills'").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("skills table not created")
	}
}
