package finder

import (
	"context"
	"find-everything/internal/ui"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// FinderOptions holds all configuration for FileFinder
type FinderOptions struct {
	CaseSensitive   bool
	MaxWorkers      int
	ExcludeDirs     []string
	ExcludePatterns []string
	FileTypes       []string
	MinSize         int64
	MaxSize         int64
	ShowProgress    bool
	MaxResults      int
}

// FileFinder handles file and directory searching
type FileFinder struct {
	basePath        string
	pattern         string
	caseSensitive   bool
	maxWorkers      int
	excludeDirs     map[string]bool
	excludePatterns []*regexp.Regexp
	fileTypes       map[string]bool
	minSize         int64
	maxSize         int64
	showProgress    bool
	maxResults      int
	progressTracker *ui.ProgressTracker
	patternRegex    *regexp.Regexp
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex
	fileCache       map[string]int64 // Cache file sizes to avoid repeated stat calls
}

func NewFileFinder(basePath, pattern string, opts FinderOptions) (*FileFinder, error) {
	// Compile pattern regex
	regexPattern := GlobToRegex(pattern)
	if !opts.CaseSensitive {
		regexPattern = "(?i)" + regexPattern
	}
	patternRegex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}

	// Compile exclude patterns
	var excludePatterns []*regexp.Regexp
	for _, p := range opts.ExcludePatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %v", p, err)
		}
		excludePatterns = append(excludePatterns, re)
	}

	// Build exclude dirs set
	excludeDirs := make(map[string]bool)
	for _, dir := range opts.ExcludeDirs {
		excludeDirs[strings.ToLower(dir)] = true
	}

	// Build file types set
	fileTypes := make(map[string]bool)
	for _, ext := range opts.FileTypes {
		fileTypes[strings.ToLower(ext)] = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	maxWorkers := opts.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	return &FileFinder{
		basePath:        basePath,
		pattern:         pattern,
		caseSensitive:   opts.CaseSensitive,
		maxWorkers:      maxWorkers,
		excludeDirs:     excludeDirs,
		excludePatterns: excludePatterns,
		fileTypes:       fileTypes,
		minSize:         opts.MinSize,
		maxSize:         opts.MaxSize,
		showProgress:    opts.ShowProgress,
		maxResults:      opts.MaxResults,
		progressTracker: ui.NewProgressTracker(),
		patternRegex:    patternRegex,
		ctx:             ctx,
		cancel:          cancel,
		fileCache:       make(map[string]int64),
	}, nil
}

func (ff *FileFinder) ShouldExclude(path string) bool {
	// Check if any component of the path matches an excluded directory name
	parts := strings.Split(path, string(os.PathSeparator))
	for _, part := range parts {
		if ff.excludeDirs[strings.ToLower(part)] {
			return true
		}
	}

	// Check exclude patterns (regex)
	for _, regex := range ff.excludePatterns {
		if regex.MatchString(path) {
			return true
		}
	}

	return false
}

func (ff *FileFinder) MatchesPattern(name string) bool {
	return ff.patternRegex.MatchString(name)
}

func (ff *FileFinder) GetFileSize(filePath string) (int64, bool) {
	// Check cache first
	ff.mu.RLock()
	if size, exists := ff.fileCache[filePath]; exists {
		ff.mu.RUnlock()
		return size, true
	}
	ff.mu.RUnlock()

	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, false
	}
	size := info.Size()

	// Cache the result with size limit to prevent memory explosion
	ff.mu.Lock()
	if len(ff.fileCache) < 10000 { // Limit cache size
		ff.fileCache[filePath] = size
	}
	ff.mu.Unlock()

	return size, true
}

func (ff *FileFinder) CheckFileSize(filePath string) bool {
	size, ok := ff.GetFileSize(filePath)
	if !ok {
		return false
	}
	return size >= ff.minSize && size <= ff.maxSize
}

func (ff *FileFinder) CheckFileType(filePath string) bool {
	if len(ff.fileTypes) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	return ff.fileTypes[ext]
}

// Utility functions
func GlobToRegex(pattern string) string {
	// Simple glob to regex conversion
	pattern = regexp.QuoteMeta(pattern)
	pattern = strings.ReplaceAll(pattern, "\\*", ".*")
	pattern = strings.ReplaceAll(pattern, "\\?", ".")
	return "^" + pattern + "$"
}
