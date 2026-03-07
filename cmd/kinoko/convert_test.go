package main

import (
	"os"
	"testing"
)

func TestConvertCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "convert" {
			found = true
			break
		}
	}
	if !found {
		t.Error("convert command not registered in rootCmd")
	}
}

func TestConvertCmd_RequiresExactlyOneArg(t *testing.T) {
	cmd := convertCmd
	// No args should fail
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error with no args")
	}

	// Two args should fail
	err = cmd.Args(cmd, []string{"a", "b"})
	if err == nil {
		t.Error("expected error with two args")
	}

	// One arg should pass
	err = cmd.Args(cmd, []string{"a"})
	if err != nil {
		t.Errorf("unexpected error with one arg: %v", err)
	}
}

func TestRunConvert_MissingFile(t *testing.T) {
	err := runConvert(convertCmd, []string{"/nonexistent/file.md"})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRunConvert_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "empty-*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = runConvert(convertCmd, []string{f.Name()})
	if err == nil {
		t.Error("expected error for empty file")
	}
}
