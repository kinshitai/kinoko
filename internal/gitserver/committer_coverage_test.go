package gitserver

import (
	"errors"
	"sync"
	"testing"
)

func TestIsAlreadyExists(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("already exists"), true},
		{errors.New("repository exists"), true},
		{errors.New("not found"), false},
		{errors.New("random error"), false},
	}
	for _, tt := range tests {
		if got := isAlreadyExists(tt.err); got != tt.want {
			t.Errorf("isAlreadyExists(%q) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestSkillMutex(t *testing.T) {
	c := &GitCommitter{
		locks: sync.Map{},
	}
	mu1 := c.skillMutex("skill-a")
	mu2 := c.skillMutex("skill-a")
	mu3 := c.skillMutex("skill-b")

	if mu1 != mu2 {
		t.Fatal("expected same mutex for same skill")
	}
	if mu1 == mu3 {
		t.Fatal("expected different mutex for different skill")
	}
}

func TestNewGitCommitter(t *testing.T) {
	c := NewGitCommitter(GitCommitterConfig{
		DataDir: t.TempDir(),
	})
	if c == nil {
		t.Fatal("expected non-nil committer")
	}
	if c.logger == nil {
		t.Fatal("expected default logger to be set")
	}
}
