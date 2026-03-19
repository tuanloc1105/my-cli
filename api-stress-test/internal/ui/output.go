// Package ui handles output formatting for the API stress test tool.
package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"api-stress-test/internal/stats"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

// colorEnabled returns true if the writer supports ANSI colors (i.e., is a terminal).
func colorEnabled(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorize(w io.Writer, color, text string) string {
	if !colorEnabled(w) {
		return text
	}
	return color + text + colorReset
}

// statusColor returns the appropriate color for an HTTP status code.
func statusColor(w io.Writer, status int) string {
	if !colorEnabled(w) {
		return ""
	}
	switch {
	case status >= 200 && status < 300:
		return colorGreen
	case status >= 400 && status < 500:
		return colorYellow
	case status >= 500:
		return colorRed
	default:
		return colorRed
	}
}

// TestConfig holds the test configuration for JSON output.
type TestConfig struct {
	URL         string  `json:"url"`
	Method      string  `json:"method"`
	Requests    int     `json:"requests,omitempty"`
	Duration    string  `json:"duration,omitempty"`
	Concurrency int     `json:"concurrency"`
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

// PrintHeader prints the test configuration before the test starts.
func PrintHeader(w io.Writer, url, method string, totalRequests, concurrency int, timeoutSec, rate float64, isDurationMode bool, duration string, bodyLen int, contentType string) {
	fmt.Fprintf(w, "%s : %s\n", colorize(w, colorBold, "Target URL           "), url)
	fmt.Fprintf(w, "%s : %s\n", colorize(w, colorBold, "HTTP method          "), method)
	if isDurationMode {
		fmt.Fprintf(w, "%s : %s\n", colorize(w, colorBold, "Duration             "), duration)
	} else {
		fmt.Fprintf(w, "%s : %d\n", colorize(w, colorBold, "Total requests       "), totalRequests)
	}
	fmt.Fprintf(w, "%s : %d\n", colorize(w, colorBold, "Concurrency (workers)"), concurrency)
	fmt.Fprintf(w, "%s : %.1f seconds\n", colorize(w, colorBold, "Timeout per request  "), timeoutSec)
	if rate > 0 {
		fmt.Fprintf(w, "%s : %.0f req/s\n", colorize(w, colorBold, "Rate limit           "), rate)
	}
	if bodyLen > 0 {
		fmt.Fprintf(w, "%s : %d bytes\n", colorize(w, colorBold, "Body size            "), bodyLen)
		if contentType != "" {
			fmt.Fprintf(w, "%s : %s\n", colorize(w, colorBold, "Content-Type         "), contentType)
		}
	}
	fmt.Fprintln(w, strings.Repeat("-", 60))
}

// PrintTextResult prints the test results in human-readable text format with colors.
func PrintTextResult(w io.Writer, stat stats.Statistics, totalTime, reqPerSec float64) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, colorize(w, colorBold, strings.Repeat("=", 60)))
	fmt.Fprintln(w, colorize(w, colorBold, "Stress test finished"))
	fmt.Fprintln(w, colorize(w, colorBold, strings.Repeat("=", 60)))
	fmt.Fprintf(w, "Total time            : %.4f seconds\n", totalTime)
	fmt.Fprintf(w, "Requests per second   : %.2f req/s\n", reqPerSec)
	fmt.Fprintf(w, "Successes             : %s\n", colorize(w, colorGreen, fmt.Sprintf("%d", stat.Successes)))
	if stat.Failures > 0 {
		fmt.Fprintf(w, "Failures              : %s\n", colorize(w, colorRed, fmt.Sprintf("%d", stat.Failures)))
	} else {
		fmt.Fprintf(w, "Failures              : %d\n", stat.Failures)
	}
	fmt.Fprintln(w, "Status codes          :")

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
		sc := statusColor(w, status)
		if sc != "" {
			fmt.Fprintf(w, "  %s%-15s%s %d\n", sc, label, colorReset, count)
		} else {
			fmt.Fprintf(w, "  %-15s %d\n", label, count)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, colorize(w, colorBold, "Latency (seconds)"))
	fmt.Fprintf(w, "  Min                 : %.4f\n", stat.MinLatency)
	fmt.Fprintf(w, "  Max                 : %.4f\n", stat.MaxLatency)
	fmt.Fprintf(w, "  Average             : %.4f\n", stat.AvgLatency)
	fmt.Fprintf(w, "  p50                 : %.4f\n", stat.P50Latency)
	fmt.Fprintf(w, "  p90                 : %.4f\n", stat.P90Latency)
	fmt.Fprintf(w, "  p99                 : %.4f\n", stat.P99Latency)

	// Histogram
	if len(stat.Histogram) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, colorize(w, colorBold, "Latency distribution"))
		printHistogram(w, stat.Histogram)
	}

	if len(stat.TopErrors) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, colorize(w, colorBold, "Top Errors            :"))
		for _, e := range stat.TopErrors {
			fmt.Fprintf(w, "  %s x %d\n", colorize(w, colorRed, e.Message), e.Count)
		}
	}
}

// printHistogram renders an ASCII histogram.
func printHistogram(w io.Writer, buckets []stats.HistogramBucket) {
	maxCount := 0
	for _, b := range buckets {
		if b.Count > maxCount {
			maxCount = b.Count
		}
	}
	if maxCount == 0 {
		return
	}

	const barWidth = 30
	total := 0
	for _, b := range buckets {
		total += b.Count
	}

	for _, b := range buckets {
		barLen := b.Count * barWidth / maxCount
		bar := strings.Repeat("█", barLen)
		pct := float64(b.Count) / float64(total) * 100
		fmt.Fprintf(w, "  [%.3f-%.3fs] %s%-*s%s %d (%.1f%%)\n",
			b.MinSec, b.MaxSec,
			func() string {
				if colorEnabled(w) {
					return colorCyan
				}
				return ""
			}(),
			barWidth, bar,
			func() string {
				if colorEnabled(w) {
					return colorReset
				}
				return ""
			}(),
			b.Count, pct)
	}
}

// PrintJSONResult prints the test results in JSON format.
func PrintJSONResult(w io.Writer, output JSONOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON output: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}
