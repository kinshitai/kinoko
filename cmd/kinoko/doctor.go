package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate Kinoko setup including LLM access",
	Long: `Doctor validates your Kinoko installation by checking:
- Configuration file
- Database and cache directories  
- SSH key for git operations
- Server connectivity
- LLM credentials and connectivity`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor()
	},
}

// CheckResult represents the result of a single check
type CheckResult struct {
	Name    string
	Status  bool
	Message string
}

// runDoctor performs all setup validation checks
func runDoctor() error {
	fmt.Println("🍄 Kinoko Doctor")
	fmt.Println()

	var results []CheckResult

	// Get home directory for path resolution
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Check 1: Config file exists and parses
	configPath := filepath.Join(home, ".kinoko", "config.yaml")
	cfg, configErr := checkConfigFile(configPath)
	results = append(results, CheckResult{
		Name:    "Config file",
		Status:  configErr == nil,
		Message: configPath,
	})
	if configErr != nil {
		results[len(results)-1].Message = fmt.Sprintf("%s (%v)", configPath, configErr)
	}

	// Check 2: Queue database path writable
	var queuePath string
	if cfg != nil {
		queuePath = cfg.Client.GetQueueDSN()
	} else {
		queuePath = filepath.Join(home, ".kinoko", "queue.db")
	}
	queueErr := checkQueueDatabase(queuePath)
	results = append(results, CheckResult{
		Name:    "Queue database",
		Status:  queueErr == nil,
		Message: queuePath,
	})
	if queueErr != nil {
		results[len(results)-1].Message = fmt.Sprintf("%s (%v)", queuePath, queueErr)
	}

	// Check 3: Cache directory exists
	cachePath := filepath.Join(home, ".kinoko", "cache")
	cacheErr := checkCacheDirectory(cachePath)
	results = append(results, CheckResult{
		Name:    "Cache directory",
		Status:  cacheErr == nil,
		Message: cachePath + "/",
	})
	if cacheErr != nil {
		results[len(results)-1].Message = fmt.Sprintf("%s/ (%v)", cachePath, cacheErr)
	}

	// Check 4: SSH key exists
	sshKeyPath := filepath.Join(home, ".kinoko", "id_ed25519")
	sshErr := checkSSHKey(sshKeyPath)
	results = append(results, CheckResult{
		Name:    "SSH key",
		Status:  sshErr == nil,
		Message: sshKeyPath,
	})
	if sshErr != nil {
		results[len(results)-1].Message = fmt.Sprintf("%s (%v)", sshKeyPath, sshErr)
	}

	// Check 5: Server reachable
	var serverURL string
	if cfg != nil {
		serverURL = fmt.Sprintf("localhost:%d", cfg.Server.GetAPIPort())
	} else {
		serverURL = "localhost:23233"
	}
	serverErr := checkServer(serverURL)
	results = append(results, CheckResult{
		Name:    "Server",
		Status:  serverErr == nil,
		Message: serverURL,
	})
	if serverErr != nil {
		results[len(results)-1].Message = fmt.Sprintf("%s (%v)", serverURL, serverErr)
	}

	// Check 6: LLM credentials resolvable
	var creds *llm.Credentials
	var credsErr error
	if cfg != nil {
		creds, credsErr = llm.ResolveCredentials(cfg.LLM)
	} else {
		creds, credsErr = llm.ResolveCredentials(config.LLMConfig{})
	}

	credStatus := credsErr == nil
	var credMessage string
	if credStatus {
		// Determine credential source
		var source string
		switch {
		case strings.HasPrefix(creds.APIKey, "sk-ant-api03-"):
			source = "API key"
		case strings.HasPrefix(creds.APIKey, "sk-ant-oat01-"):
			source = "setup token"
		case creds.Provider == "claude-cli":
			source = "CLI"
		case creds.APIKey == "" && creds.BaseURL != "":
			source = "proxy"
		default:
			source = "OAuth"
		}
		credMessage = fmt.Sprintf("%s (%s)", getProviderDisplayName(creds.Provider), source)
	} else {
		credMessage = fmt.Sprintf("(%v)", credsErr)
	}

	results = append(results, CheckResult{
		Name:    "LLM credentials",
		Status:  credStatus,
		Message: credMessage,
	})

	// Check 7: LLM connectivity (with latency measurement)
	var llmErr error
	var latency time.Duration
	if credStatus && creds != nil {
		latency, llmErr = checkLLMConnectivity(creds)
	} else {
		llmErr = fmt.Errorf("no credentials to test")
	}

	llmStatus := llmErr == nil
	var llmMessage string
	if llmStatus {
		llmMessage = fmt.Sprintf("%s (%dms)", creds.Model, latency.Milliseconds())
	} else {
		llmMessage = fmt.Sprintf("(%v)", llmErr)
	}

	results = append(results, CheckResult{
		Name:    "LLM connectivity",
		Status:  llmStatus,
		Message: llmMessage,
	})

	// Print results
	for _, result := range results {
		status := "✓"
		if !result.Status {
			status = "✗"
		}
		fmt.Printf("  %s %-16s %s\n", status, result.Name, result.Message)
	}

	// Count issues
	issues := 0
	for _, result := range results {
		if !result.Status {
			issues++
		}
	}

	fmt.Println()
	if issues == 0 {
		fmt.Println("  All checks passed! ✓")
		return nil
	} else {
		if issues == 1 {
			fmt.Println("  1 issue found. Run 'kinoko serve' to start the server.")
		} else {
			fmt.Printf("  %d issues found. Run 'kinoko init' to configure missing components.\n", issues)
		}
		// Return error to set exit code 1
		os.Exit(1)
	}

	return nil
}

// checkConfigFile validates that the config file exists and can be parsed
func checkConfigFile(configPath string) (*config.Config, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("not found")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("parse error")
	}

	return cfg, nil
}

// checkQueueDatabase validates that the queue database path is writable
func checkQueueDatabase(queuePath string) error {
	// Ensure parent directory exists
	queueDir := filepath.Dir(queuePath)
	if err := os.MkdirAll(queueDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory")
	}

	// Try to create a temp file in the same directory to test writability
	tempFile := filepath.Join(queueDir, ".kinoko-doctor-test")
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("not writable")
	}
	file.Close()
	os.Remove(tempFile)

	return nil
}

// checkCacheDirectory validates that the cache directory exists
func checkCacheDirectory(cachePath string) error {
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(cachePath, 0755); err != nil {
			return fmt.Errorf("cannot create")
		}
	}
	return nil
}

// checkSSHKey validates that the SSH key exists
func checkSSHKey(keyPath string) error {
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("not found")
	}
	return nil
}

// checkServer validates that the kinoko server is reachable
func checkServer(serverURL string) error {
	// Try HTTP health check with 2 second timeout
	client := &http.Client{Timeout: 2 * time.Second}

	// Try both HTTP protocols
	urls := []string{
		fmt.Sprintf("http://%s/health", serverURL),
		fmt.Sprintf("http://%s/api/health", serverURL),
	}

	var lastErr error
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// If all attempts failed, return connection refused (most likely error)
	if lastErr != nil && strings.Contains(lastErr.Error(), "connection refused") {
		return fmt.Errorf("connection refused")
	}
	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("unreachable")
}

// checkLLMConnectivity tests LLM connectivity and measures latency
func checkLLMConnectivity(creds *llm.Credentials) (time.Duration, error) {
	start := time.Now()

	// Reuse the testCredentials function from wizard.go
	if err := testCredentials(creds); err != nil {
		return 0, err
	}

	latency := time.Since(start)
	return latency, nil
}
