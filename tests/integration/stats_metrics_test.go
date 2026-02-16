//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/metrics"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func TestStatsAccuracy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	llmExtract := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llmExtract, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llmExtract, defaultExtractionConfig(), testLogger())

	indexer := storage.NewSQLiteIndexer(store)
	committer := &indexingCommitter{indexer: indexer, embedder: embedder}
	pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1: s1, Stage2: s2, Stage3: s3, Committer: committer, Log: testLogger(),
	})

	sess0 := goodSession("sess-stats-good-0", "test-lib")
	result0, _ := pipeline.Extract(ctx, sess0, []byte("fix database connection pooling"))
	if result0.Status != model.StatusExtracted {
		t.Fatalf("good session 0: status=%q error=%s", result0.Status, result0.Error)
	}
	insertSession(t, store.DB(), sess0, result0)

	for i := 1; i < 3; i++ {
		sess := goodSession(fmt.Sprintf("sess-stats-good-%d", i), "test-lib")
		result, _ := pipeline.Extract(ctx, sess, []byte(fmt.Sprintf("fix variant %d", i)))
		insertSession(t, store.DB(), sess, result)
	}

	for i := 0; i < 2; i++ {
		sess := shortSession(fmt.Sprintf("sess-stats-bad-%d", i), "test-lib")
		result, _ := pipeline.Extract(ctx, sess, []byte("tiny"))
		insertSession(t, store.DB(), sess, result)
	}

	collector := metrics.NewCollector(store.DB())
	m, err := collector.Collect()
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if m.TotalSessions != 5 {
		t.Errorf("total sessions = %d, want 5", m.TotalSessions)
	}
	if m.Extracted != 1 {
		t.Errorf("extracted = %d, want 1", m.Extracted)
	}
	if m.Rejected != 2 {
		t.Errorf("rejected = %d, want 2", m.Rejected)
	}
	if m.Errored != 2 {
		t.Errorf("errored = %d, want 2", m.Errored)
	}
}

func TestStatsThroughPipeline(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	llm := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.50}, llm, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

	indexer := storage.NewSQLiteIndexer(store)
	committer := &indexingCommitter{indexer: indexer, embedder: embedder}
	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1: s1, Stage2: s2, Stage3: s3, Committer: committer,
		Sessions: store, Embedder: embedder, Log: testLogger(), Extractor: "stats-pipeline-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	sess1 := goodSession("sess-sp-1", "test-lib")
	r1, _ := pipeline.Extract(ctx, sess1, []byte("fix database connection pooling"))
	if r1.Status != model.StatusExtracted {
		t.Fatalf("expected extracted, got %q: %s", r1.Status, r1.Error)
	}

	sess2 := shortSession("sess-sp-2", "test-lib")
	r2, _ := pipeline.Extract(ctx, sess2, []byte("tiny"))
	if r2.Status != model.StatusRejected {
		t.Fatalf("expected rejected, got %q", r2.Status)
	}

	collector := metrics.NewCollector(store.DB())
	m, err := collector.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if m.TotalSessions != 2 {
		t.Errorf("total sessions = %d, want 2", m.TotalSessions)
	}
	if m.Extracted != 1 {
		t.Errorf("extracted = %d, want 1", m.Extracted)
	}
	if m.Rejected != 1 {
		t.Errorf("rejected = %d, want 1", m.Rejected)
	}
}
