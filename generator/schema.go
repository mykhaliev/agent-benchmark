package generator

// sessionSchema documents the YAML structure that must be emitted by the LLM.
const sessionSchema = `
sessions:                        # Required top-level key
  - name: "session-name"         # Human-readable session identifier (string)
    tests:                       # List of test objects
      - name: "test-name"        # Human-readable test identifier (string)
        agent: "agent-name"      # Agent name (must match one of the configured agents)
        prompt: "user prompt"    # The message sent to the agent (string)
        assertions:              # List of assertion objects (see assertion types below)
          - type: tool_called
            tool: tool_name
`

// assertionTypesDoc documents every supported assertion type with required fields.
const assertionTypesDoc = `
ASSERTION TYPES
===============

Tool assertions:
  tool_called          - Asserts a specific tool was called.
                         Required: type, tool (string)
  tool_not_called      - Asserts a specific tool was NOT called.
                         Required: type, tool (string)
  tool_call_count      - Asserts a tool was called exactly N times.
                         Required: type, tool (string), count (int)
  tool_call_order      - Asserts tools were called in this order.
                         Required: type, sequence (list of strings)
  tool_param_equals    - Asserts a tool was called with specific parameters.
                         Required: type, tool (string), params (map[string]string)
  tool_param_matches_regex - Asserts a tool parameter matches a regex.
                         Required: type, tool (string), params (map[string]string - values are regexes)
  tool_result_matches_json - Asserts the tool result matches JSON path/value.
                         Required: type, tool (string), path (string), value (string)

Output assertions:
  output_contains      - Asserts the final output contains a substring.
                         Required: type, value (string)
  output_not_contains  - Asserts the final output does NOT contain a substring.
                         Required: type, value (string)
  output_regex         - Asserts the final output matches a regex.
                         Required: type, pattern (string)

Performance assertions:
  max_tokens           - Asserts total tokens used is at most N.
                         Required: type, count (int)
  max_latency_ms       - Asserts total latency is at most N milliseconds.
                         Required: type, count (int)

Behaviour assertions:
  no_error_messages    - Asserts no error messages occurred during execution.
                         Required: type only
  no_hallucinated_tools - Asserts the agent did not call tools that don't exist.
                         Required: type only
  no_clarification_questions - Asserts the agent did not ask the user for clarification.
                         Required: type only
  no_rate_limit_errors - Asserts no rate limit errors occurred.
                         Required: type only

CLI-specific assertions (only for CLI server type):
  cli_exit_code_equals - Asserts the CLI exit code equals a value.
                         Required: type, expected (int)
  cli_stdout_contains  - Asserts CLI stdout contains a substring.
                         Required: type, value (string)
  cli_stdout_regex     - Asserts CLI stdout matches a regex.
                         Required: type, pattern (string)
  cli_stderr_contains  - Asserts CLI stderr contains a substring.
                         Required: type, value (string)

Boolean combinators:
  anyOf                - Pass if ANY child assertion passes (OR logic).
                         Required: type, anyOf (list of assertions)
  allOf                - Pass if ALL child assertions pass (AND logic).
                         Required: type, allOf (list of assertions)
  not                  - Pass if the child assertion FAILS (NOT logic).
                         Required: type, not (single assertion object)
`

// validAssertionTypes is the complete list of assertion type strings.
var validAssertionTypes = []string{
	"tool_called",
	"tool_not_called",
	"tool_call_count",
	"tool_call_order",
	"tool_param_equals",
	"tool_param_matches_regex",
	"tool_result_matches_json",
	"output_contains",
	"output_not_contains",
	"output_regex",
	"max_tokens",
	"max_latency_ms",
	"no_error_messages",
	"no_hallucinated_tools",
	"no_clarification_questions",
	"no_rate_limit_errors",
	"cli_exit_code_equals",
	"cli_stdout_contains",
	"cli_stdout_regex",
	"cli_stderr_contains",
	"anyOf",
	"allOf",
	"not",
}
