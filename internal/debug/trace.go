// Package debug provides pipeline debug tracing. When enabled, each pipeline
// run writes a trace directory containing stage results, raw data, and a
// summary. All operations are best-effort — errors are logged but never
// propagate to the caller.
package debug

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Tracer creates per-run trace directories under baseDir.
type Tracer struct {
	baseDir string
}

// NewTracer returns a Tracer that writes to baseDir. Returns nil if baseDir is empty.
func NewTracer(baseDir string) *Tracer {
	if baseDir == "" {
		return nil
	}
	return &Tracer{baseDir: baseDir}
}

// StartRun creates a new trace directory and returns a RunTrace handle.
// If t is nil, returns nil (all RunTrace methods are nil-safe).
func (t *Tracer) StartRun() *RunTrace {
	if t == nil {
		return nil
	}

	now := time.Now().UTC()
	ts := now.Format("20060102T150405Z")

	for attempt := 0; attempt < 2; attempt++ {
		suffix, err := randomHex(4)
		if err != nil {
			slog.Error("debug: failed to generate random suffix", "error", err)
			return nil
		}
		id := fmt.Sprintf("%s-%s", ts, suffix)
		dir := filepath.Join(t.baseDir, id)

		if err := os.MkdirAll(dir, 0755); err != nil {
			if os.IsExist(err) && attempt == 0 {
				continue // retry with new suffix
			}
			slog.Error("debug: failed to create trace dir", "dir", dir, "error", err)
			return nil
		}
		return &RunTrace{TraceID: id, Dir: dir, started: now}
	}
	return nil
}

// RunTrace represents a single pipeline run's trace directory.
type RunTrace struct {
	TraceID string
	Dir     string
	started time.Time
}

// WriteSession writes the raw session content to session.log.
func (r *RunTrace) WriteSession(data []byte) error {
	if r == nil {
		return nil
	}
	return bestEffortWrite(filepath.Join(r.Dir, "session.log"), data)
}

// WriteStage writes a stage trace as indented JSON to <name>.json.
func (r *RunTrace) WriteStage(name string, data any) error {
	if r == nil {
		return nil
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("debug: marshal stage", "name", name, "error", err)
		return nil
	}
	return bestEffortWrite(filepath.Join(r.Dir, name+".json"), b)
}

// WriteRaw writes gzip-compressed data to <name>.<suffix>.json.gz.
func (r *RunTrace) WriteRaw(name string, suffix string, data []byte) error {
	if r == nil {
		return nil
	}
	path := filepath.Join(r.Dir, fmt.Sprintf("%s.%s.json.gz", name, suffix))
	f, err := os.Create(path)
	if err != nil {
		slog.Error("debug: create raw file", "path", path, "error", err)
		return nil
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(data); err != nil {
		slog.Error("debug: gzip write", "path", path, "error", err)
		return nil
	}
	if err := gz.Close(); err != nil {
		slog.Error("debug: gzip close", "path", path, "error", err)
	}
	return nil
}

// WriteSkill writes a skill body to skills/<skillName>.md.
func (r *RunTrace) WriteSkill(skillName string, content []byte) error {
	if r == nil {
		return nil
	}
	dir := filepath.Join(r.Dir, "skills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("debug: create skills dir", "error", err)
		return nil
	}
	return bestEffortWrite(filepath.Join(dir, skillName+".md"), content)
}

// WriteSummary writes the run summary as indented JSON to summary.json.
func (r *RunTrace) WriteSummary(summary any) error {
	if r == nil {
		return nil
	}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		slog.Error("debug: marshal summary", "error", err)
		return nil
	}
	return bestEffortWrite(filepath.Join(r.Dir, "summary.json"), b)
}

// Started returns the time the trace was started.
func (r *RunTrace) Started() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.started
}

func bestEffortWrite(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Error("debug: write file", "path", path, "error", err)
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}

// --- Trace data structures (exported for pipeline use) ---

// Stage1Trace captures Stage 1 filter results.
type Stage1Trace struct {
	Passed     bool                   `json:"passed"`
	Filters    map[string]FilterTrace `json:"filters"`
	DurationMs int64                  `json:"duration_ms"`
}

// FilterTrace captures a single filter's result.
type FilterTrace struct {
	Value     any  `json:"value"`
	Threshold any  `json:"threshold"`
	Passed    bool `json:"passed"`
}

// Stage2Trace captures Stage 2 scoring results.
type Stage2Trace struct {
	Passed           bool               `json:"passed"`
	EmbeddingNovelty *EmbeddingTrace    `json:"embedding_novelty,omitempty"`
	RubricScores     map[string]float64 `json:"rubric_scores,omitempty"`
	RubricAggregate  float64            `json:"rubric_aggregate"`
	RubricThreshold  float64            `json:"rubric_threshold"`
	Meta             *LLMMeta           `json:"meta,omitempty"`
	DurationMs       int64              `json:"duration_ms"`
}

// EmbeddingTrace captures embedding novelty details.
type EmbeddingTrace struct {
	Distance     float64 `json:"distance"`
	NearestSkill string  `json:"nearest_skill"`
	Threshold    float64 `json:"threshold"`
}

// LLMMeta captures LLM call metadata.
type LLMMeta struct {
	Model     string `json:"model"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	LatencyMs int64  `json:"latency_ms"`
}

// Stage3Trace captures Stage 3 critic results.
type Stage3Trace struct {
	Passed                 bool     `json:"passed"`
	Verdict                string   `json:"verdict"`
	Confidence             float64  `json:"confidence"`
	Reasoning              string   `json:"reasoning"`
	ContradictionsDetected bool     `json:"contradictions_detected"`
	Retries                int      `json:"retries"`
	CircuitBreakerState    string   `json:"circuit_breaker_state"`
	Meta                   *LLMMeta `json:"meta,omitempty"`
	DurationMs             int64    `json:"duration_ms"`
}

// Summary captures overall pipeline run results.
type Summary struct {
	TraceID         string              `json:"trace_id"`
	SessionFile     string              `json:"session_file"`
	StartedAt       time.Time           `json:"started_at"`
	FinishedAt      time.Time           `json:"finished_at"`
	DurationMs      int64               `json:"duration_ms"`
	Result          string              `json:"result"`
	RejectedAt      *string             `json:"rejected_at"`
	SkillsExtracted int                 `json:"skills_extracted"`
	Stages          map[string]StageSum `json:"stages"`
	CostEstimate    *CostEstimate       `json:"cost_estimate,omitempty"`
}

// StageSum captures per-stage summary data.
type StageSum struct {
	Passed     bool  `json:"passed"`
	DurationMs int64 `json:"duration_ms"`
}

// CostEstimate captures token usage for cost estimation.
type CostEstimate struct {
	TokensIn  int `json:"tokens_in"`
	TokensOut int `json:"tokens_out"`
}
