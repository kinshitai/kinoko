package gitserver

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// safePathPattern matches paths that are safe for shell interpolation.
var safePathPattern = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

// validateSafePath checks that a path is safe for shell interpolation.
func validateSafePath(name, value string) error {
	if !safePathPattern.MatchString(value) {
		return fmt.Errorf("%s contains unsafe characters: %q", name, value)
	}
	return nil
}

// InstallHooks writes global pre-receive and post-receive hook scripts to
// the Soft Serve data directory. Idempotent: overwrites existing scripts.
func InstallHooks(dataDir, kinokoBinary string, apiPort int) error {
	// P0-1: Validate inputs to prevent shell injection.
	if err := validateSafePath("dataDir", dataDir); err != nil {
		return err
	}
	if err := validateSafePath("kinokoBinary", kinokoBinary); err != nil {
		return err
	}

	hooksDir := filepath.Join(dataDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	postReceive := fmt.Sprintf(`#!/bin/sh
# Kinoko post-receive hook: index skills after push.
# Soft Serve sets GIT_DIR; we derive repo name from it.
KINOKO_DATA_DIR="%s"
while read oldrev newrev refname; do
    repo=$(echo "$GIT_DIR" | sed "s|${KINOKO_DATA_DIR}/repos/||;s|\.git$||")
    KINOKO_API_URL="http://127.0.0.1:%d" KINOKO_REPO="$repo" KINOKO_REV="$newrev" %s index 2>&1 | logger -t kinoko-hook &
done
`, dataDir, apiPort, kinokoBinary)

	preReceive := fmt.Sprintf(`#!/bin/sh
# Kinoko pre-receive hook: credential scanning.
# Scans pushed SKILL.md files for credentials. Exit 1 = push rejected.
KINOKO_DATA_DIR="%s"
while read oldrev newrev refname; do
    # Get list of changed files
    if [ "$oldrev" = "0000000000000000000000000000000000000000" ]; then
        files=$(git diff-tree --no-commit-id --name-only -r "$newrev" 2>/dev/null)
    else
        files=$(git diff --name-only "$oldrev" "$newrev" 2>/dev/null)
    fi
    for f in $files; do
        case "$f" in
            *.md|*.yaml|*.yml|*.json|*.txt|*.toml|*.cfg|*.conf|*.env)
                # P0-3: Pipe git show directly to avoid shell injection via echo.
                git show "$newrev:$f" 2>/dev/null | %s scan --stdin --reject
                if [ $? -ne 0 ]; then
                    echo "ERROR: Credentials detected in $f. Push rejected." >&2
                    exit 1
                fi
                ;;
        esac
    done
done
`, dataDir, kinokoBinary)

	for name, content := range map[string]string{
		"post-receive": postReceive,
		"pre-receive":  preReceive,
	} {
		path := filepath.Join(hooksDir, name)
		// P1-2: Only the git server user needs to run hooks.
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	return nil
}

// RepoNameFromGitDir extracts the repo name from GIT_DIR relative to the
// Soft Serve data directory. For example:
//
//	GIT_DIR="/data/repos/local/my-skill.git" → "local/my-skill"
//
// Returns an error-indicating empty string if the result contains path
// traversal sequences or is an absolute path.
func RepoNameFromGitDir(gitDir, dataDir string) string {
	if gitDir == "" {
		return ""
	}
	prefix := filepath.Join(dataDir, "repos") + "/"
	name := strings.TrimPrefix(gitDir, prefix)
	name = strings.TrimSuffix(name, ".git")

	// P0-2: Reject path traversal and absolute paths.
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		return ""
	}
	return name
}
