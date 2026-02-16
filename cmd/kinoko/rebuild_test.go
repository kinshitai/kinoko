package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	_ "modernc.org/sqlite"
)

func makeRebuildCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{RunE: runRebuild}
	cmd.Flags().BoolVar(&rebuildClean, "clean", false, "")
	cmd.Flags().StringVar(&rebuildLibrary, "library", "", "")
	cmd.Flags().StringVar(&rebuildDSN, "dsn", "", "")
	cmd.Flags().StringVar(&rebuildAPIURL, "api-url", "", "")
	cmd.Flags().StringVar(&rebuildDataDir, "data-dir", "", "")
	cmd.SetArgs(args)
	return cmd
}

func TestRebuildBasic(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create 2 bare repos with SKILL.md.
	setupBareRepo(t, dataDir, "local/skill-one", skillMD("skill-one", "tactical", false))
	setupBareRepo(t, dataDir, "local/skill-two", skillMD("skill-two", "foundational", false))

	// Run rebuild (no API available, embedding will fail gracefully).
	cmd := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}

	// Verify skills in DB.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 skills, got %d", count)
	}
}

func TestRebuildClean(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	setupBareRepo(t, dataDir, "local/skill-one", skillMD("skill-one", "", false))

	// First rebuild to populate.
	cmd := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("first rebuild failed: %v", err)
	}

	// Verify 1 skill.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 skill, got %d", count)
	}

	// Now remove the repo dir and rebuild with --clean. Should end up with 0.
	os.RemoveAll(filepath.Join(dataDir, "repos"))
	os.MkdirAll(filepath.Join(dataDir, "repos"), 0o755)

	cmd2 := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1", "--clean")
	if err := cmd2.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("clean rebuild failed: %v", err)
	}

	db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 skills after clean rebuild, got %d", count)
	}
}

func TestRebuildLibraryFilter(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	setupBareRepo(t, dataDir, "local/skill-one", skillMD("skill-one", "", false))
	setupBareRepo(t, dataDir, "remote/skill-two", skillMD("skill-two", "", false))

	cmd := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1", "--library", "local")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 skill with library filter, got %d", count)
	}

	var lib string
	db.QueryRow("SELECT library_id FROM skills LIMIT 1").Scan(&lib)
	if lib != "local" {
		t.Fatalf("expected library 'local', got %q", lib)
	}
}

func TestRebuildEmptyRepos(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	os.MkdirAll(filepath.Join(dataDir, "repos"), 0o755)

	cmd := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("rebuild with empty repos should succeed: %v", err)
	}
}

func TestRebuildMissingSkillMD(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create a bare repo without SKILL.md.
	repoPath := filepath.Join(dataDir, "repos", "local", "empty-skill.git")
	os.MkdirAll(repoPath, 0o755)
	run(t, repoPath, "git", "init", "--bare")

	// Create a temp clone, commit a dummy file (not SKILL.md), push.
	workDir := t.TempDir()
	run(t, workDir, "git", "clone", repoPath, "work")
	work := filepath.Join(workDir, "work")
	run(t, work, "git", "config", "user.email", "test@test.com")
	run(t, work, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(work, "README.md"), []byte("hello"), 0o644)
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "init")
	run(t, work, "git", "push", "origin", "HEAD")

	// Also create a valid repo.
	setupBareRepo(t, dataDir, "local/good-skill", skillMD("good-skill", "", false))

	cmd := makeRebuildCmd("--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("rebuild should succeed even with missing SKILL.md: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM skills").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 skill (skipping repo without SKILL.md), got %d", count)
	}
}
