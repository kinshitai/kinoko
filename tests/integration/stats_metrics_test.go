//go:build integration

package integration

import (
	"testing"

	"github.com/kinoko-dev/kinoko/internal/metrics"
)

func TestStatsCollectorReturnsMetrics(t *testing.T) {
	store := newTestStore(t)

	collector := metrics.NewCollector(store.DB())
	m, err := collector.Collect()
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
}
