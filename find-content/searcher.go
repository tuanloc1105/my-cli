package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileSearcher handles file content searching operations
type FileSearcher struct {
	caseSensitive    bool
	fileExtensions   map[string]bool
	excludeDirs      map[string]bool
	excludeFiles     map[string]bool
	textExtensions   map[string]bool
	suppressWarnings bool
}

// NewFileSearcher creates a new FileSearcher instance
func NewFileSearcher(caseSensitive, suppressWarnings bool, fileExtensions, excludeDirs, excludeFiles []string) *FileSearcher {
	fs := &FileSearcher{
		caseSensitive:    caseSensitive,
		suppressWarnings: suppressWarnings,
		fileExtensions:   make(map[string]bool),
		excludeDirs:      make(map[string]bool),
		excludeFiles:     make(map[string]bool),
		textExtensions:   make(map[string]bool),
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
func (fs *FileSearcher) searchInFile(filePath, keyword string, useRegex, multiline bool) []struct {
	lineNum int
	endLine int
	content string
} {
	var matches []struct {
		lineNum int
		endLine int
		content string
	}

	file, err := os.Open(filePath)
	if err != nil {
		if !fs.suppressWarnings {
			fmt.Fprintf(os.Stderr, "Warning: Could not read %s: %v\n", filePath, err)
		}
		return matches
	}
	defer file.Close()

	if multiline {
		return fs.searchInFileMultiline(filePath, file, keyword, useRegex)
	}

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
				if !fs.suppressWarnings {
					fmt.Fprintf(os.Stderr, "Warning: Invalid regex pattern: %v\n", err)
				}
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
				endLine int
				content string
			}{lineNum, lineNum, line})
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		if !fs.suppressWarnings {
			fmt.Fprintf(os.Stderr, "Warning: Error reading %s: %v\n", filePath, err)
		}
	}

	return matches
}

// searchInFileMultiline searches for multiline keyword in a single file
func (fs *FileSearcher) searchInFileMultiline(filePath string, file *os.File, keyword string, useRegex bool) []struct {
	lineNum int
	endLine int
	content string
} {
	var matches []struct {
		lineNum int
		endLine int
		content string
	}

	// Convert escaped newlines to actual newlines
	searchPattern := strings.ReplaceAll(keyword, "\\n", "\n")

	// Read file content and normalize line endings (Windows \r\n -> Unix \n)
	contentBytes, err := io.ReadAll(file)
	if err != nil {
		if !fs.suppressWarnings {
			fmt.Fprintf(os.Stderr, "Warning: Could not read %s: %v\n", filePath, err)
		}
		return matches
	}

	// Normalize Windows line endings to Unix line endings for consistent searching
	content := strings.ReplaceAll(string(contentBytes), "\r\n", "\n")

	var searchContent string
	var searchPatternLower string

	if !fs.caseSensitive {
		searchContent = strings.ToLower(content)
		searchPatternLower = strings.ToLower(searchPattern)
	} else {
		searchContent = content
		searchPatternLower = searchPattern
	}

	var foundPositions []struct {
		start int
		end   int
	}

	if useRegex {
		flags := ""
		if !fs.caseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + searchPattern)
		if err != nil {
			if !fs.suppressWarnings {
				fmt.Fprintf(os.Stderr, "Warning: Invalid regex pattern: %v\n", err)
			}
			return matches
		}
		matchesRegex := re.FindAllStringIndex(content, -1)
		for _, match := range matchesRegex {
			foundPositions = append(foundPositions, struct {
				start int
				end   int
			}{match[0], match[1]})
		}
	} else {
		idx := strings.Index(searchContent, searchPatternLower)
		patternLen := len(searchPatternLower)
		for idx != -1 {
			foundPositions = append(foundPositions, struct {
				start int
				end   int
			}{idx, idx + patternLen})
			if idx+patternLen >= len(searchContent) {
				break
			}
			nextIdx := strings.Index(searchContent[idx+patternLen:], searchPatternLower)
			if nextIdx == -1 {
				break
			}
			idx = idx + patternLen + nextIdx
		}
	}

	// Convert character positions to line numbers and build output
	for _, pos := range foundPositions {
		startLineNum := strings.Count(content[:pos.start], "\n") + 1
		endLineNum := strings.Count(content[:pos.end], "\n") + 1

		// Get the matched content and convert newlines to \n for display
		matchedContent := strings.ReplaceAll(content[pos.start:pos.end], "\n", "\\n")

		matches = append(matches, struct {
			lineNum int
			endLine int
			content string
		}{startLineNum, endLineNum, matchedContent})
	}

	return matches
}

// grepRecursive recursively searches for keyword in files
func (fs *FileSearcher) grepRecursive(rootDir, keyword string, useRegex, multiline bool, showLineNumbers, showFilePath bool, maxResults *int) int {
	info, err := os.Stat(rootDir)
	if err != nil {
		if !fs.suppressWarnings {
			fmt.Fprintf(os.Stderr, "Error: Directory does not exist: %s\n", rootDir)
		}
		return 0
	}

	if !info.IsDir() {
		if !fs.suppressWarnings {
			fmt.Fprintf(os.Stderr, "Error: Path is not a directory: %s\n", rootDir)
		}
		return 0
	}

	totalMatches := 0

	err = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		// Handle permission errors or other errors during walk
		if err != nil {
			if os.IsPermission(err) {
				if !fs.suppressWarnings {
					fmt.Fprintf(os.Stderr, "Warning: Permission denied: %s\n", path)
				}
				return nil // Skip this file/directory and continue
			}
			// For other errors, print warning and continue
			if !fs.suppressWarnings {
				fmt.Fprintf(os.Stderr, "Warning: Error accessing %s: %v\n", path, err)
			}
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

		matches := fs.searchInFile(path, keyword, useRegex, multiline)

		for _, match := range matches {
			var outputParts []string

			if showFilePath {
				outputParts = append(outputParts, path)
			}

			if showLineNumbers {
				if multiline && match.lineNum != match.endLine {
					outputParts = append(outputParts, fmt.Sprintf("%d..%d", match.lineNum, match.endLine))
				} else {
					outputParts = append(outputParts, fmt.Sprintf("%d", match.lineNum))
				}
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
