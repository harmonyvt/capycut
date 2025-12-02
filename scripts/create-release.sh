#!/bin/bash
set -e

# Script to create a release for CapyCut
# This should be run from the main/default branch

VERSION="${1:-0.0.5}"

echo "Creating release for version ${VERSION}..."
echo ""

# Validate version format (semantic versioning: x.y.z)
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Error: VERSION must be in semantic versioning format (x.y.z)"
    echo "Example: ./scripts/create-release.sh 0.0.5"
    exit 1
fi

# Check current branch
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$CURRENT_BRANCH" != "master" && "$CURRENT_BRANCH" != "main" ]]; then
    echo "Warning: You are on branch '$CURRENT_BRANCH'"
    echo "Releases should typically be created from 'main' or 'master' branch."
    read -p "Continue anyway? (y/N): " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        echo "Cancelled."
        exit 1
    fi
fi

echo "Current version tags:"
git tag -l "v*" | tail -5 || echo "  (none)"
echo ""

# Validate we're on a clean branch
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working directory is not clean. Please commit or stash your changes."
    exit 1
fi

# Create the tag
echo "Creating tag v${VERSION}..."
git tag -a "v${VERSION}" -m "Release v${VERSION}"

# Push the tag
echo "Pushing tag v${VERSION} to origin..."
git push origin "v${VERSION}"

echo ""
echo "Release v${VERSION} triggered! Check:"
echo "  https://github.com/harmonyvt/capycut/actions"
echo ""
echo "Once the GitHub Actions workflow completes, the release will be available at:"
echo "  https://github.com/harmonyvt/capycut/releases/tag/v${VERSION}"
