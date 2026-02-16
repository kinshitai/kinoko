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

type mockSkillPusher struct {
	err       error
	called    int32
	skillName string
	libraryID string
	body      []byte
}

func (m *mockSkillPusher) Push(_ context.Context, skillName, libraryID string, body []byte) error {
	atomic.AddInt32(&m.called, 1)
	m.skillName = skillName
	m.libraryID = libraryID
	m.body = body
	return m.err
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

func TestPipelineNoveltyCheck_SkipsPushWhenNotNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: false, Score: 0.3, Similar: []SimilarSkill{{Name: "existing", Score: 0.92}}},
	}
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1:  &mockStage1{result: passStage1()},
		Stage2:  &mockStage2{result: passStage2()},
		Stage3:  &mockStage3{result: passStage3()},
		Novelty: novelty,
		Pusher:  pusher,
		Log:     testLog(),
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
	if atomic.LoadInt32(&pusher.called) != 0 {
		t.Error("pusher.Push should NOT be called when not novel")
	}
}

func TestPipelineNoveltyCheck_PushesWhenNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: true, Score: 0.95},
	}
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1:  &mockStage1{result: passStage1()},
		Stage2:  &mockStage2{result: passStage2()},
		Stage3:  &mockStage3{result: passStage3()},
		Novelty: novelty,
		Pusher:  pusher,
		Log:     testLog(),
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
	if atomic.LoadInt32(&pusher.called) != 1 {
		t.Errorf("pusher.Push called %d times, want 1", pusher.called)
	}
	if pusher.libraryID != "lib-1" {
		t.Errorf("libraryID = %q, want lib-1", pusher.libraryID)
	}
}

func TestPipelineNoveltyCheck_NilNoveltySkipsPusherToo(t *testing.T) {
	// When no novelty checker configured, pusher should still be called.
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Pusher: pusher,
		Log:    testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt32(&pusher.called) != 1 {
		t.Errorf("pusher.Push called %d times, want 1 (no novelty = treat as novel)", pusher.called)
	}
}

// --- P1: Pipeline novelty error is fail-open ---

func TestPipelineNoveltyCheck_ErrorTreatedAsNovel(t *testing.T) {
	novelty := &mockNoveltyChecker{
		err: errors.New("novelty server down"),
	}
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1:  &mockStage1{result: passStage1()},
		Stage2:  &mockStage2{result: passStage2()},
		Stage3:  &mockStage3{result: passStage3()},
		Novelty: novelty,
		Pusher:  pusher,
		Log:     testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	// Fail-open: novelty error → treat as novel → push
	if atomic.LoadInt32(&pusher.called) != 1 {
		t.Errorf("pusher should be called on novelty error (fail-open), called %d times", pusher.called)
	}
}

// --- P1: Pipeline pusher failure is non-fatal ---

func TestPipelinePusher_FailureIsNonFatal(t *testing.T) {
	novelty := &mockNoveltyChecker{
		result: &NoveltyResult{Novel: true, Score: 0.9},
	}
	pusher := &mockSkillPusher{
		err: errors.New("git push failed: connection refused"),
	}

	p, err := NewPipeline(PipelineConfig{
		Stage1:  &mockStage1{result: passStage1()},
		Stage2:  &mockStage2{result: passStage2()},
		Stage3:  &mockStage3{result: passStage3()},
		Novelty: novelty,
		Pusher:  pusher,
		Log:     testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection"))
	if err != nil {
		t.Fatal(err)
	}

	// Push failure should not prevent extraction result.
	if result.Status != model.StatusExtracted {
		t.Errorf("status = %q, want extracted (push failure is non-fatal)", result.Status)
	}
	if result.Skill == nil {
		t.Error("skill should still be present despite push failure")
	}
}

// --- P1: Pipeline SkillMD override path ---

func TestPipelineSkillMD_OverridesNameVersionCategory(t *testing.T) {
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3WithSkillMD()},
		Pusher: pusher,
		Log:    testLog(),
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
	// FilePath should use the LLM-generated version
	if result.Skill.FilePath != "skills/llm-generated-skill/v3/SKILL.md" {
		t.Errorf("skill filepath = %q, want skills/llm-generated-skill/v3/SKILL.md", result.Skill.FilePath)
	}
}

func TestPipelineSkillMD_PusherReceivesGeneratedBody(t *testing.T) {
	pusher := &mockSkillPusher{}

	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3WithSkillMD()},
		Pusher: pusher,
		Log:    testLog(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.Extract(context.Background(), pipelineTestSession(), []byte("content"))
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt32(&pusher.called) != 1 {
		t.Fatal("pusher not called")
	}
	// Pusher should receive the LLM-generated SkillMD body, not the template.
	if pusher.skillName != "llm-generated-skill" {
		t.Errorf("pusher.skillName = %q, want llm-generated-skill", pusher.skillName)
	}
	body := string(pusher.body)
	if body == "" {
		t.Fatal("pusher.body is empty")
	}
	// Should contain the LLM-generated content, not template.
	if !contains(body, "CPU profiling in production") {
		t.Error("pusher.body should contain LLM-generated content")
	}
}

// --- P1: Pipeline SkillMD fallback ---

func TestPipelineSkillMD_FallsBackToTemplate(t *testing.T) {
	pusher := &mockSkillPusher{}

	// passStage3() has empty SkillMD → should fall back to buildSkillMD template.
	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Pusher: pusher,
		Log:    testLog(),
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
	// Without SkillMD, name comes from skillNameFromClassification.
	if result.Skill.Name != "fix-backend-database-connection" {
		t.Errorf("skill name = %q, want fix-backend-database-connection (from classification)", result.Skill.Name)
	}
	if result.Skill.Version != 1 {
		t.Errorf("skill version = %d, want 1 (default)", result.Skill.Version)
	}

	// Pusher body should be from template (contains "## When to Use").
	if atomic.LoadInt32(&pusher.called) != 1 {
		t.Fatal("pusher not called")
	}
	body := string(pusher.body)
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
