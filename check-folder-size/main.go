package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[42m\033[1;30m"
	colorYellow = "\033[43m\033[1;30m"
	colorRed    = "\033[41m\033[1;30m"
)

type folderItem struct {
	name string
	size int64
}

type sizeInfo struct {
	value float64
	unit  string
	color string
}

func getTerminalWidth() int {
	// Try to get terminal width, fallback to 80
	if width := getTerminalSize(); width > 0 {
		return width
	}
	return 80
}

func getTerminalSize() int {
	// This is a simplified version - in a real implementation you'd use a library like "golang.org/x/term"
	// For now, we'll return a default value
	return 80
}

func clearLine() {
	width := getTerminalWidth()
	fmt.Printf("\r%s\r", strings.Repeat(" ", width))
}

func color(msg string, colorCode string) string {
	return fmt.Sprintf("%s %s %s", colorCode, msg, colorReset)
}

func shouldExclude(name string, excludeList []string) bool {
	if len(excludeList) == 0 {
		return false
	}

	for _, excludeItem := range excludeList {
		if strings.TrimSpace(excludeItem) == name {
			return true
		}
	}
	return false
}

func getFolderSize(folderPath string, showProgress bool, excludeList []string) int64 {
	var totalSize int64
	var fileCount int

	err := filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip the root directory itself
		if path == folderPath {
			return nil
		}

		// Get relative path from root
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return nil
		}

		// Check if this item should be excluded
		parts := strings.Split(relPath, string(os.PathSeparator))
		if len(parts) > 0 && shouldExclude(parts[0], excludeList) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			totalSize += info.Size()
			fileCount++
		}

		return nil
	})

	if err != nil {
		return 0
	}

	return totalSize
}

func getSizesOfSubfolders(parentFolder string, showProgress bool, excludeList []string) map[string]int64 {
	subfolderSizes := make(map[string]int64)

	entries, err := os.ReadDir(parentFolder)
	if err != nil {
		fmt.Printf("Error accessing %s: %v\n", parentFolder, err)
		return subfolderSizes
	}

	totalItems := len(entries)

	for i, entry := range entries {
		// Skip if this item should be excluded
		if shouldExclude(entry.Name(), excludeList) {
			continue
		}

		if showProgress {
			progressMsg := fmt.Sprintf("Processing %d/%d: %s", i+1, totalItems, entry.Name())
			terminalWidth := getTerminalWidth()

			// Truncate if too long
			if len(progressMsg) > terminalWidth-1 {
				progressMsg = progressMsg[:terminalWidth-4] + "..."
			}

			// Pad to terminal width
			paddedMsg := fmt.Sprintf("%-*s", terminalWidth-1, progressMsg)
			fmt.Printf("\r%s", paddedMsg)
		}

		entryPath := filepath.Join(parentFolder, entry.Name())

		if entry.IsDir() {
			subfolderSizes[entry.Name()] = getFolderSize(entryPath, showProgress, excludeList)
		} else {
			info, err := entry.Info()
			if err == nil {
				subfolderSizes[entry.Name()] = info.Size()
			}
		}
	}

	if showProgress {
		fmt.Println() // New line after progress
	}

	return subfolderSizes
}

func formatSize(size int64) sizeInfo {
	if size == 0 {
		return sizeInfo{0, "bytes", colorGreen}
	}

	units := []string{"bytes", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	sizeFloat := float64(size)

	for sizeFloat >= 1024 && unitIndex < len(units)-1 {
		sizeFloat /= 1024
		unitIndex++
	}

	// Color based on size: green for small, yellow for medium, red for large
	var colorCode string
	if unitIndex <= 1 { // bytes, KB
		colorCode = colorGreen
	} else if unitIndex <= 2 { // MB
		colorCode = colorYellow
	} else { // GB, TB
		colorCode = colorRed
	}

	return sizeInfo{sizeFloat, units[unitIndex], colorCode}
}

func printResults(subfolderSizes map[string]int64, parentFolder, sortBy string, reverse bool) {
	if len(subfolderSizes) == 0 {
		fmt.Println("No accessible folders or files found.")
		return
	}

	// Convert map to slice for sorting
	var items []folderItem
	for name, size := range subfolderSizes {
		items = append(items, folderItem{name, size})
	}

	// Sort results
	switch sortBy {
	case "size":
		sort.Slice(items, func(i, j int) bool {
			if reverse {
				return items[i].size > items[j].size
			}
			return items[i].size < items[j].size
		})
	case "name":
		sort.Slice(items, func(i, j int) bool {
			if reverse {
				return strings.ToLower(items[i].name) > strings.ToLower(items[j].name)
			}
			return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
		})
	}

	// Calculate total size
	var totalSize int64
	for _, item := range items {
		totalSize += item.size
	}

	totalFormatted := formatSize(totalSize)

	// Print header
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("📁 Parent Folder: %s\n", parentFolder)
	fmt.Printf("📊 Total Size: %.2f %s\n", totalFormatted.value, color(totalFormatted.unit, totalFormatted.color))
	fmt.Printf("📈 Items Found: %d\n", len(subfolderSizes))
	fmt.Println(strings.Repeat("=", 80))

	// Print table header
	fmt.Printf("%-15s %-10s %-50s\n", "Size", "Unit", "Name")
	fmt.Println(strings.Repeat("-", 80))

	// Print items
	for _, item := range items {
		sizeInfo := formatSize(item.size)
		sizeStr := fmt.Sprintf("%.2f", sizeInfo.value)
		unitStr := color(sizeInfo.unit, sizeInfo.color)

		// Truncate long names
		displayName := item.name
		if len(displayName) > 50 {
			displayName = displayName[:47] + "..."
		}

		fmt.Printf("%-15s %-10s %-50s\n", sizeStr, unitStr, displayName)
	}

	fmt.Println(strings.Repeat("-", 80))
}

func main() {
	var (
		path        = flag.String("path", ".", "Path to analyze (default: current directory)")
		sortBy      = flag.String("sort", "size", "Sort by size or name")
		asc         = flag.Bool("asc", false, "Sort in ascending order")
		progress    = flag.Bool("progress", false, "Show progress during calculation")
		noClear     = flag.Bool("no-clear", false, "Don't clear screen before output")
		excludeDirs = flag.String("exclude-dirs", "", "Comma-separated list of folders/files to exclude (e.g., node_modules,.git,target)")
	)
	flag.Parse()

	// Parse exclude list
	var excludeList []string
	if *excludeDirs != "" {
		for _, item := range strings.Split(*excludeDirs, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				excludeList = append(excludeList, trimmed)
			}
		}
	}

	// Clear screen unless disabled
	if !*noClear {
		fmt.Print("\033[H\033[2J") // Clear screen
	}

	// Validate path
	parentFolder, err := filepath.Abs(*path)
	if err != nil {
		fmt.Printf("❌ Error: Invalid path '%s': %v\n", *path, err)
		os.Exit(1)
	}

	if _, err := os.Stat(parentFolder); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Path '%s' does not exist!\n", parentFolder)
		os.Exit(1)
	}

	fmt.Printf("🔍 Analyzing: %s\n", parentFolder)
	if len(excludeList) > 0 {
		fmt.Printf("🚫 Excluding: %s\n", strings.Join(excludeList, ", "))
	}
	if *progress {
		fmt.Println("⏳ Calculating sizes (this may take a while for large directories)...")
	}

	startTime := time.Now()

	// Get folder sizes
	subfolderSizes := getSizesOfSubfolders(parentFolder, *progress, excludeList)

	endTime := time.Now()

	if *progress {
		fmt.Printf("\n✅ Analysis completed in %.2f seconds\n", endTime.Sub(startTime).Seconds())
	}

	// Print results
	printResults(subfolderSizes, parentFolder, *sortBy, !*asc)
}
