package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MatchRequest is the JSON body for POST /api/v1/match.
type MatchRequest struct {
	Context  string  `json:"context"`
	Limit    int     `json:"limit"`
	MinScore float64 `json:"min_score"`
}

// MatchedSkillDTO is a single skill match with content.
type MatchedSkillDTO struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}

// MatchResponse is the JSON response for POST /api/v1/match.
type MatchResponse struct {
	Skills []MatchedSkillDTO `json:"skills"`
}

// SetMatchHandler registers the match endpoint on the server mux.
func (s *Server) SetMatchHandler() {
	if s.noveltyMux != nil {
		s.noveltyMux.HandleFunc("POST /api/v1/match", s.handleMatch)
		s.logger.Info("match endpoint registered")
	}
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	if s.embedEngine == nil {
		http.Error(w, `{"error":"embedding engine not available"}`, http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req MatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Context == "" {
		http.Error(w, `{"error":"context required"}`, http.StatusBadRequest)
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.5
	}

	ctx := r.Context()

	vec, err := s.embedEngine.Embed(ctx, req.Context)
	if err != nil {
		s.logger.Error("match embed failed", "error", err)
		http.Error(w, `{"error":"embedding failed"}`, http.StatusInternalServerError)
		return
	}

	similar, err := s.store.FindSimilar(ctx, vec, req.MinScore, req.Limit)
	if err != nil {
		s.logger.Error("match search failed", "error", err)
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
		return
	}

	const maxResponseBytes = 64 * 1024 // 64KB cap on total skill content

	skills := make([]MatchedSkillDTO, 0, len(similar))
	totalSize := 0
	for _, sim := range similar {
		content := readSkillContent(sim.FilePath)
		totalSize += len(content)
		if totalSize > maxResponseBytes {
			// Truncate this skill's content to stay within budget.
			overage := totalSize - maxResponseBytes
			content = content[:len(content)-overage]
			skills = append(skills, MatchedSkillDTO{
				Name:    sim.Name,
				Score:   sim.Score,
				Content: content,
			})
			break
		}
		skills = append(skills, MatchedSkillDTO{
			Name:    sim.Name,
			Score:   sim.Score,
			Content: content,
		})
	}

	writeJSON(w, http.StatusOK, MatchResponse{Skills: skills})
}

// readSkillContent reads a SKILL.md file from disk. Returns empty string on error.
// Rejects paths containing traversal sequences to prevent path traversal attacks.
func readSkillContent(filePath string) string {
	if filePath == "" {
		return ""
	}
	// Block any path containing traversal components.
	cleaned := filepath.Clean(filePath)
	if strings.Contains(cleaned, "..") {
		return ""
	}
	data, err := os.ReadFile(cleaned)
	if err != nil {
		return ""
	}
	return string(data)
}
