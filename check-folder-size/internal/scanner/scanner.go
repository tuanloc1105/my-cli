package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

type ScanOptions struct {
	ShowProgress bool
	ExcludeList  []string
	Ctx          context.Context
	MaxDepth     int // 0 = unlimited
}

type ScanResult struct {
	Sizes        map[string]int64
	WarningCount int64
}

// getTerminalWidth returns the width of the terminal
func getTerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return 80
}

// GetSizesOfSubfolders calculates sizes of immediate subfolders/files
func GetSizesOfSubfolders(parentFolder string, opts ScanOptions) ScanResult {
	subfolderSizes := make(map[string]int64)
	var warningCount int64

	entries, err := os.ReadDir(parentFolder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", parentFolder, err)
		return ScanResult{Sizes: subfolderSizes, WarningCount: 1}
	}

	// Optimize excludes: Use a map for O(1) lookup
	excludeMap := make(map[string]struct{})
	for _, item := range opts.ExcludeList {
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
		return ScanResult{Sizes: subfolderSizes}
	}

	// Use worker pool for parallel processing
	numWorkers := runtime.NumCPU()
	if numWorkers > totalItems {
		numWorkers = totalItems
	}

	bufSize := numWorkers * 2
	jobs := make(chan WorkItem, bufSize)
	results := make(chan WorkResult, bufSize)
	var wg sync.WaitGroup
	var processedCount int64
	var progressMu sync.Mutex

	// Start worker goroutines
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				// Check for cancellation
				select {
				case <-opts.Ctx.Done():
					return
				default:
				}

				var size int64
				if item.IsDir {
					size = getFolderSize(item.Path, excludeMap, opts.Ctx, opts.MaxDepth, &warningCount)
				} else {
					if info, err := os.Stat(item.Path); err == nil {
						size = info.Size()
					} else {
						atomic.AddInt64(&warningCount, 1)
					}
				}

				select {
				case results <- WorkResult{Name: item.Name, Size: size}:
				case <-opts.Ctx.Done():
					return
				}

				// Update progress
				if opts.ShowProgress {
					count := atomic.AddInt64(&processedCount, 1)
					progressMsg := fmt.Sprintf("Processing %d/%d: %s", count, totalItems, item.Name)
					terminalWidth := getTerminalWidth()

					runes := []rune(progressMsg)
					if len(runes) > terminalWidth-1 {
						progressMsg = string(runes[:terminalWidth-4]) + "..."
					}

					paddedMsg := fmt.Sprintf("%-*s", terminalWidth-1, progressMsg)
					progressMu.Lock()
					fmt.Printf("\r%s", paddedMsg)
					progressMu.Unlock()
				}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for _, item := range workItems {
			select {
			case jobs <- item:
			case <-opts.Ctx.Done():
				break
			}
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

	if opts.ShowProgress {
		fmt.Println()
	}

	if opts.Ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "\nScan cancelled: %v (partial results returned)\n", opts.Ctx.Err())
	}

	return ScanResult{
		Sizes:        subfolderSizes,
		WarningCount: atomic.LoadInt64(&warningCount),
	}
}

// getFolderSize recursively calculates folder size
func getFolderSize(folderPath string, excludeMap map[string]struct{}, ctx context.Context, maxDepth int, warningCount *int64) int64 {
	totalSize := int64(0)
	baseDepth := strings.Count(folderPath, string(os.PathSeparator))

	err := filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			atomic.AddInt64(warningCount, 1)
			return nil
		}

		if path == folderPath {
			return nil
		}

		// Skip symlinks to avoid potential loops
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		// Depth limit check
		if maxDepth > 0 && d.IsDir() {
			currentDepth := strings.Count(path, string(os.PathSeparator)) - baseDepth
			if currentDepth > maxDepth {
				return filepath.SkipDir
			}
		}

		// Exclusion check
		if _, excluded := excludeMap[d.Name()]; excluded {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				atomic.AddInt64(warningCount, 1)
				return nil
			}
			totalSize += info.Size()
		}

		return nil
	})

	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		atomic.AddInt64(warningCount, 1)
	}

	return totalSize
}
