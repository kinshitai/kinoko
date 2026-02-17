//go:build integration

// Package integration contains integration tests for the kinoko CLI commands:
// init, run, and serve — verifying the new split architecture.
package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// thread-safe buffer for race-free testing
// ---------------------------------------------------------------------------

// SafeBuffer wraps bytes.Buffer with a mutex for concurrent access
type SafeBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

// Write implements io.Writer interface thread-safely
func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

// String returns the buffer contents thread-safely
func (sb *SafeBuffer) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.buf.String()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func buildBinary(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "kinoko-test")
	root := findRoot(t)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/kinoko")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func findRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		p := filepath.Dir(dir)
		if p == dir {
			t.Fatal("go.mod not found")
		}
		dir = p
	}
}

func requireBin(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found, skipping", name)
	}
}

// runInit executes `kinoko init` with the given HOME and optional extra args.
func runInit(t *testing.T, bin, home string, extraArgs ...string) string {
	t.Helper()
	args := append([]string{"init"}, extraArgs...)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\n%s", err, out)
	}
	return string(out)
}

// writeMinimalConfig writes a minimal server config for serve tests.
func writeMinimalConfig(t *testing.T, path, dataDir, dbPath string, sshPort int) {
	t.Helper()
	content := fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: %d
  dataDir: %s
storage:
  driver: sqlite
  dsn: %s
libraries: []
`, sshPort, dataDir, dbPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// startServeProcess starts `kinoko serve` in background using process group isolation.
func startServeProcess(t *testing.T, bin, cfgPath string) (cmd *exec.Cmd, cancel context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd = exec.CommandContext(ctx, bin, "serve", "--config", cfgPath)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("serve start: %v", err)
	}
	return cmd, cancel
}

// stopProcess terminates a process group gracefully.
func stopProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	if cmd == nil || cmd.Process == nil {
		cancel()
		return
	}
	cancel() // triggers cmd.Cancel → SIGTERM to process group
	// Wait is needed to reap the process; ignore error (killed).
	_ = cmd.Wait()
}

// ---------------------------------------------------------------------------
// 1. TestInit_CreatesWorkspace
// ---------------------------------------------------------------------------

func TestInit_CreatesWorkspace(t *testing.T) {
	requireBin(t, "ssh-keygen")

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0755)

	runInit(t, bin, home)

	kd := filepath.Join(home, ".kinoko")

	// .kinoko/ dir exists
	if fi, err := os.Stat(kd); err != nil || !fi.IsDir() {
		t.Fatal(".kinoko dir not created")
	}

	// config.yaml exists and is valid YAML
	cfgBytes, err := os.ReadFile(filepath.Join(kd, "config.yaml"))
	if err != nil {
		t.Fatal("config.yaml not created")
	}
	var raw map[string]any
	if err := yaml.Unmarshal(cfgBytes, &raw); err != nil {
		t.Fatalf("config.yaml is not valid YAML: %v", err)
	}
	// server section should NOT exist in init config (it's a serve-side concern)
	if _, ok := raw["server"]; ok {
		t.Error("config.yaml should NOT have 'server' section (server-side concern)")
	}

	// cache/ dir
	if fi, err := os.Stat(filepath.Join(kd, "cache")); err != nil || !fi.IsDir() {
		t.Fatal("cache/ dir not created")
	}

	// SSH keys
	if _, err := os.Stat(filepath.Join(kd, "id_ed25519")); err != nil {
		t.Fatal("id_ed25519 not generated")
	}
	if _, err := os.Stat(filepath.Join(kd, "id_ed25519.pub")); err != nil {
		t.Fatal("id_ed25519.pub not generated")
	}

	// No server artifacts
	if _, err := os.Stat(filepath.Join(kd, "data")); !os.IsNotExist(err) {
		t.Error("data/ dir should NOT exist after init (server artifact)")
	}
}

// ---------------------------------------------------------------------------
// 2. TestInit_Idempotent
// ---------------------------------------------------------------------------

func TestInit_Idempotent(t *testing.T) {
	requireBin(t, "ssh-keygen")

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0755)

	runInit(t, bin, home)

	// Record state of key
	keyPath := filepath.Join(home, ".kinoko", "id_ed25519")
	info1, _ := os.Stat(keyPath)
	content1, _ := os.ReadFile(filepath.Join(home, ".kinoko", "config.yaml"))

	// Second init
	runInit(t, bin, home)

	info2, _ := os.Stat(keyPath)
	content2, _ := os.ReadFile(filepath.Join(home, ".kinoko", "config.yaml"))

	// Key should not have been regenerated
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("SSH key was regenerated on second init")
	}
	// Config preserved
	if string(content1) != string(content2) {
		t.Error("config.yaml was overwritten on second init")
	}
}

// ---------------------------------------------------------------------------
// 3. TestInit_ConnectMode (requires a running server — lightweight check)
// ---------------------------------------------------------------------------

func TestInit_ConnectMode(t *testing.T) {
	// --connect needs a reachable server; we just verify the flag is accepted
	// and the binary doesn't crash. The actual connection will fail, which is fine.
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0755)

	cmd := exec.Command(bin, "init", "--connect", "http://127.0.0.1:19999")
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	// We expect an error because server is unreachable, but it shouldn't panic
	if err == nil {
		// If it somehow succeeded, verify config has the server URL
		cfgBytes, _ := os.ReadFile(filepath.Join(home, ".kinoko", "config.yaml"))
		if !strings.Contains(string(cfgBytes), "19999") {
			t.Error("connect mode didn't save server URL in config")
		}
	} else {
		// Should mention "cannot reach" or similar, not a panic
		if strings.Contains(string(out), "panic") {
			t.Fatalf("kinoko init --connect panicked:\n%s", out)
		}
		t.Logf("Expected failure (server unreachable): %s", out)
	}
}

// ---------------------------------------------------------------------------
// 4. TestServe_SelfBootstraps
// ---------------------------------------------------------------------------

func TestServe_SelfBootstraps(t *testing.T) {
	requireBin(t, "soft")
	requireBin(t, "ssh-keygen")

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	cmd, cancel := startServeProcess(t, bin, cfgPath)
	defer stopProcess(cmd, cancel)

	// Wait for SSH port to be listening (up to 30s)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort), time.Second)
		if err == nil {
			conn.Close()
			goto sshReady
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("serve: SSH port never became ready")

sshReady:
	// data dir created
	if _, err := os.Stat(dataDir); err != nil {
		t.Error("serve did not create data dir")
	}

	// admin keypair created
	adminKey := filepath.Join(dataDir, "kinoko_admin_ed25519")
	if _, err := os.Stat(adminKey); err != nil {
		t.Error("serve did not create admin keypair")
	}
	if _, err := os.Stat(adminKey + ".pub"); err != nil {
		t.Error("serve did not create admin public key")
	}

	// API health endpoint (port+2; port+1 is Soft Serve HTTP)
	apiPort := sshPort + 2
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", apiPort)
	client := &http.Client{Timeout: 2 * time.Second}
	healthDeadline := time.Now().Add(5 * time.Second)
	var healthOK bool
	for time.Now().Before(healthDeadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				healthOK = true
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !healthOK {
		t.Fatal("API health endpoint never returned 200")
	}

	// Graceful stop
	stopProcess(cmd, cancel)

	// Process should be gone (ProcessState is set after Wait returns)
	time.Sleep(500 * time.Millisecond)
	if cmd.ProcessState == nil {
		t.Error("serve process did not exit after SIGTERM")
	}
}

// ---------------------------------------------------------------------------
// 5. TestServe_NoInitRequired
// ---------------------------------------------------------------------------

func TestServe_NoInitRequired(t *testing.T) {
	requireBin(t, "soft")
	requireBin(t, "ssh-keygen")

	// No init — just write a config and run serve.
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	cmd, cancel := startServeProcess(t, bin, cfgPath)
	defer stopProcess(cmd, cancel)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort), time.Second)
		if err == nil {
			conn.Close()
			t.Log("serve started without prior init — OK")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("serve without init: SSH port never became ready")
}

// ---------------------------------------------------------------------------
// 6. TestRun_WorkerStartsAndProcesses
// ---------------------------------------------------------------------------

func TestRun_WorkerStartsAndProcesses(t *testing.T) {
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	os.MkdirAll(dataDir, 0755)
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	t.Run("degraded_mode_without_api_key", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var stderr SafeBuffer
		cmd := exec.CommandContext(ctx, bin, "run", "--config", cfgPath)
		cmd.Cancel = func() error {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Env = filterEnv("OPENAI_API_KEY", "KINOKO_LLM_API_KEY")
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatalf("run start: %v", err)
		}
		defer stopProcess(cmd, cancel)

		// Wait for degraded mode log message
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(stderr.String(), "No LLM API key") ||
				strings.Contains(stderr.String(), "extraction disabled") ||
				strings.Contains(stderr.String(), "degraded") {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		// Verify process is still running (daemon, not crashed)
		if cmd.ProcessState != nil {
			t.Fatal("run exited unexpectedly in degraded mode")
		}

		output := stderr.String()
		if strings.Contains(output, "panic") {
			t.Fatalf("run panicked:\n%s", output)
		}

		stopProcess(cmd, cancel)
	})

	// If API key is available, verify run starts workers
	if os.Getenv("OPENAI_API_KEY") != "" {
		t.Run("with_api_key", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cmd := exec.CommandContext(ctx, bin, "run", "--config", cfgPath)
			cmd.Cancel = func() error {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			}
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if err := cmd.Start(); err != nil {
				cancel()
				t.Fatalf("run start: %v", err)
			}
			defer stopProcess(cmd, cancel)

			time.Sleep(3 * time.Second)

			if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
				t.Error("run process exited prematurely")
			}
			stopProcess(cmd, cancel)
		})
	}
}

func filterEnv(exclude ...string) []string {
	excludeSet := make(map[string]bool)
	for _, k := range exclude {
		excludeSet[k] = true
	}
	var env []string
	for _, e := range os.Environ() {
		k := strings.SplitN(e, "=", 2)[0]
		if !excludeSet[k] {
			env = append(env, e)
		}
	}
	return env
}

// ---------------------------------------------------------------------------
// 7. TestCLI_InitRunServeIntegration
// ---------------------------------------------------------------------------

func TestCLI_InitRunServeIntegration(t *testing.T) {
	requireBin(t, "soft")
	requireBin(t, "ssh-keygen")

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0755)

	// Step 1: init
	runInit(t, bin, home)

	kd := filepath.Join(home, ".kinoko")
	if _, err := os.Stat(filepath.Join(kd, "config.yaml")); err != nil {
		t.Fatal("init did not create config")
	}

	// Write a proper server config with a free port (init config has no server section)
	sshPort := freePort(t)
	dataDir := filepath.Join(kd, "data")
	dbPath := filepath.Join(kd, "test.db")
	cfgPath := filepath.Join(kd, "config.yaml")
	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	// Step 2: serve
	ctx, cancel := context.WithCancel(context.Background())
	serveCmd := exec.CommandContext(ctx, bin, "serve", "--config", cfgPath)
	serveCmd.Cancel = func() error {
		return syscall.Kill(-serveCmd.Process.Pid, syscall.SIGTERM)
	}
	serveCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	serveCmd.Env = append(os.Environ(), "HOME="+home)
	serveCmd.Stdout = io.Discard
	serveCmd.Stderr = io.Discard
	if err := serveCmd.Start(); err != nil {
		cancel()
		t.Fatalf("serve start: %v", err)
	}
	defer stopProcess(serveCmd, cancel)

	// Wait for SSH port
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort), time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("serve SSH port not ready")
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Step 3: run as daemon (enters degraded mode without API key)
	runCtx, runCancel := context.WithCancel(context.Background())
	var runStderr bytes.Buffer
	runCmd := exec.CommandContext(runCtx, bin, "run", "--config", cfgPath)
	runCmd.Cancel = func() error {
		return syscall.Kill(-runCmd.Process.Pid, syscall.SIGTERM)
	}
	runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	runCmd.Env = append(os.Environ(), "HOME="+home)
	runCmd.Stdout = io.Discard
	runCmd.Stderr = &runStderr
	if err := runCmd.Start(); err != nil {
		runCancel()
		t.Fatalf("run start: %v", err)
	}
	defer stopProcess(runCmd, runCancel)

	// Wait for log confirmation
	runDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(runDeadline) {
		output := runStderr.String()
		if strings.Contains(output, "No LLM API key") ||
			strings.Contains(output, "extraction disabled") ||
			strings.Contains(output, "degraded") ||
			strings.Contains(output, "daemon") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if runCmd.ProcessState != nil {
		t.Fatal("run exited unexpectedly")
	}

	if strings.Contains(runStderr.String(), "panic") {
		t.Fatalf("run panicked:\n%s", runStderr.String())
	}

	t.Log("full init→serve→run flow succeeded")
	stopProcess(runCmd, runCancel)
}

// ---------------------------------------------------------------------------
// 8. TestInit_CustomConfigPath
// ---------------------------------------------------------------------------

func TestInit_CustomConfigPath(t *testing.T) {
	// The init command always writes to ~/.kinoko/config.yaml.
	// This test verifies that serve/run accept --config pointing elsewhere.
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	home := filepath.Join(tmp, "home")
	os.MkdirAll(home, 0755)

	runInit(t, bin, home)

	// Copy config to a custom location
	src := filepath.Join(home, ".kinoko", "config.yaml")
	dst := filepath.Join(tmp, "custom", "my-config.yaml")
	os.MkdirAll(filepath.Dir(dst), 0755)
	data, _ := os.ReadFile(src)
	os.WriteFile(dst, data, 0644)

	// serve should accept --config
	cmd := exec.Command(bin, "serve", "--config", dst, "--help")
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "panic") {
		t.Fatalf("serve --config panicked:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// 9. TestServe_PortConflict
// ---------------------------------------------------------------------------

func TestServe_PortConflict(t *testing.T) {
	requireBin(t, "soft")
	requireBin(t, "ssh-keygen")

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	port := freePort(t)

	// Occupy the port
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	dataDir := filepath.Join(tmp, "data")
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	writeMinimalConfig(t, cfgPath, dataDir, dbPath, port)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "serve", "--config", cfgPath)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("serve start: %v", err)
	}

	// Should exit within a few seconds due to port conflict
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		cancel()
		if err == nil {
			t.Error("serve should have failed due to port conflict")
		} else {
			t.Logf("serve correctly failed with port conflict: %v", err)
		}
	case <-time.After(15 * time.Second):
		stopProcess(cmd, cancel)
		t.Error("serve did not exit after port conflict (timed out)")
	}
}

// ---------------------------------------------------------------------------
// 10. TestRun_ServerUnreachable
// ---------------------------------------------------------------------------

func TestRun_ServerUnreachable(t *testing.T) {
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	os.MkdirAll(dataDir, 0755)
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")

	// Point to a port nobody is listening on
	deadPort := freePort(t)
	writeMinimalConfig(t, cfgPath, dataDir, dbPath, deadPort)

	// kinoko run uses local SQLite — start as daemon, verify it runs, stop.
	ctx, cancel := context.WithCancel(context.Background())
	var stderr SafeBuffer
	cmd := exec.CommandContext(ctx, bin, "run", "--config", cfgPath)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = filterEnv("OPENAI_API_KEY", "KINOKO_LLM_API_KEY")
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("run start: %v", err)
	}
	defer stopProcess(cmd, cancel)

	// Wait for startup log
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		output := stderr.String()
		if strings.Contains(output, "No LLM API key") ||
			strings.Contains(output, "extraction disabled") ||
			strings.Contains(output, "degraded") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Verify process is still running
	if cmd.ProcessState != nil {
		t.Fatal("run exited unexpectedly with unreachable server config")
	}

	if strings.Contains(stderr.String(), "panic") {
		t.Fatalf("run panicked:\n%s", stderr.String())
	}

	stopProcess(cmd, cancel)
}
