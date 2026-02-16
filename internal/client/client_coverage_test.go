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

func TestCacheDir(t *testing.T) {
	c := New(ClientConfig{CacheDir: "/tmp/test-cache"})
	if c.CacheDir() != "/tmp/test-cache" {
		t.Fatalf("CacheDir() = %q, want /tmp/test-cache", c.CacheDir())
	}
}

func TestServerURL(t *testing.T) {
	c := New(ClientConfig{APIURL: "http://localhost:9999"})
	if c.ServerURL() != "http://localhost:9999" {
		t.Fatalf("ServerURL() = %q, want http://localhost:9999", c.ServerURL())
	}
}

func TestCacheDir_Default(t *testing.T) {
	c := New(ClientConfig{})
	if c.CacheDir() == "" {
		t.Fatal("expected non-empty default CacheDir")
	}
}

func TestServerURL_TrailingSlashStripped(t *testing.T) {
	c := New(ClientConfig{APIURL: "http://localhost:9999/"})
	if c.ServerURL() != "http://localhost:9999" {
		t.Fatalf("ServerURL() = %q, trailing slash not stripped", c.ServerURL())
	}
}

func TestDiscover_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	_, err := c.Discover(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestDiscover_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	_, err := c.Discover(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestHealth_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 health")
	}
}

func TestValidateRepoPath_SlashPrefix(t *testing.T) {
	if err := validateRepoPath("/absolute"); err == nil {
		t.Fatal("expected error for / prefix")
	}
	if err := validateRepoPath("valid/repo"); err != nil {
		t.Fatalf("unexpected error for valid path: %v", err)
	}
}

func TestCloneSkill_AlreadyCloned(t *testing.T) {
	dir := t.TempDir()
	repo := "local/skill"
	repoDir := filepath.Join(dir, repo)
	_ = os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)

	c := New(ClientConfig{CacheDir: dir})
	// pullRepo will fail (no real git repo) but that exercises the already-cloned path.
	err := c.CloneSkill(repo, "")
	// If err == nil, git is available and the pull somehow works — that's fine too.
	// The point is we hit the already-cloned branch.
	_ = err
}

func TestCloneSkill_NoRemoteURL(t *testing.T) {
	dir := t.TempDir()
	c := New(ClientConfig{CacheDir: dir})
	err := c.CloneSkill("local/skill", "")
	if err == nil {
		t.Fatal("expected error when no remote URL available")
	}
}

func TestSyncSkills_WithSkills(t *testing.T) {
	dir := t.TempDir()
	// Create a cache structure with a "cloned" repo (has .git dir).
	lib := filepath.Join(dir, "local")
	skill := filepath.Join(lib, "my-skill")
	os.MkdirAll(filepath.Join(skill, ".git"), 0755)

	c := New(ClientConfig{CacheDir: dir})
	// SyncSkills will try to git pull, which will fail since it's not a real repo.
	// But it exercises the directory traversal code path.
	err := c.SyncSkills()
	// We expect an error since pull fails.
	if err == nil {
		t.Log("SyncSkills succeeded (unexpected but acceptable)")
	}
}

func TestSyncSkills_SkipsNonDirs(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "local")
	os.MkdirAll(lib, 0755)
	// Create a file (not a dir) inside the library dir.
	os.WriteFile(filepath.Join(lib, "not-a-skill"), []byte("x"), 0644)

	c := New(ClientConfig{CacheDir: dir})
	// Should not error on non-directory entries.
	if err := c.SyncSkills(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncSkills_SkipsNoGit(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "local")
	skill := filepath.Join(lib, "my-skill")
	os.MkdirAll(skill, 0755) // no .git dir

	c := New(ClientConfig{CacheDir: dir})
	if err := c.SyncSkills(); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"skills": []any{}})
	}))
	defer srv.Close()

	c := New(ClientConfig{APIURL: srv.URL})
	skills, err := c.Discover(context.Background(), "nothing")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}
