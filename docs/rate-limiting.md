# Rate Limiting and Token Estimation

This document provides in-depth technical details about how agent-benchmark handles rate limiting and token estimation across different LLM providers.

## Overview

agent-benchmark provides two complementary mechanisms for managing API rate limits:

1. **Proactive Rate Limiting** - Throttles requests *before* sending them to stay within quota
2. **Reactive 429 Handling** - Retries requests *after* receiving rate limit errors

For best results, use both together as a defense-in-depth strategy.

## Quick Start

```yaml
providers:
  - name: my-provider
    type: AZURE  # or OPENAI, ANTHROPIC, GOOGLE, etc.
    model: gpt-4o
    rate_limits:
      tpm: 30000     # Tokens per minute
      rpm: 60        # Requests per minute
    retry:
      retry_on_429: true
      max_retries: 3
```

## Token Estimation Strategy

### The Challenge

Proactive rate limiting requires knowing how many tokens a request will consume *before* sending it. However:

- Different providers use different tokenizers
- System prompts, function schemas, and message formatting add overhead
- Actual token counts are only known after the API responds

### Our Solution: Tiktoken with Fallback

We use [tiktoken](https://github.com/openai/tiktoken) (via [tiktoken-go](https://github.com/pkoukk/tiktoken-go)) for token estimation, with intelligent fallbacks for non-OpenAI models.

#### Model Family Support

| Provider | Models | Tokenization Method | Estimated Accuracy |
|----------|--------|--------------------|--------------------|
| **OpenAI** | GPT-4, GPT-4o, GPT-3.5-turbo | Native tiktoken encoding | ~95-100% |
| **Azure OpenAI** | GPT-4, GPT-4o, GPT-3.5-turbo | Native tiktoken encoding | ~95-100% |
| **Anthropic** | Claude 3.5, Claude 3, Claude 2 | cl100k_base fallback | ~85-90% |
| **Google** | Gemini 1.5, Gemini Pro | cl100k_base fallback | ~80-85% |
| **Meta** | Llama 3, Llama 2 | cl100k_base fallback | ~80-90% |
| **Mistral** | Mistral, Mixtral | cl100k_base fallback | ~80-90% |
| **Other** | Any model | cl100k_base fallback | ~75-85% |

### Why cl100k_base as Fallback?

`cl100k_base` is the encoding used by GPT-4 and provides a reasonable approximation for other modern LLMs:

1. **Similar Vocabulary Size**: Most modern LLMs use BPE (Byte Pair Encoding) tokenizers with vocabulary sizes in the 30K-100K range

2. **Comparable Token Density**: For typical English text, token density (characters per token) is similar across modern LLMs:
   - GPT-4 (cl100k_base): ~4.0 chars/token
   - Claude 3: ~3.8 chars/token
   - Gemini: ~3.9 chars/token
   - Llama 3: ~3.7 chars/token

3. **Better Than Heuristics**: The naive "4 characters = 1 token" heuristic is ~60-70% accurate. cl100k_base achieves ~80-90% even for non-OpenAI models.

### Calibration Mechanism

To compensate for estimation inaccuracies, we use runtime calibration:

```
calibration_ratio = actual_tokens / estimated_tokens
```

The calibration ratio:
- Starts at 1.0 (no adjustment)
- Updates after each API call using exponential moving average (α=0.2)
- Is bounded between 1.0 and 5.0 to prevent runaway values
- Is tracked per-provider, not globally

**Example calibration in action:**

```
Request 1: Estimated 100 tokens, Actual 150 → Ratio: 1.5
Request 2: Estimated 200 tokens, Calibrated: 200 × 1.5 = 300
Request 2: Actual 280 → New ratio: 0.8 × 1.5 + 0.2 × 1.4 = 1.48
```

### Safety Margin

In addition to calibration, we apply a **50% safety margin** to all estimates:

```
final_estimate = (input_tokens + estimated_completion) × 1.5
```

This accounts for:
- Tool/function schemas not included in message content
- Message formatting overhead
- System prompt processing
- Provider-specific counting differences

## Configuration Reference

### Rate Limits

```yaml
rate_limits:
  tpm: 30000  # Tokens per minute (optional)
  rpm: 60     # Requests per minute (optional)
```

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `tpm` | integer | Maximum tokens per minute | No limit |
| `rpm` | integer | Maximum requests per minute | No limit |

**How it works:**
- Uses token bucket algorithm for smooth rate limiting
- Rate is calculated as `limit / 60` per second
- Burst capacity equals the full minute's allocation
- Requests wait if bucket is depleted

### Retry Configuration

```yaml
retry:
  retry_on_429: true   # Enable 429 retry (optional)
  max_retries: 3       # Max retry attempts (optional)
```

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `retry_on_429` | boolean | Enable automatic retry on 429 errors | `false` |
| `max_retries` | integer | Number of retry attempts | 3 (when enabled) |

**How it works:**
1. On 429 error, extracts wait duration from:
   - HTTP `Retry-After` header (preferred)
   - Error message text (fallback, e.g., "retry after 30 seconds")
2. Adds 10-second buffer to ensure rate limit window has passed
3. Uses exponential backoff if no Retry-After specified
4. Retries up to `max_retries` times

## Why Best-Effort?

Rate limiting is **best-effort, not guaranteed**. You may still encounter 429 errors because:

1. **Estimation Inaccuracy**: Even with tiktoken + calibration, estimates can be 10-20% off

2. **Async Accounting**: Token consumption is only known after the request completes; throttling decisions are made beforehand

3. **Multiple Factors**: Both TPM and RPM limits apply; we prioritize TPM but RPM can also trigger 429s

4. **Shared Quotas**: Your API quota may be shared across multiple applications or users

5. **Provider Variability**: Azure/OpenAI/etc. may count tokens differently than tiktoken

**This is why we recommend enabling both rate limiting AND 429 retry handling.**

## Monitoring and Debugging

### Statistics

Each provider tracks rate limiting statistics:

| Metric | Description |
|--------|-------------|
| `throttle_count` | Times request was proactively throttled |
| `throttle_wait_time_ms` | Total time spent waiting due to throttling |
| `rate_limit_hits` | Number of 429 errors received |
| `retry_count` | Number of retry attempts made |
| `retry_wait_time_ms` | Total time spent waiting for retries |
| `retry_success_count` | Retries that ultimately succeeded |

### Debug Logging

Enable verbose logging to see token estimation details:

```yaml
settings:
  verbose: true
```

This logs:
- Estimated vs actual token counts
- Calibration ratio updates
- Throttle wait times
- 429 retry attempts

### Tuning Your Rate Limits

1. **Start conservative**: Set TPM/RPM 20% below your actual quota
2. **Monitor `rate_limit_hits`**: If > 0, your estimates are too low
3. **Check `throttle_count`**: If very high, you may be over-throttling
4. **Adjust based on model**: Non-OpenAI models may need lower limits due to estimation variance

## Provider-Specific Notes

### Azure OpenAI
- Uses same tokenization as OpenAI (native support)
- Quota is per-deployment, not per-resource
- Consider `429` errors from both TPM and RPM limits

### Anthropic (Claude)
- Uses custom BPE tokenizer, ~15% different from cl100k_base
- Calibration typically converges after 3-5 requests
- Claude 3.5 Sonnet has higher throughput than Opus

### Google (Gemini)
- Uses SentencePiece tokenizer
- Token counts may be reported differently in usage metadata
- Calibration is especially helpful here

### Groq
- Very high rate limits but strict enforcement
- Uses Llama tokenization (close to cl100k_base)
- RPM limits often more restrictive than TPM

## Technical Implementation

### Token Bucket Algorithm

We use Go's `golang.org/x/time/rate` limiter:

```go
// TPM: Allow 'tpm/60' tokens per second, burst of 'tpm'
tpmLimiter = rate.NewLimiter(rate.Limit(tpm/60.0), tpm)

// RPM: Allow 'rpm/60' requests per second, burst of 'rpm'
rpmLimiter = rate.NewLimiter(rate.Limit(rpm/60.0), rpm)
```

### Retry-After Extraction

Priority order for determining wait time:
1. HTTP `Retry-After` header (seconds or HTTP-date format)
2. Error message parsing (regex: `retry after (\d+) seconds?`)
3. Exponential backoff starting at 1 second

### Code Location

- Rate limiting: [engine/ratelimit.go](../engine/ratelimit.go)
- HTTP client with header capture: [engine/httpclient.go](../engine/httpclient.go)
- Tests: [test/ratelimit_test.go](../test/ratelimit_test.go)

## FAQ

### Q: Why not use provider-specific tokenizers?

**A:** We evaluated this approach but found significant challenges:
- **Anthropic**: No official Go tokenizer library
- **Google**: SentencePiece requires C bindings, complicating builds
- **Llama/Mistral**: Multiple tokenizer versions exist

The cl100k_base fallback with calibration achieves ~85% accuracy with zero additional dependencies.

### Q: Can I disable token estimation entirely?

**A:** Yes, simply don't configure `rate_limits`. You can still use `retry_on_429` for reactive handling only.

### Q: Why is my estimate so different from actual?

**A:** Common causes:
- Large system prompts not included in message content
- Function/tool schemas consuming tokens
- Multi-turn conversations with accumulated context
- Provider-specific formatting overhead

The calibration mechanism should adapt after a few requests.

### Q: Should I set both TPM and RPM?

**A:** Generally yes. TPM is usually the primary constraint for LLMs, but RPM can matter for:
- Very short requests (few tokens each)
- Streaming responses
- Providers with strict concurrent request limits
