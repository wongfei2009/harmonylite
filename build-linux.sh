#!/bin/sh

# Get version information from Git
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_TAG=$(git describe --tags --exact-match 2>/dev/null || echo "none")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION=$(go version | awk '{print $3}')
PLATFORM="linux-amd64"

# Build using Makefile
make build-linux-amd64 VERSION=$VERSION

# Create dist directory if it doesn't exist
mkdir -p dist/linux/amd64

# Copy the built binary to the dist directory
mv harmonylite dist/linux/amd64/harmonylite

