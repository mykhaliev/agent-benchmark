package model

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aymerick/raymond"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/version"
	"github.com/yalp/jsonpath"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// TEST SUITE CONFIGURATION
// ============================================================================

type TestSuiteConfiguration struct {
	Name         string            `yaml:"name"`
	TestFiles    []string          `yaml:"test_files"`
	Providers    []Provider        `yaml:"providers"`
	Servers      []Server          `yaml:"servers"`
	Agents       []Agent           `yaml:"agents"`
	Settings     Settings          `yaml:"settings"`
	Variables    map[string]string `yaml:"variables,omitempty"`
	TestCriteria Criteria          `yaml:"criteria"`
	AISummary    AISummary         `yaml:"ai_summary,omitempty"`
}

// ============================================================================
// TEST CONFIGURATION
// ============================================================================

type TestConfiguration struct {
	Providers    []Provider        `yaml:"providers"`
	Servers      []Server          `yaml:"servers"`
	Agents       []Agent           `yaml:"agents"`
	Sessions     []Session         `yaml:"sessions"`
	Settings     Settings          `yaml:"settings"`
	Variables    map[string]string `yaml:"variables,omitempty"`
	TestCriteria Criteria          `yaml:"criteria"`
	AISummary    AISummary         `yaml:"ai_summary,omitempty"`
}

// ============================================================================
// PROVIDER CONFIGURATION
// ============================================================================

// RateLimitConfig defines proactive rate limiting settings for a provider.
// This throttles requests BEFORE they are sent to avoid hitting provider limits.
type RateLimitConfig struct {
	TPM int `yaml:"tpm"` // Tokens per minute limit (proactive throttling)
	RPM int `yaml:"rpm"` // Requests per minute limit (proactive throttling)
}

// RetryConfig defines reactive error handling settings for a provider.
// This controls how the system responds when errors like 429 are received.
type RetryConfig struct {
	// RetryOn429 enables automatic retry when receiving 429 (Too Many Requests) errors.
	// By default (false), 429 is treated as a regular error and fails immediately.
	// When enabled, the system will retry with exponential backoff.
	RetryOn429 bool `yaml:"retry_on_429"`
	// MaxRetries is the maximum number of retry attempts for 429 errors.
	// Only used when RetryOn429 is enabled. Default: 3
	MaxRetries int `yaml:"max_retries"`
}

type Provider struct {
	Name            string          `yaml:"name"`
	Type            ProviderType    `yaml:"type"`
	Token           string          `yaml:"token"`
	Secret          string          `yaml:"secret"`
	Model           string          `yaml:"model"`
	BaseURL         string          `yaml:"baseUrl"`          // e.g., gpt-4o-mini
	Version         string          `yaml:"version"`          // e.g., 2025-01-01-preview
	ProjectID       string          `yaml:"project_id"`       // e.g., 2025-01-01-preview
	Location        string          `yaml:"location"`         // e.g., 2025-01-01-preview
	CredentialsPath string          `yaml:"credentials_path"` // e.g., 2025-01-01-preview
	AuthType        string          `yaml:"auth_type"`        // For AZURE: "api_key" (default) or "entra_id"
	RateLimits      RateLimitConfig `yaml:"rate_limits"`      // Optional proactive rate limiting
	Retry           RetryConfig     `yaml:"retry"`            // Optional reactive error handling (e.g., 429 retries)
}

type ProviderType string

const (
	ProviderGroq            ProviderType = "GROQ"
	ProviderGoogle          ProviderType = "GOOGLE"
	ProviderVertex          ProviderType = "VERTEX"
	ProviderAnthropic       ProviderType = "ANTHROPIC"
	ProviderAmazonAnthropic ProviderType = "AMAZON-ANTHROPIC"
	ProviderOpenAI          ProviderType = "OPENAI"
	ProviderAzure           ProviderType = "AZURE"
)

// ============================================================================
// SERVER CONFIGURATION
// ============================================================================

type Server struct {
	Name         string     `yaml:"name"`
	Type         ServerType `yaml:"type"`
	Command      string     `yaml:"command,omitempty"`
	URL          string     `yaml:"url,omitempty"`
	Headers      []string   `yaml:"headers"`
	ServerDelay  string     `yaml:"server_delay,omitempty"`
	ProcessDelay string     `yaml:"process_delay,omitempty"`
	// CLI server type specific fields
	Shell                    string   `yaml:"shell,omitempty"`                       // Shell to use (powershell, cmd, bash). Default: powershell on Windows, bash on Unix
	WorkingDir               string   `yaml:"working_dir,omitempty"`                 // Working directory for CLI commands. Default: current directory
	ToolPrefix               string   `yaml:"tool_prefix,omitempty"`                 // Prefix for generated tool names (e.g., "excel" → "excel_sheet_create")
	ToolsFromCLI             bool     `yaml:"tools_from_cli,omitempty"`              // If true, discover tools by running CLI with --help or similar
	HelpCommand              string   `yaml:"help_command,omitempty"`                // DEPRECATED: Use help_commands instead. Single help command.
	HelpCommands             []string `yaml:"help_commands,omitempty"`               // Commands to run at startup to get CLI help (outputs concatenated and injected into tool description)
	DisableHelpAutoDiscovery bool     `yaml:"disable_help_auto_discovery,omitempty"` // If true, disable automatic help discovery when no help_command is configured
}

type ServerType string

const (
	Stdio ServerType = "stdio"
	SSE   ServerType = "sse"
	Http  ServerType = "http"
	CLI   ServerType = "cli"
)

// ============================================================================
// AGENT CONFIGURATION
// ============================================================================

// ClarificationDetection configures how the agent handles LLM clarification requests.
// When enabled, the agent uses an LLM to detect when the primary LLM asks for confirmation instead of acting.
// This approach is more accurate than pattern matching as it can understand context, nuance, and multiple languages.
type ClarificationDetection struct {
	Enabled       bool   `yaml:"enabled"`                  // Enable clarification detection (default: false)
	Level         string `yaml:"level,omitempty"`          // Log level: "info", "warning", "error" (default: "warning")
	JudgeProvider string `yaml:"judge_provider,omitempty"` // Provider name for the judge LLM. Use "$self" to reuse the agent's provider, or specify a provider name (required when enabled)
}

// AISummary configures automatic LLM-generated analysis of test results.
// When enabled, the system uses an LLM to generate an executive summary of the test run.
// The analysis appears as the first section in generated reports.
type AISummary struct {
	Enabled       bool   `yaml:"enabled"`                  // Enable AI summary (default: false)
	JudgeProvider string `yaml:"judge_provider,omitempty"` // Provider name for the judge LLM. Use "$self" to reuse a test agent's provider, or specify a provider name (required when enabled)
}

type Agent struct {
	Name                   string                 `yaml:"name"`
	Settings               Settings               `yaml:"settings"`
	Servers                []AgentServer          `yaml:"servers"`
	Provider               string                 `yaml:"provider"`
	SystemPrompt           string                 `yaml:"system_prompt,omitempty"`
	ClarificationDetection ClarificationDetection `yaml:"clarification_detection,omitempty"`
}

type AgentServer struct {
	Name         string   `yaml:"name"`
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
}

// ============================================================================
// TEST RESULT
// ============================================================================

type Criteria struct {
	SuccessRate string `yaml:"success_rate" json:"successRate"`
}

// ============================================================================
// AGENT CONFIGURATION
// ============================================================================

type Settings struct {
	Verbose        bool           `yaml:"verbose"`
	ToolTimeout    string         `yaml:"tool_tool_timeout"`
	MaxIterations  int            `yaml:"max_iterations"`
	TestDelay      string         `yaml:"test_delay"`
	SessionDelay   string         `yaml:"session_delay"`
	VariablePolicy VariablePolicy `yaml:"variable_policy"`
}

type VariablePolicy string

const (
	TestOnly           VariablePolicy = "test-only"
	SuiteOnly          VariablePolicy = "suite-only"
	MergeTestPriority  VariablePolicy = "merge-test-priority"
	MergeSuitePriority VariablePolicy = "merge-suite-priority"
)

// ============================================================================
// SESSION MODEL
// ============================================================================

type Session struct {
	Name         string   `yaml:"name"`
	Tests        []Test   `yaml:"tests"`
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
}

// ============================================================================
// TEST MODEL
// ============================================================================

type Test struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description,omitempty"`
	Agent        string            `yaml:"agent"`
	Prompt       string            `yaml:"prompt"`
	StartDelay   string            `yaml:"start_delay"`
	Assertions   []Assertion       `yaml:"assertions"`
	Extractors   []DataExtractor   `yaml:"extractors"`
	Variables    map[string]string `yaml:"variables,omitempty"`
	AllowedTools []string          `yaml:"allowed_tools,omitempty"`
}

type Assertion struct {
	Type     string            `yaml:"type"`
	Tool     string            `yaml:"tool,omitempty"`
	Value    string            `yaml:"value,omitempty"`
	Expected int               `yaml:"expected,omitempty"` // For cli_exit_code_equals
	Params   map[string]string `yaml:"params,omitempty"`
	Sequence []string          `yaml:"sequence,omitempty"`
	Pattern  string            `yaml:"pattern,omitempty"`
	Count    int               `yaml:"count,omitempty"`
	Path     string            `yaml:"path,omitempty"`

	// Boolean combinators (JSON Schema style)
	AnyOf []Assertion `yaml:"anyOf,omitempty"` // OR - pass if ANY child passes
	AllOf []Assertion `yaml:"allOf,omitempty"` // AND - pass if ALL children pass
	Not   *Assertion  `yaml:"not,omitempty"`   // NOT - pass if child FAILS
}

func (a Assertion) Clone() Assertion {
	// Copy map
	var params map[string]string
	if a.Params != nil {
		params = make(map[string]string, len(a.Params))
		for k, v := range a.Params {
			params[k] = v
		}
	}

	// Copy slice
	var sequence []string
	if a.Sequence != nil {
		sequence = make([]string, len(a.Sequence))
		copy(sequence, a.Sequence)
	}

	// Copy anyOf slice
	var anyOf []Assertion
	if a.AnyOf != nil {
		anyOf = make([]Assertion, len(a.AnyOf))
		for i, child := range a.AnyOf {
			anyOf[i] = child.Clone()
		}
	}

	// Copy allOf slice
	var allOf []Assertion
	if a.AllOf != nil {
		allOf = make([]Assertion, len(a.AllOf))
		for i, child := range a.AllOf {
			allOf[i] = child.Clone()
		}
	}

	// Copy not pointer
	var notAssertion *Assertion
	if a.Not != nil {
		cloned := a.Not.Clone()
		notAssertion = &cloned
	}

	return Assertion{
		Type:     a.Type,
		Tool:     a.Tool,
		Value:    a.Value,
		Params:   params,
		Sequence: sequence,
		Pattern:  a.Pattern,
		Count:    a.Count,
		Path:     a.Path,
		AnyOf:    anyOf,
		AllOf:    allOf,
		Not:      notAssertion,
	}
}

// ============================================================================
// EXECUTION RESULT
// ============================================================================

type ExecutionResult struct {
	TestName           string              `json:"testName"`
	AgentName          string              `json:"agentName"`
	ProviderType       ProviderType        `json:"providerType"`
	StartTime          time.Time           `json:"startTime"`
	EndTime            time.Time           `json:"endTime"`
	Messages           []Message           `json:"messages"`
	ToolCalls          []ToolCall          `json:"toolCalls"`
	FinalOutput        string              `json:"finalOutput"`
	TokensUsed         int                 `json:"tokensUsed"`
	LatencyMs          int64               `json:"latencyMs"`
	Errors             []string            `json:"errors"`
	SourceFile         string              `json:"sourceFile,omitempty"`         // Source test file (for suite runs)
	SuiteName          string              `json:"suiteName,omitempty"`          // Suite name (for suite runs)
	SessionName        string              `json:"sessionName,omitempty"`        // Session name
	RateLimitStats     *RateLimitStats     `json:"rateLimitStats,omitempty"`     // Rate limiting and 429 stats
	ClarificationStats *ClarificationStats `json:"clarificationStats,omitempty"` // Clarification detection stats
}

// ClarificationStats tracks when the LLM asks for clarification instead of acting
type ClarificationStats struct {
	Count      int      `json:"count"`      // Number of clarification requests detected
	Iterations []int    `json:"iterations"` // Which iterations had clarification requests
	Examples   []string `json:"examples"`   // Sample text from clarification requests (truncated)
}

// RateLimitStats tracks statistics about rate limiting and 429 handling
type RateLimitStats struct {
	// Proactive throttling stats
	ThrottleCount      int   `json:"throttleCount"`      // Number of times request was throttled
	ThrottleWaitTimeMs int64 `json:"throttleWaitTimeMs"` // Total time spent waiting due to throttling (ms)
	// Reactive 429 handling stats
	RateLimitHits     int   `json:"rateLimitHits"`     // Number of 429 errors received
	RetryCount        int   `json:"retryCount"`        // Number of retry attempts made
	RetryWaitTimeMs   int64 `json:"retryWaitTimeMs"`   // Total time spent waiting for retries (ms)
	RetrySuccessCount int   `json:"retrySuccessCount"` // Number of successful retries
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ToolCall struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
	Timestamp  time.Time              `json:"timestamp"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
	Result     Result                 `json:"result,omitempty"`
}

type Result struct {
	Content           []ContentItem     `json:"content"`
	StructuredContent StructuredContent `json:"structuredContent"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type StructuredContent struct {
	HasMore bool              `json:"has_more"`
	Result  []StructuredEntry `json:"result"`
	Total   int               `json:"total"`
}

type StructuredEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ============================================================================
// TEST RESULT
// ============================================================================

type AssertionResult struct {
	Type    string                 `json:"type"`
	Passed  bool                   `json:"passed"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
}

// ============================================================================
// DATA EXTRACTOR
// ============================================================================

type DataExtractor struct {
	ExtractorType string `yaml:"type"`
	Tool          string `yaml:"tool,omitempty"`
	Path          string `yaml:"path,omitempty"`
	VariableName  string `yaml:"variable_name,omitempty"`
}

func (e *DataExtractor) Extract(result *ExecutionResult, templateContext map[string]string) {
	if result == nil {
		return
	}
	for _, tc := range result.ToolCalls {
		if tc.Name != e.Tool {
			continue
		}
		switch e.ExtractorType {
		case "jsonpath":
			var data interface{}
			if err := json.Unmarshal([]byte(tc.Result.Content[0].Text), &data); err != nil {
				logger.Logger.Warn("Failed to unmarshal JSON: " + err.Error())
				continue
			}
			res, err := jsonpath.Read(data, e.Path)
			if err != nil {
				logger.Logger.Warn("Invalid JSONPath: " + err.Error())
				continue
			}
			logger.Logger.Debug("Extracted", "variable", e.VariableName, "value", fmt.Sprint(res))
			templateContext[e.VariableName] = normalize(res)
		default:
			continue
		}
	}

}

// ============================================================================
// ASSERTION EVALUATOR
// ============================================================================

type AssertionEvaluator struct {
	knownTools      []string
	result          *ExecutionResult
	templateContext map[string]string
}

func NewAssertionEvaluator(result *ExecutionResult, templateContext map[string]string, knownTools []string) *AssertionEvaluator {
	return &AssertionEvaluator{result: result, templateContext: templateContext, knownTools: knownTools}
}

func (e *AssertionEvaluator) Evaluate(assertions []Assertion) []AssertionResult {
	return e.evaluateWithDepth(assertions, 0)
}

const maxCombinatorDepth = 10 // Prevent infinite recursion

func (e *AssertionEvaluator) evaluateWithDepth(assertions []Assertion, depth int) []AssertionResult {
	results := make([]AssertionResult, 0, len(assertions))
	assertionsCopy := make([]Assertion, 0, len(assertions))

	for _, assertion := range assertions {
		assertionsCopy = append(assertionsCopy, assertion.Clone())
	}
	for _, assertion := range assertionsCopy {
		var result AssertionResult

		// Check for boolean combinators first
		if len(assertion.AnyOf) > 0 {
			result = e.evalAnyOf(assertion, depth)
			results = append(results, result)
			continue
		}
		if len(assertion.AllOf) > 0 {
			result = e.evalAllOf(assertion, depth)
			results = append(results, result)
			continue
		}
		if assertion.Not != nil {
			result = e.evalNot(assertion, depth)
			results = append(results, result)
			continue
		}

		//transform assertion params value
		if assertion.Params != nil {
			// pre-transform variables, if templated
			for k, v := range assertion.Params {
				t, err := raymond.Parse(v)
				if err == nil {
					transformed, err := t.Exec(e.templateContext)
					if err == nil {
						assertion.Params[k] = transformed
					}
				}
			}
		}

		// pre-transform variables, if templated
		if assertion.Value != "" {
			t, err := raymond.Parse(assertion.Value)
			if err == nil {
				transformed, err := t.Exec(e.templateContext)
				if err == nil {
					assertion.Value = transformed
				}
			}
		}

		switch assertion.Type {
		case "tool_called":
			result = e.evalToolCalled(assertion)
		case "tool_not_called":
			result = e.evalToolNotCalled(assertion)
		case "tool_call_count":
			result = e.evalToolCallCount(assertion)
		case "tool_call_order":
			result = e.evalToolCallOrder(assertion)
		case "tool_param_matches_regex":
			result = e.evalToolParamMatchesRegex(assertion)
		case "tool_param_equals":
			result = e.evalToolParamEquals(assertion)
		case "tool_result_matches_json":
			result = e.evalToolResultMatchesJson(assertion)
		case "output_contains":
			result = e.evalOutputContains(assertion)
		case "output_not_contains":
			result = e.evalOutputNotContains(assertion)
		case "output_regex":
			result = e.evalOutputRegex(assertion)
		case "max_tokens":
			result = e.evalMaxTokens(assertion)
		case "max_latency_ms":
			result = e.evalMaxLatency(assertion)
		case "no_error_messages":
			result = e.evalNoErrorMessages(assertion)
		case "no_hallucinated_tools":
			result = e.evalNoHallucinatedTools(assertion)
		case "no_clarification_questions":
			result = e.evalNoClarificationQuestions(assertion)
		case "no_rate_limit_errors":
			result = e.evalNoRateLimitErrors(assertion)
		case "cli_exit_code_equals":
			result = e.evalCLIExitCodeEquals(assertion)
		case "cli_stdout_contains":
			result = e.evalCLIStdoutContains(assertion)
		case "cli_stdout_regex":
			result = e.evalCLIStdoutRegex(assertion)
		case "cli_stderr_contains":
			result = e.evalCLIStderrContains(assertion)
		default:
			result = AssertionResult{
				Type:    assertion.Type,
				Passed:  false,
				Message: fmt.Sprintf("Unknown assertion type: %s", assertion.Type),
			}
		}

		results = append(results, result)
	}

	return results
}

// Tool assertions
func (e *AssertionEvaluator) evalToolCalled(a Assertion) AssertionResult {
	for _, tc := range e.result.ToolCalls {
		if tc.Name == a.Tool {
			return AssertionResult{
				Type:    a.Type,
				Passed:  true,
				Message: fmt.Sprintf("Tool '%s' was called", a.Tool),
			}
		}
	}
	return AssertionResult{
		Type:    a.Type,
		Passed:  false,
		Message: fmt.Sprintf("Tool '%s' was NOT called", a.Tool),
	}
}

func (e *AssertionEvaluator) evalToolNotCalled(a Assertion) AssertionResult {
	for _, tc := range e.result.ToolCalls {
		if tc.Name == a.Tool {
			return AssertionResult{
				Type:    a.Type,
				Passed:  false,
				Message: fmt.Sprintf("Tool '%s' was called but should not have been", a.Tool),
			}
		}
	}
	return AssertionResult{
		Type:    a.Type,
		Passed:  true,
		Message: fmt.Sprintf("Tool '%s' was not called (as expected)", a.Tool),
	}
}

func (e *AssertionEvaluator) evalToolParamEquals(a Assertion) AssertionResult {
	var mismatchesAll [][]string

	found := false

	for _, tc := range e.result.ToolCalls {
		if tc.Name != a.Tool {
			continue
		}
		found = true
		matches := true
		mismatches := []string{}

		for key, expectedVal := range a.Params {
			actualVal, exists := getNestedValue(tc.Parameters, key)
			if !exists {
				matches = false
				mismatches = append(mismatches, fmt.Sprintf("missing param '%s'", key))
				continue
			}
			if !DeepEqual(actualVal, expectedVal) {
				matches = false
				mismatches = append(mismatches,
					fmt.Sprintf("param '%s': expected %v, got %v", key, expectedVal, actualVal))
			}
		}

		if matches {
			// At least one call matches → assertion passed
			return AssertionResult{
				Type:    a.Type,
				Passed:  true,
				Message: fmt.Sprintf("Tool '%s' called with correct parameters", a.Tool),
			}
		}

		mismatchesAll = append(mismatchesAll, mismatches)
	}

	if !found {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Tool '%s' was not called", a.Tool),
		}
	}

	// None of the calls matched → collect all mismatches
	allMismatches := []string{}
	for _, mm := range mismatchesAll {
		allMismatches = append(allMismatches, mm...)
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  false,
		Message: fmt.Sprintf("Tool '%s' called with incorrect parameters: %v", a.Tool, allMismatches),
	}
}

func (e *AssertionEvaluator) evalToolResultMatchesJson(a Assertion) AssertionResult {
	var mismatchesAll []string
	found := false

	for _, tc := range e.result.ToolCalls {
		if tc.Name != a.Tool {
			continue
		}
		found = true

		var data interface{}
		if err := json.Unmarshal([]byte(fmt.Sprint(tc.Result.Content[0].Text)), &data); err != nil {
			mismatchesAll = append(mismatchesAll, fmt.Sprintf("Failed to parse JSON: %s", err))
			continue
		}

		res, err := jsonpath.Read(data, a.Path)
		if err != nil {
			mismatchesAll = append(mismatchesAll, fmt.Sprintf("Invalid JSONPath pattern: %s", err))
			continue
		}

		if DeepEqual(res, a.Value) {
			// At least one call matches → assertion passed
			return AssertionResult{
				Type:    a.Type,
				Passed:  true,
				Message: fmt.Sprintf("Tool '%s' result content matches parameters", a.Tool),
			}
		}

		mismatchesAll = append(mismatchesAll, fmt.Sprintf("Result does not match JSONPath '%s'", a.Path))
	}

	if !found {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Tool '%s' was not called", a.Tool),
		}
	}

	// None of the calls matched → return all mismatches
	return AssertionResult{
		Type:    a.Type,
		Passed:  false,
		Message: fmt.Sprintf("Tool '%s' result content did not match any call: %v", a.Tool, mismatchesAll),
	}
}

// getNestedValue retrieves a value from a nested map using dot notation
// e.g., "args.inner.ininner" will traverse m["args"]["inner"]["ininner"]
func getNestedValue(m map[string]interface{}, path string) (interface{}, bool) {
	keys := strings.Split(path, ".")
	var current interface{} = m

	for _, key := range keys {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}

		value, exists := currentMap[key]
		if !exists {
			return nil, false
		}

		current = value
	}

	return current, true
}
func (e *AssertionEvaluator) evalToolCallCount(a Assertion) AssertionResult {
	count := 0
	for _, tc := range e.result.ToolCalls {
		if tc.Name == a.Tool || a.Tool == "" {
			count++
		}
	}

	expected := a.Count
	passed := count == expected

	return AssertionResult{
		Type:    a.Type,
		Passed:  passed,
		Message: fmt.Sprintf("Tool '%s' called %d times (expected %d)", a.Tool, count, expected),
		Details: map[string]interface{}{
			"actual":   count,
			"expected": expected,
		},
	}
}

func (e *AssertionEvaluator) evalToolCallOrder(a Assertion) AssertionResult {
	if len(a.Sequence) == 0 {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: "No sequence specified",
		}
	}

	seqIdx := 0
	for _, tc := range e.result.ToolCalls {
		if seqIdx < len(a.Sequence) && tc.Name == a.Sequence[seqIdx] {
			seqIdx++
		}
	}

	passed := seqIdx == len(a.Sequence)

	if passed {
		return AssertionResult{
			Type:    a.Type,
			Passed:  true,
			Message: fmt.Sprintf("Tools called in correct order: %v", a.Sequence),
		}
	}

	return AssertionResult{
		Type:   a.Type,
		Passed: false,
		Message: fmt.Sprintf("Tool order mismatch: expected %v, matched %d/%d",
			a.Sequence, seqIdx, len(a.Sequence)),
	}
}

func (e *AssertionEvaluator) evalToolParamMatchesRegex(a Assertion) AssertionResult {
	var mismatchesAll [][]string

	found := false

	for _, tc := range e.result.ToolCalls {
		if tc.Name != a.Tool {
			continue
		}
		found = true
		matches := true
		mismatches := []string{}

		for key, expectedPattern := range a.Params {
			actualVal, exists := getNestedValue(tc.Parameters, key)
			if !exists {
				matches = false
				mismatches = append(mismatches, fmt.Sprintf("missing param '%s'", key))
				continue
			}

			// Convert expectedPattern to string and compile regex
			patternStr := fmt.Sprintf("%v", expectedPattern)
			re, err := regexp.Compile(patternStr)
			if err != nil {
				matches = false
				mismatches = append(mismatches,
					fmt.Sprintf("param '%s': invalid regex pattern '%s': %v", key, patternStr, err))
				continue
			}

			// Convert actual value to string and match against regex
			strVal := fmt.Sprintf("%v", actualVal)
			if !re.MatchString(strVal) {
				matches = false
				mismatches = append(mismatches,
					fmt.Sprintf("param '%s': value '%s' does not match regex '%s'", key, strVal, patternStr))
			}
		}

		if matches {
			// At least one call matches → assertion passed
			return AssertionResult{
				Type:    a.Type,
				Passed:  true,
				Message: fmt.Sprintf("Tool '%s' called with parameters matching regex patterns", a.Tool),
			}
		}

		mismatchesAll = append(mismatchesAll, mismatches)
	}

	if !found {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Tool '%s' was not called", a.Tool),
		}
	}

	// None of the calls matched → collect all mismatches
	allMismatches := []string{}
	for _, mm := range mismatchesAll {
		allMismatches = append(allMismatches, mm...)
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  false,
		Message: fmt.Sprintf("Tool '%s' called with parameters not matching regex: %v", a.Tool, allMismatches),
	}
}

// Output assertions
func (e *AssertionEvaluator) evalOutputContains(a Assertion) AssertionResult {
	needle := fmt.Sprintf("%v", a.Value)
	contains := regexp.MustCompile(regexp.QuoteMeta(needle)).MatchString(e.result.FinalOutput)

	return AssertionResult{
		Type:    a.Type,
		Passed:  contains,
		Message: fmt.Sprintf("Output contains '%s': %v", needle, contains),
	}
}

func (e *AssertionEvaluator) evalOutputNotContains(a Assertion) AssertionResult {
	needle := fmt.Sprintf("%v", a.Value)
	contains := regexp.MustCompile(regexp.QuoteMeta(needle)).MatchString(e.result.FinalOutput)

	return AssertionResult{
		Type:    a.Type,
		Passed:  !contains,
		Message: fmt.Sprintf("Output does not contain '%s': %v", needle, !contains),
	}
}

func (e *AssertionEvaluator) evalOutputRegex(a Assertion) AssertionResult {
	pattern := a.Pattern
	re, err := regexp.Compile(pattern)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Invalid regex: %s", err),
		}
	}

	matches := re.MatchString(e.result.FinalOutput)
	return AssertionResult{
		Type:    a.Type,
		Passed:  matches,
		Message: fmt.Sprintf("Output matches regex '%s': %v", pattern, matches),
	}
}

// Performance assertions
func (e *AssertionEvaluator) evalMaxTokens(a Assertion) AssertionResult {
	maxTokens, err := strconv.Atoi(a.Value)
	passed := false
	if err == nil {
		passed = e.result.TokensUsed <= maxTokens
	}
	return AssertionResult{
		Type:    a.Type,
		Passed:  passed,
		Message: fmt.Sprintf("Tokens used: %d (max: %d)", e.result.TokensUsed, maxTokens),
		Details: map[string]interface{}{
			"actual": e.result.TokensUsed,
			"max":    maxTokens,
		},
	}
}

func (e *AssertionEvaluator) evalMaxLatency(a Assertion) AssertionResult {
	maxLatency, err := strconv.ParseInt(a.Value, 10, 64)
	passed := false
	if err == nil {
		passed = e.result.LatencyMs <= maxLatency
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  passed,
		Message: fmt.Sprintf("Latency: %dms (max: %dms)", e.result.LatencyMs, maxLatency),
		Details: map[string]interface{}{
			"actual": e.result.LatencyMs,
			"max":    maxLatency,
		},
	}
}

func (e *AssertionEvaluator) evalNoHallucinatedTools(a Assertion) AssertionResult {
	if e.result.ToolCalls != nil {
		for i := range e.result.ToolCalls {
			if !contains(e.knownTools, e.result.ToolCalls[i].Name) {
				return AssertionResult{
					Type:    a.Type,
					Passed:  false,
					Message: fmt.Sprintf("Has hallucinated tool: %s", e.result.ToolCalls[i].Name),
				}
			}
		}
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  true,
		Message: "No hallucinated tools",
	}
}

func (e *AssertionEvaluator) evalNoErrorMessages(a Assertion) AssertionResult {
	hasErrors := len(e.result.Errors) > 0

	return AssertionResult{
		Type:    a.Type,
		Passed:  !hasErrors,
		Message: fmt.Sprintf("No error messages: %v (errors: %d)", !hasErrors, len(e.result.Errors)),
		Details: map[string]interface{}{
			"errors": e.result.Errors,
		},
	}
}

func (e *AssertionEvaluator) evalNoClarificationQuestions(a Assertion) AssertionResult {
	// Check if clarification detection was enabled
	if e.result.ClarificationStats == nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  true,
			Message: "Warning: clarification_detection not enabled on agent - assertion skipped",
			Details: map[string]interface{}{
				"warning": "Enable clarification_detection on the agent for this assertion to work",
			},
		}
	}

	// Check if clarification detection found any requests
	if e.result.ClarificationStats.Count > 0 {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Agent asked for clarification %d time(s)", e.result.ClarificationStats.Count),
			Details: map[string]interface{}{
				"count":      e.result.ClarificationStats.Count,
				"iterations": e.result.ClarificationStats.Iterations,
				"examples":   e.result.ClarificationStats.Examples,
			},
		}
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  true,
		Message: "No clarification questions detected",
	}
}

func (e *AssertionEvaluator) evalNoRateLimitErrors(a Assertion) AssertionResult {
	// Check if rate limit stats exist and have any 429 errors
	if e.result.RateLimitStats != nil && e.result.RateLimitStats.RateLimitHits > 0 {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Received %d rate limit error(s) (HTTP 429)", e.result.RateLimitStats.RateLimitHits),
			Details: map[string]interface{}{
				"rate_limit_hits":    e.result.RateLimitStats.RateLimitHits,
				"retry_count":        e.result.RateLimitStats.RetryCount,
				"retry_wait_time_ms": e.result.RateLimitStats.RetryWaitTimeMs,
				"retry_success":      e.result.RateLimitStats.RetrySuccessCount,
			},
		}
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  true,
		Message: "No rate limit errors (HTTP 429)",
	}
}

// ============================================================================
// CLI ASSERTION FUNCTIONS
// ============================================================================

// CLIResult represents the parsed JSON result from a CLI tool call
type CLIResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// parseCLIResult extracts CLI result from a tool call
func parseCLIResult(tc ToolCall) (*CLIResult, error) {
	if len(tc.Result.Content) == 0 {
		return nil, fmt.Errorf("no content in tool result")
	}

	var result CLIResult
	if err := json.Unmarshal([]byte(tc.Result.Content[0].Text), &result); err != nil {
		return nil, fmt.Errorf("failed to parse CLI result JSON: %w", err)
	}

	return &result, nil
}

// findCLIToolCall finds a CLI tool call by tool name pattern
// Looks for tools ending with "_execute" or exact match
func (e *AssertionEvaluator) findCLIToolCall(toolName string) (*ToolCall, *CLIResult, error) {
	for i := range e.result.ToolCalls {
		tc := &e.result.ToolCalls[i]
		// Match exact name or CLI execute pattern
		if tc.Name == toolName || strings.HasSuffix(tc.Name, "_execute") {
			result, err := parseCLIResult(*tc)
			if err == nil {
				return tc, result, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no CLI tool call found matching '%s'", toolName)
}

// evalCLIExitCodeEquals checks if CLI command returned expected exit code
// Supports both 'expected: 0' (int) and 'value: "0"' (string) formats
func (e *AssertionEvaluator) evalCLIExitCodeEquals(a Assertion) AssertionResult {
	// Use Expected field if set, otherwise parse Value for backwards compatibility
	expectedCode := a.Expected
	if a.Value != "" {
		parsed, err := strconv.Atoi(a.Value)
		if err != nil {
			return AssertionResult{
				Type:    a.Type,
				Passed:  false,
				Message: fmt.Sprintf("Invalid expected exit code value: %s", a.Value),
			}
		}
		expectedCode = parsed
	}

	// Find CLI tool call
	toolName := a.Tool
	if toolName == "" {
		toolName = "cli_execute" // Default CLI tool name
	}

	tc, cliResult, err := e.findCLIToolCall(toolName)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: err.Error(),
		}
	}

	passed := cliResult.ExitCode == expectedCode
	message := fmt.Sprintf("CLI exit code: %d (expected: %d)", cliResult.ExitCode, expectedCode)
	if !passed {
		message = fmt.Sprintf("CLI exit code mismatch: got %d, expected %d", cliResult.ExitCode, expectedCode)
	}

	return AssertionResult{
		Type:    a.Type,
		Passed:  passed,
		Message: message,
		Details: map[string]interface{}{
			"tool":          tc.Name,
			"actual_code":   cliResult.ExitCode,
			"expected_code": expectedCode,
		},
	}
}

// evalCLIStdoutContains checks if CLI stdout contains expected value
func (e *AssertionEvaluator) evalCLIStdoutContains(a Assertion) AssertionResult {
	toolName := a.Tool
	if toolName == "" {
		toolName = "cli_execute"
	}

	tc, cliResult, err := e.findCLIToolCall(toolName)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: err.Error(),
		}
	}

	contains := strings.Contains(cliResult.Stdout, a.Value)
	message := fmt.Sprintf("CLI stdout contains '%s': %v", a.Value, contains)

	return AssertionResult{
		Type:    a.Type,
		Passed:  contains,
		Message: message,
		Details: map[string]interface{}{
			"tool":           tc.Name,
			"expected":       a.Value,
			"stdout_len":     len(cliResult.Stdout),
			"stdout_preview": truncateString(cliResult.Stdout, 200),
		},
	}
}

// evalCLIStdoutRegex checks if CLI stdout matches regex pattern
func (e *AssertionEvaluator) evalCLIStdoutRegex(a Assertion) AssertionResult {
	toolName := a.Tool
	if toolName == "" {
		toolName = "cli_execute"
	}

	tc, cliResult, err := e.findCLIToolCall(toolName)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: err.Error(),
		}
	}

	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: fmt.Sprintf("Invalid regex pattern: %s", err),
		}
	}

	matches := re.MatchString(cliResult.Stdout)
	message := fmt.Sprintf("CLI stdout matches regex '%s': %v", a.Pattern, matches)

	return AssertionResult{
		Type:    a.Type,
		Passed:  matches,
		Message: message,
		Details: map[string]interface{}{
			"tool":           tc.Name,
			"pattern":        a.Pattern,
			"stdout_len":     len(cliResult.Stdout),
			"stdout_preview": truncateString(cliResult.Stdout, 200),
		},
	}
}

// evalCLIStderrContains checks if CLI stderr contains expected value
func (e *AssertionEvaluator) evalCLIStderrContains(a Assertion) AssertionResult {
	toolName := a.Tool
	if toolName == "" {
		toolName = "cli_execute"
	}

	tc, cliResult, err := e.findCLIToolCall(toolName)
	if err != nil {
		return AssertionResult{
			Type:    a.Type,
			Passed:  false,
			Message: err.Error(),
		}
	}

	contains := strings.Contains(cliResult.Stderr, a.Value)
	message := fmt.Sprintf("CLI stderr contains '%s': %v", a.Value, contains)

	return AssertionResult{
		Type:    a.Type,
		Passed:  contains,
		Message: message,
		Details: map[string]interface{}{
			"tool":           tc.Name,
			"expected":       a.Value,
			"stderr_len":     len(cliResult.Stderr),
			"stderr_preview": truncateString(cliResult.Stderr, 200),
		},
	}
}

// truncateString truncates a string to maxLen characters with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ============================================================================
// BOOLEAN COMBINATOR FUNCTIONS (anyOf, allOf, not)
// ============================================================================

// evalAnyOf evaluates an anyOf combinator - passes if ANY child assertion passes
func (e *AssertionEvaluator) evalAnyOf(a Assertion, depth int) AssertionResult {
	if depth >= maxCombinatorDepth {
		return AssertionResult{
			Type:    "anyOf",
			Passed:  false,
			Message: fmt.Sprintf("Maximum combinator nesting depth (%d) exceeded", maxCombinatorDepth),
		}
	}

	if len(a.AnyOf) == 0 {
		return AssertionResult{
			Type:    "anyOf",
			Passed:  false,
			Message: "anyOf requires at least one child assertion",
		}
	}

	childResults := e.evaluateWithDepth(a.AnyOf, depth+1)

	// Check if any child hit the depth limit - propagate that error
	for _, child := range childResults {
		if strings.Contains(child.Message, "Maximum combinator nesting depth") {
			return AssertionResult{
				Type:    "anyOf",
				Passed:  false,
				Message: child.Message,
				Details: map[string]interface{}{
					"children": childResults,
				},
			}
		}
	}

	passedCount := 0
	failedCount := 0

	for _, child := range childResults {
		if child.Passed {
			passedCount++
		} else {
			failedCount++
		}
	}

	passed := passedCount > 0
	message := fmt.Sprintf("anyOf failed: none of %d assertions passed", len(childResults))
	if passed {
		message = fmt.Sprintf("anyOf passed: %d of %d assertions passed", passedCount, len(childResults))
	}

	return AssertionResult{
		Type:    "anyOf",
		Passed:  passed,
		Message: message,
		Details: map[string]interface{}{
			"passed_count": passedCount,
			"failed_count": failedCount,
			"children":     childResults,
		},
	}
}

// evalAllOf evaluates an allOf combinator - passes if ALL child assertions pass
func (e *AssertionEvaluator) evalAllOf(a Assertion, depth int) AssertionResult {
	if depth >= maxCombinatorDepth {
		return AssertionResult{
			Type:    "allOf",
			Passed:  false,
			Message: fmt.Sprintf("Maximum combinator nesting depth (%d) exceeded", maxCombinatorDepth),
		}
	}

	if len(a.AllOf) == 0 {
		return AssertionResult{
			Type:    "allOf",
			Passed:  false,
			Message: "allOf requires at least one child assertion",
		}
	}

	childResults := e.evaluateWithDepth(a.AllOf, depth+1)

	// Check if any child hit the depth limit - propagate that error
	for _, child := range childResults {
		if strings.Contains(child.Message, "Maximum combinator nesting depth") {
			return AssertionResult{
				Type:    "allOf",
				Passed:  false,
				Message: child.Message,
				Details: map[string]interface{}{
					"children": childResults,
				},
			}
		}
	}

	passedCount := 0
	failedCount := 0

	for _, child := range childResults {
		if child.Passed {
			passedCount++
		} else {
			failedCount++
		}
	}

	passed := failedCount == 0
	message := fmt.Sprintf("allOf failed: %d of %d assertions failed", failedCount, len(childResults))
	if passed {
		message = fmt.Sprintf("allOf passed: all %d assertions passed", len(childResults))
	}

	return AssertionResult{
		Type:    "allOf",
		Passed:  passed,
		Message: message,
		Details: map[string]interface{}{
			"passed_count": passedCount,
			"failed_count": failedCount,
			"children":     childResults,
		},
	}
}

// evalNot evaluates a not combinator - passes if the child assertion FAILS
func (e *AssertionEvaluator) evalNot(a Assertion, depth int) AssertionResult {
	if depth >= maxCombinatorDepth {
		return AssertionResult{
			Type:    "not",
			Passed:  false,
			Message: fmt.Sprintf("Maximum combinator nesting depth (%d) exceeded", maxCombinatorDepth),
		}
	}

	if a.Not == nil {
		return AssertionResult{
			Type:    "not",
			Passed:  false,
			Message: "not requires a child assertion",
		}
	}

	childResults := e.evaluateWithDepth([]Assertion{*a.Not}, depth+1)
	childResult := childResults[0]

	// Check if child hit the depth limit - propagate that error
	if strings.Contains(childResult.Message, "Maximum combinator nesting depth") {
		return AssertionResult{
			Type:    "not",
			Passed:  false,
			Message: childResult.Message,
			Details: map[string]interface{}{
				"child": childResult,
			},
		}
	}

	// Invert the result
	passed := !childResult.Passed
	message := fmt.Sprintf("not failed: child assertion passed unexpectedly (%s)", childResult.Message)
	if passed {
		message = fmt.Sprintf("not passed: child assertion failed as expected (%s)", childResult.Message)
	}

	return AssertionResult{
		Type:    "not",
		Passed:  passed,
		Message: message,
		Details: map[string]interface{}{
			"child": childResult,
		},
	}
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================
func DeepEqual(a, b interface{}) bool {
	return normalize(a) == normalize(b)
}

func normalize(v interface{}) string {
	if v == nil {
		return "null"
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		var parts []string
		for i := 0; i < rv.Len(); i++ {
			parts = append(parts, normalize(rv.Index(i).Interface()))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if f == float64(int64(f)) {
			return fmt.Sprintf("%d", int64(f))
		}
		return fmt.Sprintf("%g", f)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", rv.Int())
	case reflect.String:
		return rv.String()
	default:
		return fmt.Sprint(v)
	}
}

// ============================================================================
// YAML PARSER
// ============================================================================

func ParseTestConfig(filename string) (*TestConfiguration, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var suite TestConfiguration
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &suite, nil
}

func ParseTestConfigFromString(definition string) (*TestConfiguration, error) {
	var config TestConfiguration
	if err := yaml.Unmarshal([]byte(definition), &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}

func ParseSuiteConfig(filename string) (*TestSuiteConfiguration, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var suite TestSuiteConfiguration
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &suite, nil
}

func ParseTestSuiteConfigFromString(definition string) (*TestSuiteConfiguration, error) {
	var config TestSuiteConfiguration
	if err := yaml.Unmarshal([]byte(definition), &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}

// ProviderOnlyConfig is a minimal config file containing just provider definitions
// Used for regenerating AI summaries without needing a full test configuration
type ProviderOnlyConfig struct {
	Providers []Provider `yaml:"providers"`
}

// ParseProviderConfig parses a YAML file containing only provider definitions
func ParseProviderConfig(filename string) (*ProviderOnlyConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config ProviderOnlyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}

// ============================================================================
// REPORT GENERATOR
// ============================================================================

// ServerTestResult represents the result of a test on a specific server
type ServerTestResult struct {
	ServerName  string        `json:"serverName"`
	AgentName   string        `json:"agentName"`
	Provider    ProviderType  `json:"provider"`
	Passed      bool          `json:"passed"`
	DurationRaw time.Duration `json:"-"`
	DurationMs  int64         `json:"duration"`
	Errors      []string      `json:"errors"`
}

// TestComparison represents a test run across multiple servers
type TestComparison struct {
	TestName      string                      `json:"testName"`
	ServerResults map[string]ServerTestResult `json:"serverResults"`
	TotalRuns     int                         `json:"totalRuns"`
	PassedRuns    int                         `json:"passedRuns"`
	FailedRuns    int                         `json:"failedRuns"`
}

// AgentStats holds aggregated statistics for an agent
type AgentStats struct {
	AgentName     string
	Provider      ProviderType
	TotalTests    int
	PassedTests   int
	FailedTests   int
	TotalTokens   int
	AvgTokens     int
	TotalDuration float64
	AvgDuration   float64
}
type ReportGenerator struct {
	TestFile string // Path to the original test configuration file
}

func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{}
}

// TestRun combines execution result with evaluated assertions
type TestRun struct {
	Execution    *ExecutionResult  `json:"execution"`
	Assertions   []AssertionResult `json:"assertions"`
	Passed       bool              `json:"passed"`
	TestCriteria Criteria          `json:"testCriteria"`
}

// GenerateComparisonSummary generates a comparison report across servers
func (rg *ReportGenerator) GenerateComparisonSummary(results []TestRun) map[string]TestComparison {
	comparisons := make(map[string]TestComparison)

	for _, run := range results {
		testName := run.Execution.TestName

		if _, exists := comparisons[testName]; !exists {
			comparisons[testName] = TestComparison{
				TestName:      testName,
				ServerResults: make(map[string]ServerTestResult),
				TotalRuns:     0,
				PassedRuns:    0,
				FailedRuns:    0,
			}
		}

		comp := comparisons[testName]
		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

		serverResult := ServerTestResult{
			ServerName:  run.Execution.AgentName,
			AgentName:   run.Execution.AgentName,
			Provider:    run.Execution.ProviderType,
			Passed:      run.Passed,
			DurationRaw: duration,
			DurationMs:  duration.Milliseconds(),
			Errors:      run.Execution.Errors,
		}

		comp.ServerResults[run.Execution.AgentName] = serverResult
		comp.TotalRuns++
		if run.Passed {
			comp.PassedRuns++
		} else {
			comp.FailedRuns++
		}

		comparisons[testName] = comp
	}

	return comparisons
}

// printComparisonSummary prints the comparison summary section
func (rg *ReportGenerator) printComparisonSummary(results []TestRun) {
	comparisons := rg.GenerateComparisonSummary(results)

	fmt.Println("\n" + "═══════════════════════════════════════════════════════════════")
	fmt.Println("                    SERVER COMPARISON SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	for testName, comp := range comparisons {
		successRate := float64(comp.PassedRuns) / float64(comp.TotalRuns) * 100
		rateColor := "\033[32m" // green
		if successRate < 100 && successRate >= 50 {
			rateColor = "\033[33m" // yellow
		} else if successRate < 50 {
			rateColor = "\033[31m" // red
		}

		fmt.Printf("📋 Test: %s ", testName)
		fmt.Printf("%s[%.0f%% passed]\033[0m\n", rateColor, successRate)
		fmt.Printf("   Summary: %d/%d servers passed\n\n", comp.PassedRuns, comp.TotalRuns)

		// Create a table-like view
		fmt.Println("   ┌─────────────────────────────────────────────────────────────┐")
		fmt.Printf("   │ %-25s │ %-10s │ %-10s │\n", "Server/Agent", "Status", "Duration")
		fmt.Println("   ├─────────────────────────────────────────────────────────────┤")

		for serverName, result := range comp.ServerResults {
			status := "\033[32m✓ PASS\033[0m"
			if !result.Passed {
				status = "\033[31m✗ FAIL\033[0m"
			}

			fmt.Printf("   │ %-25s │ %-19s │ %8.2fs │\n",
				truncate(serverName, 25),
				status,
				float64(result.DurationMs)/1000.0)

			// Show provider
			fmt.Printf("   │   └─ [%s]%-16s │            │          │\n",
				result.Provider,
				"")
		}

		fmt.Println("   └─────────────────────────────────────────────────────────────┘")

		// Show which servers failed
		if comp.FailedRuns > 0 {
			fmt.Println("\n   \033[31mFailed on:\033[0m")
			for serverName, result := range comp.ServerResults {
				if !result.Passed {
					fmt.Printf("   • %s [%s]\n", serverName, result.Provider)
					if len(result.Errors) > 0 {
						for _, err := range result.Errors {
							fmt.Printf("     └─ %s\n", err)
						}
					}
				}
			}
		}

		fmt.Println()
	}
}

// GenerateConsoleReport prints test results to console with comparison summary
func (rg *ReportGenerator) GenerateConsoleReport(results []TestRun) {
	// First show the comparison summary
	rg.printComparisonSummary(results)

	// Then show detailed results
	fmt.Println("\n" + "═══════════════════════════════════════════════════════════════")
	fmt.Println("                     DETAILED TEST RESULTS")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	passed := 0
	failed := 0

	// Group results by test name
	testGroups := make(map[string][]TestRun)
	for _, result := range results {
		testGroups[result.Execution.TestName] = append(testGroups[result.Execution.TestName], result)
	}

	for testName, testRuns := range testGroups {
		fmt.Printf("📋 Test: %s\n", testName)

		for _, run := range testRuns {
			duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

			if run.Passed {
				passed++
				fmt.Printf("  ✓ %s [%s] (%.2fs)\n",
					run.Execution.AgentName,
					run.Execution.ProviderType,
					duration.Seconds())
			} else {
				failed++
				fmt.Printf("  ✗ %s [%s] (%.2fs)\n",
					run.Execution.AgentName,
					run.Execution.ProviderType,
					duration.Seconds())
			}

			// Show assertion details
			for _, assertion := range run.Assertions {
				symbol := "✓"
				color := "\033[32m" // green
				if !assertion.Passed {
					symbol = "✗"
					color = "\033[31m" // red
				}
				fmt.Printf("    %s%s\033[0m %s: %s\n", color, symbol, assertion.Type, assertion.Message)

				// Show additional details if available
				if len(assertion.Details) > 0 {
					for k, v := range assertion.Details {
						fmt.Printf("      • %s: %v\n", k, v)
					}
				}
			}

			// Show errors
			if len(run.Execution.Errors) > 0 {
				fmt.Println("    \033[31mErrors:\033[0m")
				for _, err := range run.Execution.Errors {
					fmt.Printf("      • %s\n", err)
				}
			}
			fmt.Println()
		}
	}

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Total: %d | \033[32mPassed: %d\033[0m | \033[31mFailed: %d\033[0m\n",
		passed+failed, passed, failed)
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
}

func (rg *ReportGenerator) GenerateMarkdownReport(results []TestRun) string {
	var md string

	md += "# Test Results\n\n"
	md += fmt.Sprintf("**Agent Benchmark Version:** %s\n", version.Version)
	md += fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format(time.RFC3339))

	passed := 0
	failed := 0

	// Group results by test name
	testGroups := make(map[string][]TestRun)
	for _, result := range results {
		testGroups[result.Execution.TestName] = append(testGroups[result.Execution.TestName], result)
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}

	md += "## Summary\n\n"
	md += fmt.Sprintf("- **Total:** %d\n", passed+failed)
	md += fmt.Sprintf("- **Passed:** %d\n", passed)
	md += fmt.Sprintf("- **Failed:** %d\n\n", failed)

	// Add comparison summary
	md += "## Server Comparison Summary\n\n"
	comparisons := rg.GenerateComparisonSummary(results)

	for testName, comp := range comparisons {
		successRate := float64(comp.PassedRuns) / float64(comp.TotalRuns) * 100
		md += fmt.Sprintf("### %s (%.0f%% passed)\n\n", testName, successRate)
		md += fmt.Sprintf("**Summary:** %d/%d servers passed\n\n", comp.PassedRuns, comp.TotalRuns)

		md += "| Server/Agent | Provider | Status | Duration |\n"
		md += "|--------------|----------|--------|----------|\n"

		for serverName, result := range comp.ServerResults {
			status := "✅ PASS"
			if !result.Passed {
				status = "❌ FAIL"
			}

			md += fmt.Sprintf("| %s | %s | %s | %.2fs |\n",
				serverName,
				result.Provider,
				status,
				float64(result.DurationMs)/1000.0)
		}

		if comp.FailedRuns > 0 {
			md += "\n**Failed on:**\n\n"
			for serverName, result := range comp.ServerResults {
				if !result.Passed {
					md += fmt.Sprintf("- **%s** [%s]\n", serverName, result.Provider)
					if len(result.Errors) > 0 {
						for _, err := range result.Errors {
							md += fmt.Sprintf("  - %s\n", err)
						}
					}
				}
			}
		}

		md += "\n"
	}

	md += "---\n\n"
	md += "## Detailed Test Results\n\n"

	for testName, testRuns := range testGroups {
		md += fmt.Sprintf("### %s\n\n", testName)

		for _, run := range testRuns {
			status := "✅"
			if !run.Passed {
				status = "❌"
			}

			duration := run.Execution.EndTime.Sub(run.Execution.StartTime)
			md += fmt.Sprintf("#### %s %s [%s]\n\n", status, run.Execution.AgentName, run.Execution.ProviderType)
			md += fmt.Sprintf("- **Duration:** %.2fs\n", duration.Seconds())

			if len(run.Assertions) > 0 {
				md += "- **Tests:**\n"
				for _, assertion := range run.Assertions {
					assertStatus := "✅"
					if !assertion.Passed {
						assertStatus = "❌"
					}
					md += fmt.Sprintf("  - %s `%s`: %s\n", assertStatus, assertion.Type, assertion.Message)
				}
			}

			if len(run.Execution.Errors) > 0 {
				md += "- **Errors:**\n"
				for _, err := range run.Execution.Errors {
					md += fmt.Sprintf("  - %s\n", err)
				}
			}

			md += "\n"
		}
	}

	return md
}

// AISummaryData represents the AI summary to include in reports.
// This is a simple struct to avoid circular imports with the agent package.
type AISummaryData struct {
	Success   bool   `json:"success"`
	Analysis  string `json:"analysis,omitempty"`
	Error     string `json:"error,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	Guidance  string `json:"guidance,omitempty"`
}

func (rg *ReportGenerator) GenerateJSONReport(results []TestRun) string {
	return rg.GenerateJSONReportWithAnalysis(results, nil)
}

func (rg *ReportGenerator) GenerateJSONReportWithAnalysis(results []TestRun, aiSummary *AISummaryData) string {
	comparisons := rg.GenerateComparisonSummary(results)

	// Create a structured report with comparison
	reportData := map[string]interface{}{
		"agent_benchmark_version": version.Version,
		"generated_at":            time.Now().Format(time.RFC3339),
		"test_file":               rg.TestFile,
		"summary": map[string]interface{}{
			"total":  len(results),
			"passed": countPassed(results),
			"failed": countFailed(results),
		},
		"comparison_summary": comparisons,
		"detailed_results":   results,
	}

	// NOTE: ai_summary is NOT included in JSON output
	// AI summary is generated fresh during HTML/MD report generation (late-binding)
	// This keeps JSON as pure test data for reproducibility

	report, err := json.MarshalIndent(reportData, "", "  ")
	if err != nil {
		logger.Logger.Warn("Failed to generate JSON report")
		return "{}"
	}
	return string(report)
}

// generateAgentStats aggregates statistics by agent
func generateAgentStats(results []TestRun) []AgentStats {
	statsMap := make(map[string]*AgentStats)

	for _, result := range results {
		agentName := result.Execution.AgentName

		if _, exists := statsMap[agentName]; !exists {
			statsMap[agentName] = &AgentStats{
				AgentName: agentName,
				Provider:  result.Execution.ProviderType,
			}
		}

		stats := statsMap[agentName]
		stats.TotalTests++

		if result.Passed {
			stats.PassedTests++
		} else {
			stats.FailedTests++
		}

		stats.TotalTokens += result.Execution.TokensUsed
		duration := result.Execution.EndTime.Sub(result.Execution.StartTime).Seconds()
		stats.TotalDuration += duration
	}

	// Calculate averages and convert to slice
	statsList := make([]AgentStats, 0, len(statsMap))
	for _, stats := range statsMap {
		if stats.TotalTests > 0 {
			stats.AvgTokens = stats.TotalTokens / stats.TotalTests
			stats.AvgDuration = stats.TotalDuration / float64(stats.TotalTests)
		}
		statsList = append(statsList, *stats)
	}

	return statsList
}

// formatNumber formats numbers with thousand separators
func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// Helper functions for counting
func countPassed(results []TestRun) int {
	count := 0
	for _, r := range results {
		if r.Passed {
			count++
		}
	}
	return count
}

func countFailed(results []TestRun) int {
	count := 0
	for _, r := range results {
		if !r.Passed {
			count++
		}
	}
	return count
}

// Helper function to truncate strings
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func GetAllEnv() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	return envMap
}

// RenderTemplate safely parses and executes a Raymond template.
// If parsing or execution fails, it returns the input string unchanged.
func RenderTemplate(input string, context map[string]string) string {
	tmpl, err := raymond.Parse(input)
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		return input
	}

	output, err := tmpl.Exec(context)
	if err != nil {
		log.Printf("Failed to execute template: %v", err)
		return input
	}

	return output
}
