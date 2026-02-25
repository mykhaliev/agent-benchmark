package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

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
	// Provider defaults to first agent's provider when not set.
	assert.Equal(t, "gemini", cfg.Generator.Provider)
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
  provider: gpt
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
	assert.Equal(t, "gpt", cfg.Generator.Provider)
}

func TestParseGeneratorConfig_MissingFile(t *testing.T) {
	_, err := ParseGeneratorConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
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
	errs := ValidateSessions(validSessionsYAML, []string{"file-agent"})
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
	errs := ValidateSessions(content, []string{"file-agent"})
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
	errs := ValidateSessions(content, []string{"file-agent"})
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
	errs := ValidateSessions(content, []string{"file-agent"})
	assert.NotEmpty(t, errs)
	assert.True(t, containsSubstring(errs, "missing prompt"))
}

func TestValidateSessions_InvalidYAML(t *testing.T) {
	errs := ValidateSessions("this: is: not: valid: yaml: [", []string{"file-agent"})
	assert.NotEmpty(t, errs)
}

func TestValidateSessions_EmptyAgentList(t *testing.T) {
	// When no known agents are provided, agent field validation is skipped.
	errs := ValidateSessions(validSessionsYAML, []string{})
	assert.Empty(t, errs)
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

	msgs := BuildGenerationPrompt(cfg, toolsByAgent, 0, 1, nil)

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

	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 0, 2, []string{"unknown agent foo"})
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, "PREVIOUS ATTEMPT")
	assert.Contains(t, userContent, "unknown agent foo")
}

func TestBuildPrompt_IncludesSeed(t *testing.T) {
	cfg := &GeneratorConfig{
		Agents:    []model.Agent{{Name: "agent1", Provider: "p"}},
		Generator: GeneratorSettings{TestCount: 2, Complexity: "medium", MaxStepsPerTest: 3},
	}

	msgs := BuildGenerationPrompt(cfg, map[string][]mcp.Tool{}, 42, 1, nil)
	userContent := extractText(msgs[1])

	assert.Contains(t, userContent, "42")
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
