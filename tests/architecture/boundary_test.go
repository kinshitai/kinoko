//go:build integration

// Package architecture enforces import boundaries between client and server packages.
// Run with: go test -tags integration -race ./tests/architecture/
package architecture

import (
	"os/exec"
	"strings"
	"testing"
)

// modulePrefix is the Go module path for kinoko.
const modulePrefix = "github.com/kinoko-dev/kinoko/internal/"

// clientPackages live on the client side and must never touch server internals.
var clientPackages = []string{
	"queue",
	"extraction",
	"injection",
	"serverclient",
	"worker",
}

// serverPackages live on the server side and must never touch client internals.
// NOTE: decay is intentionally excluded — it's shared (used by worker for scoring
// and by server for maintenance). It depends only on model, not on storage or queue.
var serverPackages = []string{
	"api",
	"storage",
	"gitserver",
}

// TestImportBoundaries ensures client and server packages don't cross-import.
func TestImportBoundaries(t *testing.T) {
	// Collect all imports via go list.
	// Format: "importpath:dep1,dep2,dep3"
	cmd := exec.Command(
		"go", "list", "-f", `{{.ImportPath}}:{{join .Deps ","}}`, "./internal/...",
	)
	// Run from module root so the ./internal/... pattern resolves.
	cmd.Dir = findModuleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %s\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list failed: %s", err)
	}

	// Parse into map: package -> set of transitive deps.
	deps := make(map[string]map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		pkg := parts[0]
		depSet := make(map[string]bool)
		for _, d := range strings.Split(parts[1], ",") {
			if d != "" {
				depSet[d] = true
			}
		}
		deps[pkg] = depSet
	}

	clientSet := toSet(clientPackages)
	serverSet := toSet(serverPackages)

	for pkg, pkgDeps := range deps {
		short := shortName(pkg)

		if clientSet[short] {
			// Client must not import storage.
			assertNoDep(t, pkg, pkgDeps, "storage", "client package imports server's storage")
			// Client must not import any server package.
			for _, s := range serverPackages {
				assertNoDep(t, pkg, pkgDeps, s, "client package imports server package")
			}
		}

		if serverSet[short] {
			// Server must not import queue.
			assertNoDep(t, pkg, pkgDeps, "queue", "server package imports client's queue")
			// Server must not import any client package.
			for _, c := range clientPackages {
				assertNoDep(t, pkg, pkgDeps, c, "server package imports client package")
			}
		}
	}
}

// assertNoDep fails if pkgDeps contains modulePrefix+forbidden.
func assertNoDep(t *testing.T, pkg string, pkgDeps map[string]bool, forbidden, reason string) {
	t.Helper()
	target := modulePrefix + forbidden
	if pkgDeps[target] {
		t.Errorf("%s: %s depends on %s (%s)", reason, pkg, target, reason)
	}
}

// shortName extracts the last path element after internal/.
func shortName(importPath string) string {
	idx := strings.Index(importPath, "internal/")
	if idx < 0 {
		return ""
	}
	rest := importPath[idx+len("internal/"):]
	// Handle sub-packages: "api/foo" → "api"
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// findModuleRoot walks up from the test's working directory to find go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("cannot find module root: %s", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == "/dev/null" {
		t.Fatal("not inside a Go module")
	}
	// go.mod path → directory
	return gomod[:strings.LastIndex(gomod, "/")]
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
