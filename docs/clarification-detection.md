# Clarification Detection

## Overview

Clarification detection identifies when an LLM asks for user confirmation or clarification instead of taking action directly. This is important for benchmarking agent behavior - an effective agent should execute tasks without asking unnecessary questions.

## How It Works

When enabled, the benchmark uses a **separate LLM (the "judge")** to semantically classify each agent response. This approach is more accurate than pattern matching because:

- It understands context and nuance
- It works in any language
- It handles creative phrasings that wouldn't match static patterns
- It can distinguish between genuine clarification requests and similar-sounding statements

## Configuration

Add the `clarification_detection` block to your agent configuration:

```yaml
agents:
  - name: my-agent
    provider: azure-openai-gpt4
    clarification_detection:
      enabled: true
      level: warning
      judge_provider: $self  # or specify a separate provider
```

### Configuration Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | bool | No | `false` | Enable clarification detection |
| `level` | string | No | `warning` | Log level: `info`, `warning`, or `error` |
| `judge_provider` | string | Yes (when enabled) | - | Provider name for the judge LLM, or `$self` to use the agent's own provider |

### Log Levels

- **`info`**: Log clarification detections but don't count them as errors
- **`warning`** (default): Log and add to test errors (test can still pass)
- **`error`**: Log and add to test errors

## Using `$self` vs Separate Provider

### Option 1: Reuse Agent's Provider (`$self`)

The simplest configuration - uses the same LLM for both the agent and the judge:

```yaml
agents:
  - name: my-agent
    provider: azure-openai-gpt4
    clarification_detection:
      enabled: true
      judge_provider: $self
```

**Pros:**
- Simple configuration
- No extra provider setup needed

**Cons:**
- Higher cost (uses expensive model for simple classification)
- Slightly slower

### Option 2: Separate Judge Provider (Recommended)

Use a fast/cheap model specifically for classification:

```yaml
providers:
  - name: azure-openai-gpt4
    type: AZURE
    baseUrl: https://your-resource.openai.azure.com
    auth_type: entra_id
    model: gpt-4
    version: "2024-02-15-preview"

  - name: azure-openai-nano
    type: AZURE
    baseUrl: https://your-resource.openai.azure.com
    auth_type: entra_id
    model: gpt-4.1-nano
    version: "2024-02-01"

agents:
  - name: my-agent
    provider: azure-openai-gpt4
    clarification_detection:
      enabled: true
      judge_provider: azure-openai-nano
```

**Pros:**
- Much cheaper (~100x less cost with gpt-4o-mini)
- Faster classification
- Doesn't affect agent's token limits

**Cons:**
- Requires setting up an additional provider

## Recommended Judge Models by Provider

| Provider | Recommended Judge Model | Input/Output per 1M tokens |
|----------|------------------------|----------------------------|
| Azure OpenAI | `gpt-4.1-nano` | $0.10 / $0.40 |
| OpenAI | `gpt-4o-mini` | $0.15 / $0.60 |
| Anthropic | `claude-haiku-4-5` | $1.00 / $5.00 |
| Google | `gemini-2.0-flash-lite` | $0.075 / $0.30 |

## Example: Full Configuration

```yaml
providers:
  - name: azure-gpt4
    type: AZURE
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    auth_type: entra_id
    model: gpt-4
    version: "2024-02-15-preview"

  - name: azure-gpt41-nano
    type: AZURE
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    auth_type: entra_id
    model: gpt-4.1-nano
    version: "2024-02-01"

agents:
  - name: test-agent
    provider: azure-gpt4
    system_prompt: |
      You are a helpful assistant. Execute tasks directly without asking for confirmation.
    clarification_detection:
      enabled: true
      level: warning
      judge_provider: azure-gpt41-nano

sessions:
  - name: test-session
    tests:
      - name: create-file-test
        prompt: "Create a file called hello.txt with the content 'Hello, World!'"
        assertions:
          - type: tool_called
            tool: write_file
```

## How Classification Works

When the agent returns a final response (no tool calls), the judge LLM is called with the following prompt:

```
Is this assistant response asking for user confirmation or clarification before acting?
Reply only YES or NO.
```

The judge classifies the response, and if it returns "YES", the response is recorded as a clarification request.

## Statistics

When clarification requests are detected, statistics are recorded in the test results:

```json
{
  "clarificationStats": {
    "count": 2,
    "iterations": [1, 3],
    "examples": [
      "Would you like me to create the file now?",
      "Should I proceed with the deletion?"
    ]
  }
}
```

## Troubleshooting

### "Clarification detection enabled but judge_provider not specified"

You must specify `judge_provider` when `enabled: true`. Use `$self` to reuse the agent's provider, or specify a separate provider name.

### "Clarification judge provider not found"

The provider name specified in `judge_provider` doesn't exist in your `providers` list. Check for typos and ensure the provider is defined.

### False positives/negatives

The LLM-based classification is highly accurate but not perfect. If you encounter issues:

1. Check the `examples` in `clarificationStats` to see what was detected
2. Consider adjusting your agent's system prompt to be more explicit about not asking for confirmation
3. The judge timeout is 5 seconds - if the judge API is slow, detections may be skipped
