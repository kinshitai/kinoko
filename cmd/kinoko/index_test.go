package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createBareRepoWithSkill creates a bare git repo with SKILL.md committed at the given path.
func createBareRepoWithSkill(t *testing.T, skillPath, content string) string {
	t.Helper()

	// Create a regular repo, commit, then clone --bare.
	workDir := filepath.Join(t.TempDir(), "work")
	bareDir := filepath.Join(t.TempDir(), "bare.git")

	// Init regular repo.
	run(t, workDir, true, "git", "init")
	run(t, workDir, false, "git", "config", "user.email", "test@test.com")
	run(t, workDir, false, "git", "config", "user.name", "Test")

	// Write SKILL.md.
	dir := filepath.Join(workDir, filepath.Dir(skillPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, skillPath), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	run(t, workDir, false, "git", "add", ".")
	run(t, workDir, false, "git", "commit", "-m", "init")

	// Clone as bare.
	cmd := exec.Command("git", "clone", "--bare", workDir, bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %s: %v", out, err)
	}

	return bareDir
}

func run(t *testing.T, dir string, mkdir bool, name string, args ...string) {
	t.Helper()
	if mkdir {
		os.MkdirAll(dir, 0o755)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, out, err)
	}
}

func TestReadSkillMDFromBareRepo_RootSkill(t *testing.T) {
	bareDir := createBareRepoWithSkill(t, "SKILL.md", "# Test Skill\n\nversion: 1\n")

	path, body, err := readSkillMDFromBareRepo(bareDir)
	if err != nil {
		t.Fatal(err)
	}
	if path != "SKILL.md" {
		t.Errorf("expected path SKILL.md, got %q", path)
	}
	if len(body) == 0 {
		t.Error("empty body")
	}
}

func TestReadSkillMDFromBareRepo_VersionedSkill(t *testing.T) {
	// Create repo with v1/SKILL.md and v2/SKILL.md.
	workDir := filepath.Join(t.TempDir(), "work")
	bareDir := filepath.Join(t.TempDir(), "bare.git")

	run(t, workDir, true, "git", "init")
	run(t, workDir, false, "git", "config", "user.email", "test@test.com")
	run(t, workDir, false, "git", "config", "user.name", "Test")

	for _, v := range []string{"v1", "v2"} {
		dir := filepath.Join(workDir, v)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Skill "+v+"\n\nversion: 1\n"), 0o644)
	}

	run(t, workDir, false, "git", "add", ".")
	run(t, workDir, false, "git", "commit", "-m", "init")

	cmd := exec.Command("git", "clone", "--bare", workDir, bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %s: %v", out, err)
	}

	path, _, err := readSkillMDFromBareRepo(bareDir)
	if err != nil {
		t.Fatal(err)
	}
	if path != "v2/SKILL.md" {
		t.Errorf("expected v2/SKILL.md (latest version), got %q", path)
	}
}

func TestReadSkillMDFromBareRepo_NoSkill(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "work")
	bareDir := filepath.Join(t.TempDir(), "bare.git")

	run(t, workDir, true, "git", "init")
	run(t, workDir, false, "git", "config", "user.email", "test@test.com")
	run(t, workDir, false, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("hello"), 0o644)
	run(t, workDir, false, "git", "add", ".")
	run(t, workDir, false, "git", "commit", "-m", "init")

	cmd := exec.Command("git", "clone", "--bare", workDir, bareDir)
	cmd.CombinedOutput()

	_, _, err := readSkillMDFromBareRepo(bareDir)
	if err == nil {
		t.Error("expected error for repo without SKILL.md")
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"v1/SKILL.md", 1},
		{"v12/SKILL.md", 12},
		{"SKILL.md", 0},
		{"foo/SKILL.md", 0},
	}
	for _, tt := range tests {
		got := extractVersion(tt.path)
		if got != tt.want {
			t.Errorf("extractVersion(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}
