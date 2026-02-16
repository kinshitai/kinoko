package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/sanitize"
)

// ErrCredentialsFound is returned when the scanner detects credentials.
var ErrCredentialsFound = errors.New("credentials detected")

var (
	scanDir    string
	scanStdin  bool
	scanReject bool
)

var scanCmd = &cobra.Command{
	Use:   "scan [file]",
	Short: "Scan files for credentials and secrets",
	Long: `Scan files for embedded credentials, API keys, tokens, and other secrets.

Examples:
  kinoko scan config.yaml          # Scan a single file
  kinoko scan --dir ./skills       # Scan a directory recursively
  kinoko scan --stdin              # Scan stdin (for git hooks)

Exit code 1 if high-confidence findings detected, 0 if clean.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringVar(&scanDir, "dir", "", "scan directory recursively")
	scanCmd.Flags().BoolVar(&scanStdin, "stdin", false, "read content from stdin")
	scanCmd.Flags().BoolVar(&scanReject, "reject", false, "exit 1 on findings (for git hooks)")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	scanner := sanitize.New(sanitize.WithRedactThreshold(0.7))
	var totalFindings int

	switch {
	case scanStdin:
		// P1-1: Limit stdin to 10 MB to prevent memory exhaustion.
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		findings := scanner.Scan(string(data))
		totalFindings = printFindings("<stdin>", findings)
	case scanDir != "":
		totalFindings = scanDirectory(scanner, scanDir)
	case len(args) > 0:
		for _, f := range args {
			data, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading %s: %v\n", f, err)
				continue
			}
			totalFindings += printFindings(f, scanner.Scan(string(data)))
		}
	default:
		return fmt.Errorf("specify a file, --dir, or --stdin")
	}

	if totalFindings > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠ %d credential(s) detected\n", totalFindings)
		// P0-1: Return error instead of os.Exit(1) so --reject works and
		// cleanup/deferred functions run properly.
		if scanReject {
			return fmt.Errorf("credentials detected, rejecting")
		}
		return ErrCredentialsFound
	}
	return nil
}

func scanDirectory(scanner *sanitize.Scanner, dir string) int {
	total := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip binary-looking files and .git
		if strings.Contains(path, "/.git/") {
			return filepath.SkipDir
		}
		if info.Size() > 1<<20 { // skip >1MB
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		total += printFindings(path, scanner.Scan(string(data)))
		return nil
	})
	return total
}

func printFindings(file string, findings []sanitize.Finding) int {
	highConf := 0
	for _, f := range findings {
		if f.Confidence < 0.5 {
			continue // skip informational
		}
		icon := "⚠"
		if f.Confidence >= 0.9 {
			icon = "🔴"
		}
		fmt.Fprintf(os.Stderr, "%s %s:%d  %s  (%s, confidence=%.0f%%)\n",
			icon, file, f.Line, f.Match, f.Type, f.Confidence*100)
		highConf++
	}
	return highConf
}
