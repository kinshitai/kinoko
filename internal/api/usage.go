package api

import (
	"encoding/json"
	"net/http"
)

// UpdateUsageRequest is the JSON body for POST /api/v1/skills/{id}/usage.
type UpdateUsageRequest struct {
	Outcome string `json:"outcome"`
}

func (s *Server) handleUpdateUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"skill id required"}`, http.StatusBadRequest)
		return
	}

	var req UpdateUsageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateUsage(r.Context(), id, req.Outcome); err != nil {
		s.logger.Error("update usage failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updated": id})
}
