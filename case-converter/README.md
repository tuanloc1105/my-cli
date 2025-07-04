# Case Converter CLI

A command-line tool for text case conversion and transformation, written in Go using Cobra.

## Features

- Convert text to various case formats:
  - `normal`: lowercase text
  - `upper`: UPPERCASE TEXT
  - `lower`: lowercase text
  - `capitalized`: Capitalized text
  - `swapped`: SwApPeD cAsE
  - `snake_case`: snake_case
  - `kebab_case`: kebab-case
  - `camel_case`: camelCase
  - `pascal_case`: PascalCase
  - `constant_case`: CONSTANT_CASE
  - `title_case`: Title Case
  - `dot_case`: dot.case
  - `path_case`: path/case
  - `pascal_kebab`: Pascal-Kebab

- Support for reading from files
- Colored terminal output
- Automatic detection and conversion from various input formats

## Installation

1. Make sure you have Go installed (version 1.21 or later)
2. Navigate to the project directory
3. Install dependencies:
   ```bash
   go mod tidy
   ```
4. Build the application:
   ```bash
   go build -o case-converter
   ```

## Usage

### Basic Usage

```bash
# Convert text to various cases
./case-converter "hello world"

# Convert from file
./case-converter -f input.txt

# Show all case conversions
./case-converter "hello world" --all

# Output specific format only
./case-converter "hello world" --format snake
```

### Examples

```bash
# Input: "hello world"
./case-converter "hello world"

# Output:
# Original: hello world
# Normal: hello world
# Upper: HELLO WORLD
# Lower: hello world
# Capitalized: Hello world
# Swapped: hELLO wORLD
# Snake Case: hello_world
# Kebab Case: hello-world
# Camel Case: helloWorld
# Pascal Case: HelloWorld
# Constant Case: HELLO_WORLD
# Title Case: Hello World
# Dot Case: hello.world
# Path Case: hello/world
# Pascal Kebab: Hello-World
```

### Command Line Options

- `-f, --file`: Input file containing text to convert
- `--all`: Show all case conversions for each line
- `--format`: Specific format to output (normal, upper, lower, snake, kebab, camel, pascal, constant, title, dot, path)

### Supported Input Formats

The tool can automatically detect and convert from:
- `snake_case` → normal text
- `PascalCase` → normal text
- `camelCase` → normal text
- `kebab-case` → normal text

## Development

This tool is built using:
- **Go**: Programming language
- **Cobra**: CLI framework for Go
- **Unicode**: For proper character handling

## License

This project is open source and available under the MIT License. 
