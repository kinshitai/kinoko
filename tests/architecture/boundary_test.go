package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRunServeIsolation verifies that internal/run/** never imports internal/serve/**
// and vice versa. Both may import internal/shared/** and pkg/**.
func TestRunServeIsolation(t *testing.T) {
	checkNoImport(t, "run", "serve")
	checkNoImport(t, "serve", "run")
}

func checkNoImport(t *testing.T, from, to string) {
	t.Helper()

	pattern := "github.com/kinoko-dev/kinoko/internal/" + from + "/..."
	forbidden := "github.com/kinoko-dev/kinoko/internal/" + to + "/"

	out, err := exec.Command("go", "list", "-deps", "-f", `{{.ImportPath}} {{join .Imports ","}}`, pattern).CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed for %s: %v\n%s", from, err, out)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		pkg := parts[0]
		// Only check our own packages
		if !strings.Contains(pkg, "github.com/kinoko-dev/kinoko/internal/"+from) {
			continue
		}
		imports := strings.Split(parts[1], ",")
		for _, imp := range imports {
			if strings.HasPrefix(imp, forbidden) {
				t.Errorf("BOUNDARY VIOLATION: %s imports %s (%s/ must not import %s/)", pkg, imp, from, to)
			}
		}
	}
}
