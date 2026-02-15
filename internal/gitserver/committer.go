package gitserver

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// Compile-time check.
var _ model.SkillCommitter = (*GitCommitter)(nil)

// Embedder computes an embedding vector from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// GitCommitter pushes skills to Soft Serve repos and indexes after push.
type GitCommitter struct {
	server   *Server
	dataDir  string
	indexer  model.SkillIndexer
	embedder Embedder
	logger   *slog.Logger
	locks    sync.Map // keyed by "{libraryID}/{skillName}" → *sync.Mutex
}

// GitCommitterConfig holds constructor parameters.
type GitCommitterConfig struct {
	Server   *Server
	DataDir  string
	Indexer  model.SkillIndexer
	Embedder Embedder
	Logger   *slog.Logger
}

// NewGitCommitter creates a GitCommitter.
func NewGitCommitter(cfg GitCommitterConfig) *GitCommitter {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &GitCommitter{
		server:   cfg.Server,
		dataDir:  cfg.DataDir,
		indexer:  cfg.Indexer,
		embedder: cfg.Embedder,
		logger:   cfg.Logger,
	}
}

// CommitSkill creates a repo (if needed), writes the skill body, pushes to
// Soft Serve, and indexes into SQLite after a successful push.
// skillMutex returns a per-skill mutex to prevent concurrent workdir stomping.
func (g *GitCommitter) skillMutex(key string) *sync.Mutex {
	v, _ := g.locks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (g *GitCommitter) CommitSkill(ctx context.Context, libraryID string, skill *model.SkillRecord, body []byte) (string, error) {
	repoName := fmt.Sprintf("%s/%s", libraryID, skill.Name)
	workdir := filepath.Join(g.dataDir, "workdir", libraryID, skill.Name)

	mu := g.skillMutex(repoName)
	mu.Lock()
	defer mu.Unlock()

	// Create repo (ignore "already exists").
	if err := g.server.CreateRepo(repoName, skill.Name); err != nil {
		if !isAlreadyExists(err) {
			return "", fmt.Errorf("create repo %s: %w", repoName, err)
		}
	}

	// Clone or pull.
	if err := g.ensureWorkdir(ctx, repoName, workdir); err != nil {
		return "", fmt.Errorf("ensure workdir: %w", err)
	}

	// Write version directory.
	versionDir := filepath.Join(workdir, fmt.Sprintf("v%d", skill.Version))
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir version dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "SKILL.md"), body, 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}

	// Git add, commit, push.
	hash, err := g.commitAndPush(ctx, workdir, fmt.Sprintf("v%d: extracted", skill.Version))
	if err != nil {
		return "", fmt.Errorf("commit and push: %w", err)
	}

	// Post-push indexing.
	if g.indexer != nil {
		var emb []float32
		if g.embedder != nil {
			emb, err = g.embedder.Embed(ctx, string(body))
			if err != nil {
				g.logger.Warn("embedding failed, indexing without", "repo", repoName, "error", err)
			}
		}
		if err := g.indexer.IndexSkill(ctx, skill, emb); err != nil {
			g.logger.Error("post-push indexing failed", "repo", repoName, "error", err)
		}
	}

	g.logger.Info("skill committed", "repo", repoName, "version", skill.Version, "hash", hash)
	return hash, nil
}

// ensureWorkdir clones or pulls the repo into workdir.
func (g *GitCommitter) ensureWorkdir(ctx context.Context, repoName, workdir string) error {
	cloneURL := g.server.GetCloneURL(repoName)
	sshKey := g.server.adminKeyPath
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR", sshKey)

	if _, err := os.Stat(filepath.Join(workdir, ".git")); err == nil {
		// Already cloned — pull.
		cmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git pull: %s: %w", out, err)
		}
		return nil
	}

	// Fresh clone.
	if err := os.MkdirAll(filepath.Dir(workdir), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, workdir)
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Empty repo — init locally and add remote.
		if strings.Contains(string(out), "empty") || strings.Contains(string(out), "warning") {
			return g.initEmptyWorkdir(ctx, cloneURL, workdir, sshCmd)
		}
		return fmt.Errorf("git clone: %s: %w", out, err)
	}
	return nil
}

// initEmptyWorkdir handles cloning an empty Soft Serve repo.
func (g *GitCommitter) initEmptyWorkdir(ctx context.Context, cloneURL, workdir, sshCmd string) error {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}
	env := append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", cloneURL},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workdir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %s: %w", args[0], out, err)
		}
	}
	return nil
}

// commitAndPush stages all, commits, pushes, and returns the commit hash.
func (g *GitCommitter) commitAndPush(ctx context.Context, workdir, message string) (string, error) {
	sshKey := g.server.adminKeyPath
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR", sshKey)
	env := append(os.Environ(),
		"GIT_SSH_COMMAND="+sshCmd,
		"GIT_AUTHOR_NAME=kinoko",
		"GIT_AUTHOR_EMAIL=kinoko@local",
		"GIT_COMMITTER_NAME=kinoko",
		"GIT_COMMITTER_EMAIL=kinoko@local",
	)

	cmds := [][]string{
		{"add", "."},
		{"commit", "-m", message},
		{"push", "-u", "origin", "HEAD"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workdir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %s: %w", args[0], out, err)
		}
	}

	// Get commit hash.
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// isAlreadyExists checks if an error indicates the repo already exists.
func isAlreadyExists(err error) bool {
	return strings.Contains(err.Error(), "already exists") ||
		strings.Contains(err.Error(), "repository exists")
}
