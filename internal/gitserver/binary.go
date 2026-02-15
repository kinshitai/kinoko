package gitserver

import (
	"fmt"
	"os/exec"
)

// CheckSoftBinary checks if the 'soft' binary is available.
// Returns the path to the binary if found, or an error if not.
func CheckSoftBinary() (string, error) {
	path, err := exec.LookPath("soft")
	if err != nil {
		return "", fmt.Errorf("soft binary not found: %w. Install with: go install github.com/charmbracelet/soft-serve/cmd/soft@latest", err)
	}
	return path, nil
}
