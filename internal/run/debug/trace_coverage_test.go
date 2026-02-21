package debug

import (
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
