package processor

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
)

// DefaultIgnoreList contains directory names to skip during crawling.
var DefaultIgnoreList = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".idea":        true,
	".vscode":      true,
	"__pycache__":  true,
	".DS_Store":    true,
}

// SupportedExtensions contains file extensions to process.
var SupportedExtensions = []string{
	".txt", ".md", ".json", ".jsonl", ".csv", ".tsv", ".yaml", ".yml",
	".pdf", ".docx", ".xlsx", ".html", ".htm", ".sql",
	".odt", ".ods", ".odp", ".rtf", ".xml",
}

// FileResult represents a discovered file and its content.
type FileResult struct {
	Path    string
	Content string
}

// Walker recursively walks a directory and sends file contents to a channel.
type Walker struct {
	IgnoreList map[string]bool
	Registry   *ExtractorRegistry
}

// NewWalker creates a Walker with default ignore list and extractors.
func NewWalker() *Walker {
	return &Walker{
		IgnoreList: DefaultIgnoreList,
		Registry:   NewExtractorRegistry(),
	}
}

// Walk recursively walks root and sends FileResults to the returned channel.
// The channel is closed when all files have been sent.
func (w *Walker) Walk(root string) (<-chan FileResult, <-chan error) {
	files := make(chan FileResult)
	errc := make(chan error, 1)

	go func() {
		defer close(files)
		defer close(errc)

		errc <- filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				slog.Warn("walk error", "path", path, "error", err)
				return nil // skip but don't abort
			}

			// Skip ignored directories
			if d.IsDir() {
				if w.IgnoreList[d.Name()] {
					slog.Debug("skipping directory", "path", path)
					return fs.SkipDir
				}
				return nil
			}

			// Check if we have an extractor for this extension
			ext := strings.ToLower(filepath.Ext(d.Name()))
			extractor, ok := w.Registry.Get(ext)
			if !ok {
				return nil
			}

			// Extract content
			content, err := extractor.Extract(path)
			if err != nil {
				slog.Warn("extraction error", "path", path, "error", err)
				return nil
			}

			content = strings.TrimSpace(content)
			if content == "" {
				return nil
			}

			slog.Debug("found file", "path", path, "bytes", len(content))
			files <- FileResult{Path: path, Content: content}
			return nil
		})
	}()

	return files, errc
}
