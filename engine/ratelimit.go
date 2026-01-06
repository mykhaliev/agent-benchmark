package engine

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/tmc/langchaingo/llms"
	"golang.org/x/time/rate"
)

const (
	// Default retry configuration
	defaultMaxRetries     = 5
	defaultInitialBackoff = 1 * time.Second
	defaultMaxBackoff     = 60 * time.Second
)

// RateLimitedLLM wraps an llms.Model with rate limiting capabilities
type RateLimitedLLM struct {
	wrapped    llms.Model
	tpmLimiter *rate.Limiter // Tokens per minute limiter
	rpmLimiter *rate.Limiter // Requests per minute limiter
	tpmLimit   int
	rpmLimit   int
	maxRetries int // Max number of 429 retries before stopping
}

// NewRateLimitedLLM creates a new rate-limited wrapper around an LLM
// tpm: tokens per minute limit (0 means no limit)
// rpm: requests per minute limit (0 means no limit)
func NewRateLimitedLLM(wrapped llms.Model, config model.RateLimitConfig) *RateLimitedLLM {
	// Default to 1 retry if not specified
	maxRetries := config.MaxRateLimitRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	rl := &RateLimitedLLM{
		wrapped:    wrapped,
		tpmLimit:   config.TPM,
		rpmLimit:   config.RPM,
		maxRetries: maxRetries,
	}

	// Create TPM limiter if configured
	// Rate is tokens per second, burst is the full minute's worth
	if config.TPM > 0 {
		tokensPerSecond := float64(config.TPM) / 60.0
		rl.tpmLimiter = rate.NewLimiter(rate.Limit(tokensPerSecond), config.TPM)
		logger.Logger.Info("Rate limiter configured", "type", "TPM", "limit", config.TPM, "tokens_per_second", tokensPerSecond)
	}

	// Create RPM limiter if configured
	// Rate is requests per second, burst is the full minute's worth
	if config.RPM > 0 {
		requestsPerSecond := float64(config.RPM) / 60.0
		rl.rpmLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), config.RPM)
		logger.Logger.Info("Rate limiter configured", "type", "RPM", "limit", config.RPM, "requests_per_second", requestsPerSecond)
	}

	return rl
}

// GenerateContent implements llms.Model interface with rate limiting and retry logic
func (rl *RateLimitedLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// Wait for RPM limit (one request)
	if rl.rpmLimiter != nil {
		logger.Logger.Debug("Waiting for RPM rate limit")
		if err := rl.rpmLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	// For TPM, we estimate tokens before the call using a rough heuristic
	// A more accurate approach would be to use a tokenizer, but for simplicity
	// we estimate based on message content length (roughly 4 chars per token)
	estimatedInputTokens := rl.estimateInputTokens(messages)

	if rl.tpmLimiter != nil && estimatedInputTokens > 0 {
		logger.Logger.Debug("Waiting for TPM rate limit", "estimated_tokens", estimatedInputTokens)
		if err := rl.tpmLimiter.WaitN(ctx, estimatedInputTokens); err != nil {
			return nil, err
		}
	}

	// Retry loop with exponential backoff for 429 errors
	var response *llms.ContentResponse
	var err error
	backoff := defaultInitialBackoff

	for attempt := 0; attempt <= rl.maxRetries; attempt++ {
		start := time.Now()
		response, err = rl.wrapped.GenerateContent(ctx, messages, options...)
		elapsed := time.Since(start)

		if err == nil {
			// Success - adjust token limiter and return
			if response != nil && rl.tpmLimiter != nil {
				actualTokens := rl.getActualTokens(response)
				if actualTokens > estimatedInputTokens {
					additional := actualTokens - estimatedInputTokens
					reservation := rl.tpmLimiter.ReserveN(time.Now(), additional)
					if reservation.OK() {
						logger.Logger.Debug("Reserved additional tokens",
							"estimated", estimatedInputTokens,
							"actual", actualTokens,
							"additional", additional,
							"delay", reservation.Delay())
					}
				}
				logger.Logger.Debug("Request completed",
					"estimated_tokens", estimatedInputTokens,
					"actual_tokens", rl.getActualTokens(response),
					"duration", elapsed)
			}
			return response, nil
		}

		// Check if this is a rate limit error (429)
		if !rl.isRateLimitError(err) {
			// Not a rate limit error, return immediately
			return nil, err
		}

		// Extract retry-after duration from error message
		retryAfter := rl.extractRetryAfter(err)
		if retryAfter > 0 {
			backoff = retryAfter
		}

		// Cap backoff at max
		if backoff > defaultMaxBackoff {
			backoff = defaultMaxBackoff
		}

		if attempt < rl.maxRetries {
			logger.Logger.Warn("Rate limit hit, waiting before retry",
				"attempt", attempt+1,
				"max_retries", rl.maxRetries,
				"wait_seconds", backoff.Seconds(),
				"error", err.Error())

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}

			// Exponential backoff for next attempt (if retry-after wasn't specified)
			if retryAfter == 0 {
				backoff *= 2
			}
		}
	}

	// All retries exhausted
	logger.Logger.Error("Rate limit retries exhausted", "max_retries", rl.maxRetries, "error", err.Error())
	return nil, err
}

// isRateLimitError checks if the error is a 429 rate limit error
func (rl *RateLimitedLLM) isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "Rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "Too Many Requests")
}

// extractRetryAfter extracts the retry-after duration from an error message
// Azure OpenAI errors typically contain "Please retry after X seconds"
func (rl *RateLimitedLLM) extractRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}

	errStr := err.Error()

	// Pattern: "retry after X seconds" or "retry after X second"
	re := regexp.MustCompile(`retry after (\d+) seconds?`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) >= 2 {
		seconds, parseErr := strconv.Atoi(matches[1])
		if parseErr == nil && seconds > 0 {
			// Add a small buffer to ensure we're past the rate limit window
			return time.Duration(seconds+1) * time.Second
		}
	}

	return 0
}

// estimateInputTokens provides a rough estimate of input tokens
// using the heuristic of ~4 characters per token
func (rl *RateLimitedLLM) estimateInputTokens(messages []llms.MessageContent) int {
	totalChars := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				totalChars += len(p.Text)
			}
		}
	}
	// Rough estimate: 4 characters per token, with a minimum of 1
	tokens := totalChars / 4
	if tokens < 1 && totalChars > 0 {
		tokens = 1
	}
	return tokens
}

// getActualTokens extracts actual token counts from the response
func (rl *RateLimitedLLM) getActualTokens(response *llms.ContentResponse) int {
	if response == nil || len(response.Choices) == 0 {
		return 0
	}

	choice := response.Choices[0]
	if choice.GenerationInfo == nil {
		return 0
	}

	totalTokens := 0

	// Try to get total tokens
	if total, ok := choice.GenerationInfo["TotalTokens"]; ok {
		if t, ok := total.(int); ok {
			return t
		}
	}

	// Fall back to prompt + completion tokens
	if prompt, ok := choice.GenerationInfo["PromptTokens"]; ok {
		if p, ok := prompt.(int); ok {
			totalTokens += p
		}
	}
	if completion, ok := choice.GenerationInfo["CompletionTokens"]; ok {
		if c, ok := completion.(int); ok {
			totalTokens += c
		}
	}

	return totalTokens
}

// Call implements the llms.Model interface for simple text generation
func (rl *RateLimitedLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	// Convert to MessageContent format and use GenerateContent
	messages := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: prompt},
			},
		},
	}

	response, err := rl.GenerateContent(ctx, messages, options...)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", nil
	}

	return response.Choices[0].Content, nil
}

// HasRateLimiting returns true if any rate limiting is configured
func HasRateLimiting(config model.RateLimitConfig) bool {
	return config.TPM > 0 || config.RPM > 0
}
