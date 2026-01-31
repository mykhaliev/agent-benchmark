# Best Practices for Agent Benchmarks

## System Prompts

### Be Explicit About Autonomy
LLMs often ask for confirmation. Prevent this with clear system prompts:

```yaml
agents:
  - name: autonomous-agent
    provider: gpt4
    system_prompt: |
      You are {{AGENT_NAME}}, an autonomous agent.
      
      CRITICAL RULES:
      - Execute all tasks IMMEDIATELY without asking for confirmation
      - NEVER ask "Would you like me to...", "Should I proceed...", or similar
      - NEVER request clarification - make reasonable assumptions and proceed
      - Use available tools directly to complete tasks
      - Report results AFTER completion, not before starting
```

### Enable Clarification Detection
Catch when agents ask questions instead of acting:

```yaml
agents:
  - name: test-agent
    provider: gpt4
    clarification_detection:
      enabled: true
      judge_provider: gpt4  # Use "$self" to use same provider
    # ...
```

Then assert:
```yaml
assertions:
  - type: no_clarification_questions
```

## Flexible Assertions

### Allow Multiple Valid Approaches
LLMs may use different tools to achieve the same goal:

```yaml
assertions:
  # Accept either tool for typing text
  - anyOf:
      - type: tool_called
        tool: keyboard_control
      - type: tool_called
        tool: ui_type
```

### Use Regex for Output Validation
Don't expect exact output - use patterns:

```yaml
assertions:
  - type: output_regex
    pattern: "(?i)(success|completed|done|finished)"
```

## Sessions and State

### Use Sessions for Multi-Step Workflows
Tests in a session share conversation history:

```yaml
sessions:
  - name: User Workflow
    tests:
      - name: Create resource
        prompt: "Create user with email test@example.com"
        extractors:
          - type: jsonpath
            tool: create_user
            path: "$.data.id"
            variable_name: user_id
            
      - name: Verify resource
        prompt: "Get user {{user_id}}"  # Uses extracted ID
```

### Independent Tests in Separate Sessions
```yaml
sessions:
  - name: Create Tests
    tests:
      - name: Test A
        # ...
  - name: Delete Tests  # Fresh context
    tests:
      - name: Test B
        # ...
```

## Timing Configuration

### Server Delays for Slow Startup
```yaml
servers:
  - name: slow-server
    type: stdio
    command: python heavy_server.py
    server_delay: 45s     # Wait for initialization
    process_delay: 2s     # Wait after process starts
```

### Test Delays for Rate Limits
```yaml
settings:
  test_delay: 2s      # Pause between tests
  session_delay: 30s  # Pause between sessions (resource cleanup)
```

### Per-Test Delays
```yaml
tests:
  - name: Rate-limited API
    start_delay: 5s  # Wait before this specific test
    prompt: "..."
```

## Portable Paths

### Use TEST_DIR for Relative Paths
```yaml
variables:
  data_dir: "{{TEST_DIR}}/test-data"
  mcp_server: "{{TEST_DIR}}/bin/server.exe"

servers:
  - name: my-server
    type: stdio
    command: "{{mcp_server}}"
```

### Use TEMP_DIR for Outputs
```yaml
variables:
  output_file: "{{TEMP_DIR}}/agent-benchmark-{{RUN_ID}}.txt"
```

## Safety Assertions

### Always Include Quality Checks
```yaml
assertions:
  # Functional assertions
  - type: tool_called
    tool: expected_tool
    
  # Safety checks
  - type: no_error_messages
  - type: no_hallucinated_tools
  - type: no_clarification_questions
  - type: no_rate_limit_errors
  
  # Negative checks
  - type: output_not_contains
    value: "failed"
  - type: tool_not_called
    tool: dangerous_tool
```

## Iterations and Timeouts

### Set Reasonable Limits
```yaml
settings:
  max_iterations: 10   # Prevent infinite loops
  tool_timeout: 30s    # Fail slow tools
```

The agent loops until:
1. It provides a final answer (no tool calls)
2. `max_iterations` is reached
3. An error occurs

## Multi-Agent Comparison

### Test Same Prompts Across Providers
```yaml
providers:
  - name: gpt4
    type: AZURE
    # ...
  - name: claude
    type: ANTHROPIC
    # ...

agents:
  - name: gpt4-agent
    provider: gpt4
    servers: [{name: my-server}]
    
  - name: claude-agent
    provider: claude
    servers: [{name: my-server}]

# Both agents run all tests - compare in report
```

## Success Criteria

### Set Pass Rate for Suites
```yaml
criteria:
  success_rate: 0.8  # 80% must pass
```

Exit codes:
- `0` - Success rate met
- `1` - Success rate not met
