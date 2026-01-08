package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/engine"
)

func TestRetryAfterHTTPClient_ParsesSecondsHeader(t *testing.T) {
	// Create a test server that returns 429 with Retry-After header in seconds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify the Retry-After was captured
	duration, capturedAt := client.GetLastRetryAfter()
	if duration != 30*time.Second {
		t.Errorf("Expected 30s retry-after, got %v", duration)
	}
	if time.Since(capturedAt) > time.Second {
		t.Errorf("Captured time should be recent, got %v ago", time.Since(capturedAt))
	}
}

func TestRetryAfterHTTPClient_ParsesHTTPDateHeader(t *testing.T) {
	// Create a test server that returns 429 with Retry-After header as HTTP date
	retryTime := time.Now().Add(45 * time.Second).UTC()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", retryTime.Format(time.RFC1123))
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify the Retry-After was captured (allow some tolerance for timing)
	duration, _ := client.GetLastRetryAfter()
	if duration < 40*time.Second || duration > 50*time.Second {
		t.Errorf("Expected ~45s retry-after, got %v", duration)
	}
}

func TestRetryAfterHTTPClient_IgnoresNon429Responses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify no Retry-After was captured for 200 response
	duration, _ := client.GetLastRetryAfter()
	if duration != 0 {
		t.Errorf("Expected no retry-after for 200 response, got %v", duration)
	}
}

func TestRetryAfterHTTPClient_ClearRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify initial capture
	duration, _ := client.GetLastRetryAfter()
	if duration != 30*time.Second {
		t.Errorf("Expected 30s retry-after, got %v", duration)
	}

	// Clear and verify
	client.ClearRetryAfter()
	duration, _ = client.GetLastRetryAfter()
	if duration != 0 {
		t.Errorf("Expected 0 after clear, got %v", duration)
	}
}

func TestRetryAfterHTTPClient_StaleValuesIgnored(t *testing.T) {
	client := engine.NewRetryAfterHTTPClient(nil)
	
	// Manually check that stale values are ignored
	// We can't easily test the 60-second staleness without waiting,
	// but we can verify the interface works correctly
	duration, capturedAt := client.GetLastRetryAfter()
	if duration != 0 {
		t.Errorf("Expected 0 for empty client, got %v", duration)
	}
	if !capturedAt.IsZero() {
		t.Errorf("Expected zero time for empty client")
	}
}

func TestRetryAfterHTTPClient_HandlesEmptyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 429 without Retry-After header
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify no Retry-After captured when header is missing
	duration, _ := client.GetLastRetryAfter()
	if duration != 0 {
		t.Errorf("Expected 0 for missing header, got %v", duration)
	}
}

func TestRetryAfterHTTPClient_HandlesInvalidHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "not-a-number-or-date")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	client := engine.NewRetryAfterHTTPClient(nil)
	
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify no Retry-After captured for invalid value
	duration, _ := client.GetLastRetryAfter()
	if duration != 0 {
		t.Errorf("Expected 0 for invalid header, got %v", duration)
	}
}
