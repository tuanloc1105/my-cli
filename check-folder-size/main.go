package main

import (
	"common-module/utils"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type SizeInfo struct {
	Name string
	Size int64
}

type FormatResult struct {
	Size  float64
	Unit  string
	Color int
}

func getTerminalWidth() int {
	// Simple fallback to 80 columns
	return 80
}

func color(msg string, bg int) string {
	return fmt.Sprintf("\033[%dm\033[1;30m %s \033[0m", bg, msg)
}

func shouldExclude(name string, excludeList []string) bool {
	if len(excludeList) == 0 {
		return false
	}

	for _, excludeItem := range excludeList {
		if name == strings.TrimSpace(excludeItem) {
			return true
		}
	}
	return false
}

func getFolderSize(folderPath string, excludeList []string) int64 {
	totalSize := int64(0)
	fileCount := 0

	err := filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip the root directory itself
		if path == folderPath {
			return nil
		}

		// Get relative path from the root
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return nil
		}

		// Check if this path should be excluded
		parts := strings.Split(relPath, string(os.PathSeparator))
		for _, part := range parts {
			if shouldExclude(part, excludeList) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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
		// Ignore errors, just return what we have
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
			// Create progress message
			progressMsg := fmt.Sprintf("Processing %d/%d: %s", i+1, totalItems, entry.Name())
			terminalWidth := getTerminalWidth()

			// Truncate if too long, then pad
			if len(progressMsg) > terminalWidth-1 {
				progressMsg = progressMsg[:terminalWidth-4] + "..."
			}

			// Pad to terminal width and clear any remnants
			paddedMsg := fmt.Sprintf("%-*s", terminalWidth-1, progressMsg)
			fmt.Printf("\r%s", paddedMsg)
		}

		if entry.IsDir() {
			fullPath := filepath.Join(parentFolder, entry.Name())
			subfolderSizes[entry.Name()] = getFolderSize(fullPath, excludeList)
		} else {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			subfolderSizes[entry.Name()] = info.Size()
		}
	}

	if showProgress {
		fmt.Println() // New line after progress
	}

	return subfolderSizes
}

func formatSize(size int64) FormatResult {
	if size == 0 {
		return FormatResult{0, "bytes", 42}
	}

	units := []string{"bytes", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	sizeFloat := float64(size)

	for sizeFloat >= 1024 && unitIndex < len(units)-1 {
		sizeFloat /= 1024
		unitIndex++
	}

	// Color based on size: green for small, yellow for medium, red for large
	var msgColor int
	if unitIndex <= 1 { // bytes, KB
		msgColor = 42 // green
	} else if unitIndex <= 2 { // MB
		msgColor = 43 // yellow
	} else { // GB, TB
		msgColor = 41 // red
	}

	return FormatResult{sizeFloat, units[unitIndex], msgColor}
}

func printResults(subfolderSizes map[string]int64, parentFolder, sortBy string, reverse bool) {
	if len(subfolderSizes) == 0 {
		fmt.Println("No accessible folders or files found.")
		return
	}

	// Convert map to slice for sorting
	var items []SizeInfo
	for name, size := range subfolderSizes {
		items = append(items, SizeInfo{name, size})
	}

	// Sort results
	switch sortBy {
	case "size":
		sort.Slice(items, func(i, j int) bool {
			if reverse {
				return items[i].Size > items[j].Size
			}
			return items[i].Size < items[j].Size
		})
	case "name":
		sort.Slice(items, func(i, j int) bool {
			if reverse {
				return strings.ToLower(items[i].Name) > strings.ToLower(items[j].Name)
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})
	}

	// Calculate total size
	var totalSize int64
	for _, item := range items {
		totalSize += item.Size
	}
	totalFormatted := formatSize(totalSize)

	// Print header
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("📁 Parent Folder: %s\n", parentFolder)
	fmt.Printf("📊 Total Size: %.2f %s\n", totalFormatted.Size, color(totalFormatted.Unit, totalFormatted.Color))
	fmt.Printf("📈 Items Found: %d\n", len(subfolderSizes))
	fmt.Printf("%s\n", strings.Repeat("=", 80))

	// Print table header
	fmt.Printf("%-15s %-10s %-50s\n", "Size", "Unit", "Name")
	fmt.Println(strings.Repeat("-", 80))

	// Print items
	for _, item := range items {
		formatted := formatSize(item.Size)
		sizeStr := fmt.Sprintf("%.2f", formatted.Size)
		unitStr := color(formatted.Unit, formatted.Color)

		// Truncate long names
		displayName := item.Name
		if len(displayName) > 50 {
			displayName = displayName[:47] + "..."
		}

		fmt.Printf("%-15s %-10s %-50s\n", sizeStr, unitStr, displayName)
	}

	fmt.Println(strings.Repeat("-", 80))
}

func main() {
	var (
		sortBy      string
		asc         bool
		progress    bool
		noClear     bool
		excludeDirs string
	)

	rootCmd := &cobra.Command{
		Use:   "check-folder-size [path]",
		Short: "Calculate folder sizes with improved features",
		Long:  `A tool to analyze folder sizes with progress tracking, exclusion lists, and colored output.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Parse exclude list
			var excludeList []string
			if excludeDirs != "" {
				excludeList = strings.Split(excludeDirs, ",")
				// Trim whitespace from each item
				for i, item := range excludeList {
					excludeList[i] = strings.TrimSpace(item)
				}
			}

			// Determine path to analyze
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			// Clear screen unless disabled
			if !noClear {
				utils.CLS() // Clear screen
			}

			// Validate path
			parentFolder, err := filepath.Abs(path)
			if err != nil {
				fmt.Printf("❌ Error: Invalid path '%s': %v\n", path, err)
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
			if progress {
				fmt.Println("⏳ Calculating sizes (this may take a while for large directories)...")
			}

			startTime := time.Now()

			// Get folder sizes
			subfolderSizes := getSizesOfSubfolders(parentFolder, progress, excludeList)

			endTime := time.Now()

			if progress {
				fmt.Printf("\n✅ Analysis completed in %.2f seconds\n", endTime.Sub(startTime).Seconds())
			}

			// Print results
			printResults(subfolderSizes, parentFolder, sortBy, !asc)
		},
	}

	rootCmd.Flags().StringVarP(&sortBy, "sort", "s", "size", "Sort by size or name")
	rootCmd.Flags().BoolVarP(&asc, "asc", "a", false, "Sort in ascending order")
	rootCmd.Flags().BoolVarP(&progress, "progress", "p", false, "Show progress during calculation")
	rootCmd.Flags().BoolVarP(&noClear, "no-clear", "n", false, "Don't clear screen before output")
	rootCmd.Flags().StringVarP(&excludeDirs, "exclude-dirs", "e", "", "Comma-separated list of folders/files to exclude (e.g., node_modules,.git,target)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}
