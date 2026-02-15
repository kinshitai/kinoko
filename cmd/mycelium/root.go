package main

import (
	"github.com/spf13/cobra"
)

// Version is set by ldflags during build
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "mycelium",
	Short: "Mycelium - Knowledge sharing infrastructure for AI agents",
	Long: `Mycelium is infrastructure where every problem solved once is solved for everyone.
People work with agents. Agents extract what was learned. Other people's agents absorb it.
No one writes documentation. No one publishes anything. They just get better results.`,
	Version: Version,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(initCmd)
}