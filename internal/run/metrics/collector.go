// Package metrics collects client-side pipeline health metrics.
// Currently a placeholder — will be rebuilt for client queue stats.
package metrics

import (
	"database/sql"
)

// PipelineMetrics holds client-side pipeline metrics.
// Currently empty — to be populated with queue/extraction stats.
type PipelineMetrics struct{}

// Collector computes pipeline metrics from the client database.
type Collector struct {
	db *sql.DB
}

// NewCollector creates a Collector.
func NewCollector(db *sql.DB) *Collector {
	return &Collector{db: db}
}

// Collect gathers client-side pipeline metrics.
func (c *Collector) Collect() (*PipelineMetrics, error) {
	return &PipelineMetrics{}, nil
}
