package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <file.md>",
	Short: "Import a markdown file as a skill through the quality critic",
	Long: `Reads a markdown file, runs it through the Stage 3 LLM critic for quality
evaluation, and if approved, indexes and pushes to the skill library.

The critic applies the same quality gate as extraction: substitution test,
hard reject triggers, and SKILL.md generation. Raw input is transformed into
a well-structured skill.

  kinoko ingest CLAUDE.md
  kinoko ingest my-notes.md --category LEARN --tags go,testing
  kinoko ingest SKILL.md --force    # skip critic, validate and push as-is`,
	Args: cobra.ExactArgs(1),
	RunE: runIngest,
}

var (
	ingestName       string
	ingestCategory   string
	ingestLibrary    string
	ingestTags       string
	ingestAPIURL     string
	ingestDryRun     bool
	ingestForce      bool
	ingestConfigPath string
)

func init() {
	ingestCmd.Flags().StringVar(&ingestName, "name", "", "Skill name (kebab-case; default: derived from filename or LLM)")
	ingestCmd.Flags().StringVar(&ingestCategory, "category", "", "Skill category (overrides LLM/front matter)")
	ingestCmd.Flags().StringVar(&ingestLibrary, "library", "local", "Library ID")
	ingestCmd.Flags().StringVar(&ingestTags, "tags", "", "Comma-separated tags (overrides LLM/front matter)")
	ingestCmd.Flags().StringVar(&ingestAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
	ingestCmd.Flags().BoolVar(&ingestDryRun, "dry-run", false, "Evaluate only, don't push to git")
	ingestCmd.Flags().BoolVar(&ingestForce, "force", false, "Skip critic, validate structure and push as-is")
	ingestCmd.Flags().StringVar(&ingestConfigPath, "config", "", "Config file path")
}

func runIngest(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	body, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	cfg, err := config.Load(ingestConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	apiURL := firstNonEmpty(ingestAPIURL, os.Getenv("KINOKO_API_URL"), "http://127.0.0.1:23233")

	var skillBody []byte
	var skillName, skillCategory string
	var skillTags []string
	var skillVersion int
	var verdict string

	if ingestForce {
		// --force: skip critic, validate structure, normalize.
		verdict = "force"
		skillBody = body
		skillVersion = 1

		// Parse front matter if present.
		if strings.HasPrefix(strings.TrimSpace(string(body)), "---") {
			parsedName, parsedVersion, parsedCategory, parsedTags, parseErr := extraction.ParseGeneratedSkillMD(string(body))
			if parseErr == nil {
				skillName = parsedName
				skillVersion = parsedVersion
				skillCategory = parsedCategory
				skillTags = parsedTags
			} else {
				logger.Warn("front matter parse failed", "error", parseErr)
			}
		}
	} else {
		// Run through Stage 3 critic.
		llmAPIKey := cfg.LLM.APIKey
		if llmAPIKey == "" {
			llmAPIKey = os.Getenv("KINOKO_LLM_API_KEY")
		}
		if llmAPIKey == "" {
			llmAPIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if llmAPIKey == "" {
			llmAPIKey = os.Getenv("OPENAI_API_KEY")
		}
		if llmAPIKey == "" {
			return fmt.Errorf("LLM API key required: set KINOKO_LLM_API_KEY, ANTHROPIC_API_KEY, or OPENAI_API_KEY")
		}

		llmModel := cfg.LLM.Model
		if llmModel == "" {
			llmModel = "gpt-4o-mini"
		}
		llmClient, err := llm.NewClient(cfg.LLM.Provider, llmAPIKey, llmModel, cfg.LLM.BaseURL)
		if err != nil {
			return fmt.Errorf("create LLM client: %w", err)
		}

		stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

		// Create a synthetic session record for the critic.
		session := model.SessionRecord{
			ID:        uuid.Must(uuid.NewV7()).String(),
			LibraryID: ingestLibrary,
			StartedAt: time.Now(),
			EndedAt:   time.Now(),
		}

		// Run critic with nil Stage2 result (we're skipping Stage 1+2).
		result, err := stage3.Evaluate(cmd.Context(), session, body, nil)
		if err != nil {
			return fmt.Errorf("critic evaluation failed: %w", err)
		}

		if !result.Passed {
			fmt.Println("─── Ingest Rejected ───")
			fmt.Printf("  File:    %s\n", filePath)
			fmt.Printf("  Reason:  %s\n", result.CriticReasoning)
			fmt.Println("───────────────────────")
			return &exitError{code: 2, msg: "rejected by critic"}
		}

		verdict = "extract"

		// Use LLM-generated SKILL.md if available.
		if result.SkillMD != "" {
			skillBody = []byte(result.SkillMD)
			// Parse the generated SKILL.md for metadata.
			parsedName, parsedVersion, parsedCategory, parsedTags, parseErr := extraction.ParseGeneratedSkillMD(result.SkillMD)
			if parseErr == nil {
				skillName = parsedName
				skillVersion = parsedVersion
				skillCategory = parsedCategory
				skillTags = parsedTags
			}
		} else {
			// Critic approved but didn't generate SKILL.md — use original body.
			skillBody = body
			skillVersion = 1
		}
	}

	// CLI flags override everything.
	if ingestName != "" {
		skillName = ingestName
	}
	if skillName == "" {
		base := filepath.Base(filePath)
		ext := filepath.Ext(base)
		skillName = strings.TrimSuffix(base, ext)
		skillName = strings.ToLower(skillName)
		skillName = strings.ReplaceAll(skillName, " ", "-")
		skillName = strings.ReplaceAll(skillName, "_", "-")
	}
	if ingestCategory != "" {
		skillCategory = ingestCategory
	}
	if skillCategory == "" {
		skillCategory = string(model.CategoryTactical)
	}
	if ingestTags != "" {
		skillTags = nil
		for _, t := range strings.Split(ingestTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				skillTags = append(skillTags, t)
			}
		}
	}

	// Open store.
	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Compute embedding via server API.
	emb, embErr := fetchEmbedding(apiURL, string(skillBody))
	if embErr != nil {
		logger.Warn("embedding failed, indexing without vector", "error", embErr)
		emb = nil
	}

	now := time.Now()
	skill := &model.SkillRecord{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Name:        skillName,
		Version:     skillVersion,
		LibraryID:   ingestLibrary,
		Category:    model.SkillCategory(skillCategory),
		Patterns:    skillTags,
		ExtractedBy: "cli-ingest",
		FilePath:    fmt.Sprintf("skills/%s/v%d/SKILL.md", skillName, skillVersion),
		DecayScore:  1.0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	indexer := storage.NewSQLiteIndexer(store)
	if err := indexer.IndexSkill(cmd.Context(), skill, emb); err != nil {
		return fmt.Errorf("index skill: %w", err)
	}

	// Push to git if not dry-run.
	pushed := false
	if !ingestDryRun {
		serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		home, _ := os.UserHomeDir()
		keyPath := filepath.Join(home, ".kinoko", "id_ed25519")
		pusher, pushErr := extraction.NewGitPusher(serverAddr, keyPath, logger)
		if pushErr != nil {
			logger.Warn("git pusher unavailable, skill not pushed", "error", pushErr)
		} else {
			if err := pusher.Push(cmd.Context(), skillName, ingestLibrary, skillBody); err != nil {
				logger.Error("push failed", "error", err)
			} else {
				pushed = true
			}
		}
	}

	// Novelty check (informational — we push anyway for ingest).
	threshold := cfg.Embedding.GetNoveltyThreshold()
	noveltyClient := extraction.NewNoveltyClient(apiURL, threshold, logger)
	noveltyResult, _ := noveltyClient.Check(cmd.Context(), string(skillBody))

	// Print summary.
	fmt.Println("─── Ingest Summary ───")
	fmt.Printf("  File:     %s\n", filePath)
	fmt.Printf("  Verdict:  %s\n", verdict)
	fmt.Printf("  Skill:    %s\n", skillName)
	fmt.Printf("  Category: %s\n", skillCategory)
	fmt.Printf("  Library:  %s\n", ingestLibrary)
	fmt.Printf("  Version:  %d\n", skillVersion)
	if len(skillTags) > 0 {
		fmt.Printf("  Tags:     %s\n", strings.Join(skillTags, ", "))
	}
	fmt.Printf("  Indexed:  yes\n")
	if emb != nil {
		fmt.Printf("  Embedded: yes (%d dims)\n", len(emb))
	} else {
		fmt.Printf("  Embedded: no\n")
	}
	if noveltyResult != nil {
		fmt.Printf("  Novel:    %v (score: %.3f)\n", noveltyResult.Novel, noveltyResult.Score)
	}
	if ingestDryRun {
		fmt.Println("  Pushed:   no (dry-run)")
	} else if pushed {
		fmt.Println("  Pushed:   yes")
	} else {
		fmt.Println("  Pushed:   no")
	}
	fmt.Println("──────────────────────")

	return nil
}
