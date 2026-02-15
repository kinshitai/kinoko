// Package metrics collects pipeline health metrics from the database:
// stage pass rates, extraction yield, injection utilization, A/B test
// statistical significance, quality score distributions, and decay buckets.
package metrics

import (
	"database/sql"
	"fmt"
	"math"
)

// PipelineMetrics holds all metrics from §3.1-3.2 and A/B results.
type PipelineMetrics struct {
	// Sessions
	TotalSessions    int
	Extracted        int
	Rejected         int
	Errored          int
	ExtractionYield  float64 // extracted / total
	// Stage pass rates (§3.1)
	Stage1Total      int
	Stage1Passed     int
	Stage1PassRate   float64
	Stage2Total      int
	Stage2Passed     int
	Stage2PassRate   float64
	Stage3Total      int
	Stage3Passed     int
	Stage3PassRate   float64
	// Human review precision (§3.1)
	HumanReviewTotal    int
	HumanReviewUseful   int
	ExtractionPrecision float64 // useful / total reviewed
	// Injection metrics (§3.2)
	InjectionEvents       int
	SessionsWithInjection int
	InjectionRate         float64 // sessions with injection / total sessions
	TotalSkills           int
	InjectedDistinctSkills int
	SkillUtilization      float64 // distinct injected / total skills
	// Skills by category
	SkillsByCategory map[string]int
	// Quality
	AvgComposite   float64
	AvgConfidence  float64
	// Decay distribution
	DecayBuckets []DecayBucket
	// A/B test results (§3.3)
	AB *ABTestResult // nil if no A/B data
}

// DecayBucket holds a count for a decay score range.
type DecayBucket struct {
	Label string
	Min   float64
	Max   float64
	Count int
}

// ABTestResult holds per-group success rates and z-test.
type ABTestResult struct {
	TreatmentSessions int
	TreatmentSuccess  int
	TreatmentRate     float64
	ControlSessions   int
	ControlSuccess    int
	ControlRate       float64
	ZScore            float64
	PValue            float64
	Significant       bool // p < 0.05
	SufficientData    bool // true when both groups >= MinSampleSize
}

// Collector computes pipeline metrics from the database.
type Collector struct {
	db             *sql.DB
	minSampleSize  int // minimum sessions per A/B group before computing z-test
}

// NewCollector creates a Collector. Options: WithMinSampleSize.
func NewCollector(db *sql.DB, opts ...CollectorOption) *Collector {
	c := &Collector{db: db, minSampleSize: 100}
	for _, o := range opts {
		o(c)
	}
	return c
}

// CollectorOption configures a Collector.
type CollectorOption func(*Collector)

// WithMinSampleSize sets the minimum sessions per A/B group for z-test computation.
func WithMinSampleSize(n int) CollectorOption {
	return func(c *Collector) {
		if n > 0 {
			c.minSampleSize = n
		}
	}
}

// Collect gathers all pipeline metrics.
func (c *Collector) Collect() (*PipelineMetrics, error) {
	m := &PipelineMetrics{
		SkillsByCategory: make(map[string]int),
	}

	// Sessions
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&m.TotalSessions); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'extracted'`).Scan(&m.Extracted); err != nil {
		return nil, fmt.Errorf("count extracted: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'rejected'`).Scan(&m.Rejected); err != nil {
		return nil, fmt.Errorf("count rejected: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'error'`).Scan(&m.Errored); err != nil {
		return nil, fmt.Errorf("count errored: %w", err)
	}
	if m.TotalSessions > 0 {
		m.ExtractionYield = float64(m.Extracted) / float64(m.TotalSessions)
	}

	// Stage pass rates: rejected_at_stage tells us where sessions were rejected.
	// Exclude error sessions — only count pending→extracted and pending→rejected flows.
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status NOT IN ('pending', 'error')`).Scan(&m.Stage1Total); err != nil {
		return nil, fmt.Errorf("stage1 total: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status NOT IN ('pending', 'error') AND (rejected_at_stage > 1 OR rejected_at_stage = 0)`).Scan(&m.Stage1Passed); err != nil {
		return nil, fmt.Errorf("stage1 passed: %w", err)
	}
	if m.Stage1Total > 0 {
		m.Stage1PassRate = float64(m.Stage1Passed) / float64(m.Stage1Total)
	}

	// Stage 2: total = stage1 passed, passed = those past stage 2
	m.Stage2Total = m.Stage1Passed
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status NOT IN ('pending', 'error') AND (rejected_at_stage > 2 OR rejected_at_stage = 0)`).Scan(&m.Stage2Passed); err != nil {
		return nil, fmt.Errorf("stage2 passed: %w", err)
	}
	if m.Stage2Total > 0 {
		m.Stage2PassRate = float64(m.Stage2Passed) / float64(m.Stage2Total)
	}

	// Stage 3: total = stage2 passed, passed = extracted
	m.Stage3Total = m.Stage2Passed
	m.Stage3Passed = m.Extracted
	if m.Stage3Total > 0 {
		m.Stage3PassRate = float64(m.Stage3Passed) / float64(m.Stage3Total)
	}

	// Human review precision (§3.4).
	// Verdicts: 'agree', 'disagree_should_extract', 'disagree_should_reject'.
	// Precision = agree / (agree + disagree_should_reject).
	// Recall = agree / (agree + disagree_should_extract).
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM human_review_samples WHERE verdict IN ('agree', 'disagree_should_extract', 'disagree_should_reject')`).Scan(&m.HumanReviewTotal); err != nil {
		return nil, fmt.Errorf("review total: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM human_review_samples WHERE verdict = 'agree'`).Scan(&m.HumanReviewUseful); err != nil {
		return nil, fmt.Errorf("review agree: %w", err)
	}
	// Precision denominator: agree + disagree_should_reject (i.e. total minus disagree_should_extract).
	var disagreeReject int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM human_review_samples WHERE verdict = 'disagree_should_reject'`).Scan(&disagreeReject); err != nil {
		return nil, fmt.Errorf("review disagree_should_reject: %w", err)
	}
	precisionDenom := m.HumanReviewUseful + disagreeReject
	if precisionDenom > 0 {
		m.ExtractionPrecision = float64(m.HumanReviewUseful) / float64(precisionDenom)
	}

	// Injection metrics
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM injection_events`).Scan(&m.InjectionEvents); err != nil {
		return nil, fmt.Errorf("injection events: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM injection_events WHERE delivered = 1`).Scan(&m.SessionsWithInjection); err != nil {
		return nil, fmt.Errorf("sessions with injection: %w", err)
	}
	if m.TotalSessions > 0 {
		m.InjectionRate = float64(m.SessionsWithInjection) / float64(m.TotalSessions)
	}

	if err := c.db.QueryRow(`SELECT COUNT(*) FROM skills`).Scan(&m.TotalSkills); err != nil {
		return nil, fmt.Errorf("total skills: %w", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT skill_id) FROM injection_events`).Scan(&m.InjectedDistinctSkills); err != nil {
		return nil, fmt.Errorf("distinct injected: %w", err)
	}
	if m.TotalSkills > 0 {
		m.SkillUtilization = float64(m.InjectedDistinctSkills) / float64(m.TotalSkills)
	}

	// Skills by category
	rows, err := c.db.Query(`SELECT category, COUNT(*) FROM skills GROUP BY category ORDER BY category`)
	if err != nil {
		return nil, fmt.Errorf("skills by category: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cat string
		var count int
		if err := rows.Scan(&cat, &count); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		m.SkillsByCategory[cat] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Quality averages
	var avgC, avgConf sql.NullFloat64
	if err := c.db.QueryRow(`SELECT AVG(q_composite_score), AVG(q_critic_confidence) FROM skills`).Scan(&avgC, &avgConf); err != nil {
		return nil, fmt.Errorf("quality avg: %w", err)
	}
	if avgC.Valid {
		m.AvgComposite = avgC.Float64
		m.AvgConfidence = avgConf.Float64
	}

	// Decay distribution
	buckets := []DecayBucket{
		{Label: "dead (0.00)", Min: 0.0, Max: 0.001},
		{Label: "low (0.00-0.25)", Min: 0.001, Max: 0.25},
		{Label: "medium (0.25-0.50)", Min: 0.25, Max: 0.50},
		{Label: "high (0.50-0.75)", Min: 0.50, Max: 0.75},
		{Label: "fresh (0.75-1.00)", Min: 0.75, Max: 1.01},
	}
	for i := range buckets {
		if err := c.db.QueryRow(`SELECT COUNT(*) FROM skills WHERE decay_score >= ? AND decay_score < ?`, buckets[i].Min, buckets[i].Max).Scan(&buckets[i].Count); err != nil {
			return nil, fmt.Errorf("decay bucket: %w", err)
		}
	}
	m.DecayBuckets = buckets

	// A/B test results
	ab, err := c.collectAB()
	if err != nil {
		return nil, fmt.Errorf("ab results: %w", err)
	}
	m.AB = ab

	return m, nil
}

func (c *Collector) collectAB() (*ABTestResult, error) {
	// Check if any A/B data exists.
	var abCount int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM injection_events WHERE ab_group != ''`).Scan(&abCount); err != nil {
		return nil, err
	}
	if abCount == 0 {
		return nil, nil
	}

	ab := &ABTestResult{}

	// Count distinct sessions per group.
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM injection_events WHERE ab_group = 'treatment'`).Scan(&ab.TreatmentSessions); err != nil {
		return nil, err
	}
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM injection_events WHERE ab_group = 'control'`).Scan(&ab.ControlSessions); err != nil {
		return nil, err
	}

	// Success = sessions where session_outcome = 'success'.
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM injection_events WHERE ab_group = 'treatment' AND session_outcome = 'success'`).Scan(&ab.TreatmentSuccess); err != nil {
		return nil, err
	}
	if err := c.db.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM injection_events WHERE ab_group = 'control' AND session_outcome = 'success'`).Scan(&ab.ControlSuccess); err != nil {
		return nil, err
	}

	if ab.TreatmentSessions > 0 {
		ab.TreatmentRate = float64(ab.TreatmentSuccess) / float64(ab.TreatmentSessions)
	}
	if ab.ControlSessions > 0 {
		ab.ControlRate = float64(ab.ControlSuccess) / float64(ab.ControlSessions)
	}

	// Only compute z-test when both groups have sufficient data.
	ab.SufficientData = ab.TreatmentSessions >= c.minSampleSize && ab.ControlSessions >= c.minSampleSize
	if ab.SufficientData {
		ab.ZScore, ab.PValue = TwoProportionZTest(
			ab.TreatmentSuccess, ab.TreatmentSessions,
			ab.ControlSuccess, ab.ControlSessions,
		)
		ab.Significant = ab.PValue < 0.05
	}

	return ab, nil
}

// TwoProportionZTest computes the z-score and two-tailed p-value for
// comparing two proportions (treatment vs control).
func TwoProportionZTest(success1, n1, success2, n2 int) (z, p float64) {
	if n1 == 0 || n2 == 0 {
		return 0, 1
	}
	p1 := float64(success1) / float64(n1)
	p2 := float64(success2) / float64(n2)
	// Pooled proportion.
	pPool := float64(success1+success2) / float64(n1+n2)

	se := math.Sqrt(pPool * (1 - pPool) * (1.0/float64(n1) + 1.0/float64(n2)))
	if se == 0 {
		return 0, 1
	}
	z = (p1 - p2) / se
	// Two-tailed p-value using normal approximation.
	p = 2 * normalCDF(-math.Abs(z))
	return z, p
}

// normalCDF computes the cumulative distribution function of the standard
// normal distribution using the Abramowitz & Stegun approximation.
func normalCDF(x float64) float64 {
	// Handbook of Mathematical Functions, formula 26.2.17.
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)
	sign := 1.0
	if x < 0 {
		sign = -1.0
	}
	x = math.Abs(x) / math.Sqrt(2)
	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)
	return 0.5 * (1.0 + sign*y)
}
