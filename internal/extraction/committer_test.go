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
	w := &mockWriter{}
	c := &mockCommitter{hash: "abc123"}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Writer:    w,
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
	if !w.called {
		t.Error("writer.Put not called")
	}
	if !c.called {
		t.Error("committer.CommitSkill not called")
	}
	if c.libraryID != "lib-1" {
		t.Errorf("libraryID = %q, want lib-1", c.libraryID)
	}
}

func TestPipeline_CommitterErrorNonFatal(t *testing.T) {
	w := &mockWriter{}
	c := &mockCommitter{err: errors.New("push failed")}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Writer:    w,
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
	// Extraction should still succeed even if committer fails.
	if result.Status != model.StatusExtracted {
		t.Fatalf("status = %s, want extracted", result.Status)
	}
}

func TestPipeline_NilCommitterSkipped(t *testing.T) {
	w := &mockWriter{}

	p, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Writer: w,
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
