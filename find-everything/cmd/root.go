package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"common-module/utils"
	"find-everything/internal/finder"
	"find-everything/internal/ui"

	"github.com/spf13/cobra"
)

func Execute() {
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

			fmt.Printf("%s%sEnhanced File and Directory Finder%s\n", ui.ColorBold, ui.ColorHeader, ui.ColorEndC)
			fmt.Printf("%sSearching in: %s%s\n", ui.ColorOKBlue, basePath, ui.ColorEndC)
			fmt.Printf("%sPattern: %s%s\n", ui.ColorOKBlue, pattern, ui.ColorEndC)

			options := finder.FinderOptions{
				CaseSensitive:   caseSensitive,
				MaxWorkers:      maxWorkers,
				ExcludeDirs:     processedExcludeDirs,
				ExcludePatterns: excludePatterns,
				FileTypes:       fileTypes,
				MinSize:         minSizeBytes,
				MaxSize:         maxSizeBytes,
				MaxResults:      maxResults,
				ShowProgress:    !noProgress,
			}

			f, err := finder.NewFileFinder(basePath, pattern, options)
			if err != nil {
				return err
			}

			files, dirs := f.FindFilesAndDirs()
			ui.PrintResults(files, dirs, showDetails, pattern, basePath)

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
		fmt.Printf("%sError: %v%s\n", ui.ColorFail, err, ui.ColorEndC)
		os.Exit(1)
	}
}

func parseSize(sizeStr string) (int64, error) {
	if strings.ToLower(sizeStr) == "inf" {
		return 1<<63 - 1, nil // Max int64
	}

	sizeStr = strings.ToUpper(sizeStr)

	// Ordered from longest suffix to shortest to avoid ambiguous matching
	// (e.g., "1KB" matching "B" before "KB")
	units := []struct {
		suffix     string
		multiplier int64
	}{
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}

	for _, u := range units {
		if strings.HasSuffix(sizeStr, u.suffix) {
			numStr := strings.TrimSuffix(sizeStr, u.suffix)
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, err
			}
			return int64(num * float64(u.multiplier)), nil
		}
	}

	// No unit specified, assume bytes
	return strconv.ParseInt(sizeStr, 10, 64)
}
