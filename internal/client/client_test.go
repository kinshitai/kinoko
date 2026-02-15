package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealth_Unreachable(t *testing.T) {
	c := New(ClientConfig{APIURL: "http://127.0.0.1:1"})
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscover(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/discover" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["prompt"] != "fix database queries" {
			t.Errorf("unexpected prompt: %v", req["prompt"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"skills": []map[string]any{
				{"repo": "local/fix-nplus1", "name": "fix-nplus1", "score": 0.87, "clone_url": "ssh://localhost/local/fix-nplus1"},
			},
		})
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	skills, err := c.Discover(context.Background(), "fix database queries")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Repo != "local/fix-nplus1" {
		t.Errorf("unexpected repo: %s", skills[0].Repo)
	}
	if skills[0].Score != 0.87 {
		t.Errorf("unexpected score: %f", skills[0].Score)
	}
}

func TestReadSkill(t *testing.T) {
	dir := t.TempDir()
	repo := "local/test-skill"
	repoDir := filepath.Join(dir, repo)
	os.MkdirAll(filepath.Join(repoDir, "v1"), 0755)
	os.WriteFile(filepath.Join(repoDir, "v1", "SKILL.md"), []byte("# Test Skill\nDo the thing."), 0644)

	c := New(ClientConfig{CacheDir: dir})
	md, err := c.ReadSkill(repo)
	if err != nil {
		t.Fatal(err)
	}
	if md.Repo != repo {
		t.Errorf("unexpected repo: %s", md.Repo)
	}
	if md.Content != "# Test Skill\nDo the thing." {
		t.Errorf("unexpected content: %s", md.Content)
	}
}

func TestReadSkill_NotFound(t *testing.T) {
	dir := t.TempDir()
	c := New(ClientConfig{CacheDir: dir})
	_, err := c.ReadSkill("nonexistent/skill")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSyncSkills_EmptyCache(t *testing.T) {
	dir := t.TempDir()
	c := New(ClientConfig{CacheDir: dir})
	// Should not error on empty cache
	if err := c.SyncSkills(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncSkills_NonexistentCache(t *testing.T) {
	c := New(ClientConfig{CacheDir: "/nonexistent/path"})
	if err := c.SyncSkills(); err != nil {
		t.Fatal(err)
	}
}

func TestParseServerURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"localhost", "http://localhost:23232"},
		{"http://example.com:8080", "http://example.com:8080"},
		{"https://example.com:9999/foo", "http://example.com:9999"},
		{"192.168.1.1", "http://192.168.1.1:23232"},
	}
	for _, tt := range tests {
		got, err := ParseServerURL(tt.input)
		if err != nil {
			t.Errorf("ParseServerURL(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseServerURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
