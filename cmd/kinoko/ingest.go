package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/gitserver"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
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
	ingestTimeout    time.Duration
)

// skillNameRe matches valid skill name characters.
var skillNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func init() {
	ingestCmd.Flags().StringVar(&ingestName, "name", "", "Skill name (kebab-case; default: derived from filename or LLM)")
	ingestCmd.Flags().StringVar(&ingestCategory, "category", "", "Skill category (overrides LLM/front matter)")
	ingestCmd.Flags().StringVar(&ingestLibrary, "library", "local", "Library ID")
	ingestCmd.Flags().StringVar(&ingestTags, "tags", "", "Comma-separated tags (overrides LLM/front matter)")
	ingestCmd.Flags().StringVar(&ingestAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
	ingestCmd.Flags().BoolVar(&ingestDryRun, "dry-run", false, "Evaluate only, don't push to git")
	ingestCmd.Flags().BoolVar(&ingestForce, "force", false, "Skip critic, validate structure and push as-is")
	ingestCmd.Flags().StringVar(&ingestConfigPath, "config", "", "Config file path")
	ingestCmd.Flags().DurationVar(&ingestTimeout, "timeout", 5*time.Minute, "Command timeout")
}

// sanitizeSkillName strips characters not in [a-zA-Z0-9_-] and rejects empty results.
func sanitizeSkillName(name string) (string, error) {
	clean := skillNameRe.ReplaceAllString(name, "")
	if clean == "" {
		return "", fmt.Errorf("skill name %q is empty after sanitization", name)
	}
	return strings.ToLower(clean), nil
}

func runIngest(cmd *cobra.Command, args []string) error {
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, ingestTimeout)
	defer cancel()

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
	var sourceSessionID string

	if ingestForce {
		// --force: skip critic, but validate basic structure.
		verdict = "force"
		skillBody = body

		// Validate: non-empty, valid UTF-8, reasonable size.
		if len(body) == 0 {
			return fmt.Errorf("file is empty")
		}
		if len(body) > 1<<20 { // 1MB
			return fmt.Errorf("file too large (%d bytes, max 1MB for --force)", len(body))
		}
		if !utf8.Valid(body) {
			return fmt.Errorf("file is not valid UTF-8 (binary?)")
		}

		// --force still requires valid YAML front matter with a name field.
		if !strings.HasPrefix(strings.TrimSpace(string(body)), "---") {
			return fmt.Errorf("--force requires valid YAML front matter (file must start with ---)")
		}
		parsedName, parsedVersion, parsedCategory, parsedTags, parseErr := extraction.ParseGeneratedSkillMD(string(body))
		if parseErr != nil {
			return fmt.Errorf("--force requires valid front matter: %w", parseErr)
		}
		if parsedName == "" {
			return fmt.Errorf("--force requires 'name' field in front matter")
		}
		skillName = parsedName
		skillVersion = parsedVersion
		skillCategory = parsedCategory
		skillTags = parsedTags
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
		sourceSessionID = session.ID

		// Run critic with nil Stage2 result (we're skipping Stage 1+2).
		result, err := stage3.Evaluate(ctx, session, body, nil)
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
	}

	// P1-4: Sanitize skill name to prevent path traversal.
	var sanitizeErr error
	skillName, sanitizeErr = sanitizeSkillName(skillName)
	if sanitizeErr != nil {
		return sanitizeErr
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

	now := time.Now()
	skill := &model.SkillRecord{
		ID:              uuid.Must(uuid.NewV7()).String(),
		Name:            skillName,
		Version:         skillVersion,
		LibraryID:       ingestLibrary,
		Category:        model.SkillCategory(skillCategory),
		Patterns:        skillTags,
		SourceSessionID: sourceSessionID,
		ExtractedBy:     "cli-ingest",
		FilePath:        fmt.Sprintf("skills/%s/v%d/SKILL.md", skillName, skillVersion),
		// Default mid-range scores for --force bypass (skips critic evaluation)
		Quality: model.QualityScores{
			ProblemSpecificity:    3,
			SolutionCompleteness:  3,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     3,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        0.6,
			CriticConfidence:      0.5,
		},
		DecayScore: 1.0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Novelty check BEFORE commit — gate duplicates.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	noveltyClient := extraction.NewNoveltyClient(apiURL, threshold, logger)
	noveltyResult, noveltyErr := noveltyClient.Check(ctx, string(skillBody))

	isNovel := true // fail-open
	if noveltyErr != nil {
		logger.Warn("novelty check failed, treating as novel", "error", noveltyErr)
	} else if noveltyResult != nil && !noveltyResult.Novel {
		isNovel = false
		logger.Warn("skill is not novel, skipping commit",
			"score", noveltyResult.Score,
			"similar", len(noveltyResult.Similar))
	}

	// Commit to git if novel and not dry-run. The post-receive hook handles SQLite indexing.
	committed := false
	var commitHash string
	if !ingestDryRun && isNovel {
		server, srvErr := gitserver.NewServer(cfg)
		if srvErr != nil {
			return fmt.Errorf("create git server: %w", srvErr)
		}
		committer := gitserver.NewGitCommitter(gitserver.GitCommitterConfig{
			Server:  server,
			DataDir: cfg.Server.DataDir,
			Logger:  logger,
		})
		hash, commitErr := committer.CommitSkill(ctx, ingestLibrary, skill, skillBody)
		if commitErr != nil {
			logger.Error("git commit failed", "error", commitErr)
		} else {
			committed = true
			commitHash = hash
		}
	}

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
	if noveltyResult != nil {
		fmt.Printf("  Novel:    %v (score: %.3f)\n", noveltyResult.Novel, noveltyResult.Score)
	}
	switch {
	case ingestDryRun:
		fmt.Println("  Committed: no (dry-run)")
	case !isNovel:
		fmt.Println("  Committed: no (duplicate)")
	case committed:
		fmt.Printf("  Committed: yes (%s)\n", commitHash)
	default:
		fmt.Println("  Committed: no")
	}
	fmt.Println("──────────────────────")

	return nil
}
