# Check Folder Size CLI

A Go-based command-line tool to analyze folder sizes with colored output and progress tracking.

## Features

- 📊 Calculate sizes of all subfolders and files in a directory
- 🎨 Colored output based on size (green for small, yellow for medium, red for large)
- 📈 Progress tracking during calculation
- 🚫 Exclude specific folders/files from analysis
- 📋 Sort results by size or name (ascending/descending)
- 📱 Terminal-aware formatting
- ⚡ Fast and efficient Go implementation

## Installation

```bash
cd my-cli/check-folder-size
go build -o check-folder-size main.go
```

## Usage

### Basic Usage

```bash
# Analyze current directory
./check-folder-size

# Analyze specific directory
./check-folder-size -path /path/to/directory

# Analyze with progress tracking
./check-folder-size -progress

# Sort by name instead of size
./check-folder-size -sort name

# Sort in ascending order
./check-folder-size -asc
```

### Advanced Usage

```bash
# Exclude specific folders/files
./check-folder-size -exclude-dirs "node_modules,.git,target"

# Combine multiple options
./check-folder-size -path /home/user -progress -sort name -asc -exclude-dirs "node_modules,.git"

# Don't clear screen before output
./check-folder-size -no-clear
```

## Command Line Options

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-path` | string | `.` | Path to analyze (default: current directory) |
| `-sort` | string | `size` | Sort by `size` or `name` |
| `-asc` | bool | `false` | Sort in ascending order |
| `-progress` | bool | `false` | Show progress during calculation |
| `-no-clear` | bool | `false` | Don't clear screen before output |
| `-exclude-dirs` | string | `` | Comma-separated list of folders/files to exclude |

## Output Format

The tool displays results in a formatted table with:

- **Size**: File/folder size in appropriate units (bytes, KB, MB, GB, TB)
- **Unit**: Colored unit indicator
- **Name**: File/folder name (truncated if too long)

Colors indicate size ranges:
- 🟢 **Green**: Small files (bytes, KB)
- 🟡 **Yellow**: Medium files (MB)
- 🔴 **Red**: Large files (GB, TB)

## Examples

### Example Output

```
================================================================================
📁 Parent Folder: /home/user/projects
📊 Total Size: 2.45 GB 
📈 Items Found: 15
================================================================================
Size            Unit        Name
--------------------------------------------------------------------------------
1.23 GB         GB          node_modules
456.78 MB       MB          dist
123.45 MB       MB          build
67.89 MB        MB          .git
45.67 MB        MB          src
23.45 MB        MB          tests
12.34 MB        MB          docs
8.90 MB         MB          README.md
5.67 MB         MB          package.json
3.45 MB         MB          .gitignore
2.34 MB         MB          tsconfig.json
1.23 MB         MB          webpack.config.js
987.65 KB       KB          .eslintrc.js
456.78 KB       KB          .prettierrc
123.45 KB       KB          .env
--------------------------------------------------------------------------------
```

## Performance

The Go implementation provides:
- Fast file system traversal using `filepath.WalkDir`
- Efficient memory usage
- Progress tracking for large directories
- Graceful error handling for permission issues

## Comparison with Python Version

This Go CLI replicates the functionality of the Python script `python_check_folder_size.py` with:

✅ **Same features**: All core functionality preserved
✅ **Better performance**: Go's compiled nature provides faster execution
✅ **Smaller binary**: Single executable file
✅ **Cross-platform**: Works on Linux, macOS, and Windows
✅ **No dependencies**: Self-contained binary

## License

This project is part of the personal CLI tools collection. 
