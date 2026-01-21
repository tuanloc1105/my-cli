package main

import (
	"flag"
	"fmt"
	"time"

	"api-stress-test/cmd"
	"api-stress-test/internal/request"
)

// main is the entry point for the API stress test CLI tool.
// It parses command-line flags, validates input, and runs the stress test.
func main() {
	// Define command-line flags
	var (
		targetURL   = flag.String("url", "", "Target URL of the API endpoint (required)")
		method      = flag.String("method", "GET", "HTTP method to use (GET, POST, PUT, DELETE, etc). Default is GET.")
		requests    = flag.Int("requests", 100, "Total number of requests to send. Default is 100.")
		concurrency = flag.Int("concurrency", 10, "Number of concurrent worker goroutines. Default is 10.")
		timeout     = flag.Float64("timeout", 5.0, "Timeout for each request in seconds. Default is 5.0.")
		headers     = flag.String("headers", "", "Optional request headers in 'key1:value1,key2:value2' format.")

		// Body-related flags (only one should be specified)
		data            = flag.String("data", "", "Optional form data in 'key1=value1&key2=value2' format.")
		jsonBody        = flag.String("json-body", "", "Optional JSON body as a string.")
		jsonFile        = flag.String("json-file", "", "Optional path to valid JSON file.")
		rawBody         = flag.String("body", "", "Optional raw body content as a string.")
		rawFile         = flag.String("file", "", "Optional path to any file to use as request body.")
		contentTypeFlag = flag.String("content-type", "", "Explicit Content-Type header (overrides default for --body/--file).")
	)

	flag.Parse()

	// Validate required URL parameter
	if *targetURL == "" {
		fmt.Fprintf(flag.CommandLine.Output(), "Error: --url is required\n")
		flag.Usage()
		return
	}

	if err := cmd.ValidateURL(*targetURL); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Error: %v\n", err)
		return
	}

	// Parse and validate headers
	parsedHeaders := request.ParseHeaders(*headers)

	// Parse and validate form data
	parsedData, err := request.ParseData(*data)
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Error parsing --data: %v\n", err)
		return
	}

	// Validate that only one body source is provided to avoid conflicts
	sources := 0
	if parsedData != nil {
		sources++
	}
	if *jsonBody != "" {
		sources++
	}
	if *jsonFile != "" {
		sources++
	}
	if *rawBody != "" {
		sources++
	}
	if *rawFile != "" {
		sources++
	}

	if sources > 1 {
		fmt.Fprintf(flag.CommandLine.Output(), "Error: Please specify only one body source (--data, --json-body, --json-file, --body, or --file).\n")
		return
	}

	// Prepare body
	body, contentType, err := request.PrepareBody(*jsonBody, *jsonFile, parsedData, *rawBody, *rawFile, *contentTypeFlag)
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Error preparing body: %v\n", err)
		return
	}

	// Validate and set defaults for requests and concurrency
	if *requests <= 0 {
		*requests = 100
	}
	if *concurrency <= 0 {
		*concurrency = 10
	}

	// Run stress test
	cmd.RunStressTest(
		*targetURL,
		*method,
		*requests,
		*concurrency,
		time.Duration(*timeout*float64(time.Second)),
		parsedHeaders,
		body,
		contentType,
	)
}
