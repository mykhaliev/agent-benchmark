package explorer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/generator"
	"github.com/tmc/langchaingo/llms"
)

// ExplorerDecision is the structured JSON output from the exploration LLM.
// It embeds TestIntent so that all 19 check types and validation/repair logic
// from the generator package can be reused without duplication.
type ExplorerDecision struct {
	generator.TestIntent
	Reasoning string `json:"reasoning"`
}

const explorerSystemPrompt = `You are an autonomous test explorer for AI agents.

Your job is to decide the NEXT test to run for an agent, based on the exploration goal and what has already been tested.

You must respond with a single JSON object (no markdown, no code fences) with exactly these fields:
{
  "name": "<short descriptive test name>",
  "goal": "<what this test verifies>",
  "session_name": "exploration",
  "prompt": "<the exact user prompt to send to the agent>",
  "checks": [
    {"type": "<check_type>", ...}
  ],
  "reasoning": "<why you chose this test>"
}

CHECK TYPES you may use (flat assertions only — no anyOf/allOf/not):
- {"type": "tool_called",              "tool": "<tool_name>"}
- {"type": "tool_not_called",          "tool": "<tool_name>"}
- {"type": "tool_call_count",          "tool": "<tool_name>", "count": <N>}
- {"type": "tool_call_order",          "sequence": ["<tool1>", "<tool2>"]}
- {"type": "tool_param_equals",        "tool": "<tool_name>", "params": {"<param>": "<expected_value>"}}
- {"type": "tool_param_matches_regex", "tool": "<tool_name>", "params": {"<param>": "<regex>"}}
- {"type": "tool_result_matches_json", "tool": "<tool_name>", "path": "<jsonpath>", "value": "<expected>"}
- {"type": "output_contains",          "value": "<text>"}
- {"type": "output_not_contains",      "value": "<text>"}
- {"type": "output_regex",             "pattern": "<regex>"}
- {"type": "max_tokens",               "count": <N>}
- {"type": "max_latency_ms",           "count": <N>}
- {"type": "no_error_messages"}
- {"type": "no_hallucinated_tools"}
- {"type": "no_clarification_questions"}
- {"type": "no_rate_limit_errors"}
- {"type": "cli_exit_code_equals",     "expected": <N>}
- {"type": "cli_stdout_contains",      "value": "<text>"}
- {"type": "cli_stdout_regex",         "pattern": "<regex>"}
- {"type": "cli_stderr_contains",      "value": "<text>"}

RULES:
1. Choose a test that explores a different aspect than what has already been tested.
2. Make the prompt realistic — something a real user would send.
3. Include at least one check.
4. Do not use anyOf/allOf/not check types.
5. Keep the prompt concise and focused.
6. Use only tool names shown in AVAILABLE TOOLS.
7. If a previous test passed, explore adjacent functionality; if it failed, try a simpler variant.`

// IterationContext records the outcome of a previous exploration iteration.
type IterationContext struct {
	Iteration int
	TestName  string
	Prompt    string
	Passed    bool
	Summary   string // Short summary of what happened
}

// BuildDecisionPrompt builds the system+user message pair for the exploration LLM.
// It describes the available tools, the goal, the history, and asks for the next test.
func BuildDecisionPrompt(
	cfg *ExplorerConfig,
	tools []mcp.Tool,
	history []IterationContext,
	iteration int,
) ([]llms.MessageContent, string) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("EXPLORATION GOAL\n================\n%s\n\n", cfg.Explorer.Goal))

	// Tool descriptions
	if len(tools) > 0 {
		sb.WriteString("AVAILABLE TOOLS\n===============\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- %s", t.Name))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf(": %s", t.Description))
			}
			sb.WriteString("\n")
			if len(t.InputSchema.Properties) > 0 {
				paramsJSON, _ := json.Marshal(t.InputSchema.Properties)
				sb.WriteString(fmt.Sprintf("  Parameters: %s\n", string(paramsJSON)))
			}
			if len(t.InputSchema.Required) > 0 {
				sb.WriteString(fmt.Sprintf("  Required: %s\n", strings.Join(t.InputSchema.Required, ", ")))
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("AVAILABLE TOOLS\n===============\n(none — agent responds from LLM knowledge only)\n\n")
	}

	// History
	if len(history) > 0 {
		sb.WriteString("PREVIOUS ITERATIONS\n===================\n")
		for _, h := range history {
			status := "PASSED"
			if !h.Passed {
				status = "FAILED"
			}
			sb.WriteString(fmt.Sprintf("Iteration %d [%s]: %q\n", h.Iteration, status, h.TestName))
			sb.WriteString(fmt.Sprintf("  Prompt: %s\n", h.Prompt))
			if h.Summary != "" {
				sb.WriteString(fmt.Sprintf("  Summary: %s\n", h.Summary))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("CURRENT ITERATION: %d of %d\n\n", iteration, cfg.Explorer.MaxIterations))
	sb.WriteString("Now decide the next test to run. Output only a JSON object as specified.\n")

	userText := sb.String()

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: explorerSystemPrompt}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: userText}},
		},
	}, userText
}

// ExtractJSONFromResponse strips optional markdown code fences from an LLM
// response and returns the raw JSON content.
func ExtractJSONFromResponse(content string) string {
	content = strings.TrimSpace(content)

	for _, fence := range []string{"```json", "```"} {
		if strings.HasPrefix(content, fence) {
			content = strings.TrimPrefix(content, fence)
			if idx := strings.LastIndex(content, "```"); idx >= 0 {
				content = content[:idx]
			}
			break
		}
	}

	return strings.TrimSpace(content)
}

// ParseExplorerDecision parses the JSON object returned by the exploration LLM
// into an ExplorerDecision. It only checks structural validity (name and prompt
// present); semantic validation is delegated to generator.ValidateTestIntent.
func ParseExplorerDecision(jsonStr string) (*ExplorerDecision, error) {
	jsonStr = ExtractJSONFromResponse(jsonStr)

	var d ExplorerDecision
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decision JSON: %w", err)
	}

	if d.Name == "" {
		return nil, fmt.Errorf("decision missing name")
	}
	if d.Prompt == "" {
		return nil, fmt.Errorf("decision missing prompt")
	}

	return &d, nil
}
