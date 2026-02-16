package extraction

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// --- Phase B/C/D mock types ---

type mockNoveltyChecker struct {
	result *NoveltyResult
	err    error
	called int32
	input  string
}

func (m *mockNoveltyChecker) Check(_ context.Context, content string) (*NoveltyResult, error) {
	atomic.AddInt32(&m.called, 1)
	m.input = content
	return m.result, m.err
}

// passStage3WithSkillMD returns a Stage3Result with a valid SkillMD.
func passStage3WithSkillMD() *model.Stage3Result {
	r := passStage3()
	r.SkillMD = `---
name: llm-generated-skill
version: 3
category: DEBUG
tags:
  - debugging/profiling
  - go/pprof
---

# LLM Generated Skill

## Problem
CPU profiling in production.

## Solution
Use pprof with low sampling rate.
`
	return r
}

// --- P1: Pipeline novelty check integration ---

func TestPipelineNoveltyCheck_SkipsCommitWhenNotNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: false, Score: 0.3, Similar: []SimilarSkill{{Name: "existing", Score: 0.92}}},
	}
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Novelty:   novelty,
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != model.StatusExtracted {
		t.Errorf("status = %q, want extracted", result.Status)
	}
	if atomic.LoadInt32(&novelty.called) != 1 {
		t.Errorf("novelty.Check called %d times, want 1", novelty.called)
	}
	if atomic.LoadInt32(&committerCalled) != 0 {
		t.Error("committer should NOT be called when not novel")
	}
}

func TestPipelineNoveltyCheck_CommitsWhenNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: true, Score: 0.95},
	}
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Novelty:   novelty,
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != model.StatusExtracted {
		t.Errorf("status = %q, want extracted", result.Status)
	}
	if atomic.LoadInt32(&committerCalled) != 1 {
		t.Errorf("committer called %d times, want 1", committerCalled)
	}
}

func TestPipelineNoveltyCheck_NilNoveltyStillCommits(t *testing.T) {
	// When no novelty checker configured, committer should still be called.
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt32(&committerCalled) != 1 {
		t.Errorf("committer called %d times, want 1 (no novelty = treat as novel)", committerCalled)
	}
}

// --- P1: Pipeline novelty error is fail-open ---

func TestPipelineNoveltyCheck_ErrorTreatedAsNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		err: errors.New("novelty server down"),
	}
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Novelty:   novelty,
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	// Fail-open: novelty error → treat as novel → commit
	if atomic.LoadInt32(&committerCalled) != 1 {
		t.Errorf("committer should be called on novelty error (fail-open), called %d times", committerCalled)
	}
}

// --- P1: Pipeline committer failure returns error status ---

func TestPipelineCommitter_FailureReturnsError(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: true, Score: 0.9},
	}
	committer := &mockCommitterWithError{
		err: errors.New("git commit failed: connection refused"),
	}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Novelty:   novelty,
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	// Committer failure → error status (git is the only write path now).
	if result.Status != model.StatusError {
		t.Errorf("status = %q, want error (commit failure)", result.Status)
	}
}

// --- P1: Pipeline SkillMD override path ---

func TestPipelineSkillMD_OverridesNameVersionCategory(t *testing.T) {
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3WithSkillMD()},
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if result.Skill == nil {
		t.Fatal("expected skill")
	}
	if result.Skill.Name != "llm-generated-skill" {
		t.Errorf("skill name = %q, want llm-generated-skill (from SkillMD)", result.Skill.Name)
	}
	if result.Skill.Version != 3 {
		t.Errorf("skill version = %d, want 3 (from SkillMD)", result.Skill.Version)
	}
	if result.Skill.Category != "DEBUG" {
		t.Errorf("skill category = %q, want DEBUG (from SkillMD)", result.Skill.Category)
	}
	if len(result.Skill.Patterns) != 2 || result.Skill.Patterns[0] != "debugging/profiling" {
		t.Errorf("skill patterns = %v, want [debugging/profiling go/pprof]", result.Skill.Patterns)
	}
	if result.Skill.FilePath != "skills/llm-generated-skill/v3/SKILL.md" {
		t.Errorf("skill filepath = %q, want skills/llm-generated-skill/v3/SKILL.md", result.Skill.FilePath)
	}
}

func TestPipelineSkillMD_CommitterReceivesGeneratedBody(t *testing.T) {
	committer := &mockCommitterWithBody{}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3WithSkillMD()},
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("content"))
	if err != nil {
		t.Fatal(err)
	}

	if !committer.wasCalled {
		t.Fatal("committer not called")
	}
	// Committer should receive the LLM-generated SkillMD body, not the template.
	if committer.skill.Name != "llm-generated-skill" {
		t.Errorf("committer skill name = %q, want llm-generated-skill", committer.skill.Name)
	}
	body := string(committer.body)
	if body == "" {
		t.Fatal("committer body is empty")
	}
	if !contains(body, "CPU profiling in production") {
		t.Error("committer body should contain LLM-generated content")
	}
}

// --- P1: Pipeline SkillMD fallback ---

func TestPipelineSkillMD_FallsBackToTemplate(t *testing.T) {
	committer := &mockCommitterWithBody{}

	// passStage3() has empty SkillMD → should fall back to buildSkillMD template.
	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if result.Skill == nil {
		t.Fatal("expected skill")
	}
	if result.Skill.Name != "fix-backend-database-connection" {
		t.Errorf("skill name = %q, want fix-backend-database-connection (from classification)", result.Skill.Name)
	}
	if result.Skill.Version != 1 {
		t.Errorf("skill version = %d, want 1 (default)", result.Skill.Version)
	}

	if !committer.wasCalled {
		t.Fatal("committer not called")
	}
	body := string(committer.body)
	if !contains(body, "## When to Use") {
		t.Error("fallback body should contain template sections")
	}
}

// --- P1: Pipeline committer also skipped when not novel ---

func TestPipelineNoveltyCheck_SkipsCommitterWhenNotNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: false, Score: 0.2},
	}
	var committerCalled int32
	committer := &mockCommitterCounter{called: &committerCalled}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Novelty:   novelty,
		Committer: committer,
		Log:       testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("content"))
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt32(&committerCalled) != 0 {
		t.Error("committer should NOT be called when skill is not novel")
	}
}

// --- P1: Pipeline requires committer ---

func TestPipelineRequiresCommitter(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Log:    testLog(),
	})
	if err == nil {
		t.Fatal("expected error when Committer is nil")
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockCommitterCounter is a minimal SkillCommitter that counts calls.
type mockCommitterCounter struct {
	called *int32
}

func (m *mockCommitterCounter) CommitSkill(_ context.Context, _ string, _ *model.SkillRecord, _ []byte) (string, error) {
	atomic.AddInt32(m.called, 1)
	return "abc123", nil
}

// mockCommitterWithError always returns an error.
type mockCommitterWithError struct {
	err error
}

func (m *mockCommitterWithError) CommitSkill(_ context.Context, _ string, _ *model.SkillRecord, _ []byte) (string, error) {
	return "", m.err
}

// mockCommitterWithBody captures the committed skill and body.
type mockCommitterWithBody struct {
	wasCalled bool
	skill     *model.SkillRecord
	body      []byte
}

func (m *mockCommitterWithBody) CommitSkill(_ context.Context, _ string, skill *model.SkillRecord, body []byte) (string, error) {
	m.wasCalled = true
	m.skill = skill
	m.body = body
	return "abc123", nil
}
