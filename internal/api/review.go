package api

import (
	"encoding/json"
	"net/http"
)

// CreateReviewSampleRequest is the JSON body for POST /api/v1/review-samples.
type CreateReviewSampleRequest struct {
	SessionID  string          `json:"session_id"`
	ResultJSON json.RawMessage `json:"result_json"`
}

func (s *Server) handleCreateReviewSample(w http.ResponseWriter, r *http.Request) {
	var req CreateReviewSampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || len(req.ResultJSON) == 0 {
		http.Error(w, `{"error":"session_id and result_json required"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.InsertReviewSample(r.Context(), req.SessionID, req.ResultJSON); err != nil {
		s.logger.Error("insert review sample failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"session_id": req.SessionID})
}
