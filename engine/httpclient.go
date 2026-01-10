package engine

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mykhaliev/agent-benchmark/logger"
)

// RetryAfterHTTPClient wraps an http.Client to capture Retry-After headers from 429 responses.
// This is needed because LangChainGo doesn't expose HTTP headers in errors, only the error message.
// By intercepting the response, we can extract the actual Retry-After header value.
type RetryAfterHTTPClient struct {
	wrapped *http.Client

	mu               sync.RWMutex
	lastRetryAfter   time.Duration
	lastRetryAfterAt time.Time
}

// NewRetryAfterHTTPClient creates a new HTTP client wrapper that captures Retry-After headers.
// If wrapped is nil, a default http.Client with 30 second timeout is used.
func NewRetryAfterHTTPClient(wrapped *http.Client) *RetryAfterHTTPClient {
	if wrapped == nil {
		wrapped = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &RetryAfterHTTPClient{
		wrapped: wrapped,
	}
}

// Do implements the http.RoundTripper-like interface that LangChainGo expects (Doer interface).
// It captures Retry-After headers from 429 responses before returning them.
func (c *RetryAfterHTTPClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.wrapped.Do(req)
	if err != nil {
		return resp, err
	}

	// Check for 429 status and capture Retry-After header
	// Azure OpenAI returns both retry-after (seconds) and retry-after-ms (milliseconds)
	// We prefer retry-after-ms for higher precision
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := c.extractRetryAfterFromResponse(resp)
		if retryAfter > 0 {
			c.mu.Lock()
			c.lastRetryAfter = retryAfter
			c.lastRetryAfterAt = time.Now()
			c.mu.Unlock()
			if logger.Logger != nil {
				logger.Logger.Debug("Captured Retry-After from 429 response",
					"retry_after_seconds", retryAfter.Seconds(),
					"retry_after_ms_header", resp.Header.Get("retry-after-ms"),
					"retry_after_header", resp.Header.Get("Retry-After"))
			}
		}
	}

	return resp, err
}

// extractRetryAfterFromResponse extracts the retry duration from response headers.
// Azure OpenAI returns both headers:
// - retry-after-ms: milliseconds (more precise, preferred)
// - retry-after / Retry-After: seconds (fallback)
// See: https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/provisioned-get-started#handling-high-utilization
func (c *RetryAfterHTTPClient) extractRetryAfterFromResponse(resp *http.Response) time.Duration {
	// First, try retry-after-ms (Azure OpenAI specific, more precise)
	// Header names are case-insensitive in HTTP, but Go's Header.Get is case-insensitive
	if msValue := resp.Header.Get("retry-after-ms"); msValue != "" {
		if ms, err := strconv.Atoi(strings.TrimSpace(msValue)); err == nil && ms > 0 {
			if logger.Logger != nil {
				logger.Logger.Debug("Using retry-after-ms header", "milliseconds", ms)
			}
			return time.Duration(ms) * time.Millisecond
		}
	}

	// Fall back to standard Retry-After header (seconds or HTTP-date)
	return c.parseRetryAfterHeader(resp.Header.Get("Retry-After"))
}

// GetLastRetryAfter returns the last captured Retry-After duration and when it was captured.
// Returns (0, zero time) if no Retry-After has been captured or if it's stale (> 60 seconds old).
func (c *RetryAfterHTTPClient) GetLastRetryAfter() (time.Duration, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Consider values older than 60 seconds as stale
	if time.Since(c.lastRetryAfterAt) > 60*time.Second {
		return 0, time.Time{}
	}

	return c.lastRetryAfter, c.lastRetryAfterAt
}

// ClearRetryAfter clears the cached Retry-After value.
// Call this after successfully using the value to avoid reusing stale data.
func (c *RetryAfterHTTPClient) ClearRetryAfter() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastRetryAfter = 0
	c.lastRetryAfterAt = time.Time{}
}

// parseRetryAfterHeader parses the Retry-After header value.
// The header can be either:
// - An integer representing seconds (e.g., "120")
// - An HTTP-date (e.g., "Wed, 21 Oct 2025 07:28:00 GMT")
func (c *RetryAfterHTTPClient) parseRetryAfterHeader(value string) time.Duration {
	if value == "" {
		return 0
	}

	value = strings.TrimSpace(value)

	// Try parsing as seconds (most common for rate limiting)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP-date (RFC 1123 format)
	// Common formats: "Mon, 02 Jan 2006 15:04:05 GMT"
	httpDateFormats := []string{
		time.RFC1123,
		time.RFC1123Z,
		"Mon, 02 Jan 2006 15:04:05 MST",
	}

	for _, format := range httpDateFormats {
		if t, err := time.Parse(format, value); err == nil {
			duration := time.Until(t)
			if duration > 0 {
				return duration
			}
			// If the date is in the past, return a minimum backoff
			return time.Second
		}
	}

	// Could not parse - some APIs return non-standard formats
	if logger.Logger != nil {
		logger.Logger.Warn("Could not parse Retry-After header", "value", value)
	}
	return 0
}

// RetryAfterProvider is an interface for components that can provide Retry-After information
type RetryAfterProvider interface {
	GetLastRetryAfter() (time.Duration, time.Time)
	ClearRetryAfter()
}
