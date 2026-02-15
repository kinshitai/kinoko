package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print pipeline metrics",
	Long:  `Query the database and print extraction pipeline metrics: sessions processed, extraction yield, skills by category, quality scores, decay distribution.`,
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

	db := store.DB()

	// Sessions
	fmt.Println("=== Sessions ===")
	var totalSessions, extracted, rejected, errored int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&totalSessions); err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'extracted'`).Scan(&extracted); err != nil {
		return fmt.Errorf("query extracted: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'rejected'`).Scan(&rejected); err != nil {
		return fmt.Errorf("query rejected: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE extraction_status = 'error'`).Scan(&errored); err != nil {
		return fmt.Errorf("query errored: %w", err)
	}

	fmt.Printf("  Total:     %d\n", totalSessions)
	fmt.Printf("  Extracted: %d\n", extracted)
	fmt.Printf("  Rejected:  %d\n", rejected)
	fmt.Printf("  Errors:    %d\n", errored)
	if totalSessions > 0 {
		fmt.Printf("  Yield:     %.1f%%\n", float64(extracted)/float64(totalSessions)*100)
	}

	// Skills by category
	fmt.Println("\n=== Skills by Category ===")
	rows, err := db.Query(`SELECT category, COUNT(*) FROM skills GROUP BY category ORDER BY category`)
	if err != nil {
		return fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	totalSkills := 0
	for rows.Next() {
		var cat string
		var count int
		if err := rows.Scan(&cat, &count); err != nil {
			return fmt.Errorf("scan skill category: %w", err)
		}
		fmt.Printf("  %-15s %d\n", cat, count)
		totalSkills += count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate skill categories: %w", err)
	}
	fmt.Printf("  %-15s %d\n", "TOTAL", totalSkills)

	// Quality scores
	fmt.Println("\n=== Quality Scores (avg) ===")
	var avgComposite, avgConfidence sql.NullFloat64
	if err := db.QueryRow(`SELECT AVG(q_composite_score), AVG(q_critic_confidence) FROM skills`).Scan(&avgComposite, &avgConfidence); err != nil {
		return fmt.Errorf("query quality scores: %w", err)
	}
	if avgComposite.Valid {
		fmt.Printf("  Composite:  %.2f\n", avgComposite.Float64)
		fmt.Printf("  Confidence: %.2f\n", avgConfidence.Float64)
	} else {
		fmt.Println("  (no skills)")
	}

	// Decay distribution
	fmt.Println("\n=== Decay Distribution ===")
	type bucket struct {
		label string
		min   float64
		max   float64
	}
	buckets := []bucket{
		{"dead (0.00)", 0.0, 0.001},
		{"low (0.00-0.25)", 0.001, 0.25},
		{"medium (0.25-0.50)", 0.25, 0.50},
		{"high (0.50-0.75)", 0.50, 0.75},
		{"fresh (0.75-1.00)", 0.75, 1.01},
	}
	for _, b := range buckets {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM skills WHERE decay_score >= ? AND decay_score < ?`, b.min, b.max).Scan(&count); err != nil {
			return fmt.Errorf("query decay bucket %s: %w", b.label, err)
		}
		fmt.Printf("  %-20s %d\n", b.label, count)
	}

	// Injection events
	fmt.Println("\n=== Injection Events ===")
	var injCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM injection_events`).Scan(&injCount); err != nil {
		return fmt.Errorf("query injection events: %w", err)
	}
	fmt.Printf("  Total: %d\n", injCount)

	return nil
}
