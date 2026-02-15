package extraction

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Compile-time interface check.
var _ Extractor = (*Pipeline)(nil)

// SkillWriter persists a skill record and its SKILL.md body.
type SkillWriter interface {
	Put(ctx context.Context, skill *SkillRecord, body []byte) error
}

// SessionWriter persists and updates session records.
type SessionWriter interface {
	InsertSession(ctx context.Context, session *SessionRecord) error
	UpdateSessionResult(ctx context.Context, session *SessionRecord) error
}

// HumanReviewWriter writes extraction results selected for human review.
type HumanReviewWriter interface {
	InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error
}

// SkillEmbedder computes an embedding for skill content.
type SkillEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// RandIntn returns a random int in [0, n). Injectable for testing.
type RandIntn func(n int) int

// Pipeline implements Extractor by wiring Stage1 → Stage2 → Stage3 → SkillWriter.
type Pipeline struct {
	stage1     Stage1Filter
	stage2     Stage2Scorer
	stage3     Stage3Critic
	writer     SkillWriter
	sessions   SessionWriter
	embedder   SkillEmbedder
	reviewer   HumanReviewWriter
	log        *slog.Logger
	sampleRate float64 // 0.0–1.0, e.g. 0.01 for 1%
	randIntn   RandIntn
	extractor  string // pipeline version identifier

	// Stratified sampling counters: maintain ~50/50 extracted vs rejected.
	extractedSamples int
	rejectedSamples  int
}

// PipelineConfig holds constructor parameters for Pipeline.
type PipelineConfig struct {
	Stage1     Stage1Filter
	Stage2     Stage2Scorer
	Stage3     Stage3Critic
	Writer     SkillWriter
	Sessions   SessionWriter
	Embedder   SkillEmbedder
	Reviewer   HumanReviewWriter
	Log        *slog.Logger
	SampleRate float64
	RandIntn   RandIntn
	Extractor  string
}

// NewPipeline creates a Pipeline. If RandIntn is nil, crypto/rand is used.
// Returns an error if required dependencies (Stage1, Stage2, Stage3, Writer, Log) are nil.
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Stage1 == nil {
		return nil, fmt.Errorf("pipeline: Stage1 is required")
	}
	if cfg.Stage2 == nil {
		return nil, fmt.Errorf("pipeline: Stage2 is required")
	}
	if cfg.Stage3 == nil {
		return nil, fmt.Errorf("pipeline: Stage3 is required")
	}
	if cfg.Writer == nil {
		return nil, fmt.Errorf("pipeline: Writer is required")
	}
	if cfg.Log == nil {
		return nil, fmt.Errorf("pipeline: Log is required")
	}
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
		sessions:   cfg.Sessions,
		embedder:   cfg.Embedder,
		reviewer:   cfg.Reviewer,
		log:        cfg.Log,
		sampleRate: cfg.SampleRate,
		randIntn:   r,
		extractor:  ext,
	}, nil
}

func cryptoRandIntn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// Entropy exhaustion is catastrophic; fail loudly rather than silently biasing.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
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

	// Persist session before extraction begins.
	if p.sessions != nil {
		session.ExtractionStatus = StatusPending
		if err := p.sessions.InsertSession(ctx, &session); err != nil {
			p.log.Error("failed to insert session", "session_id", session.ID, "error", err)
			// Non-fatal: continue extraction even if session persistence fails.
		}
	}

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
		p.updateSessionStatus(ctx, &session, result)
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
		result.Error = fmt.Sprintf("stage2 [session=%s]: %v", session.ID, err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 2,
			"error", err,
			"stage2_ms", s2Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
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
		p.updateSessionStatus(ctx, &session, result)
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
		result.Error = fmt.Sprintf("stage3 [session=%s]: %v", session.ID, err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 3,
			"error", err,
			"stage3_ms", s3Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
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
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		return result, nil
	}
	p.log.Info("stage3 pass", "session_id", session.ID, "stage3_ms", s3Ms)

	// Build skill and persist
	skillName := skillNameFromClassification(s2.ClassifiedPatterns, s2.ClassifiedCategory)
	skillID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()

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
		DecayScore:      1.0, // New skills start fully active; 0.0 would hide them from injection queries.
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Compute embedding for the skill content so injection can use cosine similarity.
	if p.embedder != nil {
		emb, embErr := p.embedder.Embed(ctx, string(content))
		if embErr != nil {
			p.log.Warn("failed to compute skill embedding, storing without", "session_id", session.ID, "error", embErr)
		} else {
			skill.Embedding = emb
		}
	}

	body := buildSkillMD(skill, s3, content)

	storeStart := time.Now()
	if err := p.writer.Put(ctx, skill, body); err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("store [session=%s]: %v", session.ID, err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", "store",
			"error", err,
			"store_ms", time.Since(storeStart).Milliseconds(),
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
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

	// Update session with extraction result.
	if p.sessions != nil {
		session.ExtractionStatus = StatusExtracted
		session.ExtractedSkillID = skillID
		if err := p.sessions.UpdateSessionResult(ctx, &session); err != nil {
			p.log.Error("failed to update session result", "session_id", session.ID, "error", err)
		}
	}

	p.maybeSample(ctx, session.ID, result)
	return result, nil
}

// updateSessionStatus updates the session record with rejection/error info if sessions are being tracked.
func (p *Pipeline) updateSessionStatus(ctx context.Context, session *SessionRecord, result *ExtractionResult) {
	if p.sessions == nil {
		return
	}
	session.ExtractionStatus = result.Status
	if result.Status == StatusRejected {
		if result.Stage1 != nil && !result.Stage1.Passed {
			session.RejectedAtStage = 1
			session.RejectionReason = result.Stage1.Reason
		} else if result.Stage2 != nil && !result.Stage2.Passed {
			session.RejectedAtStage = 2
			session.RejectionReason = result.Stage2.Reason
		} else if result.Stage3 != nil && !result.Stage3.Passed {
			session.RejectedAtStage = 3
			session.RejectionReason = result.Stage3.CriticReasoning
		}
	}
	if result.Status == StatusError {
		session.RejectionReason = result.Error
	}
	if err := p.sessions.UpdateSessionResult(ctx, session); err != nil {
		p.log.Error("failed to update session result", "session_id", session.ID, "error", err)
	}
}

// maybeSample writes to human_review_samples with stratified sampling per §3.4.
// Maintains ~50/50 split between extracted and rejected pools by always sampling
// from whichever pool is underrepresented, and probabilistically from the other.
func (p *Pipeline) maybeSample(ctx context.Context, sessionID string, result *ExtractionResult) {
	if p.reviewer == nil || p.sampleRate <= 0 {
		return
	}

	isExtracted := result.Status == StatusExtracted
	pool := "rejected"
	if isExtracted {
		pool = "extracted"
	}

	// Stratified sampling: maintain ~50/50 between extracted and rejected pools.
	// Underrepresented pool: always sample.
	// Overrepresented pool: only sample at base rate.
	// Equal counts: sample at base rate.
	underrepresented := false
	overrepresented := false
	if isExtracted {
		underrepresented = p.extractedSamples < p.rejectedSamples
		overrepresented = p.extractedSamples > p.rejectedSamples
	} else {
		underrepresented = p.rejectedSamples < p.extractedSamples
		overrepresented = p.rejectedSamples > p.extractedSamples
	}

	if overrepresented {
		// Skip — let the other pool catch up.
		return
	}

	if !underrepresented {
		// Equal counts — use probabilistic sampling at base rate.
		threshold := int(p.sampleRate * 10000)
		if threshold <= 0 {
			return
		}
		roll := p.randIntn(10000)
		if roll >= threshold {
			return
		}
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

	// Update counters after successful insert.
	if isExtracted {
		p.extractedSamples++
	} else {
		p.rejectedSamples++
	}

	p.log.Info("human review sampled", "session_id", sessionID, "status", result.Status, "pool", pool,
		"extracted_samples", p.extractedSamples, "rejected_samples", p.rejectedSamples)
}

// skillNameFromClassification derives a kebab-case skill name from classified
// patterns and category. E.g. "FIX/Backend/DatabaseConnection" → "fix-backend-database-connection".
func skillNameFromClassification(patterns []string, category SkillCategory) string {
	if len(patterns) > 0 {
		// Use first pattern: split on "/" and kebab-case.
		parts := strings.Split(patterns[0], "/")
		var segments []string
		for _, p := range parts {
			seg := kebab(p)
			if seg != "" {
				segments = append(segments, seg)
			}
		}
		if name := strings.Join(segments, "-"); name != "" {
			if len(name) > 50 {
				name = name[:50]
				name = strings.TrimRight(name, "-")
			}
			return name
		}
	}

	// Fallback to category.
	if category != "" {
		return string(category) + "-skill"
	}
	return "unnamed-skill"
}

// kebab converts a CamelCase or mixed string to kebab-case.
// Handles all-caps segments: "FIX" → "fix", "DatabaseConnection" → "database-connection".
func kebab(s string) string {
	var out strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			// Insert dash before uppercase if preceded by lowercase or
			// if this is the start of a new word (upper followed by lower, after uppers).
			if i > 0 {
				prev := runes[i-1]
				prevIsLower := prev >= 'a' && prev <= 'z'
				prevIsDigit := prev >= '0' && prev <= '9'
				nextIsLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
				if prevIsLower || prevIsDigit || (prev >= 'A' && prev <= 'Z' && nextIsLower) {
					out.WriteByte('-')
				}
			}
			out.WriteRune(r + ('a' - 'A'))
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		} else if r == '-' || r == '_' || r == ' ' {
			if out.Len() > 0 {
				out.WriteByte('-')
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

// titleCase converts a space-separated string to title case without using deprecated strings.Title.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// buildSkillMD generates a proper SKILL.md with YAML front matter and structured body.
// It populates sections from the Stage 3 critic reasoning and session content.
func buildSkillMD(skill *SkillRecord, stage3 *Stage3Result, content []byte) []byte {
	var b strings.Builder

	// YAML front matter
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", skill.Name)
	fmt.Fprintf(&b, "id: %s\n", skill.ID)
	fmt.Fprintf(&b, "version: %d\n", skill.Version)
	fmt.Fprintf(&b, "category: %s\n", skill.Category)
	if len(skill.Patterns) > 0 {
		fmt.Fprintf(&b, "patterns:\n")
		for _, p := range skill.Patterns {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}
	fmt.Fprintf(&b, "extracted_by: %s\n", skill.ExtractedBy)
	fmt.Fprintf(&b, "quality: %.2f\n", skill.Quality.CompositeScore)
	fmt.Fprintf(&b, "confidence: %.2f\n", skill.Quality.CriticConfidence)
	fmt.Fprintf(&b, "source_session: %s\n", skill.SourceSessionID)
	fmt.Fprintf(&b, "created: %s\n", skill.CreatedAt.Format(time.DateOnly))
	fmt.Fprintf(&b, "---\n\n")

	// Title from name
	title := strings.ReplaceAll(skill.Name, "-", " ")
	title = titleCase(title)
	fmt.Fprintf(&b, "# %s\n\n", title)

	// Populate body from Stage 3 critic analysis and session content.
	// The critic already read the full session, so its reasoning is the
	// most distilled knowledge we have without a separate summarisation stage.

	reasoning := ""
	if stage3 != nil {
		reasoning = stage3.CriticReasoning
	}

	// When to Use — derived from patterns and category
	fmt.Fprintf(&b, "## When to Use\n\n")
	if len(skill.Patterns) > 0 {
		fmt.Fprintf(&b, "Applicable when encountering: %s\n\n", strings.Join(skill.Patterns, ", "))
	}
	fmt.Fprintf(&b, "Category: %s\n\n", skill.Category)

	// Solution — session content summary (truncated for readability)
	fmt.Fprintf(&b, "## Solution\n\n")
	if len(content) > 0 {
		// Include a meaningful excerpt: first 4KB of session content.
		excerpt := content
		if len(excerpt) > 4096 {
			excerpt = excerpt[:4096]
		}
		fmt.Fprintf(&b, "```\n%s\n```\n\n", string(excerpt))
	}

	// Why It Works — from critic reasoning
	fmt.Fprintf(&b, "## Why It Works\n\n")
	if reasoning != "" {
		fmt.Fprintf(&b, "%s\n\n", reasoning)
	}

	// Pitfalls — note if critic flagged contradictions
	fmt.Fprintf(&b, "## Pitfalls\n\n")
	if stage3 != nil && stage3.ContradictsBestPractices {
		fmt.Fprintf(&b, "**Warning:** This skill may contradict established best practices. Review carefully before applying.\n\n")
	}

	return []byte(b.String())
}
