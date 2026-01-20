package request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Result holds the result of a single HTTP request execution.
// It contains the request outcome, status code, latency, and any error information.
type Result struct {
	OK         bool    // true if status code is 2xx
	StatusCode int     // HTTP status code (0 if request failed)
	Elapsed    float64 // Request duration in seconds
	Error      string  // Error message if request failed
}

// ParseHeaders parses HTTP headers from a comma-separated string format.
// Expected format: 'key1:value1,key2:value2'
// Returns an empty map if the input string is empty.
// Invalid entries (missing colon) are silently skipped.
func ParseHeaders(raw string) map[string]string {
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

// ParseData parses form data from URL-encoded string format.
// Expected format: 'key1=value1&key2=value2'
// Returns nil, nil if the input string is empty.
// Invalid entries (missing equals sign) are silently skipped.
func ParseData(raw string) (map[string]string, error) {
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

// PrepareBody prepares the HTTP request body and determines the Content-Type header.
// It processes body sources in the following priority order:
//   1. JSON body (from file or string) - validates JSON and sets Content-Type to application/json
//   2. Form data - encodes as application/x-www-form-urlencoded
//   3. Raw body (from file or string) - uses provided Content-Type or defaults to text/plain
// Returns the body bytes, content type, and any error encountered during processing.
func PrepareBody(
	jsonBody string, jsonFile string,
	formData map[string]string,
	rawBody string, rawFile string,
	contentTypeFlag string,
) ([]byte, string, error) {
	// Priority 1: JSON body (highest priority, includes validation)
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

	// Priority 2: Form data (URL-encoded)
	if formData != nil {
		values := url.Values{}
		for k, v := range formData {
			values.Set(k, v)
		}
		return []byte(values.Encode()), "application/x-www-form-urlencoded", nil
	}

	// Priority 3: Raw body content (file or string)
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

// ExecuteRequest executes a single HTTP request and measures its performance.
// It creates the request with the provided method, URL, headers, and body,
// executes it using the given HTTP client, and records the elapsed time.
// The response body is drained to allow connection reuse.
// Returns a Result containing the request outcome and metrics.
func ExecuteRequest(
	ctx context.Context,
	client *http.Client,
	method, targetURL string,
	headers map[string]string,
	body []byte,
	contentType string,
) Result {
	startedAt := time.Now()

	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), targetURL, reqBody)
	if err != nil {
		return Result{
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
		return Result{
			OK:      false,
			Elapsed: elapsed,
			Error:   err.Error(),
		}
	}
	defer resp.Body.Close()
	// Drain response body completely to allow HTTP connection reuse
	// This improves performance when making multiple requests to the same host
	io.Copy(io.Discard, resp.Body)

	statusCode := resp.StatusCode
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300

	return Result{
		OK:         ok,
		StatusCode: statusCode,
		Elapsed:    elapsed,
		Error:      "",
	}
}