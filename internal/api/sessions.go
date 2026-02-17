package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// CreateSessionRequest is the JSON body for POST /api/v1/sessions.
type CreateSessionRequest struct {
	Session          model.SessionRecord `json:"session"`
	ExtractionResult *UpdateSessionBody  `json:"extraction_result,omitempty"`
}

// UpdateSessionBody holds extraction result fields for PUT /api/v1/sessions/{id}.
type UpdateSessionBody struct {
	ExtractionStatus string `json:"extraction_status"`
	RejectedAtStage  int    `json:"rejected_at_stage"`
	RejectionReason  string `json:"rejection_reason"`
	ExtractedSkillID string `json:"extracted_skill_id"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Session.ID == "" {
		http.Error(w, `{"error":"session.id required"}`, http.StatusBadRequest)
		return
	}

	sess := &req.Session
	if sess.ExtractionStatus == "" {
		sess.ExtractionStatus = model.StatusPending
	}

	if req.ExtractionResult != nil {
		sess.ExtractionStatus = model.ExtractionStatus(req.ExtractionResult.ExtractionStatus)
		sess.RejectedAtStage = req.ExtractionResult.RejectedAtStage
		sess.RejectionReason = req.ExtractionResult.RejectionReason
		sess.ExtractedSkillID = req.ExtractionResult.ExtractedSkillID
	}

	if err := s.store.InsertSession(r.Context(), sess); err != nil {
		s.logger.Error("insert session failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": sess.ID})
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}

	var body UpdateSessionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	sess := &model.SessionRecord{
		ID:               id,
		ExtractionStatus: model.ExtractionStatus(body.ExtractionStatus),
		RejectedAtStage:  body.RejectedAtStage,
		RejectionReason:  body.RejectionReason,
		ExtractedSkillID: body.ExtractedSkillID,
	}

	if err := s.store.UpdateSessionResult(r.Context(), sess); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		s.logger.Error("update session failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updated": id})
}
