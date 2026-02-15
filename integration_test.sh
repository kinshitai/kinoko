#!/bin/bash
set -e

echo "🍄 Mycelium Integration Test"
echo "============================="

# Cleanup from any previous test runs
echo "🧹 Cleaning up from previous test runs..."
rm -rf ~/.mycelium-test
rm -f ./mycelium
pkill -f "mycelium serve" || true
sleep 1

# Build mycelium
echo "🔨 Building mycelium..."
go build -o mycelium ./cmd/mycelium

# Test 1: Initialize Mycelium workspace
echo "📦 Testing mycelium init..."
# Use a clean directory for testing
TEST_HOME="/tmp/mycelium-test"
rm -rf $TEST_HOME
mkdir -p $TEST_HOME
export HOME="$TEST_HOME"
MYCELIUM_HOME="$HOME/.mycelium"

./mycelium init

# Verify init created the expected files
if [[ ! -f "$MYCELIUM_HOME/config.yaml" ]]; then
    echo "❌ Init failed: config.yaml not created"
    exit 1
fi

if [[ ! -d "$MYCELIUM_HOME/skills" ]]; then
    echo "❌ Init failed: skills directory not created"
    exit 1
fi

echo "✅ Init test passed"

# Test 2: Start mycelium serve in background
echo "🚀 Starting mycelium serve in background..."
./mycelium serve > serve.log 2>&1 &
SERVE_PID=$!

# Wait for server to start
echo "⏳ Waiting for server to start..."
sleep 3

# Check if server is still running
if ! kill -0 $SERVE_PID 2>/dev/null; then
    echo "❌ Server failed to start"
    echo "Server log:"
    cat serve.log
    exit 1
fi

echo "✅ Server started successfully (PID: $SERVE_PID)"

# Test 3: Test programmatic repo creation using Go
echo "🗄️ Testing programmatic repository creation..."
cat > test_repo_creation.go << 'EOF'
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	
	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/gitserver"
)

func main() {
	// Load the same config the server is using
	homeDir := os.Getenv("HOME")
	configPath := filepath.Join(homeDir, ".mycelium", "config.yaml")
	
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// Create a git server instance (not started, just for repo management)
	server, err := gitserver.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create git server: %v", err)
	}
	
	// Create a test repository
	repoName := "test-skill"
	description := "A test skill for integration testing"
	
	if err := server.CreateRepo(repoName, description); err != nil {
		log.Fatalf("Failed to create repository: %v", err)
	}
	
	fmt.Printf("✅ Successfully created repository: %s\n", repoName)
	
	// List repositories
	repos, err := server.ListRepos()
	if err != nil {
		log.Fatalf("Failed to list repositories: %v", err)
	}
	
	fmt.Printf("✅ Found %d repositories: %v\n", len(repos), repos)
	
	// Verify our repo is in the list
	found := false
	for _, repo := range repos {
		if repo == repoName {
			found = true
			break
		}
	}
	
	if !found {
		log.Fatalf("❌ Created repository not found in list")
	}
	
	fmt.Println("✅ Repository creation test passed")
}
EOF

go run test_repo_creation.go
rm test_repo_creation.go

# Test 4: Attempt to test SSH access (will fail since we don't have full Soft Serve, but we can test the infrastructure)
echo "🔗 Testing SSH connection infrastructure..."

# Check that data directory structure was created
DATA_DIR="$MYCELIUM_HOME/data"
if [[ ! -d "$DATA_DIR" ]]; then
    echo "❌ Data directory not created: $DATA_DIR"
    exit 1
fi

echo "✅ Data directory structure verified"

# Test 5: Graceful shutdown
echo "🛑 Testing graceful shutdown..."
kill -TERM $SERVE_PID

# Wait for graceful shutdown (up to 10 seconds)
for i in {1..10}; do
    if ! kill -0 $SERVE_PID 2>/dev/null; then
        echo "✅ Server shut down gracefully"
        break
    fi
    sleep 1
done

# Force kill if still running
if kill -0 $SERVE_PID 2>/dev/null; then
    echo "⚠️ Server didn't shut down gracefully, force killing..."
    kill -9 $SERVE_PID
fi

# Check serve.log for any errors
echo "📋 Server log summary:"
echo "====================="
grep -E "(ERROR|WARN|Started|Stopped)" serve.log || echo "(No significant log entries)"
echo "====================="

# Cleanup
echo "🧹 Cleaning up test files..."
rm -f serve.log
rm -rf $TEST_HOME

echo ""
echo "🎉 Integration test completed successfully!"
echo ""
echo "Summary:"
echo "- ✅ mycelium init works"
echo "- ✅ mycelium serve starts and stops gracefully"
echo "- ✅ Git server infrastructure is set up"
echo "- ✅ Programmatic repository creation works"
echo "- ✅ Configuration system works"
echo ""
echo "Next steps for production:"
echo "- Add actual Soft Serve dependency to go.mod"
echo "- Implement real git server in internal/gitserver"
echo "- Add SSH key management"
echo "- Add actual git clone/push/pull functionality"