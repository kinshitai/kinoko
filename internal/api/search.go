package api

import (
	"encoding/json"
	"net/http"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

// SearchRequest is the JSON body for POST /api/v1/search.
type SearchRequest struct {
	Patterns   []string  `json:"patterns,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	LibraryIDs []string  `json:"library_ids,omitempty"`
	MinQuality float64   `json:"min_quality,omitempty"`
	MinDecay   float64   `json:"min_decay,omitempty"`
	Limit      int       `json:"limit,omitempty"`
}

// SearchResponse is the JSON response for POST /api/v1/search.
type SearchResponse struct {
	Results []model.ScoredSkill `json:"results"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	results, err := s.store.Query(r.Context(), storage.SkillQuery{
		Patterns:   req.Patterns,
		Embedding:  req.Embedding,
		LibraryIDs: req.LibraryIDs,
		MinQuality: req.MinQuality,
		MinDecay:   req.MinDecay,
		Limit:      limit,
	})
	if err != nil {
		s.logger.Error("search query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if results == nil {
		results = []model.ScoredSkill{}
	}
	writeJSON(w, http.StatusOK, SearchResponse{Results: results})
}
