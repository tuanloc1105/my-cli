package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/term"
)

type WorkItem struct {
	Name  string
	Path  string
	IsDir bool
}

type WorkResult struct {
	Name string
	Size int64
}

// getTerminalWidth returns the width of the terminal
func getTerminalWidth() int {
	// Try to get actual terminal width
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	// Fallback to 80 columns if unable to detect
	return 80
}

// GetSizesOfSubfolders calculates sizes of immediate subfolders/files
func GetSizesOfSubfolders(parentFolder string, showProgress bool, excludeList []string) map[string]int64 {
	subfolderSizes := make(map[string]int64)

	entries, err := os.ReadDir(parentFolder)
	if err != nil {
		fmt.Printf("Error accessing %s: %v\n", parentFolder, err)
		return subfolderSizes
	}

	// Optimize excludes: Use a map for O(1) lookup
	excludeMap := make(map[string]struct{})
	for _, item := range excludeList {
		excludeMap[item] = struct{}{}
	}

	// Filter out excluded items
	var workItems []WorkItem
	for _, entry := range entries {
		if _, excluded := excludeMap[entry.Name()]; excluded {
			continue
		}
		workItems = append(workItems, WorkItem{
			Name:  entry.Name(),
			Path:  filepath.Join(parentFolder, entry.Name()),
			IsDir: entry.IsDir(),
		})
	}

	totalItems := len(workItems)
	if totalItems == 0 {
		return subfolderSizes
	}

	// Use worker pool for parallel processing
	numWorkers := runtime.NumCPU()
	if numWorkers > totalItems {
		numWorkers = totalItems
	}

	jobs := make(chan WorkItem, totalItems)
	results := make(chan WorkResult, totalItems)
	var wg sync.WaitGroup
	var processedCount int64

	// Start worker goroutines
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				var size int64
				if item.IsDir {
					size = getFolderSize(item.Path, excludeMap)
				} else {
					if info, err := os.Stat(item.Path); err == nil {
						size = info.Size()
					}
				}

				results <- WorkResult{Name: item.Name, Size: size}

				// Update progress
				if showProgress {
					count := atomic.AddInt64(&processedCount, 1)
					progressMsg := fmt.Sprintf("Processing %d/%d: %s", count, totalItems, item.Name)
					terminalWidth := getTerminalWidth()

					if len(progressMsg) > terminalWidth-1 {
						progressMsg = progressMsg[:terminalWidth-4] + "..."
					}

					paddedMsg := fmt.Sprintf("%-*s", terminalWidth-1, progressMsg)
					fmt.Printf("\r%s", paddedMsg)
				}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for _, item := range workItems {
			jobs <- item
		}
		close(jobs)
	}()

	// Collect results in a separate goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Gather results
	for result := range results {
		subfolderSizes[result.Name] = result.Size
	}

	if showProgress {
		fmt.Println() // New line after progress
	}

	return subfolderSizes
}

// getFolderSize recursively calculates folder size
func getFolderSize(folderPath string, excludeMap map[string]struct{}) int64 {
	totalSize := int64(0)

	err := filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip the root directory itself
		if path == folderPath {
			return nil
		}

		// Check if this file/dir name is excluded
		// optimization: check name directly against map
		if _, excluded := excludeMap[d.Name()]; excluded {
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
		}

		return nil
	})

	if err != nil {
		// Ignore errors, just return what we have
	}

	return totalSize
}
