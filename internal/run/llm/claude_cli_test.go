package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is the re-exec entry point for faking the Claude CLI.
// It is not a real test — it exits immediately when GO_TEST_HELPER is unset.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	switch os.Getenv("GO_TEST_SCENARIO") {
	case "success":
		fmt.Print(`{"result":"Hello from Claude","is_error":false}`)
	case "error":
		fmt.Print(`{"result":"something went wrong","is_error":true}`)
	case "not_logged_in":
		fmt.Print(`{"result":"Not logged in","is_error":true}`)
	case "malformed":
		fmt.Print(`{not json}`)
	case "timeout":
		time.Sleep(5 * time.Second)
		fmt.Print(`{"result":"too late","is_error":false}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario")
		os.Exit(1)
	}
	os.Exit(0)
}

// fakeCommand overrides newCLICommand to re-exec the test binary
// as a helper process with the given scenario.
func fakeCommand(t *testing.T, scenario string) {
	t.Helper()
	orig := newCLICommand
	t.Cleanup(func() { newCLICommand = orig })

	newCLICommand = func(ctx context.Context, args ...string) *exec.Cmd {
		// Re-exec the test binary, running only TestHelperProcess.
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GO_TEST_HELPER=1",
			"GO_TEST_SCENARIO="+scenario,
		)
		return cmd
	}
}

func TestClaudeCLI_Success(t *testing.T) {
	fakeCommand(t, "success")

	client := NewClaudeCLIClient("")
	result, err := client.Complete(context.Background(), "say hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello from Claude" {
		t.Fatalf("expected 'Hello from Claude', got %q", result)
	}
}

func TestClaudeCLI_Error(t *testing.T) {
	fakeCommand(t, "error")

	client := NewClaudeCLIClient("")
	_, err := client.Complete(context.Background(), "fail")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claude-cli: something went wrong") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestClaudeCLI_NotLoggedIn(t *testing.T) {
	fakeCommand(t, "not_logged_in")

	client := NewClaudeCLIClient("")
	_, err := client.Complete(context.Background(), "fail")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claude CLI not authenticated") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestClaudeCLI_MalformedJSON(t *testing.T) {
	fakeCommand(t, "malformed")

	client := NewClaudeCLIClient("")
	_, err := client.Complete(context.Background(), "bad json")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid JSON response") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestClaudeCLI_Timeout(t *testing.T) {
	fakeCommand(t, "timeout")

	client := NewClaudeCLIClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Complete(ctx, "slow")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNewClient_ClaudeCLI(t *testing.T) {
	c, err := NewClient("claude-cli", "", "claude-sonnet-4-20250514", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*ClaudeCLIClient); !ok {
		t.Fatalf("expected *ClaudeCLIClient, got %T", c)
	}
}
