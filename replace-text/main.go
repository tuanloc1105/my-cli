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

// processFile checks if a file is text and performs the replacement if it is.
// It reads the file only once for efficiency.
func processFile(filename, oldText, newText string, createBackup bool) error {
	// Read the entire file content
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Check if it's a valid UTF-8 text file
	if !utf8.Valid(content) {
		// Silently skip binary files (or log verbose if we had a verbose flag)
		return nil
	}

	contentStr := string(content)

	// If oldText is not in the file, there is nothing to do
	if !strings.Contains(contentStr, oldText) {
		return nil
	}

	var backupFilename string
	if createBackup {
		// Create a backup file by renaming the original
		backupFilename = filename + ".bak"

		// Remove existing backup if it exists
		os.Remove(backupFilename)

		// Rename original to backup
		if err := os.Rename(filename, backupFilename); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Perform the replacement
	newContent := strings.ReplaceAll(contentStr, oldText, newText)

	// Write the new content to the original filename
	if err := os.WriteFile(filename, []byte(newContent), 0644); err != nil {
		if createBackup {
			// Attempt to restore from backup on error
			if backupErr := os.Rename(backupFilename, filename); backupErr != nil {
				return fmt.Errorf("failed to write file and restore backup: %w (backup error: %v)", err, backupErr)
			}
		}
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Successfully replaced text in '%s'.\n", filename)
	return nil
}

// findAndReplace finds and replaces all occurrences of oldText with newText
// If 'path' is a file, it modifies that file.
// If 'path' is a directory, it recursively modifies all text files within it.
func findAndReplace(path, oldText, newText string, createBackup bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path '%s' not found or is not a valid file/directory: %w", path, err)
	}

	if info.IsDir() {
		fmt.Printf("Processing directory: %s\n", path)
		err := filepath.WalkDir(path, func(walkPath string, d fs.DirEntry, err error) error {
			if err != nil {
				// Log error but continue walking (unless it's the root path which is critical,
				// but usually WalkDir sends err for children)
				// If we can't access a directory, we should skip it.
				if d.IsDir() {
					fmt.Fprintf(os.Stderr, "Warning: Skipping directory '%s' due to error: %v\n", walkPath, err)
					return filepath.SkipDir
				}
				fmt.Fprintf(os.Stderr, "Warning: Skipping file '%s' due to error: %v\n", walkPath, err)
				return nil
			}

			if d.IsDir() {
				// Skip .git directories
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}

			// It's a file
			if err := processFile(walkPath, oldText, newText, createBackup); err != nil {
				fmt.Fprintf(os.Stderr, "Error processing '%s': %v\n", walkPath, err)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}
		fmt.Printf("\nFinished processing directory '%s'.\n", path)
		if createBackup {
			fmt.Println("Backup files (.bak) were created for all modified files.")
			fmt.Println("You can delete them if they are not needed.")
		}
	} else {
		// Single file processing
		if err := processFile(path, oldText, newText, createBackup); err != nil {
			return err
		}
		if createBackup {
			fmt.Printf("Backup file created at '%s.bak'.\n", path)
			fmt.Println("You can delete the backup file if it's not needed.")
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
	var createBackup bool

	var rootCmd = &cobra.Command{
		Use:   "replace-text [old-text] [new-text] [file-or-directory-path]",
		Short: "Find and replace text in files or directories",
		Long: `A tool to find and replace text in files or directories.
Supports both single files and recursive directory processing.
Optionally creates backup files (.bak) for all modified files with --backup flag.

Examples:
  replace-text 'hello' 'goodbye' /path/to/file.txt
  replace-text 'hello' 'goodbye' /path/to/your_folder
  replace-text 'hello' 'goodbye' /path/to/file.txt --backup
  replace-text '\\n' '\\r\\n' /path/to/file.txt  # Replace newlines with CRLF`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Un-escape the arguments (e.g. '\\n' becomes a newline character)
			oldText := unescapeString(args[0])
			newText := unescapeString(args[1])
			path := args[2]

			return findAndReplace(path, oldText, newText, createBackup)
		},
	}

	rootCmd.Flags().BoolVar(&createBackup, "backup", false, "Create backup files (.bak) before replacing")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
