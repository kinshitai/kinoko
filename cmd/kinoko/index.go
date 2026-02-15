package main

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kinoko-dev/kinoko/internal/embedding"
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
	indexRepo string
	indexRev  string
	indexDSN  string
)

func init() {
	indexCmd.Flags().StringVar(&indexRepo, "repo", "", "Repo name (default: $KINOKO_REPO)")
	indexCmd.Flags().StringVar(&indexRev, "rev", "", "Git revision (default: $KINOKO_REV)")
	indexCmd.Flags().StringVar(&indexDSN, "dsn", "", "SQLite DSN (default: $KINOKO_STORAGE_DSN)")
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

	// Find latest SKILL.md by walking version directories.
	skillPath, err := findLatestSkillMD(repoPath)
	if err != nil {
		return fmt.Errorf("find SKILL.md in %s: %w", repo, err)
	}

	parsed, err := skillpkg.ParseFile(skillPath)
	if err != nil {
		return fmt.Errorf("parse SKILL.md: %w", err)
	}

	body, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}

	// Derive library and skill name from repo path.
	parts := strings.SplitN(repo, "/", 2)
	libraryID := "local"
	skillName := repo
	if len(parts) == 2 {
		libraryID = parts[0]
		skillName = parts[1]
	}

	skill := &model.SkillRecord{
		ID:              fmt.Sprintf("%s/%s/v%d", libraryID, skillName, parsed.Version),
		Name:            skillName,
		Version:         parsed.Version,
		LibraryID:       libraryID,
		Category:        model.CategoryTactical, // Default; could be parsed from SKILL.md metadata.
		Patterns:        parsed.Tags,
		ExtractedBy:     "kinoko-index",
		FilePath:        fmt.Sprintf("skills/%s/v%d/SKILL.md", skillName, parsed.Version),
		DecayScore:      1.0,
	}

	// Compute embedding if API key available.
	var emb []float32
	apiKey := os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey != "" {
		embCfg := embedding.DefaultConfig()
		embCfg.APIKey = apiKey
		embedder := embedding.New(embCfg, logger)
		emb, err = embedder.Embed(cmd.Context(), string(body))
		if err != nil {
			logger.Warn("embedding failed, indexing without", "repo", repo, "error", err)
		}
	}

	indexer := storage.NewSQLiteIndexer(store)
	if err := indexer.IndexSkill(cmd.Context(), skill, emb); err != nil {
		return fmt.Errorf("index skill: %w", err)
	}

	logger.Info("skill indexed", "repo", repo, "rev", rev, "skill", skillName, "version", parsed.Version)
	return nil
}

// findLatestSkillMD walks a bare git repo's worktree or a regular directory
// for version directories and returns the path to the latest SKILL.md.
// For bare repos, we look in the git tree directly; for checked-out repos,
// we walk the filesystem.
func findLatestSkillMD(repoPath string) (string, error) {
	// For non-bare repos (workdirs), walk filesystem.
	var found []string
	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.Name() == "SKILL.md" && !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no SKILL.md found in %s", repoPath)
	}

	// Sort descending to get the latest version directory first.
	sort.Sort(sort.Reverse(sort.StringSlice(found)))
	return found[0], nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
