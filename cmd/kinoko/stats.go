package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print client pipeline metrics",
	Long:  `Print client-side pipeline metrics: queue status, extraction counts. Server-side skill stats are available via Soft Serve.`,
	RunE:  runStats,
}

func init() {
}

func runStats(cmd *cobra.Command, args []string) error {
	fmt.Println("Client pipeline stats — not yet implemented.")
	fmt.Println("Server skill stats available via: ssh <server> info")
	return nil
}
