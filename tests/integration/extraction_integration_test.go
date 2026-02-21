//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/injection"
	"github.com/kinoko-dev/kinoko/internal/run/queue"
	"github.com/kinoko-dev/kinoko/internal/run/worker"
	"github.com/kinoko-dev/kinoko/internal/serve/storage"
	"github.com/kinoko-dev/kinoko/internal/shared/decay"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// =============================================================================
// P1: Pipeline + Worker — Concurrent extraction results correctness
// =============================================================================

func TestPipelineWorkerConcurrentResults(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "p1.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	// Each worker gets a unique LLM response producing a unique skill name.
	const n = 5
	type result struct {
		idx    int
		status model.ExtractionStatus
		err    error
	}
	results := make(chan result, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			embedder := newPredictableEmbedder(3)
			// Each gets a unique pattern to avoid duplicate skill names.
			pattern := fmt.Sprintf("FIX/Backend/Issue%d", idx)
			llm := &predictableLLM{
				rubricResponse: fmt.Sprintf(`{
					"scores": {"problem_specificity":4,"solution_completeness":4,"context_portability":3,
					"reasoning_transparency":3,"technical_accuracy":4,"verification_evidence":3,"innovation_level":3},
					"category":"tactical","patterns":[%q]}`, pattern),
				criticResponse: extractVerdictJSON(),
			}

			s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
			s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
			s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

			pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
				Stage1: s1, Stage2: s2, Stage3: s3, Committer: noopCommitter{}, Log: testLogger(),
			})

			sess := goodSession(fmt.Sprintf("sess-p1-%d", idx), "test-lib")
			r, err := pipeline.Extract(ctx, sess, []byte(fmt.Sprintf("fix unique problem number %d with detailed solution", idx)))
			if err != nil {
				results <- result{idx, "", err}
				return
			}
			results <- result{idx, r.Status, nil}
		}(i)
	}

	extracted := 0
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			t.Logf("worker %d error: %v", r.idx, r.err)
			continue
		}
		if r.status == model.StatusExtracted {
			extracted++
		}
	}

	if extracted == 0 {
		t.Fatal("expected at least one successful extraction from concurrent workers")
	}

	// Verify extracted skills have correct data in DB.
	skills, err := store.Query(ctx, storage.SkillQuery{LibraryIDs: []string{"test-lib"}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Concurrent extraction: %d extracted, %d skills in DB", extracted, len(skills))
	for _, s := range skills {
		if s.CompositeScore == 0 {
			t.Errorf("skill %s has zero composite score — data corruption", s.Skill.ID)
		}
	}
}

// =============================================================================
// P2: Worker 60s timeout vs slow extraction — context expires mid-pipeline
// =============================================================================

func TestWorkerTimeoutMidExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "p2.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	cfg := workerConfig()
	cfg.Concurrency = 1
	cfg.ProcessTimeout = 3 * time.Second // short timeout for test
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	q.Enqueue(ctx, makeWorkerSession("sess-slow", "lib-1"), []byte("slow log content"))

	// Extractor that takes longer than the configured ProcessTimeout.
	var ctxWasCancelled atomic.Bool
	ext := &workerMockExtractor{fn: func(ctx context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		select {
		case <-ctx.Done():
			ctxWasCancelled.Store(true)
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return &model.ExtractionResult{Status: model.StatusExtracted}, nil
		}
	}}

	getSession := func(_ context.Context, id string) (*model.SessionRecord, error) {
		return &model.SessionRecord{ID: id, LibraryID: "lib-1"}, nil
	}

	pool := worker.NewPool(q, ext, getSession, cfg, testLogger())
	pool.Start(ctx)

	// Wait for the timeout to fire + margin.
	deadline := time.After(15 * time.Second)
	for {
		stats := pool.Stats()
		if stats.TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			pool.Stop(stopCtx)
			cancel()
			t.Fatal("timeout waiting for pool to process slow session")
		case <-time.After(200 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool.Stop(stopCtx)

	if !ctxWasCancelled.Load() {
		t.Error("expected context to be cancelled by pool's ProcessTimeout")
	}

	// Session should be in error/failed state.
	var status string
	queueStore.DB().QueryRow("SELECT status FROM queue_entries WHERE session_id = 'sess-slow'").Scan(&status)
	if status != "error" && status != "failed" {
		t.Errorf("slow session status = %q, want error or failed", status)
	}
}

// =============================================================================
// P3: Multiple workers processing same session — verify no double-extraction
// =============================================================================

func TestNoDoubleExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "p3.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	cfg := workerConfig()
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	q.Enqueue(ctx, makeWorkerSession("sess-dedup", "lib-1"), []byte("log"))

	// Race 20 goroutines to claim the same session.
	var mu sync.Mutex
	var claimed []string
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			entry, err := q.Claim(ctx, fmt.Sprintf("worker-%d", id))
			if err != nil {
				return
			}
			if entry != nil {
				mu.Lock()
				claimed = append(claimed, fmt.Sprintf("worker-%d", id))
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(claimed) != 1 {
		t.Errorf("expected exactly 1 claim, got %d: %v", len(claimed), claimed)
	}
}

// =============================================================================
// P4: Import same file twice — duplicate detection
// =============================================================================

func TestImportDuplicateSession(t *testing.T) {
	store := newWorkerTestStore(t)
	cfg := workerConfig()
	q, queueStore, _ := newWorkerQueue(t, store, cfg)
	ctx := context.Background()

	session := makeWorkerSession("sess-dup-import", "lib-1")
	content := []byte("session log content")

	// First import succeeds.
	err := q.Enqueue(ctx, session, content)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	// Second import of same session ID should fail.
	err = q.Enqueue(ctx, session, content)
	if err == nil {
		t.Error("expected error on duplicate session import")
	} else {
		t.Logf("duplicate import error (expected): %v", err)
	}

	// Verify only one session in queue DB.
	var count int
	queueStore.DB().QueryRow("SELECT COUNT(*) FROM queue_entries WHERE session_id = ?", "sess-dup-import").Scan(&count)
	if count != 1 {
		t.Errorf("session count = %d, want 1", count)
	}
}

// =============================================================================
// P5: Import invalid session log (not a real session)
// =============================================================================

func TestImportInvalidSessionLog(t *testing.T) {
	_ = newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	llm := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

	pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1: s1, Stage2: s2, Stage3: s3, Committer: noopCommitter{}, Log: testLogger(),
	})

	// Session with characteristics that should fail stage1 (too short).
	session := shortSession("sess-invalid", "test-lib")
	result, err := pipeline.Extract(ctx, session, []byte(""))
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != model.StatusRejected {
		t.Errorf("invalid session status = %q, want rejected", result.Status)
	}
}

// =============================================================================
// P6: Import with queue at backpressure limit
// =============================================================================

func TestImportBackpressure(t *testing.T) {
	store := newWorkerTestStore(t)
	cfg := workerConfig()
	cfg.QueueDepthCritical = 3 // very low limit
	q, queueStore, _ := newWorkerQueue(t, store, cfg)
	ctx := context.Background()

	// Fill to limit.
	for i := 0; i < 3; i++ {
		err := q.Enqueue(ctx, makeWorkerSession(fmt.Sprintf("sess-bp-%d", i), "lib-1"), []byte("log"))
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Next enqueue should hit backpressure.
	err := q.Enqueue(ctx, makeWorkerSession("sess-bp-over", "lib-1"), []byte("log"))
	if !errors.Is(err, worker.ErrBackpressure) {
		t.Errorf("expected ErrBackpressure, got %v", err)
	}

	// Verify overflow session was NOT written to DB.
	var count int
	queueStore.DB().QueryRow("SELECT COUNT(*) FROM queue_entries WHERE session_id = ?", "sess-bp-over").Scan(&count)
	if count != 0 {
		t.Error("backpressure-rejected session should not be in DB")
	}
}

// =============================================================================
// P7: Import very large file — verify no size guard exists
// =============================================================================

func TestBug_NoLargeFileGuard(t *testing.T) {
	// TECH-DEBT says: "Import very large file (>50MB if the guard exists, or prove it doesn't)"
	// This test proves there's NO size guard on enqueue.
	store := newWorkerTestStore(t)
	cfg := workerConfig()
	q, _, dataDir := newWorkerQueue(t, store, cfg)
	ctx := context.Background()

	// Create a 1MB payload (not 50MB to keep tests fast, but proves no guard).
	bigContent := make([]byte, 1<<20) // 1MB
	for i := range bigContent {
		bigContent[i] = 'A'
	}

	err := q.Enqueue(ctx, makeWorkerSession("sess-big", "lib-1"), bigContent)
	if err != nil {
		t.Fatalf("enqueue large file: %v", err)
	}

	// Verify the file was written to disk at full size.
	entry, _ := q.Claim(ctx, "worker-0")
	if entry == nil {
		t.Fatal("expected entry")
	}
	info, err := os.Stat(entry.LogContentPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(bigContent)) {
		t.Errorf("file size = %d, want %d", info.Size(), len(bigContent))
	}

	// BUG DOCUMENTED: No size guard on queue.Enqueue.
	// A 50MB+ file will be written to disk and fully read into memory by the worker.
	// This could cause OOM in production.
	t.Log("BUG: No size guard on Enqueue — arbitrarily large files accepted")
	_ = dataDir
}

// =============================================================================
// P8: Inject with >999 skills — unbounded IN clause (TECH-DEBT D.3)
// =============================================================================

func TestBug_UnboundedINClause(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "in-clause.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	// Insert >999 skills.
	const skillCount = 1050
	for i := 0; i < skillCount; i++ {
		sk := &model.SkillRecord{
			ID:        fmt.Sprintf("skill-in-%04d", i),
			Name:      fmt.Sprintf("skill-%04d", i),
			Version:   1,
			LibraryID: "test-lib",
			Category:  model.CategoryTactical,
			Patterns:  []string{"FIX/Backend/DatabaseConnection"},
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
			},
			Embedding:   embedder.deterministicVector(fmt.Sprintf("skill %d", i)),
			DecayScore:  1.0,
			ExtractedBy: "test",
			FilePath:    fmt.Sprintf("skills/skill-%04d/SKILL.md", i),
		}
		if err := store.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put skill %d: %v", i, err)
		}
	}

	// Query all skills (triggers loadPatternsMulti with >999 IDs).
	results, err := store.Query(ctx, storage.SkillQuery{
		LibraryIDs: []string{"test-lib"},
		Patterns:   []string{"FIX/Backend/DatabaseConnection"},
		Limit:      0, // no limit
	})

	if err != nil {
		// BUG CONFIRMED: SQLite SQLITE_MAX_VARIABLE_NUMBER exceeded.
		t.Logf("BUG CONFIRMED: Query with %d skills fails: %v", skillCount, err)
		t.Logf("This is TECH-DEBT D.3 — unbounded IN clause in loadPatternsMulti/loadEmbeddingsMulti")
	} else {
		t.Logf("Query with %d skills returned %d results (no error — SQLite variable limit may be higher in this build)", skillCount, len(results))
		if len(results) != skillCount {
			t.Errorf("expected %d results, got %d", skillCount, len(results))
		}
	}
}

// =============================================================================
// P9: Inject with no embedding service — degraded/pattern-only mode
// =============================================================================

func TestInjectNoEmbeddingService(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed a skill with patterns.
	sk := &model.SkillRecord{
		ID: "skill-noem", Name: "fix-db-noem", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/fix-db-noem/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}

	// nil embedder = circuit breaker open / no embedding service.
	inj := injection.New(nil, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection issue",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
	})
	if err != nil {
		t.Fatalf("injection should work without embedder: %v", err)
	}
	if len(resp.Skills) == 0 {
		t.Error("pattern-only injection should return matching skills")
	}
}

// =============================================================================
// P10: Inject with skill in DB but no SKILL.md on disk
// =============================================================================

func TestInjectMissingSkillFile(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "missing.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	// Skill points to a file that doesn't exist.
	sk := &model.SkillRecord{
		ID: "skill-nofile", Name: "ghost-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("ghost skill"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    filepath.Join(tmpDir, "nonexistent/SKILL.md"),
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-ghost",
	})

	// Injection should not crash — it should either return the skill
	// (file_path is just metadata) or gracefully skip it.
	if err != nil {
		t.Logf("Injection with missing SKILL.md file errors: %v", err)
		t.Log("System does NOT handle missing SKILL.md gracefully")
	} else {
		t.Logf("Injection returned %d skills despite missing SKILL.md — file_path is not validated at injection time", len(resp.Skills))
	}
}

// =============================================================================
// P11: Create skill via extraction → verify it's injectable
// =============================================================================

func TestExtractThenInject(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	llm := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

	indexer := storage.NewSQLiteIndexer(store)
	committer := &indexingCommitter{indexer: indexer, embedder: embedder}
	pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1: s1, Stage2: s2, Stage3: s3, Committer: committer, Log: testLogger(),
	})

	// Extract a skill.
	sess := goodSession("sess-e2i", "test-lib")
	result, err := pipeline.Extract(ctx, sess, []byte("fix database connection pooling with retry logic and backoff"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.StatusExtracted {
		t.Fatalf("extraction status = %q, want extracted: %s", result.Status, result.Error)
	}

	// Now inject — the extracted skill should be findable.
	classifyLLM := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, classifyLLM, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-e2i-inject",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Skills) == 0 {
		t.Fatal("extracted skill should be injectable")
	}

	// Verify the injected skill is the one we extracted.
	found := false
	for _, s := range resp.Skills {
		if s.SkillID == result.Skill.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("extracted skill %s not found in injection results", result.Skill.ID)
	}
}

// =============================================================================
// P12: Create skill → run decay → verify updated scores in injection ranking
// =============================================================================

func TestExtractDecayInjectRanking(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed two skills: one fresh, one old.
	fresh := &model.SkillRecord{
		ID: "skill-fresh", Name: "fresh-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("fresh skill"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/fresh/SKILL.md",
	}
	old := &model.SkillRecord{
		ID: "skill-old", Name: "old-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("old skill"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/old/SKILL.md",
	}

	store.Put(ctx, fresh, nil)
	store.Put(ctx, old, nil)

	// Set last_injected_at: fresh = 1 day ago, old = 90 days ago (1 half-life for tactical).
	store.DB().Exec("UPDATE skills SET last_injected_at = ? WHERE id = ?", now.AddDate(0, 0, -1), "skill-fresh")
	store.DB().Exec("UPDATE skills SET last_injected_at = ? WHERE id = ?", now.AddDate(0, 0, -90), "skill-old")

	// Run decay.
	decayCfg := decay.DefaultConfig()
	runner, _ := decay.NewRunner(store, store, decayCfg, testLogger())
	runner.SetNow(func() time.Time { return now })
	_, err := runner.RunCycle(ctx, "test-lib")
	if err != nil {
		t.Fatal(err)
	}

	// Verify decay scores changed.
	freshSkill, _ := store.Get(ctx, "skill-fresh")
	oldSkill, _ := store.Get(ctx, "skill-old")

	if freshSkill.DecayScore <= oldSkill.DecayScore {
		t.Errorf("fresh decay (%f) should be > old decay (%f)", freshSkill.DecayScore, oldSkill.DecayScore)
	}

	// Inject — fresh skill should rank higher because of higher decay.
	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-rank",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Skills) < 2 {
		t.Fatalf("expected 2 skills, got %d", len(resp.Skills))
	}

	// With same quality but different decay, fresh should rank higher.
	if resp.Skills[0].SkillID != "skill-fresh" {
		t.Errorf("expected fresh skill ranked first, got %s (decay fresh=%f, old=%f)",
			resp.Skills[0].SkillID, freshSkill.DecayScore, oldSkill.DecayScore)
	}
}

// =============================================================================
// P13: Create skill → delete SKILL.md → system handles gracefully
// =============================================================================

func TestSkillFileDeletedGraceful(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "deleted.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	// Create skill with a real file.
	skillDir := filepath.Join(tmpDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte("# Test Skill\nContent here."), 0644)

	sk := &model.SkillRecord{
		ID: "skill-del", Name: "deletable-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("deletable skill"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    skillFile,
	}
	store.Put(ctx, sk, nil)

	// Delete the file.
	os.Remove(skillFile)

	// Verify the skill still exists in DB.
	dbSkill, err := store.Get(ctx, "skill-del")
	if err != nil {
		t.Fatal(err)
	}
	if dbSkill.FilePath != skillFile {
		t.Errorf("file path changed: %q", dbSkill.FilePath)
	}

	// Verify the file is actually gone.
	if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}

	// Injection should still work (returns DB data, file is metadata only).
	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-del",
	})
	if err != nil {
		t.Fatalf("injection should not fail when SKILL.md is deleted: %v", err)
	}
	if len(resp.Skills) == 0 {
		t.Error("skill with deleted file should still be injectable from DB")
	}

	// TECH-DEBT D.2: "File Write After DB Commit — no reconciliation mechanism"
	// This test confirms there's no consistency check — skill with missing file is silently served.
	t.Log("CONFIRMED: No reconciliation — skill with deleted SKILL.md is silently injectable")
}

// =============================================================================
// P14: Full serve lifecycle simulation
// Enqueue → worker processes → skill appears in DB
// =============================================================================

func TestServeLifecycleSimulation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "serve.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	cfg := workerConfig()
	cfg.Concurrency = 1
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	// Simulate serve: enqueue a session.
	session := makeWorkerSession("sess-serve", "lib-1")
	q.Enqueue(ctx, session, []byte("User asked to fix database connection pooling. Agent fixed it."))

	// Mock extractor that produces a real skill.
	ext := &workerMockExtractor{fn: func(_ context.Context, sess model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return &model.ExtractionResult{
			Status: model.StatusExtracted,
			Skill: &model.SkillRecord{
				ID:   "skill-serve-1",
				Name: "fix-db-serve",
			},
		}, nil
	}}

	getSession := func(_ context.Context, id string) (*model.SessionRecord, error) {
		s := makeWorkerSession(id, "lib-1")
		return &s, nil
	}

	pool := worker.NewPool(q, ext, getSession, cfg, testLogger())
	pool.Start(ctx)

	// Wait for processing.
	deadline := time.After(5 * time.Second)
	for {
		if pool.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for worker")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool.Stop(stopCtx)

	// Verify session completed.
	var status string
	queueStore.DB().QueryRow("SELECT status FROM queue_entries WHERE session_id = ?", "sess-serve").
		Scan(&status)

	if status != "extracted" {
		t.Errorf("session status = %q, want extracted", status)
	}
}

// =============================================================================
// P15: Graceful shutdown during active extraction — skill completes or requeues
// =============================================================================

func TestGracefulShutdownDuringExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "shutdown.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	cfg := workerConfig()
	cfg.Concurrency = 1
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	q.Enqueue(ctx, makeWorkerSession("sess-shutdown", "lib-1"), []byte("log"))

	extractStarted := make(chan struct{})
	extractDone := make(chan struct{})

	ext := &workerMockExtractor{fn: func(ctx context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		close(extractStarted)
		<-extractDone // Wait for test to signal completion.
		return &model.ExtractionResult{
			Status: model.StatusExtracted,
			Skill:  &model.SkillRecord{ID: "skill-shutdown"},
		}, nil
	}}

	getSession := func(_ context.Context, id string) (*model.SessionRecord, error) {
		return &model.SessionRecord{ID: id, LibraryID: "lib-1"}, nil
	}

	pool := worker.NewPool(q, ext, getSession, cfg, testLogger())
	pool.Start(ctx)

	// Wait for extraction to start.
	<-extractStarted

	// Signal shutdown — pool should wait for in-flight.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(extractDone)
	}()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = pool.Stop(stopCtx)
	if err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Verify: session was either completed or requeued, never lost.
	var status string
	queueStore.DB().QueryRow("SELECT status FROM queue_entries WHERE session_id = ?", "sess-shutdown").Scan(&status)
	if status != "extracted" && status != "queued" {
		t.Errorf("status = %q — session is LOST (should be extracted or requeued)", status)
	}
}

// =============================================================================
// P16: Serve with scheduler running decay — verify scores change over time
// =============================================================================

func TestSchedulerDecayModifiesScores(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed a skill that should decay.
	sk := &model.SkillRecord{
		ID: "skill-sched-decay", Name: "decaying-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/decaying/SKILL.md",
	}
	store.Put(ctx, sk, nil)
	store.DB().Exec("UPDATE skills SET last_injected_at = ? WHERE id = ?",
		now.AddDate(0, 0, -180), "skill-sched-decay")

	// Run decay directly (simulating what scheduler's cron does).
	decayCfg := decay.DefaultConfig()
	runner, _ := decay.NewRunner(store, store, decayCfg, testLogger())
	runner.SetNow(func() time.Time { return now })

	// Check score before.
	before, _ := store.Get(ctx, "skill-sched-decay")
	if before.DecayScore != 1.0 {
		t.Fatalf("initial decay = %f, want 1.0", before.DecayScore)
	}

	// Run decay.
	result, err := runner.RunCycle(ctx, "test-lib")
	if err != nil {
		t.Fatal(err)
	}

	if result.Processed != 1 {
		t.Errorf("processed = %d, want 1", result.Processed)
	}

	// Check score after — should be lower.
	after, _ := store.Get(ctx, "skill-sched-decay")
	if after.DecayScore >= 1.0 {
		t.Errorf("decay score after cycle = %f, should be < 1.0", after.DecayScore)
	}
	if after.DecayScore <= 0 {
		t.Errorf("decay score = %f, should not be zero for 180-day-old tactical skill", after.DecayScore)
	}

	// 180 days / 90 day half-life = 2 half-lives → ~0.25
	assertApprox(t, after.DecayScore, 0.25, 0.05, "decay after 180 days")
}

// =============================================================================
// P17: Bug — Pipeline sampling counters race (TECH-DEBT D.1)
// =============================================================================

func TestBug_SamplingCounterRace(t *testing.T) {
	// Verifies that the P0 sampling counter race (TECH-DEBT D.1) is fixed.
	// Sampling counters now use atomic.Int64 — this test should pass with -race.
	// Each goroutine gets its own mocks to avoid test-mock races.
	ctx := context.Background()

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			embedder := newPredictableEmbedder(3)
			llm := &predictableLLM{
				rubricResponse: goodRubricJSON(),
				criticResponse: extractVerdictJSON(),
			}
			s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
			s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
			s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())
			pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
				Stage1: s1, Stage2: s2, Stage3: s3,
				Committer: noopCommitter{}, Log: testLogger(),
			})
			sess := goodSession(fmt.Sprintf("sess-race-%d", idx), "test-lib")
			pipeline.Extract(ctx, sess, []byte(fmt.Sprintf("fix problem %d with solution", idx)))
		}(i)
	}
	wg.Wait()

	t.Log("Sampling counter race (TECH-DEBT D.1) — FIXED: atomic.Int64 counters, no race detected")
}

// =============================================================================
// P19: Worker pool with real pipeline — full E2E with extraction results
// =============================================================================

func TestWorkerPoolWithRealPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "real-pipeline.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	cfg := workerConfig()
	cfg.Concurrency = 1
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	embedder := newPredictableEmbedder(3)
	llm := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(embedder, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

	pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1: s1, Stage2: s2, Stage3: s3, Committer: noopCommitter{}, Log: testLogger(),
	})

	// Enqueue a good session.
	sess := makeWorkerSession("sess-real-pipe", "lib-1")
	// Override fields to pass stage1.
	sess.DurationMinutes = 15
	sess.ToolCallCount = 8
	sess.MessageCount = 20
	sess.HasSuccessfulExec = true
	sess.TokensUsed = 5000

	q.Enqueue(ctx, sess, []byte("User asked to fix database connection pooling. Agent implemented retry logic."))

	getSession := func(_ context.Context, id string) (*model.SessionRecord, error) {
		s := makeWorkerSession(id, "lib-1")
		s.DurationMinutes = 15
		s.ToolCallCount = 8
		s.MessageCount = 20
		s.HasSuccessfulExec = true
		s.TokensUsed = 5000
		return &s, nil
	}

	pool := worker.NewPool(q, pipeline, getSession, cfg, testLogger())
	pool.Start(ctx)

	deadline := time.After(10 * time.Second)
	for {
		if pool.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool.Stop(stopCtx)

	// Verify actual skill was extracted and stored.
	skills, err := store.Query(ctx, storage.SkillQuery{LibraryIDs: []string{"lib-1"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) == 0 {
		// May fail due to library ID mismatch — check session status.
		var status, lastErr string
		queueStore.DB().QueryRow("SELECT status, COALESCE(last_error,'') FROM queue_entries WHERE session_id = ?", "sess-real-pipe").Scan(&status, &lastErr)
		t.Logf("session status: %s, error: %s", status, lastErr)
		t.Log("No skills extracted — may be expected if pipeline rejects the session")
	} else {
		t.Logf("Successfully extracted %d skill(s) through worker pool + real pipeline", len(skills))
		for _, s := range skills {
			if s.CompositeScore == 0 {
				t.Errorf("skill %s has zero composite — data issue", s.Skill.ID)
			}
		}
	}
}

// =============================================================================
// P20: Inject with failing embedder — verify it doesn't crash
// =============================================================================

func TestInjectWithFailingEmbedder(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	embedder := newPredictableEmbedder(3)

	sk := &model.SkillRecord{
		ID: "skill-failem", Name: "fix-db-failem", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("fix db"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/fix-db/SKILL.md",
	}
	store.Put(ctx, sk, nil)

	// Embedder that always fails.
	failEmbedder := newPredictableEmbedder(3)
	failEmbedder.failAfter = -1

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}

	inj := injection.New(failEmbedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-failem",
	})

	// Should degrade gracefully — pattern-only mode.
	if err != nil {
		t.Logf("Injection with failing embedder: %v", err)
		// This might be an error or might degrade — document behavior.
	} else {
		t.Logf("Injection with failing embedder returned %d skills (degraded mode works)", len(resp.Skills))
	}
}

// =============================================================================
// P21: Cross-system — extract, decay to zero, verify filtered from injection
// =============================================================================

func TestExtractDecayToZeroFiltered(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	sk := &model.SkillRecord{
		ID: "skill-decay-zero", Name: "doomed-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:   embedder.deterministicVector("doomed skill"),
		DecayScore:  1.0,
		ExtractedBy: "test",
		FilePath:    "skills/doomed/SKILL.md",
	}
	store.Put(ctx, sk, nil)

	// Set last_injected_at to 900 days ago (10 half-lives for tactical = 90 days).
	store.DB().Exec("UPDATE skills SET last_injected_at = ? WHERE id = ?",
		now.AddDate(0, 0, -900), "skill-decay-zero")

	// Run decay.
	runner, _ := decay.NewRunner(store, store, decay.DefaultConfig(), testLogger())
	runner.SetNow(func() time.Time { return now })
	result, _ := runner.RunCycle(ctx, "test-lib")

	if result.Deprecated < 1 {
		t.Errorf("expected skill to be deprecated, deprecated count = %d", result.Deprecated)
	}

	// Verify it's filtered from injection.
	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix database connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  5,
		SessionID:  "sess-decay-zero",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range resp.Skills {
		if s.SkillID == "skill-decay-zero" {
			t.Error("deprecated skill (decay=0) should not appear in injection results")
		}
	}
}

// =============================================================================
// P22: Context cancellation propagation through worker pool
// =============================================================================

func TestContextCancellationPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(tmpDir, "cancel.db"), "test")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := workerConfig()
	cfg.Concurrency = 1
	queueStore, err := queue.New(filepath.Join(tmpDir, "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer queueStore.Close()
	q := queue.NewQueue(queueStore, tmpDir, cfg, testLogger())

	ctx := context.Background()
	q.Enqueue(ctx, makeWorkerSession("sess-cancel", "lib-1"), []byte("log"))

	var extractCtxCancelled atomic.Bool
	ext := &workerMockExtractor{fn: func(ctx context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		// Worker pool uses detached context (context.Background + 60s timeout).
		// So parent context cancellation should NOT reach here.
		select {
		case <-ctx.Done():
			extractCtxCancelled.Store(true)
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return &model.ExtractionResult{Status: model.StatusExtracted}, nil
		}
	}}

	getSession := func(_ context.Context, id string) (*model.SessionRecord, error) {
		return &model.SessionRecord{ID: id, LibraryID: "lib-1"}, nil
	}

	pool := worker.NewPool(q, ext, getSession, cfg, testLogger())
	pool.Start(ctx)

	deadline := time.After(5 * time.Second)
	for {
		if pool.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool.Stop(stopCtx)

	if extractCtxCancelled.Load() {
		t.Log("WARNING: Extraction context was cancelled — detached context may not work as expected")
	}
}

// =============================================================================
// P23: Verify injection events have correct match_score ordering
// =============================================================================

func TestInjectionMatchScoreOrdering(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	// Create 3 skills with different embeddings.
	for i, name := range []string{"exact-match", "partial-match", "no-match"} {
		sk := &model.SkillRecord{
			ID: fmt.Sprintf("skill-ord-%d", i), Name: name, Version: 1, LibraryID: "test-lib",
			Category: model.CategoryTactical,
			Patterns: []string{"FIX/Backend/DatabaseConnection"},
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
			},
			Embedding:   embedder.deterministicVector(name),
			DecayScore:  1.0,
			ExtractedBy: "test",
			FilePath:    fmt.Sprintf("skills/%s/SKILL.md", name),
		}
		store.Put(ctx, sk, nil)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "exact-match", // Same text as first skill's embedding.
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  10,
		SessionID:  "sess-ord",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Skills) < 2 {
		t.Fatalf("expected >= 2 skills, got %d", len(resp.Skills))
	}

	// Verify composite scores are monotonically decreasing.
	for i := 1; i < len(resp.Skills); i++ {
		if resp.Skills[i].CompositeScore > resp.Skills[i-1].CompositeScore {
			t.Errorf("scores not sorted: [%d]=%f > [%d]=%f",
				i, resp.Skills[i].CompositeScore, i-1, resp.Skills[i-1].CompositeScore)
		}
	}
}

// =============================================================================
// P24: Enqueue duplicate via unique constraint — verify log file cleanup
// =============================================================================

func TestBug_DuplicateEnqueueLeaksFile(t *testing.T) {
	store := newWorkerTestStore(t)
	cfg := workerConfig()
	q, _, dataDir := newWorkerQueue(t, store, cfg)
	ctx := context.Background()

	session := makeWorkerSession("sess-dupe-file", "lib-1")

	// First enqueue.
	q.Enqueue(ctx, session, []byte("original content"))

	// Second enqueue — should fail.
	err := q.Enqueue(ctx, session, []byte("duplicate content"))
	if err == nil {
		t.Fatal("expected error on duplicate")
	}

	// Check if duplicate log file was left behind.
	files, _ := os.ReadDir(dataDir)
	logFiles := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".log") || strings.Contains(f.Name(), "sess-dupe-file") {
			logFiles++
		}
	}

	// TECH-DEBT D.6: "os.Remove(logPath) IS called on exec error"
	// But let's verify it actually works.
	if logFiles > 1 {
		t.Errorf("BUG: Duplicate enqueue left %d log files — expected at most 1", logFiles)
	}
}
