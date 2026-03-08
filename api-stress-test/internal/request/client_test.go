package request

import (
	"os"
	"path/filepath"
	"testing"
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
			name:     "entry without equals skipped",
			input:    "key=value&invalid&other=data",
			expected: map[string]string{"key": "value", "other": "data"},
		},
		{
			name:     "whitespace trimmed",
			input:    " key = value & other = data ",
			expected: map[string]string{"key": "value", "other": "data"},
		},
		{
			name:    "all entries invalid returns nil",
			input:   "noequalssign",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseData(tt.input)
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
