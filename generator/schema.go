package generator

// sessionSchema documents the YAML structure that must be emitted by the LLM.
const sessionSchema = `
variables:                       # Optional: static values available in all tests (map[string]string)
  filename: "report.csv"         # Reference as {{filename}} in any prompt or assertion
sessions:                        # Required top-level key
  - name: "session-name"         # Human-readable session identifier (string)
    tests:                       # List of test objects
      - name: "test-name"        # Human-readable test identifier (string)
        prompt: "user prompt"    # Message sent to the agent. Supports {{variableName}} syntax.
        assertions:              # List of assertion objects (see assertion types below)
          - type: tool_called
            tool: tool_name
        extractors:              # Optional: capture runtime values for later tests in this session
          - type: jsonpath        # Only "jsonpath" is supported
            tool: tool_name      # Tool whose result to read
            path: "$.id"         # JSONPath expression
            variable_name: recordId  # Use as {{recordId}} in subsequent tests
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

// variablesAndExtractorsDoc explains static variables and dynamic extractors to the LLM.
const variablesAndExtractorsDoc = `
TEMPLATE VARIABLES
==================

Prompts and assertion values/params support Handlebars syntax: {{variableName}}.

STATIC VARIABLES (top-level variables:)
----------------------------------------
Define literal values once in the top-level "variables:" block and reference them via
{{name}} in any test's "prompt" and assertion "value" / "params" fields throughout the
entire generated output.

When to use: the same literal string appears in both the prompt and assertions.

Example:
variables:
  filename: "report.csv"
sessions:
  - name: "File operations"
    tests:
      - name: "Write report"
        prompt: "Create a file named {{filename}} with headers: id,name"
        assertions:
          - type: tool_called
            tool: write_file
          - type: tool_param_equals
            tool: write_file
            params:
              path: "{{filename}}"

DYNAMIC VARIABLES (extractors → cross-test references)
-------------------------------------------------------
Use "extractors:" to capture a value from a tool result and make it available
as {{variableName}} in all subsequent tests within the same session.

When to use: a later test needs a value produced at runtime by an earlier test
(e.g. an ID returned by a "create" operation).

Example:
sessions:
  - name: "User lifecycle"
    tests:
      - name: "Create record"
        prompt: "Create a new user with name Alice"
        assertions:
          - type: tool_called
            tool: create_user
        extractors:
          - type: jsonpath
            tool: create_user
            path: "$.id"
            variable_name: userId

      - name: "Fetch record"
        prompt: "Get user {{userId}}"
        assertions:
          - type: tool_called
            tool: get_user
          - type: tool_param_equals
            tool: get_user
            params:
              id: "{{userId}}"
`

// planSchema documents the JSON structure that the LLM must emit during the planning phase.
// It is intentionally compact — the LLM only decides WHAT to test, not how to write YAML.
const planSchema = `{
  "sessions": [
    {
      "name": "Session Name",
      "tests": [
        {
          "name": "Test Name",
          "goal": "Brief description of what this test verifies",
          "tools_expected": ["tool_name_1"],
          "assertions": ["tool_called: tool_name_1", "output_contains: expected text"]
        }
      ]
    }
  ]
}`

// validIntentCheckTypes is validAssertionTypes minus the boolean combinators
// (anyOf, allOf, not), which are forbidden in TestIntent checks.
var validIntentCheckTypes = []string{
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
}

// testIntentSchema is a JSON example shown to the LLM in BuildTestIntentPrompt.
const testIntentSchema = `{
  "name": "descriptive test name",
  "goal": "what this test verifies",
  "session_name": "session name (must match plan exactly)",
  "agent": "agent-name (omit if only one agent)",
  "prompt": "user prompt. Use {{varName}} for defined variables.",
  "expected_tools": ["tool_name"],
  "checks": [
    {"type": "tool_called",              "tool": "tool_name"},
    {"type": "tool_param_equals",        "tool": "tool_name", "params": {"param": "value", "nested.field": "value"}},
    {"type": "output_contains",          "value": "expected text"},
    {"type": "tool_call_count",          "tool": "tool_name", "count": 1},
    {"type": "tool_call_order",          "sequence": ["tool_a", "tool_b"]},
    {"type": "tool_param_matches_regex", "tool": "tool_name", "params": {"param": "regex"}},
    {"type": "tool_result_matches_json", "tool": "tool_name", "path": "$.field", "value": "x"},
    {"type": "output_regex",             "pattern": "regex"},
    {"type": "no_error_messages"},
    {"type": "no_hallucinated_tools"},
    {"type": "cli_exit_code_equals",     "expected": 0},
    {"type": "max_tokens",               "count": 1000}
  ],
  "extractors": [
    {"tool": "tool_name", "path": "$.id", "variable_name": "recordId"}
  ],
  "allowed_tools": ["tool_name"]
}`

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
