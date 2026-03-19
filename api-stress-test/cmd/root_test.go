package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-stress-test/internal/ui"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/path?q=1", false},
		{"valid with port", "http://localhost:8080/api", false},
		{"empty", "", true},
		{"missing scheme", "example.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"missing host", "http://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMethod(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		wantErr bool
	}{
		{"GET", "GET", false},
		{"POST", "POST", false},
		{"PUT", "PUT", false},
		{"DELETE", "DELETE", false},
		{"PATCH", "PATCH", false},
		{"HEAD", "HEAD", false},
		{"OPTIONS", "OPTIONS", false},
		{"lowercase get", "get", false},
		{"mixed case", "Post", false},
		{"invalid", "INVALID", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMethod(tt.method)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMethod(%q) error = %v, wantErr %v", tt.method, err, tt.wantErr)
			}
		})
	}
}

// runTest is a helper that calls RunStressTest with default options for the new flags.
func runTest(t *testing.T, buf *bytes.Buffer, url, method string, requests, concurrency int, timeout time.Duration, headers map[string]string, body []byte, contentType string, rate float64, duration time.Duration, output string) error {
	t.Helper()
	return RunStressTest(buf, url, method, requests, concurrency, timeout, headers, body, contentType, rate, duration, output, false, false, false, 0, "", 0)
}

func TestRunStressTest_BasicSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	err := runTest(t, &buf, server.URL, "GET", 10, 2, 5*time.Second, nil, nil, "", 0, 0, "text")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Stress test finished") {
		t.Errorf("expected 'Stress test finished' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Successes") {
		t.Errorf("expected 'Successes' in output")
	}
}

func TestRunStressTest_AllFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	err := runTest(t, &buf, server.URL, "GET", 10, 2, 5*time.Second, nil, nil, "", 0, 0, "text")

	if err == nil {
		t.Fatal("expected error for all-failure test")
	}
	if !strings.Contains(err.Error(), "10 out of 10 requests failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunStressTest_JSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	err := runTest(t, &buf, server.URL, "GET", 10, 2, 5*time.Second, nil, nil, "", 0, 0, "json")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output ui.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	if output.Statistics.Total != 10 {
		t.Errorf("total = %d, want 10", output.Statistics.Total)
	}
	if output.Statistics.Successes != 10 {
		t.Errorf("successes = %d, want 10", output.Statistics.Successes)
	}
	if output.Config.URL != server.URL {
		t.Errorf("config URL = %q, want %q", output.Config.URL, server.URL)
	}
	if output.ReqPerSec <= 0 {
		t.Errorf("req/s = %f, want > 0", output.ReqPerSec)
	}
}

func TestRunStressTest_DurationMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	_ = runTest(t, &buf, server.URL, "GET", 100, 2, 5*time.Second, nil, nil, "", 0, 1*time.Second, "json")

	var output ui.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if output.Statistics.Total == 0 {
		t.Error("expected some requests in duration mode")
	}
	if output.Config.Duration == "" {
		t.Error("expected duration in config")
	}
}

func TestRunStressTest_RateLimiting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	start := time.Now()
	err := runTest(t, &buf, server.URL, "GET", 5, 2, 5*time.Second, nil, nil, "", 10, 0, "json")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("rate limiting too fast: %v (expected >= 400ms)", elapsed)
	}
}

func TestRunStressTest_WithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1024)
		n, _ := r.Body.Read(b)
		receivedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	body := []byte(`{"test":true}`)
	err := runTest(t, &buf, server.URL, "POST", 1, 1, 5*time.Second, nil, body, "application/json", 0, 0, "json")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody != `{"test":true}` {
		t.Errorf("received body = %q, want %q", receivedBody, `{"test":true}`)
	}
}

func TestRunStressTest_ExpectStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201
	}))
	defer server.Close()

	var buf bytes.Buffer
	// Expect 200 but server returns 201 → all should fail
	err := RunStressTest(&buf, server.URL, "GET", 5, 1, 5*time.Second, nil, nil, "", 0, 0, "json", false, false, false, 200, "", 0)

	if err == nil {
		t.Fatal("expected error when expect-status doesn't match")
	}
}

func TestRunStressTest_ExpectBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","data":"hello"}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	// Expect body containing "missing" but server returns "hello" → should fail
	err := RunStressTest(&buf, server.URL, "GET", 5, 1, 5*time.Second, nil, nil, "", 0, 0, "json", false, false, false, 0, "missing", 0)

	if err == nil {
		t.Fatal("expected error when expect-body doesn't match")
	}
}
