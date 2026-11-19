#!/usr/bin/env bash
# Run this script from WSL / Linux to set up deps and run tests.
# Usage:  bash bootstrap.sh
set -euo pipefail

cd "$(dirname "$0")"

echo "==> go mod tidy (fetch deps)..."
go mod tidy

echo "==> go vet ./... (static analysis)..."
go vet ./...

echo "==> go test ./processor/... -v (unit tests)..."
go test ./processor/... -v -count=1

echo ""
echo "All checks passed! Run the bot with: go run ."
