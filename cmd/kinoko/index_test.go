package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	_ "modernc.org/sqlite"
)

// skillMD returns a valid SKILL.md with the given name, category, and optional quality block.
func skillMD(name, category string, withQuality bool) string {
	quality := ""
	if withQuality {
		quality = `quality:
  problem_specificity: 4
  solution_completeness: 3
  context_portability: 5
  reasoning_transparency: 2
  technical_accuracy: 4
  verification_evidence: 3
  innovation_level: 2
  composite_score: 3.5
  critic_confidence: 0.8
`
	}
	cat := ""
	if category != "" {
		cat = "category: " + category + "\n"
	}
	return "---\nname: " + name + "\ndescription: A test skill for " + name + "\nversion: 1\nauthor: test\nconfidence: 0.9\ncreated: 2025-01-01\ntags:\n  - testing\n" + cat + quality + "---\n\n# " + name + "\n\n## When to Use\n\nAlways.\n\n## Solution\n\nDo the thing.\n"
}

// setupBareRepo creates a bare git repo with SKILL.md committed, under dataDir/repos/<repoName>.git.
func setupBareRepo(t *testing.T, dataDir, repoName, content string) string {
	t.Helper()
	repoPath := filepath.Join(dataDir, "repos", repoName+".git")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Init bare repo
	run(t, repoPath, "git", "init", "--bare")

	// Create a temp working clone to commit into
	workDir := t.TempDir()
	run(t, workDir, "git", "clone", repoPath, "work")
	work := filepath.Join(workDir, "work")

	// Configure git user
	run(t, work, "git", "config", "user.email", "test@test.com")
	run(t, work, "git", "config", "user.name", "Test")

	// Write SKILL.md and commit
	if err := os.WriteFile(filepath.Join(work, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "init")
	run(t, work, "git", "push", "origin", "HEAD")

	return repoPath
}

// updateRepo updates SKILL.md in an existing bare repo.
func updateRepo(t *testing.T, bareRepoPath, content string) {
	t.Helper()
	workDir := t.TempDir()
	run(t, workDir, "git", "clone", bareRepoPath, "work")
	work := filepath.Join(workDir, "work")
	run(t, work, "git", "config", "user.email", "test@test.com")
	run(t, work, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, "git", "add", ".")
	run(t, work, "git", "commit", "-m", "update")
	run(t, work, "git", "push", "origin", "HEAD")
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("command %s %v failed: %v", name, args, err)
	}
}

func makeCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{RunE: runIndex}
	cmd.Flags().StringVar(&indexRepo, "repo", "", "")
	cmd.Flags().StringVar(&indexRev, "rev", "", "")
	cmd.Flags().StringVar(&indexDSN, "dsn", "", "")
	cmd.Flags().StringVar(&indexAPIURL, "api-url", "", "")
	cmd.Flags().StringVar(&indexDataDir, "data-dir", "", "")
	cmd.Flags().BoolVar(&indexJSON, "json", false, "")
	cmd.SetArgs(args)
	return cmd
}

func TestIndexCreatesSkill(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	setupBareRepo(t, dataDir, "local/test-skill", skillMD("test-skill", "tactical", false))

	cmd := makeCmd("--repo", "local/test-skill", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1") // bad api-url so embedding fails gracefully
	if err := cmd.Execute(); err != nil {
		t.Fatalf("runIndex failed: %v", err)
	}

	// Verify skill in SQLite
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var name, libraryID, category string
	var version int
	err = db.QueryRowContext(context.Background(),
		`SELECT name, library_id, version, category FROM skills WHERE name = ? AND library_id = ?`,
		"test-skill", "local").Scan(&name, &libraryID, &version, &category)
	if err != nil {
		t.Fatalf("skill not found in db: %v", err)
	}
	if name != "test-skill" || libraryID != "local" || version != 1 || category != "tactical" {
		t.Errorf("unexpected values: name=%s lib=%s ver=%d cat=%s", name, libraryID, version, category)
	}
}

func TestIndexUpsert(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	bareRepoPath := setupBareRepo(t, dataDir, "local/upsert-skill", skillMD("upsert-skill", "tactical", false))

	cmd := makeCmd("--repo", "local/upsert-skill", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first index failed: %v", err)
	}

	// Update the skill
	updateRepo(t, bareRepoPath, skillMD("upsert-skill", "foundational", true))

	cmd2 := makeCmd("--repo", "local/upsert-skill", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second index failed: %v", err)
	}

	// Verify upsert
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM skills WHERE name = ? AND library_id = ?`, "upsert-skill", "local").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 skill row, got %d", count)
	}

	var category string
	var qPS int
	db.QueryRowContext(context.Background(), `SELECT category, q_problem_specificity FROM skills WHERE name = ? AND library_id = ?`,
		"upsert-skill", "local").Scan(&category, &qPS)
	if category != "foundational" {
		t.Errorf("expected category foundational, got %s", category)
	}
	if qPS != 4 {
		t.Errorf("expected q_problem_specificity 4, got %d", qPS)
	}
}

func TestIndexJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	setupBareRepo(t, dataDir, "local/json-skill", skillMD("json-skill", "tactical", false))

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := makeCmd("--repo", "local/json-skill", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1", "--json")
	err := cmd.Execute()
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runIndex --json failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result indexResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, buf.String())
	}
	if result.Repo != "local/json-skill" {
		t.Errorf("expected repo local/json-skill, got %s", result.Repo)
	}
	if result.Skill != "json-skill" {
		t.Errorf("expected skill json-skill, got %s", result.Skill)
	}
	if result.Action != "created" {
		t.Errorf("expected action created, got %s", result.Action)
	}
	if result.Embedded {
		t.Error("expected embedded false")
	}
}

func TestIndexMissingRepo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	cmd := makeCmd("--repo", "local/nonexistent", "--dsn", dbPath, "--data-dir", tmpDir, "--api-url", "http://127.0.0.1:1")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("repo not found")) {
		t.Errorf("expected 'repo not found' error, got: %v", err)
	}
}

func TestIndexPathTraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	cmd := makeCmd("--repo", "../../etc/passwd", "--dsn", dbPath, "--data-dir", tmpDir, "--api-url", "http://127.0.0.1:1")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for path traversal repo name")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("..")) {
		t.Errorf("expected error mentioning '..', got: %v", err)
	}
}

func TestIndexInvalidCategory(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	setupBareRepo(t, dataDir, "local/bad-cat", skillMD("bad-cat", "bogus", false))

	cmd := makeCmd("--repo", "local/bad-cat", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("runIndex failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var category string
	db.QueryRowContext(context.Background(), `SELECT category FROM skills WHERE name = ? AND library_id = ?`,
		"bad-cat", "local").Scan(&category)
	if category != "tactical" {
		t.Errorf("expected default category tactical for invalid input, got %s", category)
	}
}

func TestIndexQualityScoreOutOfBounds(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	// Create a SKILL.md with an out-of-bounds quality score
	badQuality := "---\nname: bad-quality\ndescription: A skill with bad quality scores\nversion: 1\nauthor: test\nconfidence: 0.9\ncreated: 2025-01-01\ntags:\n  - testing\nquality:\n  problem_specificity: 99\n  solution_completeness: 3\n  context_portability: 3\n  reasoning_transparency: 3\n  technical_accuracy: 3\n  verification_evidence: 3\n  innovation_level: 3\n  composite_score: 3.0\n  critic_confidence: 0.8\n---\n\n# bad-quality\n\n## When to Use\n\nAlways.\n\n## Solution\n\nDo the thing.\n"
	setupBareRepo(t, dataDir, "local/bad-quality", badQuality)

	cmd := makeCmd("--repo", "local/bad-quality", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for out-of-bounds quality score")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("must be 0-5")) {
		t.Errorf("expected '0-5' bounds error, got: %v", err)
	}
}

func TestIndexDefaultCategory(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbPath := filepath.Join(tmpDir, "kinoko.db")

	// No category in frontmatter
	setupBareRepo(t, dataDir, "local/no-cat", skillMD("no-cat", "", false))

	cmd := makeCmd("--repo", "local/no-cat", "--dsn", dbPath, "--data-dir", dataDir, "--api-url", "http://127.0.0.1:1")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("runIndex failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var category string
	db.QueryRowContext(context.Background(), `SELECT category FROM skills WHERE name = ? AND library_id = ?`,
		"no-cat", "local").Scan(&category)
	if category != "tactical" {
		t.Errorf("expected default category tactical, got %s", category)
	}
}
