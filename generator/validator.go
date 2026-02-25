package generator

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/model"
	"gopkg.in/yaml.v3"
)

// sessionsWrapper is a helper for unmarshalling only the sessions block.
type sessionsWrapper struct {
	Variables map[string]string `yaml:"variables,omitempty"`
	Sessions  []model.Session   `yaml:"sessions"`
}

// isStructurallyBroken reports whether raw is too damaged to send to the repair phase.
// YAML parse errors and empty session lists mean the LLM went off-rails; only
// a full regeneration can fix them. Semantic errors (wrong tool/param names) are
// fixable by repair and therefore not considered structural.
func isStructurallyBroken(raw string) bool {
	var w sessionsWrapper
	return yaml.Unmarshal([]byte(raw), &w) != nil || len(w.Sessions) == 0
}

// planWrapper is the JSON structure emitted by the LLM planning phase.
type planWrapper struct {
	Sessions []planSession `json:"sessions"`
}

type planSession struct {
	Name  string     `json:"name"`
	Tests []planTest `json:"tests"`
}

type planTest struct {
	Name          string   `json:"name"`
	Goal          string   `json:"goal"`
	ToolsExpected []string `json:"tools_expected"`
	Assertions    []string `json:"assertions"`
}

// toolAssertionTypes is the set of assertion types that reference a tool by name
// via the "tool" field. Used for semantic tool-name validation.
var toolAssertionTypes = map[string]bool{
	"tool_called":              true,
	"tool_not_called":          true,
	"tool_call_count":          true,
	"tool_param_equals":        true,
	"tool_param_matches_regex": true,
	"tool_result_matches_json": true,
}

// paramAssertionTypes is the set of assertion types whose "params" keys must
// match the tool's InputSchema.Properties. Used for semantic param-name validation.
var paramAssertionTypes = map[string]bool{
	"tool_param_equals":        true,
	"tool_param_matches_regex": true,
}

// builtinTemplateVars is the set of names that are always available as template
// variables (built-in engine vars and Handlebars helpers). These are never flagged
// as undefined forward references.
var builtinTemplateVars = map[string]bool{
	// Engine-provided static vars
	"TEST_DIR":  true,
	"TEMP_DIR":  true,
	"RUN_ID":    true,
	"SKILL_DIR": true,
	// Engine-provided runtime vars
	"AGENT_NAME":    true,
	"SESSION_NAME":  true,
	"PROVIDER_NAME": true,
	// Template helpers
	"randomValue":   true,
	"randomInt":     true,
	"randomDecimal": true,
	"faker":         true,
	"now":           true,
	"cut":           true,
	"replace":       true,
	"substring":     true,
}

// templateVarRe matches simple {{varname}} references (word characters only).
var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// extractTemplateVars returns all {{varname}} references found in s,
// excluding built-in template variables and helpers.
func extractTemplateVars(s string) []string {
	matches := templateVarRe.FindAllStringSubmatch(s, -1)
	var vars []string
	for _, m := range matches {
		name := m[1]
		if !builtinTemplateVars[name] {
			vars = append(vars, name)
		}
	}
	return vars
}

// ValidatePlan validates a JSON test plan against the known tool names.
// It checks that:
//   - The JSON is well-formed and contains at least one session.
//   - Every tool name in tools_expected exists in at least one agent's tool list.
//
// Returns a list of human-readable error strings; an empty list means the plan is valid.
func ValidatePlan(planJSON string, toolsByAgent map[string][]mcp.Tool) []string {
	var errs []string

	var plan planWrapper
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return []string{fmt.Sprintf("plan JSON parse error: %v", err)}
	}

	if len(plan.Sessions) == 0 {
		return []string{"plan has no sessions"}
	}

	// Build combined tool name set across all agents.
	allToolNames := make(map[string]bool)
	for _, tools := range toolsByAgent {
		for _, t := range tools {
			allToolNames[t.Name] = true
		}
	}

	for _, session := range plan.Sessions {
		for _, test := range session.Tests {
			for _, toolName := range test.ToolsExpected {
				if len(allToolNames) > 0 && !allToolNames[toolName] {
					errs = append(errs, fmt.Sprintf(
						"plan session %q, test %q: tool %q not found in any agent",
						session.Name, test.Name, toolName,
					))
				}
			}
		}
	}

	return errs
}

// ValidateSessions parses the YAML content (which must contain a "sessions:" key)
// and validates it structurally and semantically.
//
// Structural checks: YAML parse, required fields (name, prompt), known agent names,
// known assertion types.
//
// Semantic checks (only when toolsByAgent is non-nil/non-empty):
//   - Tool names in tool assertions exist in the agent's tool list.
//   - Param keys in tool_param_equals / tool_param_matches_regex match the tool schema.
//   - {{variable}} references in prompts/assertions are defined before use.
//
// Returns a list of human-readable error strings; an empty list means valid.
func ValidateSessions(yamlContent string, knownAgents []string, toolsByAgent map[string][]mcp.Tool) []string {
	var errs []string

	var wrapper sessionsWrapper
	if err := yaml.Unmarshal([]byte(yamlContent), &wrapper); err != nil {
		return []string{fmt.Sprintf("YAML parse error: %v", err)}
	}

	if len(wrapper.Sessions) == 0 {
		return []string{"no sessions found in generated output"}
	}

	agentSet := make(map[string]bool, len(knownAgents))
	for _, a := range knownAgents {
		agentSet[a] = true
	}

	assertionTypeSet := make(map[string]bool, len(validAssertionTypes))
	for _, t := range validAssertionTypes {
		assertionTypeSet[t] = true
	}

	// Build semantic tool lookup tables from toolsByAgent.
	allToolNames := make(map[string]bool)
	toolParamNames := make(map[string]map[string]bool) // tool name → set of param names
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

	// Global variables defined at the top-level variables: block.
	globalVars := make(map[string]bool, len(wrapper.Variables))
	for k := range wrapper.Variables {
		globalVars[k] = true
	}

	for si, session := range wrapper.Sessions {
		sessionLabel := fmt.Sprintf("session[%d](%q)", si, session.Name)

		if session.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: missing name", sessionLabel))
		}
		if len(session.Tests) == 0 {
			errs = append(errs, fmt.Sprintf("%s: has no tests", sessionLabel))
		}

		// Per-session variable scope (starts with global variables).
		sessionVars := make(map[string]bool, len(globalVars))
		for k := range globalVars {
			sessionVars[k] = true
		}

		for ti, test := range session.Tests {
			testLabel := fmt.Sprintf("%s/test[%d](%q)", sessionLabel, ti, test.Name)

			if test.Name == "" {
				errs = append(errs, fmt.Sprintf("%s: missing name", testLabel))
			}
			if test.Prompt == "" {
				errs = append(errs, fmt.Sprintf("%s: missing prompt", testLabel))
			}

			// Check agent name (when knownAgents is non-empty).
			if test.Agent != "" && len(agentSet) > 0 && !agentSet[test.Agent] {
				errs = append(errs, fmt.Sprintf(
					"%s: references unknown agent %q", testLabel, test.Agent,
				))
			}

			// Check forward variable references in prompt.
			for _, ref := range extractTemplateVars(test.Prompt) {
				if !sessionVars[ref] {
					errs = append(errs, fmt.Sprintf(
						"%s: prompt references undefined variable {{%s}}", testLabel, ref,
					))
				}
			}

			for ai, assertion := range test.Assertions {
				assertionLabel := fmt.Sprintf("%s/assertion[%d]", testLabel, ai)
				errs = append(errs, validateAssertion(
					assertion, assertionLabel,
					assertionTypeSet, allToolNames, toolParamNames, sessionVars,
				)...)
			}

			// After checking this test's assertions, add variables from extractors
			// so that subsequent tests in the same session can reference them.
			for _, ex := range test.Extractors {
				if ex.Tool != "" && len(allToolNames) > 0 && !allToolNames[ex.Tool] {
					errs = append(errs, fmt.Sprintf(
						"%s: extractor references unknown tool %q", testLabel, ex.Tool,
					))
				}
				if ex.VariableName != "" {
					sessionVars[ex.VariableName] = true
				}
			}
		}
	}

	return errs
}

// validateAssertion checks a single assertion for type validity, tool-name existence,
// param-name existence, and variable forward references.
func validateAssertion(
	assertion model.Assertion,
	label string,
	assertionTypeSet map[string]bool,
	allToolNames map[string]bool,
	toolParamNames map[string]map[string]bool,
	sessionVars map[string]bool,
) []string {
	var errs []string

	if assertion.Type == "" {
		errs = append(errs, fmt.Sprintf("%s: missing type", label))
		return errs
	}
	if !assertionTypeSet[assertion.Type] {
		errs = append(errs, fmt.Sprintf("%s: unknown assertion type %q", label, assertion.Type))
		return errs
	}

	// Semantic tool-name check for assertions that carry a "tool" field.
	if toolAssertionTypes[assertion.Type] && assertion.Tool != "" {
		if len(allToolNames) > 0 && !allToolNames[assertion.Tool] {
			errs = append(errs, fmt.Sprintf(
				"%s: tool %q not found in any agent's tool list", label, assertion.Tool,
			))
		}
	}

	// tool_call_order: check every name in the sequence.
	if assertion.Type == "tool_call_order" {
		for _, toolName := range assertion.Sequence {
			if len(allToolNames) > 0 && !allToolNames[toolName] {
				errs = append(errs, fmt.Sprintf(
					"%s: tool %q in sequence not found in any agent's tool list", label, toolName,
				))
			}
		}
	}

	// Semantic param-name check for assertions that carry a "params" map.
	if paramAssertionTypes[assertion.Type] && assertion.Tool != "" {
		if params, ok := toolParamNames[assertion.Tool]; ok && len(params) > 0 {
			for paramKey := range assertion.Params {
				if !params[paramKey] {
					errs = append(errs, fmt.Sprintf(
						"%s: param %q not found in tool %q's input schema",
						label, paramKey, assertion.Tool,
					))
				}
			}
		}
	}

	// Variable forward-reference checks in string fields.
	for _, ref := range extractTemplateVars(assertion.Value) {
		if !sessionVars[ref] {
			errs = append(errs, fmt.Sprintf(
				"%s: value references undefined variable {{%s}}", label, ref,
			))
		}
	}
	for _, ref := range extractTemplateVars(assertion.Pattern) {
		if !sessionVars[ref] {
			errs = append(errs, fmt.Sprintf(
				"%s: pattern references undefined variable {{%s}}", label, ref,
			))
		}
	}
	for k, v := range assertion.Params {
		for _, ref := range extractTemplateVars(v) {
			if !sessionVars[ref] {
				errs = append(errs, fmt.Sprintf(
					"%s: params[%s] references undefined variable {{%s}}", label, k, ref,
				))
			}
		}
	}

	// Recurse into boolean combinator children.
	for ci, child := range assertion.AnyOf {
		childLabel := fmt.Sprintf("%s/anyOf[%d]", label, ci)
		errs = append(errs, validateAssertion(child, childLabel, assertionTypeSet, allToolNames, toolParamNames, sessionVars)...)
	}
	for ci, child := range assertion.AllOf {
		childLabel := fmt.Sprintf("%s/allOf[%d]", label, ci)
		errs = append(errs, validateAssertion(child, childLabel, assertionTypeSet, allToolNames, toolParamNames, sessionVars)...)
	}
	if assertion.Not != nil {
		errs = append(errs, validateAssertion(*assertion.Not, label+"/not", assertionTypeSet, allToolNames, toolParamNames, sessionVars)...)
	}

	return errs
}
