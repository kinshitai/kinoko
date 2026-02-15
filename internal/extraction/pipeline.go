package extraction

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// SkillWriter persists a skill record and its SKILL.md body.
type SkillWriter interface {
	Put(ctx context.Context, skill *SkillRecord, body []byte) error
}

// HumanReviewWriter writes extraction results selected for human review.
type HumanReviewWriter interface {
	InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error
}

// RandIntn returns a random int in [0, n). Injectable for testing.
type RandIntn func(n int) int

// Pipeline implements Extractor by wiring Stage1 → Stage2 → Stage3 → SkillWriter.
type Pipeline struct {
	stage1     Stage1Filter
	stage2     Stage2Scorer
	stage3     Stage3Critic
	writer     SkillWriter
	reviewer   HumanReviewWriter
	log        *slog.Logger
	sampleRate float64 // 0.0–1.0, e.g. 0.01 for 1%
	randIntn   RandIntn
	extractor  string // pipeline version identifier
}

// PipelineConfig holds constructor parameters for Pipeline.
type PipelineConfig struct {
	Stage1     Stage1Filter
	Stage2     Stage2Scorer
	Stage3     Stage3Critic
	Writer     SkillWriter
	Reviewer   HumanReviewWriter
	Log        *slog.Logger
	SampleRate float64
	RandIntn   RandIntn
	Extractor  string
}

// NewPipeline creates a Pipeline. If RandIntn is nil, crypto/rand is used.
func NewPipeline(cfg PipelineConfig) *Pipeline {
	r := cfg.RandIntn
	if r == nil {
		r = cryptoRandIntn
	}
	ext := cfg.Extractor
	if ext == "" {
		ext = "pipeline-v1"
	}
	return &Pipeline{
		stage1:     cfg.Stage1,
		stage2:     cfg.Stage2,
		stage3:     cfg.Stage3,
		writer:     cfg.Writer,
		reviewer:   cfg.Reviewer,
		log:        cfg.Log,
		sampleRate: cfg.SampleRate,
		randIntn:   r,
		extractor:  ext,
	}
}

func cryptoRandIntn(n int) int {
	v, _ := rand.Int(rand.Reader, big.NewInt(int64(n)))
	return int(v.Int64())
}

// Extract runs the full extraction pipeline on a session.
func (p *Pipeline) Extract(ctx context.Context, session SessionRecord, content []byte) (*ExtractionResult, error) {
	start := time.Now()
	result := &ExtractionResult{
		SessionID:   session.ID,
		ProcessedAt: start,
	}

	p.log.Info("pipeline start", "session_id", session.ID)

	// Stage 1
	p.log.Info("stage1 entry", "session_id", session.ID)
	s1Start := time.Now()
	s1 := p.stage1.Filter(session)
	s1Ms := time.Since(s1Start).Milliseconds()
	result.Stage1 = s1

	if !s1.Passed {
		result.Status = StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 1,
			"reason", s1.Reason,
			"stage1_ms", s1Ms,
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	p.log.Info("stage1 pass", "session_id", session.ID, "stage1_ms", s1Ms)

	// Stage 2
	p.log.Info("stage2 entry", "session_id", session.ID)
	s2Start := time.Now()
	s2, err := p.stage2.Score(ctx, session, content)
	s2Ms := time.Since(s2Start).Milliseconds()
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("stage2: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 2,
			"error", err,
			"stage2_ms", s2Ms,
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	result.Stage2 = s2

	if !s2.Passed {
		result.Status = StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 2,
			"reason", s2.Reason,
			"stage2_ms", s2Ms,
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	p.log.Info("stage2 pass", "session_id", session.ID, "stage2_ms", s2Ms)

	// Stage 3
	p.log.Info("stage3 entry", "session_id", session.ID)
	s3Start := time.Now()
	s3, err := p.stage3.Evaluate(ctx, session, content, s2)
	s3Ms := time.Since(s3Start).Milliseconds()
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("stage3: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 3,
			"error", err,
			"stage3_ms", s3Ms,
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	result.Stage3 = s3

	if !s3.Passed {
		result.Status = StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 3,
			"reason", s3.CriticReasoning,
			"stage3_ms", s3Ms,
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	p.log.Info("stage3 pass", "session_id", session.ID, "stage3_ms", s3Ms)

	// Build skill and persist
	skillName := generateSkillName(content)
	skillID := fmt.Sprintf("skill-%s-%d", skillName, start.UnixMilli())

	skill := &SkillRecord{
		ID:              skillID,
		Name:            skillName,
		Version:         1,
		LibraryID:       session.LibraryID,
		Category:        s2.ClassifiedCategory,
		Patterns:        s2.ClassifiedPatterns,
		Quality:         s3.RefinedScores,
		SourceSessionID: session.ID,
		ExtractedBy:     p.extractor,
		FilePath:        fmt.Sprintf("skills/%s/v1/SKILL.md", skillName),
	}

	body := buildSkillMD(skill, content)

	storeStart := time.Now()
	if err := p.writer.Put(ctx, skill, body); err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("store: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", "store",
			"error", err,
			"store_ms", time.Since(storeStart).Milliseconds(),
			"total_ms", result.DurationMs,
		)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	storeMs := time.Since(storeStart).Milliseconds()

	result.Status = StatusExtracted
	result.Skill = skill
	result.DurationMs = time.Since(start).Milliseconds()

	p.log.Info("pipeline extracted",
		"session_id", session.ID,
		"skill_id", skillID,
		"skill_name", skillName,
		"store_ms", storeMs,
		"total_ms", result.DurationMs,
	)

	p.maybeSample(ctx, session.ID, result)
	return result, nil
}

// maybeSample writes to human_review_samples with configured probability.
func (p *Pipeline) maybeSample(ctx context.Context, sessionID string, result *ExtractionResult) {
	if p.reviewer == nil || p.sampleRate <= 0 {
		return
	}

	// Roll dice: sample if rand < sampleRate * 10000
	threshold := int(p.sampleRate * 10000)
	if threshold <= 0 {
		return
	}
	roll := p.randIntn(10000)
	if roll >= threshold {
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		p.log.Warn("sample marshal error", "session_id", sessionID, "error", err)
		return
	}

	if err := p.reviewer.InsertReviewSample(ctx, sessionID, data); err != nil {
		p.log.Warn("sample insert error", "session_id", sessionID, "error", err)
		return
	}

	p.log.Info("human review sampled", "session_id", sessionID, "status", result.Status)
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// generateSkillName extracts a kebab-case name from content.
func generateSkillName(content []byte) string {
	// Take first line or first 80 chars of meaningful text.
	s := string(content)
	s = strings.TrimSpace(s)

	// Try first non-empty line.
	for _, line := range strings.SplitN(s, "\n", 10) {
		line = strings.TrimSpace(line)
		// Skip markdown headers markers, timestamps, empty lines.
		cleaned := strings.TrimLeft(line, "# ")
		cleaned = strings.TrimSpace(cleaned)
		if len(cleaned) > 5 {
			s = cleaned
			break
		}
	}

	// Truncate to reasonable length.
	if len(s) > 60 {
		s = s[:60]
	}

	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	s = strings.TrimSpace(s)
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if s == "" {
		s = "unnamed-skill"
	}

	// Cap at 50 chars.
	if len(s) > 50 {
		s = s[:50]
		s = strings.TrimRight(s, "-")
	}

	return s
}

func buildSkillMD(skill *SkillRecord, content []byte) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", skill.Name)
	fmt.Fprintf(&b, "category: %s\n", skill.Category)
	if len(skill.Patterns) > 0 {
		fmt.Fprintf(&b, "patterns:\n")
		for _, p := range skill.Patterns {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}
	fmt.Fprintf(&b, "source_session: %s\n", skill.SourceSessionID)
	fmt.Fprintf(&b, "---\n\n")

	// Include a summary from content (first 2000 bytes).
	summary := string(content)
	if len(summary) > 2000 {
		summary = summary[:2000] + "\n\n[truncated]"
	}
	b.WriteString(summary)
	b.WriteString("\n")

	return []byte(b.String())
}
