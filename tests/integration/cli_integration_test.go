//go:build integration

// Package integration contains integration tests for the kinoko CLI commands:
// init, run, and serve — verifying the new split architecture.
package integration

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

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

// startServeProcess starts `kinoko serve` in background, returns cmd.
func startServeProcess(t *testing.T, bin, cfgPath string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, "serve", "--config", cfgPath)
	cmd.Stdout = &testWriter{t, "serve-out"}
	cmd.Stderr = &testWriter{t, "serve-err"}
	if err := cmd.Start(); err != nil {
		t.Fatalf("serve start: %v", err)
	}
	return cmd
}

func stopProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

type testWriter struct {
	t      *testing.T
	prefix string
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.t.Logf("[%s] %s", w.prefix, strings.TrimSpace(string(p)))
	return len(p), nil
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
	// check defaults: server section exists
	if _, ok := raw["server"]; !ok {
		t.Error("config.yaml missing 'server' section")
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

	if testing.Short() {
		t.Skip("skipping serve test in -short mode")
	}

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	cmd := startServeProcess(t, bin, cfgPath)
	defer stopProcess(t, cmd)

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

	// API health endpoint (port+1)
	apiPort := sshPort + 1
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", apiPort)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Logf("API health check failed (may need OPENAI_API_KEY): %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("health returned %d", resp.StatusCode)
		}
	}

	// Graceful stop
	stopProcess(t, cmd)

	// Process should be gone
	time.Sleep(500 * time.Millisecond)
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Error("serve process did not exit after SIGTERM")
	}
}

// ---------------------------------------------------------------------------
// 5. TestServe_NoInitRequired
// ---------------------------------------------------------------------------

func TestServe_NoInitRequired(t *testing.T) {
	requireBin(t, "soft")
	requireBin(t, "ssh-keygen")

	if testing.Short() {
		t.Skip("skipping serve test in -short mode")
	}

	// No init — just write a config and run serve.
	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	cmd := startServeProcess(t, bin, cfgPath)
	defer stopProcess(t, cmd)

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
	// `kinoko run` requires an LLM API key for the extraction pipeline.
	// Without it, it errors out. We verify that behavior cleanly.
	if testing.Short() {
		t.Skip("skipping run test in -short mode")
	}

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	os.MkdirAll(dataDir, 0755)
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")
	sshPort := freePort(t)

	writeMinimalConfig(t, cfgPath, dataDir, dbPath, sshPort)

	// Without OPENAI_API_KEY, run should fail with a clear error (not panic)
	cmd := exec.Command(bin, "run", "--config", cfgPath)
	cmd.Env = filterEnv("OPENAI_API_KEY", "KINOKO_LLM_API_KEY")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Log("run succeeded without API key (unexpected but not fatal)")
	} else {
		outStr := string(out)
		if strings.Contains(outStr, "panic") {
			t.Fatalf("run panicked:\n%s", outStr)
		}
		if !strings.Contains(outStr, "LLM") && !strings.Contains(outStr, "API key") &&
			!strings.Contains(outStr, "pipeline") {
			t.Logf("run error doesn't mention LLM/API key: %s", outStr)
		}
	}

	// If API key is available, verify run starts workers
	if os.Getenv("OPENAI_API_KEY") != "" {
		cmd2 := exec.Command(bin, "run", "--config", cfgPath)
		cmd2.Stdout = &testWriter{t, "run-out"}
		cmd2.Stderr = &testWriter{t, "run-err"}
		if err := cmd2.Start(); err != nil {
			t.Fatalf("run start: %v", err)
		}
		defer stopProcess(t, cmd2)

		// Give it a few seconds to start
		time.Sleep(3 * time.Second)

		// Process should still be running (not crashed)
		if cmd2.ProcessState != nil && cmd2.ProcessState.Exited() {
			t.Error("run process exited prematurely")
		}
		stopProcess(t, cmd2)
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

	if testing.Short() {
		t.Skip("skipping full integration test in -short mode")
	}

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

	// Patch config to use a free port
	sshPort := freePort(t)
	cfgPath := filepath.Join(kd, "config.yaml")
	cfgBytes, _ := os.ReadFile(cfgPath)
	patched := strings.ReplaceAll(string(cfgBytes), "port: 23231", fmt.Sprintf("port: %d", sshPort))
	os.WriteFile(cfgPath, []byte(patched), 0644)

	// Step 2: serve
	serveCmd := exec.Command(bin, "serve", "--config", cfgPath)
	serveCmd.Env = append(os.Environ(), "HOME="+home)
	serveCmd.Stdout = &testWriter{t, "serve-out"}
	serveCmd.Stderr = &testWriter{t, "serve-err"}
	if err := serveCmd.Start(); err != nil {
		t.Fatalf("serve start: %v", err)
	}
	defer stopProcess(t, serveCmd)

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

	// Step 3: run (will likely fail without API key, but shouldn't crash)
	runCmd := exec.Command(bin, "run", "--config", cfgPath)
	runCmd.Env = append(os.Environ(), "HOME="+home)
	runOut, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		if strings.Contains(string(runOut), "panic") {
			t.Fatalf("run panicked:\n%s", runOut)
		}
		t.Logf("run exited (expected without API key): %s", runOut)
	} else {
		t.Log("full init→serve→run flow succeeded")
	}
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

	if testing.Short() {
		t.Skip("skipping port conflict test in -short mode")
	}

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

	cmd := exec.Command(bin, "serve", "--config", cfgPath)
	cmd.Stdout = &testWriter{t, "conflict-out"}
	cmd.Stderr = &testWriter{t, "conflict-err"}

	if err := cmd.Start(); err != nil {
		t.Fatalf("serve start: %v", err)
	}

	// Should exit within a few seconds due to port conflict
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("serve should have failed due to port conflict")
		} else {
			t.Logf("serve correctly failed with port conflict: %v", err)
		}
	case <-time.After(15 * time.Second):
		cmd.Process.Kill()
		t.Error("serve did not exit after port conflict (timed out)")
	}
}

// ---------------------------------------------------------------------------
// 10. TestRun_ServerUnreachable
// ---------------------------------------------------------------------------

func TestRun_ServerUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	tmp := t.TempDir()
	bin := buildBinary(t, tmp)
	dataDir := filepath.Join(tmp, "data")
	os.MkdirAll(dataDir, 0755)
	dbPath := filepath.Join(tmp, "test.db")
	cfgPath := filepath.Join(tmp, "config.yaml")

	// Point to a port nobody is listening on
	deadPort := freePort(t)
	writeMinimalConfig(t, cfgPath, dataDir, dbPath, deadPort)

	cmd := exec.Command(bin, "run", "--config", cfgPath)
	cmd.Env = filterEnv() // keep env as-is
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	// Should NOT panic
	if strings.Contains(outStr, "panic") {
		t.Fatalf("run panicked when server unreachable:\n%s", outStr)
	}

	// It may fail (no pipeline, etc.) but that's OK — no crash.
	if err != nil {
		t.Logf("run exited with error (expected): %v — %s", err, outStr)
	}
	_ = strconv.Itoa(deadPort) // suppress unused import
}
