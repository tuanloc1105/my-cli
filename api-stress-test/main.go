package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RequestResult holds the result of a single HTTP request
type RequestResult struct {
	OK         bool
	StatusCode int
	Elapsed    float64
	Error      string
}

// parseHeaders parses headers from 'key1:value1,key2:value2' format
func parseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	if raw == "" {
		return headers
	}

	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		idx := strings.Index(part, ":")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if key != "" {
			headers[key] = value
		}
	}

	return headers
}

// parseData parses form data from 'key1=value1&key2=value2' format
func parseData(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}

	data := make(map[string]string)
	parts := strings.Split(raw, "&")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		idx := strings.Index(part, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if key != "" {
			data[key] = value
		}
	}

	if len(data) == 0 {
		return nil, nil
	}

	return data, nil
}

// prepareBody prepares the body bytes and content type based on flags
func prepareBody(
	jsonBody string, jsonFile string,
	formData map[string]string,
	rawBody string, rawFile string,
	contentTypeFlag string,
) ([]byte, string, error) {
	// 1. JSON (High priority, includes validation)
	if jsonFile != "" {
		data, err := os.ReadFile(jsonFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read JSON file: %w", err)
		}
		// Validate JSON
		if !json.Valid(data) {
			return nil, "", fmt.Errorf("invalid JSON in file")
		}
		return data, "application/json", nil
	}

	if jsonBody != "" {
		data := []byte(strings.TrimSpace(jsonBody))
		// Validate JSON
		if !json.Valid(data) {
			return nil, "", fmt.Errorf("invalid JSON string")
		}
		return data, "application/json", nil
	}

	// 2. Form Data
	if formData != nil {
		values := url.Values{}
		for k, v := range formData {
			values.Set(k, v)
		}
		return []byte(values.Encode()), "application/x-www-form-urlencoded", nil
	}

	// 3. Generic Body / Raw File
	if rawFile != "" {
		data, err := os.ReadFile(rawFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read file: %w", err)
		}
		ct := contentTypeFlag
		if ct == "" {
			ct = "text/plain" // default for raw file
		}
		return data, ct, nil
	}

	if rawBody != "" {
		ct := contentTypeFlag
		if ct == "" {
			ct = "text/plain" // default for raw string
		}
		return []byte(rawBody), ct, nil
	}

	return nil, "", nil
}

// doRequest executes a single HTTP request and returns metrics
func doRequest(
	ctx context.Context,
	client *http.Client,
	method, targetURL string,
	headers map[string]string,
	body []byte,
	contentType string,
) RequestResult {
	startedAt := time.Now()

	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), targetURL, reqBody)
	if err != nil {
		return RequestResult{
			OK:      false,
			Elapsed: time.Since(startedAt).Seconds(),
			Error:   fmt.Sprintf("failed to create request: %v", err),
		}
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Execute request
	resp, err := client.Do(req)
	elapsed := time.Since(startedAt).Seconds()

	if err != nil {
		return RequestResult{
			OK:      false,
			Elapsed: elapsed,
			Error:   err.Error(),
		}
	}
	defer resp.Body.Close()
	// Drain body to reuse connection
	io.Copy(io.Discard, resp.Body)

	statusCode := resp.StatusCode
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300

	return RequestResult{
		OK:         ok,
		StatusCode: statusCode,
		Elapsed:    elapsed,
		Error:      "",
	}
}

// runStressTest runs the stress test and prints summary statistics
func runStressTest(
	targetURL string,
	method string,
	totalRequests int,
	concurrency int,
	timeout time.Duration,
	headers map[string]string,
	body []byte,
	contentType string,
) {
	fmt.Printf("Target URL            : %s\n", targetURL)
	fmt.Printf("HTTP method           : %s\n", strings.ToUpper(method))
	fmt.Printf("Total requests        : %d\n", totalRequests)
	fmt.Printf("Concurrency (workers) : %d\n", concurrency)
	fmt.Printf("Timeout per request   : %.1f seconds\n", timeout.Seconds())
	if len(body) > 0 {
		fmt.Printf("Body size             : %d bytes\n", len(body))
		if contentType != "" {
			fmt.Printf("Content-Type          : %s\n", contentType)
		}
	}
	fmt.Println(strings.Repeat("-", 60))

	// Configure shared Transport
	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	// Setup Graceful Shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nStopping requests... (waiting for active workers to finish)")
		cancel()
	}()

	startTime := time.Now()

	var mu sync.Mutex
	successes := 0
	failures := 0
	latencies := make([]float64, 0, totalRequests)
	statusCount := make(map[int]int)

	// Worker pool pattern
	jobs := make(chan struct{}, totalRequests)
	results := make(chan RequestResult, totalRequests)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				// Stop if context cancelled
				if ctx.Err() != nil {
					return
				}
				results <- doRequest(ctx, client, method, targetURL, headers, body, contentType)
			}
		}()
	}

	// Feed jobs
	go func() {
		for i := 0; i < totalRequests; i++ {
			if ctx.Err() != nil {
				break
			}
			jobs <- struct{}{}
		}
		close(jobs)
	}()

	// Close results channel when workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	completed := 0
	for res := range results {
		completed++
		mu.Lock()
		latencies = append(latencies, res.Elapsed)
		if res.StatusCode != 0 {
			statusCount[res.StatusCode]++
		} else {
			statusCount[0]++
		}
		if res.OK {
			successes++
		} else {
			failures++
		}
		mu.Unlock()

		if completed%max(1, totalRequests/10) == 0 {
			fmt.Printf("Completed %d/%d requests...\n", completed, totalRequests)
		}
	}

	totalTime := time.Since(startTime).Seconds()

	if len(latencies) == 0 {
		fmt.Println("No requests were executed.")
		return
	}

	// Sort latencies for percentile calculation
	sort.Float64s(latencies)

	// Calculate statistics
	avgLatency := 0.0
	for _, l := range latencies {
		avgLatency += l
	}
	avgLatency /= float64(len(latencies))

	p50 := latencies[int(0.5*float64(len(latencies)))]
	p90 := latencies[int(0.9*float64(len(latencies)))]
	p99Idx := int(0.99 * float64(len(latencies)))
	if p99Idx >= len(latencies) {
		p99Idx = len(latencies) - 1
	}
	p99 := latencies[p99Idx]

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Stress test finished")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total time            : %.4f seconds\n", totalTime)
	fmt.Printf("Requests per second   : %.2f req/s\n", float64(completed)/totalTime) // Use completed count
	fmt.Printf("Successes             : %d\n", successes)
	fmt.Printf("Failures              : %d\n", failures)
	fmt.Println("Status codes          :")

	// Sort status codes
	var statusKeys []int
	for k := range statusCount {
		statusKeys = append(statusKeys, k)
	}
	sort.Ints(statusKeys)

	for _, status := range statusKeys {
		count := statusCount[status]
		label := "ERROR/NO STATUS"
		if status != 0 {
			label = fmt.Sprintf("%d", status)
		}
		fmt.Printf("  %-15s %d\n", label, count)
	}

	fmt.Println()
	fmt.Println("Latency (seconds)")
	fmt.Printf("  Average             : %.4f\n", avgLatency)
	fmt.Printf("  p50                 : %.4f\n", p50)
	fmt.Printf("  p90                 : %.4f\n", p90)
	fmt.Printf("  p99                 : %.4f\n", p99)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	var (
		targetURL   = flag.String("url", "", "Target URL of the API endpoint (required)")
		method      = flag.String("method", "GET", "HTTP method to use (GET, POST, PUT, DELETE, etc). Default is GET.")
		requests    = flag.Int("requests", 100, "Total number of requests to send. Default is 100.")
		concurrency = flag.Int("concurrency", 10, "Number of concurrent worker goroutines. Default is 10.")
		timeout     = flag.Float64("timeout", 5.0, "Timeout for each request in seconds. Default is 5.0.")
		headers     = flag.String("headers", "", "Optional request headers in 'key1:value1,key2:value2' format.")

		// Body flags
		data            = flag.String("data", "", "Optional form data in 'key1=value1&key2=value2' format.")
		jsonBody        = flag.String("json-body", "", "Optional JSON body as a string.")
		jsonFile        = flag.String("json-file", "", "Optional path to valid JSON file.")
		rawBody         = flag.String("body", "", "Optional raw body content as a string.")
		rawFile         = flag.String("file", "", "Optional path to any file to use as request body.")
		contentTypeFlag = flag.String("content-type", "", "Explicit Content-Type header (overrides default for --body/--file).")
	)

	flag.Parse()

	if *targetURL == "" {
		fmt.Fprintf(flag.CommandLine.Output(), "Error: --url is required\n")
		flag.Usage()
		return
	}

	parsedHeaders := parseHeaders(*headers)
	parsedData, err := parseData(*data)
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Error parsing --data: %v\n", err)
		return
	}

	// Count how many body sources are provided
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

	body, contentType, err := prepareBody(*jsonBody, *jsonFile, parsedData, *rawBody, *rawFile, *contentTypeFlag)
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Error preparing body: %v\n", err)
		return
	}

	if *requests <= 0 {
		*requests = 100
	}
	if *concurrency <= 0 {
		*concurrency = 10
	}

	runStressTest(
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
