package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// UpdateInjectionOutcomeRequest is the JSON body for PUT /api/v1/injection-events/{session_id}/outcome.
type UpdateInjectionOutcomeRequest struct {
	Outcome string `json:"outcome"`
}

func (s *Server) handleCreateInjectionEvent(w http.ResponseWriter, r *http.Request) {
	var req model.InjectionEventRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.SessionID == "" {
		http.Error(w, `{"error":"id and session_id required"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.WriteInjectionEvent(r.Context(), req); err != nil {
		s.logger.Error("write injection event failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": req.ID})
}

func (s *Server) handleUpdateInjectionOutcome(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		http.Error(w, `{"error":"session_id required"}`, http.StatusBadRequest)
		return
	}

	var req UpdateInjectionOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Outcome == "" {
		http.Error(w, `{"error":"outcome required"}`, http.StatusBadRequest)
		return
	}
	if req.Outcome != "success" && req.Outcome != "failure" && req.Outcome != "unknown" {
		http.Error(w, `{"error":"outcome must be success, failure, or unknown"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateInjectionOutcome(r.Context(), sessionID, req.Outcome); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "injection event not found"})
			return
		}
		s.logger.Error("update injection outcome failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updated": sessionID})
}
