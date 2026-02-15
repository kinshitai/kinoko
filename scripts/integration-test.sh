#!/bin/bash

# Integration test script for Mycelium with real Soft Serve integration
# Tests the full workflow: serve → create repo → clone → push skill → verify

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="/tmp/mycelium-integration-test-$$"
MYCELIUM_BIN="$PROJECT_ROOT/bin/mycelium"
CONFIG_DIR="$TEST_DIR/config"
DATA_DIR="$TEST_DIR/data"
SKILLS_DIR="$TEST_DIR/skills"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

cleanup() {
    log "Cleaning up test environment..."
    
    # Kill mycelium server if running
    if [[ -n "${MYCELIUM_PID:-}" ]]; then
        log "Stopping mycelium server (PID: $MYCELIUM_PID)"
        kill "$MYCELIUM_PID" 2>/dev/null || true
        wait "$MYCELIUM_PID" 2>/dev/null || true
    fi
    
    # Remove test directory
    if [[ -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
        log "Removed test directory: $TEST_DIR"
    fi
}

trap cleanup EXIT

check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if soft binary is available
    if ! command -v soft &> /dev/null; then
        error "soft binary not found. Install with: go install github.com/charmbracelet/soft-serve/cmd/soft@latest"
        exit 1
    fi
    
    # Check if git is available
    if ! command -v git &> /dev/null; then
        error "git binary not found. Please install git"
        exit 1
    fi
    
    log "Prerequisites check passed"
}

build_mycelium() {
    log "Building mycelium binary..."
    
    cd "$PROJECT_ROOT"
    
    # Create bin directory
    mkdir -p bin
    
    # Build the binary
    go build -o "$MYCELIUM_BIN" ./cmd/mycelium
    
    if [[ ! -f "$MYCELIUM_BIN" ]]; then
        error "Failed to build mycelium binary"
        exit 1
    fi
    
    log "Built mycelium binary: $MYCELIUM_BIN"
}

setup_test_environment() {
    log "Setting up test environment in: $TEST_DIR"
    
    # Create directories
    mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$SKILLS_DIR"
    
    # Create test config
    cat > "$CONFIG_DIR/config.yaml" << EOF
server:
  host: "127.0.0.1"
  port: 23235
  dataDir: "$DATA_DIR"

storage:
  driver: "sqlite"
  dsn: "$DATA_DIR/mycelium.db"

libraries:
  - name: "local"
    path: "$SKILLS_DIR"
    priority: 100
    description: "Local skills for integration testing"

extraction:
  auto_extract: true
  min_confidence: 0.5
  require_validation: true

hooks:
  credential_scan: true
  format_validation: true
  llm_critic: false

defaults:
  author: "integration-test"
  confidence: 0.7
EOF
    
    log "Created test configuration"
}

start_mycelium_server() {
    log "Starting mycelium server..."
    
    # Start server in background
    "$MYCELIUM_BIN" serve --config "$CONFIG_DIR/config.yaml" &
    MYCELIUM_PID=$!
    
    # Wait for server to start
    log "Waiting for server to start (PID: $MYCELIUM_PID)..."
    sleep 10
    
    # Check if server is still running
    if ! kill -0 "$MYCELIUM_PID" 2>/dev/null; then
        error "Mycelium server failed to start or exited"
        exit 1
    fi
    
    # Test SSH connection
    log "Testing SSH connection to server..."
    local retries=30
    local connected=false
    local admin_key_path="$DATA_DIR/mycelium_admin_ed25519"
    
    # Wait for admin key to be generated
    for ((i=1; i<=10; i++)); do
        if [[ -f "$admin_key_path" ]]; then
            break
        fi
        log "Waiting for admin SSH key to be generated... (attempt $i/10)"
        sleep 1
    done
    
    if [[ ! -f "$admin_key_path" ]]; then
        error "Admin SSH key not found at $admin_key_path"
        exit 1
    fi
    
    for ((i=1; i<=retries; i++)); do
        if ssh -p 23235 -i "$admin_key_path" -o ConnectTimeout=2 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR 127.0.0.1 repo list &>/dev/null; then
            connected=true
            break
        fi
        sleep 1
    done
    
    if [[ "$connected" != "true" ]]; then
        error "Could not connect to mycelium server via SSH using admin key"
        exit 1
    fi
    
    log "Mycelium server is running and accepting connections"
}

test_repo_management() {
    log "Testing repository management..."
    
    local repo_name="test-skill-integration"
    local clone_dir="$TEST_DIR/cloned-repo"
    local admin_key_path="$DATA_DIR/mycelium_admin_ed25519"
    
    # Wait for admin key to be generated
    local retries=10
    for ((i=1; i<=retries; i++)); do
        if [[ -f "$admin_key_path" ]]; then
            break
        fi
        log "Waiting for admin SSH key to be generated... (attempt $i/$retries)"
        sleep 1
    done
    
    if [[ ! -f "$admin_key_path" ]]; then
        error "Admin SSH key not found at $admin_key_path"
        exit 1
    fi
    
    # Create repository via SSH using admin key
    log "Creating repository: $repo_name"
    ssh -p 23235 -i "$admin_key_path" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR 127.0.0.1 repo create "$repo_name"
    
    # List repositories
    log "Listing repositories..."
    local repos
    repos=$(ssh -p 23235 -i "$admin_key_path" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR 127.0.0.1 repo list)
    echo "$repos"
    
    if ! echo "$repos" | grep -q "$repo_name"; then
        error "Repository $repo_name not found in list"
        exit 1
    fi
    
    # Clone repository
    log "Cloning repository..."
    export GIT_SSH_COMMAND="ssh -p 23235 -i $admin_key_path -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR"
    git clone "ssh://127.0.0.1:23235/$repo_name" "$clone_dir"
    
    if [[ ! -d "$clone_dir" ]]; then
        error "Failed to clone repository"
        exit 1
    fi
    
    # Create a test skill file
    log "Creating test skill file..."
    cd "$clone_dir"
    
    cat > SKILL.md << EOF
# Test Skill

## Description
This is a test skill created during integration testing.

## Usage
\`\`\`bash
echo "Hello from test skill"
\`\`\`

## Metadata
- Author: integration-test
- Version: 1.0.0
- Confidence: 0.8
EOF
    
    cat > skill.sh << EOF
#!/bin/bash
echo "Hello from test skill!"
EOF
    
    chmod +x skill.sh
    
    # Commit and push
    log "Committing and pushing test skill..."
    git config user.email "test@example.com"
    git config user.name "Integration Test"
    
    git add .
    git commit -m "Add test skill"
    git push origin master
    
    log "Successfully pushed test skill to repository"
    
    # Verify the push worked by cloning again
    local verify_dir="$TEST_DIR/verify-clone"
    log "Verifying push by cloning again..."
    git clone "ssh://127.0.0.1:23235/$repo_name" "$verify_dir"
    
    if [[ ! -f "$verify_dir/SKILL.md" ]] || [[ ! -f "$verify_dir/skill.sh" ]]; then
        error "Files not found in verification clone"
        exit 1
    fi
    
    log "Repository management test passed"
    
    # Test HTTP clone as well
    log "Testing HTTP clone..."
    local http_clone_dir="$TEST_DIR/http-clone"
    git clone "http://127.0.0.1:23236/$repo_name" "$http_clone_dir"
    
    if [[ ! -f "$http_clone_dir/SKILL.md" ]]; then
        error "HTTP clone failed"
        exit 1
    fi
    
    log "HTTP clone test passed"
    
    # Clean up repository
    log "Cleaning up test repository..."
    ssh -p 23235 -i "$admin_key_path" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR 127.0.0.1 repo delete "$repo_name"
    
    log "Repository management tests completed successfully"
}

run_integration_tests() {
    log "Starting Mycelium integration tests..."
    
    check_prerequisites
    build_mycelium
    setup_test_environment
    start_mycelium_server
    test_repo_management
    
    log "All integration tests passed!"
}

main() {
    if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]]; then
        echo "Usage: $0"
        echo ""
        echo "Runs integration tests for Mycelium with real Soft Serve integration."
        echo "Requires 'soft' and 'git' binaries to be available."
        echo ""
        echo "The test will:"
        echo "  1. Build the mycelium binary"
        echo "  2. Start a mycelium server with Soft Serve"
        echo "  3. Create, clone, push to, and delete a test repository"
        echo "  4. Test both SSH and HTTP clone methods"
        echo ""
        echo "Test artifacts are created in /tmp/mycelium-integration-test-<pid>"
        echo "and cleaned up automatically on exit."
        exit 0
    fi
    
    run_integration_tests
}

main "$@"