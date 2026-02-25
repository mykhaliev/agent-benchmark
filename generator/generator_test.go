package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func TestMain(m *testing.M) {
	logger.SetupLogger(io.Discard, false)
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// TestParseGeneratorConfig
// ---------------------------------------------------------------------------

func TestParseGeneratorConfig_Defaults(t *testing.T) {
	content := `
providers:
  - name: gemini
    type: GOOGLE
    token: "test-token"
    model: gemini-2.0-flash

agents:
  - name: file-agent
    provider: gemini
`
	path := writeTempYAML(t, content)

	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.Generator.TestCount, "default TestCount should be 5")
	assert.Equal(t, "medium", cfg.Generator.Complexity, "default Complexity should be medium")
	assert.Equal(t, 5, cfg.Generator.MaxStepsPerTest, "default MaxStepsPerTest should be 5")
	assert.False(t, cfg.Generator.IncludeEdgeCases, "default IncludeEdgeCases should be false")
	// Agent defaults to first agent's name when not set.
	assert.Equal(t, "file-agent", cfg.Generator.Agent)
}

func TestParseGeneratorConfig_Explicit(t *testing.T) {
	content := `
providers:
  - name: gpt
    type: OPENAI
    token: "sk-test"
    model: gpt-4o

agents:
  - name: my-agent
    provider: gpt

generator:
  agent: my-agent
  test_count: 10
  complexity: complex
  include_edge_cases: true
  max_steps_per_test: 8
`
	path := writeTempYAML(t, content)

	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 10, cfg.Generator.TestCount)
	assert.Equal(t, "complex", cfg.Generator.Complexity)
	assert.True(t, cfg.Generator.IncludeEdgeCases)
	assert.Equal(t, 8, cfg.Generator.MaxStepsPerTest)
	assert.Equal(t, "my-agent", cfg.Generator.Agent)
}

func TestParseGeneratorConfig_MissingFile(t *testing.T) {
	_, err := ParseGeneratorConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestParseGeneratorConfig_MaxTokens(t *testing.T) {
	content := `
providers:
  - name: gpt
    type: OPENAI
    token: "sk-test"
    model: gpt-4o

agents:
  - name: my-agent
    provider: gpt

generator:
  max_tokens: 5000
`
	path := writeTempYAML(t, content)

	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 5000, cfg.Generator.MaxTokens)
}

func TestParseGeneratorConfig_MaxTokensDefaultZero(t *testing.T) {
	content := `
providers:
  - name: gpt
    type: OPENAI
    token: "sk-test"
    model: gpt-4o

agents:
  - name: my-agent
    provider: gpt
`
	path := writeTempYAML(t, content)

	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 0, cfg.Generator.MaxTokens, "omitted max_tokens should default to 0 (unlimited)")
}

// ---------------------------------------------------------------------------
// TestExtractYAMLFromResponse
// ---------------------------------------------------------------------------

func TestExtractYAMLFromResponse_NoFences(t *testing.T) {
	input := "sessions:\n  - name: test\n"
	assert.Equal(t, "sessions:\n  - name: test", ExtractYAMLFromResponse(input))
}

func TestExtractYAMLFromResponse_YamlFence(t *testing.T) {
	input := "```yaml\nsessions:\n  - name: test\n```"
	got := ExtractYAMLFromResponse(input)
	assert.Equal(t, "sessions:\n  - name: test", got)
}

func TestExtractYAMLFromResponse_PlainFence(t *testing.T) {
	input := "```\nsessions:\n  - name: test\n```"
	got := ExtractYAMLFromResponse(input)
	assert.Equal(t, "sessions:\n  - name: test", got)
}

func TestExtractYAMLFromResponse_YmlFence(t *testing.T) {
	input := "```yml\nsessions:\n  - name: test\n```"
	got := ExtractYAMLFromResponse(input)
	assert.Equal(t, "sessions:\n  - name: test", got)
}

func TestExtractYAMLFromResponse_LeadingTrailingWhitespace(t *testing.T) {
	input := "  \n```yaml\nsessions:\n  - name: test\n```\n  "
	got := ExtractYAMLFromResponse(input)
	assert.Equal(t, "sessions:\n  - name: test", got)
}

// ---------------------------------------------------------------------------
// TestExtractJSONFromResponse
// ---------------------------------------------------------------------------

func TestExtractJSONFromResponse_NoFences(t *testing.T) {
	input := `{"sessions":[]}`
	assert.Equal(t, `{"sessions":[]}`, ExtractJSONFromResponse(input))
}

func TestExtractJSONFromResponse_JsonFence(t *testing.T) {
	input := "```json\n{\"sessions\":[]}\n```"
	assert.Equal(t, `{"sessions":[]}`, ExtractJSONFromResponse(input))
}

func TestExtractJSONFromResponse_PlainFence(t *testing.T) {
	input := "```\n{\"sessions\":[]}\n```"
	assert.Equal(t, `{"sessions":[]}`, ExtractJSONFromResponse(input))
}

// ---------------------------------------------------------------------------
// TestValidateSessions
// ---------------------------------------------------------------------------

const validSessionsYAML = `
sessions:
  - name: basic-tests
    tests:
      - name: list files
        agent: file-agent
        prompt: List the files in /tmp
        assertions:
          - type: tool_called
            tool: list_directory
          - type: output_contains
            value: /tmp
`

func TestValidateSessions_Valid(t *testing.T) {
	errs := ValidateSessions(validSessionsYAML, []string{"file-agent"}, nil)
	assert.Empty(t, errs)
}

func TestValidateSessions_InvalidAssertionType(t *testing.T) {
	content := `
sessions:
  - name: bad-assertion
    tests:
      - name: test1
        agent: file-agent
        prompt: Do something
        assertions:
          - type: nonexistent_assertion_type
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "nonexistent_assertion_type"), "should flag unknown assertion type")
}

func TestValidateSessions_UnknownAgent(t *testing.T) {
	content := `
sessions:
  - name: unknown-agent-test
    tests:
      - name: test1
        agent: ghost-agent
        prompt: Do something
        assertions:
          - type: no_error_messages
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost-agent"), "should flag unknown agent name")
}

func TestValidateSessions_MissingPrompt(t *testing.T) {
	content := `
sessions:
  - name: no-prompt
    tests:
      - name: test1
        agent: file-agent
        assertions:
          - type: no_error_messages
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "missing prompt"))
}

func TestValidateSessions_InvalidYAML(t *testing.T) {
	errs := ValidateSessions("this: is: not: valid: yaml: [", []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
}

func TestValidateSessions_EmptyAgentList(t *testing.T) {
	// When no known agents are provided, agent field validation is skipped.
	errs := ValidateSessions(validSessionsYAML, []string{}, nil)
	assert.Empty(t, errs)
}

// ---------------------------------------------------------------------------
// TestValidateSessions — semantic checks
// ---------------------------------------------------------------------------

func TestValidateSessions_SemanticToolName(t *testing.T) {
	content := `
sessions:
  - name: tool-check
    tests:
      - name: call ghost tool
        prompt: Do something
        assertions:
          - type: tool_called
            tool: ghost_tool
`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "read_file"},
		},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"), "should flag unknown tool name")
}

func TestValidateSessions_SemanticToolName_Valid(t *testing.T) {
	content := `
sessions:
  - name: tool-check
    tests:
      - name: call real tool
        prompt: Do something
        assertions:
          - type: tool_called
            tool: read_file
`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "read_file"},
		},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.Empty(t, errs)
}

func TestValidateSessions_SemanticParamName(t *testing.T) {
	content := `
sessions:
  - name: param-check
    tests:
      - name: wrong param name
        prompt: Do something
        assertions:
          - type: tool_param_equals
            tool: write_file
            params:
              ghost_param: value
`
	writeFileTool := mcp.Tool{Name: "write_file"}
	writeFileTool.InputSchema.Properties = map[string]any{
		"path":    map[string]any{"type": "string"},
		"content": map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {writeFileTool},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_param"), "should flag unknown param name")
}

func TestValidateSessions_SemanticParamName_Valid(t *testing.T) {
	content := `
sessions:
  - name: param-check
    tests:
      - name: correct param name
        prompt: Do something
        assertions:
          - type: tool_param_equals
            tool: write_file
            params:
              path: /tmp/file.txt
`
	writeFileTool := mcp.Tool{Name: "write_file"}
	writeFileTool.InputSchema.Properties = map[string]any{
		"path":    map[string]any{"type": "string"},
		"content": map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {writeFileTool},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.Empty(t, errs)
}

func TestValidateSessions_ForwardReference(t *testing.T) {
	content := `
sessions:
  - name: ref-check
    tests:
      - name: use undefined var
        prompt: Create file {{undefinedVar}}
        assertions:
          - type: no_error_messages
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "undefinedVar"), "should flag undefined variable")
}

func TestValidateSessions_ForwardReference_DefinedInVariables(t *testing.T) {
	content := `
variables:
  filename: report.csv
sessions:
  - name: ref-check
    tests:
      - name: use defined var
        prompt: Create file {{filename}}
        assertions:
          - type: no_error_messages
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.Empty(t, errs)
}

func TestValidateSessions_ForwardReference_DefinedByExtractor(t *testing.T) {
	content := `
sessions:
  - name: lifecycle
    tests:
      - name: create record
        prompt: Create a user
        assertions:
          - type: tool_called
            tool: create_user
        extractors:
          - type: jsonpath
            tool: create_user
            path: "$.id"
            variable_name: userId
      - name: fetch record
        prompt: Get user {{userId}}
        assertions:
          - type: tool_called
            tool: get_user
`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "create_user"},
			{Name: "get_user"},
		},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.Empty(t, errs)
}

func TestValidateSessions_ForwardReference_UsedBeforeExtractor(t *testing.T) {
	content := `
sessions:
  - name: lifecycle
    tests:
      - name: use var before extraction
        prompt: Get user {{userId}}
        assertions:
          - type: no_error_messages
        extractors:
          - type: jsonpath
            tool: create_user
            path: "$.id"
            variable_name: userId
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "userId"), "should flag variable used before extraction")
}

func TestValidateSessions_BuiltinTemplateVars(t *testing.T) {
	// Built-in template variables (TEST_DIR, RUN_ID, randomValue, etc.) must never be flagged.
	content := `
sessions:
  - name: builtins
    tests:
      - name: use builtins
        prompt: "Write to {{TEST_DIR}}/{{RUN_ID}}.txt"
        assertions:
          - type: no_error_messages
`
	errs := ValidateSessions(content, []string{"file-agent"}, nil)
	assert.Empty(t, errs)
}

func TestValidateSessions_ToolCallOrder_SemanticCheck(t *testing.T) {
	content := `
sessions:
  - name: order-check
    tests:
      - name: ordered tools
        prompt: Do something
        assertions:
          - type: tool_call_order
            sequence:
              - real_tool
              - ghost_tool
`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "real_tool"},
		},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"), "should flag ghost_tool in sequence")
}

func TestValidateSessions_ExtractorUnknownTool(t *testing.T) {
	content := `
sessions:
  - name: extractor-check
    tests:
      - name: extractor with ghost tool
        prompt: Create something
        assertions:
          - type: no_error_messages
        extractors:
          - type: jsonpath
            tool: ghost_tool
            path: "$.id"
            variable_name: resultId
`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "real_tool"},
		},
	}
	errs := ValidateSessions(content, []string{"file-agent"}, toolsByAgent)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"), "should flag extractor with unknown tool")
}

// ---------------------------------------------------------------------------
// TestValidatePlan
// ---------------------------------------------------------------------------

func TestValidatePlan_Valid(t *testing.T) {
	planJSON := `{
		"sessions": [
			{
				"name": "File Operations",
				"tests": [
					{
						"name": "Write CSV",
						"goal": "Verify agent writes a CSV file",
						"tools_expected": ["write_file"],
						"assertions": ["tool_called: write_file"]
					}
				]
			}
		]
	}`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {{Name: "write_file"}},
	}
	errs := ValidatePlan(planJSON, toolsByAgent)
	assert.Empty(t, errs)
}

func TestValidatePlan_UnknownTool(t *testing.T) {
	planJSON := `{
		"sessions": [
			{
				"name": "File Operations",
				"tests": [
					{
						"name": "Ghost op",
						"goal": "Test ghost tool",
						"tools_expected": ["ghost_tool"],
						"assertions": []
					}
				]
			}
		]
	}`
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {{Name: "write_file"}},
	}
	errs := ValidatePlan(planJSON, toolsByAgent)
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"), "should flag unknown tool in plan")
}

func TestValidatePlan_InvalidJSON(t *testing.T) {
	errs := ValidatePlan("not valid json {[", map[string][]mcp.Tool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "parse error"), "should report parse error")
}

func TestValidatePlan_NoSessions(t *testing.T) {
	errs := ValidatePlan(`{"sessions":[]}`, map[string][]mcp.Tool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "no sessions"), "should report no sessions")
}

func TestValidatePlan_EmptyToolsByAgent(t *testing.T) {
	// When toolsByAgent is empty, tool name checks are skipped — plan is valid.
	planJSON := `{
		"sessions": [
			{
				"name": "S1",
				"tests": [{"name":"T1","goal":"g","tools_expected":["any_tool"],"assertions":[]}]
			}
		]
	}`
	errs := ValidatePlan(planJSON, map[string][]mcp.Tool{})
	assert.Empty(t, errs)
}

// ---------------------------------------------------------------------------
// TestBuildPlanPrompt
// ---------------------------------------------------------------------------

func TestBuildPlanPrompt_Structure(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "my-agent", Provider: "gpt"},
		},
		Generator: GeneratorSettings{
			TestCount:  5,
			Complexity: "medium",
		},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {
			{Name: "read_file", Description: "Read a file from disk"},
			{Name: "write_file", Description: "Write content to a file"},
		},
	}

	msgs := BuildPlanPrompt(cfg, toolsByAgent)

	require.Len(t, msgs, 2, "plan prompt should have system + user message")

	systemContent := extractText(msgs[0])
	userContent := extractText(msgs[1])

	// System prompt should mention JSON output rules.
	assert.Contains(t, systemContent, "JSON")
	assert.Contains(t, systemContent, "YAML")

	// User message should contain tool names and constraints.
	assert.Contains(t, userContent, "read_file")
	assert.Contains(t, userContent, "write_file")
	assert.Contains(t, userContent, "my-agent")
	assert.Contains(t, userContent, "5") // test count
}

func TestBuildPlanPrompt_IncludesAgentSystemPrompt(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "file-agent", Provider: "gpt",
				SystemPrompt: "You are a filesystem expert."},
		},
		Generator: GeneratorSettings{TestCount: 3, Complexity: "simple"},
	}

	msgs := BuildPlanPrompt(cfg, map[string][]mcp.Tool{})

	require.Len(t, msgs, 2)
	systemContent := extractText(msgs[0])

	assert.Contains(t, systemContent, "You are a filesystem expert.")
}

func TestBuildPlanPrompt_IncludesUserPrompt(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "agent1", Provider: "p"},
		},
		Generator: GeneratorSettings{
			TestCount:  3,
			Complexity: "simple",
			Goal:       "Focus on file creation workflows",
		},
	}

	msgs := BuildPlanPrompt(cfg, map[string][]mcp.Tool{})
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, "Focus on file creation workflows")
}

// ---------------------------------------------------------------------------
// TestBuildPrompt
// ---------------------------------------------------------------------------

func TestBuildPrompt_ContainsToolNamesAndAgentNames(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "my-agent", Provider: "gpt"},
		},
		Generator: GeneratorSettings{
			TestCount:       3,
			Complexity:      "simple",
			MaxStepsPerTest: 2,
		},
	}

	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {
			{Name: "read_file", Description: "Read a file from disk"},
			{Name: "write_file", Description: "Write content to a file"},
		},
	}

	msgs := BuildGenerationPrompt(cfg, toolsByAgent, 0, 1, "", nil, "")

	require.Len(t, msgs, 2)

	systemContent := extractText(msgs[0])
	userContent := extractText(msgs[1])

	// System prompt should contain schema info.
	assert.Contains(t, systemContent, "sessions:")

	// User message should contain tool names and agent names.
	assert.Contains(t, userContent, "my-agent")
	assert.Contains(t, userContent, "read_file")
	assert.Contains(t, userContent, "write_file")
	assert.Contains(t, userContent, "test_count: 3")
	assert.Contains(t, userContent, "complexity: simple")
}

func TestBuildPrompt_IncludesRetryErrors(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "agent1", Provider: "p"}},
		Generator: GeneratorSettings{TestCount: 2, Complexity: "medium", MaxStepsPerTest: 3},
	}

	prevYAML := "sessions:\n  - name: bad-session\n    tests:\n      - name: t\n        agent: foo\n        prompt: p\n        assertions: []\n"
	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 0, 2, prevYAML, []string{"unknown agent foo"}, "")

	require.Len(t, msgs, 4)
	assistantContent := extractText(msgs[2])
	fixContent := extractText(msgs[3])

	assert.Contains(t, assistantContent, "sessions:")
	assert.Contains(t, fixContent, "attempt 1")
	assert.Contains(t, fixContent, "unknown agent foo")
}

func TestBuildPrompt_IncludesAgentSystemPrompt(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "file-agent", Provider: "gpt",
				SystemPrompt: "You are a filesystem expert."},
		},
		Generator: GeneratorSettings{TestCount: 2, Complexity: "simple", MaxStepsPerTest: 3},
	}

	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 0, 1, "", nil, "")

	require.Len(t, msgs, 2)
	systemContent := extractText(msgs[0])

	// Agent system prompt must appear before the standard rules.
	assert.Contains(t, systemContent, "You are a filesystem expert.")
	agentCtxIdx := strings.Index(systemContent, "You are a filesystem expert.")
	standardIdx := strings.Index(systemContent, "You are a test generation expert")
	assert.Greater(t, standardIdx, agentCtxIdx, "agent context should precede standard prompt")
}

func TestBuildPrompt_IncludesSeed(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "agent1", Provider: "p"}},
		Generator: GeneratorSettings{TestCount: 2, Complexity: "medium", MaxStepsPerTest: 3},
	}

	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 42, 1, "", nil, "")
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, "42")
}

func TestBuildPrompt_IncludesPlan(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "agent1", Provider: "p"}},
		Generator: GeneratorSettings{TestCount: 2, Complexity: "medium", MaxStepsPerTest: 3},
	}

	plan := `{"sessions":[{"name":"S1","tests":[{"name":"T1","goal":"g","tools_expected":["read_file"],"assertions":[]}]}]}`
	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 0, 1, "", nil, plan)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, plan)
	assert.Contains(t, userContent, "TEST PLAN")
}

// ---------------------------------------------------------------------------
// TestFilterToolsByName
// ---------------------------------------------------------------------------

func TestFilterToolsByName_FiltersCorrectly(t *testing.T) {
	toolsByAgent := map[string][]mcp.Tool{
		"agent1": {
			{Name: "read_file"},
			{Name: "write_file"},
			{Name: "list_directory"},
		},
	}

	result := filterToolsByName(toolsByAgent, []string{"read_file", "list_directory"})

	require.Len(t, result["agent1"], 2)
	names := make([]string, 0, 2)
	for _, t := range result["agent1"] {
		names = append(names, t.Name)
	}
	assert.Contains(t, names, "read_file")
	assert.Contains(t, names, "list_directory")
	assert.NotContains(t, names, "write_file")
}

func TestFilterToolsByName_EmptyAllowlist(t *testing.T) {
	toolsByAgent := map[string][]mcp.Tool{
		"agent1": {
			{Name: "read_file"},
			{Name: "write_file"},
		},
	}

	// An empty allowlist means "include all tools" — the map is returned unchanged.
	result := filterToolsByName(toolsByAgent, []string{})
	assert.Len(t, result["agent1"], 2)
}

// ---------------------------------------------------------------------------
// mockLLM — test double for llms.Model
// ---------------------------------------------------------------------------

type mockLLM struct {
	responses []*llms.ContentResponse
	callIdx   int
}

func (m *mockLLM) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *mockLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses configured")
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// ---------------------------------------------------------------------------
// shared test fixtures for intent-pipeline tests
// ---------------------------------------------------------------------------

// testPlan1Test is a minimal valid plan JSON with one session and one test.
const testPlan1Test = `{"sessions":[{"name":"basic-tests","tests":[{"name":"list files","goal":"verify files are listed","tools_expected":[],"assertions":[]}]}]}`

// testValidIntentJSON is a valid TestIntent for the test in testPlan1Test.
const testValidIntentJSON = `{"name":"list files","session_name":"basic-tests","prompt":"List the files in /tmp","checks":[{"type":"no_error_messages"}]}`

// testBadIntentJSON parses as valid JSON/TestIntent but fails ValidateTestIntent.
const testBadIntentJSON = `{"name":"list files","session_name":"basic-tests","prompt":"List the files in /tmp","checks":[{"type":"fake_assertion_type"}]}`

// ---------------------------------------------------------------------------
// TestGenerateWithRetry — token limit tests (updated for intent pipeline)
// ---------------------------------------------------------------------------

func TestGenerateWithRetry_TokenLimitReached(t *testing.T) {
	// Plan call uses 50 tokens. Before the first intent attempt, the limit (10) is exceeded.
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{
				{
					Content:        testPlan1Test,
					GenerationInfo: map[string]any{"TotalTokens": 50},
				},
			}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  10, // 50 tokens used > 10 limit
			TestCount:  1,
			Complexity: "medium",
		},
	}

	_, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token limit")
	assert.Contains(t, err.Error(), "10")
	assert.Equal(t, 1, mock.callIdx, "expected 1 LLM call (plan only; intent blocked by token limit)")
}

func TestGenerateWithRetry_TokenLimitZeroIsUnlimited(t *testing.T) {
	// Plan call uses 999999 tokens. With MaxTokens==0 (unlimited), the intent call proceeds.
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{
				{
					Content:        testPlan1Test,
					GenerationInfo: map[string]any{"TotalTokens": 999999},
				},
			}},
			{Choices: []*llms.ContentChoice{
				{Content: testValidIntentJSON},
			}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0, // unlimited
			TestCount:  1,
			Complexity: "medium",
		},
	}

	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 2, mock.callIdx, "expected 2 LLM calls: plan + 1 intent")
}

func TestGenerateWithRetry_TokenLimitNotCheckedOnFirstSuccess(t *testing.T) {
	// Plan uses 5 explicit tokens (< MaxTokens=10), so the intent call proceeds.
	// The intent then uses 9999 tokens, pushing total well above MaxTokens.
	// Since the intent succeeded, no post-call check fires — the result is returned.
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{
				{
					Content:        testPlan1Test,
					GenerationInfo: map[string]any{"TotalTokens": 5},
				},
			}},
			{Choices: []*llms.ContentChoice{
				{
					Content:        testValidIntentJSON,
					GenerationInfo: map[string]any{"TotalTokens": 9999},
				},
			}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  10, // plan uses 5 (<10) so intent proceeds; no check after success
			TestCount:  1,
			Complexity: "medium",
		},
	}

	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err, "token limit is not checked after a successful call")
	assert.NotEmpty(t, result)
	assert.Equal(t, 2, mock.callIdx, "expected 2 LLM calls: plan + 1 intent")
}

// TestGenerateWithRetry_PlanPhase verifies that the plan phase is mandatory:
// a plan call is always made first, and its output drives intent generation.
func TestGenerateWithRetry_PlanPhase(t *testing.T) {
	planJSON := `{"sessions":[{"name":"S1","tests":[{"name":"T1","goal":"g","tools_expected":["read_file"],"assertions":[]}]}]}`
	intentJSON := `{"name":"T1","session_name":"S1","agent":"file-agent","prompt":"Read a file","checks":[{"type":"tool_called","tool":"read_file"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			// First call: plan phase
			{Choices: []*llms.ContentChoice{{Content: planJSON}}},
			// Second call: intent for T1
			{Choices: []*llms.ContentChoice{{Content: intentJSON}}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	// toolsByAgent must include read_file so intent validation passes.
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {
			{Name: "read_file"},
			{Name: "list_directory"},
		},
	}

	result, err := generateWithRetry(context.Background(), mock, cfg, toolsByAgent, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 2, mock.callIdx, "expected exactly 2 LLM calls (plan + 1 intent)")
}

// TestGenerateWithRetry_RepairRetried verifies that when an intent fails validation,
// one repair attempt is made per generation attempt, and success on repair is accepted.
// Sequence: plan → intent-fail → repair-success = 3 total LLM calls.
func TestGenerateWithRetry_RepairRetried(t *testing.T) {
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			// Call 0: plan
			{Choices: []*llms.ContentChoice{{Content: testPlan1Test}}},
			// Call 1: intent attempt 1 → fails validation (unknown check type)
			{Choices: []*llms.ContentChoice{{Content: testBadIntentJSON}}},
			// Call 2: repair attempt 1 → valid intent
			{Choices: []*llms.ContentChoice{{Content: testValidIntentJSON}}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 3, mock.callIdx, "expected 3 LLM calls: plan + 1 intent gen + 1 repair")
}

// TestGenerateWithRetry_RepairExhaustedReturnsError verifies that when every
// generation + repair cycle fails, an error is returned.
// MaxRetries=3: plan + 3*(1 gen + 1 repair) = 7 total calls.
func TestGenerateWithRetry_RepairExhaustedReturnsError(t *testing.T) {
	responses := []*llms.ContentResponse{
		// Call 0: plan
		{Choices: []*llms.ContentChoice{{Content: testPlan1Test}}},
	}
	// Calls 1-6: alternating gen/repair failures (all return bad intent)
	for i := 0; i < 6; i++ {
		responses = append(responses, &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{Content: testBadIntentJSON}},
		})
	}

	mock := &mockLLM{responses: responses}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	_, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate intent")
	assert.Equal(t, 7, mock.callIdx, "expected 7 LLM calls: 1 plan + 3*(1 gen + 1 repair)")
}

// TestGenerateWithRetry_StructuralFailureRetriesGeneration verifies that a
// structurally broken response (JSON parse error) skips repair and retries generation.
func TestGenerateWithRetry_StructuralFailureRetriesGeneration(t *testing.T) {
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			// Call 0: plan
			{Choices: []*llms.ContentChoice{{Content: testPlan1Test}}},
			// Call 1: intent → structural failure (not valid JSON)
			{Choices: []*llms.ContentChoice{{Content: "not valid json: :::"}}},
			// Call 2: intent retry → valid
			{Choices: []*llms.ContentChoice{{Content: testValidIntentJSON}}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 3, mock.callIdx, "expected 3 LLM calls: plan + 1 structural fail + 1 success")
}

// TestGenerateWithRetry_StructuralFailureExhausted verifies that when all
// generation attempts produce structurally broken JSON, an error is returned.
func TestGenerateWithRetry_StructuralFailureExhausted(t *testing.T) {
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			// Call 0: plan
			{Choices: []*llms.ContentChoice{{Content: testPlan1Test}}},
			// Calls 1-3: all structural JSON failures
			{Choices: []*llms.ContentChoice{{Content: "not valid json: :::"}}},
			{Choices: []*llms.ContentChoice{{Content: "not valid json: :::"}}},
			{Choices: []*llms.ContentChoice{{Content: "not valid json: :::"}}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 3,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	_, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate intent")
	assert.Equal(t, 4, mock.callIdx, "expected 4 LLM calls: 1 plan + 3 structural failures")
}

// TestGenerateWithRetry_PlanExhaustedReturnsError verifies that when plan generation
// repeatedly fails (MaxRetries=2), an error is returned — no fallback to one-phase.
func TestGenerateWithRetry_PlanExhaustedReturnsError(t *testing.T) {
	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			// Call 1: plan attempt 1 → bad JSON (fails ValidatePlan)
			{Choices: []*llms.ContentChoice{{Content: "not valid json"}}},
			// Call 2: plan attempt 2 → bad JSON (fails ValidatePlan)
			{Choices: []*llms.ContentChoice{{Content: "not valid json"}}},
		},
	}

	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{
			MaxRetries: 2,
			MaxTokens:  0,
			TestCount:  1,
			Complexity: "medium",
		},
	}

	_, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.Error(t, err, "exhausted plan retries should return an error")
	assert.Contains(t, err.Error(), "plan")
	assert.Equal(t, 2, mock.callIdx, "expected 2 LLM calls (plan attempts exhausted)")
}

// ---------------------------------------------------------------------------
// TestBuildSessions — intent.go tests
// ---------------------------------------------------------------------------

func TestBuildSessions_SingleSession(t *testing.T) {
	intents := []TestIntent{
		{
			Name:        "test1",
			SessionName: "session-a",
			Prompt:      "Do something",
			Checks: []Check{
				{Type: "no_error_messages"},
			},
		},
	}
	sessions := BuildSessions(intents, nil)
	require.Len(t, sessions, 1)
	assert.Equal(t, "session-a", sessions[0].Name)
	require.Len(t, sessions[0].Tests, 1)
	assert.Equal(t, "test1", sessions[0].Tests[0].Name)
	assert.Equal(t, "Do something", sessions[0].Tests[0].Prompt)
	require.Len(t, sessions[0].Tests[0].Assertions, 1)
	assert.Equal(t, "no_error_messages", sessions[0].Tests[0].Assertions[0].Type)
}

func TestBuildSessions_MultiSession(t *testing.T) {
	intents := []TestIntent{
		{Name: "t1", SessionName: "s1", Prompt: "p1", Checks: []Check{{Type: "no_error_messages"}}},
		{Name: "t2", SessionName: "s2", Prompt: "p2", Checks: []Check{{Type: "no_error_messages"}}},
	}
	sessions := BuildSessions(intents, nil)
	require.Len(t, sessions, 2)
	assert.Equal(t, "s1", sessions[0].Name)
	assert.Equal(t, "s2", sessions[1].Name)
}

func TestBuildSessions_PreservesInsertionOrder(t *testing.T) {
	// "b-session" appears first, so it should be first in output regardless of name sort order.
	intents := []TestIntent{
		{Name: "t1", SessionName: "b-session", Prompt: "p", Checks: []Check{{Type: "no_error_messages"}}},
		{Name: "t2", SessionName: "a-session", Prompt: "p", Checks: []Check{{Type: "no_error_messages"}}},
		{Name: "t3", SessionName: "b-session", Prompt: "p", Checks: []Check{{Type: "no_error_messages"}}},
	}
	sessions := BuildSessions(intents, nil)
	require.Len(t, sessions, 2)
	assert.Equal(t, "b-session", sessions[0].Name, "first-seen session should be first")
	assert.Equal(t, "a-session", sessions[1].Name)
	require.Len(t, sessions[0].Tests, 2, "b-session should have t1 and t3")
	assert.Equal(t, "t1", sessions[0].Tests[0].Name)
	assert.Equal(t, "t3", sessions[0].Tests[1].Name)
}

func TestBuildSessions_EmptyIntents(t *testing.T) {
	sessions := BuildSessions(nil, nil)
	assert.Empty(t, sessions)
}

func TestBuildSessions_ExtractorsConverted(t *testing.T) {
	intents := []TestIntent{
		{
			Name:        "create",
			SessionName: "s1",
			Prompt:      "Create record",
			Checks:      []Check{{Type: "tool_called", Tool: "create_record"}},
			Extractors: []ExtractorIntent{
				{Tool: "create_record", Path: "$.id", VariableName: "recordId"},
			},
		},
	}
	sessions := BuildSessions(intents, nil)
	require.Len(t, sessions, 1)
	require.Len(t, sessions[0].Tests[0].Extractors, 1)
	ex := sessions[0].Tests[0].Extractors[0]
	assert.Equal(t, "jsonpath", ex.ExtractorType)
	assert.Equal(t, "create_record", ex.Tool)
	assert.Equal(t, "$.id", ex.Path)
	assert.Equal(t, "recordId", ex.VariableName)
}

func TestBuildSessions_AllowedToolsPreserved(t *testing.T) {
	intents := []TestIntent{
		{
			Name:         "t1",
			SessionName:  "s1",
			Prompt:       "p",
			Checks:       []Check{{Type: "no_error_messages"}},
			AllowedTools: []string{"tool_a", "tool_b"},
		},
	}
	sessions := BuildSessions(intents, nil)
	assert.Equal(t, []string{"tool_a", "tool_b"}, sessions[0].Tests[0].AllowedTools)
}

func TestBuildAssertion_ToolCalled(t *testing.T) {
	c := Check{Type: "tool_called", Tool: "read_file"}
	a := BuildAssertion(c)
	assert.Equal(t, "tool_called", a.Type)
	assert.Equal(t, "read_file", a.Tool)
}

func TestBuildAssertion_ToolParamEquals(t *testing.T) {
	c := Check{
		Type:   "tool_param_equals",
		Tool:   "write_file",
		Params: map[string]json.RawMessage{"path": json.RawMessage(`"/tmp/out.txt"`)},
	}
	a := BuildAssertion(c)
	assert.Equal(t, "tool_param_equals", a.Type)
	assert.Equal(t, "write_file", a.Tool)
	assert.Equal(t, "/tmp/out.txt", a.Params["path"])
}

func TestBuildAssertion_ToolParamEquals_JSONStringFlattened(t *testing.T) {
	// LLM sometimes encodes nested params as a JSON-encoded string; verify they get flattened.
	c := Check{
		Type:   "tool_param_equals",
		Tool:   "some_tool",
		Params: map[string]json.RawMessage{"args": json.RawMessage(`"{\"service_name\":\"MyNewService\",\"workspace_id\":\"123\"}"`)},
	}
	a := BuildAssertion(c)
	assert.Equal(t, "MyNewService", a.Params["args.service_name"])
	assert.Equal(t, "123", a.Params["args.workspace_id"])
	assert.NotContains(t, a.Params, "args")
}

func TestBuildAssertion_ToolCallCount(t *testing.T) {
	c := Check{Type: "tool_call_count", Tool: "list_dir", Count: 2}
	a := BuildAssertion(c)
	assert.Equal(t, "tool_call_count", a.Type)
	assert.Equal(t, 2, a.Count)
}

func TestBuildAssertion_OutputContains(t *testing.T) {
	c := Check{Type: "output_contains", Value: "success"}
	a := BuildAssertion(c)
	assert.Equal(t, "output_contains", a.Type)
	assert.Equal(t, "success", a.Value)
}

func TestBuildAssertion_CLIExitCode(t *testing.T) {
	c := Check{Type: "cli_exit_code_equals", Expected: 0}
	a := BuildAssertion(c)
	assert.Equal(t, "cli_exit_code_equals", a.Type)
	assert.Equal(t, 0, a.Expected)
}

func TestBuildAssertion_MaxTokens(t *testing.T) {
	c := Check{Type: "max_tokens", Count: 1000}
	a := BuildAssertion(c)
	assert.Equal(t, "max_tokens", a.Type)
	assert.Equal(t, 1000, a.Count)
}

// ---------------------------------------------------------------------------
// TestValidateTestIntent — intent_prompt.go tests
// ---------------------------------------------------------------------------

// makeIntentTools returns a toolsByAgent with one "read_file" tool.
func makeIntentTools() map[string][]mcp.Tool {
	tool := mcp.Tool{Name: "read_file", Description: "Read a file"}
	tool.InputSchema.Properties = map[string]any{
		"path": map[string]any{"type": "string"},
	}
	tool.InputSchema.Required = []string{"path"}
	return map[string][]mcp.Tool{
		"file-agent": {tool},
	}
}

func TestValidateTestIntent_Valid(t *testing.T) {
	intent := TestIntent{
		Name:        "Read file test",
		SessionName: "s1",
		Prompt:      "Read the file",
		Checks:      []Check{{Type: "tool_called", Tool: "read_file"}},
	}
	errs := ValidateTestIntent(intent, makeIntentTools(), []string{"file-agent"}, map[string]bool{})
	assert.Empty(t, errs)
}

func TestValidateTestIntent_MissingName(t *testing.T) {
	intent := TestIntent{
		Prompt: "Do something",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "missing name"))
}

func TestValidateTestIntent_MissingPrompt(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "missing prompt"))
}

func TestValidateTestIntent_NoChecks(t *testing.T) {
	intent := TestIntent{Name: "test", Prompt: "Do it"}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "no checks"))
}

func TestValidateTestIntent_UnknownAgent(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Agent:  "ghost-agent",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	errs := ValidateTestIntent(intent, nil, []string{"file-agent"}, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost-agent"))
}

func TestValidateTestIntent_UnknownCheckType(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{{Type: "fake_check_type"}},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "fake_check_type"))
}

func TestValidateTestIntent_ForbiddenCombinator(t *testing.T) {
	for _, combinator := range []string{"anyOf", "allOf", "not"} {
		t.Run(combinator, func(t *testing.T) {
			intent := TestIntent{
				Name:   "test",
				Prompt: "Do it",
				Checks: []Check{{Type: combinator}},
			}
			errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
			assert.NotEmpty(t, errs)
			assert.True(t, containsSubstring(errs, "forbidden"),
				"should flag %s as forbidden combinator", combinator)
		})
	}
}

func TestValidateTestIntent_UnknownToolInCheck(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{{Type: "tool_called", Tool: "ghost_tool"}},
	}
	errs := ValidateTestIntent(intent, makeIntentTools(), nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"))
}

func TestValidateTestIntent_UnknownParamInCheck(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{
			{
				Type:   "tool_param_equals",
				Tool:   "read_file",
				Params: map[string]json.RawMessage{"ghost_param": json.RawMessage(`"value"`)},
			},
		},
	}
	errs := ValidateTestIntent(intent, makeIntentTools(), nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_param"))
}

func TestValidateTestIntent_ToolCallOrderBadTool(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{
			{Type: "tool_call_order", Sequence: []string{"read_file", "ghost_tool"}},
		},
	}
	errs := ValidateTestIntent(intent, makeIntentTools(), nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"))
}

func TestValidateTestIntent_UndefinedVarInPrompt(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Process file {{undefinedVar}}",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "undefinedVar"))
}

func TestValidateTestIntent_DefinedVarInPrompt(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Process file {{filename}}",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	sessionVars := map[string]bool{"filename": true}
	errs := ValidateTestIntent(intent, nil, nil, sessionVars)
	assert.Empty(t, errs)
}

func TestValidateTestIntent_UndefinedVarInCheckValue(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{
			{Type: "output_contains", Value: "{{undefinedVar}}"},
		},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "undefinedVar"))
}

func TestValidateTestIntent_ExtractorUnknownTool(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{{Type: "no_error_messages"}},
		Extractors: []ExtractorIntent{
			{Tool: "ghost_tool", Path: "$.id", VariableName: "myId"},
		},
	}
	errs := ValidateTestIntent(intent, makeIntentTools(), nil, map[string]bool{})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "ghost_tool"))
}

func TestValidateTestIntent_BuiltinVarsAllowed(t *testing.T) {
	// Built-in template variables must not be flagged as undefined.
	intent := TestIntent{
		Name:   "test",
		Prompt: "Write to {{TEST_DIR}}/{{RUN_ID}}.txt",
		Checks: []Check{{Type: "no_error_messages"}},
	}
	errs := ValidateTestIntent(intent, nil, nil, map[string]bool{})
	assert.Empty(t, errs, "built-in template vars should not be flagged")
}

// ---------------------------------------------------------------------------
// TestBuildTestIntentPrompt — intent_prompt.go tests
// ---------------------------------------------------------------------------

func TestBuildTestIntentPrompt_ContainsScenarioGoal(t *testing.T) {
	pt := planTest{
		Name:          "Create record",
		Goal:          "Verify that a new record is created",
		ToolsExpected: []string{"create_record"},
	}
	msgs := BuildTestIntentPrompt(pt, "s1", "file-agent", []string{"file-agent"},
		map[string][]mcp.Tool{}, map[string]bool{}, &GeneratorConfig{}, nil)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "Verify that a new record is created")
	assert.Contains(t, userContent, "Create record")
}

func TestBuildTestIntentPrompt_ContainsToolSection(t *testing.T) {
	pt := planTest{Name: "T1", Goal: "g"}
	toolsByAgent := map[string][]mcp.Tool{
		"file-agent": {{Name: "read_file", Description: "Read a file"}},
	}
	msgs := BuildTestIntentPrompt(pt, "s1", "file-agent", []string{"file-agent"},
		toolsByAgent, map[string]bool{}, &GeneratorConfig{}, nil)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "read_file")
	assert.Contains(t, userContent, "file-agent")
}

// ---------------------------------------------------------------------------
// TestBuildTestIntentRepairPrompt — intent_prompt.go tests
// ---------------------------------------------------------------------------

func TestBuildTestIntentRepairPrompt_ContainsErrors(t *testing.T) {
	intent := TestIntent{
		Name:   "test",
		Prompt: "Do it",
		Checks: []Check{{Type: "fake_type"}},
	}
	errs := []string{`check[0]: unknown check type "fake_type"`, "intent: missing agent"}
	msgs := BuildTestIntentRepairPrompt(intent, errs, map[string][]mcp.Tool{}, []string{"file-agent"})

	require.Len(t, msgs, 2)
	assert.Equal(t, llms.ChatMessageTypeSystem, msgs[0].Role)
	content := extractText(msgs[1])
	assert.Contains(t, content, "fake_type")
	assert.Contains(t, content, "missing agent")
}

func TestBuildTestIntentRepairPrompt_HasSystemPrompt(t *testing.T) {
	intent := TestIntent{Name: "t", Prompt: "p", Checks: []Check{{Type: "output_contains", Value: "x"}}}
	msgs := BuildTestIntentRepairPrompt(intent, []string{"some error"}, map[string][]mcp.Tool{}, nil)

	require.Len(t, msgs, 2)
	sysContent := extractText(msgs[0])
	assert.Contains(t, sysContent, "OUTPUT RULES")
	assert.Contains(t, sysContent, "Start your output with: {")
}

// ---------------------------------------------------------------------------
// TestBuildTestIntentParseRepairPrompt — intent_prompt.go tests
// ---------------------------------------------------------------------------

func TestBuildTestIntentParseRepairPrompt_ContainsSimplificationInstruction(t *testing.T) {
	pt := planTest{
		Name:          "Complex Flow",
		Goal:          "Verify multi-step workflow",
		ToolsExpected: []string{"tool_a", "tool_b"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"agent1": {{Name: "tool_a", Description: "Do A"}},
	}
	msgs := BuildTestIntentParseRepairPrompt(pt, toolsByAgent, []string{"agent1"})

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "truncated")
	assert.Contains(t, userContent, "SIMPLER")
	assert.Contains(t, userContent, "Complex Flow")
	assert.Contains(t, userContent, "tool_a")
}

func TestBuildTestIntentPrompt_PrevErrorsSection(t *testing.T) {
	pt := planTest{Name: "T1", Goal: "g"}
	prevErrors := []string{`check[0]: param "service_name" not found in tool "svc"'s input schema`}
	msgs := BuildTestIntentPrompt(pt, "s1", "agent1", []string{"agent1"},
		map[string][]mcp.Tool{}, map[string]bool{}, &GeneratorConfig{}, prevErrors)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "PREVIOUS ATTEMPT FAILED")
	assert.Contains(t, userContent, "service_name")
}

func TestBuildTestIntentPrompt_NoPrevErrorsSection(t *testing.T) {
	pt := planTest{Name: "T1", Goal: "g"}
	msgs := BuildTestIntentPrompt(pt, "s1", "agent1", []string{"agent1"},
		map[string][]mcp.Tool{}, map[string]bool{}, &GeneratorConfig{}, nil)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.NotContains(t, userContent, "PREVIOUS ATTEMPT FAILED")
}

func TestBuildTestIntentPrompt_SuppressPlanAssertionsOnParamError(t *testing.T) {
	pt := planTest{
		Name:       "Create service",
		Goal:       "Verify service creation",
		Assertions: []string{"tool_param_equals: workspace_id=abc"},
	}
	prevErrors := []string{`check[0]: param "workspace_id" not found in tool "create_service"'s input schema`}
	msgs := BuildTestIntentPrompt(pt, "s1", "agent1", []string{"agent1"},
		map[string][]mcp.Tool{}, map[string]bool{}, &GeneratorConfig{}, prevErrors)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	// Warning must be present; original hint must be absent.
	assert.Contains(t, userContent, "IMPORTANT: The plan's suggested assertions contained wrong parameter names.")
	assert.Contains(t, userContent, "Use ONLY parameter names shown in AGENT TOOLS above.")
	assert.NotContains(t, userContent, "workspace_id=abc")
}

func TestBuildTestIntentPrompt_ShowsPlanAssertionsWithoutParamError(t *testing.T) {
	pt := planTest{
		Name:       "List services",
		Goal:       "Verify listing works",
		Assertions: []string{"tool_called: list_services"},
	}
	// Non-param error should not suppress assertions.
	prevErrors := []string{`check[0]: unknown check type "tool_present"`}
	msgs := BuildTestIntentPrompt(pt, "s1", "agent1", []string{"agent1"},
		map[string][]mcp.Tool{}, map[string]bool{}, &GeneratorConfig{}, prevErrors)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "Suggested assertions:")
	assert.Contains(t, userContent, "tool_called: list_services")
	assert.NotContains(t, userContent, "IMPORTANT: The plan's suggested assertions contained wrong parameter names.")
}

// ---------------------------------------------------------------------------
// TestBuildParamReference — intent_prompt.go helper tests
// ---------------------------------------------------------------------------

func TestBuildParamReference_SingleTool(t *testing.T) {
	tool := mcp.Tool{Name: "create_service"}
	tool.InputSchema.Properties = map[string]any{
		"name":         map[string]any{"type": "string"},
		"region":       map[string]any{"type": "string"},
		"service_type": map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {tool},
	}
	result := buildParamReference(toolsByAgent, []string{"my-agent"})

	assert.Contains(t, result, "PARAM REFERENCE")
	assert.Contains(t, result, `Tool "create_service":`)
	assert.Contains(t, result, "name")
	assert.Contains(t, result, "region")
	assert.Contains(t, result, "service_type")
}

func TestBuildParamReference_EmptyTools(t *testing.T) {
	// Tool with no properties — helper must return empty string.
	tool := mcp.Tool{Name: "no_params_tool"}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {tool},
	}
	result := buildParamReference(toolsByAgent, []string{"my-agent"})
	assert.Empty(t, result)
}

func TestBuildTestIntentPrompt_ParamReferenceSectionPresent(t *testing.T) {
	tool := mcp.Tool{Name: "create_service"}
	tool.InputSchema.Properties = map[string]any{
		"service_type": map[string]any{"type": "string"},
		"name":         map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {tool},
	}
	prevErrors := []string{`check[0]: param "workspace_id" not found in tool "create_service"'s input schema`}

	pt := planTest{Name: "T1", Goal: "g"}
	msgs := BuildTestIntentPrompt(pt, "s1", "my-agent", []string{"my-agent"},
		toolsByAgent, map[string]bool{}, &GeneratorConfig{}, prevErrors)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "PARAM REFERENCE")
	assert.Contains(t, userContent, `Tool "create_service":`)
	assert.Contains(t, userContent, "name")
	assert.Contains(t, userContent, "service_type")
}

func TestBuildTestIntentPrompt_ParamReferenceSectionAbsent(t *testing.T) {
	// When prevErrors is nil (first attempt), no PARAM REFERENCE section should appear.
	tool := mcp.Tool{Name: "create_service"}
	tool.InputSchema.Properties = map[string]any{
		"service_type": map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {tool},
	}
	pt := planTest{Name: "T1", Goal: "g"}
	msgs := BuildTestIntentPrompt(pt, "s1", "my-agent", []string{"my-agent"},
		toolsByAgent, map[string]bool{}, &GeneratorConfig{}, nil)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.NotContains(t, userContent, "PARAM REFERENCE")
}

func TestBuildTestIntentParseRepairPrompt_ContainsParamReference(t *testing.T) {
	// Parse-repair prompt must always contain the PARAM REFERENCE section when tools have params.
	tool := mcp.Tool{Name: "create_service"}
	tool.InputSchema.Properties = map[string]any{
		"service_type": map[string]any{"type": "string"},
		"name":         map[string]any{"type": "string"},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {tool},
	}
	pt := planTest{Name: "T1", Goal: "g"}
	msgs := BuildTestIntentParseRepairPrompt(pt, toolsByAgent, []string{"my-agent"})

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])
	assert.Contains(t, userContent, "PARAM REFERENCE")
	assert.Contains(t, userContent, `Tool "create_service":`)
	assert.Contains(t, userContent, "name")
	assert.Contains(t, userContent, "service_type")
}

// ---------------------------------------------------------------------------
// capturingMockLLM — records conversation messages per call
// ---------------------------------------------------------------------------

type capturingMockLLM struct {
	responses    []*llms.ContentResponse
	callIdx      int
	capturedMsgs [][]llms.MessageContent // one entry per GenerateContent call
}

func (m *capturingMockLLM) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *capturingMockLLM) GenerateContent(_ context.Context, msgs []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses configured")
	}
	m.capturedMsgs = append(m.capturedMsgs, msgs)
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// ---------------------------------------------------------------------------
// TestGenerateWithRetry — integration tests for the intent pipeline
// ---------------------------------------------------------------------------

func TestGenerateWithRetry_ParseRepairAfterConsecutiveParseFails(t *testing.T) {
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	truncated := `{"name":"t1"` // truncated — fails json.Unmarshal
	validIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"no_error_messages"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},
			{Choices: []*llms.ContentChoice{{Content: truncated}}},   // parse fail #1 — no repair yet
			{Choices: []*llms.ContentChoice{{Content: truncated}}},   // parse fail #2 → triggers parse repair
			{Choices: []*llms.ContentChoice{{Content: validIntent}}}, // parse repair call → success
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 5, TestCount: 1, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 4, mock.callIdx, "expected 4 calls: plan + 2 parse-fails + 1 parse-repair")
}

func TestGenerateWithRetry_ValidationErrCarriedIntoRetryPrompt(t *testing.T) {
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	badIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"fake_type"}]}`
	goodIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"no_error_messages"}]}`

	mock := &capturingMockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},       // plan call (idx 0)
			{Choices: []*llms.ContentChoice{{Content: badIntent}}},  // gen attempt 1 → validation fail (idx 1)
			{Choices: []*llms.ContentChoice{{Content: badIntent}}},  // repair attempt 1 → still invalid (idx 2)
			{Choices: []*llms.ContentChoice{{Content: goodIntent}}}, // gen attempt 2 → success (idx 3)
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 3, TestCount: 1, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// capturedMsgs[3] is gen attempt 2 (plan=0, gen1=1, repair1=2, gen2=3).
	require.Len(t, mock.capturedMsgs, 4, "expected 4 LLM calls")
	gen2UserContent := extractText(mock.capturedMsgs[3][1]) // user message of gen attempt 2
	assert.Contains(t, gen2UserContent, "PREVIOUS ATTEMPT FAILED")
	assert.Contains(t, gen2UserContent, "fake_type")
}

func TestGenerateWithRetry_IntentPipeline_Success(t *testing.T) {
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	intent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"no_error_messages"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},
			{Choices: []*llms.ContentChoice{{Content: intent}}},
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 3, TestCount: 1, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "sessions:")
	assert.Equal(t, 2, mock.callIdx)
}

func TestGenerateWithRetry_PerTestRetry(t *testing.T) {
	// Structural failure (JSON parse error) for intent → skip repair → retry → success.
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	validIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"no_error_messages"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},
			{Choices: []*llms.ContentChoice{{Content: "not-json:::"}}},
			{Choices: []*llms.ContentChoice{{Content: validIntent}}},
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 3, TestCount: 1, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 3, mock.callIdx, "expected 3 calls: plan + struct-fail + success")
}

func TestGenerateWithRetry_RepairIntent(t *testing.T) {
	// Validation failure for intent → repair → repair succeeds.
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	badIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"fake_type"}]}`
	goodIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"no_error_messages"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},
			{Choices: []*llms.ContentChoice{{Content: badIntent}}},
			{Choices: []*llms.ContentChoice{{Content: goodIntent}}},
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 3, TestCount: 1, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Equal(t, 3, mock.callIdx, "expected 3 calls: plan + gen-fail + repair-success")
}

func TestGenerateWithRetry_TestExhausted(t *testing.T) {
	// All retries for the single test exhausted (MaxRetries=2 → 1 plan + 2*(1 gen + 1 repair) = 5).
	plan := `{"sessions":[{"name":"s1","tests":[{"name":"t1","goal":"g","tools_expected":[],"assertions":[]}]}]}`
	badIntent := `{"name":"t1","session_name":"s1","prompt":"Do something","checks":[{"type":"fake_type"}]}`

	responses := []*llms.ContentResponse{
		{Choices: []*llms.ContentChoice{{Content: plan}}},
	}
	for i := 0; i < 4; i++ { // 2 attempts × (1 gen + 1 repair)
		responses = append(responses, &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{Content: badIntent}},
		})
	}

	mock := &mockLLM{responses: responses}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 2, TestCount: 1, Complexity: "medium"},
	}
	_, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate intent")
	assert.Equal(t, 5, mock.callIdx, "expected 5 calls: 1 plan + 2*(1 gen + 1 repair)")
}

func TestGenerateWithRetry_SessionVarFlow(t *testing.T) {
	// Test 1 extracts a variable; test 2 uses it in prompt — should succeed without var errors.
	plan := `{"sessions":[{"name":"lifecycle","tests":[` +
		`{"name":"create","goal":"create record","tools_expected":[],"assertions":[]},` +
		`{"name":"fetch","goal":"fetch record","tools_expected":[],"assertions":[]}` +
		`]}]}`
	intent1 := `{"name":"create","session_name":"lifecycle","prompt":"Create a record","checks":[{"type":"no_error_messages"}],"extractors":[{"tool":"create_tool","path":"$.id","variable_name":"recordId"}]}`
	intent2 := `{"name":"fetch","session_name":"lifecycle","prompt":"Fetch {{recordId}}","checks":[{"type":"no_error_messages"}]}`

	mock := &mockLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: plan}}},
			{Choices: []*llms.ContentChoice{{Content: intent1}}},
			{Choices: []*llms.ContentChoice{{Content: intent2}}},
		},
	}
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "file-agent", Provider: "p"}},
		Generator: GeneratorSettings{MaxRetries: 3, TestCount: 2, Complexity: "medium"},
	}
	result, err := generateWithRetry(context.Background(), mock, cfg, nil, []string{"file-agent"}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "recordId")
	assert.Equal(t, 3, mock.callIdx, "expected 3 calls: plan + 2 intents")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func containsSubstring(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// extractText pulls the text content from the first part of an llms.MessageContent.
func extractText(msg llms.MessageContent) string {
	for _, part := range msg.Parts {
		if tc, ok := part.(llms.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// TestBuildPlanChunkPrompt
// ---------------------------------------------------------------------------

func TestBuildPlanChunkPrompt_NoAlreadyPlanned(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "my-agent", Provider: "gpt"},
		},
		Generator: GeneratorSettings{
			TestCount:     10,
			PlanChunkSize: 3,
			Complexity:    "medium",
		},
	}
	toolsByAgent := map[string][]mcp.Tool{
		"my-agent": {
			{Name: "read_file", Description: "Read a file from disk"},
			{Name: "write_file", Description: "Write content to a file"},
		},
	}

	msgs := BuildPlanChunkPrompt(cfg, toolsByAgent, 3, nil)

	require.Len(t, msgs, 2, "chunk prompt should have system + user message")

	userContent := extractText(msgs[1])

	// Should contain tool names.
	assert.Contains(t, userContent, "read_file")
	assert.Contains(t, userContent, "write_file")
	assert.Contains(t, userContent, "my-agent")

	// Should have constraint with chunk size.
	assert.Contains(t, userContent, "3")

	// Should NOT have the already-planned section when nil is passed.
	assert.NotContains(t, userContent, "ALREADY PLANNED TESTS")
}

func TestBuildPlanChunkPrompt_WithAlreadyPlanned(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{
			{Name: "my-agent", Provider: "gpt"},
		},
		Generator: GeneratorSettings{
			TestCount:     10,
			PlanChunkSize: 5,
			Complexity:    "simple",
		},
	}

	alreadyPlanned := []planTest{
		{Name: "Test Alpha", Goal: "verify read works"},
		{Name: "Test Beta", Goal: "verify write works"},
	}

	msgs := BuildPlanChunkPrompt(cfg, map[string][]mcp.Tool{}, 5, alreadyPlanned)

	require.Len(t, msgs, 2)
	userContent := extractText(msgs[1])

	// Already-planned section must be present.
	assert.Contains(t, userContent, "ALREADY PLANNED TESTS")
	assert.Contains(t, userContent, "Test Alpha")
	assert.Contains(t, userContent, "verify read works")
	assert.Contains(t, userContent, "Test Beta")
	assert.Contains(t, userContent, "verify write works")

	// Constraint line references total count and offset.
	assert.Contains(t, userContent, "10") // total tests
	assert.Contains(t, userContent, "5")  // chunk size
}

func TestBuildPlanChunkPrompt_IncludesGoalAndEdgeCases(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents: []model.Agent{{Name: "agent1", Provider: "p"}},
		Generator: GeneratorSettings{
			TestCount:        6,
			PlanChunkSize:    3,
			Complexity:       "complex",
			IncludeEdgeCases: true,
			Goal:             "Focus on error handling",
		},
	}

	msgs := BuildPlanChunkPrompt(cfg, map[string][]mcp.Tool{}, 3, nil)
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, "edge cases")
	assert.Contains(t, userContent, "Focus on error handling")
	assert.Contains(t, userContent, "complex")
}

// ---------------------------------------------------------------------------
// TestMergePlanChunks
// ---------------------------------------------------------------------------

func TestMergePlanChunks_SameSession(t *testing.T) {
	chunk1 := planWrapper{
		Sessions: []planSession{
			{
				Name: "session-a",
				Tests: []planTest{
					{Name: "test-1", Goal: "goal 1"},
					{Name: "test-2", Goal: "goal 2"},
				},
			},
		},
	}
	chunk2 := planWrapper{
		Sessions: []planSession{
			{
				Name: "session-a",
				Tests: []planTest{
					{Name: "test-3", Goal: "goal 3"},
				},
			},
		},
	}

	merged := mergePlanChunks([]planWrapper{chunk1, chunk2})

	require.Len(t, merged.Sessions, 1, "same-name sessions should be merged into one")
	assert.Equal(t, "session-a", merged.Sessions[0].Name)
	require.Len(t, merged.Sessions[0].Tests, 3, "all tests from both chunks should be present")
	assert.Equal(t, "test-1", merged.Sessions[0].Tests[0].Name)
	assert.Equal(t, "test-2", merged.Sessions[0].Tests[1].Name)
	assert.Equal(t, "test-3", merged.Sessions[0].Tests[2].Name)
}

func TestMergePlanChunks_DifferentSessions(t *testing.T) {
	chunk1 := planWrapper{
		Sessions: []planSession{
			{Name: "session-a", Tests: []planTest{{Name: "test-1"}}},
		},
	}
	chunk2 := planWrapper{
		Sessions: []planSession{
			{Name: "session-b", Tests: []planTest{{Name: "test-2"}}},
		},
	}

	merged := mergePlanChunks([]planWrapper{chunk1, chunk2})

	require.Len(t, merged.Sessions, 2, "different sessions should both be preserved")
	names := []string{merged.Sessions[0].Name, merged.Sessions[1].Name}
	assert.Contains(t, names, "session-a")
	assert.Contains(t, names, "session-b")
}

func TestMergePlanChunks_Empty(t *testing.T) {
	merged := mergePlanChunks(nil)
	assert.Empty(t, merged.Sessions)
}

// ---------------------------------------------------------------------------
// TestParseGeneratorConfig_PlanChunkSize
// ---------------------------------------------------------------------------

func TestParseGeneratorConfig_PlanChunkSizeDefault(t *testing.T) {
	content := `
providers:
  - name: gpt
    type: OPENAI
    token: "sk-test"
    model: gpt-4o

agents:
  - name: my-agent
    provider: gpt
`
	path := writeTempYAML(t, content)
	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.Generator.PlanChunkSize, "default PlanChunkSize should be 5")
}

func TestParseGeneratorConfig_PlanChunkSizeExplicit(t *testing.T) {
	content := `
providers:
  - name: gpt
    type: OPENAI
    token: "sk-test"
    model: gpt-4o

agents:
  - name: my-agent
    provider: gpt

generator:
  plan_chunk_size: 8
`
	path := writeTempYAML(t, content)
	cfg, err := ParseGeneratorConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.Generator.PlanChunkSize)
}

// ---------------------------------------------------------------------------
// TestIsTokenLimitStopReason
// ---------------------------------------------------------------------------

func TestIsTokenLimitStopReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		// OpenAI / Groq
		{"length", true},
		{"LENGTH", true}, // case-insensitive
		// Anthropic / Google
		{"max_tokens", true},
		{"MAX_TOKENS", true}, // case-insensitive
		// Vertex AI
		{"FinishReasonMaxTokens", true},
		{"finishreason_maxtokens", true}, // contains "maxtokens"
		// non-matching
		{"stop", false},
		{"end_turn", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			assert.Equal(t, tc.want, isTokenLimitStopReason(tc.reason),
				"isTokenLimitStopReason(%q)", tc.reason)
		})
	}
}

// ---------------------------------------------------------------------------
// TestContinueJSONResponse
// ---------------------------------------------------------------------------

func TestContinueJSONResponse_CompletesOnFirstContinuation(t *testing.T) {
	// Simulate: initial generation was cut off mid-JSON (stop reason = "length").
	// The continuation call returns the rest of the JSON and finishes naturally.
	partial := `{"name":"test","session_name":"s1","prompt":"Do it","checks":[`

	continuation := `{"type":"no_error_messages"}]}`
	contResp := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: continuation, StopReason: "stop"}, // natural finish
		},
	}

	mock := &mockLLM{responses: []*llms.ContentResponse{contResp}}

	conversation := []llms.MessageContent{
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: "generate intent"}}},
	}
	tokens := 0
	result, err := continueJSONResponse(context.Background(), mock, conversation, partial, 3, 0, &tokens)

	require.NoError(t, err)
	assert.Equal(t, 1, mock.callIdx, "expected exactly 1 continuation call")

	// Result must be valid JSON.
	var probe interface{}
	require.NoError(t, json.Unmarshal([]byte(result), &probe))
}

func TestContinueJSONResponse_StripsCodeFencesFromContinuation(t *testing.T) {
	// LLM wraps its continuation in code fences — they must be stripped before accumulating.
	partial := `{"name":"test","session_name":"s1","prompt":"Do it","checks":[`

	// Continuation wrapped in fences; the extractor must strip them.
	fencedCont := "```json\n{\"type\":\"no_error_messages\"}]}\n```"
	contResp := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: fencedCont, StopReason: "stop"},
		},
	}

	mock := &mockLLM{responses: []*llms.ContentResponse{contResp}}

	conversation := []llms.MessageContent{
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: "generate intent"}}},
	}
	tokens := 0
	result, err := continueJSONResponse(context.Background(), mock, conversation, partial, 3, 0, &tokens)

	require.NoError(t, err)

	// Result must be valid JSON (fences stripped, partial + continuation joined correctly).
	var probe interface{}
	require.NoError(t, json.Unmarshal([]byte(result), &probe))
}

func TestContinueJSONResponse_TokenLimitExceeded(t *testing.T) {
	// totalTokens already at/above the limit → should return an error immediately.
	partial := `{"name":"test"`
	tokens := 100
	mock := &mockLLM{}
	conversation := []llms.MessageContent{}

	_, err := continueJSONResponse(context.Background(), mock, conversation, partial, 3, 50, &tokens)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token limit exceeded")
	assert.Equal(t, 0, mock.callIdx, "no LLM calls should be made when token limit is already exceeded")
}

func TestContinueJSONResponse_ExhaustsMaxContinuations(t *testing.T) {
	// Every continuation response is still token-limit truncated → exhaust retries.
	contResp := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: `,"key":"value"`, StopReason: "length"},
		},
	}

	mock := &mockLLM{responses: []*llms.ContentResponse{contResp, contResp, contResp}}

	conversation := []llms.MessageContent{}
	partial := `{"name":"test"`
	tokens := 0
	_, err := continueJSONResponse(context.Background(), mock, conversation, partial, 3, 0, &tokens)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete after 3 continuations")
	assert.Equal(t, 3, mock.callIdx, "expected exactly maxContinuations calls")
}
