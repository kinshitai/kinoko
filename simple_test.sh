#!/bin/bash
set -e

echo "🍄 Simple Mycelium Test"
echo "======================="

# Build
echo "🔨 Building..."
go build -o mycelium ./cmd/mycelium

# Test version
echo "📋 Testing version..."
VERSION=$(./mycelium --version)
echo "Version: $VERSION"

# Test help
echo "📋 Testing help..."
./mycelium --help > /dev/null

# Test that serve starts and stops (5 second test)
echo "🚀 Testing serve command (5s test)..."
./mycelium serve &
PID=$!
sleep 2
kill -TERM $PID
wait $PID || true

echo "✅ Basic functionality tests passed!"

# Cleanup
rm -f ./mycelium