package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func setupNoveltyTest(t *testing.T) (*NoveltyChecker, *storage.SQLiteStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath, "mock-model")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	engine := embedding.NewMockEngine(384)
	nc := NewNoveltyChecker(NoveltyCheckerConfig{
		Engine:    engine,
		Store:     store,
		Threshold: 0.85,
	})
	return nc, store
}

func TestNoveltyHandler_Success_NoSkills(t *testing.T) {
	nc, _ := setupNoveltyTest(t)

	body, _ := json.Marshal(NoveltyRequest{Content: "how to deploy to kubernetes"})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp NoveltyResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Novel {
		t.Error("expected novel=true when no skills exist")
	}
	if resp.Score != 0 {
		t.Errorf("score = %f, want 0", resp.Score)
	}
}

func TestNoveltyHandler_EmptyContent(t *testing.T) {
	nc, _ := setupNoveltyTest(t)

	body, _ := json.Marshal(NoveltyRequest{Content: ""})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNoveltyHandler_InvalidJSON(t *testing.T) {
	nc, _ := setupNoveltyTest(t)

	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestNoveltyHandler_NoEngine(t *testing.T) {
	nc := NewNoveltyChecker(NoveltyCheckerConfig{
		Store: nil,
	})
	nc.engine = nil

	body, _ := json.Marshal(NoveltyRequest{Content: "test"})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestNoveltyHandler_WithThresholdOverride(t *testing.T) {
	nc, _ := setupNoveltyTest(t)

	body, _ := json.Marshal(NoveltyRequest{Content: "test content", Threshold: 0.5})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestNoveltyChecker_DefaultThreshold(t *testing.T) {
	nc := NewNoveltyChecker(NoveltyCheckerConfig{})
	if nc.threshold != 0.85 {
		t.Errorf("threshold = %f, want 0.85", nc.threshold)
	}

	nc2 := NewNoveltyChecker(NoveltyCheckerConfig{Threshold: 0.9})
	if nc2.threshold != 0.9 {
		t.Errorf("threshold = %f, want 0.9", nc2.threshold)
	}
}

func TestNoveltyHandler_WithTracing(t *testing.T) {
	traceDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath, "mock-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	nc := NewNoveltyChecker(NoveltyCheckerConfig{
		Engine:    embedding.NewMockEngine(384),
		Store:     store,
		Threshold: 0.85,
		TraceDir:  traceDir,
	})

	body, _ := json.Marshal(NoveltyRequest{Content: "some content to check"})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	// Check trace file was created
	entries, err := os.ReadDir(filepath.Join(traceDir, "novelty"))
	if err != nil {
		t.Fatalf("read trace dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected trace file to be written")
	}
}

func TestTruncateForTrace(t *testing.T) {
	if got := truncateForTrace("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncateForTrace("a long string", 5); got != "a lon..." {
		t.Errorf("got %q", got)
	}
}

func TestNoveltyHandler_NotNovel(t *testing.T) {
	nc, store := setupNoveltyTest(t)

	// Store a skill with an embedding so the novelty check finds a match.
	engine := embedding.NewMockEngine(384)
	content := "how to deploy to kubernetes"
	vec, err := engine.Embed(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.SkillRecord{
		ID:        "skill-k8s-deploy",
		Name:      "kubernetes-deploy",
		Version:   1,
		LibraryID: "test-lib",
		Category:  model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity:    3,
			SolutionCompleteness:  3,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     3,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3,
			CriticConfidence:      0.8,
		},
		Embedding: vec,
	}
	if err := store.Put(context.Background(), skill, nil); err != nil {
		t.Fatal(err)
	}

	// Query with the same content — should be novel=false with score ≥ threshold.
	body, _ := json.Marshal(NoveltyRequest{Content: content})
	req := httptest.NewRequest("POST", "/api/v1/novelty", bytes.NewReader(body))
	w := httptest.NewRecorder()

	nc.HandleNovelty(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp NoveltyResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Novel {
		t.Errorf("expected novel=false, got true (score=%f)", resp.Score)
	}
	if resp.Score < 0.85 {
		t.Errorf("score = %f, want >= 0.85 (threshold)", resp.Score)
	}
}

func TestFindSimilar_EmptyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath, "mock-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	results, err := store.FindSimilar(context.Background(), make([]float32, 384), 0.3, 10)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
