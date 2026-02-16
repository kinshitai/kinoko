package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAPIPort_Default(t *testing.T) {
	cfg := DefaultConfig()
	want := cfg.Server.Port + 1
	if got := cfg.Server.GetAPIPort(); got != want {
		t.Fatalf("GetAPIPort() = %d, want %d", got, want)
	}
}

func TestGetAPIPort_Explicit(t *testing.T) {
	s := ServerConfig{Port: 8000, APIPort: 9000}
	if got := s.GetAPIPort(); got != 9000 {
		t.Fatalf("GetAPIPort() = %d, want 9000", got)
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	// Empty path should use default location; since that file likely doesn't
	// exist, it returns defaults.
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 23231 {
		t.Fatalf("expected default port, got %d", cfg.Server.Port)
	}
}

func TestSave_EmptyPath(t *testing.T) {
	cfg := DefaultConfig()
	// Save with empty path writes to default location. This may or may not work
	// depending on home dir permissions, but exercises the code path.
	err := cfg.Save("")
	// Clean up: don't leave files around.
	if err == nil {
		home, _ := os.UserHomeDir()
		os.Remove(filepath.Join(home, ".kinoko", "config.yaml"))
	}
}

func TestSave_AndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.yaml")

	cfg := DefaultConfig()
	cfg.Server.Port = 12345
	cfg.Server.Host = "0.0.0.0"

	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != 12345 {
		t.Fatalf("expected port 12345, got %d", loaded.Server.Port)
	}
	if loaded.Server.Host != "0.0.0.0" {
		t.Fatalf("expected host 0.0.0.0, got %s", loaded.Server.Host)
	}
}

func TestValidate_NegativeLibPriority(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Libraries[0].Priority = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative priority")
	}
}

func TestValidate_EmptyDSN(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Storage.DSN = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestValidate_BadConfidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MinConfidence = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}
}

func TestValidate_NegativeMinDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MinDurationMinutes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative min duration")
	}
}

func TestValidate_NegativeMaxDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MaxDurationMinutes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative max duration")
	}
}

func TestValidate_MinGtMaxDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MinDurationMinutes = 200
	cfg.Extraction.MaxDurationMinutes = 100
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for min > max duration")
	}
}

func TestValidate_NegativeMinToolCalls(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MinToolCalls = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative min tool calls")
	}
}

func TestValidate_BadMaxErrorRate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.MaxErrorRate = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for max_error_rate > 1")
	}
}

func TestValidate_BadNoveltyMin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.NoveltyMinDistance = -0.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative novelty_min_distance")
	}
}

func TestValidate_BadNoveltyMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.NoveltyMaxDistance = 1.5
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for novelty_max_distance > 1")
	}
}

func TestValidate_NoveltyMinGtMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.NoveltyMinDistance = 0.9
	cfg.Extraction.NoveltyMaxDistance = 0.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for novelty min > max")
	}
}

func TestValidate_BadVersionSimilarity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Extraction.VersionSimilarityThreshold = -0.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative version_similarity_threshold")
	}
}

func TestValidate_BadDefaultsConfidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.Confidence = 2.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for defaults confidence > 1")
	}
}

func TestExpandPath_AbsolutePath(t *testing.T) {
	if got := expandPath("/absolute/path"); got != "/absolute/path" {
		t.Fatalf("expandPath(/absolute/path) = %q", got)
	}
}

func TestExpandPath_HomeVar(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got := expandPath("~/test")
	if got != filepath.Join(home, "test") {
		t.Fatalf("expandPath(~/test) = %q, want %q", got, filepath.Join(home, "test"))
	}
}

func TestLoad_FileReadError(t *testing.T) {
	// Create a directory where a file is expected.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.Mkdir(path, 0755) // path is a dir, not a file

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when config path is a directory")
	}
}
