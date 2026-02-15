package gitserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_CreatesScripts(t *testing.T) {
	dataDir := t.TempDir()
	kinokoBin := "/usr/local/bin/kinoko"

	if err := InstallHooks(dataDir, kinokoBin); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	hooksDir := filepath.Join(dataDir, "hooks")

	for _, name := range []string{"post-receive", "pre-receive"} {
		path := filepath.Join(hooksDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s: not executable (mode %v)", name, info.Mode())
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

	if err := InstallHooks(dataDir, kinokoBin); err != nil {
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

	if err := InstallHooks(dataDir, "/bin/kinoko"); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(dataDir, "/bin/kinoko"); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestRepoNameFromGitDir(t *testing.T) {
	tests := []struct {
		gitDir string
		dataDir string
		want   string
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
