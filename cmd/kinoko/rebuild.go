package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
	skillpkg "github.com/kinoko-dev/kinoko/pkg/skill"
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild SQLite cache from git repos",
	Long: `Scans the repos directory for bare git repositories containing SKILL.md files,
parses and indexes each one into SQLite. Use --clean to drop existing skill data first.`,
	RunE:         runRebuild,
	SilenceUsage: true,
}

var (
	rebuildClean   bool
	rebuildLibrary string
	rebuildDSN     string
	rebuildAPIURL  string
	rebuildDataDir string
)

func init() {
	rebuildCmd.Flags().BoolVar(&rebuildClean, "clean", false, "Drop skill tables before rebuilding")
	rebuildCmd.Flags().StringVar(&rebuildLibrary, "library", "", "Only rebuild repos under this library prefix")
	rebuildCmd.Flags().StringVar(&rebuildDSN, "dsn", "", "SQLite DSN (default: $KINOKO_STORAGE_DSN)")
	rebuildCmd.Flags().StringVar(&rebuildAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
	rebuildCmd.Flags().StringVar(&rebuildDataDir, "data-dir", "", "Soft Serve data directory (default: $SOFT_SERVE_DATA_PATH or ~/.kinoko/data)")
}

func runRebuild(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	dsn := firstNonEmpty(rebuildDSN, os.Getenv("KINOKO_STORAGE_DSN"))
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

	if rebuildClean {
		db := store.DB()
		for _, table := range []string{"skill_embeddings", "skill_patterns", "skills"} {
			if _, err := db.ExecContext(cmd.Context(), "DELETE FROM "+table); err != nil { //nolint:gosec // table names are hardcoded constants
				return fmt.Errorf("clean table %s: %w", table, err)
			}
		}
		logger.Info("cleaned skill tables")
	}

	dataDir := firstNonEmpty(rebuildDataDir, os.Getenv("SOFT_SERVE_DATA_PATH"))
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".kinoko", "data")
	}

	reposRoot := filepath.Join(dataDir, "repos")
	cleanReposRoot := filepath.Clean(reposRoot)

	// Discover bare repos.
	var repos []struct {
		name string
		path string
	}

	err = filepath.Walk(reposRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if !info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".git") {
			return nil
		}
		// Verify it's a bare repo by checking for HEAD file.
		if _, statErr := os.Stat(filepath.Join(path, "HEAD")); statErr != nil {
			return nil
		}

		// Derive repo name: strip reposRoot prefix and .git suffix.
		rel, relErr := filepath.Rel(cleanReposRoot, path)
		if relErr != nil {
			return nil
		}
		repoName := strings.TrimSuffix(rel, ".git")

		// Path traversal validation.
		if strings.Contains(repoName, "..") {
			return nil
		}

		// Library filter.
		if rebuildLibrary != "" {
			parts := strings.SplitN(repoName, "/", 2)
			if len(parts) < 2 || parts[0] != rebuildLibrary {
				return nil
			}
		}

		repos = append(repos, struct {
			name string
			path string
		}{name: repoName, path: path})

		return filepath.SkipDir // don't descend into .git dirs
	})
	if err != nil {
		return fmt.Errorf("walk repos: %w", err)
	}

	total := len(repos)
	indexed := 0
	errors := 0

	apiURL := firstNonEmpty(rebuildAPIURL, os.Getenv("KINOKO_API_URL"), "http://127.0.0.1:23233")
	indexer := storage.NewSQLiteIndexer(store)

	for i, repo := range repos {
		fmt.Fprintf(cmd.OutOrStderr(), "Indexed %d/%d skills...\r", i, total)

		skillPath, body, readErr := readSkillMDFromBareRepo(repo.path)
		if readErr != nil {
			logger.Warn("skipping repo (no SKILL.md)", "repo", repo.name, "error", readErr)
			errors++
			continue
		}

		parsed, parseErr := skillpkg.Parse(bytes.NewReader(body))
		if parseErr != nil {
			logger.Warn("skipping repo (parse error)", "repo", repo.name, "error", parseErr)
			errors++
			continue
		}

		parts := strings.SplitN(repo.name, "/", 2)
		libraryID := "local"
		skillName := repo.name
		if len(parts) == 2 {
			libraryID = parts[0]
			skillName = parts[1]
		}

		category := model.CategoryTactical
		if parsed.Category != "" {
			switch model.SkillCategory(parsed.Category) {
			case model.CategoryFoundational, model.CategoryTactical, model.CategoryContextual:
				category = model.SkillCategory(parsed.Category)
			}
		}

		skill := &model.SkillRecord{
			ID:          fmt.Sprintf("%s/%s/v%d", libraryID, skillName, parsed.Version),
			Name:        skillName,
			Version:     parsed.Version,
			LibraryID:   libraryID,
			Category:    category,
			Patterns:    parsed.Tags,
			ExtractedBy: "kinoko-rebuild",
			FilePath:    skillPath,
			DecayScore:  1.0,
		}

		if parsed.Quality != nil {
			if err := validateQualityScores(parsed.Quality); err != nil {
				logger.Warn("skipping repo (bad quality scores)", "repo", repo.name, "error", err)
				errors++
				continue
			}
			skill.Quality = model.QualityScores{
				ProblemSpecificity:    parsed.Quality.ProblemSpecificity,
				SolutionCompleteness:  parsed.Quality.SolutionCompleteness,
				ContextPortability:    parsed.Quality.ContextPortability,
				ReasoningTransparency: parsed.Quality.ReasoningTransparency,
				TechnicalAccuracy:     parsed.Quality.TechnicalAccuracy,
				VerificationEvidence:  parsed.Quality.VerificationEvidence,
				InnovationLevel:       parsed.Quality.InnovationLevel,
				CompositeScore:        parsed.Quality.CompositeScore,
				CriticConfidence:      parsed.Quality.CriticConfidence,
			}
		}

		var emb []float32
		emb, embErr := fetchEmbedding(sharedHTTPClient, apiURL, string(body))
		if embErr != nil {
			logger.Warn("embedding failed, indexing without", "repo", repo.name, "error", embErr)
			emb = nil
		}

		if idxErr := indexer.IndexSkill(cmd.Context(), skill, emb); idxErr != nil {
			logger.Warn("failed to index skill", "repo", repo.name, "error", idxErr)
			errors++
			continue
		}

		indexed++
	}

	fmt.Fprintf(cmd.OutOrStderr(), "\nRebuild complete: %d skills indexed, %d errors\n", indexed, errors)
	return nil
}
