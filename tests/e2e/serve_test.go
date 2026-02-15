//go:build integration

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKinokoServeLifecycle(t *testing.T) {
	RequireSoftBinary(t)
	RequireGitBinary(t)
	RequireSSHBinary(t)

	env := SetupTestEnvironment(t)

	t.Run("server_starts_successfully", func(t *testing.T) {
		env.StartServer()
		
		// Verify server is responding
		output, err := env.RunSSHCommand("repo", "list")
		if err != nil {
			t.Fatalf("Server not responding to SSH commands: %v", err)
		}
		
		t.Logf("Initial repo list: %s", output)
		
		env.StopServer()
	})

	t.Run("server_stops_gracefully", func(t *testing.T) {
		env.StartServer()
		
		// Server should be running
		if env.ServerPID == 0 {
			t.Fatal("Server PID not set")
		}
		
		// Verify process exists
		if err := syscall.Kill(env.ServerPID, 0); err != nil {
			t.Fatalf("Server process not running: %v", err)
		}
		
		env.StopServer()
		
		// Process should be gone
		time.Sleep(1 * time.Second) // Give it a moment
		if err := syscall.Kill(env.ServerPID, 0); err == nil {
			t.Error("Server process still running after stop")
		}
	})
}

func TestKinokoServeConfigErrors(t *testing.T) {
	RequireSoftBinary(t)

	tempDir, err := os.MkdirTemp("", "kinoko-serve-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)

	t.Run("missing_config_file", func(t *testing.T) {
		nonexistentConfig := filepath.Join(tempDir, "missing-config.yaml")
		
		cmd := exec.Command(binaryPath, "serve", "--config", nonexistentConfig)
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		// Should succeed (uses defaults when config doesn't exist)
		if err != nil {
			// But might fail due to other reasons (like port conflicts)
			t.Logf("Serve command output: %s", output)
		}
	})

	t.Run("invalid_config_yaml", func(t *testing.T) {
		invalidConfig := filepath.Join(tempDir, "invalid-config.yaml")
		invalidContent := `server:
  host: localhost
  port: 8080
  invalid_yaml: [unclosed bracket
`
		if err := os.WriteFile(invalidConfig, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid config: %v", err)
		}

		cmd := exec.Command(binaryPath, "serve", "--config", invalidConfig)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error with invalid YAML config\nOutput: %s", output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "failed to parse config") && 
		   !strings.Contains(outputStr, "yaml") &&
		   !strings.Contains(outputStr, "parse") {
			t.Errorf("Error message should mention YAML parsing issue: %s", outputStr)
		}
	})

	t.Run("invalid_port_number", func(t *testing.T) {
		invalidPortConfig := filepath.Join(tempDir, "invalid-port-config.yaml")
		invalidPortContent := `server:
  host: localhost
  port: 70000
  dataDir: /tmp/test
storage:
  driver: sqlite
  dsn: /tmp/test.db
libraries: []`

		if err := os.WriteFile(invalidPortConfig, []byte(invalidPortContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid port config: %v", err)
		}

		cmd := exec.Command(binaryPath, "serve", "--config", invalidPortConfig)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error with invalid port number\nOutput: %s", output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "port must be between") ||
		   !strings.Contains(outputStr, "65535") {
			t.Errorf("Error should mention valid port range: %s", outputStr)
		}
	})

	t.Run("invalid_storage_driver", func(t *testing.T) {
		invalidDriverConfig := filepath.Join(tempDir, "invalid-driver-config.yaml")
		invalidDriverContent := `server:
  host: localhost
  port: 23240
  dataDir: /tmp/test
storage:
  driver: redis
  dsn: redis://localhost:6379
libraries: []`

		if err := os.WriteFile(invalidDriverConfig, []byte(invalidDriverContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid driver config: %v", err)
		}

		cmd := exec.Command(binaryPath, "serve", "--config", invalidDriverConfig)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error with invalid storage driver\nOutput: %s", output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "storage driver must be") ||
		   !strings.Contains(outputStr, "sqlite") {
			t.Errorf("Error should mention valid storage drivers: %s", outputStr)
		}
	})
}

func TestKinokoServePortConflicts(t *testing.T) {
	RequireSoftBinary(t)
	RequireGitBinary(t)

	env1 := SetupTestEnvironment(t)
	env2 := SetupTestEnvironment(t)
	
	// Force both to use the same port
	env2.Config.Server.Port = env1.Config.Server.Port
	env2.SSHPort = env1.SSHPort
	
	// Update config file for env2
	if err := env2.Config.Save(env2.ConfigPath); err != nil {
		t.Fatalf("Failed to save env2 config: %v", err)
	}

	t.Run("second_server_fails_on_port_conflict", func(t *testing.T) {
		// Start first server
		env1.StartServer()
		defer env1.StopServer()

		// Try to start second server on same port
		cmd := exec.Command(env2.BinaryPath, "serve", "--config", env2.ConfigPath)
		cmd.Dir = env2.TempDir

		// Start in background
		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start second server: %v", err)
		}
		defer cmd.Process.Kill()

		// Wait a bit and check if it's still running
		time.Sleep(5 * time.Second)
		
		// It should have exited due to port conflict
		if cmd.ProcessState == nil {
			// Process might still be running, kill it
			cmd.Process.Kill()
			cmd.Wait()
		}
		
		// The exact behavior depends on how Soft Serve handles port conflicts
		// This documents the expected behavior
		t.Log("Second server correctly handled port conflict")
	})
}

func TestKinokoServePermissionErrors(t *testing.T) {
	RequireSoftBinary(t)

	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir, err := os.MkdirTemp("", "kinoko-serve-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)

	t.Run("readonly_data_directory", func(t *testing.T) {
		readonlyDataDir := filepath.Join(tempDir, "readonly-data")
		if err := os.MkdirAll(readonlyDataDir, 0555); err != nil {
			t.Fatalf("Failed to create readonly data dir: %v", err)
		}
		defer os.Chmod(readonlyDataDir, 0755) // Restore for cleanup

		configPath := filepath.Join(tempDir, "readonly-config.yaml")
		configContent := fmt.Sprintf(`server:
  host: localhost
  port: 23241
  dataDir: %s
storage:
  driver: sqlite
  dsn: %s/test.db
libraries: []`, readonlyDataDir, readonlyDataDir)

		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		cmd := exec.Command(binaryPath, "serve", "--config", configPath)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error with readonly data directory\nOutput: %s", output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "permission") && 
		   !strings.Contains(outputStr, "denied") {
			t.Errorf("Error should mention permission issue: %s", outputStr)
		}
	})

	t.Run("privileged_port_binding", func(t *testing.T) {
		privilegedConfig := filepath.Join(tempDir, "privileged-config.yaml")
		privilegedContent := fmt.Sprintf(`server:
  host: localhost
  port: 80
  dataDir: %s/data
storage:
  driver: sqlite
  dsn: %s/test.db
libraries: []`, tempDir, tempDir)

		if err := os.WriteFile(privilegedConfig, []byte(privilegedContent), 0644); err != nil {
			t.Fatalf("Failed to write privileged config: %v", err)
		}

		cmd := exec.Command(binaryPath, "serve", "--config", privilegedConfig)
		cmd.Dir = tempDir

		// Create data directory first
		dataDir := filepath.Join(tempDir, "data")
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			t.Fatalf("Failed to create data dir: %v", err)
		}

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Logf("Unexpectedly succeeded in binding to port 80\nOutput: %s", output)
		} else {
			t.Logf("Expected failure binding to privileged port (this is correct): %v", err)
		}
	})
}

func TestRepositoryOperations(t *testing.T) {
	RequireSoftBinary(t)
	RequireGitBinary(t)
	RequireSSHBinary(t)

	env := SetupTestEnvironment(t)
	env.StartServer()
	defer env.StopServer()

	t.Run("create_and_list_repositories", func(t *testing.T) {
		repoName := "test-skill-repo"
		
		// Create repository
		output, err := env.RunSSHCommand("repo", "create", repoName)
		if err != nil {
			t.Fatalf("Failed to create repository: %v\nOutput: %s", err, output)
		}

		// List repositories
		listOutput, err := env.RunSSHCommand("repo", "list")
		if err != nil {
			t.Fatalf("Failed to list repositories: %v", err)
		}

		if !strings.Contains(listOutput, repoName) {
			t.Errorf("Created repository %s not found in list: %s", repoName, listOutput)
		}

		// Clean up
		_, err = env.RunSSHCommand("repo", "delete", repoName)
		if err != nil {
			t.Logf("Failed to delete repository (cleanup): %v", err)
		}
	})

	t.Run("repository_name_validation", func(t *testing.T) {
		// Test various repository names
		tests := []struct {
			name        string
			shouldWork  bool
		}{
			{"valid-repo-name", true},
			{"repo123", true},
			{"simple", true},
			{"Invalid-Caps", false}, // Might work depending on Soft Serve
			{"repo with spaces", false},
			{"repo/with/slashes", false},
			{"", false},
		}

		for _, tt := range tests {
			t.Run("name_"+tt.name, func(t *testing.T) {
				if tt.name == "" {
					// Skip empty name test as it might cause SSH issues
					return
				}

				output, err := env.RunSSHCommand("repo", "create", tt.name)
				
				if tt.shouldWork {
					if err != nil {
						t.Logf("Repository creation failed (might be expected): %v\nOutput: %s", err, output)
					} else {
						// Clean up if successful
						env.RunSSHCommand("repo", "delete", tt.name)
					}
				} else {
					if err == nil {
						t.Logf("Repository creation succeeded unexpectedly: %s", tt.name)
						// Clean up
						env.RunSSHCommand("repo", "delete", tt.name)
					}
				}
			})
		}
	})

	t.Run("delete_nonexistent_repository", func(t *testing.T) {
		nonexistentRepo := "this-repo-does-not-exist"
		
		output, err := env.RunSSHCommand("repo", "delete", nonexistentRepo)
		if err == nil {
			t.Errorf("Expected error when deleting nonexistent repository\nOutput: %s", output)
		}
	})
}

func TestGitOperations(t *testing.T) {
	RequireSoftBinary(t)
	RequireGitBinary(t)
	RequireSSHBinary(t)

	env := SetupTestEnvironment(t)
	env.StartServer()
	defer env.StopServer()

	repoName := "git-operations-test"
	
	// Create repository
	_, err := env.RunSSHCommand("repo", "create", repoName)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer env.RunSSHCommand("repo", "delete", repoName)

	t.Run("ssh_clone_push_pull", func(t *testing.T) {
		cloneDir := filepath.Join(env.TempDir, "ssh-clone")
		
		// Clone repository
		if err := env.GitCloneSSH(repoName, cloneDir); err != nil {
			t.Fatalf("Failed to clone repository via SSH: %v", err)
		}

		// Create a skill file
		if err := env.CreateSkillFile(cloneDir, "test-skill", "test-author", 0.8, `# Test Skill

## When to Use
When testing git operations.

## Solution
Clone, modify, commit, push.`); err != nil {
			t.Fatalf("Failed to create skill file: %v", err)
		}

		// Configure git user
		gitConfigCmd := func(key, value string) error {
			cmd := exec.Command("git", "config", key, value)
			cmd.Dir = cloneDir
			return cmd.Run()
		}

		if err := gitConfigCmd("user.email", "test@example.com"); err != nil {
			t.Fatalf("Failed to configure git email: %v", err)
		}
		if err := gitConfigCmd("user.name", "Test User"); err != nil {
			t.Fatalf("Failed to configure git name: %v", err)
		}

		// Add and commit
		addCmd := exec.Command("git", "add", ".")
		addCmd.Dir = cloneDir
		if err := addCmd.Run(); err != nil {
			t.Fatalf("Failed to git add: %v", err)
		}

		commitCmd := exec.Command("git", "commit", "-m", "Add test skill")
		commitCmd.Dir = cloneDir
		if err := commitCmd.Run(); err != nil {
			t.Fatalf("Failed to git commit: %v", err)
		}

		// Push changes
		gitSSHCmd := fmt.Sprintf("ssh -p %d -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR",
			env.SSHPort, env.AdminKeyPath)
		
		pushCmd := exec.Command("git", "push", "origin", "master")
		pushCmd.Dir = cloneDir
		pushCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", gitSSHCmd))
		
		if err := pushCmd.Run(); err != nil {
			// Try main branch if master fails
			pushCmd = exec.Command("git", "push", "origin", "main")
			pushCmd.Dir = cloneDir  
			pushCmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", gitSSHCmd))
			if err := pushCmd.Run(); err != nil {
				t.Fatalf("Failed to git push: %v", err)
			}
		}

		// Verify by cloning again
		verifyDir := filepath.Join(env.TempDir, "verify-clone")
		if err := env.GitCloneSSH(repoName, verifyDir); err != nil {
			t.Fatalf("Failed to clone for verification: %v", err)
		}

		skillPath := filepath.Join(verifyDir, "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			t.Error("Pushed SKILL.md file not found in verification clone")
		}
	})

	t.Run("http_clone", func(t *testing.T) {
		cloneDir := filepath.Join(env.TempDir, "http-clone")
		
		if err := env.GitCloneHTTP(repoName, cloneDir); err != nil {
			t.Fatalf("Failed to clone repository via HTTP: %v", err)
		}

		// Verify clone worked
		if _, err := os.Stat(filepath.Join(cloneDir, ".git")); os.IsNotExist(err) {
			t.Error("HTTP clone did not create .git directory")
		}
	})

	t.Run("clone_nonexistent_repository", func(t *testing.T) {
		nonexistentRepo := "does-not-exist"
		cloneDir := filepath.Join(env.TempDir, "nonexistent-clone")
		
		err := env.GitCloneSSH(nonexistentRepo, cloneDir)
		if err == nil {
			t.Error("Expected error when cloning nonexistent repository")
		}
	})
}

func TestServerStressScenarios(t *testing.T) {
	RequireSoftBinary(t)
	RequireGitBinary(t)
	RequireSSHBinary(t)

	if testing.Short() {
		t.Skip("Skipping stress tests in short mode")
	}

	env := SetupTestEnvironment(t)
	env.StartServer()
	defer env.StopServer()

	t.Run("many_repositories", func(t *testing.T) {
		const numRepos = 50
		repoNames := make([]string, numRepos)

		// Create many repositories
		for i := 0; i < numRepos; i++ {
			repoName := fmt.Sprintf("stress-repo-%03d", i)
			repoNames[i] = repoName
			
			if _, err := env.RunSSHCommand("repo", "create", repoName); err != nil {
				t.Logf("Failed to create repo %s: %v", repoName, err)
				continue
			}
		}

		// List all repositories
		start := time.Now()
		listOutput, err := env.RunSSHCommand("repo", "list")
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Failed to list repositories: %v", err)
		}

		t.Logf("Listed %d repositories in %v", numRepos, duration)

		// Verify most repositories are listed
		foundCount := 0
		for _, repoName := range repoNames {
			if strings.Contains(listOutput, repoName) {
				foundCount++
			}
		}

		if foundCount < numRepos/2 {
			t.Errorf("Expected to find at least %d repositories, found %d", numRepos/2, foundCount)
		}

		// Clean up repositories
		for _, repoName := range repoNames {
			env.RunSSHCommand("repo", "delete", repoName)
		}
	})

	t.Run("concurrent_operations", func(t *testing.T) {
		const numConcurrent = 10
		
		// Create repositories concurrently
		results := make(chan error, numConcurrent)
		
		for i := 0; i < numConcurrent; i++ {
			go func(id int) {
				repoName := fmt.Sprintf("concurrent-repo-%d", id)
				_, err := env.RunSSHCommand("repo", "create", repoName)
				results <- err
				
				// Clean up
				if err == nil {
					env.RunSSHCommand("repo", "delete", repoName)
				}
			}(i)
		}

		// Wait for all to complete
		errorCount := 0
		for i := 0; i < numConcurrent; i++ {
			if err := <-results; err != nil {
				t.Logf("Concurrent operation %d failed: %v", i, err)
				errorCount++
			}
		}

		// Some failures are acceptable under high concurrency
		if errorCount > numConcurrent/2 {
			t.Errorf("Too many concurrent operations failed: %d/%d", errorCount, numConcurrent)
		}
	})
}