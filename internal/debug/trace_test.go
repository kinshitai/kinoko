package debug

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewTracer_EmptyDir(t *testing.T) {
	tr := NewTracer("")
	if tr != nil {
		t.Fatal("expected nil tracer for empty baseDir")
	}
}

func TestNilTracer_StartRun(t *testing.T) {
	var tr *Tracer
	rt := tr.StartRun()
	if rt != nil {
		t.Fatal("expected nil RunTrace from nil Tracer")
	}
}

func TestNilRunTrace_Methods(t *testing.T) {
	var rt *RunTrace
	// All methods are nil-safe and return nothing.
	rt.WriteSession([]byte("data"))
	rt.WriteStage("s1", map[string]string{"a": "b"})
	rt.WriteRaw("s2", "req", []byte("{}"))
	rt.WriteSkill("skill1", []byte("# Skill"))
	rt.WriteSummary(map[string]string{"ok": "true"})
}

func TestStartRun_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()
	if rt == nil {
		t.Fatal("expected non-nil RunTrace")
	}
	if rt.TraceID == "" {
		t.Fatal("expected non-empty TraceID")
	}
	if _, err := os.Stat(rt.Dir); err != nil {
		t.Fatalf("trace dir not created: %v", err)
	}
	// Verify ID format: YYYYMMDDTHHMMSSZ-XXXX
	parts := strings.SplitN(rt.TraceID, "-", 2)
	if len(parts) != 2 || len(parts[1]) != 4 {
		t.Fatalf("unexpected trace ID format: %s", rt.TraceID)
	}
	if !strings.HasSuffix(parts[0], "Z") {
		t.Fatalf("timestamp missing Z suffix: %s", parts[0])
	}
}

func TestWriteSession(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()
	content := []byte("session content here")
	rt.WriteSession(content)

	got, err := os.ReadFile(filepath.Join(rt.Dir, "session.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestWriteStage(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()

	data := Stage1Trace{Passed: true, Filters: map[string]FilterTrace{
		"duration": {Value: 5.0, Threshold: 2.0, Passed: true},
	}, DurationMs: 42}

	rt.WriteStage("stage1-filter", data)

	got, err := os.ReadFile(filepath.Join(rt.Dir, "stage1-filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed Stage1Trace
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestWriteRaw(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()
	payload := []byte(`{"prompt":"hello"}`)
	rt.WriteRaw("stage2-scoring", "req", payload)

	f, err := os.Open(filepath.Join(rt.Dir, "stage2-scoring.req.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestWriteSkill(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()
	rt.WriteSkill("docker-compose", []byte("# Docker Compose"))

	got, err := os.ReadFile(filepath.Join(rt.Dir, "skills", "docker-compose.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Docker Compose" {
		t.Fatalf("unexpected content: %s", got)
	}
}

func TestWriteSummary(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	rt := tr.StartRun()

	sum := Summary{TraceID: rt.TraceID, Result: "extracted", SkillsExtracted: 1}
	rt.WriteSummary(sum)

	got, err := os.ReadFile(filepath.Join(rt.Dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed Summary
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Result != "extracted" {
		t.Fatalf("unexpected result: %s", parsed.Result)
	}
}

func TestConcurrentStartRun(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracer(dir)
	var wg sync.WaitGroup
	traces := make([]*RunTrace, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			traces[idx] = tr.StartRun()
		}(i)
	}
	wg.Wait()

	ids := make(map[string]bool)
	for _, rt := range traces {
		if rt == nil {
			t.Fatal("got nil RunTrace in concurrent test")
		}
		if ids[rt.TraceID] {
			t.Fatalf("duplicate trace ID: %s", rt.TraceID)
		}
		ids[rt.TraceID] = true
	}
}

func TestCollisionRetry(t *testing.T) {
	// We can't easily force a collision, but we verify that StartRun
	// succeeds even when called rapidly (same second).
	dir := t.TempDir()
	tr := NewTracer(dir)
	r1 := tr.StartRun()
	r2 := tr.StartRun()
	if r1 == nil || r2 == nil {
		t.Fatal("expected both traces to succeed")
	}
	if r1.TraceID == r2.TraceID {
		t.Fatal("expected different trace IDs")
	}
}
