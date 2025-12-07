package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
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

// color formats text with ANSI colors
func color(msg string, bg int) string {
	return fmt.Sprintf("\033[%dm\033[1;30m %s \033[0m", bg, msg)
}

// formatSize converts bytes to human readable format
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

// PrintResults displays the folder analysis results
func PrintResults(subfolderSizes map[string]int64, parentFolder, sortBy string, reverse bool) {
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

	// Initialize tabwriter for clean table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print table header
	fmt.Fprintln(w, "Size\tUnit\tName")
	fmt.Fprintln(w, "----\t----\t----")

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

		fmt.Fprintf(w, "%s\t%s\t%s\n", sizeStr, unitStr, displayName)
	}

	// Flush the buffer
	w.Flush()

	fmt.Println(strings.Repeat("-", 80))
}
