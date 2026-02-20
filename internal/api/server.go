// Package api provides the HTTP discovery and ingestion API for Kinoko.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

// Server is the HTTP API server for discovery and ingestion.
type Server struct {
	httpServer  *http.Server
	store       *storage.SQLiteStore
	embedder    embedding.Embedder
	sshURL      string // SSH clone base URL
	logger      *slog.Logger
	enqueue     func(ctx context.Context, session model.SessionRecord, log []byte) error
	discoverSem chan struct{} // P1-7: semaphore to limit concurrent discover requests
	embedEngine embedding.Engine
}

// Config configures the API server.
type Config struct {
	Host     string
	Port     int
	Store    *storage.SQLiteStore
	Embedder embedding.Embedder
	SSHURL   string // e.g. "ssh://localhost:23231"
	Logger   *slog.Logger
	Enqueue  func(ctx context.Context, session model.SessionRecord, log []byte) error
}

// DiscoverRequest is the JSON body for POST /api/v1/discover.
type DiscoverRequest struct {
	Prompt     string    `json:"prompt"`
	Embedding  []float64 `json:"embedding"` // Note: API accepts float64, we convert to float32 internally
	Patterns   []string  `json:"patterns"`
	LibraryIDs []string  `json:"library_ids"`
	MinQuality float64   `json:"min_quality"`
	TopK       int       `json:"top_k"`
	Limit      int       `json:"limit"` // Keep for backward compatibility
}

// SkillMatch is a single discovery result.
type SkillMatch struct {
	Repo        string  `json:"repo"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	CloneURL    string  `json:"clone_url"`
}

// DiscoverResponse is the JSON response for discovery.
type DiscoverResponse struct {
	Skills []SkillMatch `json:"skills"`
}

// IngestRequest is the JSON body for POST /api/v1/ingest.
type IngestRequest struct {
	SessionID string `json:"session_id"`
	Log       string `json:"log"`
}

// HealthResponse is the JSON response for GET /api/v1/health.
type HealthResponse struct {
	Status string `json:"status"`
	Skills int    `json:"skills,omitempty"` // P1-8: skill count
}

// New creates a new API server.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Port == 0 {
		cfg.Port = 23233
	}
	s := &Server{
		store:       cfg.Store,
		embedder:    cfg.Embedder,
		sshURL:      cfg.SSHURL,
		logger:      cfg.Logger,
		enqueue:     cfg.Enqueue,
		discoverSem: make(chan struct{}, 10), // P1-7: max 10 concurrent discover requests
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/discover", s.handleDiscover)
	mux.HandleFunc("POST /api/v1/embed", s.handleEmbed)
	mux.HandleFunc("POST /api/v1/ingest", s.handleIngest)
	mux.HandleFunc("GET /api/v1/skills/decay", s.handleListByDecay)
	mux.HandleFunc("PATCH /api/v1/skills/{id}/decay", s.handleUpdateDecay)

	// Novelty endpoint removed in API consolidation

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return s
}

// SetNoveltyChecker removed - novelty endpoint consolidated into discover

// Start starts the HTTP server in a goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}
	s.logger.Info("API server listening", "addr", ln.Addr().String())
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", "error", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server's listen address (useful in tests).
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{Status: "ok"}
	// P1-8: Include skill count if store is available.
	if s.store != nil {
		if n, err := s.store.CountSkills(r.Context()); err == nil {
			resp.Skills = n
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleDiscoverGET removed - use POST /api/v1/discover instead

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Security bounds validation
	if len(req.Embedding) > 2048 {
		http.Error(w, `{"error":"embedding dimensions cannot exceed 2048"}`, http.StatusBadRequest)
		return
	}
	if len(req.Patterns) > 50 {
		http.Error(w, `{"error":"patterns cannot exceed 50"}`, http.StatusBadRequest)
		return
	}
	if len(req.LibraryIDs) > 50 {
		http.Error(w, `{"error":"library_ids cannot exceed 50"}`, http.StatusBadRequest)
		return
	}

	// Validate: at least one of prompt, embedding, or patterns must be provided
	if req.Prompt == "" && len(req.Embedding) == 0 && len(req.Patterns) == 0 {
		http.Error(w, `{"error":"at least one of prompt, embedding, or patterns must be provided"}`, http.StatusBadRequest)
		return
	}

	// Handle TopK vs Limit for backward compatibility
	limit := req.TopK
	if limit <= 0 {
		limit = req.Limit
	}
	if limit <= 0 {
		limit = 10 // default from spec
	}

	// If prompt is provided without embedding, embed it
	var embedding []float32
	if req.Prompt != "" && len(req.Embedding) == 0 {
		if s.embedder == nil {
			http.Error(w, `{"error":"embedding not configured — set KINOKO_EMBEDDING_API_KEY or OPENAI_API_KEY"}`, http.StatusServiceUnavailable)
			return
		}
		vec, err := s.embedder.Embed(r.Context(), req.Prompt)
		if err != nil {
			s.logger.Error("embed prompt failed", "error", err)
			http.Error(w, `{"error":"embedding failed"}`, http.StatusInternalServerError)
			return
		}
		embedding = vec
	} else if len(req.Embedding) > 0 {
		// Convert float64 to float32 for internal use
		embedding = make([]float32, len(req.Embedding))
		for i, v := range req.Embedding {
			embedding[i] = float32(v)
		}
	}

	s.discoverWithQuery(w, r, req, embedding, limit)
}

func (s *Server) discoverWithQuery(w http.ResponseWriter, r *http.Request, req DiscoverRequest, embedding []float32, limit int) {
	// P1-7: Rate limit concurrent discover requests.
	select {
	case s.discoverSem <- struct{}{}:
		defer func() { <-s.discoverSem }()
	default:
		http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
		return
	}

	// Build SkillQuery from all provided params
	query := storage.SkillQuery{
		Patterns:   req.Patterns,
		Embedding:  embedding,
		LibraryIDs: req.LibraryIDs,
		MinQuality: req.MinQuality,
		Limit:      limit,
	}

	// Query store
	results, err := s.store.Query(r.Context(), query)
	if err != nil {
		s.logger.Error("query failed", "error", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	skills := make([]SkillMatch, 0, len(results))
	for _, result := range results {
		cloneURL := ""
		if s.sshURL != "" {
			cloneURL = s.sshURL + "/" + result.Skill.LibraryID + "/" + result.Skill.Name
		}
		skills = append(skills, SkillMatch{
			Repo:        result.Skill.LibraryID + "/" + result.Skill.Name,
			Name:        result.Skill.Name,
			Description: result.Skill.Description,
			Score:       result.CompositeScore,
			CloneURL:    cloneURL,
		})
	}

	writeJSON(w, http.StatusOK, DiscoverResponse{Skills: skills})
}

// Removed unused discover() method - replaced by unified handleDiscover

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	// P1-6: Limit request body to 10 MB to prevent memory exhaustion.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Log == "" {
		http.Error(w, `{"error":"session_id and log required"}`, http.StatusBadRequest)
		return
	}

	// Create session record in store if available.
	if s.store != nil {
		sess := &model.SessionRecord{
			ID:               req.SessionID,
			ExtractionStatus: model.StatusQueued,
		}
		if err := s.store.InsertSession(r.Context(), sess); err != nil {
			s.logger.Warn("insert session on ingest (may already exist)", "error", err)
		}
	}

	if s.enqueue == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "ingestion not available — use 'kinoko run' to process sessions"})
		return
	}

	session := model.SessionRecord{ID: req.SessionID}
	if err := s.enqueue(r.Context(), session, []byte(req.Log)); err != nil {
		s.logger.Error("ingest enqueue failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"queued": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
