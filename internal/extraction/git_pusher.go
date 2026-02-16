package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var safeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// GitPusher pushes extracted skills to a Soft Serve git server via SSH.
type GitPusher struct {
	serverAddr string
	keyPath    string
	log        *slog.Logger
}

// NewGitPusher creates a GitPusher. It validates serverAddr format (host:port)
// and that keyPath points to an existing regular file.
func NewGitPusher(serverAddr, keyPath string, log *slog.Logger) (*GitPusher, error) {
	// Validate serverAddr is host:port with no shell metacharacters.
	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil || host == "" || port == "" {
		return nil, fmt.Errorf("git pusher: invalid server address %q: must be host:port", serverAddr)
	}
	// Reject metacharacters in host or port.
	safeHostPort := regexp.MustCompile(`^[a-zA-Z0-9.\-]+$`)
	if !safeHostPort.MatchString(host) || !safeHostPort.MatchString(port) {
		return nil, fmt.Errorf("git pusher: invalid server address %q: contains invalid characters", serverAddr)
	}

	// Validate keyPath is a regular file.
	if keyPath == "" {
		return nil, fmt.Errorf("git pusher: keyPath must not be empty")
	}
	fi, err := os.Stat(keyPath)
	if err != nil {
		return nil, fmt.Errorf("git pusher: keyPath %q: %w", keyPath, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("git pusher: keyPath %q is not a regular file", keyPath)
	}

	return &GitPusher{
		serverAddr: serverAddr,
		keyPath:    keyPath,
		log:        log,
	}, nil
}

// Push creates a temp repo, writes SKILL.md, and force-pushes to the remote.
func (p *GitPusher) Push(ctx context.Context, skillName, libraryID string, body []byte) error {
	if !safeNameRe.MatchString(skillName) {
		return fmt.Errorf("git push: invalid skillName %q", skillName)
	}
	if !safeNameRe.MatchString(libraryID) {
		return fmt.Errorf("git push: invalid libraryID %q", libraryID)
	}

	// Re-check keyPath at push time in case it was removed after construction.
	if p.keyPath == "" {
		return fmt.Errorf("git push: keyPath is empty")
	}
	if fi, err := os.Stat(p.keyPath); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("git push: keyPath %q is not a valid file", p.keyPath)
	}

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
		{"git", "config", "user.email", "kinoko@local"},
		{"git", "config", "user.name", "kinoko"},
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

	p.log.Info("skill pushed to git", "skill", skillName, "library", libraryID, "remote", remote)
	return nil
}
