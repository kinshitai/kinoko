package decay

import (
	"testing"
	"time"
)

func TestSetNow(t *testing.T) {
	cfg := DefaultConfig()
	r, err := NewRunner(nil, nil, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	r.SetNow(func() time.Time { return fixed })
}
