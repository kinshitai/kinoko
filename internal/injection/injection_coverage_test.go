package injection

import (
	"log/slog"
	"testing"
)

func TestDefaultABConfig(t *testing.T) {
	cfg := DefaultABConfig()
	if cfg.Enabled {
		t.Fatal("expected Enabled=false by default")
	}
	if cfg.ControlRatio != 0.1 {
		t.Fatalf("ControlRatio = %f, want 0.1", cfg.ControlRatio)
	}
	if cfg.MinSampleSize != 100 {
		t.Fatalf("MinSampleSize = %d, want 100", cfg.MinSampleSize)
	}
}

func TestNew_NilLogger(t *testing.T) {
	// New with nil logger should use slog.Default.
	inj := New(nil, nil, nil, nil, nil)
	if inj == nil {
		t.Fatal("expected non-nil injector")
	}
}

func TestNew_WithLogger(t *testing.T) {
	logger := slog.Default()
	inj := New(nil, nil, nil, nil, logger)
	if inj == nil {
		t.Fatal("expected non-nil injector")
	}
}
