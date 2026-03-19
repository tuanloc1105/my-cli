package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "single header",
			input:    "Authorization:Bearer token",
			expected: map[string]string{"Authorization": "Bearer token"},
		},
		{
			name:     "multiple headers",
			input:    "Authorization:Bearer token;Content-Type:application/json",
			expected: map[string]string{"Authorization": "Bearer token", "Content-Type": "application/json"},
		},
		{
			name:     "value with commas preserved",
			input:    "Accept:text/html,application/json;Authorization:Bearer xyz",
			expected: map[string]string{"Accept": "text/html,application/json", "Authorization": "Bearer xyz"},
		},
		{
			name:     "whitespace trimmed",
			input:    " Authorization : Bearer token ; Accept : */* ",
			expected: map[string]string{"Authorization": "Bearer token", "Accept": "*/*"},
		},
		{
			name:     "entry without colon skipped",
			input:    "Authorization:Bearer token;invalidentry;Accept:*/*",
			expected: map[string]string{"Authorization": "Bearer token", "Accept": "*/*"},
		},
		{
			name:     "value with colons",
			input:    "X-Custom:a:b:c",
			expected: map[string]string{"X-Custom": "a:b:c"},
		},
		{
			name:     "empty entries skipped",
			input:    "Authorization:Bearer token;;Accept:*/*",
			expected: map[string]string{"Authorization": "Bearer token", "Accept": "*/*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseHeaders(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("got %d headers, want %d", len(result), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("header %q = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestParseData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantNil  bool
		wantErr  bool
	}{
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:     "single pair",
			input:    "key=value",
			expected: map[string]string{"key": "value"},
		},
		{
			name:     "multiple pairs",
			input:    "name=John&age=30&city=NYC",
			expected: map[string]string{"name": "John", "age": "30", "city": "NYC"},
		},
		{
			name:    "entry without equals returns error",
			input:   "key=value&invalid&other=data",
			wantErr: true,
		},
		{
			name:     "whitespace trimmed",
			input:    " key = value & other = data ",
			expected: map[string]string{"key": "value", "other": "data"},
		},
		{
			name:    "all entries invalid returns error",
			input:   "noequalssign",
			wantErr: true,
		},
		{
			name:     "value with equals sign",
			input:    "key=val=ue",
			expected: map[string]string{"key": "val=ue"},
		},
		{
			name:    "empty entries skipped",
			input:   "key=value&&other=data",
			expected: map[string]string{"key": "value", "other": "data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseData(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if result != nil {
					t.Errorf("got %v, want nil", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("got %d entries, want %d", len(result), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("data[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestPrepareBody(t *testing.T) {
	t.Run("no body sources", func(t *testing.T) {
		body, ct, err := PrepareBody("", "", nil, "", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if body != nil || ct != "" {
			t.Errorf("expected nil body and empty content type, got body=%v ct=%q", body, ct)
		}
	})

	t.Run("json body string", func(t *testing.T) {
		body, ct, err := PrepareBody(`{"key":"value"}`, "", nil, "", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != `{"key":"value"}` {
			t.Errorf("body = %q, want %q", body, `{"key":"value"}`)
		}
		if ct != "application/json" {
			t.Errorf("content-type = %q, want %q", ct, "application/json")
		}
	})

	t.Run("invalid json string", func(t *testing.T) {
		_, _, err := PrepareBody("{invalid", "", nil, "", "", "")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("json file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.json")
		os.WriteFile(path, []byte(`{"test":true}`), 0644)

		body, ct, err := PrepareBody("", path, nil, "", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != `{"test":true}` {
			t.Errorf("body = %q, want %q", body, `{"test":true}`)
		}
		if ct != "application/json" {
			t.Errorf("content-type = %q, want %q", ct, "application/json")
		}
	})

	t.Run("invalid json file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		os.WriteFile(path, []byte(`not json`), 0644)

		_, _, err := PrepareBody("", path, nil, "", "", "")
		if err == nil {
			t.Fatal("expected error for invalid JSON file")
		}
	})

	t.Run("form data", func(t *testing.T) {
		formData := map[string]string{"key": "value", "foo": "bar"}
		body, ct, err := PrepareBody("", "", formData, "", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q, want %q", ct, "application/x-www-form-urlencoded")
		}
		if len(body) == 0 {
			t.Error("expected non-empty body for form data")
		}
	})

	t.Run("raw body default content type", func(t *testing.T) {
		body, ct, err := PrepareBody("", "", nil, "raw content", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != "raw content" {
			t.Errorf("body = %q, want %q", body, "raw content")
		}
		if ct != "text/plain" {
			t.Errorf("content-type = %q, want %q", ct, "text/plain")
		}
	})

	t.Run("raw body custom content type", func(t *testing.T) {
		body, ct, err := PrepareBody("", "", nil, "<xml/>", "", "application/xml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != "<xml/>" {
			t.Errorf("body = %q, want %q", body, "<xml/>")
		}
		if ct != "application/xml" {
			t.Errorf("content-type = %q, want %q", ct, "application/xml")
		}
	})

	t.Run("raw file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.bin")
		os.WriteFile(path, []byte("file content"), 0644)

		body, ct, err := PrepareBody("", "", nil, "", path, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != "file content" {
			t.Errorf("body = %q, want %q", body, "file content")
		}
		if ct != "text/plain" {
			t.Errorf("content-type = %q, want %q", ct, "text/plain")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, _, err := PrepareBody("", "/nonexistent/file.json", nil, "", "", "")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})
}

func TestExecuteRequest_Success200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := server.Client()
	result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

	if !result.OK {
		t.Errorf("expected OK=true, got false")
	}
	if result.StatusCode != 200 {
		t.Errorf("status = %d, want 200", result.StatusCode)
	}
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.Elapsed <= 0 {
		t.Errorf("elapsed = %f, want > 0", result.Elapsed)
	}
}

func TestExecuteRequest_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := server.Client()
	result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

	if result.OK {
		t.Errorf("expected OK=false for 500 status")
	}
	if result.StatusCode != 500 {
		t.Errorf("status = %d, want 500", result.StatusCode)
	}
}

func TestExecuteRequest_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

	if result.OK {
		t.Errorf("expected OK=false for timeout")
	}
	if result.Error == "" {
		t.Errorf("expected error message for timeout")
	}
}

func TestExecuteRequest_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := server.Client()
	result := ExecuteRequest(ctx, client, "GET", server.URL, nil, nil, "", 0, "")

	if result.OK {
		t.Errorf("expected OK=false for cancelled context")
	}
	if result.Error == "" {
		t.Errorf("expected error message for cancelled context")
	}
}

func TestExecuteRequest_HeadersAndBody(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody string
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedMethod = r.Method
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedBody = string(bodyBytes)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{
		"X-Custom":      "test-value",
		"Authorization": "Bearer abc",
	}
	body := []byte(`{"key":"value"}`)

	client := server.Client()
	result := ExecuteRequest(context.Background(), client, "POST", server.URL, headers, body, "application/json", 0, "")

	if !result.OK {
		t.Fatalf("expected OK=true, got error: %s", result.Error)
	}
	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if receivedHeaders.Get("X-Custom") != "test-value" {
		t.Errorf("X-Custom header = %q, want %q", receivedHeaders.Get("X-Custom"), "test-value")
	}
	if receivedHeaders.Get("Authorization") != "Bearer abc" {
		t.Errorf("Authorization header = %q, want %q", receivedHeaders.Get("Authorization"), "Bearer abc")
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedHeaders.Get("Content-Type"), "application/json")
	}
	if receivedBody != `{"key":"value"}` {
		t.Errorf("body = %q, want %q", receivedBody, `{"key":"value"}`)
	}
}

func TestExecuteRequest_LargeResponseDrained(t *testing.T) {
	// Server returns a large response; verify it doesn't cause issues
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("x", 1024*512))) // 512KB
	}))
	defer server.Close()

	client := server.Client()
	result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}
}

func TestExecuteRequest_NoBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) > 0 {
				t.Errorf("expected empty body, got %d bytes", len(bodyBytes))
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := server.Client()
	result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

	if !result.OK {
		t.Errorf("expected OK=true, got error: %s", result.Error)
	}
}

func TestExecuteRequest_StatusCodeClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantOK     bool
	}{
		{"200 OK", 200, true},
		{"201 Created", 201, true},
		{"204 No Content", 204, true},
		{"299 edge", 299, true},
		{"301 Redirect", 301, false},
		{"400 Bad Request", 400, false},
		{"404 Not Found", 404, false},
		{"500 Internal", 500, false},
		{"503 Unavailable", 503, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			result := ExecuteRequest(context.Background(), client, "GET", server.URL, nil, nil, "", 0, "")

			if result.OK != tt.wantOK {
				t.Errorf("status %d: OK = %v, want %v", tt.statusCode, result.OK, tt.wantOK)
			}
			if result.StatusCode != tt.statusCode {
				t.Errorf("status = %d, want %d", result.StatusCode, tt.statusCode)
			}
		})
	}
}
