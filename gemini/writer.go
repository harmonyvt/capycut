package gemini

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WriteOptions configures markdown output writing
type WriteOptions struct {
	// OutputDir is the directory to write files to
	OutputDir string

	// Overwrite allows overwriting existing files
	Overwrite bool

	// AddFrontMatter adds YAML front matter to markdown files
	AddFrontMatter bool

	// AddTableOfContents adds a TOC at the beginning of each file
	AddTableOfContents bool

	// CreateIndexFile creates an index.md linking all documents
	CreateIndexFile bool

	// Verbose enables verbose output
	Verbose bool
}

// WriteResult contains information about written files
type WriteResult struct {
	FilesWritten []string
	TotalBytes   int64
	Errors       []error
}

// WriteDocuments writes markdown documents to the filesystem
func WriteDocuments(docs []*MarkdownDocument, opts WriteOptions) (*WriteResult, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("no documents to write")
	}

	// Create output directory if needed
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	result := &WriteResult{
		FilesWritten: make([]string, 0, len(docs)),
	}

	// Write each document
	for _, doc := range docs {
		path := filepath.Join(opts.OutputDir, doc.Filename)

		// Check if file exists
		if !opts.Overwrite {
			if _, err := os.Stat(path); err == nil {
				result.Errors = append(result.Errors, fmt.Errorf("file exists: %s (use --overwrite to replace)", path))
				continue
			}
		}

		// Build content
		content := buildDocumentContent(doc, opts)

		// Write file
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write %s: %w", path, err))
			continue
		}

		result.FilesWritten = append(result.FilesWritten, path)
		result.TotalBytes += int64(len(content))

		if opts.Verbose {
			fmt.Printf("  Wrote: %s (%d bytes)\n", path, len(content))
		}
	}

	// Create index file if requested
	if opts.CreateIndexFile && len(result.FilesWritten) > 1 {
		indexPath := filepath.Join(opts.OutputDir, "index.md")
		indexContent := buildIndexContent(docs, opts)

		if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write index: %w", err))
		} else {
			result.FilesWritten = append(result.FilesWritten, indexPath)
			result.TotalBytes += int64(len(indexContent))

			if opts.Verbose {
				fmt.Printf("  Wrote: %s (%d bytes)\n", indexPath, len(indexContent))
			}
		}
	}

	return result, nil
}

// buildDocumentContent builds the full content for a document
func buildDocumentContent(doc *MarkdownDocument, opts WriteOptions) string {
	var sb strings.Builder

	// Add front matter if requested
	if opts.AddFrontMatter {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("title: %q\n", doc.Title))
		sb.WriteString(fmt.Sprintf("pages: %d-%d\n", doc.PageRange.Start, doc.PageRange.End))
		sb.WriteString(fmt.Sprintf("generated: %s\n", time.Now().Format(time.RFC3339)))
		if doc.Metadata != nil {
			if doc.Metadata.Author != "" {
				sb.WriteString(fmt.Sprintf("author: %q\n", doc.Metadata.Author))
			}
			if doc.Metadata.Language != "" {
				sb.WriteString(fmt.Sprintf("language: %s\n", doc.Metadata.Language))
			}
			if len(doc.Metadata.Keywords) > 0 {
				sb.WriteString("keywords:\n")
				for _, kw := range doc.Metadata.Keywords {
					sb.WriteString(fmt.Sprintf("  - %s\n", kw))
				}
			}
		}
		sb.WriteString("---\n\n")
	}

	// Add table of contents if requested
	if opts.AddTableOfContents && len(doc.Sections) > 1 {
		sb.WriteString("## Table of Contents\n\n")
		for _, section := range doc.Sections {
			indent := strings.Repeat("  ", section.Level-1)
			anchor := strings.ToLower(strings.ReplaceAll(section.Title, " ", "-"))
			sb.WriteString(fmt.Sprintf("%s- [%s](#%s)\n", indent, section.Title, anchor))
		}
		sb.WriteString("\n---\n\n")
	}

	// Add main content
	sb.WriteString(doc.Content)

	// Ensure trailing newline
	content := sb.String()
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	return content
}

// buildIndexContent creates an index file linking all documents
func buildIndexContent(docs []*MarkdownDocument, opts WriteOptions) string {
	var sb strings.Builder

	if opts.AddFrontMatter {
		sb.WriteString("---\n")
		sb.WriteString("title: \"Document Index\"\n")
		sb.WriteString(fmt.Sprintf("generated: %s\n", time.Now().Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("documents: %d\n", len(docs)))
		sb.WriteString("---\n\n")
	}

	sb.WriteString("# Document Index\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().Format("January 2, 2006 3:04 PM")))

	// Calculate total pages
	totalPages := 0
	for _, doc := range docs {
		totalPages += doc.PageRange.End - doc.PageRange.Start + 1
	}
	sb.WriteString(fmt.Sprintf("**Total Documents:** %d  \n", len(docs)))
	sb.WriteString(fmt.Sprintf("**Total Pages:** %d\n\n", totalPages))

	sb.WriteString("## Documents\n\n")
	sb.WriteString("| # | Title | Pages | Filename |\n")
	sb.WriteString("|---|-------|-------|----------|\n")

	for i, doc := range docs {
		pageRange := fmt.Sprintf("%d-%d", doc.PageRange.Start, doc.PageRange.End)
		if doc.PageRange.Start == doc.PageRange.End {
			pageRange = fmt.Sprintf("%d", doc.PageRange.Start)
		}
		sb.WriteString(fmt.Sprintf("| %d | [%s](%s) | %s | %s |\n",
			i+1, doc.Title, doc.Filename, pageRange, doc.Filename))
	}

	sb.WriteString("\n")
	return sb.String()
}

// CleanupFiles removes previously generated files (for regeneration)
func CleanupFiles(paths []string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}
	return nil
}

// GetOutputFilenames returns the filenames that would be generated
func GetOutputFilenames(docs []*MarkdownDocument, outputDir string, createIndex bool) []string {
	paths := make([]string, 0, len(docs)+1)

	for _, doc := range docs {
		paths = append(paths, filepath.Join(outputDir, doc.Filename))
	}

	if createIndex && len(docs) > 1 {
		paths = append(paths, filepath.Join(outputDir, "index.md"))
	}

	return paths
}
