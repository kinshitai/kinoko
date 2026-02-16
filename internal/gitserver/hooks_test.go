package gitserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_CreatesScripts(t *testing.T) {
	dataDir := t.TempDir()
	kinokoBin := "/usr/local/bin/kinoko"

	if err := InstallHooks(dataDir, kinokoBin, 23233); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	hooksDir := filepath.Join(dataDir, "hooks")

	for _, name := range []string{"post-receive", "pre-receive"} {
		path := filepath.Join(hooksDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// P1-2: Should be 0o700, not 0o755.
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s: expected perm 0700, got %04o", name, perm)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(content) == 0 {
			t.Errorf("%s: empty", name)
		}
	}
}

func TestInstallHooks_PostReceiveContainsBinary(t *testing.T) {
	dataDir := t.TempDir()
	kinokoBin := "/opt/kinoko/bin/kinoko"

	if err := InstallHooks(dataDir, kinokoBin, 23233); err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dataDir, "hooks", "post-receive"))
	s := string(content)
	if !contains(s, kinokoBin) {
		t.Errorf("post-receive does not reference kinoko binary %q", kinokoBin)
	}
	if !contains(s, "index") {
		t.Error("post-receive does not call 'index' command")
	}
}

func TestInstallHooks_Idempotent(t *testing.T) {
	dataDir := t.TempDir()

	if err := InstallHooks(dataDir, "/bin/kinoko", 23233); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(dataDir, "/bin/kinoko", 23233); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestInstallHooks_RejectsShellInjection(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		binary  string
	}{
		{"dollar in dataDir", "/tmp/foo$bar", "/bin/kinoko"},
		{"backtick in binary", "/tmp/ok", "/bin/`whoami`"},
		{"double quote in dataDir", `/tmp/foo"bar`, "/bin/kinoko"},
		{"backslash in binary", "/tmp/ok", `/bin/kin\oko`},
		{"space in dataDir", "/tmp/foo bar", "/bin/kinoko"},
		{"semicolon in binary", "/tmp/ok", "/bin/kinoko;rm -rf /"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InstallHooks(tt.dataDir, tt.binary, 23233)
			if err == nil {
				t.Error("expected error for unsafe input, got nil")
			}
		})
	}
}

func TestRepoNameFromGitDir(t *testing.T) {
	tests := []struct {
		gitDir  string
		dataDir string
		want    string
	}{
		{"/data/repos/local/my-skill.git", "/data", "local/my-skill"},
		{"/data/repos/company/circuit-breaker.git", "/data", "company/circuit-breaker"},
		{"/data/repos/simple.git", "/data", "simple"},
		{"", "/data", ""},
	}
	for _, tt := range tests {
		got := RepoNameFromGitDir(tt.gitDir, tt.dataDir)
		if got != tt.want {
			t.Errorf("RepoNameFromGitDir(%q, %q) = %q, want %q", tt.gitDir, tt.dataDir, got, tt.want)
		}
	}
}

func TestRepoNameFromGitDir_RejectsTraversal(t *testing.T) {
	tests := []struct {
		name    string
		gitDir  string
		dataDir string
	}{
		{"dotdot traversal", "/data/repos/../../etc/passwd.git", "/data"},
		{"absolute result", "/etc/passwd.git", "/data"},
		{"embedded dotdot", "/data/repos/foo/../bar.git", "/data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RepoNameFromGitDir(tt.gitDir, tt.dataDir)
			if got != "" {
				t.Errorf("expected empty string for unsafe path, got %q", got)
			}
		})
	}
}

// P1-4: Tests for hook script generation content.
func TestInstallHooks_ScriptContent(t *testing.T) {
	dataDir := t.TempDir()
	kinokoBin := "/usr/local/bin/kinoko"

	if err := InstallHooks(dataDir, kinokoBin, 23233); err != nil {
		t.Fatal(err)
	}

	preReceive, err := os.ReadFile(filepath.Join(dataDir, "hooks", "pre-receive"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(preReceive)

	// Verify expected paths are present
	if !contains(script, dataDir) {
		t.Error("pre-receive should contain dataDir path")
	}
	if !contains(script, kinokoBin) {
		t.Error("pre-receive should contain kinoko binary path")
	}

	// P0-3: Verify script pipes git show directly (no echo $content)
	if contains(script, "echo \"$content\"") {
		t.Error("pre-receive should NOT use echo $content (shell injection risk)")
	}
	if !contains(script, "git show \"$newrev:$f\"") {
		t.Error("pre-receive should pipe git show directly")
	}
	if !contains(script, "scan --stdin --reject") {
		t.Error("pre-receive should call scan --stdin --reject")
	}
}

func TestInstallHooks_PostReceiveContainsAPIURL(t *testing.T) {
	dataDir := t.TempDir()
	if err := InstallHooks(dataDir, "/usr/local/bin/kinoko", 9999); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(dataDir, "hooks", "post-receive"))
	s := string(content)
	if !contains(s, `KINOKO_API_URL="http://127.0.0.1:9999"`) {
		t.Error("post-receive should contain KINOKO_API_URL with correct port")
	}
}

func TestInstallHooks_PathValidationRejectsMalicious(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		binary  string
	}{
		{"pipe in path", "/tmp/ok", "/bin/kinoko|evil"},
		{"ampersand", "/tmp/ok", "/bin/kinoko&evil"},
		{"newline-like", "/tmp/ok", "/bin/kinoko\nevil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InstallHooks(tt.dataDir, tt.binary, 23233)
			if err == nil {
				t.Error("expected error for unsafe input")
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
