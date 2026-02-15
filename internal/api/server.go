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
	httpServer *http.Server
	store      *storage.SQLiteStore
	embedder   embedding.Embedder
	sshURL     string // SSH clone base URL
	logger     *slog.Logger
	enqueue    func(ctx context.Context, session model.SessionRecord, log []byte) error
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
	Prompt string `json:"prompt"`
	Limit  int    `json:"limit"`
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
}

// New creates a new API server.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Port == 0 {
		cfg.Port = 23232
	}
	s := &Server{
		store:    cfg.Store,
		embedder: cfg.Embedder,
		sshURL:   cfg.SSHURL,
		logger:   cfg.Logger,
		enqueue:  cfg.Enqueue,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/discover", s.handleDiscover)
	mux.HandleFunc("GET /api/v1/discover", s.handleDiscoverGET)
	mux.HandleFunc("POST /api/v1/ingest", s.handleIngest)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return s
}

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
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleDiscoverGET(w http.ResponseWriter, r *http.Request) {
	prompt := r.URL.Query().Get("q")
	if prompt == "" {
		prompt = r.URL.Query().Get("prompt")
	}
	if prompt == "" {
		http.Error(w, `{"error":"missing q or prompt parameter"}`, http.StatusBadRequest)
		return
	}
	limit := 5
	s.discover(w, r, prompt, limit)
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		http.Error(w, `{"error":"prompt required"}`, http.StatusBadRequest)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	s.discover(w, r, req.Prompt, limit)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request, prompt string, limit int) {
	ctx := r.Context()

	// Embed the prompt
	vec, err := s.embedder.Embed(ctx, prompt)
	if err != nil {
		s.logger.Error("embed prompt failed", "error", err)
		http.Error(w, `{"error":"embedding failed"}`, http.StatusInternalServerError)
		return
	}

	// Query store
	results, err := s.store.Query(ctx, storage.SkillQuery{
		Embedding: vec,
		Limit:     limit,
	})
	if err != nil {
		s.logger.Error("query failed", "error", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	skills := make([]SkillMatch, 0, len(results))
	for _, r := range results {
		cloneURL := ""
		if s.sshURL != "" {
			cloneURL = s.sshURL + "/" + r.Skill.LibraryID + "/" + r.Skill.Name
		}
		skills = append(skills, SkillMatch{
			Repo:        r.Skill.LibraryID + "/" + r.Skill.Name,
			Name:        r.Skill.Name,
			Description: r.Skill.FilePath, // best available description field
			Score:       r.CompositeScore,
			CloneURL:    cloneURL,
		})
	}

	writeJSON(w, http.StatusOK, DiscoverResponse{Skills: skills})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Log == "" {
		http.Error(w, `{"error":"session_id and log required"}`, http.StatusBadRequest)
		return
	}

	if s.enqueue != nil {
		session := model.SessionRecord{ID: req.SessionID}
		if err := s.enqueue(r.Context(), session, []byte(req.Log)); err != nil {
			s.logger.Error("ingest enqueue failed", "error", err)
			http.Error(w, `{"error":"enqueue failed"}`, http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"queued": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
