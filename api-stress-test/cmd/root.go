// Package cmd provides the command-line interface and test execution logic
// for the API stress test tool.
package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"api-stress-test/internal/request"
	"api-stress-test/internal/stats"
	"api-stress-test/internal/ui"

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
		targetURL        string
		method           string
		requests         int
		concurrency      int
		timeout          float64
		headers          string
		data             string
		jsonBody         string
		jsonFile         string
		rawBody          string
		rawFile          string
		contentTypeFlag  string
		rate             float64
		duration         string
		outputFormat     string
		insecure         bool
		disableKeepalive bool
		disableRedirects bool
		expectStatus     int
		expectBody       string
		warmup           string
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
  api-stress-test --url http://example.com/api --requests 100 --output json
  api-stress-test --url https://example.com/api --insecure --expect-status 200`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ValidateURL(targetURL); err != nil {
				return err
			}
			if err := ValidateMethod(method); err != nil {
				return err
			}
			if outputFormat != "text" && outputFormat != "json" {
				return fmt.Errorf("unsupported output format: %s (supported: text, json)", outputFormat)
			}

			parsedHeaders := request.ParseHeaders(headers)

			parsedData, err := request.ParseData(data)
			if err != nil {
				return fmt.Errorf("parsing --data: %w", err)
			}

			body, contentType, err := request.PrepareBody(jsonBody, jsonFile, parsedData, rawBody, rawFile, contentTypeFlag)
			if err != nil {
				return fmt.Errorf("preparing body: %w", err)
			}

			if timeout <= 0 {
				return fmt.Errorf("timeout must be positive (got %.2f)", timeout)
			}
			if cmd.Flags().Changed("rate") && rate <= 0 {
				return fmt.Errorf("rate must be positive when specified (got %.2f)", rate)
			}
			if concurrency > 10000 {
				return fmt.Errorf("concurrency too high: %d (max 10000)", concurrency)
			}

			if requests <= 0 {
				requests = 100
			}
			if concurrency <= 0 {
				concurrency = 10
			}

			var dur time.Duration
			if duration != "" {
				dur, err = time.ParseDuration(duration)
				if err != nil {
					return fmt.Errorf("invalid duration: %w", err)
				}
			}

			var warmupDur time.Duration
			if warmup != "" {
				warmupDur, err = time.ParseDuration(warmup)
				if err != nil {
					return fmt.Errorf("invalid warmup duration: %w", err)
				}
			}

			return RunStressTest(
				os.Stdout,
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
				insecure,
				disableKeepalive,
				disableRedirects,
				expectStatus,
				expectBody,
				warmupDur,
			)
		},
	}

	// Required flag
	rootCmd.Flags().StringVar(&targetURL, "url", "", "Target URL (required)")
	_ = rootCmd.MarkFlagRequired("url")

	// Request options
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

	// Load control
	rootCmd.Flags().Float64Var(&rate, "rate", 0, "Max requests per second (0 = unlimited)")
	rootCmd.Flags().StringVar(&duration, "duration", "", "Test duration (e.g., 30s, 1m) instead of fixed request count")

	// Transport tuning
	rootCmd.Flags().BoolVarP(&insecure, "insecure", "k", false, "Skip TLS certificate verification")
	rootCmd.Flags().BoolVar(&disableKeepalive, "disable-keepalive", false, "Disable HTTP keep-alive (new connection per request)")
	rootCmd.Flags().BoolVar(&disableRedirects, "disable-redirects", false, "Do not follow HTTP redirects")

	// Response validation
	rootCmd.Flags().IntVar(&expectStatus, "expect-status", 0, "Expected HTTP status code (others count as failure)")
	rootCmd.Flags().StringVar(&expectBody, "expect-body", "", "Expected substring in response body")

	// Warm-up
	rootCmd.Flags().StringVar(&warmup, "warmup", "", "Warm-up duration before recording stats (e.g., 5s)")

	// Output
	rootCmd.Flags().StringVar(&outputFormat, "output", "text", "Output format: text or json")

	// Mutual exclusivity
	rootCmd.MarkFlagsMutuallyExclusive("data", "json-body", "json-file", "body", "file")
	rootCmd.MarkFlagsMutuallyExclusive("requests", "duration")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// RunStressTest runs the HTTP stress test and returns an error if there are failures.
// Output is written to w; pass os.Stdout for normal CLI usage.
func RunStressTest(
	w io.Writer,
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
	insecure bool,
	disableKeepalive bool,
	disableRedirects bool,
	expectStatus int,
	expectBody string,
	warmup time.Duration,
) error {
	isJSON := outputFormat == "json"
	isDurationMode := duration > 0

	if !isJSON {
		durationStr := ""
		if isDurationMode {
			durationStr = duration.String()
		}
		ui.PrintHeader(w, targetURL, method, totalRequests, concurrency, timeout.Seconds(), rate, isDurationMode, durationStr, len(body), contentType)
	}

	// Configure HTTP Transport
	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   disableKeepalive,
	}
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	if disableRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Run warm-up phase (requests without recording stats)
	if warmup > 0 {
		if !isJSON {
			fmt.Fprintf(w, "Warming up for %s...\n", warmup)
		}
		warmCtx, warmCancel := context.WithTimeout(context.Background(), warmup)
		defer warmCancel() // Bug 3 fix: defer cancel immediately

		// Bug 4 fix: handle signals during warmup
		warmSigChan := make(chan os.Signal, 1)
		signal.Notify(warmSigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			select {
			case <-warmSigChan:
				warmCancel()
			case <-warmCtx.Done():
			}
		}()

		var warmWg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			warmWg.Add(1)
			go func() {
				defer warmWg.Done()
				for warmCtx.Err() == nil {
					res := request.ExecuteRequest(warmCtx, client, method, targetURL, headers, body, contentType, 0, "")
					// Bug 5 fix: back off on failure to prevent CPU spin
					if !res.OK && res.Elapsed < 0.01 {
						time.Sleep(10 * time.Millisecond)
					}
				}
			}()
		}
		warmWg.Wait()
		signal.Stop(warmSigChan)
		if !isJSON {
			fmt.Fprintln(w, "Warm-up complete. Starting test...")
			fmt.Fprintln(w, strings.Repeat("-", 60))
		}
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
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		if !isJSON {
			fmt.Fprintln(w, "\nStopping requests... (waiting for active workers to finish)")
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

	// Setup live progress display
	var progress *ui.Progress
	if !isJSON {
		progress = ui.NewProgress(w, int64(totalRequests), isDurationMode, duration)
		progress.Start()
	}

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
				func() {
					defer func() {
						if r := recover(); r != nil {
							results <- request.Result{
								OK:    false,
								Error: fmt.Sprintf("panic: %v", r),
							}
						}
					}()
					results <- request.ExecuteRequest(ctx, client, method, targetURL, headers, body, contentType, expectStatus, expectBody)
				}()
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
	batchSize := max(1, concurrency/2)
	batch := make([]request.Result, 0, batchSize)

	for res := range results {
		batch = append(batch, res)

		if len(batch) >= batchSize {
			for _, result := range batch {
				collector.Record(result.StatusCode, result.Elapsed, result.OK, result.Error)
			}
			if progress != nil {
				progress.Add(int64(len(batch)))
			}
			batch = batch[:0]
		}
	}

	// Flush remaining batch
	if len(batch) > 0 {
		for _, result := range batch {
			collector.Record(result.StatusCode, result.Elapsed, result.OK, result.Error)
		}
		if progress != nil {
			progress.Add(int64(len(batch)))
		}
	}

	// Stop progress display
	if progress != nil {
		progress.Stop()
	}

	totalTime := time.Since(startTime).Seconds()
	stat := collector.GetStatistics()

	if stat.Total == 0 {
		if !isJSON {
			fmt.Fprintln(w, "No requests were executed.")
		}
		return nil
	}

	var reqPerSec float64
	if totalTime > 0 {
		reqPerSec = float64(stat.Total) / totalTime
	}

	// Output results
	if isJSON {
		output := ui.JSONOutput{
			Config: ui.TestConfig{
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
		if err := ui.PrintJSONResult(w, output); err != nil {
			return err
		}
	} else {
		ui.PrintTextResult(w, stat, totalTime, reqPerSec)
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
