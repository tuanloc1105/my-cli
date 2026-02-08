package finder

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"find-everything/internal/ui"
)

func (ff *FileFinder) FindFilesAndDirs() ([]string, []string) {
	if ff.showProgress {
		fmt.Printf("%sStarting search...%s\n", ui.ColorOKBlue, ui.ColorEndC)
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

	var matchedFiles []string
	var matchedDirs []string
	var resultsMu sync.Mutex

	// Use a channel for directories to process
	dirQueue := make(chan string, 10000)

	// WaitGroup to track active tasks (files/dirs being processed)
	var processingWg sync.WaitGroup
	var workerWg sync.WaitGroup

	// Atomic counters
	var totalDirs int64
	var skippedDirs int64

	// Start workers
	for i := 0; i < ff.maxWorkers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()

			localFiles := make([]string, 0, 100)
			localDirs := make([]string, 0, 100)

			// Helper to flush local results
			flush := func() {
				if len(localFiles) > 0 || len(localDirs) > 0 {
					resultsMu.Lock()
					matchedFiles = append(matchedFiles, localFiles...)
					matchedDirs = append(matchedDirs, localDirs...)
					newCount := len(matchedFiles) + len(matchedDirs)
					resultsMu.Unlock()

					// Check max results limit
					if newCount >= ff.maxResults {
						ff.cancel()
					}

					localFiles = localFiles[:0]
					localDirs = localDirs[:0]
				}
			}

			// Ensure final flush
			defer flush()

			for path := range dirQueue {
				processDir(ff, path, dirQueue, &processingWg, &localFiles, &localDirs, &totalDirs, &skippedDirs)

				// Flush periodically
				if len(localFiles)+len(localDirs) > 100 {
					flush()
				}

				// Task done
				processingWg.Done()
			}
		}()
	}

	// Initial seed
	atomic.AddInt64(&totalDirs, 1)
	ff.progressTracker.SetTotalDirs(1)
	processingWg.Add(1)
	dirQueue <- ff.basePath

	// Monitor completion
	go func() {
		processingWg.Wait()
		close(dirQueue)
	}()

	// Wait for all workers to finish
	workerWg.Wait()

	if ff.showProgress {
		fmt.Println() // New line after progress
	}

	if skipped := atomic.LoadInt64(&skippedDirs); skipped > 0 {
		fmt.Printf("%sWarning: %d directories could not be read (permission denied or other errors)%s\n",
			ui.ColorWarning, skipped, ui.ColorEndC)
	}

	return matchedFiles, matchedDirs
}

func processDir(ff *FileFinder, path string, dirQueue chan string, wg *sync.WaitGroup, localFiles *[]string, localDirs *[]string, totalDirs *int64, skippedDirs *int64) {
	entries, err := os.ReadDir(path)
	if err != nil {
		atomic.AddInt64(skippedDirs, 1)
		return
	}

	ff.progressTracker.UpdateProcessedDirs(1)

	for _, entry := range entries {
		entryName := entry.Name()
		fullPath := filepath.Join(path, entryName)

		if ff.ShouldExclude(fullPath) {
			continue
		}

		isDir := entry.IsDir()

		// Check for match
		if ff.MatchesPattern(entryName) {
			if isDir {
				*localDirs = append(*localDirs, fullPath)
				ff.progressTracker.Update(0, 1)
			} else {
				shouldAdd := true
				if !ff.CheckFileType(fullPath) {
					shouldAdd = false
				} else if ff.minSize > 0 || ff.maxSize < (1<<63-1) {
					if !ff.CheckFileSize(fullPath) {
						shouldAdd = false
					}
				}

				if shouldAdd {
					*localFiles = append(*localFiles, fullPath)
					ff.progressTracker.Update(1, 0)
				}
			}
		}

		// If directory, queue for traversal
		if isDir {
			select {
			case <-ff.ctx.Done():
				return
			default:
				atomic.AddInt64(totalDirs, 1)
				ff.progressTracker.SetTotalDirs(int(atomic.LoadInt64(totalDirs)))

				wg.Add(1)

				// Non-blocking send to prevent deadlock: all workers are both
				// producers and consumers of dirQueue. If channel is full and
				// all workers block on send, nobody consumes → deadlock.
				// Fallback goroutine keeps the worker free to continue consuming.
				select {
				case dirQueue <- fullPath:
				default:
					go func(p string) {
						dirQueue <- p
					}(fullPath)
				}
			}
		}
	}
}
