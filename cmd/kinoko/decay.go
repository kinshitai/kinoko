// decay.go implements the "kinoko decay" command — a server-side tool that
// runs a single decay cycle applying half-life scoring to all indexed skills.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

var decayCmd = &cobra.Command{
	Use:   "decay",
	Short: "Run one decay cycle",
	Long:  `Runs one decay cycle over all skills in a library. Applies half-life degradation based on category and rescues recently-used skills.`,
	RunE:  runDecay,
}

var (
	decayDryRun     bool
	decayLibrary    string
	decayConfigPath string
)

func init() {
	decayCmd.Flags().BoolVar(&decayDryRun, "dry-run", false, "Print what would change without writing")
	decayCmd.Flags().StringVar(&decayLibrary, "library", "", "Library ID (default: first configured library)")
	decayCmd.Flags().StringVar(&decayConfigPath, "config", "", "Config file path")
}

func runDecay(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(decayConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	libraryID := decayLibrary
	if libraryID == "" && len(cfg.Libraries) > 0 {
		libraryID = cfg.Libraries[0].Name
	}
	if libraryID == "" {
		return fmt.Errorf("no library specified and none configured")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	decayCfg := decayConfigFromYAML(cfg.Decay)

	if decayDryRun {
		return runDecayDryRun(cmd.Context(), store, decayCfg, libraryID, logger)
	}

	runner, err := decay.NewRunner(store, store, decayCfg, logger)
	if err != nil {
		return fmt.Errorf("create decay runner: %w", err)
	}
	result, err := runner.RunCycle(cmd.Context(), libraryID)
	if err != nil {
		return fmt.Errorf("decay cycle failed: %w", err)
	}

	fmt.Printf("Decay cycle complete for library %q\n", libraryID)
	fmt.Printf("  Processed:  %d\n", result.Processed)
	fmt.Printf("  Demoted:    %d\n", result.Demoted)
	fmt.Printf("  Deprecated: %d\n", result.Deprecated)
	fmt.Printf("  Rescued:    %d\n", result.Rescued)

	return nil
}

// runDecayDryRun simulates a decay cycle without writing changes.
func runDecayDryRun(ctx context.Context, store *storage.SQLiteStore, cfg decay.Config, libraryID string, logger *slog.Logger) error {
	// Use a no-op writer for dry run
	runner, err := decay.NewRunner(store, &noopDecayWriter{}, cfg, logger)
	if err != nil {
		return fmt.Errorf("create decay runner: %w", err)
	}
	result, err := runner.RunCycle(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("decay dry run failed: %w", err)
	}

	fmt.Printf("Decay dry run for library %q\n", libraryID)
	fmt.Printf("  Would process:   %d\n", result.Processed)
	fmt.Printf("  Would demote:    %d\n", result.Demoted)
	fmt.Printf("  Would deprecate: %d\n", result.Deprecated)
	fmt.Printf("  Would rescue:    %d\n", result.Rescued)

	return nil
}

// decayConfigFromYAML converts config.DecayConfig to decay.Config,
// falling back to defaults for zero values.
func decayConfigFromYAML(cfg config.DecayConfig) decay.Config {
	d := decay.DefaultConfig()
	if cfg.FoundationalHalfLifeDays > 0 {
		d.FoundationalHalfLifeDays = cfg.FoundationalHalfLifeDays
	}
	if cfg.TacticalHalfLifeDays > 0 {
		d.TacticalHalfLifeDays = cfg.TacticalHalfLifeDays
	}
	if cfg.ContextualHalfLifeDays > 0 {
		d.ContextualHalfLifeDays = cfg.ContextualHalfLifeDays
	}
	if cfg.DeprecationThreshold > 0 {
		d.DeprecationThreshold = cfg.DeprecationThreshold
	}
	if cfg.RescueBoost > 0 {
		d.RescueBoost = cfg.RescueBoost
	}
	if cfg.RescueWindowDays > 0 {
		d.RescueWindowDays = cfg.RescueWindowDays
	}
	return d
}

// Compile-time interface check.
var _ decay.SkillWriter = (*noopDecayWriter)(nil)

type noopDecayWriter struct{}

func (w *noopDecayWriter) UpdateDecay(_ context.Context, _ string, _ float64) error {
	return nil
}
