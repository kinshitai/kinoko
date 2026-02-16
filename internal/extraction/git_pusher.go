package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// GitPusher pushes extracted skills to a Soft Serve git server via SSH.
type GitPusher struct {
	serverAddr string
	keyPath    string
	logger     *slog.Logger
}

// NewGitPusher creates a GitPusher.
func NewGitPusher(serverAddr, keyPath string, logger *slog.Logger) *GitPusher {
	return &GitPusher{
		serverAddr: serverAddr,
		keyPath:    keyPath,
		logger:     logger,
	}
}

// Push creates a temp repo, writes SKILL.md, and force-pushes to the remote.
func (p *GitPusher) Push(ctx context.Context, skillName, libraryID string, body []byte) error {
	tmpDir, err := os.MkdirTemp("", "kinoko-push-*")
	if err != nil {
		return fmt.Errorf("git push: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	skillPath := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(skillPath, body, 0644); err != nil {
		return fmt.Errorf("git push: write SKILL.md: %w", err)
	}

	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", p.keyPath)
	remote := fmt.Sprintf("ssh://%s/%s/%s", p.serverAddr, libraryID, skillName)

	commands := [][]string{
		{"git", "init"},
		{"git", "add", "SKILL.md"},
		{"git", "commit", "-m", fmt.Sprintf("extract: %s", skillName)},
		{"git", "branch", "-M", "main"},
		{"git", "remote", "add", "origin", remote},
		{"git", "push", "origin", "main", "--force"},
	}

	for _, args := range commands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = tmpDir
		cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git push: %v: %w\n%s", args, err, out)
		}
	}

	p.logger.Info("skill pushed to git", "skill", skillName, "library", libraryID, "remote", remote)
	return nil
}
