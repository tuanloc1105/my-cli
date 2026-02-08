package ui

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"
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
	totalDirs     int64
	processedDirs int64
	foundFiles    int64
	foundDirs     int64
	startTime     time.Time
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		startTime: time.Now(),
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
	elapsed := time.Since(pt.startTime).Seconds()
	processedDirs := atomic.LoadInt64(&pt.processedDirs)
	foundFiles := atomic.LoadInt64(&pt.foundFiles)
	foundDirs := atomic.LoadInt64(&pt.foundDirs)
	fmt.Printf("\r%sProcessed: %d | Found: %d files, %d dirs | Time: %.1fs%s",
		ColorOKCyan, processedDirs, foundFiles, foundDirs, elapsed, ColorEndC)
}

func FormatSize(sizeBytes int64) string {
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

func SaveResultsToFile(files, dirs []string, pattern, basePath string, showDetails bool) string {
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
					fmt.Fprintf(writer, "  %s (%s)\n", filePath, FormatSize(info.Size()))
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

func PrintResults(files, dirs []string, showDetails bool, pattern, basePath string) {
	totalResults := len(files) + len(dirs)

	// If results exceed 100, save to file instead of printing
	if totalResults > 100 {
		filename := SaveResultsToFile(files, dirs, pattern, basePath, showDetails)
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
					fmt.Printf("  %s (%s)\n", filePath, FormatSize(info.Size()))
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
