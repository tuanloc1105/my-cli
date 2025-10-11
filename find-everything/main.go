package main

import (
	"bufio"
	"common-module/utils"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
)

// Colors for terminal output
const (
	ColorHeader    = "\033[95m"
	ColorOKBlue    = "\033[94m"
	ColorOKCyan    = "\033[96m"
	ColorOKGreen   = "\033[92m"
	ColorWarning   = "\033[93m"
	ColorFail      = "\033[91m"
	ColorEndC      = "\033[0m"
	ColorBold      = "\033[1m"
	ColorUnderline = "\033[4m"
)

// ProgressTracker tracks search progress
type ProgressTracker struct {
	mu             sync.Mutex
	totalDirs      int64
	processedDirs  int64
	foundFiles     int64
	foundDirs      int64
	startTime      time.Time
	lastUpdate     time.Time
	updateInterval time.Duration
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		startTime:      time.Now(),
		lastUpdate:     time.Now(),
		updateInterval: 100 * time.Millisecond, // Update progress every 100ms
	}
}

func (pt *ProgressTracker) Update(filesCount, dirsCount int) {
	atomic.AddInt64(&pt.foundFiles, int64(filesCount))
	atomic.AddInt64(&pt.foundDirs, int64(dirsCount))
}

func (pt *ProgressTracker) UpdateProcessedDirs(count int) {
	atomic.AddInt64(&pt.processedDirs, int64(count))
}

func (pt *ProgressTracker) SetTotalDirs(total int) {
	atomic.StoreInt64(&pt.totalDirs, int64(total))
}

func (pt *ProgressTracker) PrintProgress() {
	now := time.Now()
	if now.Sub(pt.lastUpdate) < pt.updateInterval {
		return // Skip update if not enough time has passed
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.lastUpdate = now
	elapsed := time.Since(pt.startTime).Seconds()
	// totalDirs := atomic.LoadInt64(&pt.totalDirs)
	processedDirs := atomic.LoadInt64(&pt.processedDirs)
	foundFiles := atomic.LoadInt64(&pt.foundFiles)
	foundDirs := atomic.LoadInt64(&pt.foundDirs)
	fmt.Printf("\r%sProcessed: %d | Found: %d files, %d dirs | Time: %.1fs%s",
		ColorOKCyan, processedDirs, foundFiles, foundDirs, elapsed, ColorEndC)

	/* if totalDirs > 0 {
		progress := float64(processedDirs) / float64(totalDirs) * 100
		fmt.Printf("\r%sProgress: %.1f%% | Processed: %d/%d | Found: %d files, %d dirs | Time: %.1fs%s",
			ColorOKCyan, progress, processedDirs, totalDirs, foundFiles, foundDirs, elapsed, ColorEndC)
	} */ /* else {
		fmt.Printf("\r%sProcessed: %d | Found: %d files, %d dirs | Time: %.1fs%s",
			ColorOKCyan, processedDirs, foundFiles, foundDirs, elapsed, ColorEndC)
	} */
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
	progressTracker *ProgressTracker
	patternRegex    *regexp.Regexp
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex
	fileCache       map[string]int64 // Cache file sizes to avoid repeated stat calls
}

// SearchResult represents a single search result
type SearchResult struct {
	Path     string
	IsDir    bool
	Size     int64
	FullPath string
}

func NewFileFinder(basePath, pattern string, options map[string]interface{}) (*FileFinder, error) {
	// Compile pattern regex
	regexPattern := globToRegex(pattern)
	if !options["caseSensitive"].(bool) {
		regexPattern = "(?i)" + regexPattern
	}
	patternRegex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}

	// Compile exclude patterns
	var excludePatterns []*regexp.Regexp
	for _, pattern := range options["excludePatterns"].([]string) {
		if re, err := regexp.Compile(pattern); err == nil {
			excludePatterns = append(excludePatterns, re)
		}
	}

	// Build exclude dirs set
	excludeDirs := make(map[string]bool)
	for _, dir := range options["excludeDirs"].([]string) {
		excludeDirs[strings.ToLower(dir)] = true
	}

	// Build file types set
	fileTypes := make(map[string]bool)
	for _, ext := range options["fileTypes"].([]string) {
		fileTypes[strings.ToLower(ext)] = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	maxWorkers := options["maxWorkers"].(int)

	return &FileFinder{
		basePath:        basePath,
		pattern:         pattern,
		caseSensitive:   options["caseSensitive"].(bool),
		maxWorkers:      maxWorkers,
		excludeDirs:     excludeDirs,
		excludePatterns: excludePatterns,
		fileTypes:       fileTypes,
		minSize:         options["minSize"].(int64),
		maxSize:         options["maxSize"].(int64),
		showProgress:    options["showProgress"].(bool),
		maxResults:      options["maxResults"].(int),
		progressTracker: NewProgressTracker(),
		patternRegex:    patternRegex,
		ctx:             ctx,
		cancel:          cancel,
		fileCache:       make(map[string]int64),
	}, nil
}

func (ff *FileFinder) shouldExclude(path string) bool {
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

func (ff *FileFinder) matchesPattern(name string) bool {
	return ff.patternRegex.MatchString(name)
}

func (ff *FileFinder) getFileSize(filePath string) (int64, bool) {
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

func (ff *FileFinder) checkFileSize(filePath string) bool {
	size, ok := ff.getFileSize(filePath)
	if !ok {
		return false
	}
	return size >= ff.minSize && size <= ff.maxSize
}

func (ff *FileFinder) checkFileType(filePath string) bool {
	if len(ff.fileTypes) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	return ff.fileTypes[ext]
}

func (ff *FileFinder) processDirectory(root string, entries []fs.DirEntry) []SearchResult {
	// Pre-allocate slices with estimated capacity
	estimatedCapacity := len(entries) / 4 // Assume 25% match rate
	if estimatedCapacity < 10 {
		estimatedCapacity = 10
	}

	results := make([]SearchResult, 0, estimatedCapacity)

	for _, entry := range entries {
		entryName := entry.Name()
		fullPath := filepath.Join(root, entryName)

		if ff.shouldExclude(fullPath) {
			continue
		}

		if ff.matchesPattern(entryName) {
			if entry.IsDir() {
				results = append(results, SearchResult{Path: entryName, IsDir: true, FullPath: fullPath})
			} else {
				// Check file type first (cheaper than size check)
				if ff.checkFileType(fullPath) && ff.checkFileSize(fullPath) {
					size, _ := ff.getFileSize(fullPath)
					results = append(results, SearchResult{Path: entryName, IsDir: false, Size: size, FullPath: fullPath})
				}
			}
		}
	}

	return results
}

type dirJob struct {
	path    string
	entries []fs.DirEntry
}

func (ff *FileFinder) findFilesAndDirs() ([]string, []string) {
	if ff.showProgress {
		fmt.Printf("%sStarting search...%s\n", ColorOKBlue, ColorEndC)
	}

	// Start progress updater goroutine
	var progressTicker *time.Ticker
	if ff.showProgress {
		progressTicker = time.NewTicker(100 * time.Millisecond)
		defer progressTicker.Stop()
		go func() {
			for {
				select {
				case <-progressTicker.C:
					ff.progressTracker.PrintProgress()
				case <-ff.ctx.Done():
					return
				}
			}
		}()
	}

	fmt.Println("Max workers: ", ff.maxWorkers)

	// Collect results with proper synchronization
	var matchedFiles []string
	var matchedDirs []string
	var resultsMu sync.Mutex

	// Process directories with improved concurrency - single pass
	dirQueue := make(chan dirJob, ff.maxWorkers*4)
	var dirWg sync.WaitGroup
	totalDirs := int64(0)

	// Start directory processors
	for i := 0; i < ff.maxWorkers; i++ {
		dirWg.Add(1)
		go func() {
			defer dirWg.Done()
			localFiles := make([]string, 0, 100)
			localDirs := make([]string, 0, 100)

			for {
				select {
				case job, ok := <-dirQueue:
					if !ok {
						// Flush local results
						if len(localFiles) > 0 || len(localDirs) > 0 {
							resultsMu.Lock()
							matchedFiles = append(matchedFiles, localFiles...)
							matchedDirs = append(matchedDirs, localDirs...)
							resultsMu.Unlock()
						}
						return
					}

					results := ff.processDirectory(job.path, job.entries)

					// Batch results locally to reduce lock contention
					for _, result := range results {
						if result.IsDir {
							localDirs = append(localDirs, result.FullPath)
						} else {
							localFiles = append(localFiles, result.FullPath)
						}
					}

					// Flush when batch gets large
					if len(localFiles)+len(localDirs) > 100 {
						resultsMu.Lock()
						matchedFiles = append(matchedFiles, localFiles...)
						matchedDirs = append(matchedDirs, localDirs...)
						newCount := int32(len(matchedFiles) + len(matchedDirs))
						resultsMu.Unlock()

						localFiles = localFiles[:0]
						localDirs = localDirs[:0]

						// Check if we've reached max results
						if newCount >= int32(ff.maxResults) {
							ff.cancel()
							return
						}
					}

					ff.progressTracker.Update(len(results), 0)
					ff.progressTracker.UpdateProcessedDirs(1)

				case <-ff.ctx.Done():
					return
				}
			}
		}()
	}

	// Walk directories and queue them for processing - single pass
	filepath.WalkDir(ff.basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		if ff.shouldExclude(path) {
			return filepath.SkipDir
		}

		// Check for cancellation
		select {
		case <-ff.ctx.Done():
			return filepath.SkipAll
		default:
		}

		// Count and process in one pass
		atomic.AddInt64(&totalDirs, 1)
		ff.progressTracker.SetTotalDirs(int(atomic.LoadInt64(&totalDirs)))

		// Read directory entries
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		// Queue directory for processing
		select {
		case dirQueue <- dirJob{path: path, entries: entries}:
		case <-ff.ctx.Done():
			return filepath.SkipAll
		}

		return nil
	})

	// Close channels and wait for completion
	close(dirQueue)
	dirWg.Wait()

	if ff.showProgress {
		fmt.Println() // New line after progress
	}

	return matchedFiles, matchedDirs
}

// Utility functions
func globToRegex(pattern string) string {
	// Simple glob to regex conversion
	pattern = regexp.QuoteMeta(pattern)
	pattern = strings.ReplaceAll(pattern, "\\*", ".*")
	pattern = strings.ReplaceAll(pattern, "\\?", ".")
	return "^" + pattern + "$"
}

func formatSize(sizeBytes int64) string {
	const unit = 1024
	if sizeBytes < unit {
		return fmt.Sprintf("%d B", sizeBytes)
	}
	div, exp := int64(unit), 0
	for n := sizeBytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(sizeBytes)/float64(div), "KMGTPE"[exp])
}

func parseSize(sizeStr string) (int64, error) {
	if strings.ToLower(sizeStr) == "inf" {
		return 1<<63 - 1, nil // Max int64
	}

	sizeStr = strings.ToUpper(sizeStr)
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	for unit, multiplier := range multipliers {
		if strings.HasSuffix(sizeStr, unit) {
			numStr := strings.TrimSuffix(sizeStr, unit)
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, err
			}
			return int64(num * float64(multiplier)), nil
		}
	}

	// No unit specified, assume bytes
	return strconv.ParseInt(sizeStr, 10, 64)
}

func saveResultsToFile(files, dirs []string, pattern, basePath string, showDetails bool) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("search_results_%s.txt", timestamp)

	file, err := os.Create(filename)
	if err != nil {
		return ""
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	fmt.Fprintf(writer, "Enhanced File and Directory Finder Results\n")
	fmt.Fprintf(writer, "%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(writer, "Search Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(writer, "Base Path: %s\n", basePath)
	fmt.Fprintf(writer, "Search Pattern: %s\n", pattern)
	fmt.Fprintf(writer, "Files found: %d\n", len(files))
	fmt.Fprintf(writer, "Directories found: %d\n", len(dirs))
	fmt.Fprintf(writer, "Total results: %d\n", len(files)+len(dirs))
	fmt.Fprintf(writer, "%s\n\n", strings.Repeat("=", 80))

	if len(files) > 0 {
		fmt.Fprintf(writer, "MATCHING FILES:\n")
		fmt.Fprintf(writer, "%s\n", strings.Repeat("-", 40))
		sort.Strings(files)
		for _, filePath := range files {
			if showDetails {
				if info, err := os.Stat(filePath); err == nil {
					fmt.Fprintf(writer, "  %s (%s)\n", filePath, formatSize(info.Size()))
				} else {
					fmt.Fprintf(writer, "  %s (size unknown)\n", filePath)
				}
			} else {
				fmt.Fprintf(writer, "  %s\n", filePath)
			}
		}
		fmt.Fprintf(writer, "\n")
	}

	if len(dirs) > 0 {
		fmt.Fprintf(writer, "MATCHING DIRECTORIES:\n")
		fmt.Fprintf(writer, "%s\n", strings.Repeat("-", 40))
		sort.Strings(dirs)
		for _, dirPath := range dirs {
			fmt.Fprintf(writer, "  %s\n", dirPath)
		}
		fmt.Fprintf(writer, "\n")
	}

	return filename
}

func printResults(files, dirs []string, showDetails bool, pattern, basePath string) {
	totalResults := len(files) + len(dirs)

	// If results exceed 100, save to file instead of printing
	if totalResults > 100 {
		filename := saveResultsToFile(files, dirs, pattern, basePath, showDetails)
		fmt.Printf("\n%s%sSearch Results:%s\n", ColorBold, ColorHeader, ColorEndC)
		fmt.Printf("%sFiles found: %d%s\n", ColorOKGreen, len(files), ColorEndC)
		fmt.Printf("%sDirectories found: %d%s\n", ColorOKBlue, len(dirs), ColorEndC)
		fmt.Printf("%sTotal results: %d (exceeds 100)%s\n", ColorWarning, totalResults, ColorEndC)
		fmt.Printf("%sResults saved to: %s%s\n", ColorOKCyan, filename, ColorEndC)
		return
	}

	// Print to console if results <= 100
	fmt.Printf("\n%s%sSearch Results:%s\n", ColorBold, ColorHeader, ColorEndC)
	fmt.Printf("%sFiles found: %d%s\n", ColorOKGreen, len(files), ColorEndC)
	fmt.Printf("%sDirectories found: %d%s\n", ColorOKBlue, len(dirs), ColorEndC)

	if len(files) > 0 {
		fmt.Printf("\n%s%sMatching Files:%s\n", ColorBold, ColorOKGreen, ColorEndC)
		sort.Strings(files)
		for _, filePath := range files {
			if showDetails {
				if info, err := os.Stat(filePath); err == nil {
					fmt.Printf("  %s (%s)\n", filePath, formatSize(info.Size()))
				} else {
					fmt.Printf("  %s (size unknown)\n", filePath)
				}
			} else {
				fmt.Printf("  %s\n", filePath)
			}
		}
	}

	if len(dirs) > 0 {
		fmt.Printf("\n%s%sMatching Directories:%s\n", ColorBold, ColorOKBlue, ColorEndC)
		sort.Strings(dirs)
		for _, dirPath := range dirs {
			fmt.Printf("  %s\n", dirPath)
		}
	}
}

func main() {
	var (
		caseSensitive   bool
		maxWorkers      int
		excludeDirs     []string
		excludePatterns []string
		fileTypes       []string
		minSize         string
		maxSize         string
		maxResults      int
		noProgress      bool
		showDetails     bool
	)

	rootCmd := &cobra.Command{
		Use:   "find-everything [base-path] [pattern]",
		Short: "Enhanced file and directory finder with advanced filtering options",
		Long: `Enhanced file and directory finder with advanced filtering options.

This tool provides comprehensive file and directory searching capabilities with
support for glob patterns, size filtering, file type filtering, and exclusion rules.`,
		Example: `  find-everything "C:\" "*.txt" --file-types .txt .log
  find-everything "/home/user" "*.py" --exclude-dirs node_modules .git
  find-everything "D:\" "zalo*" --min-size 1MB --max-size 100MB
  find-everything "." "*.jpg" --case-sensitive --show-details`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			basePath := args[0]
			pattern := args[1]

			// Parse size arguments
			minSizeBytes, err := parseSize(minSize)
			if err != nil {
				return fmt.Errorf("error parsing min-size: %v", err)
			}

			maxSizeBytes, err := parseSize(maxSize)
			if err != nil {
				return fmt.Errorf("error parsing max-size: %v", err)
			}

			// Process exclude_dirs to handle comma-separated values
			processedExcludeDirs := []string{}
			for _, item := range excludeDirs {
				for _, dir := range strings.Split(item, ",") {
					dir = strings.TrimSpace(dir)
					if dir != "" {
						processedExcludeDirs = append(processedExcludeDirs, dir)
					}
				}
			}

			// Clear screen
			utils.CLS()

			fmt.Printf("%s%sEnhanced File and Directory Finder%s\n", ColorBold, ColorHeader, ColorEndC)
			fmt.Printf("%sSearching in: %s%s\n", ColorOKBlue, basePath, ColorEndC)
			fmt.Printf("%sPattern: %s%s\n", ColorOKBlue, pattern, ColorEndC)

			options := map[string]interface{}{
				"caseSensitive":   caseSensitive,
				"maxWorkers":      maxWorkers,
				"excludeDirs":     processedExcludeDirs,
				"excludePatterns": excludePatterns,
				"fileTypes":       fileTypes,
				"minSize":         minSizeBytes,
				"maxSize":         maxSizeBytes,
				"maxResults":      maxResults,
				"showProgress":    !noProgress,
			}

			finder, err := NewFileFinder(basePath, pattern, options)
			if err != nil {
				return err
			}

			files, dirs := finder.findFilesAndDirs()
			printResults(files, dirs, showDetails, pattern, basePath)

			return nil
		},
	}

	// Add flags
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "Case sensitive search")
	rootCmd.Flags().IntVarP(&maxWorkers, "max-workers", "w", runtime.NumCPU(), "Maximum number of worker goroutines")
	rootCmd.Flags().StringSliceVarP(&excludeDirs, "exclude-dirs", "e", []string{}, "Directories to exclude from search")
	rootCmd.Flags().StringSliceVarP(&excludePatterns, "exclude-patterns", "p", []string{}, "Patterns to exclude (regex)")
	rootCmd.Flags().StringSliceVarP(&fileTypes, "file-types", "t", []string{}, "File extensions to include")
	rootCmd.Flags().StringVar(&minSize, "min-size", "0", "Minimum file size (e.g., 1KB, 1MB, 1GB)")
	rootCmd.Flags().StringVar(&maxSize, "max-size", "inf", "Maximum file size (e.g., 1KB, 1MB, 1GB)")
	rootCmd.Flags().IntVar(&maxResults, "max-results", 10000, "Maximum number of results to find")
	rootCmd.Flags().BoolVar(&noProgress, "no-progress", false, "Disable progress display")
	rootCmd.Flags().BoolVarP(&showDetails, "show-details", "d", false, "Show file sizes and details")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("%sError: %v%s\n", ColorFail, err, ColorEndC)
		os.Exit(1)
	}
}
