#!/usr/bin/env bash
# Install development tools for merrymaker-go
# This script installs Air (live-reload) and other dev dependencies

set -Eeuo pipefail

echo "[INFO] Installing development tools..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "[ERROR] Go is not installed. Please install Go 1.24+ first."
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "[INFO] Go version: $GO_VERSION"

# Install Air (pinned version for consistency)
AIR_VERSION="v1.63.0"
echo "[INFO] Installing Air $AIR_VERSION..."
go install github.com/air-verse/air@${AIR_VERSION}

# Verify Air installation
if command -v air &> /dev/null; then
    INSTALLED_VERSION=$(air -v 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)
    echo "[SUCCESS] Air installed successfully: $INSTALLED_VERSION"
else
    echo "[ERROR] Air installation failed. Make sure \$GOPATH/bin is in your \$PATH"
    echo "[INFO] Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
    echo "       export PATH=\$PATH:\$(go env GOPATH)/bin"
    exit 1
fi

# Check if Bun is installed (for frontend builds)
if ! command -v bun &> /dev/null; then
    echo "[WARNING] Bun is not installed. Frontend builds require Bun."
    echo "[INFO] Install Bun: curl -fsSL https://bun.sh/install | bash"
else
    BUN_VERSION=$(bun --version)
    echo "[SUCCESS] Bun is installed: v$BUN_VERSION"
fi

# Install frontend dependencies
if [ -d "frontend" ]; then
    echo "[INFO] Installing frontend dependencies..."
    cd frontend
    if command -v bun &> /dev/null; then
        bun install
        echo "[SUCCESS] Frontend dependencies installed"
    else
        echo "[WARNING] Skipping frontend dependencies (Bun not installed)"
    fi
    cd ..
fi

echo ""
echo "[SUCCESS] Development tools installed successfully!"
echo ""
echo "Quick start:"
echo "  make dev-full    # Start full dev environment (DB + live-reload)"
echo "  make dev         # Start live-reload only (no DB)"
echo ""
echo "See docs/development-workflow.md for more details."

