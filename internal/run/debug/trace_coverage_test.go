package debug

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStarted_BeforeAndAfterStartRun(t *testing.T) {
	// Nil RunTrace returns zero time.
	var rt *RunTrace
	if !rt.Started().IsZero() {
		t.Fatal("expected zero time from nil RunTrace")
	}

	// After StartRun, Started returns a non-zero time.
	tr := NewTracer(t.TempDir())
	rt = tr.StartRun()
	if rt == nil {
		t.Fatal("expected non-nil RunTrace")
	}
	s := rt.Started()
	if s.IsZero() {
		t.Fatal("expected non-zero started time")
	}
	if time.Since(s) > 5*time.Second {
		t.Fatal("started time too far in the past")
	}
}

func TestBestEffortWrite_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")
	bestEffortWrite(path, data)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}

	// Verify permissions are 0600.
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestBestEffortWrite_BadPath(t *testing.T) {
	// Writing to a non-existent directory should not panic (best-effort).
	bestEffortWrite("/nonexistent/dir/file.txt", []byte("data"))
}

func TestStartRun_BadBaseDir(t *testing.T) {
	// Use a path that can't be created.
	tr := NewTracer("/dev/null/impossible")
	rt := tr.StartRun()
	if rt != nil {
		t.Fatal("expected nil RunTrace for unwritable base dir")
	}
}

func TestRandomHex_Length(t *testing.T) {
	h, err := randomHex(8)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 8 {
		t.Fatalf("expected length 8, got %d", len(h))
	}
}

func TestWriteStage_UnmarshalableData(t *testing.T) {
	tr := NewTracer(t.TempDir())
	rt := tr.StartRun()
	// chan is not JSON-marshalable; should log error but not panic.
	rt.WriteStage("bad", make(chan int))

	// Verify file was NOT created.
	_, err := os.Stat(filepath.Join(rt.Dir, "bad.json"))
	if err == nil {
		t.Fatal("expected no file for unmarshalable data")
	}
}

func TestWriteSummary_UnmarshalableData(t *testing.T) {
	tr := NewTracer(t.TempDir())
	rt := tr.StartRun()
	rt.WriteSummary(make(chan int))

	_, err := os.Stat(filepath.Join(rt.Dir, "summary.json"))
	if err == nil {
		t.Fatal("expected no file for unmarshalable data")
	}
}

func TestStartRun_ProcFakedir(t *testing.T) {
	tr := NewTracer("/proc/fakedir")
	rt := tr.StartRun()
	if rt != nil {
		t.Fatal("expected nil RunTrace for /proc/fakedir base dir")
	}
}

func TestWriteRaw_ValidGzip(t *testing.T) {
	tr := NewTracer(t.TempDir())
	rt := tr.StartRun()
	if rt == nil {
		t.Fatal("expected non-nil RunTrace")
	}

	payload := []byte(`{"model":"gpt-4","prompt":"test"}`)
	rt.WriteRaw("llm", "request", payload)

	path := filepath.Join(rt.Dir, "llm.request.json.gz")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected .gz file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty .gz file")
	}

	// Verify it's valid gzip with correct content.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not valid gzip: %v", err)
	}
	defer gz.Close()

	got, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}

	// Verify JSON is intact.
	var parsed map[string]string
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("gzip content is not valid JSON: %v", err)
	}
	if parsed["model"] != "gpt-4" {
		t.Fatalf("unexpected model: %s", parsed["model"])
	}
}

func TestWriteSkill_ContentVerification(t *testing.T) {
	tr := NewTracer(t.TempDir())
	rt := tr.StartRun()
	if rt == nil {
		t.Fatal("expected non-nil RunTrace")
	}

	content := []byte("# My Skill\n\nThis skill does things.\n")
	rt.WriteSkill("my-skill", content)

	path := filepath.Join(rt.Dir, "skills", "my-skill.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected skill file to exist: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("got %q, want %q", got, content)
	}

	// Verify permissions are 0600.
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestWriteSkill_BadDir(t *testing.T) {
	// RunTrace with a directory that can't have subdirs created.
	rt := &RunTrace{TraceID: "test", Dir: "/dev/null/impossible"}
	// Should not panic.
	rt.WriteSkill("skill1", []byte("content"))
}

func TestWriteRaw_BadDir(t *testing.T) {
	rt := &RunTrace{TraceID: "test", Dir: "/dev/null/impossible"}
	// Should not panic.
	rt.WriteRaw("stage", "req", []byte("data"))
}
