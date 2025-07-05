package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// isTextFile checks if a file is likely a text file by trying to read it with UTF-8 encoding
func isTextFile(filepath string) bool {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return false
	}
	return utf8.Valid(data)
}

// replaceInFile performs the find-and-replace operation on a single file, supporting multi-line replacements
func replaceInFile(filename, oldText, newText string) error {
	// Read the entire file content to handle multi-line strings
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	contentStr := string(content)

	// If oldText is not in the file, there is nothing to do
	if !strings.Contains(contentStr, oldText) {
		return nil
	}

	// Create a backup file by renaming the original
	backupFilename := filename + ".bak"

	// Remove existing backup if it exists
	os.Remove(backupFilename)

	// Rename original to backup
	if err := os.Rename(filename, backupFilename); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Perform the replacement
	newContent := strings.ReplaceAll(contentStr, oldText, newText)

	// Write the new content to the original filename
	if err := os.WriteFile(filename, []byte(newContent), 0644); err != nil {
		// Attempt to restore from backup on error
		if backupErr := os.Rename(backupFilename, filename); backupErr != nil {
			return fmt.Errorf("failed to write file and restore backup: %w (backup error: %v)", err, backupErr)
		}
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Successfully replaced text in '%s'.\n", filename)
	return nil
}

// findAndReplace finds and replaces all occurrences of oldText with newText
// If 'path' is a file, it modifies that file.
// If 'path' is a directory, it recursively modifies all text files within it.
func findAndReplace(path, oldText, newText string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path '%s' not found or is not a valid file/directory: %w", path, err)
	}

	if info.IsDir() {
		fmt.Printf("Processing directory: %s\n", path)
		err := filepath.WalkDir(path, func(filepath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && isTextFile(filepath) {
				if replaceErr := replaceInFile(filepath, oldText, newText); replaceErr != nil {
					fmt.Fprintf(os.Stderr, "Error processing '%s': %v\n", filepath, replaceErr)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}
		fmt.Printf("\nFinished processing directory '%s'.\n", path)
		fmt.Println("Backup files (.bak) were created for all modified files.")
		fmt.Println("You can delete them if they are not needed.")
	} else {
		if isTextFile(path) {
			if err := replaceInFile(path, oldText, newText); err != nil {
				return err
			}
			fmt.Printf("Backup file created at '%s.bak'.\n", path)
			fmt.Println("You can delete the backup file if it's not needed.")
		} else {
			fmt.Fprintf(os.Stderr, "Skipping binary file: %s\n", path)
		}
	}

	return nil
}

// unescapeString converts escaped sequences like \\n to actual characters
func unescapeString(s string) string {
	// Handle common escape sequences
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "replace-text [old-text] [new-text] [file-or-directory-path]",
		Short: "Find and replace text in files or directories",
		Long: `A tool to find and replace text in files or directories.
Supports both single files and recursive directory processing.
Creates backup files (.bak) for all modified files.

Examples:
  replace-text 'hello' 'goodbye' /path/to/file.txt
  replace-text 'hello' 'goodbye' /path/to/your_folder
  replace-text '\\n' '\\r\\n' /path/to/file.txt  # Replace newlines with CRLF`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Un-escape the arguments (e.g. '\\n' becomes a newline character)
			oldText := unescapeString(args[0])
			newText := unescapeString(args[1])
			path := args[2]

			return findAndReplace(path, oldText, newText)
		},
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
