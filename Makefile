.PHONY: build run test test-integration clean release-dry release release-version install dev help download-test-video

# Build variables
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Test video URL (Discord video)
TEST_VIDEO_URL ?= https://cdn.discordapp.com/attachments/719548482825748511/1321112526980780033/C566AC0F-5DDC-4F8D-9BF9-E8BE6A508198.mp4?ex=692f972b&is=692e45ab&hm=228adc7ec03ac02eac6b52a6a4cbfac4de32fe6659405a703c41ef65507d4138&
TEST_VIDEO_PATH ?= test_video.mp4

# Default target
help:
	@echo "CapyCut Development Commands"
	@echo ""
	@echo "  make build            Build the binary"
	@echo "  make run              Build and run interactively"
	@echo "  make test             Run all unit tests"
	@echo "  make test-integration Run integration tests (requires test video)"
	@echo "  make download-test-video  Download test video for integration tests"
	@echo "  make dev              Run without building (go run)"
	@echo "  make clean            Remove build artifacts"
	@echo "  make install          Install to /usr/local/bin"
	@echo ""
	@echo "  make release-dry      Test release build locally"
	@echo "  make release          Create and push a new release tag (interactive)"
	@echo "  make release-version VERSION=x.y.z"
	@echo "                        Create and push a specific release tag (non-interactive)"
	@echo ""
	@echo "  make debug            Run with delve debugger"
	@echo "  make version          Show version info"

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

# Run unit tests
test:
	@echo "Running unit tests..."
	@go test -v ./...

# Download test video for integration tests
download-test-video:
	@if [ ! -f $(TEST_VIDEO_PATH) ]; then \
		echo "Downloading test video..."; \
		curl -L -o $(TEST_VIDEO_PATH) "$(TEST_VIDEO_URL)"; \
		echo "Downloaded: $(TEST_VIDEO_PATH)"; \
	else \
		echo "Test video already exists: $(TEST_VIDEO_PATH)"; \
	fi

# Run integration tests (requires test video and FFmpeg)
test-integration: build download-test-video
	@echo "Running integration tests..."
	@which ffmpeg > /dev/null || (echo "Error: FFmpeg is required for integration tests" && exit 1)
	@TEST_VIDEO_PATH=$(TEST_VIDEO_PATH) go test -v -tags=integration ./...

# Clean build artifacts
clean:
	@rm -f capycut
	@rm -f $(TEST_VIDEO_PATH)
	@rm -rf dist/
	@rm -f *_clip*.mp4
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

# Create a specific release (non-interactive)
# Usage: make release-version VERSION=0.0.5
release-version:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION not specified. Usage: make release-version VERSION=0.0.5"; \
		exit 1; \
	fi
	@echo "Current version tags:"
	@git tag -l "v*" | tail -5 || echo "  (none)"
	@echo ""
	@echo "Creating release v$(VERSION)..."
	@git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	@git push origin "v$(VERSION)"
	@echo ""
	@echo "Release v$(VERSION) triggered! Check:"
	@echo "  https://github.com/harmonyvt/capycut/actions"

# Debug with delve
debug:
	@which dlv > /dev/null || (echo "Installing delve..." && go install github.com/go-delve/delve/cmd/dlv@latest)
	@dlv debug $(LDFLAGS) .

# Show version
version: build
	@./capycut --version
