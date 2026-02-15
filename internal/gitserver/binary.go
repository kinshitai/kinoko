package gitserver

import (
	"fmt"
	"os/exec"
)

// CheckSoftBinary checks if the 'soft' binary is available
// Returns the path to the binary if found, or an error if not
func CheckSoftBinary() (string, error) {
	path, err := exec.LookPath("soft")
	if err != nil {
		return "", fmt.Errorf("soft binary not found: %w. Install with: go install github.com/charmbracelet/soft-serve/cmd/soft@latest", err)
	}
	return path, nil
}

// InstallSoftBinary attempts to install the soft binary using go install
func InstallSoftBinary() error {
	cmd := exec.Command("go", "install", "github.com/charmbracelet/soft-serve/cmd/soft@latest")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install soft binary: %w", err)
	}
	return nil
}