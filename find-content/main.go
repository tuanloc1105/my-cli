package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// FileSearcher handles file content searching operations
type FileSearcher struct {
	caseSensitive  bool
	fileExtensions map[string]bool
	excludeDirs    map[string]bool
	excludeFiles   map[string]bool
	textExtensions map[string]bool
}

// NewFileSearcher creates a new FileSearcher instance
func NewFileSearcher(caseSensitive bool, fileExtensions, excludeDirs, excludeFiles []string) *FileSearcher {
	fs := &FileSearcher{
		caseSensitive:  caseSensitive,
		fileExtensions: make(map[string]bool),
		excludeDirs:    make(map[string]bool),
		excludeFiles:   make(map[string]bool),
		textExtensions: make(map[string]bool),
	}

	// Set default excluded directories
	defaultExcludeDirs := []string{".git", "__pycache__", "node_modules", ".vscode", ".idea", "target", "build", "dist"}
	for _, dir := range defaultExcludeDirs {
		fs.excludeDirs[dir] = true
	}

	// Add custom excluded directories
	for _, dir := range excludeDirs {
		fs.excludeDirs[dir] = true
	}

	// Add custom excluded files
	for _, file := range excludeFiles {
		fs.excludeFiles[file] = true
	}

	// Add custom file extensions
	for _, ext := range fileExtensions {
		fs.fileExtensions[strings.ToLower(ext)] = true
	}

	// Common text file extensions
	textExts := []string{
		".txt", ".md", ".py", ".js", ".ts", ".html", ".css", ".scss", ".json", ".xml",
		".yaml", ".yml", ".ini", ".cfg", ".conf", ".sh", ".bash", ".sql", ".java",
		".cpp", ".c", ".h", ".hpp", ".cs", ".php", ".rb", ".go", ".rs", ".swift",
		".kt", ".scala", ".r", ".m", ".pl", ".lua", ".dart", ".vue", ".jsx", ".tsx", ".properties", ".log",
	}
	for _, ext := range textExts {
		fs.textExtensions[ext] = true
	}

	return fs
}

// isTextFile checks if a file is likely a text file
func (fs *FileSearcher) isTextFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Check explicit extensions first
	if len(fs.fileExtensions) > 0 && !fs.fileExtensions[ext] {
		return false
	}

	// Check if it's a known text extension
	return fs.textExtensions[ext]
}

// shouldSkipDirectory checks if directory should be skipped
func (fs *FileSearcher) shouldSkipDirectory(dirName string) bool {
	return fs.excludeDirs[dirName]
}

// shouldSkipFile checks if file should be skipped
func (fs *FileSearcher) shouldSkipFile(fileName string) bool {
	return fs.excludeFiles[fileName]
}

// searchInFile searches for keyword in a single file
func (fs *FileSearcher) searchInFile(filePath, keyword string, useRegex bool) []struct {
	lineNum int
	content string
} {
	var matches []struct {
		lineNum int
		content string
	}

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not read %s: %v\n", filePath, err)
		return matches
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 1

	for scanner.Scan() {
		line := scanner.Text()
		var matched bool

		if useRegex {
			flags := regexp.MustCompilePOSIX("")
			if !fs.caseSensitive {
				flags = regexp.MustCompilePOSIX("(?i)")
			}
			re, err := regexp.CompilePOSIX(flags.String() + keyword)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Invalid regex pattern: %v\n", err)
				return matches
			}
			matched = re.MatchString(line)
		} else {
			searchLine := line
			searchKeyword := keyword
			if !fs.caseSensitive {
				searchLine = strings.ToLower(line)
				searchKeyword = strings.ToLower(keyword)
			}
			matched = strings.Contains(searchLine, searchKeyword)
		}

		if matched {
			matches = append(matches, struct {
				lineNum int
				content string
			}{lineNum, line})
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error reading %s: %v\n", filePath, err)
	}

	return matches
}

// grepRecursive recursively searches for keyword in files
func (fs *FileSearcher) grepRecursive(rootDir, keyword string, useRegex bool, showLineNumbers, showFilePath bool, maxResults *int) int {
	info, err := os.Stat(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Directory does not exist: %s\n", rootDir)
		return 0
	}

	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: Path is not a directory: %s\n", rootDir)
		return 0
	}

	totalMatches := 0

	err = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		// Handle permission errors or other errors during walk
		if err != nil {
			if os.IsPermission(err) {
				fmt.Fprintf(os.Stderr, "Warning: Permission denied: %s\n", path)
				return nil // Skip this file/directory and continue
			}
			// For other errors, print warning and continue
			fmt.Fprintf(os.Stderr, "Warning: Error accessing %s: %v\n", path, err)
			return nil
		}

		if d.IsDir() {
			if fs.shouldSkipDirectory(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if fs.shouldSkipFile(d.Name()) {
			return nil
		}

		if !fs.isTextFile(path) {
			return nil
		}

		matches := fs.searchInFile(path, keyword, useRegex)

		for _, match := range matches {
			var outputParts []string

			if showFilePath {
				outputParts = append(outputParts, path)
			}

			if showLineNumbers {
				outputParts = append(outputParts, fmt.Sprintf("%d", match.lineNum))
			}

			outputParts = append(outputParts, match.content)

			fmt.Println(strings.Join(outputParts, ":"))
			totalMatches++

			if maxResults != nil && totalMatches >= *maxResults {
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during search: %v\n", err)
	}

	return totalMatches
}

// listDirectoryContents lists directory contents
func (fs *FileSearcher) listDirectoryContents(path string, showHidden bool) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	for _, entry := range entries {
		if !showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
		}

		sizeStr := ""
		if entryType == "file" {
			sizeStr = fmt.Sprintf(" (%d bytes)", info.Size())
		}

		fmt.Printf("%10s %s%s\n", entryType, entry.Name(), sizeStr)
	}

	return nil
}

func main() {
	var (
		useRegex      bool
		caseSensitive bool
		extensions    string
		excludeDirs   string
		excludeFiles  string
		noLineNumbers bool
		noFilePath    bool
		maxResults    int
		listMode      bool
		showHidden    bool
	)

	rootCmd := &cobra.Command{
		Use:   "find-content [directory] [keyword]",
		Short: "Improved file content search utility",
		Long: `A powerful file content search utility that supports recursive search with various options.

Examples:
  find-content /path/to/search "keyword"
  find-content /path/to/search "pattern" --regex
  find-content /path/to/search "text" --extensions py,js,txt
  find-content /path/to/search "version" --case-sensitive
  find-content /path/to/search "error" --exclude-dirs node_modules,.git`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			directory := args[0]
			keyword := args[1]

			// Parse comma-separated arguments
			var fileExtensions, excludeDirsList, excludeFilesList []string
			if extensions != "" {
				fileExtensions = strings.Split(extensions, ",")
			}
			if excludeDirs != "" {
				excludeDirsList = strings.Split(excludeDirs, ",")
			}
			if excludeFiles != "" {
				excludeFilesList = strings.Split(excludeFiles, ",")
			}

			searcher := NewFileSearcher(caseSensitive, fileExtensions, excludeDirsList, excludeFilesList)

			if listMode {
				if err := searcher.listDirectoryContents(directory, showHidden); err != nil {
					os.Exit(1)
				}
			} else {
				var maxResultsPtr *int
				if maxResults > 0 {
					maxResultsPtr = &maxResults
				}

				matches := searcher.grepRecursive(
					directory,
					keyword,
					useRegex,
					!noLineNumbers,
					!noFilePath,
					maxResultsPtr,
				)

				if matches == 0 {
					fmt.Println("No matches found")
				} else {
					fmt.Printf("\nFound %d match(es)\n", matches)
				}
			}
		},
	}

	// Add flags
	rootCmd.Flags().BoolVarP(&useRegex, "regex", "r", false, "Treat keyword as regex pattern")
	rootCmd.Flags().BoolVarP(&caseSensitive, "case-sensitive", "c", false, "Case sensitive search")
	rootCmd.Flags().StringVarP(&extensions, "extensions", "e", "", "Comma-separated list of file extensions to search")
	rootCmd.Flags().StringVar(&excludeDirs, "exclude-dirs", "", "Comma-separated list of directories to exclude")
	rootCmd.Flags().StringVar(&excludeFiles, "exclude-files", "", "Comma-separated list of files to exclude")
	rootCmd.Flags().BoolVar(&noLineNumbers, "no-line-numbers", false, "Hide line numbers in output")
	rootCmd.Flags().BoolVar(&noFilePath, "no-file-path", false, "Hide file paths in output")
	rootCmd.Flags().IntVarP(&maxResults, "max-results", "m", 0, "Maximum number of results to show")
	rootCmd.Flags().BoolVarP(&listMode, "list", "l", false, "List directory contents instead of searching")
	rootCmd.Flags().BoolVar(&showHidden, "show-hidden", false, "Show hidden files when listing")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
