package main

import (
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/serve/storage"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestExtractCmdArgs(t *testing.T) {
	// extract requires exactly 1 arg
	cmd := extractCmd
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("expected error with no args")
	}
	if err := cmd.Args(cmd, []string{"file.log"}); err != nil {
		t.Errorf("unexpected error with 1 arg: %v", err)
	}
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error with 2 args")
	}
}

func TestStatsCmdExists(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "stats" {
			found = true
			break
		}
	}
	if !found {
		t.Error("stats command not registered")
	}
}

func TestParseSessionFromLog(t *testing.T) {
	log := []byte(`2025-01-15T10:00:00 Session start model=claude-3-opus
tool_call: exec ls -la
tool_call: exec cat file.txt
tool_call: exec go build
error: build failed
tool_call: exec go build ./...
2025-01-15T10:15:00 Session end`)

	session := extraction.ParseSessionFromLog(log, "test-lib")

	if session.LibraryID != "test-lib" {
		t.Errorf("LibraryID = %q, want test-lib", session.LibraryID)
	}
	if session.ToolCallCount < 4 {
		t.Errorf("ToolCallCount = %d, want >= 4", session.ToolCallCount)
	}
	if session.ErrorCount < 1 {
		t.Errorf("ErrorCount = %d, want >= 1", session.ErrorCount)
	}
	if !session.HasSuccessfulExec {
		t.Error("HasSuccessfulExec = false, want true")
	}
	if session.DurationMinutes < 14 || session.DurationMinutes > 16 {
		t.Errorf("DurationMinutes = %.1f, want ~15", session.DurationMinutes)
	}
	if session.AgentModel != "claude-3-opus" {
		t.Errorf("AgentModel = %q, want claude-3-opus", session.AgentModel)
	}
}

func TestRootCmdHasAllCommands(t *testing.T) {
	want := map[string]bool{
		"serve":   false,
		"init":    false,
		"extract": false,
		"stats":   false,
		"import":  false,
		"queue":   false,
	}
	for _, cmd := range rootCmd.Commands() {
		name := cmd.Name()
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("command %q not registered on root", name)
		}
	}
}

func TestImportCmdFlags(t *testing.T) {
	f := importCmd.Flags()
	for _, name := range []string{"config", "library", "dir"} {
		if f.Lookup(name) == nil {
			t.Errorf("--%s flag not found on import command", name)
		}
	}
}

func TestImportRequiresArgs(t *testing.T) {
	// Running import with no args and no --dir should fail.
	err := runImport(importCmd, []string{})
	if err == nil {
		t.Error("expected error with no args and no --dir")
	}
}

func TestQueueSubcommands(t *testing.T) {
	subs := map[string]bool{"stats": false, "list": false, "retry": false}
	for _, cmd := range queueCmd.Commands() {
		if _, ok := subs[cmd.Name()]; ok {
			subs[cmd.Name()] = true
		}
	}
	for name, found := range subs {
		if !found {
			t.Errorf("queue subcommand %q not registered", name)
		}
	}
}

func TestQueueRetryRequiresArg(t *testing.T) {
	if err := queueRetryCmd.Args(queueRetryCmd, []string{}); err == nil {
		t.Error("expected error with no args")
	}
	if err := queueRetryCmd.Args(queueRetryCmd, []string{"some-id"}); err != nil {
		t.Errorf("unexpected error with 1 arg: %v", err)
	}
}

func TestStoreQuerierInterface(t *testing.T) {
	// Compile-time check that storage.NewSkillQuerier returns SkillQuerier.
	var _ model.SkillQuerier = storage.NewSkillQuerier(nil)
}
