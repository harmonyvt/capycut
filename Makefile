.PHONY: build run test clean release-dry release install dev help

# Build variables
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Default target
help:
	@echo "CapyCut Development Commands"
	@echo ""
	@echo "  make build        Build the binary"
	@echo "  make run          Build and run interactively"
	@echo "  make test         Run all tests"
	@echo "  make dev          Run without building (go run)"
	@echo "  make clean        Remove build artifacts"
	@echo "  make install      Install to /usr/local/bin"
	@echo ""
	@echo "  make release-dry  Test release build locally"
	@echo "  make release      Create and push a new release tag"
	@echo ""
	@echo "  make debug        Run with delve debugger"
	@echo "  make version      Show version info"

# Build the binary
build:
	@echo "Building capycut..."
	@go build $(LDFLAGS) -o capycut .
	@echo "Done: ./capycut"

# Run interactively
run: build
	@./capycut

# Run without building
dev:
	@go run $(LDFLAGS) .

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@rm -f capycut
	@rm -rf dist/
	@echo "Cleaned."

# Install to system
install: build
	@echo "Installing to /usr/local/bin..."
	@sudo cp capycut /usr/local/bin/
	@echo "Installed! Run 'capycut' from anywhere."

# Test release locally (no publish)
release-dry:
	@echo "Testing release build..."
	@goreleaser release --snapshot --clean

# Create a release (interactive)
release:
	@echo "Current version tags:"
	@git tag -l "v*" | tail -5 || echo "  (none)"
	@echo ""
	@read -p "Enter new version (e.g., 0.1.0): " ver; \
	git tag -a "v$$ver" -m "Release v$$ver"; \
	git push origin "v$$ver"; \
	echo ""; \
	echo "Release v$$ver triggered! Check:"; \
	echo "  https://github.com/harmonyvt/capycut/actions"

# Debug with delve
debug:
	@which dlv > /dev/null || (echo "Installing delve..." && go install github.com/go-delve/delve/cmd/dlv@latest)
	@dlv debug $(LDFLAGS) .

# Show version
version: build
	@./capycut --version
