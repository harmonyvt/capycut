package gemini

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadImages loads and validates image files from various sources
// Supports: directory path, glob pattern, or list of file paths
func LoadImages(sources []string) ([]string, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("no image sources provided")
	}

	var allPaths []string
	seen := make(map[string]bool)

	for _, source := range sources {
		paths, err := resolveSource(source)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", source, err)
		}

		for _, p := range paths {
			absPath, err := filepath.Abs(p)
			if err != nil {
				return nil, fmt.Errorf("failed to get absolute path for %s: %w", p, err)
			}

			if !seen[absPath] {
				seen[absPath] = true
				allPaths = append(allPaths, absPath)
			}
		}
	}

	if len(allPaths) == 0 {
		return nil, fmt.Errorf("no valid image files found")
	}

	// Sort by filename for consistent ordering
	sort.Slice(allPaths, func(i, j int) bool {
		return naturalSort(filepath.Base(allPaths[i]), filepath.Base(allPaths[j]))
	})

	return allPaths, nil
}

// resolveSource resolves a source to a list of file paths
func resolveSource(source string) ([]string, error) {
	// Check if it's an existing file
	info, err := os.Stat(source)
	if err == nil {
		if info.IsDir() {
			return loadFromDirectory(source)
		}
		if isImageFile(source) {
			return []string{source}, nil
		}
		return nil, fmt.Errorf("not a supported image file: %s", source)
	}

	// Try as glob pattern
	matches, err := filepath.Glob(source)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no files found matching: %s", source)
	}

	// Filter to only image files
	var imagePaths []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		if info.IsDir() {
			dirImages, err := loadFromDirectory(match)
			if err == nil {
				imagePaths = append(imagePaths, dirImages...)
			}
		} else if isImageFile(match) {
			imagePaths = append(imagePaths, match)
		}
	}

	return imagePaths, nil
}

// loadFromDirectory loads all image files from a directory
func loadFromDirectory(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var images []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if isImageFile(path) {
			images = append(images, path)
		}
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no image files found in directory")
	}

	return images, nil
}

// isImageFile checks if a file has a supported image extension
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, supported := range SupportedImageTypes {
		if ext == supported {
			return true
		}
	}
	return false
}

// naturalSort performs natural sorting for filenames with numbers
// e.g., page_2.png comes before page_10.png
func naturalSort(a, b string) bool {
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)

	aPos, bPos := 0, 0

	for aPos < len(aLower) && bPos < len(bLower) {
		aChar := aLower[aPos]
		bChar := bLower[bPos]

		aIsDigit := aChar >= '0' && aChar <= '9'
		bIsDigit := bChar >= '0' && bChar <= '9'

		if aIsDigit && bIsDigit {
			// Extract full numbers from both strings
			aNumStart := aPos
			bNumStart := bPos

			for aPos < len(aLower) && aLower[aPos] >= '0' && aLower[aPos] <= '9' {
				aPos++
			}
			for bPos < len(bLower) && bLower[bPos] >= '0' && bLower[bPos] <= '9' {
				bPos++
			}

			// Parse numbers
			aNum := parseNumber(aLower[aNumStart:aPos])
			bNum := parseNumber(bLower[bNumStart:bPos])

			if aNum != bNum {
				return aNum < bNum
			}
			// Numbers are equal, continue comparing
		} else {
			// Compare characters directly
			if aChar != bChar {
				return aChar < bChar
			}
			aPos++
			bPos++
		}
	}

	// Shorter string comes first
	return len(aLower) < len(bLower)
}

// parseNumber parses a string of digits into an integer
func parseNumber(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// ValidateImages checks that all images are valid
func ValidateImages(paths []string) error {
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("image %d: failed to access %s: %w", i+1, path, err)
		}

		if info.IsDir() {
			return fmt.Errorf("image %d: %s is a directory", i+1, path)
		}

		if info.Size() > MaxFileSize {
			return fmt.Errorf("image %d: %s exceeds maximum size of 20MB", i+1, path)
		}

		if !isImageFile(path) {
			return fmt.Errorf("image %d: %s is not a supported image format", i+1, path)
		}
	}

	if len(paths) > MaxTotalImages {
		return fmt.Errorf("too many images: %d (maximum %d)", len(paths), MaxTotalImages)
	}

	return nil
}

// GetImageStats returns statistics about a set of images
func GetImageStats(paths []string) (totalSize int64, count int, err error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to stat %s: %w", path, err)
		}
		totalSize += info.Size()
		count++
	}
	return totalSize, count, nil
}

// FormatSize formats a byte size as a human-readable string
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
