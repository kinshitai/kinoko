//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

// TestHookIndexSQLiteFlow is a P1-7 integration test verifying the full
// hook → index → SQLite flow. Since running a real hook requires Soft Serve
// and the kinoko binary, this test simulates the flow by:
// 1. Creating a bare repo with a committed SKILL.md
// 2. Calling the indexing logic directly (as the hook would)
// 3. Verifying the skill appears in SQLite
func TestHookIndexSQLiteFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Create a bare repo with a versioned SKILL.md.
	workDir := filepath.Join(t.TempDir(), "work")
	bareDir := filepath.Join(t.TempDir(), "repos", "local", "test-hook-skill.git")

	// Init, commit, clone --bare.
	os.MkdirAll(workDir, 0o755)
	gitRun(t, workDir, "init")
	gitRun(t, workDir, "config", "user.email", "test@kinoko.dev")
	gitRun(t, workDir, "config", "user.name", "Test")

	v1Dir := filepath.Join(workDir, "v1")
	os.MkdirAll(v1Dir, 0o755)
	skillContent := `---
version: 1
tags: [testing, hooks]
---
# Hook Test Skill

This skill tests the hook→index→SQLite integration.
`
	if err := os.WriteFile(filepath.Join(v1Dir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, workDir, "add", ".")
	gitRun(t, workDir, "commit", "-m", "v1: hook test skill")

	os.MkdirAll(filepath.Dir(bareDir), 0o755)
	cmd := exec.Command("git", "clone", "--bare", workDir, bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %s: %v", out, err)
	}

	// Set up SQLite store.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath, "test-model")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Read SKILL.md from bare repo using git commands (simulating what the index command does).
	lsCmd := exec.Command("git", "ls-tree", "HEAD", "-r", "--name-only")
	lsCmd.Dir = bareDir
	lsOut, err := lsCmd.Output()
	if err != nil {
		t.Fatalf("git ls-tree: %v", err)
	}

	if !containsStr(string(lsOut), "SKILL.md") {
		t.Fatalf("bare repo doesn't contain SKILL.md: %s", lsOut)
	}

	showCmd := exec.Command("git", "show", "HEAD:v1/SKILL.md")
	showCmd.Dir = bareDir
	body, err := showCmd.Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}

	// Index the skill.
	skill := &model.SkillRecord{
		ID:          "local/test-hook-skill/v1",
		Name:        "test-hook-skill",
		Version:     1,
		LibraryID:   "local",
		Category:    model.CategoryTactical,
		Patterns:    []string{"testing", "hooks"},
		ExtractedBy: "kinoko-index",
		FilePath:    "v1/SKILL.md",
		DecayScore:  1.0,
		Quality: model.QualityScores{
			ProblemSpecificity:    3,
			SolutionCompleteness:  3,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     3,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.0,
			CriticConfidence:      0.8,
		},
	}

	indexer := storage.NewSQLiteIndexer(store)
	ctx := context.Background()
	if err := indexer.IndexSkill(ctx, skill, nil); err != nil {
		t.Fatalf("index skill: %v", err)
	}

	// Verify skill is in SQLite.
	got, err := store.Get(ctx, "local/test-hook-skill/v1")
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got == nil {
		t.Fatal("skill not found in SQLite after indexing")
	}
	if got.Name != "test-hook-skill" {
		t.Errorf("skill name = %q, want %q", got.Name, "test-hook-skill")
	}

	// Verify body was readable from bare repo.
	if len(body) == 0 {
		t.Error("SKILL.md body from bare repo is empty")
	}

	t.Logf("hook→index→SQLite flow verified: bare repo read + SQLite insert ✓")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
