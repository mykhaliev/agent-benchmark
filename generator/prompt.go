package generator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"
)

const systemPrompt = `You are a test generation expert for agent-benchmark, a Go-based AI agent testing framework.

Your task is to generate a complete, valid YAML "sessions" block that can be used directly as a test configuration.

OUTPUT RULES (strictly enforced):
1. Output ONLY valid YAML — no markdown, no explanations, no code fences.
2. Start your output with the line: sessions:
3. Every test must have: name, agent, prompt, and at least one assertion.
4. The "agent" field must exactly match one of the configured agent names.
5. Use realistic, specific prompts that a real user would send to the agent.
6. Use multiple assertions per test (tool assertions + output assertions together).
7. Prefer "tool_called" assertions whenever the task clearly requires a specific tool.
8. Do not use "anyOf", "allOf", or "not" unless complexity is "complex".
9. Never use cli_* assertion types unless the server type is "cli".

` + sessionSchema + `
` + assertionTypesDoc

// ToolInfo is a simplified representation of an MCP tool for prompt building.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]any
	Required    []string
}

// buildToolInfo converts mcp.Tool slices to a prompt-friendly representation.
func buildToolInfo(tools []mcp.Tool) []ToolInfo {
	result := make([]ToolInfo, 0, len(tools))
	for _, t := range tools {
		result = append(result, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema.Properties,
			Required:    t.InputSchema.Required,
		})
	}
	return result
}

// BuildGenerationPrompt builds the system+user message pair for the LLM.
func BuildGenerationPrompt(
	cfg *GeneratorConfig,
	toolsByAgent map[string][]mcp.Tool,
	seed int64,
	attempt int,
	prevErrors []string,
) []llms.MessageContent {
	// Build user message content
	var sb strings.Builder

	// Agent and tool descriptions
	sb.WriteString("AGENT TOOLS\n===========\n")
	for _, agent := range cfg.Agents {
		tools, ok := toolsByAgent[agent.Name]
		if !ok || len(tools) == 0 {
			sb.WriteString(fmt.Sprintf("\nAgent: %q (no tools available)\n", agent.Name))
			continue
		}
		sb.WriteString(fmt.Sprintf("\nAgent: %q\n", agent.Name))
		for _, t := range buildToolInfo(tools) {
			sb.WriteString(fmt.Sprintf("  Tool: %s\n", t.Name))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf("    Description: %s\n", t.Description))
			}
			if len(t.Parameters) > 0 {
				paramsJSON, _ := json.Marshal(t.Parameters)
				sb.WriteString(fmt.Sprintf("    Parameters: %s\n", string(paramsJSON)))
			}
			if len(t.Required) > 0 {
				sb.WriteString(fmt.Sprintf("    Required: %s\n", strings.Join(t.Required, ", ")))
			}
		}
	}

	// Generation constraints
	sb.WriteString("\nGENERATION CONSTRAINTS\n======================\n")
	sb.WriteString(fmt.Sprintf("test_count: %d\n", cfg.Generator.TestCount))
	sb.WriteString(fmt.Sprintf("complexity: %s\n", cfg.Generator.Complexity))
	if cfg.Generator.IncludeEdgeCases {
		sb.WriteString("include_edge_cases: true — include tests for error cases, boundary conditions, and unexpected inputs\n")
	}
	sb.WriteString(fmt.Sprintf("max_steps_per_test: %d — keep prompts simple enough to complete in at most %d tool calls\n",
		cfg.Generator.MaxStepsPerTest, cfg.Generator.MaxStepsPerTest))

	complexityGuide := map[string]string{
		"simple":  "Single tool call per test. Straightforward prompts with obvious expected behaviour.",
		"medium":  "One to three tool calls per test. Prompts may chain tools or require intermediate results.",
		"complex": "Multiple tool calls, conditional logic, or multi-step workflows. May use anyOf/allOf combinators.",
	}
	if guide, ok := complexityGuide[cfg.Generator.Complexity]; ok {
		sb.WriteString(fmt.Sprintf("complexity guide: %s\n", guide))
	}

	if seed != 0 {
		sb.WriteString(fmt.Sprintf("\nUse this seed for any randomisation decisions: %d\n", seed))
	}

	// Retry context
	if attempt > 1 && len(prevErrors) > 0 {
		sb.WriteString(fmt.Sprintf("\nPREVIOUS ATTEMPT %d FAILED WITH ERRORS\n", attempt-1))
		sb.WriteString("Fix all of the following issues in your new output:\n")
		for _, e := range prevErrors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	sb.WriteString("\nNow generate the sessions YAML block:\n")

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: systemPrompt}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}

// ExtractYAMLFromResponse strips markdown code fences (```yaml ... ``` or ``` ... ```)
// from an LLM response, returning only the raw YAML content.
func ExtractYAMLFromResponse(content string) string {
	content = strings.TrimSpace(content)

	// Strip opening ```yaml or ``` fence
	for _, fence := range []string{"```yaml", "```yml", "```"} {
		if strings.HasPrefix(content, fence) {
			content = strings.TrimPrefix(content, fence)
			// Strip trailing ```
			if idx := strings.LastIndex(content, "```"); idx >= 0 {
				content = content[:idx]
			}
			break
		}
	}

	return strings.TrimSpace(content)
}
