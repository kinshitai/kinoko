package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// DecayListResponse is the JSON response for GET /api/v1/skills/decay.
type DecayListResponse struct {
	Skills []model.SkillRecord `json:"skills"`
}

// UpdateDecayRequest is the JSON body for PATCH /api/v1/skills/{id}/decay.
type UpdateDecayRequest struct {
	DecayScore float64 `json:"decay_score"`
}

func (s *Server) handleListByDecay(w http.ResponseWriter, r *http.Request) {
	libraryID := r.URL.Query().Get("library_id")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	skills, err := s.store.ListByDecay(r.Context(), libraryID, limit)
	if err != nil {
		s.logger.Error("list by decay failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if skills == nil {
		skills = []model.SkillRecord{}
	}
	writeJSON(w, http.StatusOK, DecayListResponse{Skills: skills})
}

func (s *Server) handleUpdateDecay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"skill id required"}`, http.StatusBadRequest)
		return
	}

	var req UpdateDecayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.DecayScore < 0.0 || req.DecayScore > 1.0 {
		http.Error(w, `{"error":"decay_score must be between 0.0 and 1.0"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateDecay(r.Context(), id, req.DecayScore); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		s.logger.Error("update decay failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updated": id})
}
