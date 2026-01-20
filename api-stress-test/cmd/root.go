package cmd

import (
	"context"
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
)

// RunStressTest runs the HTTP stress test and prints summary statistics.
// It sets up a worker pool to execute concurrent requests, collects results,
// and calculates comprehensive statistics including latency percentiles.
// Supports graceful shutdown via Ctrl+C (SIGINT/SIGTERM).
func RunStressTest(
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

	// Configure HTTP Transport for connection reuse and performance optimization
	// MaxIdleConns and MaxIdleConnsPerHost are set to concurrency level to match worker pool size
	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	// Setup graceful shutdown: listen for interrupt signals and cancel context
	// This allows workers to finish current requests before stopping
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

	// Create statistics collector with pre-allocated capacity
	collector := stats.NewCollector(totalRequests)

	// Worker pool pattern: use buffered channels for better throughput
	// Jobs channel: sends work items to workers
	// Results channel: receives completed request results (buffered to reduce blocking)
	jobs := make(chan struct{}, totalRequests)
	results := make(chan request.Result, concurrency*2) // Buffer size = 2x workers for better throughput
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
				result := request.ExecuteRequest(ctx, client, method, targetURL, headers, body, contentType)
				results <- result
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

	// Process results with batching to reduce mutex contention in the statistics collector
	// Batching multiple results together reduces the number of lock acquisitions
	completed := 0
	batchSize := max(1, concurrency/2) // Batch size proportional to concurrency
	batch := make([]request.Result, 0, batchSize)

	for res := range results {
		batch = append(batch, res)
		completed++

		// Process batch when full or last result
		if len(batch) >= batchSize || completed == totalRequests {
			for _, result := range batch {
				collector.Record(result.StatusCode, result.Elapsed, result.OK)
			}
			batch = batch[:0] // Reset batch

			if completed%max(1, totalRequests/10) == 0 {
				fmt.Printf("Completed %d/%d requests...\n", completed, totalRequests)
			}
		}
	}

	// Process any remaining results
	for _, result := range batch {
		collector.Record(result.StatusCode, result.Elapsed, result.OK)
	}

	totalTime := time.Since(startTime).Seconds()

	stat := collector.GetStatistics()

	if stat.Total == 0 {
		fmt.Println("No requests were executed.")
		return
	}

	// Display results
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Stress test finished")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total time            : %.4f seconds\n", totalTime)
	fmt.Printf("Requests per second   : %.2f req/s\n", float64(stat.Total)/totalTime)
	fmt.Printf("Successes             : %d\n", stat.Successes)
	fmt.Printf("Failures              : %d\n", stat.Failures)
	fmt.Println("Status codes          :")

	// Sort status codes for display
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
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ValidateURL validates the URL format and ensures it's a valid HTTP/HTTPS URL.
// It checks for required scheme (http or https) and host presence.
// Returns an error if the URL is invalid, empty, or doesn't meet requirements.
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