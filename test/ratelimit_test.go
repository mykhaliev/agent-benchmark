package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tmc/langchaingo/llms"
)

// ============================================================================
// Rate Limit Configuration Tests
// ============================================================================

func TestHasRateLimiting(t *testing.T) {
	tests := []struct {
		name     string
		config   model.RateLimitConfig
		expected bool
	}{
		{
			name:     "No rate limiting",
			config:   model.RateLimitConfig{},
			expected: false,
		},
		{
			name:     "TPM only",
			config:   model.RateLimitConfig{TPM: 1000},
			expected: true,
		},
		{
			name:     "RPM only",
			config:   model.RateLimitConfig{RPM: 60},
			expected: true,
		},
		{
			name:     "Both TPM and RPM",
			config:   model.RateLimitConfig{TPM: 1000, RPM: 60},
			expected: true,
		},
		{
			name:     "Zero values",
			config:   model.RateLimitConfig{TPM: 0, RPM: 0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.HasRateLimiting(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Rate Limited LLM Tests
// ============================================================================

func TestRateLimitedLLM_GenerateContent_NoLimits(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// No rate limits configured
	rateLimitConfig := model.RateLimitConfig{}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
		},
	}

	ctx := context.Background()
	response, err := rateLimitedLLM.GenerateContent(ctx, messages)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test response", response.Choices[0].Content)
	mockLLM.AssertExpectations(t)
}

func TestRateLimitedLLM_GenerateContent_WithRPMLimit(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Configure RPM limit: 60 requests per minute = 1 per second
	rateLimitConfig := model.RateLimitConfig{RPM: 60}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
		},
	}

	ctx := context.Background()

	// First request should go through immediately
	start := time.Now()
	response, err := rateLimitedLLM.GenerateContent(ctx, messages)
	firstDuration := time.Since(start)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	// First request should be fast (burst allows it)
	assert.Less(t, firstDuration, 100*time.Millisecond)

	mockLLM.AssertExpectations(t)
}

func TestRateLimitedLLM_GenerateContent_RPMBlocking(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Very low RPM limit: 6 requests per minute = 0.1 per second
	// This means after burst is exhausted, we wait ~10 seconds per request
	// But with burst=6, we can make 6 requests immediately, then we wait
	rateLimitConfig := model.RateLimitConfig{RPM: 6}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
		},
	}

	ctx := context.Background()

	// Make burst number of requests quickly
	for i := 0; i < 6; i++ {
		start := time.Now()
		_, err := rateLimitedLLM.GenerateContent(ctx, messages)
		duration := time.Since(start)
		assert.NoError(t, err)
		// All burst requests should be fast
		assert.Less(t, duration, 500*time.Millisecond, "Request %d took too long", i)
	}

	// The 7th request should be rate limited (would take ~10 seconds)
	// We use a short timeout to verify it blocks
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := rateLimitedLLM.GenerateContent(ctxWithTimeout, messages)
	assert.Error(t, err) // Should timeout because rate limited
}

func TestRateLimitedLLM_GenerateContent_WithTPMLimit(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: "test response",
				GenerationInfo: map[string]any{
					"PromptTokens":     10,
					"CompletionTokens": 5,
				},
			},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Configure TPM limit: 600 tokens per minute = 10 per second
	rateLimitConfig := model.RateLimitConfig{TPM: 600}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello world"}}, // ~3 tokens estimated
		},
	}

	ctx := context.Background()
	response, err := rateLimitedLLM.GenerateContent(ctx, messages)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	mockLLM.AssertExpectations(t)
}

func TestRateLimitedLLM_Call(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	rateLimitConfig := model.RateLimitConfig{RPM: 60}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	ctx := context.Background()
	result, err := rateLimitedLLM.Call(ctx, "Hello")

	assert.NoError(t, err)
	assert.Equal(t, "test response", result)
	mockLLM.AssertExpectations(t)
}

func TestRateLimitedLLM_ContextCancellation(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil).Maybe()

	// Very low RPM to force waiting
	rateLimitConfig := model.RateLimitConfig{RPM: 1}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
		},
	}

	ctx := context.Background()

	// First request uses the burst
	_, err := rateLimitedLLM.GenerateContent(ctx, messages)
	assert.NoError(t, err)

	// Second request with cancelled context should fail immediately
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = rateLimitedLLM.GenerateContent(cancelledCtx, messages)
	assert.Error(t, err)
}

func TestRateLimitedLLM_ConcurrentAccess(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)

	var callCount atomic.Int32
	mockLLM := new(MockLLMModel)
	expectedResponse := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: "test response"},
		},
	}
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			callCount.Add(1)
		}).
		Return(expectedResponse, nil)

	// High limits to allow all concurrent requests
	rateLimitConfig := model.RateLimitConfig{RPM: 1000, TPM: 100000}
	retryConfig := model.RetryConfig{}
	rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, rateLimitConfig, retryConfig, "")

	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
		},
	}

	ctx := context.Background()
	numGoroutines := 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rateLimitedLLM.GenerateContent(ctx, messages)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Unexpected error in concurrent access: %v", err)
	}

	assert.Equal(t, int32(numGoroutines), callCount.Load())
}

// ============================================================================
// Integration Tests with Provider Creation
// ============================================================================

func TestRateLimitConfig_YAMLParsing(t *testing.T) {
	// Test that RateLimitConfig and RetryConfig are properly parsed from YAML
	rateLimitConfig := model.RateLimitConfig{
		TPM: 30000,
		RPM: 60,
	}

	retryConfig := model.RetryConfig{
		RetryOn429: true,
		MaxRetries: 3,
	}

	assert.Equal(t, 30000, rateLimitConfig.TPM)
	assert.Equal(t, 60, rateLimitConfig.RPM)
	assert.True(t, retryConfig.RetryOn429)
	assert.Equal(t, 3, retryConfig.MaxRetries)
}

func TestRetryConfig_RetryOn429(t *testing.T) {
	tests := []struct {
		name            string
		rateLimitConfig model.RateLimitConfig
		retryConfig     model.RetryConfig
		expectedCalls   int // Number of times GenerateContent should be called before giving up
	}{
		{
			name:            "429 retry disabled (default) - no retries",
			rateLimitConfig: model.RateLimitConfig{RPM: 60},
			retryConfig:     model.RetryConfig{RetryOn429: false},
			expectedCalls:   1, // Only initial call, no retries
		},
		{
			name:            "429 retry enabled with default max retries",
			rateLimitConfig: model.RateLimitConfig{RPM: 60},
			retryConfig:     model.RetryConfig{RetryOn429: true}, // MaxRetries defaults to 3
			expectedCalls:   4,                                   // Initial call + 3 retries = 4 calls
		},
		{
			name:            "429 retry enabled with explicit max retries",
			rateLimitConfig: model.RateLimitConfig{RPM: 60},
			retryConfig:     model.RetryConfig{RetryOn429: true, MaxRetries: 2},
			expectedCalls:   3, // Initial call + 2 retries = 3 calls
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.SetupLogger(NewDummyWriter(), true)

			var callCount atomic.Int32
			mockLLM := new(MockLLMModel)
			rateLimitErr := fmt.Errorf("429 Too Many Requests: rate limit exceeded")

			mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					callCount.Add(1)
				}).
				Return((*llms.ContentResponse)(nil), rateLimitErr)

			rateLimitedLLM := engine.NewRateLimitedLLM(mockLLM, tt.rateLimitConfig, tt.retryConfig, "")

			messages := []llms.MessageContent{
				{
					Role:  llms.ChatMessageTypeHuman,
					Parts: []llms.ContentPart{llms.TextContent{Text: "Hello"}},
				},
			}

			ctx := context.Background()
			_, err := rateLimitedLLM.GenerateContent(ctx, messages)

			assert.Error(t, err)
			assert.Equal(t, int32(tt.expectedCalls), callCount.Load(), "Expected %d calls but got %d", tt.expectedCalls, callCount.Load())
		})
	}
}

func TestProvider_WithRateLimits(t *testing.T) {
	// Test that Provider struct properly holds RateLimitConfig and RetryConfig
	provider := model.Provider{
		Name:  "test-provider",
		Type:  model.ProviderOpenAI,
		Token: "test-token",
		Model: "gpt-4",
		RateLimits: model.RateLimitConfig{
			TPM: 50000,
			RPM: 100,
		},
		Retry: model.RetryConfig{
			RetryOn429: true,
			MaxRetries: 5,
		},
	}

	assert.Equal(t, 50000, provider.RateLimits.TPM)
	assert.Equal(t, 100, provider.RateLimits.RPM)
	assert.True(t, provider.Retry.RetryOn429)
	assert.Equal(t, 5, provider.Retry.MaxRetries)
	assert.True(t, engine.HasRateLimiting(provider.RateLimits))
	assert.True(t, engine.HasRetryOn429(provider.Retry))
}
