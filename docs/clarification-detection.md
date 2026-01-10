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

Use a capable model specifically for classification. We recommend **gpt-4.1** for accurate classification - smaller models like gpt-4.1-nano may have reduced accuracy on edge cases:

```yaml
providers:
  - name: azure-openai-gpt4
    type: AZURE
    baseUrl: https://your-resource.openai.azure.com
    auth_type: entra_id
    model: gpt-4
    version: "2024-02-15-preview"

  - name: azure-openai-judge
    type: AZURE
    baseUrl: https://your-resource.openai.azure.com
    auth_type: entra_id
    model: gpt-4.1
    version: "2025-01-01-preview"

agents:
  - name: my-agent
    provider: azure-openai-gpt4
    clarification_detection:
      enabled: true
      judge_provider: azure-openai-judge
```

**Pros:**
- Best classification accuracy (100% in testing)
- Handles edge cases correctly (formatted completions, destructive confirmations)
- Doesn't affect agent's token limits

**Cons:**
- Slightly higher cost than nano models (but still minimal for single-call classification)

## Recommended Judge Models

For accurate classification, use capable models. Smaller/cheaper models (like gpt-4.1-nano) may have reduced accuracy on edge cases such as distinguishing formatted completion summaries from clarification requests.

| Provider | Recommended Judge Model | Notes |
|----------|------------------------|-------|
| Azure OpenAI | `gpt-4.1` | Tested - 100% accuracy on 17 scenarios |
| OpenAI | `gpt-4.1` | Same model as Azure, expected same accuracy |
| Anthropic | `claude-sonnet-4` or better | Not tested - use capable model |
| Google | `gemini-2.0-pro` or better | Not tested - avoid smaller flash models |

## Example: Full Configuration

```yaml
providers:
  - name: azure-gpt4
    type: AZURE
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    auth_type: entra_id
    model: gpt-4
    version: "2024-02-15-preview"

  - name: azure-gpt41-judge
    type: AZURE
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    auth_type: entra_id
    model: gpt-4.1
    version: "2025-01-01-preview"

agents:
  - name: test-agent
    provider: azure-gpt4
    system_prompt: |
      You are a helpful assistant. Execute tasks directly without asking for confirmation.
    clarification_detection:
      enabled: true
      level: warning
      judge_provider: azure-gpt41-judge

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
4. **Use gpt-4.1 or equivalent** - smaller models like gpt-4.1-nano may incorrectly classify formatted completion summaries as clarification requests, or miss edge cases like destructive action confirmations
