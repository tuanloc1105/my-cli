#!/bin/bash

echo "Building Case Converter CLI..."

# Clean previous build
rm -f case-converter

# Download dependencies
go mod tidy

# Build the application
go build -o case-converter .

if [ $? -eq 0 ]; then
    echo "Build successful! You can now run:"
    echo "  ./case-converter --help"
    echo "  ./case-converter \"hello world\""
else
    echo "Build failed!"
    exit 1
fi 
