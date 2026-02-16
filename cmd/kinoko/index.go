package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
	skillpkg "github.com/kinoko-dev/kinoko/pkg/skill"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index a skill repo into SQLite (called by post-receive hook)",
	Long: `Parses SKILL.md from a repo, computes embedding, and indexes into SQLite.
Typically invoked by the Soft Serve post-receive hook.

Environment variables:
  KINOKO_REPO         Repo name (e.g. "local/fix-nplus1")
  KINOKO_REV          Git revision (commit hash)
  KINOKO_STORAGE_DSN  SQLite database path`,
	RunE:         runIndex,
	SilenceUsage: true,
}

var (
	indexRepo   string
	indexRev    string
	indexDSN    string
	indexAPIURL string
)

func init() {
	indexCmd.Flags().StringVar(&indexRepo, "repo", "", "Repo name (default: $KINOKO_REPO)")
	indexCmd.Flags().StringVar(&indexRev, "rev", "", "Git revision (default: $KINOKO_REV)")
	indexCmd.Flags().StringVar(&indexDSN, "dsn", "", "SQLite DSN (default: $KINOKO_STORAGE_DSN)")
	indexCmd.Flags().StringVar(&indexAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
}

func runIndex(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	repo := firstNonEmpty(indexRepo, os.Getenv("KINOKO_REPO"))
	if repo == "" {
		return fmt.Errorf("repo name required: set KINOKO_REPO or --repo")
	}

	rev := firstNonEmpty(indexRev, os.Getenv("KINOKO_REV"))

	dsn := firstNonEmpty(indexDSN, os.Getenv("KINOKO_STORAGE_DSN"))
	if dsn == "" {
		home, _ := os.UserHomeDir()
		dsn = filepath.Join(home, ".kinoko", "kinoko.db")
	}

	embModel := os.Getenv("KINOKO_EMBEDDING_MODEL")
	if embModel == "" {
		embModel = "text-embedding-3-small"
	}

	store, err := storage.NewSQLiteStore(dsn, embModel)
	if err != nil {
		return fmt.Errorf("open store %s: %w", dsn, err)
	}
	defer store.Close()

	// Find repo on disk. Soft Serve stores repos in {dataDir}/repos/{name}.git.
	dataDir := os.Getenv("SOFT_SERVE_DATA_PATH")
	if dataDir == "" {
		dataDir = "data" // Soft Serve default
	}
	repoPath := filepath.Join(dataDir, "repos", repo+".git")

	// P0-3: Read SKILL.md from bare repo using git commands.
	skillPath, body, err := readSkillMDFromBareRepo(repoPath)
	if err != nil {
		return fmt.Errorf("find SKILL.md in %s: %w", repo, err)
	}

	parsed, err := skillpkg.Parse(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("parse SKILL.md: %w", err)
	}

	// Derive library and skill name from repo path.
	parts := strings.SplitN(repo, "/", 2)
	libraryID := "local"
	skillName := repo
	if len(parts) == 2 {
		libraryID = parts[0]
		skillName = parts[1]
	}

	category := model.CategoryTactical
	// P2-4: Log a warning when defaulting to CategoryTactical.
	logger.Warn("defaulting skill category to tactical", "repo", repo, "skill", skillName)

	skill := &model.SkillRecord{
		ID:          fmt.Sprintf("%s/%s/v%d", libraryID, skillName, parsed.Version),
		Name:        skillName,
		Version:     parsed.Version,
		LibraryID:   libraryID,
		Category:    category,
		Patterns:    parsed.Tags,
		ExtractedBy: "kinoko-index",
		FilePath:    skillPath,
		DecayScore:  1.0,
	}

	// Compute embedding via server API endpoint.
	var emb []float32
	apiURL := firstNonEmpty(indexAPIURL, os.Getenv("KINOKO_API_URL"), "http://127.0.0.1:23233")
	emb, err = fetchEmbedding(sharedHTTPClient, apiURL, string(body))
	if err != nil {
		logger.Warn("embedding failed, indexing without", "repo", repo, "error", err)
		emb = nil
	}

	indexer := storage.NewSQLiteIndexer(store)
	if err := indexer.IndexSkill(cmd.Context(), skill, emb); err != nil {
		return fmt.Errorf("index skill: %w", err)
	}

	logger.Info("skill indexed", "repo", repo, "rev", rev, "skill", skillName, "version", parsed.Version)
	return nil
}

// versionDirPattern matches version directories like "v1", "v2", etc.
var versionDirPattern = regexp.MustCompile(`^v(\d+)$`)

// readSkillMDFromBareRepo reads SKILL.md from a bare git repo using git commands.
// It looks for versioned paths (vN/SKILL.md) first, then falls back to root SKILL.md.
// Returns the in-repo path and file contents.
func readSkillMDFromBareRepo(repoPath string) (string, []byte, error) {
	// List all files in HEAD.
	cmd := exec.Command("git", "ls-tree", "HEAD", "-r", "--name-only")
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("git ls-tree: %w", err)
	}

	// Find all SKILL.md files.
	var skillPaths []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.HasSuffix(line, "SKILL.md") {
			skillPaths = append(skillPaths, line)
		}
	}

	if len(skillPaths) == 0 {
		return "", nil, fmt.Errorf("no SKILL.md found in %s", repoPath)
	}

	// Sort: prefer versioned paths (highest version first), then root.
	sort.Slice(skillPaths, func(i, j int) bool {
		vi := extractVersion(skillPaths[i])
		vj := extractVersion(skillPaths[j])
		return vi > vj // higher version first
	})

	bestPath := skillPaths[0]

	// Read the file content using git show.
	showCmd := exec.Command("git", "show", "HEAD:"+bestPath) //nolint:gosec // controlled input
	showCmd.Dir = repoPath
	body, err := showCmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("git show HEAD:%s: %w", bestPath, err)
	}

	return bestPath, body, nil
}

// extractVersion extracts the version number from a path like "v3/SKILL.md".
// Returns 0 for root SKILL.md or unrecognized patterns.
func extractVersion(path string) int {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return 0
	}
	m := versionDirPattern.FindStringSubmatch(parts[0])
	if m == nil {
		return 0
	}
	var v int
	_, _ = fmt.Sscanf(m[1], "%d", &v)
	return v
}

// sharedHTTPClient is reused across embedding calls to avoid per-call connection overhead.
var sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}

// fetchEmbedding calls the server's /api/v1/embed endpoint.
func fetchEmbedding(client *http.Client, apiURL, text string) ([]float32, error) {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	resp, err := client.Post(apiURL+"/api/v1/embed", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	return result.Vector, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
