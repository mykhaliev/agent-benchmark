# Assertions Reference

agent-benchmark provides 20+ assertion types to validate agent behavior.

## Tool Assertions

### tool_called
Verify a tool was invoked:
```yaml
- type: tool_called
  tool: write_file
```

### tool_not_called
Ensure a tool was NOT invoked:
```yaml
- type: tool_not_called
  tool: delete_database
```

### tool_call_count
Validate exact number of calls:
```yaml
- type: tool_call_count
  tool: search_api
  count: 3
```

### tool_call_order
Verify tools called in sequence:
```yaml
- type: tool_call_order
  sequence:
    - validate_input
    - process_data
    - save_results
```

### tool_param_equals
Check parameters match exactly:
```yaml
- type: tool_param_equals
  tool: create_user
  params:
    name: "John Doe"
    config.timeout: "30"  # Nested with dot notation
```

### tool_param_matches_regex
Validate parameters with regex:
```yaml
- type: tool_param_matches_regex
  tool: send_email
  params:
    recipient: "^[a-zA-Z0-9._%+-]+@example\\.com$"
```

### tool_result_matches_json
Validate results with JSONPath:
```yaml
- type: tool_result_matches_json
  tool: get_user
  path: "$.data.user.name"
  value: "John Doe"
```

### no_hallucinated_tools
Verify agent only uses available tools:
```yaml
- type: no_hallucinated_tools
```

## Output Assertions

### output_contains
Check output contains text:
```yaml
- type: output_contains
  value: "successfully"
```

### output_not_contains
Ensure output doesn't contain text:
```yaml
- type: output_not_contains
  value: "error"
```

### output_regex
Validate output with regex:
```yaml
- type: output_regex
  pattern: "(?i)(success|completed|done)"
```

## Performance Assertions

### max_tokens
Limit token usage:
```yaml
- type: max_tokens
  value: 1000
```

### max_latency_ms
Ensure completion within time:
```yaml
- type: max_latency_ms
  value: 5000
```

## Quality Assertions

### no_error_messages
No errors during execution:
```yaml
- type: no_error_messages
```

### no_clarification_questions
Agent executed without asking for confirmation:
```yaml
- type: no_clarification_questions
```

### no_rate_limit_errors
No 429 errors encountered:
```yaml
- type: no_rate_limit_errors
```

## Boolean Combinators

### anyOf (OR logic)
Pass if ANY child passes:
```yaml
- anyOf:
    - type: tool_called
      tool: keyboard_control
    - type: tool_called
      tool: ui_automation
```

### allOf (AND logic)
Pass if ALL children pass:
```yaml
- allOf:
    - type: tool_called
      tool: create_file
    - type: output_contains
      value: "created"
```

### not (negation)
Pass if child FAILS:
```yaml
- not:
    type: output_contains
    value: "error"
```

### Nested Combinators
```yaml
# (tool_a OR tool_b) AND no errors
- allOf:
    - anyOf:
        - type: tool_called
          tool: tool_a
        - type: tool_called
          tool: tool_b
    - type: no_error_messages
```

## Common Patterns

### Flexible Tool Usage
When LLMs may use different tools for same outcome:
```yaml
- anyOf:
    - type: tool_call_order
      sequence: [app, keyboard_control]
    - type: tool_call_order
      sequence: [app, ui_type]
```

### Verify Output Contains Evidence
```yaml
- type: output_regex
  pattern: "(?i)(hello\\s*world|text was typed)"
```

### Safety Checks
```yaml
- type: output_not_contains
  value: "failed"
- type: no_error_messages
- type: no_hallucinated_tools
```
