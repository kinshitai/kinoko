package extraction

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
)

type mockCommitter struct {
	called    bool
	libraryID string
	skill     *model.SkillRecord
	body      []byte
	hash      string
	err       error
}

func (m *mockCommitter) CommitSkill(_ context.Context, libraryID string, skill *model.SkillRecord, body []byte) (string, error) {
	m.called = true
	m.libraryID = libraryID
	m.skill = skill
	m.body = body
	return m.hash, m.err
}

func TestPipeline_CommitterCalledOnExtract(t *testing.T) {
	c := &mockCommitter{hash: "abc123"}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Committer: c,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("test content"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusExtracted {
		t.Fatalf("status = %s, want extracted", result.Status)
	}
	if !c.called {
		t.Error("committer.CommitSkill not called")
	}
	if c.libraryID != "lib-1" {
		t.Errorf("libraryID = %q, want lib-1", c.libraryID)
	}
	if result.CommitHash != "abc123" {
		t.Errorf("commit hash = %q, want abc123", result.CommitHash)
	}
}

func TestPipeline_CommitterErrorIsFatal(t *testing.T) {
	c := &mockCommitter{err: errors.New("push failed")}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Committer: c,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("test content"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusError {
		t.Fatalf("status = %s, want error", result.Status)
	}
}

func TestPipeline_NilCommitterExtractsWithoutGit(t *testing.T) {
	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Log:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("test content"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusExtracted {
		t.Fatalf("status = %s, want extracted", result.Status)
	}
}
