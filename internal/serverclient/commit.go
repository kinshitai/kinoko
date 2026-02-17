package serverclient

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// GitPushCommitter implements model.SkillCommitter by writing SKILL.md files
// into a local git clone and pushing over SSH.
type GitPushCommitter struct {
	sshURL  string
	dataDir string
	log     *slog.Logger
}

// NewGitPushCommitter creates a new GitPushCommitter.
// sshURL is the SSH clone URL for the skill library repo.
// dataDir is the local directory for git workdirs.
func NewGitPushCommitter(sshURL, dataDir string, log *slog.Logger) *GitPushCommitter {
	return &GitPushCommitter{sshURL: sshURL, dataDir: dataDir, log: log}
}

// CommitSkill writes a SKILL.md file into the library repo and pushes it.
func (g *GitPushCommitter) CommitSkill(ctx context.Context, libraryID string, skill *model.SkillRecord, body []byte) (string, error) {
	repoDir := filepath.Join(g.dataDir, libraryID)

	if err := g.ensureRepo(ctx, repoDir); err != nil {
		return "", fmt.Errorf("ensure repo: %w", err)
	}

	// Write skill file.
	skillDir := filepath.Join(repoDir, "skills", skill.ID)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir skill dir: %w", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, body, 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}

	// Git add + commit + push.
	relPath := filepath.Join("skills", skill.ID, "SKILL.md")
	commitMsg := fmt.Sprintf("skill: %s (v%d)", skill.Name, skill.Version)

	if err := g.gitCmd(ctx, repoDir, "add", relPath); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if err := g.gitCmd(ctx, repoDir, "commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	// Get commit hash.
	hash, err := g.gitOutput(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}

	if err := g.gitCmd(ctx, repoDir, "push"); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}

	g.log.Info("committed skill", "library_id", libraryID, "skill_id", skill.ID, "commit", hash)
	return hash, nil
}

func (g *GitPushCommitter) ensureRepo(ctx context.Context, repoDir string) error {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		// Repo exists, pull latest.
		return g.gitCmd(ctx, repoDir, "pull", "--ff-only")
	}
	// Clone fresh.
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", g.sshURL, repoDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}

func (g *GitPushCommitter) gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (g *GitPushCommitter) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
