package generator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"
)

// ValidateTestIntent validates one TestIntent and returns human-readable error strings.
// sessionVars must contain global variables plus extractor outputs from prior tests
// in the same session.
func ValidateTestIntent(
	intent TestIntent,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
	sessionVars map[string]bool,
) []string {
	var errs []string

	if intent.Name == "" {
		errs = append(errs, "intent: missing name")
	}
	if intent.Prompt == "" {
		errs = append(errs, "intent: missing prompt")
	}
	if len(intent.Checks) == 0 {
		errs = append(errs, "intent: no checks defined")
	}

	// Validate agent name when agentNames is non-empty.
	if intent.Agent != "" && len(agentNames) > 0 {
		agentSet := make(map[string]bool, len(agentNames))
		for _, a := range agentNames {
			agentSet[a] = true
		}
		if !agentSet[intent.Agent] {
			errs = append(errs, fmt.Sprintf("intent: unknown agent %q", intent.Agent))
		}
	}

	// Build combined tool name and param-name lookup tables.
	allToolNames := make(map[string]bool)
	toolParamNames := make(map[string]map[string]bool)
	for _, tools := range toolsByAgent {
		for _, t := range tools {
			allToolNames[t.Name] = true
			params := make(map[string]bool, len(t.InputSchema.Properties))
			for k := range t.InputSchema.Properties {
				params[k] = true
			}
			toolParamNames[t.Name] = params
		}
	}

	intentCheckTypeSet := make(map[string]bool, len(validIntentCheckTypes))
	for _, t := range validIntentCheckTypes {
		intentCheckTypeSet[t] = true
	}

	for i, check := range intent.Checks {
		checkLabel := fmt.Sprintf("check[%d]", i)

		// Explicit error for forbidden combinators.
		if check.Type == "anyOf" || check.Type == "allOf" || check.Type == "not" {
			errs = append(errs, fmt.Sprintf("%s: combinator type %q is forbidden in intent checks", checkLabel, check.Type))
			continue
		}

		if !intentCheckTypeSet[check.Type] {
			errs = append(errs, fmt.Sprintf("%s: unknown check type %q", checkLabel, check.Type))
			continue
		}

		// Tool name validation.
		if toolAssertionTypes[check.Type] && check.Tool != "" {
			if len(allToolNames) > 0 && !allToolNames[check.Tool] {
				errs = append(errs, fmt.Sprintf("%s: tool %q not found in any agent's tool list", checkLabel, check.Tool))
			}
		}

		// tool_call_order sequence validation.
		if check.Type == "tool_call_order" {
			for _, toolName := range check.Sequence {
				if len(allToolNames) > 0 && !allToolNames[toolName] {
					errs = append(errs, fmt.Sprintf("%s: tool %q in sequence not found in any agent's tool list", checkLabel, toolName))
				}
			}
		}

		// Param-name validation.
		if paramAssertionTypes[check.Type] && check.Tool != "" {
			if params, ok := toolParamNames[check.Tool]; ok && len(params) > 0 {
				for paramKey := range check.Params {
					if !params[paramKey] {
						errs = append(errs, fmt.Sprintf("%s: param %q not found in tool %q's input schema", checkLabel, paramKey, check.Tool))
					}
				}
			}
		}

		// Variable forward-reference checks.
		for _, ref := range extractTemplateVars(check.Value) {
			if !sessionVars[ref] {
				errs = append(errs, fmt.Sprintf("%s: value references undefined variable {{%s}}", checkLabel, ref))
			}
		}
		for _, ref := range extractTemplateVars(check.Pattern) {
			if !sessionVars[ref] {
				errs = append(errs, fmt.Sprintf("%s: pattern references undefined variable {{%s}}", checkLabel, ref))
			}
		}
		for k, raw := range check.Params {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				// Non-string param value (object/number/bool) — skip template-var check.
				continue
			}
			for _, ref := range extractTemplateVars(v) {
				if !sessionVars[ref] {
					errs = append(errs, fmt.Sprintf("%s: params[%s] references undefined variable {{%s}}", checkLabel, k, ref))
				}
			}
		}
	}

	// Prompt variable forward-reference checks.
	for _, ref := range extractTemplateVars(intent.Prompt) {
		if !sessionVars[ref] {
			errs = append(errs, fmt.Sprintf("prompt references undefined variable {{%s}}", ref))
		}
	}

	// Extractor tool-name validation.
	for i, ex := range intent.Extractors {
		if ex.Tool != "" && len(allToolNames) > 0 && !allToolNames[ex.Tool] {
			errs = append(errs, fmt.Sprintf("extractor[%d]: tool %q not found in any agent's tool list", i, ex.Tool))
		}
	}

	return errs
}

// intentSystemPrompt is the shared system prompt for test-intent generation and parse-repair.
const intentSystemPrompt = `You are a test-intent generator for AI agent testing.

OUTPUT RULES (strictly enforced):
1. Output ONLY valid JSON. No markdown, no explanations, no code fences.
2. Start your output with: {
3. Output exactly ONE TestIntent JSON object.
4. Do NOT use anyOf, allOf, or not in checks — flat assertions only.
5. Do NOT output YAML.
6. Use only tool names shown in AGENT TOOLS.
7. Use only param keys shown for each tool.
8. Reference variables with {{varName}} syntax.
9. "params" values MUST always be strings. Never use nested objects as param values.
   Wrong:  "params": {"args": {"service_name": "x"}}
   Right:  "params": {"service_name": "x"}  (use only top-level param keys)`

// hasParamNotFoundError returns true if any error string indicates a param-not-found
// validation failure (i.e. the plan's suggested param names were wrong).
func hasParamNotFoundError(errs []string) bool {
	for _, e := range errs {
		if strings.Contains(e, "param") && strings.Contains(e, "not found") {
			return true
		}
	}
	return false
}

// buildParamReference produces a flat "PARAM REFERENCE" block listing valid param names
// per tool, iterated in agentNames order. Tools with no properties are skipped.
// Returns an empty string if no tools have properties.
func buildParamReference(toolsByAgent map[string][]mcp.Tool, agentNames []string) string {
	var sb strings.Builder
	written := false
	for _, name := range agentNames {
		for _, t := range toolsByAgent[name] {
			if len(t.InputSchema.Properties) == 0 {
				continue
			}
			if !written {
				sb.WriteString("PARAM REFERENCE — use ONLY these exact param names in tool_param_equals\n")
				sb.WriteString("======================================================================\n")
				written = true
			}
			keys := make([]string, 0, len(t.InputSchema.Properties))
			for k := range t.InputSchema.Properties {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			sb.WriteString(fmt.Sprintf("Tool %q: %s\n", t.Name, strings.Join(keys, ", ")))
		}
	}
	if written {
		sb.WriteString("\n")
	}
	return sb.String()
}

// BuildTestIntentPrompt builds the LLM conversation messages for one test scenario.
// prevErrors, if non-empty, appends a section listing errors from the previous attempt
// so the LLM can fix them on retry.
func BuildTestIntentPrompt(
	pt planTest,
	sessionName string,
	agentName string,
	agentNames []string,
	toolsByAgent map[string][]mcp.Tool,
	sessionVars map[string]bool,
	_ *GeneratorConfig,
	prevErrors []string,
) []llms.MessageContent {
	systemText := intentSystemPrompt

	var sb strings.Builder

	// Agent tools (with full params/required).
	sb.WriteString(buildToolSection(toolsByAgent, agentNames))
	sb.WriteString("\n")

	// Inject compact param reference when a previous attempt had param-not-found errors.
	if hasParamNotFoundError(prevErrors) {
		sb.WriteString(buildParamReference(toolsByAgent, agentNames))
	}

	// Available session vars (excluding built-ins).
	if len(sessionVars) > 0 {
		hasUserVars := false
		for v := range sessionVars {
			if !builtinTemplateVars[v] {
				hasUserVars = true
				break
			}
		}
		if hasUserVars {
			sb.WriteString("AVAILABLE SESSION VARS\n======================\n")
			for v := range sessionVars {
				if !builtinTemplateVars[v] {
					sb.WriteString(fmt.Sprintf("  {{%s}}\n", v))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Test scenario from the plan.
	sb.WriteString("TEST SCENARIO\n=============\n")
	sb.WriteString(fmt.Sprintf("Name: %s\n", pt.Name))
	sb.WriteString(fmt.Sprintf("Goal: %s\n", pt.Goal))
	sb.WriteString(fmt.Sprintf("Session: %s\n", sessionName))
	if agentName != "" {
		sb.WriteString(fmt.Sprintf("Agent: %s\n", agentName))
	}
	if len(pt.ToolsExpected) > 0 {
		sb.WriteString(fmt.Sprintf("Expected tools: %s\n", strings.Join(pt.ToolsExpected, ", ")))
	}
	if len(pt.Assertions) > 0 {
		if hasParamNotFoundError(prevErrors) {
			sb.WriteString("IMPORTANT: The plan's suggested assertions contained wrong parameter names.\n")
			sb.WriteString("Do NOT follow them. Use ONLY parameter names shown in AGENT TOOLS above.\n")
		} else {
			sb.WriteString("Suggested assertions:\n")
			for _, a := range pt.Assertions {
				sb.WriteString(fmt.Sprintf("  - %s\n", a))
			}
		}
	}

	sb.WriteString("\nOutput ONE TestIntent JSON following this schema:\n")
	sb.WriteString(testIntentSchema)

	if len(prevErrors) > 0 {
		sb.WriteString("\n\nPREVIOUS ATTEMPT FAILED WITH THESE ERRORS — FIX THEM:\n")
		for _, e := range prevErrors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
		sb.WriteString("Only use parameter names that appear in the tool schema above.\n")
	}

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: systemText}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}

// BuildTestIntentParseRepairPrompt builds LLM messages for recovering from a JSON parse
// failure. It instructs the LLM to produce a simpler, shorter intent with fewer checks
// so that the response fits within the output token budget.
func BuildTestIntentParseRepairPrompt(
	pt planTest,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
) []llms.MessageContent {
	var sb strings.Builder

	sb.WriteString(buildToolSection(toolsByAgent, agentNames))
	sb.WriteString("\n")

	// Always inject param reference to help the model avoid param name hallucination.
	sb.WriteString(buildParamReference(toolsByAgent, agentNames))

	sb.WriteString("TEST SCENARIO\n=============\n")
	sb.WriteString(fmt.Sprintf("Name: %s\n", pt.Name))
	sb.WriteString(fmt.Sprintf("Goal: %s\n", pt.Goal))
	if len(pt.ToolsExpected) > 0 {
		sb.WriteString(fmt.Sprintf("Expected tools: %s\n", strings.Join(pt.ToolsExpected, ", ")))
	}

	sb.WriteString("\nYour previous response produced truncated or invalid JSON.")
	sb.WriteString(" Generate a SIMPLER, SHORTER intent with at most 2-3 essential checks.")
	sb.WriteString(" Skip extractors unless required by the test goal.")
	sb.WriteString(" Return valid JSON only.\n\n")

	sb.WriteString("Output ONE TestIntent JSON following this schema:\n")
	sb.WriteString(testIntentSchema)

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: intentSystemPrompt}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}

// BuildTestIntentRepairPrompt builds the repair conversation for a failed TestIntent.
func BuildTestIntentRepairPrompt(
	intent TestIntent,
	errs []string,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
) []llms.MessageContent {
	var sb strings.Builder

	sb.WriteString(buildToolSection(toolsByAgent, agentNames))
	sb.WriteString("\nORIGINAL INTENT JSON:\n")

	intentJSON, _ := json.Marshal(intent)
	sb.WriteString(string(intentJSON))

	sb.WriteString("\n\nERRORS:\n")
	for _, e := range errs {
		sb.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	sb.WriteString("\nFix ALL errors. Return corrected JSON only. No explanation.")

	return []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: intentSystemPrompt}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: sb.String()}},
		},
	}
}
