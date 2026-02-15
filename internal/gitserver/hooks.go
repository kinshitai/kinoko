package gitserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallHooks writes global pre-receive and post-receive hook scripts to
// the Soft Serve data directory. Idempotent: overwrites existing scripts.
func InstallHooks(dataDir, kinokoBinary string) error {
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
    KINOKO_REPO="$repo" KINOKO_REV="$newrev" %s index 2>&1 | logger -t kinoko-hook &
done
`, dataDir, kinokoBinary)

	preReceive := `#!/bin/sh
# Kinoko pre-receive hook: credential scanning (placeholder).
# Will call: kinoko scan --stdin --reject
exit 0
`

	for name, content := range map[string]string{
		"post-receive": postReceive,
		"pre-receive":  preReceive,
	} {
		path := filepath.Join(hooksDir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	return nil
}

// RepoNameFromGitDir extracts the repo name from GIT_DIR relative to the
// Soft Serve data directory. For example:
//
//	GIT_DIR="/data/repos/local/my-skill.git" → "local/my-skill"
func RepoNameFromGitDir(gitDir, dataDir string) string {
	if gitDir == "" {
		return ""
	}
	prefix := filepath.Join(dataDir, "repos") + "/"
	name := strings.TrimPrefix(gitDir, prefix)
	name = strings.TrimSuffix(name, ".git")
	return name
}
