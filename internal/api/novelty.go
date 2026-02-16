package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

// NoveltyRequest is the JSON body for POST /api/v1/novelty.
type NoveltyRequest struct {
	Content   string  `json:"content"`
	Threshold float64 `json:"threshold,omitempty"` // override default threshold
}

// NoveltyResponse is the JSON response for the novelty check.
type NoveltyResponse struct {
	Novel   bool              `json:"novel"`
	Score   float64           `json:"score"` // highest similarity score found
	Similar []SimilarSkillDTO `json:"similar"`
}

// SimilarSkillDTO is a skill match returned by the novelty endpoint.
type SimilarSkillDTO struct {
	Name      string  `json:"name"`
	LibraryID string  `json:"library_id"`
	Score     float64 `json:"score"`
}

// NoveltyChecker handles novelty API requests.
type NoveltyChecker struct {
	engine    embedding.Engine
	store     *storage.SQLiteStore
	threshold float64 // default novelty threshold
	logger    *slog.Logger
	traceDir  string // debug trace dir (empty = disabled)
}

// NoveltyCheckerConfig configures the novelty checker.
type NoveltyCheckerConfig struct {
	Engine    embedding.Engine
	Store     *storage.SQLiteStore
	Threshold float64 // default 0.85
	Logger    *slog.Logger
	TraceDir  string // empty = no tracing
}

// NewNoveltyChecker creates a NoveltyChecker.
func NewNoveltyChecker(cfg NoveltyCheckerConfig) *NoveltyChecker {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		cfg.Threshold = 0.85
	}
	return &NoveltyChecker{
		engine:    cfg.Engine,
		store:     cfg.Store,
		threshold: cfg.Threshold,
		logger:    cfg.Logger,
		traceDir:  cfg.TraceDir,
	}
}

// HandleNovelty is the HTTP handler for POST /api/v1/novelty.
func (nc *NoveltyChecker) HandleNovelty(w http.ResponseWriter, r *http.Request) {
	var req NoveltyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		http.Error(w, `{"error":"content required"}`, http.StatusBadRequest)
		return
	}

	threshold := nc.threshold
	if req.Threshold > 0 && req.Threshold <= 1 {
		threshold = req.Threshold
	}

	ctx := r.Context()

	if nc.engine == nil {
		http.Error(w, `{"error":"embedding engine not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Embed the input content
	vec, err := nc.engine.Embed(ctx, req.Content)
	if err != nil {
		nc.logger.Error("novelty embed failed", "error", err)
		http.Error(w, `{"error":"embedding failed"}`, http.StatusInternalServerError)
		return
	}

	// Find similar skills (anything above a low threshold to return useful results)
	searchThreshold := 0.3
	similar, err := nc.store.FindSimilar(ctx, vec, searchThreshold, 10)
	if err != nil {
		nc.logger.Error("novelty search failed", "error", err)
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
		return
	}

	// Build response
	maxScore := 0.0
	dtos := make([]SimilarSkillDTO, 0, len(similar))
	for _, s := range similar {
		if s.Score > maxScore {
			maxScore = s.Score
		}
		dtos = append(dtos, SimilarSkillDTO{
			Name:      s.Name,
			LibraryID: s.LibraryID,
			Score:     s.Score,
		})
	}

	novel := maxScore < threshold

	resp := NoveltyResponse{
		Novel:   novel,
		Score:   maxScore,
		Similar: dtos,
	}

	// Debug tracing
	nc.traceNovelty(req.Content, threshold, maxScore, novel, dtos)

	writeJSON(w, http.StatusOK, resp)
}

// traceNovelty writes debug info to trace dir if enabled.
func (nc *NoveltyChecker) traceNovelty(content string, threshold, maxScore float64, novel bool, similar []SimilarSkillDTO) {
	if nc.traceDir == "" {
		return
	}

	entry := map[string]any{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"content":    truncateForTrace(content, 500),
		"threshold":  threshold,
		"max_score":  maxScore,
		"novel":      novel,
		"similar":    similar,
		"model":      "",
	}
	if nc.engine != nil {
		entry["model"] = nc.engine.ModelID()
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		nc.logger.Warn("novelty trace marshal failed", "error", err)
		return
	}

	dir := filepath.Join(nc.traceDir, "novelty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		nc.logger.Warn("novelty trace mkdir failed", "error", err)
		return
	}

	filename := fmt.Sprintf("novelty_%s.json", time.Now().UTC().Format("20060102_150405"))
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		nc.logger.Warn("novelty trace write failed", "error", err)
	}
}

func truncateForTrace(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
