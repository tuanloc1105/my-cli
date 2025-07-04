package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

// CaseConverter contains all text transformation methods
type CaseConverter struct{}

// RemoveNonAlpha removes non-alphabetic characters from a string, keeping whitespace and alphanumeric
func (cc *CaseConverter) RemoveNonAlpha(s string) string {
	var result strings.Builder
	for _, char := range s {
		if unicode.IsLetter(char) || unicode.IsSpace(char) || unicode.IsNumber(char) {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// ToSnakeCase converts string to snake_case
func (cc *CaseConverter) ToSnakeCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}

// ToPascalCase converts string to PascalCase
func (cc *CaseConverter) ToPascalCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

// ToKebabCase converts string to kebab-case
func (cc *CaseConverter) ToKebabCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "-"))
}

// ToConstantCase converts string to CONSTANT_CASE
func (cc *CaseConverter) ToConstantCase(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, " ", "_"))
}

// ToPathCase converts string to path/case
func (cc *CaseConverter) ToPathCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "/"))
}

// ToCamelCase converts string to camelCase
func (cc *CaseConverter) ToCamelCase(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		if len(words[i]) > 0 {
			result += strings.ToUpper(words[i][:1]) + strings.ToLower(words[i][1:])
		}
	}
	return result
}

// ToTitleCase converts string to Title Case
func (cc *CaseConverter) ToTitleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// ToDotCase converts string to dot.case
func (cc *CaseConverter) ToDotCase(s string) string {
	return strings.Join(strings.Fields(s), ".")
}

// FromSnakeCase converts snake_case to normal text
func (cc *CaseConverter) FromSnakeCase(s string) string {
	words := strings.Split(s, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// FromPascalCase converts PascalCase to normal text
func (cc *CaseConverter) FromPascalCase(s string) string {
	re := regexp.MustCompile(`(?m)(?<!^)(?=[A-Z])`)
	return re.ReplaceAllString(s, " ")
}

// FromCamelCase converts camelCase to normal text
func (cc *CaseConverter) FromCamelCase(s string) string {
	re := regexp.MustCompile(`(?m)(?<!^)(?=[A-Z])`)
	return re.ReplaceAllString(s, " ")
}

// FromKebabCase converts kebab-case to normal text
func (cc *CaseConverter) FromKebabCase(s string) string {
	words := strings.Split(s, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// ColorOutput provides colored terminal output
type ColorOutput struct{}

// Green returns green colored text
func (co *ColorOutput) Green(msg string) string {
	return fmt.Sprintf("\033[42m\033[1;30m %s \033[0m", msg)
}

// Blue returns blue colored text
func (co *ColorOutput) Blue(msg string) string {
	return fmt.Sprintf("\033[44m\033[1;30m %s \033[0m", msg)
}

// ProcessCaseConversions processes text and returns all case conversions
func ProcessCaseConversions(text string) map[string]string {
	cc := &CaseConverter{}

	// Handle potential all-caps input by checking for spaces
	var normalized string
	if strings.Contains(text, " ") {
		normalized = text
	} else {
		// If no spaces, then apply from_pascal_case and others
		normalized = text
		normalized = cc.FromCamelCase(normalized)
		normalized = cc.FromSnakeCase(normalized)
		normalized = cc.FromKebabCase(normalized)
		normalized = cc.FromPascalCase(normalized)
	}

	// Clean up the text
	words := strings.Fields(strings.TrimSpace(normalized))
	cleanText := cc.RemoveNonAlpha(strings.Join(words, " "))
	cleanText = strings.ToLower(cleanText)

	return map[string]string{
		"normal":        cleanText,
		"upper":         strings.ToUpper(cleanText),
		"lower":         strings.ToLower(cleanText),
		"capitalized":   strings.ToUpper(cleanText[:1]) + strings.ToLower(cleanText[1:]),
		"swapped":       swapCase(cleanText),
		"snake_case":    cc.ToSnakeCase(cleanText),
		"kebab_case":    cc.ToKebabCase(cleanText),
		"camel_case":    cc.ToCamelCase(cleanText),
		"pascal_case":   cc.ToPascalCase(cleanText),
		"constant_case": cc.ToConstantCase(cleanText),
		"title_case":    cc.ToTitleCase(cleanText),
		"dot_case":      cc.ToDotCase(cleanText),
		"path_case":     cc.ToPathCase(cleanText),
		"pascal_kebab":  strings.ReplaceAll(cc.ToTitleCase(cleanText), " ", "-"),
	}
}

// swapCase swaps the case of each character
func swapCase(s string) string {
	var result strings.Builder
	for _, char := range s {
		if unicode.IsUpper(char) {
			result.WriteRune(unicode.ToLower(char))
		} else if unicode.IsLower(char) {
			result.WriteRune(unicode.ToUpper(char))
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// PrintConversions prints all case conversions for a given line
func PrintConversions(line string) {
	co := &ColorOutput{}
	fmt.Printf("\n%s: %s\n", co.Blue("Original"), line)
	conversions := ProcessCaseConversions(line)
	for formatName, converted := range conversions {
		displayName := strings.ReplaceAll(formatName, "_", " ")
		displayName = strings.Title(displayName)
		fmt.Printf("%s: %s\n", co.Green(displayName), converted)
	}
}

var (
	text   string
	file   string
	all    bool
	format string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "case-converter",
		Short: "Case Converter CLI Tool - A text case conversion utility",
		Long: `Case Converter CLI Tool - A command-line tool for text case conversion and transformation.

Examples:
  # Convert text to various cases
  case-converter "hello world"

  # Convert from file
  case-converter -f input.txt

  # Show all case conversions
  case-converter "hello world" --all

  # Output specific format only
  case-converter "hello world" --format snake`,
		Run: func(cmd *cobra.Command, args []string) {
			// Clear screen
			fmt.Print("\033[H\033[2J")

			var inputText string
			if file != "" {
				content, err := os.ReadFile(file)
				if err != nil {
					fmt.Printf("Error reading file: %v\n", err)
					os.Exit(1)
				}
				inputText = string(content)
			} else if len(args) > 0 {
				inputText = args[0]
			} else {
				cmd.Help()
				return
			}

			// Split by lines if multiple lines
			lines := strings.Split(strings.TrimSpace(inputText), "\n")

			if format != "" {
				// Output specific format
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						conversions := ProcessCaseConversions(line)
						if result, exists := conversions[format]; exists {
							fmt.Println(result)
						} else {
							fmt.Println(line)
						}
					}
				}
			} else if all {
				// Output all formats
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						PrintConversions(line)
					}
				}
			} else {
				// Default: show all formats for first line
				if len(lines) > 0 {
					line := strings.TrimSpace(lines[0])
					if line != "" {
						PrintConversions(line)
					}
				}
			}
		},
	}

	rootCmd.Flags().StringVarP(&file, "file", "f", "", "Input file containing text to convert")
	rootCmd.Flags().BoolVar(&all, "all", false, "Show all case conversions")
	rootCmd.Flags().StringVar(&format, "format", "", "Specific format to output (normal, upper, lower, snake, kebab, camel, pascal, constant, title, dot, path)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
