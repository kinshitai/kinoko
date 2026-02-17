package api

import (
	"encoding/json"
	"net/http"

	"github.com/kinoko-dev/kinoko/internal/embedding"
)

// EmbedRequest is the JSON body for POST /api/v1/embed.
type EmbedRequest struct {
	Text string `json:"text"`
}

// EmbedResponse is the JSON response for POST /api/v1/embed.
type EmbedResponse struct {
	Vector []float32 `json:"vector"`
	Model  string    `json:"model"`
	Dims   int       `json:"dims"`
}

// SetEmbedEngine sets the embedding engine used by the /api/v1/embed endpoint.
func (s *Server) SetEmbedEngine(engine embedding.Engine) {
	s.embedEngine = engine
	// Note: embed endpoint registration moved to server.go in API consolidation
	if engine != nil {
		s.logger.Info("embed engine configured")
	}
}

func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	if s.embedEngine == nil {
		http.Error(w, `{"error":"embedding engine not available"}`, http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req EmbedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text required"}`, http.StatusBadRequest)
		return
	}

	vec, err := s.embedEngine.Embed(r.Context(), req.Text)
	if err != nil {
		s.logger.Error("embed failed", "error", err)
		http.Error(w, `{"error":"embedding failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, EmbedResponse{
		Vector: vec,
		Model:  s.embedEngine.ModelID(),
		Dims:   s.embedEngine.Dims(),
	})
}
