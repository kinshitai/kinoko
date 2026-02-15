// Package client provides the Kinoko client library for discovering and
// managing skills from a remote Kinoko server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SkillMatch represents a discovery result from the server.
type SkillMatch struct {
	Repo        string  `json:"repo"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	CloneURL    string  `json:"clone_url"`
}

// SkillMD represents a parsed SKILL.md from a local clone.
type SkillMD struct {
	Repo    string
	Path    string
	Content string
}

// Client connects to a Kinoko server for discovery and manages local skill cache.
type Client struct {
	apiURL   string
	sshURL   string
	cacheDir string
	http     *http.Client
}

// ClientConfig configures a Client.
type ClientConfig struct {
	APIURL   string // HTTP API base URL (e.g. http://localhost:23232)
	SSHURL   string // SSH URL for git clone (e.g. ssh://localhost:23231)
	CacheDir string // Local cache directory (e.g. ~/.kinoko/cache)
}

// New creates a new Client.
func New(cfg ClientConfig) *Client {
	if cfg.CacheDir == "" {
		home, _ := os.UserHomeDir()
		cfg.CacheDir = filepath.Join(home, ".kinoko", "cache")
	}
	return &Client{
		apiURL:   strings.TrimRight(cfg.APIURL, "/"),
		sshURL:   strings.TrimRight(cfg.SSHURL, "/"),
		cacheDir: cfg.CacheDir,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Health checks if the server is reachable.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+"/api/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// Discover queries the server for skills matching the given prompt.
func (c *Client) Discover(ctx context.Context, prompt string) ([]SkillMatch, error) {
	body, _ := json.Marshal(map[string]any{"prompt": prompt, "limit": 10})
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/discover", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discover failed (%d): %s", resp.StatusCode, string(b))
	}

	var result struct {
		Skills []SkillMatch `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode discover response: %w", err)
	}
	return result.Skills, nil
}

// CloneSkill clones a skill repo into the local cache.
// repo is like "local/fix-nplus1". cloneURL overrides SSH if non-empty.
func (c *Client) CloneSkill(repo string, cloneURL string) error {
	dest := filepath.Join(c.cacheDir, repo)
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		// Already cloned, pull instead
		return c.pullRepo(dest)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	remote := cloneURL
	if remote == "" && c.sshURL != "" {
		remote = c.sshURL + "/" + repo
	}
	if remote == "" {
		return fmt.Errorf("no clone URL for %s", repo)
	}

	cmd := exec.Command("git", "clone", "--depth=1", remote, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", repo, err)
	}
	return nil
}

// SyncSkills pulls latest for all cloned skills in the cache.
func (c *Client) SyncSkills() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var errs []string
	for _, lib := range entries {
		if !lib.IsDir() {
			continue
		}
		libPath := filepath.Join(c.cacheDir, lib.Name())
		skills, err := os.ReadDir(libPath)
		if err != nil {
			continue
		}
		for _, skill := range skills {
			if !skill.IsDir() {
				continue
			}
			repoPath := filepath.Join(libPath, skill.Name())
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
				continue
			}
			if err := c.pullRepo(repoPath); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", lib.Name(), skill.Name(), err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ReadSkill reads SKILL.md from a locally cloned skill repo.
func (c *Client) ReadSkill(repo string) (*SkillMD, error) {
	repoPath := filepath.Join(c.cacheDir, repo)

	// Try common SKILL.md locations
	candidates := []string{
		filepath.Join(repoPath, "SKILL.md"),
		filepath.Join(repoPath, "v1", "SKILL.md"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return &SkillMD{
				Repo:    repo,
				Path:    path,
				Content: string(data),
			}, nil
		}
	}
	return nil, fmt.Errorf("SKILL.md not found in %s", repo)
}

// CacheDir returns the local cache directory.
func (c *Client) CacheDir() string {
	return c.cacheDir
}

// ServerURL returns the configured API URL.
func (c *Client) ServerURL() string {
	return c.apiURL
}

func (c *Client) pullRepo(dir string) error {
	cmd := exec.Command("git", "-C", dir, "pull", "--ff-only")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ParseServerURL takes a user-supplied URL (possibly without scheme) and
// returns a normalized HTTP API URL.
func ParseServerURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	// Default API port
	if u.Port() == "" {
		u.Host = u.Hostname() + ":23232"
	}
	u.Scheme = "http"
	u.Path = ""
	return u.String(), nil
}
