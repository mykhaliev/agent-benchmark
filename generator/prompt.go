package generator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/tmc/langchaingo/llms"
)

const systemPrompt = `You are a test generation expert for agent-benchmark, a Go-based AI agent testing framework.

Your task is to generate a complete, valid YAML "sessions" block that can be used directly as a test configuration.

OUTPUT RULES (strictly enforced):
1. Output ONLY valid YAML — no markdown, no explanations, no code fences.
2. Start your output with the line: sessions:
3. Every test must have: name, prompt, and at least one assertion.
4. Use realistic, specific prompts that a real user would send to the agent.
5. Use multiple assertions per test (tool assertions + output assertions together).
6. Prefer "tool_called" assertions whenever the task clearly requires a specific tool.
7. Do not use "anyOf", "allOf", or "not" unless complexity is "complex".
8. Never use cli_* assertion types unless the server type is "cli".
9. When a prompt and an assertion both reference the same literal value (filename,
    keyword, ID, etc.), define it once in the top-level "variables:" block and use
    {{name}} in both. Do not hardcode the same string in multiple places.
10. When a session contains a create-then-use workflow (create → fetch/update/delete),
    use "extractors:" in the first test to capture the returned ID/key, then reference
    {{variableName}} in subsequent tests.
11. Never reference {{variableName}} before it is defined (either in the top-level
    "variables:" block, or by a preceding "extractors:" in the same session).
12. Use top-level "variables:" for values chosen at generation time (filenames, search
    terms, content). Use "extractors:" only for values the agent produces at runtime.

` + sessionSchema + `
` + assertionTypesDoc + `
` + variablesAndExtractorsDoc

const planSystemPrompt = `You are a test planning expert for AI agent testing.

Your task is to create a compact JSON test plan describing what scenarios to test.
Think about what is interesting to test — focus on correctness, coverage, and edge cases.

OUTPUT RULES:
1. Output ONLY valid JSON — no markdown, no explanations, no code fences.
2. Start your output with: {
3. Do NOT write YAML — that comes in a separate step.
4. Use ONLY tool names that appear in the AGENT TOOLS section.
5. Group related tests into sessions with descriptive names.
`

// ToolInfo is a simplified representation of an MCP tool for prompt building.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]any
	Required    []string
}

// buildToolSection produces the "AGENT TOOLS" block for the given agents.
// agentNames controls the iteration order.
func buildToolSection(toolsByAgent map[string][]mcp.Tool, agentNames []string) string {
	var sb strings.Builder
	sb.WriteString("AGENT TOOLS\n===========\n")
	for _, name := range agentNames {
		tools := toolsByAgent[name]
		if len(tools) == 0 {
			sb.WriteString(fmt.Sprintf("\nAgent: %q (no tools available)\n", name))
			continue
		}
		sb.WriteString(fmt.Sprintf("\nAgent: %q\n", name))
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
	return sb.String()
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

// buildSystemMessage prepends any agent system prompts to the standard generation
// system prompt. Agents without a system_prompt are skipped.
func buildSystemMessage(agents []model.Agent) string {
	var sb strings.Builder

	for _, a := range agents {
		if strings.TrimSpace(a.SystemPrompt) == "" {
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString("AGENT CONTEXT\n=============\n")
		}
		sb.WriteString(fmt.Sprintf("\nAgent %q system prompt:\n%s\n", a.Name, strings.TrimSpace(a.SystemPrompt)))
	}

	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	sb.WriteString(systemPrompt)
	return sb.String()
}

// buildPlanSystemMessage prepends any agent system prompts to the plan system prompt.
func buildPlanSystemMessage(agents []model.Agent) string {
	var sb strings.Builder

	for _, a := range agents {
		if strings.TrimSpace(a.SystemPrompt) == "" {
			continue
		}
		if sb.Len() == 0 {
			sb.WriteString("AGENT CONTEXT\n=============\n")
		}
		sb.WriteString(fmt.Sprintf("\nAgent %q system prompt:\n%s\n", a.Name, strings.TrimSpace(a.SystemPrompt)))
	}

	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	sb.WriteString(planSystemPrompt)
	return sb.String()
}

// BuildPlanPrompt builds the system+user message pair for the LLM planning phase.
// The LLM is asked to produce a compact JSON test plan (no YAML) describing scenarios,
// expected tools, and assertions at a high level. This separates creative "what to test"
// decisions from mechanical "how to write valid YAML" decisions.
func BuildPlanPrompt(cfg *GeneratorConfig, toolsByAgent map[string][]mcp.Tool) []llms.MessageContent {
	var sb strings.Builder

	// Agent and tool descriptions (names + descriptions only, no params — keep it lean)
	sb.WriteString("AGENT TOOLS\n===========\n")
	for _, ag := range cfg.Agents {
		tools, ok := toolsByAgent[ag.Name]
		if !ok || len(tools) == 0 {
			sb.WriteString(fmt.Sprintf("\nAgent: %q (no tools available)\n", ag.Name))
			continue
		}
		sb.WriteString(fmt.Sprintf("\nAgent: %q\n", ag.Name))
		for _, t := range buildToolInfo(tools) {
			sb.WriteString(fmt.Sprintf("  Tool: %s\n", t.Name))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf("    Description: %s\n", t.Description))
			}
		}
	}

	// Constraints
	sb.WriteString("\nCONSTRAINTS\n===========\n")
	sb.WriteString(fmt.Sprintf("Generate exactly %d tests total across all sessions.\n", cfg.Generator.TestCount))
	sb.WriteString(fmt.Sprintf("Complexity: %s\n", cfg.Generator.Complexity))
	if cfg.Generator.IncludeEdgeCases {
		sb.WriteString("Include edge cases: error conditions, boundary inputs, unexpected values.\n")
	}
	if cfg.Generator.Goal != "" {
		sb.WriteString(fmt.Sprintf("\nTest goal: %s\n", cfg.Generator.Goal))
	}

	sb.WriteString("\nOutput the plan in this exact JSON format:\n")
	sb.WriteString(planSchema)

	sysText := buildPlanSystemMessage(cfg.Agents)

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: sysText}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}

// BuildPlanChunkPrompt builds the plan prompt for a single chunk of tests.
// chunkSize is how many tests to generate in this chunk.
// alreadyPlanned is the flat list of tests planned in prior chunks (may be nil).
func BuildPlanChunkPrompt(
	cfg *GeneratorConfig,
	toolsByAgent map[string][]mcp.Tool,
	chunkSize int,
	alreadyPlanned []planTest,
) []llms.MessageContent {
	var sb strings.Builder

	// Agent and tool descriptions (names + descriptions only, no params — keep it lean)
	sb.WriteString("AGENT TOOLS\n===========\n")
	for _, ag := range cfg.Agents {
		tools, ok := toolsByAgent[ag.Name]
		if !ok || len(tools) == 0 {
			sb.WriteString(fmt.Sprintf("\nAgent: %q (no tools available)\n", ag.Name))
			continue
		}
		sb.WriteString(fmt.Sprintf("\nAgent: %q\n", ag.Name))
		for _, t := range buildToolInfo(tools) {
			sb.WriteString(fmt.Sprintf("  Tool: %s\n", t.Name))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf("    Description: %s\n", t.Description))
			}
		}
	}

	// Explicit tool name constraint to prevent hallucination.
	sb.WriteString("\nIMPORTANT: tools_expected values MUST be chosen ONLY from the tool names listed\n")
	sb.WriteString("above for each agent. Using any other tool name will fail validation.\n")
	for _, ag := range cfg.Agents {
		tools, ok := toolsByAgent[ag.Name]
		if !ok || len(tools) == 0 {
			continue
		}
		names := make([]string, 0, len(tools))
		for _, t := range tools {
			names = append(names, t.Name)
		}
		sb.WriteString(fmt.Sprintf("Valid tool names for agent %q: %s\n", ag.Name, strings.Join(names, ", ")))
	}

	// Already planned tests section — sliding window to keep later chunks lean.
	if len(alreadyPlanned) > 0 {
		const maxShown = 25
		shown := alreadyPlanned
		header := "ALREADY PLANNED TESTS (DO NOT DUPLICATE)"
		if len(alreadyPlanned) > maxShown {
			shown = alreadyPlanned[len(alreadyPlanned)-maxShown:]
			header = fmt.Sprintf("ALREADY PLANNED TESTS (DO NOT DUPLICATE) — %d total, showing last %d",
				len(alreadyPlanned), maxShown)
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", header))
		sb.WriteString(strings.Repeat("=", len(header)))
		sb.WriteString("\nThe following tests are already planned. Your new tests MUST cover different\n")
		sb.WriteString("scenarios — do not repeat the same workflow, tool sequence, or assertion.\n\n")
		for i, pt := range shown {
			sb.WriteString(fmt.Sprintf("%d. %q — %s\n", i+1, pt.Name, pt.Goal))
		}
	}

	// Constraints
	totalPlanned := len(alreadyPlanned)
	totalTests := cfg.Generator.TestCount
	sb.WriteString("\nCONSTRAINTS\n===========\n")
	sb.WriteString(fmt.Sprintf(
		"Generate exactly %d tests (tests %d–%d of %d total).\n",
		chunkSize, totalPlanned+1, totalPlanned+chunkSize, totalTests,
	))
	sb.WriteString(fmt.Sprintf("Complexity: %s\n", cfg.Generator.Complexity))
	if cfg.Generator.IncludeEdgeCases {
		sb.WriteString("Include edge cases: error conditions, boundary inputs, unexpected values.\n")
	}
	if cfg.Generator.Goal != "" {
		sb.WriteString(fmt.Sprintf("\nTest goal: %s\n", cfg.Generator.Goal))
	}

	sb.WriteString("\nOutput the plan in this exact JSON format:\n")
	sb.WriteString(planSchema)

	sysText := buildPlanSystemMessage(cfg.Agents)

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: sysText}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}

// BuildGenerationPrompt builds the system+user message pair for the LLM.
// On retries (attempt > 1), prevYAML is the raw YAML from the previous attempt and
// prevErrors are the validation errors found in it. The previous response is included
// as an assistant turn so the LLM can see exactly what it produced and fix it.
// When plan is non-empty (from a successful planning phase), it is injected into the
// user message so the LLM only needs to translate the plan to YAML.
func BuildGenerationPrompt(
	cfg *GeneratorConfig,
	toolsByAgent map[string][]mcp.Tool,
	seed int64,
	attempt int,
	prevYAML string,
	prevErrors []string,
	plan string,
) []llms.MessageContent {
	// Build user message content
	var sb strings.Builder

	// Agent and tool descriptions
	agentNames := make([]string, 0, len(cfg.Agents))
	for _, ag := range cfg.Agents {
		agentNames = append(agentNames, ag.Name)
	}
	sb.WriteString(buildToolSection(toolsByAgent, agentNames))

	// Generation constraints
	sb.WriteString("\nGENERATION CONSTRAINTS\n======================\n")
	sb.WriteString(fmt.Sprintf("test_count: %d — you MUST generate exactly %d tests in total across all sessions\n", cfg.Generator.TestCount, cfg.Generator.TestCount))
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

	if cfg.Generator.Goal != "" {
		sb.WriteString(fmt.Sprintf("\nTEST GOAL ====================: %s\n", cfg.Generator.Goal))
		sb.WriteString(cfg.Generator.Goal)
	}

	// If a validated plan is available, inject it as a strong guide.
	// The LLM only needs to translate the plan to YAML — no creative decisions remain.
	if plan != "" {
		sb.WriteString("\nVALIDATED TEST PLAN (translate this plan to YAML exactly)\n==========================================================\n")
		sb.WriteString("The following test plan has been validated against the agent's actual tool list.\n")
		sb.WriteString("Implement every session and test in the plan. Do not add or remove tests.\n\n")
		sb.WriteString(plan)
		sb.WriteString("\n")
	}

	sb.WriteString("\nNow generate the sessions YAML block:\n")

	sysText := buildSystemMessage(cfg.Agents)

	msgs := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: sysText}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}

	// On retries, append the previous (broken) response as an assistant turn followed
	// by a human correction message. This lets the LLM see exactly what it produced
	// and fix only the specific problems rather than regenerating from scratch.
	if attempt > 1 && prevYAML != "" && len(prevErrors) > 0 {
		var fixSb strings.Builder
		fixSb.WriteString(fmt.Sprintf("Your previous output (attempt %d) had the following validation errors:\n", attempt-1))
		for _, e := range prevErrors {
			fixSb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
		fixSb.WriteString("\nFix all of the above issues and output the corrected sessions YAML block:\n")

		msgs = append(msgs,
			llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{llms.TextContent{Text: prevYAML}},
			},
			llms.MessageContent{
				Role:  llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{llms.TextContent{Text: fixSb.String()}},
			},
		)
	}

	return msgs
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

// ExtractJSONFromResponse strips markdown code fences (```json ... ``` or ``` ... ```)
// from an LLM response, returning only the raw JSON content.
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
