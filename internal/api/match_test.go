package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func setupMatchTest(t *testing.T) (*Server, *storage.SQLiteStore, embedding.Engine) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	engine := embedding.NewMockEngine(384)
	store, err := storage.NewSQLiteStore(dbPath, engine.ModelID())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	s := &Server{
		embedEngine: engine,
		store:       store,
		logger:      slog.Default(),
		noveltyMux:  http.NewServeMux(),
	}
	return s, store, engine
}

func seedSkill(t *testing.T, store *storage.SQLiteStore, engine embedding.Engine, name, content string) {
	t.Helper()
	ctx := context.Background()

	dir := filepath.Join(t.TempDir(), "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skill := &model.SkillRecord{
		ID:        "skill-" + name,
		Name:      name,
		Version:   1,
		LibraryID: "test-lib",
		Category:  model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.5,
			CriticConfidence:      0.8,
		},
		DecayScore: 1.0,
		FilePath:  skillPath,
	}
	vec, err := engine.Embed(ctx, content)
	if err != nil {
		t.Fatal(err)
	}
	indexer := storage.NewSQLiteIndexer(store)
	if err := indexer.IndexSkill(ctx, skill, vec); err != nil {
		t.Fatalf("index skill: %v", err)
	}
}

func TestHandleMatch_Success(t *testing.T) {
	s, store, engine := setupMatchTest(t)
	seedSkill(t, store, engine, "fix-db-timeout", "# Fix DB Timeout\nRestart the connection pool.")

	body, _ := json.Marshal(MatchRequest{Context: "database connection timeout", Limit: 5, MinScore: 0.0})
	req := httptest.NewRequest("POST", "/api/v1/match", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleMatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp MatchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) == 0 {
		t.Fatal("expected at least one skill match")
	}
	if resp.Skills[0].Name != "fix-db-timeout" {
		t.Errorf("name = %q, want fix-db-timeout", resp.Skills[0].Name)
	}
	if resp.Skills[0].Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestHandleMatch_NoEngine(t *testing.T) {
	s := &Server{logger: slog.Default()}

	body := `{"context":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/match", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleMatch(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleMatch_EmptyContext(t *testing.T) {
	s, _, _ := setupMatchTest(t)

	body := `{"context":""}`
	req := httptest.NewRequest("POST", "/api/v1/match", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleMatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleMatch_NoResults(t *testing.T) {
	s, _, _ := setupMatchTest(t)

	body, _ := json.Marshal(MatchRequest{Context: "something", MinScore: 0.99})
	req := httptest.NewRequest("POST", "/api/v1/match", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleMatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp MatchResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(resp.Skills))
	}
}

func TestHandleMatch_InvalidJSON(t *testing.T) {
	s, _, _ := setupMatchTest(t)

	req := httptest.NewRequest("POST", "/api/v1/match", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	s.handleMatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
