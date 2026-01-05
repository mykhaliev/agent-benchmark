package tests

import (
	"os"
	"testing"

	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// YAML Parser Tests
// ============================================================================

func TestParseTestConfig(t *testing.T) {
	t.Run("Valid configuration", func(t *testing.T) {
		yamlContent := `
providers:
  - name: test-provider
    type: OPENAI
    model: gpt-4
    token: test-token

servers:
  - name: test-server
    type: stdio
    command: "node server.js"

agents:
  - name: test-agent
    provider: test-provider
    servers:
      - name: test-server

sessions:
  - name: test-session
    tests:
      - name: test-1
        agent: test-agent
        prompt: "Test prompt"
        assertions:
          - type: output_contains
            value: "success"
`
		tmpfile := createTempYAML(t, yamlContent)

		config, err := model.ParseTestConfig(tmpfile)
		require.NoError(t, err)
		assert.NotNil(t, config)

		assert.Len(t, config.Providers, 1)
		assert.Equal(t, "test-provider", config.Providers[0].Name)

		assert.Len(t, config.Servers, 1)
		assert.Equal(t, "test-server", config.Servers[0].Name)

		assert.Len(t, config.Agents, 1)
		assert.Equal(t, "test-agent", config.Agents[0].Name)
	})

	t.Run("Invalid YAML", func(t *testing.T) {
		tmpfile := createTempYAML(t, "invalid: yaml: content:")

		_, err := model.ParseTestConfig(tmpfile)
		assert.Error(t, err)
	})

	t.Run("Non-existent file", func(t *testing.T) {
		_, err := model.ParseTestConfig("/non/existent/file.yaml")
		assert.Error(t, err)
	})
}

func TestParseTestConfigFromString(t *testing.T) {
	t.Run("Valid YAML string", func(t *testing.T) {
		yamlStr := `
providers:
  - name: test-provider
    type: OPENAI
    model: gpt-4
`
		config, err := model.ParseTestConfigFromString(yamlStr)
		require.NoError(t, err)
		assert.Len(t, config.Providers, 1)
	})

	t.Run("Invalid YAML string", func(t *testing.T) {
		_, err := model.ParseTestConfigFromString("invalid: yaml: :")
		assert.Error(t, err)
	})
}

func TestParseAgentClarificationDetection(t *testing.T) {
	tests := []struct {
		name                    string
		yaml                    string
		expectedEnabled         bool
		expectedLevel           string
		expectedUseBuiltin      *bool
		expectedCustomPatterns  []string
	}{
		{
			name: "clarification detection enabled with warning level",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      level: warning
`,
			expectedEnabled:        true,
			expectedLevel:          "warning",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection enabled with error level",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      level: error
`,
			expectedEnabled:        true,
			expectedLevel:          "error",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection enabled with info level",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      level: info
`,
			expectedEnabled:        true,
			expectedLevel:          "info",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection disabled",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: false
`,
			expectedEnabled:        false,
			expectedLevel:          "",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection not specified (defaults)",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
`,
			expectedEnabled:        false,
			expectedLevel:          "",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection enabled without level (uses default)",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
`,
			expectedEnabled:        true,
			expectedLevel:          "",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: nil,
		},
		{
			name: "clarification detection with custom patterns",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      level: warning
      custom_patterns:
        - "(?i)¿te gustaría"
        - "(?i)möchten sie"
`,
			expectedEnabled:        true,
			expectedLevel:          "warning",
			expectedUseBuiltin:     nil,
			expectedCustomPatterns: []string{"(?i)¿te gustaría", "(?i)möchten sie"},
		},
		{
			name: "clarification detection with use_builtin_patterns false",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      use_builtin_patterns: false
      custom_patterns:
        - "(?i)custom pattern"
`,
			expectedEnabled:        true,
			expectedLevel:          "",
			expectedUseBuiltin:     boolPtr(false),
			expectedCustomPatterns: []string{"(?i)custom pattern"},
		},
		{
			name: "clarification detection with use_builtin_patterns true explicitly",
			yaml: `
agents:
  - name: test-agent
    provider: test-provider
    clarification_detection:
      enabled: true
      use_builtin_patterns: true
`,
			expectedEnabled:        true,
			expectedLevel:          "",
			expectedUseBuiltin:     boolPtr(true),
			expectedCustomPatterns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := model.ParseTestConfigFromString(tt.yaml)
			require.NoError(t, err)
			require.Len(t, config.Agents, 1)

			agent := config.Agents[0]
			assert.Equal(t, tt.expectedEnabled, agent.ClarificationDetection.Enabled,
				"ClarificationDetection.Enabled mismatch")
			assert.Equal(t, tt.expectedLevel, agent.ClarificationDetection.Level,
				"ClarificationDetection.Level mismatch")
			if tt.expectedUseBuiltin == nil {
				assert.Nil(t, agent.ClarificationDetection.UseBuiltinPatterns,
					"ClarificationDetection.UseBuiltinPatterns should be nil")
			} else {
				require.NotNil(t, agent.ClarificationDetection.UseBuiltinPatterns,
					"ClarificationDetection.UseBuiltinPatterns should not be nil")
				assert.Equal(t, *tt.expectedUseBuiltin, *agent.ClarificationDetection.UseBuiltinPatterns,
					"ClarificationDetection.UseBuiltinPatterns mismatch")
			}
			assert.Equal(t, tt.expectedCustomPatterns, agent.ClarificationDetection.CustomPatterns,
				"ClarificationDetection.CustomPatterns mismatch")
		})
	}
}

// boolPtr is a helper function to create a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}

// ============================================================================
// Assertion Evaluator Tests
// ============================================================================

func TestAssertionEvaluator_ToolCalled(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "get_weather"},
			{Name: "calculate"},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		assertion  model.Assertion
		wantPassed bool
	}{
		{
			name:       "Tool was called",
			assertion:  model.Assertion{Type: "tool_called", Tool: "get_weather"},
			wantPassed: true,
		},
		{
			name:       "Tool was not called",
			assertion:  model.Assertion{Type: "tool_called", Tool: "non_existent"},
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := evaluator.Evaluate([]model.Assertion{tt.assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_ToolNotCalled(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "get_weather"},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		toolName   string
		wantPassed bool
	}{
		{
			name:       "Tool not called (pass)",
			toolName:   "calculate",
			wantPassed: true,
		},
		{
			name:       "Tool was called (fail)",
			toolName:   "get_weather",
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{Type: "tool_not_called", Tool: tt.toolName}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_ToolCallCount(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "get_weather"},
			{Name: "get_weather"},
			{Name: "calculate"},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		toolName   string
		count      int
		wantPassed bool
	}{
		{
			name:       "Total count",
			count:      3,
			wantPassed: true,
		},
		{
			name:       "Correct count",
			toolName:   "get_weather",
			count:      2,
			wantPassed: true,
		},
		{
			name:       "Incorrect count",
			toolName:   "get_weather",
			count:      1,
			wantPassed: false,
		},
		{
			name:       "Single call",
			toolName:   "calculate",
			count:      1,
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:  "tool_call_count",
				Tool:  tt.toolName,
				Count: tt.count,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_ToolCallOrder(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "step1"},
			{Name: "step2"},
			{Name: "other"},
			{Name: "step3"},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		sequence   []string
		wantPassed bool
	}{
		{
			name:       "Correct order",
			sequence:   []string{"step1", "step2", "step3"},
			wantPassed: true,
		},
		{
			name:       "Incorrect order",
			sequence:   []string{"step2", "step1"},
			wantPassed: false,
		},
		{
			name:       "Partial match",
			sequence:   []string{"step1", "step3"},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:     "tool_call_order",
				Sequence: tt.sequence,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_ToolParamEquals(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{
				Name: "get_weather",
				Parameters: map[string]interface{}{
					"location": "New York",
					"units":    "celsius",
				},
			},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		params     map[string]string
		wantPassed bool
	}{
		{
			name: "Matching params",
			params: map[string]string{
				"location": "New York",
			},
			wantPassed: true,
		},
		{
			name: "Non-matching params",
			params: map[string]string{
				"location": "London",
			},
			wantPassed: false,
		},
		{
			name: "Missing param",
			params: map[string]string{
				"country": "USA",
			},
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:   "tool_param_equals",
				Tool:   "get_weather",
				Params: tt.params,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_ToolParamMatchesRegex(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{
				Name: "send_email",
				Parameters: map[string]interface{}{
					"email": "test@example.com",
				},
			},
		},
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		pattern    string
		params     map[string]string
		wantPassed bool
	}{
		{
			name: "Matching regex",
			params: map[string]string{
				"email": `^[a-z]+@[a-z]+\.com$`,
			},
			wantPassed: true,
		},
		{
			name: "Non-matching regex",
			params: map[string]string{
				"email": `^[0-9]+$`,
			},
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:    "tool_param_matches_regex",
				Tool:    "send_email",
				Pattern: tt.pattern,
				Params:  tt.params,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_OutputContains(t *testing.T) {
	result := &model.ExecutionResult{
		FinalOutput: "The weather in New York is sunny",
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		value      string
		wantPassed bool
	}{
		{
			name:       "Contains substring",
			value:      "sunny",
			wantPassed: true,
		},
		{
			name:       "Does not contain",
			value:      "rainy",
			wantPassed: false,
		},
		{
			name:       "Case sensitive",
			value:      "Sunny",
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:  "output_contains",
				Value: tt.value,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_OutputRegex(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		pattern     string
		wantPassed  bool
		description string
	}{
		{
			name:        "Simple temperature pattern",
			output:      "Temperature: 72°F",
			pattern:     `Temperature: \d+°F`,
			wantPassed:  true,
			description: "Basic digit matching with special character",
		},
		{
			name:        "Non-matching temperature unit",
			output:      "Temperature: 72°F",
			pattern:     `Temperature: \d+°C`,
			wantPassed:  false,
			description: "Should fail when unit doesn't match",
		},
		{
			name:        "Email validation",
			output:      "Contact us at support@example.com for help",
			pattern:     `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
			wantPassed:  true,
			description: "Complex email regex pattern",
		},
		{
			name:        "Invalid email pattern",
			output:      "Contact us at support.example.com",
			pattern:     `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
			wantPassed:  false,
			description: "Should fail when email format is invalid",
		},
		{
			name:        "URL pattern matching",
			output:      "Visit https://www.example.com/api/v1/users",
			pattern:     `https?://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(/[a-zA-Z0-9/_-]*)?`,
			wantPassed:  true,
			description: "Match HTTP/HTTPS URLs with paths",
		},
		{
			name:        "IPv4 address",
			output:      "Server IP: 192.168.1.100",
			pattern:     `\b(?:\d{1,3}\.){3}\d{1,3}\b`,
			wantPassed:  true,
			description: "Match IPv4 address pattern",
		},
		{
			name:        "Date format YYYY-MM-DD",
			output:      "Event scheduled for 2024-12-25",
			pattern:     `\d{4}-\d{2}-\d{2}`,
			wantPassed:  true,
			description: "Match ISO date format",
		},
		{
			name:        "Date format MM/DD/YYYY",
			output:      "Event scheduled for 12/25/2024",
			pattern:     `\d{2}/\d{2}/\d{4}`,
			wantPassed:  true,
			description: "Match US date format",
		},
		{
			name:        "Phone number with dashes",
			output:      "Call us at 555-123-4567",
			pattern:     `\d{3}-\d{3}-\d{4}`,
			wantPassed:  true,
			description: "Match phone number format",
		},
		{
			name:        "Credit card number",
			output:      "Card: 4532-1234-5678-9010",
			pattern:     `\d{4}-\d{4}-\d{4}-\d{4}`,
			wantPassed:  true,
			description: "Match credit card format",
		},
		{
			name:        "UUID pattern",
			output:      "Request ID: 550e8400-e29b-41d4-a716-446655440000",
			pattern:     `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
			wantPassed:  true,
			description: "Match UUID v4 format",
		},
		{
			name:        "Currency amount",
			output:      "Total: $1,234.56",
			pattern:     `\$\d{1,3}(,\d{3})*\.\d{2}`,
			wantPassed:  true,
			description: "Match currency with thousands separator",
		},
		{
			name:        "Hex color code",
			output:      "Background color: #FF5733",
			pattern:     `#[0-9A-Fa-f]{6}`,
			wantPassed:  true,
			description: "Match 6-digit hex color",
		},
		{
			name:        "Semantic version",
			output:      "Version: v2.14.3",
			pattern:     `v\d+\.\d+\.\d+`,
			wantPassed:  true,
			description: "Match semantic versioning",
		},
		{
			name:        "Multiline pattern",
			output:      "Line 1\nLine 2\nLine 3",
			pattern:     `Line \d+`,
			wantPassed:  true,
			description: "Match pattern across multiple lines",
		},
		{
			name:        "Case insensitive flag",
			output:      "Status: SUCCESS",
			pattern:     `(?i)status: success`,
			wantPassed:  true,
			description: "Match with case insensitive flag",
		},
		{
			name:        "Word boundary",
			output:      "The cat sat on the mat",
			pattern:     `\bcat\b`,
			wantPassed:  true,
			description: "Match whole word only",
		},
		{
			name:        "Multiple matches",
			output:      "Files: file1.txt, file2.txt, file3.txt",
			pattern:     `file\d+\.txt`,
			wantPassed:  true,
			description: "Match multiple occurrences",
		},
		{
			name:        "JSON structure",
			output:      `{"status":"success","code":200}`,
			pattern:     `\{.*"status"\s*:\s*"success".*\}`,
			wantPassed:  true,
			description: "Match JSON with specific field",
		},
		{
			name:        "Time format HH:MM:SS",
			output:      "Timestamp: 14:30:45",
			pattern:     `\d{2}:\d{2}:\d{2}`,
			wantPassed:  true,
			description: "Match 24-hour time format",
		},
		{
			name:        "Time format with AM/PM",
			output:      "Meeting at 2:30 PM",
			pattern:     `\d{1,2}:\d{2}\s*(?:AM|PM)`,
			wantPassed:  true,
			description: "Match 12-hour time with period",
		},
		{
			name:        "Quoted string",
			output:      `Message: "Hello, World!"`,
			pattern:     `"[^"]*"`,
			wantPassed:  true,
			description: "Match text within quotes",
		},
		{
			name:        "File path Unix",
			output:      "Path: /home/user/documents/file.txt",
			pattern:     `(/[a-zA-Z0-9_-]+)+\.[a-z]+`,
			wantPassed:  true,
			description: "Match Unix file path",
		},
		{
			name:        "File path Windows",
			output:      `Path: C:\Users\John\Documents\file.txt`,
			pattern:     `[A-Z]:\\(?:[^\\]+\\)*[^\\]+`,
			wantPassed:  true,
			description: "Match Windows file path",
		},
		{
			name:        "MAC address",
			output:      "Device MAC: 00:1B:44:11:3A:B7",
			pattern:     `([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}`,
			wantPassed:  true,
			description: "Match MAC address format",
		},
		{
			name:        "Percentage",
			output:      "Progress: 85.5%",
			pattern:     `\d+\.?\d*%`,
			wantPassed:  true,
			description: "Match percentage with decimals",
		},
		{
			name:        "Negative numbers",
			output:      "Temperature: -15.5°C",
			pattern:     `-?\d+\.?\d*°C`,
			wantPassed:  true,
			description: "Match negative temperature",
		},
		{
			name:        "HTML tag",
			output:      "<div class='container'>Content</div>",
			pattern:     `<[^>]+>`,
			wantPassed:  true,
			description: "Match HTML tags",
		},
		{
			name:        "GitHub repo URL",
			output:      "Repository: https://github.com/user/repo-name",
			pattern:     `https://github\.com/[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+`,
			wantPassed:  true,
			description: "Match GitHub repository URL",
		},
		{
			name:        "SSH key fingerprint",
			output:      "SSH: SHA256:nThbg6kXUpJWGl7E1IGOCspRomTxdCARLviKw6E5SY8",
			pattern:     `SHA256:[A-Za-z0-9+/]{43}`,
			wantPassed:  true,
			description: "Match SSH key fingerprint",
		},
		{
			name:        "Docker image tag",
			output:      "Image: nginx:1.21.6-alpine",
			pattern:     `[a-z0-9._-]+:[a-z0-9._-]+`,
			wantPassed:  true,
			description: "Match Docker image with tag",
		},
		{
			name:        "AWS ARN",
			output:      "ARN: arn:aws:s3:::my-bucket/path/to/file",
			pattern:     `arn:aws:[a-z0-9-]+:[a-z0-9-]*:\d*:[a-zA-Z0-9:/_-]+`,
			wantPassed:  true,
			description: "Match AWS ARN format",
		},
		{
			name:        "SQL query pattern",
			output:      "Query: SELECT * FROM users WHERE id = 123",
			pattern:     `SELECT\s+\*\s+FROM\s+\w+`,
			wantPassed:  true,
			description: "Match basic SQL SELECT query",
		},
		{
			name:        "Environment variable",
			output:      "Using $HOME and $PATH variables",
			pattern:     `\$[A-Z_]+`,
			wantPassed:  true,
			description: "Match environment variable syntax",
		},
		{
			name:        "Error code pattern",
			output:      "Error: ERR_CONNECTION_REFUSED (code: 500)",
			pattern:     `ERR_[A-Z_]+`,
			wantPassed:  true,
			description: "Match error code format",
		},
		{
			name:        "Invalid regex pattern",
			output:      "Some text",
			pattern:     `[unclosed`,
			wantPassed:  false,
			description: "Should fail with invalid regex",
		},
		{
			name:        "Empty output",
			output:      "",
			pattern:     `\w+`,
			wantPassed:  false,
			description: "Should fail on empty output",
		},
		{
			name:        "Start anchor",
			output:      "The start of text",
			pattern:     `^The`,
			wantPassed:  true,
			description: "Match at start of string",
		},
		{
			name:        "End anchor",
			output:      "The end of text",
			pattern:     `text$`,
			wantPassed:  true,
			description: "Match at end of string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &model.ExecutionResult{
				FinalOutput: tt.output,
			}

			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

			assertion := model.Assertion{
				Type:    "output_regex",
				Pattern: tt.pattern,
			}

			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed,
				"Test: %s\nDescription: %s\nOutput: %s\nPattern: %s\nMessage: %s",
				tt.name, tt.description, tt.output, tt.pattern, results[0].Message)
		})
	}
}

func TestAssertionEvaluator_OutputRegex_EdgeCases(t *testing.T) {
	t.Run("Multiple patterns on same output", func(t *testing.T) {
		result := &model.ExecutionResult{
			FinalOutput: "User alice@example.com logged in at 2024-12-21 14:30:45 from IP 192.168.1.100",
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

		assertions := []model.Assertion{
			{Type: "output_regex", Pattern: `[a-z]+@[a-z]+\.[a-z]+`},       // email
			{Type: "output_regex", Pattern: `\d{4}-\d{2}-\d{2}`},           // date
			{Type: "output_regex", Pattern: `\d{2}:\d{2}:\d{2}`},           // time
			{Type: "output_regex", Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`}, // IP
		}

		results := evaluator.Evaluate(assertions)
		require.Len(t, results, 4)

		for i, result := range results {
			assert.True(t, result.Passed, "Assertion %d should pass", i)
		}
	})

	t.Run("Complex nested groups", func(t *testing.T) {
		result := &model.ExecutionResult{
			FinalOutput: "Transaction ID: TXN-2024-001-ABC-123",
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

		assertion := model.Assertion{
			Type:    "output_regex",
			Pattern: `TXN-(\d{4})-(\d{3})-([A-Z]{3})-(\d{3})`,
		}

		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("Unicode characters", func(t *testing.T) {
		result := &model.ExecutionResult{
			FinalOutput: "Price: 50€, Temperature: 20°C, Rating: ★★★★☆",
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

		assertions := []model.Assertion{
			{Type: "output_regex", Pattern: `\d+€`},
			{Type: "output_regex", Pattern: `\d+°C`},
			{Type: "output_regex", Pattern: `★+`},
		}

		results := evaluator.Evaluate(assertions)
		require.Len(t, results, 3)

		for _, result := range results {
			assert.True(t, result.Passed)
		}
	})

	t.Run("Very long output", func(t *testing.T) {
		longOutput := ""
		for i := 0; i < 1000; i++ {
			longOutput += "data line " + string(rune(i)) + "\n"
		}
		longOutput += "MARKER_FOUND_HERE"

		result := &model.ExecutionResult{
			FinalOutput: longOutput,
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

		assertion := model.Assertion{
			Type:    "output_regex",
			Pattern: `MARKER_FOUND_HERE`,
		}

		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("Special regex characters in output", func(t *testing.T) {
		result := &model.ExecutionResult{
			FinalOutput: "Match this: $100 + $200 = $300 (total)",
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

		assertion := model.Assertion{
			Type:    "output_regex",
			Pattern: `\$\d+ \+ \$\d+ = \$\d+ \(total\)`,
		}

		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})
}

func BenchmarkAssertionEvaluator_OutputRegex(b *testing.B) {
	result := &model.ExecutionResult{
		FinalOutput: "The quick brown fox jumps over the lazy dog. Temperature: 72°F. Email: test@example.com",
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	assertion := model.Assertion{
		Type:    "output_regex",
		Pattern: `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.Evaluate([]model.Assertion{assertion})
	}
}

func TestAssertionEvaluator_MaxTokens(t *testing.T) {
	result := &model.ExecutionResult{
		TokensUsed: 500,
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		maxTokens  string
		wantPassed bool
	}{
		{
			name:       "Under limit",
			maxTokens:  "1000",
			wantPassed: true,
		},
		{
			name:       "Over limit",
			maxTokens:  "100",
			wantPassed: false,
		},
		{
			name:       "Exactly at limit",
			maxTokens:  "500",
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:  "max_tokens",
				Value: tt.maxTokens,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_MaxLatency(t *testing.T) {
	result := &model.ExecutionResult{
		LatencyMs: 2500,
	}

	evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

	tests := []struct {
		name       string
		maxLatency string
		wantPassed bool
	}{
		{
			name:       "Under limit",
			maxLatency: "5000",
			wantPassed: true,
		},
		{
			name:       "Over limit",
			maxLatency: "1000",
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertion := model.Assertion{
				Type:  "max_latency_ms",
				Value: tt.maxLatency,
			}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_NoErrorMessages(t *testing.T) {
	tests := []struct {
		name       string
		errors     []string
		wantPassed bool
	}{
		{
			name:       "No errors",
			errors:     []string{},
			wantPassed: true,
		},
		{
			name:       "Has errors",
			errors:     []string{"Error 1", "Error 2"},
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &model.ExecutionResult{
				Errors: tt.errors,
			}
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

			assertion := model.Assertion{Type: "no_error_messages"}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_NoHallucinatedTools(t *testing.T) {
	knownTools := []string{"get_weather", "calculate"}

	tests := []struct {
		name       string
		toolCalls  []model.ToolCall
		wantPassed bool
	}{
		{
			name: "All known tools",
			toolCalls: []model.ToolCall{
				{Name: "get_weather"},
				{Name: "calculate"},
			},
			wantPassed: true,
		},
		{
			name: "Contains hallucinated tool",
			toolCalls: []model.ToolCall{
				{Name: "get_weather"},
				{Name: "unknown_tool"},
			},
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &model.ExecutionResult{
				ToolCalls: tt.toolCalls,
			}
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, knownTools)

			assertion := model.Assertion{Type: "no_hallucinated_tools"}
			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

// ============================================================================
// Boolean Combinator Tests (anyOf, allOf, not)
// ============================================================================

func TestAssertionEvaluator_AnyOf(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "keyboard_control"},
		},
		FinalOutput: "Task completed successfully",
	}

	tests := []struct {
		name       string
		assertion  model.Assertion
		wantPassed bool
	}{
		{
			name: "Any passes - first matches",
			assertion: model.Assertion{
				AnyOf: []model.Assertion{
					{Type: "tool_called", Tool: "keyboard_control"},
					{Type: "tool_called", Tool: "ui_automation"},
				},
			},
			wantPassed: true,
		},
		{
			name: "Any passes - second matches",
			assertion: model.Assertion{
				AnyOf: []model.Assertion{
					{Type: "tool_called", Tool: "ui_automation"},
					{Type: "tool_called", Tool: "keyboard_control"},
				},
			},
			wantPassed: true,
		},
		{
			name: "None match - fails",
			assertion: model.Assertion{
				AnyOf: []model.Assertion{
					{Type: "tool_called", Tool: "ui_automation"},
					{Type: "tool_called", Tool: "window_control"},
				},
			},
			wantPassed: false,
		},
		{
			name: "Both match - passes",
			assertion: model.Assertion{
				AnyOf: []model.Assertion{
					{Type: "tool_called", Tool: "keyboard_control"},
					{Type: "output_contains", Value: "completed"},
				},
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
			results := evaluator.Evaluate([]model.Assertion{tt.assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
			assert.Equal(t, "anyOf", results[0].Type)
		})
	}
}

func TestAssertionEvaluator_AllOf(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "keyboard_control"},
			{Name: "window_control"},
		},
		FinalOutput: "Task completed successfully",
	}

	tests := []struct {
		name       string
		assertion  model.Assertion
		wantPassed bool
	}{
		{
			name: "All pass",
			assertion: model.Assertion{
				AllOf: []model.Assertion{
					{Type: "tool_called", Tool: "keyboard_control"},
					{Type: "tool_called", Tool: "window_control"},
				},
			},
			wantPassed: true,
		},
		{
			name: "One fails",
			assertion: model.Assertion{
				AllOf: []model.Assertion{
					{Type: "tool_called", Tool: "keyboard_control"},
					{Type: "tool_called", Tool: "ui_automation"},
				},
			},
			wantPassed: false,
		},
		{
			name: "All fail",
			assertion: model.Assertion{
				AllOf: []model.Assertion{
					{Type: "tool_called", Tool: "ui_automation"},
					{Type: "tool_called", Tool: "mouse_control"},
				},
			},
			wantPassed: false,
		},
		{
			name: "Mixed assertion types - all pass",
			assertion: model.Assertion{
				AllOf: []model.Assertion{
					{Type: "tool_called", Tool: "keyboard_control"},
					{Type: "output_contains", Value: "completed"},
				},
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
			results := evaluator.Evaluate([]model.Assertion{tt.assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
			assert.Equal(t, "allOf", results[0].Type)
		})
	}
}

func TestAssertionEvaluator_Not(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "keyboard_control"},
		},
		FinalOutput: "Task completed successfully",
	}

	tests := []struct {
		name       string
		assertion  model.Assertion
		wantPassed bool
	}{
		{
			name: "Child fails - not passes",
			assertion: model.Assertion{
				Not: &model.Assertion{
					Type: "tool_called",
					Tool: "ui_automation",
				},
			},
			wantPassed: true,
		},
		{
			name: "Child passes - not fails",
			assertion: model.Assertion{
				Not: &model.Assertion{
					Type: "tool_called",
					Tool: "keyboard_control",
				},
			},
			wantPassed: false,
		},
		{
			name: "Not with output_contains - negation",
			assertion: model.Assertion{
				Not: &model.Assertion{
					Type:  "output_contains",
					Value: "error",
				},
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
			results := evaluator.Evaluate([]model.Assertion{tt.assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
			assert.Equal(t, "not", results[0].Type)
		})
	}
}

func TestAssertionEvaluator_NestedCombinators(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "keyboard_control"},
			{Name: "window_control"},
		},
		FinalOutput: "Task completed successfully",
	}

	tests := []struct {
		name       string
		assertion  model.Assertion
		wantPassed bool
	}{
		{
			name: "anyOf with nested allOf - passes",
			assertion: model.Assertion{
				AnyOf: []model.Assertion{
					{
						AllOf: []model.Assertion{
							{Type: "tool_called", Tool: "ui_automation"},
							{Type: "tool_called", Tool: "mouse_control"},
						},
					},
					{
						AllOf: []model.Assertion{
							{Type: "tool_called", Tool: "keyboard_control"},
							{Type: "tool_called", Tool: "window_control"},
						},
					},
				},
			},
			wantPassed: true,
		},
		{
			name: "allOf with nested anyOf - passes",
			assertion: model.Assertion{
				AllOf: []model.Assertion{
					{
						AnyOf: []model.Assertion{
							{Type: "tool_called", Tool: "keyboard_control"},
							{Type: "tool_called", Tool: "ui_automation"},
						},
					},
					{Type: "output_contains", Value: "completed"},
				},
			},
			wantPassed: true,
		},
		{
			name: "not with nested anyOf - passes",
			assertion: model.Assertion{
				Not: &model.Assertion{
					AnyOf: []model.Assertion{
						{Type: "tool_called", Tool: "ui_automation"},
						{Type: "tool_called", Tool: "mouse_control"},
					},
				},
			},
			wantPassed: true,
		},
		{
			name: "not with nested anyOf - fails (child passes)",
			assertion: model.Assertion{
				Not: &model.Assertion{
					AnyOf: []model.Assertion{
						{Type: "tool_called", Tool: "keyboard_control"},
						{Type: "tool_called", Tool: "mouse_control"},
					},
				},
			},
			wantPassed: false,
		},
		{
			name: "Double negation",
			assertion: model.Assertion{
				Not: &model.Assertion{
					Not: &model.Assertion{
						Type: "tool_called",
						Tool: "keyboard_control",
					},
				},
			},
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
			results := evaluator.Evaluate([]model.Assertion{tt.assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed)
		})
	}
}

func TestAssertionEvaluator_CombinatorDetails(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "keyboard_control"},
		},
	}

	t.Run("anyOf includes child results in details", func(t *testing.T) {
		assertion := model.Assertion{
			AnyOf: []model.Assertion{
				{Type: "tool_called", Tool: "keyboard_control"},
				{Type: "tool_called", Tool: "ui_automation"},
			},
		}
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		details := results[0].Details
		assert.NotNil(t, details)
		assert.Equal(t, 1, details["passed_count"])
		assert.Equal(t, 1, details["failed_count"])
		assert.NotNil(t, details["children"])
	})

	t.Run("allOf includes child results in details", func(t *testing.T) {
		assertion := model.Assertion{
			AllOf: []model.Assertion{
				{Type: "tool_called", Tool: "keyboard_control"},
				{Type: "tool_called", Tool: "ui_automation"},
			},
		}
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		details := results[0].Details
		assert.NotNil(t, details)
		assert.Equal(t, 1, details["passed_count"])
		assert.Equal(t, 1, details["failed_count"])
		assert.NotNil(t, details["children"])
	})

	t.Run("not includes child result in details", func(t *testing.T) {
		assertion := model.Assertion{
			Not: &model.Assertion{
				Type: "tool_called",
				Tool: "ui_automation",
			},
		}
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		details := results[0].Details
		assert.NotNil(t, details)
		assert.NotNil(t, details["child"])
	})
}

func TestAssertionEvaluator_CombinatorDepthLimit(t *testing.T) {
	result := &model.ExecutionResult{
		ToolCalls: []model.ToolCall{
			{Name: "test_tool"},
		},
	}

	// Build deeply nested structure that exceeds depth limit (10)
	// Create 12 levels of nesting to exceed the limit
	buildDeeplyNested := func(depth int) model.Assertion {
		innermost := model.Assertion{Type: "tool_called", Tool: "test_tool"}
		current := &innermost
		for i := 0; i < depth; i++ {
			next := model.Assertion{
				Not: current,
			}
			current = &next
		}
		return *current
	}

	t.Run("Deeply nested combinators exceed depth limit", func(t *testing.T) {
		// 12 levels of nesting should exceed the limit of 10
		assertion := buildDeeplyNested(12)
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		// Should fail due to depth limit
		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "Maximum combinator nesting depth")
	})

	t.Run("Nesting within limit succeeds", func(t *testing.T) {
		// 5 levels of nesting should be fine (within limit of 10)
		// 5 levels of NOT on tool_called("test_tool"):
		// not(not(not(not(not(tool_called))))) = not(tool_called) = false (since tool was called)
		assertion := buildDeeplyNested(5)
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		// Should not fail due to depth limit
		assert.NotContains(t, results[0].Message, "Maximum combinator nesting depth")
	})

	t.Run("anyOf depth limit", func(t *testing.T) {
		// Build anyOf chain that exceeds depth - need proper nesting to avoid circular refs
		var buildAnyOfChain func(depth int) model.Assertion
		buildAnyOfChain = func(depth int) model.Assertion {
			if depth == 0 {
				return model.Assertion{Type: "tool_called", Tool: "test_tool"}
			}
			return model.Assertion{
				AnyOf: []model.Assertion{buildAnyOfChain(depth - 1)},
			}
		}

		assertion := buildAnyOfChain(12)
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "Maximum combinator nesting depth")
	})

	t.Run("allOf depth limit", func(t *testing.T) {
		// Build allOf chain that exceeds depth - need proper nesting to avoid circular refs
		var buildAllOfChain func(depth int) model.Assertion
		buildAllOfChain = func(depth int) model.Assertion {
			if depth == 0 {
				return model.Assertion{Type: "tool_called", Tool: "test_tool"}
			}
			return model.Assertion{
				AllOf: []model.Assertion{buildAllOfChain(depth - 1)},
			}
		}

		assertion := buildAllOfChain(12)
		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)

		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "Maximum combinator nesting depth")
	})
}

// ============================================================================
// Utility Functions Tests
// ============================================================================

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected bool
	}{
		{"Same strings", "hello", "hello", true},
		{"Different strings", "hello", "world", false},
		{"Same numbers", 42, 42, true},
		{"Same arrays", []int{1, 2, 3}, []int{1, 2, 3}, true},
		{"Different arrays", []int{1, 2}, []int{1, 3}, false},
		{"Nil values", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.DeepEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		context  map[string]string
		expected string
	}{
		{
			name:     "Simple variable",
			input:    "Hello {{name}}",
			context:  map[string]string{"name": "World"},
			expected: "Hello World",
		},
		{
			name:     "Multiple variables",
			input:    "{{greeting}} {{name}}!",
			context:  map[string]string{"greeting": "Hello", "name": "Alice"},
			expected: "Hello Alice!",
		},
		{
			name:     "Invalid template returns original",
			input:    "{{unclosed",
			context:  map[string]string{},
			expected: "{{unclosed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.RenderTemplate(tt.input, tt.context)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func createTempYAML(t *testing.T, content string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp("", "test-*.yaml")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	_, err = tmpfile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())

	return tmpfile.Name()
}

func TestAssertionEvaluator_ToolResultMatchesJson(t *testing.T) {
	tests := []struct {
		name       string
		toolResult string
		path       string
		value      string
		wantPassed bool
	}{
		{
			name: "Simple JSONPath match",
			toolResult: `{
				"status": "success",
				"data": {"temperature": 72}
			}`,
			path:       "$.status",
			value:      "success",
			wantPassed: true,
		},
		{
			name: "Nested JSONPath match",
			toolResult: `{
				"data": {"temperature": 72}
			}`,
			path:       "$.data.temperature",
			value:      "72",
			wantPassed: true,
		},
		{
			name: "Array JSONPath match",
			toolResult: `{
				"items": ["apple", "banana", "orange"]
			}`,
			path:       "$.items[0]",
			value:      "apple",
			wantPassed: true,
		},
		{
			name: "Non-matching value",
			toolResult: `{
				"status": "success"
			}`,
			path:       "$.status",
			value:      "failure",
			wantPassed: false,
		},
		{
			name: "Invalid JSONPath",
			toolResult: `{
				"status": "success"
			}`,
			path:       "$.nonexistent.path",
			value:      "value",
			wantPassed: false,
		},
		{
			name:       "Invalid JSON",
			toolResult: `{invalid json}`,
			path:       "$.status",
			value:      "success",
			wantPassed: false,
		},
		{
			name: "Complex nested structure",
			toolResult: `{
				"user": {
					"profile": {
						"name": "John Doe",
						"age": 30
					}
				}
			}`,
			path:       "$.user.profile.name",
			value:      "John Doe",
			wantPassed: true,
		},
		{
			name: "Array length check",
			toolResult: `{
				"items": [1, 2, 3, 4, 5]
			}`,
			path:       "$.items[4]",
			value:      "5",
			wantPassed: true,
		},
		{
			name: "Boolean value",
			toolResult: `{
				"active": true
			}`,
			path:       "$.active",
			value:      "true",
			wantPassed: true,
		},
		{
			name: "Null value",
			toolResult: `{
				"value": null
			}`,
			path:       "$.value",
			value:      "null",
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &model.ExecutionResult{
				ToolCalls: []model.ToolCall{
					{
						Name: "test_tool",
						Result: model.Result{
							Content: []model.ContentItem{
								{
									Type: "text",
									Text: tt.toolResult,
								},
							},
						},
					},
				},
			}

			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

			assertion := model.Assertion{
				Type:  "tool_result_matches_json",
				Tool:  "test_tool",
				Path:  tt.path,
				Value: tt.value,
			}

			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed,
				"Expected passed=%v, got passed=%v. Message: %s",
				tt.wantPassed, results[0].Passed, results[0].Message)
		})
	}
}

func TestAssertionEvaluator_ToolResultMatchesJson_MultipleCalls(t *testing.T) {
	t.Run("Multiple tool calls - first matches", func(t *testing.T) {
		result := &model.ExecutionResult{
			ToolCalls: []model.ToolCall{
				{
					Name: "test_tool",
					Result: model.Result{
						Content: []model.ContentItem{
							{Type: "text", Text: `{"status": "success"}`},
						},
					},
				},
				{
					Name: "test_tool",
					Result: model.Result{
						Content: []model.ContentItem{
							{Type: "text", Text: `{"status": "failure"}`},
						},
					},
				},
			},
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		assertion := model.Assertion{
			Type:  "tool_result_matches_json",
			Tool:  "test_tool",
			Path:  "$.status",
			Value: "success",
		}

		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed, "Should pass when at least one call matches")
	})

	t.Run("Tool not called", func(t *testing.T) {
		result := &model.ExecutionResult{
			ToolCalls: []model.ToolCall{
				{
					Name: "other_tool",
					Result: model.Result{
						Content: []model.ContentItem{
							{Type: "text", Text: `{"status": "success"}`},
						},
					},
				},
			},
		}

		evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})
		assertion := model.Assertion{
			Type:  "tool_result_matches_json",
			Tool:  "test_tool",
			Path:  "$.status",
			Value: "success",
		}

		results := evaluator.Evaluate([]model.Assertion{assertion})
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "was not called")
	})
}

func TestAssertionEvaluator_OutputNotContains(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		value      string
		wantPassed bool
	}{
		{
			name:       "Does not contain substring",
			output:     "The weather is sunny today",
			value:      "rainy",
			wantPassed: true,
		},
		{
			name:       "Contains substring (should fail)",
			output:     "The weather is sunny today",
			value:      "sunny",
			wantPassed: false,
		},
		{
			name:       "Case sensitive - different case",
			output:     "Hello World",
			value:      "hello",
			wantPassed: true,
		},
		{
			name:       "Case sensitive - exact match",
			output:     "Hello World",
			value:      "Hello",
			wantPassed: false,
		},
		{
			name:       "Empty value",
			output:     "Some text",
			value:      "",
			wantPassed: false,
		},
		{
			name:       "Special characters",
			output:     "Price: $50.00",
			value:      "$100",
			wantPassed: true,
		},
		{
			name:       "Special characters present",
			output:     "Price: $50.00",
			value:      "$50",
			wantPassed: false,
		},
		{
			name:       "Partial word match",
			output:     "The test passed",
			value:      "fail",
			wantPassed: true,
		},
		{
			name:       "Whitespace in value",
			output:     "Hello World",
			value:      "Hello  World",
			wantPassed: true,
		},
		{
			name:       "Newline in output",
			output:     "Line 1\nLine 2\nLine 3",
			value:      "Line 4",
			wantPassed: true,
		},
		{
			name:       "Newline present",
			output:     "Line 1\nLine 2",
			value:      "Line 2",
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &model.ExecutionResult{
				FinalOutput: tt.output,
			}

			evaluator := model.NewAssertionEvaluator(result, map[string]string{}, []string{})

			assertion := model.Assertion{
				Type:  "output_not_contains",
				Value: tt.value,
			}

			results := evaluator.Evaluate([]model.Assertion{assertion})
			require.Len(t, results, 1)
			assert.Equal(t, tt.wantPassed, results[0].Passed,
				"Expected passed=%v, got passed=%v. Message: %s",
				tt.wantPassed, results[0].Passed, results[0].Message)
		})
	}
}
