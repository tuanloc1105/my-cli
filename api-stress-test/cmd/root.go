// Package cmd provides the command-line interface and test execution logic
// for the API stress test tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"api-stress-test/internal/request"
	"api-stress-test/internal/stats"

	"github.com/spf13/cobra"
)

// validMethods defines accepted HTTP methods.
var validMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true,
}

// Execute sets up the Cobra root command and runs the CLI.
func Execute() {
	var (
		targetURL       string
		method          string
		requests        int
		concurrency     int
		timeout         float64
		headers         string
		data            string
		jsonBody        string
		jsonFile        string
		rawBody         string
		rawFile         string
		contentTypeFlag string
		rate            float64
		duration        string
		outputFormat    string
	)

	rootCmd := &cobra.Command{
		Use:   "api-stress-test",
		Short: "HTTP load/stress testing tool",
		Long:  "A CLI tool for HTTP load and stress testing with concurrent workers, latency percentiles, and detailed statistics.",
		Example: `  api-stress-test --url http://example.com/api --requests 1000 --concurrency 50
  api-stress-test --url http://example.com/api --method POST --json-body '{"key":"value"}'
  api-stress-test --url http://example.com/api --headers "Authorization:Bearer token;Accept:application/json"
  api-stress-test --url http://example.com/api --duration 30s --concurrency 20
  api-stress-test --url http://example.com/api --requests 500 --rate 50
  api-stress-test --url http://example.com/api --requests 100 --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate URL
			if err := ValidateURL(targetURL); err != nil {
				return err
			}

			// Validate HTTP method
			if err := ValidateMethod(method); err != nil {
				return err
			}

			// Validate output format
			if outputFormat != "text" && outputFormat != "json" {
				return fmt.Errorf("unsupported output format: %s (supported: text, json)", outputFormat)
			}

			// Parse headers
			parsedHeaders := request.ParseHeaders(headers)

			// Parse form data
			parsedData, err := request.ParseData(data)
			if err != nil {
				return fmt.Errorf("parsing --data: %w", err)
			}

			// Prepare body
			body, contentType, err := request.PrepareBody(jsonBody, jsonFile, parsedData, rawBody, rawFile, contentTypeFlag)
			if err != nil {
				return fmt.Errorf("preparing body: %w", err)
			}

			// Validate and set defaults
			if requests <= 0 {
				requests = 100
			}
			if concurrency <= 0 {
				concurrency = 10
			}

			// Parse duration if provided
			var dur time.Duration
			if duration != "" {
				dur, err = time.ParseDuration(duration)
				if err != nil {
					return fmt.Errorf("invalid duration: %w", err)
				}
			}

			return RunStressTest(
				targetURL,
				strings.ToUpper(method),
				requests,
				concurrency,
				time.Duration(timeout*float64(time.Second)),
				parsedHeaders,
				body,
				contentType,
				rate,
				dur,
				outputFormat,
			)
		},
	}

	// Required flag
	rootCmd.Flags().StringVar(&targetURL, "url", "", "Target URL (required)")
	_ = rootCmd.MarkFlagRequired("url")

	// Optional flags
	rootCmd.Flags().StringVar(&method, "method", "GET", "HTTP method (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)")
	rootCmd.Flags().IntVar(&requests, "requests", 100, "Total requests to send")
	rootCmd.Flags().IntVar(&concurrency, "concurrency", 10, "Number of concurrent workers")
	rootCmd.Flags().Float64Var(&timeout, "timeout", 5.0, "Timeout per request in seconds")
	rootCmd.Flags().StringVar(&headers, "headers", "", "Headers in 'key1:value1;key2:value2' format")
	rootCmd.Flags().StringVar(&data, "data", "", "Form data in 'key1=value1&key2=value2' format")
	rootCmd.Flags().StringVar(&jsonBody, "json-body", "", "JSON body string")
	rootCmd.Flags().StringVar(&jsonFile, "json-file", "", "Path to JSON file for body")
	rootCmd.Flags().StringVar(&rawBody, "body", "", "Raw body string")
	rootCmd.Flags().StringVar(&rawFile, "file", "", "Path to file for body")
	rootCmd.Flags().StringVar(&contentTypeFlag, "content-type", "", "Explicit Content-Type header")

	// New feature flags
	rootCmd.Flags().Float64Var(&rate, "rate", 0, "Max requests per second (0 = unlimited)")
	rootCmd.Flags().StringVar(&duration, "duration", "", "Test duration (e.g., 30s, 1m) instead of fixed request count")
	rootCmd.Flags().StringVar(&outputFormat, "output", "text", "Output format: text or json")

	// Mutual exclusivity
	rootCmd.MarkFlagsMutuallyExclusive("data", "json-body", "json-file", "body", "file")
	rootCmd.MarkFlagsMutuallyExclusive("requests", "duration")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// TestConfig holds the test configuration for JSON output.
type TestConfig struct {
	URL         string `json:"url"`
	Method      string `json:"method"`
	Requests    int    `json:"requests,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Concurrency int    `json:"concurrency"`
	Timeout     float64 `json:"timeout_seconds"`
	Rate        float64 `json:"rate,omitempty"`
}

// JSONOutput wraps the full result for JSON output format.
type JSONOutput struct {
	Config     TestConfig       `json:"config"`
	Statistics stats.Statistics  `json:"statistics"`
	TotalTime  float64          `json:"total_time_seconds"`
	ReqPerSec  float64          `json:"requests_per_second"`
}

// RunStressTest runs the HTTP stress test and returns an error if there are failures.
func RunStressTest(
	targetURL string,
	method string,
	totalRequests int,
	concurrency int,
	timeout time.Duration,
	headers map[string]string,
	body []byte,
	contentType string,
	rate float64,
	duration time.Duration,
	outputFormat string,
) error {
	isJSON := outputFormat == "json"
	isDurationMode := duration > 0

	if !isJSON {
		fmt.Printf("Target URL            : %s\n", targetURL)
		fmt.Printf("HTTP method           : %s\n", method)
		if isDurationMode {
			fmt.Printf("Duration              : %s\n", duration)
		} else {
			fmt.Printf("Total requests        : %d\n", totalRequests)
		}
		fmt.Printf("Concurrency (workers) : %d\n", concurrency)
		fmt.Printf("Timeout per request   : %.1f seconds\n", timeout.Seconds())
		if rate > 0 {
			fmt.Printf("Rate limit            : %.0f req/s\n", rate)
		}
		if len(body) > 0 {
			fmt.Printf("Body size             : %d bytes\n", len(body))
			if contentType != "" {
				fmt.Printf("Content-Type          : %s\n", contentType)
			}
		}
		fmt.Println(strings.Repeat("-", 60))
	}

	// Configure HTTP Transport
	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	// Setup context with graceful shutdown
	var ctx context.Context
	var cancel context.CancelFunc
	if isDurationMode {
		ctx, cancel = context.WithTimeout(context.Background(), duration)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if !isJSON {
			fmt.Println("\nStopping requests... (waiting for active workers to finish)")
		}
		cancel()
	}()

	startTime := time.Now()

	// Pre-allocate collector capacity
	initialCap := totalRequests
	if isDurationMode {
		initialCap = concurrency * 1000
	}
	collector := stats.NewCollector(initialCap)

	// Setup rate limiter
	limiter := request.NewRateLimiter(rate)
	defer limiter.Stop()

	// Worker pool
	jobs := make(chan struct{}, concurrency*2)
	results := make(chan request.Result, concurrency*2)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if ctx.Err() != nil {
					return
				}
				result := request.ExecuteRequest(ctx, client, method, targetURL, headers, body, contentType)
				results <- result
			}
		}()
	}

	// Feed jobs
	go func() {
		defer close(jobs)
		if isDurationMode {
			for {
				if !limiter.Wait(ctx) {
					return
				}
				select {
				case jobs <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		} else {
			for i := 0; i < totalRequests; i++ {
				if !limiter.Wait(ctx) {
					return
				}
				select {
				case jobs <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Close results when workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	completed := 0
	batchSize := max(1, concurrency/2)
	batch := make([]request.Result, 0, batchSize)
	progressInterval := max(1, totalRequests/10)

	for res := range results {
		batch = append(batch, res)
		completed++

		if len(batch) >= batchSize || (!isDurationMode && completed == totalRequests) {
			for _, result := range batch {
				collector.Record(result.StatusCode, result.Elapsed, result.OK, result.Error)
			}
			batch = batch[:0]

			if !isJSON && !isDurationMode && completed%progressInterval == 0 {
				fmt.Printf("Completed %d/%d requests...\n", completed, totalRequests)
			}
			if !isJSON && isDurationMode && completed%max(1, concurrency*10) == 0 {
				fmt.Printf("Completed %d requests (%.1fs elapsed)...\n", completed, time.Since(startTime).Seconds())
			}
		}
	}

	// Flush remaining batch
	for _, result := range batch {
		collector.Record(result.StatusCode, result.Elapsed, result.OK, result.Error)
	}

	totalTime := time.Since(startTime).Seconds()
	stat := collector.GetStatistics()

	if stat.Total == 0 {
		if !isJSON {
			fmt.Println("No requests were executed.")
		}
		return nil
	}

	reqPerSec := float64(stat.Total) / totalTime

	// JSON output
	if isJSON {
		output := JSONOutput{
			Config: TestConfig{
				URL:         targetURL,
				Method:      method,
				Concurrency: concurrency,
				Timeout:     timeout.Seconds(),
			},
			Statistics: stat,
			TotalTime:  totalTime,
			ReqPerSec:  reqPerSec,
		}
		if isDurationMode {
			output.Config.Duration = duration.String()
		} else {
			output.Config.Requests = totalRequests
		}
		if rate > 0 {
			output.Config.Rate = rate
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON output: %w", err)
		}
		fmt.Println(string(data))
	} else {
		// Text output
		fmt.Println()
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println("Stress test finished")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf("Total time            : %.4f seconds\n", totalTime)
		fmt.Printf("Requests per second   : %.2f req/s\n", reqPerSec)
		fmt.Printf("Successes             : %d\n", stat.Successes)
		fmt.Printf("Failures              : %d\n", stat.Failures)
		fmt.Println("Status codes          :")

		var statusKeys []int
		for k := range stat.StatusCount {
			statusKeys = append(statusKeys, k)
		}
		sort.Ints(statusKeys)

		for _, status := range statusKeys {
			count := stat.StatusCount[status]
			label := "ERROR/NO STATUS"
			if status != 0 {
				label = fmt.Sprintf("%d", status)
			}
			fmt.Printf("  %-15s %d\n", label, count)
		}

		fmt.Println()
		fmt.Println("Latency (seconds)")
		fmt.Printf("  Min                 : %.4f\n", stat.MinLatency)
		fmt.Printf("  Max                 : %.4f\n", stat.MaxLatency)
		fmt.Printf("  Average             : %.4f\n", stat.AvgLatency)
		fmt.Printf("  p50                 : %.4f\n", stat.P50Latency)
		fmt.Printf("  p90                 : %.4f\n", stat.P90Latency)
		fmt.Printf("  p99                 : %.4f\n", stat.P99Latency)

		if len(stat.TopErrors) > 0 {
			fmt.Println()
			fmt.Println("Top Errors            :")
			for _, e := range stat.TopErrors {
				fmt.Printf("  %-45s x %d\n", e.Message, e.Count)
			}
		}
	}

	if stat.Failures > 0 {
		return fmt.Errorf("%d out of %d requests failed", stat.Failures, stat.Total)
	}
	return nil
}

// ValidateURL validates that the URL is a valid HTTP/HTTPS URL.
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL is required")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must contain a host")
	}

	return nil
}

// ValidateMethod validates that the HTTP method is supported.
func ValidateMethod(method string) error {
	if !validMethods[strings.ToUpper(method)] {
		return fmt.Errorf("unsupported HTTP method: %s (supported: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)", method)
	}
	return nil
}
