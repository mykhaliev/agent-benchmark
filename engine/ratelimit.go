package engine

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/pkoukk/tiktoken-go"
	"github.com/tmc/langchaingo/llms"
	"golang.org/x/time/rate"
)

const (
	// Default retry configuration
	defaultMaxRetries     = 5
	defaultInitialBackoff = 1 * time.Second
	defaultMaxBackoff     = 60 * time.Second
)

// RateLimitStats tracks statistics about rate limiting and 429 handling
type RateLimitStats struct {
	mu sync.Mutex
	// Proactive throttling stats
	ThrottleCount    int           `json:"throttleCount"`    // Number of times request was throttled
	ThrottleWaitTime time.Duration `json:"throttleWaitTime"` // Total time spent waiting due to throttling
	// Reactive 429 handling stats
	RateLimitHits     int           `json:"rateLimitHits"`     // Number of 429 errors received
	RetryCount        int           `json:"retryCount"`        // Number of retry attempts made
	RetryWaitTime     time.Duration `json:"retryWaitTime"`     // Total time spent waiting for retries
	RetrySuccessCount int           `json:"retrySuccessCount"` // Number of successful retries
}

// RateLimitedLLM wraps an llms.Model with rate limiting and optional 429 retry capabilities.
//
// IMPORTANT: Rate limiting is BEST-EFFORT, not guaranteed.
//
// Why best-effort?
//  1. Inaccurate estimates: Token estimation (even with tiktoken) is not 100% accurate.
//     Azure may count tokens differently than our estimation. We use a 50% safety margin
//     but this may still be insufficient in edge cases.
//
//  2. Async API consumption: The actual API doesn't immediately return usage info.
//     We estimate tokens before the request, but the actual consumption is only known
//     after the response completes. This means rate limiting decisions are made on estimates.
//
//  3. Multiple factors: Rate limits depend on both RPM (requests per minute) and TPM
//     (tokens per minute). We track both, but prioritize TPM since it's usually more restrictive.
//
//  4. Request overhead: We don't account for Azure infrastructure overhead, retransmissions,
//     or other hidden token consumption. Our estimates may be conservative but not perfect.
//
// Best Practice:
// - Use rate limiting + 429 retries together (defense in depth)
// - Enable both TPM and RPM limits for your quota
// - Monitor throttle_count and rate_limit_hits in stats
// - If 429 errors still occur, our retry mechanism handles them gracefully
// - For mission-critical workloads, add additional backoff or request queuing above this layer
type RateLimitedLLM struct {
	wrapped    llms.Model
	tpmLimiter *rate.Limiter // Tokens per minute limiter (proactive)
	rpmLimiter *rate.Limiter // Requests per minute limiter (proactive)
	tpmLimit   int
	rpmLimit   int
	modelName  string // Model name for accurate tokenization
	// Calibration (in-memory per run)
	calibrationMu          sync.Mutex
	calibrationRatio       float64
	calibrationInitialized bool
	// 429 retry handling (reactive) - separate from rate limiting
	retryOn429         bool               // Whether to retry on 429 errors (default: false)
	maxRetries         int                // Max number of 429 retries (only used if retryOn429 is true)
	retryAfterProvider RetryAfterProvider // Optional provider for Retry-After header values
	// Statistics tracking
	stats RateLimitStats
}

// NewRateLimitedLLM creates a new rate-limited wrapper around an LLM
// rateLimitConfig: proactive rate limiting (TPM/RPM throttling)
// retryConfig: reactive error handling (429 retry behavior)
// modelName: model identifier for accurate tokenization (e.g., "gpt-4", "gpt-3.5-turbo")
func NewRateLimitedLLM(wrapped llms.Model, rateLimitConfig model.RateLimitConfig, retryConfig model.RetryConfig, modelName string) *RateLimitedLLM {
	// Default max retries to 3 if retry is enabled but max not specified
	maxRetries := retryConfig.MaxRetries
	if retryConfig.RetryOn429 && maxRetries <= 0 {
		maxRetries = 3
	}

	rl := &RateLimitedLLM{
		wrapped:    wrapped,
		tpmLimit:   rateLimitConfig.TPM,
		rpmLimit:   rateLimitConfig.RPM,
		retryOn429: retryConfig.RetryOn429,
		maxRetries: maxRetries,
		modelName:  modelName,
	}

	// Create TPM limiter if configured (proactive rate limiting)
	// Rate is tokens per second, burst is the full minute's worth
	if rateLimitConfig.TPM > 0 {
		tokensPerSecond := float64(rateLimitConfig.TPM) / 60.0
		rl.tpmLimiter = rate.NewLimiter(rate.Limit(tokensPerSecond), rateLimitConfig.TPM)
		logger.Logger.Info("Rate limiter configured", "type", "TPM", "limit", rateLimitConfig.TPM, "tokens_per_second", tokensPerSecond)
	}

	// Create RPM limiter if configured (proactive rate limiting)
	// Rate is requests per second, burst is the full minute's worth
	if rateLimitConfig.RPM > 0 {
		requestsPerSecond := float64(rateLimitConfig.RPM) / 60.0
		rl.rpmLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), rateLimitConfig.RPM)
		logger.Logger.Info("Rate limiter configured", "type", "RPM", "limit", rateLimitConfig.RPM, "requests_per_second", requestsPerSecond)
	}

	// Log 429 retry configuration if enabled
	if retryConfig.RetryOn429 {
		logger.Logger.Info("429 retry handling enabled", "max_retries", maxRetries)
	}

	return rl
}

// SetRetryAfterProvider sets the provider for Retry-After header values.
// This should be called after construction if using a custom HTTP client that captures headers.
func (rl *RateLimitedLLM) SetRetryAfterProvider(provider RetryAfterProvider) {
	rl.retryAfterProvider = provider
	if provider != nil {
		logger.Logger.Debug("Retry-After provider configured for HTTP header extraction")
	}
}

// GenerateContent implements llms.Model interface with rate limiting and retry logic
func (rl *RateLimitedLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// Wait for RPM limit (one request) - track throttle time
	if rl.rpmLimiter != nil {
		logger.Logger.Debug("Waiting for RPM rate limit")
		throttleStart := time.Now()
		if err := rl.rpmLimiter.Wait(ctx); err != nil {
			return nil, err
		}
		throttleTime := time.Since(throttleStart)
		if throttleTime > 10*time.Millisecond { // Only count significant waits
			rl.recordThrottle(throttleTime)
		}
	}

	// Estimate tokens before the call
	baseEstimatedTokens := rl.estimateInputTokens(messages)
	calibratedTokens := rl.applyCalibration(baseEstimatedTokens)

	if rl.tpmLimiter != nil && calibratedTokens > 0 {
		// Proactive rate limiting: Wait if we would exceed the TPM limit.
		// NOTE: This is BEST-EFFORT based on token estimates, not guaranteed.
		// Actual API token consumption may differ, so 429 errors can still occur.
		// See RateLimitedLLM type comment for details on why this is best-effort.
		logger.Logger.Debug("Waiting for TPM rate limit",
			"base_estimated_tokens", baseEstimatedTokens,
			"calibrated_tokens", calibratedTokens,
			"calibration_ratio", rl.getCalibrationRatio())
		throttleStart := time.Now()
		if err := rl.tpmLimiter.WaitN(ctx, calibratedTokens); err != nil {
			return nil, err
		}
		throttleTime := time.Since(throttleStart)
		if throttleTime > 10*time.Millisecond { // Only count significant waits
			rl.recordThrottle(throttleTime)
		}
	}

	// Make the API call
	start := time.Now()
	response, err := rl.wrapped.GenerateContent(ctx, messages, options...)
	elapsed := time.Since(start)

	if err == nil {
		// Success - adjust token limiter and return
		if response != nil && rl.tpmLimiter != nil {
			actualTokens := rl.getActualTokens(response)
			rl.updateCalibration(baseEstimatedTokens, actualTokens)
			if actualTokens > calibratedTokens {
				additional := actualTokens - calibratedTokens
				reservation := rl.tpmLimiter.ReserveN(time.Now(), additional)
				if reservation.OK() {
					logger.Logger.Debug("Reserved additional tokens",
						"base_estimated", baseEstimatedTokens,
						"calibrated", calibratedTokens,
						"actual", actualTokens,
						"additional", additional,
						"delay", reservation.Delay())
				}
			}
			logger.Logger.Debug("Request completed",
				"base_estimated_tokens", baseEstimatedTokens,
				"calibrated_tokens", calibratedTokens,
				"actual_tokens", actualTokens,
				"calibration_ratio", rl.getCalibrationRatio(),
				"duration", elapsed)
		}
		return response, nil
	}

	// Check if this is a 429 error and if retry is enabled
	if !rl.retryOn429 || !rl.isRateLimitError(err) {
		// Track 429 hit even if retry is disabled
		if rl.isRateLimitError(err) {
			rl.recordRateLimitHit()
		}
		// 429 retry not enabled or not a rate limit error - return immediately
		return nil, err
	}

	// 429 retry handling is enabled - record the hit and attempt retries
	rl.recordRateLimitHit()

	backoff := defaultInitialBackoff
	for attempt := 1; attempt <= rl.maxRetries; attempt++ {
		// Extract retry-after duration from error message
		retryAfter := rl.extractRetryAfter(err)
		if retryAfter > 0 {
			backoff = retryAfter
		}

		// Cap backoff at max
		if backoff > defaultMaxBackoff {
			backoff = defaultMaxBackoff
		}

		logger.Logger.Warn("429 rate limit hit, retrying",
			"attempt", attempt,
			"max_retries", rl.maxRetries,
			"wait_seconds", backoff.Seconds(),
			"error", err.Error())

		// Wait before retry - track retry wait time
		retryWaitStart := time.Now()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		retryWaitTime := time.Since(retryWaitStart)
		rl.recordRetry(retryWaitTime)

		// Retry the request
		response, err = rl.wrapped.GenerateContent(ctx, messages, options...)
		if err == nil {
			logger.Logger.Info("Request succeeded after 429 retry", "attempt", attempt)
			rl.recordRetrySuccess()
			return response, nil
		}

		// If not a rate limit error anymore, return immediately
		if !rl.isRateLimitError(err) {
			return nil, err
		}

		// Record another 429 hit
		rl.recordRateLimitHit()

		// Exponential backoff for next attempt (if retry-after wasn't specified)
		if retryAfter == 0 {
			backoff *= 2
		}
	}

	// All retries exhausted
	logger.Logger.Error("429 retries exhausted", "max_retries", rl.maxRetries, "error", err.Error())
	return nil, err
}

// Stats tracking methods
func (rl *RateLimitedLLM) recordThrottle(waitTime time.Duration) {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.ThrottleCount++
	rl.stats.ThrottleWaitTime += waitTime
	logger.Logger.Debug("Throttle recorded", "count", rl.stats.ThrottleCount, "wait_time", waitTime)
}

func (rl *RateLimitedLLM) recordRateLimitHit() {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.RateLimitHits++
	logger.Logger.Debug("429 hit recorded", "total_hits", rl.stats.RateLimitHits)
}

func (rl *RateLimitedLLM) recordRetry(waitTime time.Duration) {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.RetryCount++
	rl.stats.RetryWaitTime += waitTime
}

func (rl *RateLimitedLLM) recordRetrySuccess() {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.RetrySuccessCount++
}

// GetStats returns a copy of the current rate limit statistics
func (rl *RateLimitedLLM) GetStats() model.RateLimitStats {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	return model.RateLimitStats{
		ThrottleCount:      rl.stats.ThrottleCount,
		ThrottleWaitTimeMs: rl.stats.ThrottleWaitTime.Milliseconds(),
		RateLimitHits:      rl.stats.RateLimitHits,
		RetryCount:         rl.stats.RetryCount,
		RetryWaitTimeMs:    rl.stats.RetryWaitTime.Milliseconds(),
		RetrySuccessCount:  rl.stats.RetrySuccessCount,
	}
}

// ResetStats clears all statistics
func (rl *RateLimitedLLM) ResetStats() {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.ThrottleCount = 0
	rl.stats.ThrottleWaitTime = 0
	rl.stats.RateLimitHits = 0
	rl.stats.RetryCount = 0
	rl.stats.RetryWaitTime = 0
	rl.stats.RetrySuccessCount = 0
}

// RateLimitStatsProvider is an interface for LLMs that can provide rate limit statistics
type RateLimitStatsProvider interface {
	GetStats() model.RateLimitStats
	ResetStats()
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

// extractRetryAfter extracts the retry-after duration from multiple sources:
// 1. HTTP Retry-After header (via RetryAfterProvider) - most reliable
// 2. Error message text parsing (fallback for providers that include it in errors)
// Azure OpenAI errors typically contain "Please retry after X seconds"
func (rl *RateLimitedLLM) extractRetryAfter(err error) time.Duration {
	// First, try to get the value from HTTP Retry-After header
	// This is the most reliable source as it comes directly from the server
	if rl.retryAfterProvider != nil {
		if duration, capturedAt := rl.retryAfterProvider.GetLastRetryAfter(); duration > 0 {
			// Only use if captured recently (within last 5 seconds) to ensure it's from this request
			if time.Since(capturedAt) < 5*time.Second {
				logger.Logger.Debug("Using Retry-After from HTTP header",
					"duration_seconds", duration.Seconds(),
					"captured_ago_ms", time.Since(capturedAt).Milliseconds())
				// Clear the value so it's not reused for subsequent requests
				rl.retryAfterProvider.ClearRetryAfter()
				// Add a buffer to ensure we're past the rate limit window
				// Azure OpenAI token buckets refill gradually, so extra buffer helps avoid immediate re-throttling
				return duration + 10*time.Second
			}
		}
	}

	// Fallback: parse from error message text
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
			logger.Logger.Debug("Using Retry-After from error message", "seconds", seconds)
			// Add a buffer to ensure we're past the rate limit window
			return time.Duration(seconds)*time.Second + 10*time.Second
		}
	}

	return 0
}

// estimateInputTokens provides accurate token estimation using tiktoken
// Falls back to simple heuristic if tokenizer is unavailable
func (rl *RateLimitedLLM) estimateInputTokens(messages []llms.MessageContent) int {
	// Try accurate tokenization first if model name is available
	if rl.modelName != "" {
		if tokens := rl.estimateInputTokensAccurate(messages); tokens > 0 {
			return tokens
		}
	}
	// Fallback to simple heuristic
	return rl.estimateInputTokensSimple(messages)
}

func (rl *RateLimitedLLM) applyCalibration(estimated int) int {
	if estimated <= 0 {
		return estimated
	}

	ratio := rl.getCalibrationRatio()
	if ratio <= 1.0 {
		return estimated
	}

	adjusted := int(math.Ceil(float64(estimated) * ratio))
	if adjusted < estimated {
		return estimated
	}

	return adjusted
}

func (rl *RateLimitedLLM) getCalibrationRatio() float64 {
	rl.calibrationMu.Lock()
	defer rl.calibrationMu.Unlock()

	if !rl.calibrationInitialized {
		return 1.0
	}

	return rl.calibrationRatio
}

func (rl *RateLimitedLLM) updateCalibration(estimated, actual int) {
	if estimated <= 0 || actual <= 0 {
		return
	}

	ratio := float64(actual) / float64(estimated)
	// Keep calibration conservative and bounded
	if ratio < 1.0 {
		ratio = 1.0
	}
	if ratio > 5.0 {
		ratio = 5.0
	}

	rl.calibrationMu.Lock()
	if !rl.calibrationInitialized {
		rl.calibrationRatio = ratio
		rl.calibrationInitialized = true
	} else {
		// Exponential moving average to smooth spikes
		alpha := 0.2
		rl.calibrationRatio = (1.0-alpha)*rl.calibrationRatio + alpha*ratio
	}
	currentRatio := rl.calibrationRatio
	rl.calibrationMu.Unlock()

	logger.Logger.Debug("Updated token calibration",
		"estimated_tokens", estimated,
		"actual_tokens", actual,
		"observed_ratio", ratio,
		"calibrated_ratio", currentRatio)
}

// estimateInputTokensSimple provides a rough estimate using the heuristic of ~4 characters per token
func (rl *RateLimitedLLM) estimateInputTokensSimple(messages []llms.MessageContent) int {
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

// estimateInputTokensAccurate provides accurate token estimation using tiktoken
// Returns 0 if tokenization fails (caller should fall back to simple estimation)
func (rl *RateLimitedLLM) estimateInputTokensAccurate(messages []llms.MessageContent) int {
	// Get the tokenizer for this model
	tkm, err := tiktoken.EncodingForModel(rl.modelName)
	if err != nil {
		logger.Logger.Debug("Tiktoken encoding not available for model, falling back to simple estimation",
			"model", rl.modelName,
			"error", err.Error())
		return 0
	}

	// Count input tokens
	inputTokens := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if textPart, ok := part.(llms.TextContent); ok {
				tokens := tkm.Encode(textPart.Text, nil, nil)
				inputTokens += len(tokens)
			}
		}
	}

	// Estimate completion tokens based on input
	// Conservative estimate: assume completion is 50% of input length
	// This is better than ignoring completion entirely
	estimatedCompletion := inputTokens / 2

	// Add a conservative safety margin (50%) to account for:
	// - Tokenization overhead and message formatting
	// - Function schemas and system prompts
	// - Azure's token counting differences vs. tiktoken
	// - Tool use tokens and additional processing
	// A 50% margin is aggressive but necessary to prevent 429 errors in practice
	totalEstimate := inputTokens + estimatedCompletion
	safetyMargin := totalEstimate / 2 // 50%
	finalEstimate := totalEstimate + safetyMargin

	logger.Logger.Debug("Accurate token estimation",
		"model", rl.modelName,
		"input_tokens", inputTokens,
		"estimated_completion", estimatedCompletion,
		"safety_margin_percent", 50,
		"safety_margin_tokens", safetyMargin,
		"total_estimate", finalEstimate)

	return finalEstimate
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

	// Try to get total tokens
	if v := extractInt(choice.GenerationInfo["TotalTokens"]); v > 0 {
		return v
	}
	if v := extractInt(choice.GenerationInfo["total_tokens"]); v > 0 {
		return v
	}

	// Try prompt + completion tokens
	promptTokens := extractInt(choice.GenerationInfo["PromptTokens"])
	completionTokens := extractInt(choice.GenerationInfo["CompletionTokens"])
	if promptTokens > 0 || completionTokens > 0 {
		return promptTokens + completionTokens
	}

	// Try provider variants
	promptTokens = extractInt(choice.GenerationInfo["prompt_tokens"])
	completionTokens = extractInt(choice.GenerationInfo["completion_tokens"])
	if promptTokens > 0 || completionTokens > 0 {
		return promptTokens + completionTokens
	}

	inputTokens := extractInt(choice.GenerationInfo["input_tokens"])
	outputTokens := extractInt(choice.GenerationInfo["output_tokens"])
	if inputTokens > 0 || outputTokens > 0 {
		return inputTokens + outputTokens
	}

	return 0
}

func extractInt(v any) int {
	if v == nil {
		return 0
	}

	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
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

// HasRateLimiting returns true if any proactive rate limiting is configured
func HasRateLimiting(config model.RateLimitConfig) bool {
	return config.TPM > 0 || config.RPM > 0
}

// HasRetryOn429 returns true if 429 retry handling is enabled
func HasRetryOn429(config model.RetryConfig) bool {
	return config.RetryOn429
}

// NeedsLLMWrapper returns true if the LLM needs to be wrapped for rate limiting or retry handling
func NeedsLLMWrapper(rateLimits model.RateLimitConfig, retry model.RetryConfig) bool {
	return HasRateLimiting(rateLimits) || HasRetryOn429(retry)
}
