package main

import (
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/metrics"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print pipeline metrics",
	Long:  `Query the database and print extraction pipeline metrics: stage pass rates, extraction yield, injection metrics, A/B test results, quality scores, decay distribution.`,
	RunE:  runStats,
}

var statsConfigPath string

func init() {
	statsCmd.Flags().StringVar(&statsConfigPath, "config", "", "Config file path")
}

func runStats(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(statsConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	_ = logger

	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	collector := metrics.NewCollector(store.DB())
	m, err := collector.Collect()
	if err != nil {
		return fmt.Errorf("collect metrics: %w", err)
	}

	// Sessions
	fmt.Println("=== Sessions ===")
	fmt.Printf("  Total:     %d\n", m.TotalSessions)
	fmt.Printf("  Extracted: %d\n", m.Extracted)
	fmt.Printf("  Rejected:  %d\n", m.Rejected)
	fmt.Printf("  Errors:    %d\n", m.Errored)
	if m.TotalSessions > 0 {
		fmt.Printf("  Yield:     %.1f%%\n", m.ExtractionYield*100)
	}

	// Stage pass rates
	fmt.Println("\n=== Stage Pass Rates ===")
	fmt.Printf("  Stage 1: %d / %d (%.1f%%)\n", m.Stage1Passed, m.Stage1Total, m.Stage1PassRate*100)
	fmt.Printf("  Stage 2: %d / %d (%.1f%%)\n", m.Stage2Passed, m.Stage2Total, m.Stage2PassRate*100)
	fmt.Printf("  Stage 3: %d / %d (%.1f%%)\n", m.Stage3Passed, m.Stage3Total, m.Stage3PassRate*100)

	// Human review
	if m.HumanReviewTotal > 0 {
		fmt.Println("\n=== Extraction Precision ===")
		fmt.Printf("  Reviewed: %d, Useful: %d, Precision: %.1f%%\n", m.HumanReviewTotal, m.HumanReviewUseful, m.ExtractionPrecision*100)
	}

	// Injection
	fmt.Println("\n=== Injection Metrics ===")
	fmt.Printf("  Events:       %d\n", m.InjectionEvents)
	fmt.Printf("  Sessions:     %d / %d (rate: %.1f%%)\n", m.SessionsWithInjection, m.TotalSessions, m.InjectionRate*100)
	fmt.Printf("  Utilization:  %d / %d skills (%.1f%%)\n", m.InjectedDistinctSkills, m.TotalSkills, m.SkillUtilization*100)

	// A/B
	if m.AB != nil {
		fmt.Println("\n=== A/B Test Results ===")
		fmt.Printf("  Treatment: %d sessions, %d success (%.1f%%)\n", m.AB.TreatmentSessions, m.AB.TreatmentSuccess, m.AB.TreatmentRate*100)
		fmt.Printf("  Control:   %d sessions, %d success (%.1f%%)\n", m.AB.ControlSessions, m.AB.ControlSuccess, m.AB.ControlRate*100)
		fmt.Printf("  Z-score:   %.3f\n", m.AB.ZScore)
		fmt.Printf("  P-value:   %.4f\n", m.AB.PValue)
		if m.AB.Significant {
			fmt.Println("  Result:    SIGNIFICANT (p < 0.05)")
		} else {
			fmt.Println("  Result:    not significant")
		}
	}

	// Skills by category
	fmt.Println("\n=== Skills by Category ===")
	cats := make([]string, 0, len(m.SkillsByCategory))
	for c := range m.SkillsByCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Printf("  %-15s %d\n", c, m.SkillsByCategory[c])
	}
	fmt.Printf("  %-15s %d\n", "TOTAL", m.TotalSkills)

	// Quality
	fmt.Println("\n=== Quality Scores (avg) ===")
	if m.TotalSkills > 0 {
		fmt.Printf("  Composite:  %.2f\n", m.AvgComposite)
		fmt.Printf("  Confidence: %.2f\n", m.AvgConfidence)
	} else {
		fmt.Println("  (no skills)")
	}

	// Decay
	fmt.Println("\n=== Decay Distribution ===")
	for _, b := range m.DecayBuckets {
		fmt.Printf("  %-20s %d\n", b.Label, b.Count)
	}

	return nil
}
