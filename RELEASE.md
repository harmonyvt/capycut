# Creating a Release for CapyCut

This document describes how to create a new release using the existing Makefile.

## Prerequisites

- You must have push access to the repository
- You should be on the main/default branch (or have permission to create tags from your current branch)
- Your working directory should be clean

## Method 1: Using Make (Interactive)

The Makefile includes a `release` target that guides you through creating a release:

```bash
make release
```

This will:
1. Show current version tags
2. Prompt you to enter a new version (e.g., `0.0.5`)
3. Create an annotated git tag (e.g., `v0.0.5`)
4. Push the tag to GitHub
5. Trigger the GitHub Actions workflow to build and publish the release

## Method 2: Using Make (Non-Interactive)

For automation or CI/CD, use the `release-version` target:

```bash
make release-version VERSION=0.0.5
```

This does the same as the interactive version but without prompting for input.

## Method 3: Using Make with Manual Steps

If the interactive `make release` doesn't work in your environment, you can manually create and push the tag:

```bash
# View existing tags
git tag -l "v*"

# Create the tag for version 0.0.5
git tag -a "v0.0.5" -m "Release v0.0.5"

# Push the tag
git push origin "v0.0.5"
```

## Method 4: Using the Helper Script

A helper script is provided in `scripts/create-release.sh`:

```bash
# Create release v0.0.5 (default)
./scripts/create-release.sh

# Or specify a different version
./scripts/create-release.sh 0.1.0
```

## What Happens After Pushing the Tag

Once the tag is pushed to GitHub:

1. The `.github/workflows/release.yml` workflow is triggered automatically
2. GoReleaser builds binaries for multiple platforms (Linux, macOS, Windows)
3. A GitHub Release is created with the built binaries
4. The release will be available at: https://github.com/harmonyvt/capycut/releases/tag/v0.0.5

## Repository Protection Rules

Note: If you encounter an error like "Cannot create ref due to creations being restricted", this means:
- The repository has protection rules that restrict tag creation
- You may need to create the tag from the main branch
- Or you may need additional permissions

To resolve this:
1. Ensure you're on the main/default branch
2. Or ask a repository administrator to adjust the protection rules
3. Or ask someone with appropriate permissions to create the tag

## For Version 0.0.5

To create release v0.0.5 specifically, run from the main branch:

**Interactive:**
```bash
make release
# When prompted, enter: 0.0.5
```

**Non-interactive:**
```bash
make release-version VERSION=0.0.5
```

**Manual:**
```bash
git tag -a "v0.0.5" -m "Release v0.0.5"
git push origin "v0.0.5"
```
