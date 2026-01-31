# Advanced Features

Optional features for production-ready test configurations.

## System Prompts

Customize agent behavior with system prompts:

```yaml
agents:
  - name: autonomous-agent
    provider: azure-gpt
    system_prompt: |
      You are an autonomous testing agent.
      Execute tasks directly without asking for clarification.
      Never ask "would you like me to..." - just do it.
    servers:
      - name: mcp-server
```

**Template variables in system prompts:**

```yaml
agents:
  - name: test-agent
    provider: azure-gpt
    system_prompt: |
      You are {{AGENT_NAME}} running on {{PROVIDER_NAME}}.
      Current session: {{SESSION_NAME}}.
      Execute all tasks autonomously.
```

| Variable | Description |
|----------|-------------|
| `{{AGENT_NAME}}` | Current agent name |
| `{{SESSION_NAME}}` | Current session name |
| `{{PROVIDER_NAME}}` | Provider being used |

## Agent Skills

Load domain-specific knowledge into agents:

```yaml
agents:
  - name: skilled-agent
    provider: azure-openai
    skill:
      path: "./skills/my-skill"  # Path to skill directory
      file_access: false          # Allow reading references/*.md (default: false)
    servers:
      - name: mcp-server
```

Skill directory structure:
```
my-skill/
├── SKILL.md              # Required: YAML frontmatter + instructions
└── references/           # Optional: additional docs
    └── api.md
```

The `{{SKILL_DIR}}` template variable provides the absolute skill path.

## Combining System Prompt with Skills

When both `skill` and `system_prompt` are specified, they are combined:

```yaml
agents:
  - name: expert-agent
    provider: azure-gpt
    skill:
      path: "./skills/excel-automation"
      file_access: true
    system_prompt: |
      Additional context for this specific test run:
      - Focus on performance optimization
      - Prefer batch operations over individual calls
    servers:
      - name: excel-mcp
```

**Order of injection:**
1. Skill content (from SKILL.md body) is injected first
2. Custom `system_prompt` is appended after

This allows skills to provide domain knowledge while system_prompt adds test-specific instructions.

**Example skill + system_prompt combination:**

```yaml
# The agent receives:
# 1. Full content of skills/excel-automation/SKILL.md body
# 2. Then your custom system_prompt text
# 3. Then the user's test prompt

agents:
  - name: excel-agent
    provider: azure-gpt
    skill:
      path: "{{TEST_DIR}}/skills/excel-automation"
    system_prompt: |
      You are {{AGENT_NAME}}. 
      Execute Excel operations without confirmation.
      Current working directory: {{TEST_DIR}}/test-data
```

## Rate Limiting (RPM/TPM)

Proactively throttle requests to avoid hitting API quotas:

```yaml
providers:
  - name: azure-gpt
    type: AZURE
    token: {{AZURE_API_KEY}}
    model: gpt-4
    baseUrl: https://your-resource.openai.azure.com
    version: 2024-02-15-preview
    rate_limits:
      tpm: 30000    # Tokens per minute
      rpm: 60       # Requests per minute
```

Uses token bucket algorithm to limit request rates before they're sent.

## 429 Retry Handling

By default, 429 errors fail immediately. Enable automatic retry:

```yaml
providers:
  - name: azure-gpt
    type: AZURE
    token: {{AZURE_API_KEY}}
    model: gpt-4
    baseUrl: https://your-resource.openai.azure.com
    version: 2024-02-15-preview
    retry:
      retry_on_429: true    # Enable retry (default: false)
      max_retries: 3        # Retry attempts (default: 3)
```

Extracts wait duration from `Retry-After` header or error message text.

**Assertion to verify no rate limits hit:**
```yaml
assertions:
  - type: no_rate_limit_errors
```

## AI Summary (LLM Analysis)

Generate AI-powered executive summary of test results:

```yaml
ai_summary:
  enabled: true
  judge_provider: azure-gpt  # Provider from your providers section
```

The AI analysis appears in HTML reports with:
- Overall verdict and confidence
- Trade-offs between agents
- Notable observations
- Failure pattern analysis
- Actionable recommendations

## Clarification Detection

Detect when agents ask for clarification instead of acting:

```yaml
agents:
  - name: autonomous-agent
    provider: my-provider
    clarification_detection:
      enabled: true
      judge_provider: azure-gpt  # Recommend gpt-4.1 for accuracy
```

Use with assertion:
```yaml
assertions:
  - type: no_clarification_questions
```

## Session Delay

Allow resource cleanup between sessions (useful for COM, external apps):

```yaml
settings:
  session_delay: 30s  # Pause between sessions
```

## Test Delay

Pause between tests to respect rate limits or allow state to settle:

```yaml
settings:
  test_delay: 2s  # Global delay after each test
```

Or per-test:
```yaml
tests:
  - name: Rate-limited call
    prompt: "Make API request"
    start_delay: 5s  # Wait before this specific test
```

## Built-in Template Variables

Available everywhere (providers, servers, variables, prompts):

| Variable | Description |
|----------|-------------|
| `{{TEST_DIR}}` | Directory containing the test YAML file |
| `{{TEMP_DIR}}` | System temp directory |
| `{{RUN_ID}}` | Unique UUID for this test run |
| `{{SKILL_DIR}}` | Skill directory (when skill loaded) |

Runtime only (prompts, assertions, system_prompt):

| Variable | Description |
|----------|-------------|
| `{{AGENT_NAME}}` | Current agent name |
| `{{SESSION_NAME}}` | Current session name |
| `{{PROVIDER_NAME}}` | Provider name |

## Complete Production Example

```yaml
providers:
  - name: azure-gpt
    type: AZURE
    auth_type: entra_id  # Passwordless auth
    model: gpt-4
    baseUrl: https://my-resource.openai.azure.com
    version: 2024-02-15-preview
    rate_limits:
      tpm: 30000
      rpm: 60
    retry:
      retry_on_429: true
      max_retries: 3

agents:
  - name: test-agent
    provider: azure-gpt
    skill:
      path: "{{TEST_DIR}}/skills/domain-knowledge"
    clarification_detection:
      enabled: true
      judge_provider: azure-gpt
    system_prompt: |
      You are {{AGENT_NAME}}. Execute tasks autonomously.
    servers:
      - name: mcp-server

settings:
  verbose: true
  max_iterations: 10
  tool_timeout: 30s
  test_delay: 2s
  session_delay: 10s

ai_summary:
  enabled: true
  judge_provider: azure-gpt

sessions:
  - name: Production Tests
    tests:
      - name: Critical operation
        prompt: "Execute the critical workflow"
        assertions:
          - type: no_clarification_questions
          - type: no_rate_limit_errors
          - type: no_error_messages
          - type: tool_called
            tool: execute_workflow
```
