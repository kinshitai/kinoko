package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// RegisterRequest is the JSON body for POST /api/v1/register.
type RegisterRequest struct {
	PublicKey string `json:"public_key"` // e.g. "ssh-ed25519 AAAA..."
	Name      string `json:"name"`       // hostname / identifier for the user
}

// RegisterResponse is the JSON response for POST /api/v1/register.
type RegisterResponse struct {
	Status   string `json:"status"`
	Username string `json:"username"`
}

// RegisterFn is called by the register handler to perform the actual registration.
// It receives the validated public key and sanitized username.
type RegisterFn func(ctx context.Context, pubkey, username string) error

// reValidName matches alphanumeric characters and hyphens, 1-64 chars.
var reValidName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,63}$`)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Auth gate: if registrationToken is configured, require Bearer token.
	if s.registrationToken != "" {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(s.registrationToken)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Validate public key: must look like an SSH public key with key-type and base64 data.
	req.PublicKey = strings.TrimSpace(req.PublicKey)
	pubkeyParts := strings.Fields(req.PublicKey)
	if len(pubkeyParts) < 2 || !strings.HasPrefix(pubkeyParts[0], "ssh-") || len(pubkeyParts[1]) < 20 {
		http.Error(w, `{"error":"invalid public_key: must be a valid SSH public key (e.g. ssh-ed25519 AAAA...)"}`, http.StatusBadRequest)
		return
	}

	// Validate and sanitize name.
	req.Name = strings.TrimSpace(req.Name)
	if !reValidName.MatchString(req.Name) {
		http.Error(w, `{"error":"invalid name: must be alphanumeric with hyphens, 1-64 chars"}`, http.StatusBadRequest)
		return
	}

	if s.registerFn == nil {
		http.Error(w, `{"error":"registration not configured"}`, http.StatusNotImplemented)
		return
	}

	if err := s.registerFn(r.Context(), req.PublicKey, req.Name); err != nil {
		s.logger.Error("registration failed", "name", req.Name, "error", err)
		http.Error(w, `{"error":"registration failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, RegisterResponse{
		Status:   "registered",
		Username: req.Name,
	})
}
