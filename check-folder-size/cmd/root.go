package cmd

import (
	"check-folder-size/internal/scanner"
	"check-folder-size/internal/ui"
	"common-module/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	sortBy      string
	asc         bool
	progress    bool
	noClear     bool
	excludeDirs string
)

var RootCmd = &cobra.Command{
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
		subfolderSizes := scanner.GetSizesOfSubfolders(parentFolder, progress, excludeList)

		endTime := time.Now()

		if progress {
			fmt.Printf("\n✅ Analysis completed in %.2f seconds\n", endTime.Sub(startTime).Seconds())
		}

		// Print results
		ui.PrintResults(subfolderSizes, parentFolder, sortBy, !asc)
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.Flags().StringVarP(&sortBy, "sort", "s", "size", "Sort by size or name")
	RootCmd.Flags().BoolVarP(&asc, "asc", "a", false, "Sort in ascending order")
	RootCmd.Flags().BoolVarP(&progress, "progress", "p", false, "Show progress during calculation")
	RootCmd.Flags().BoolVarP(&noClear, "no-clear", "n", false, "Don't clear screen before output")
	RootCmd.Flags().StringVarP(&excludeDirs, "exclude-dirs", "e", "", "Comma-separated list of folders/files to exclude (e.g., node_modules,.git,target)")
}
