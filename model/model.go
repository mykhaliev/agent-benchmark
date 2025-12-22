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
}

// ============================================================================
// PROVIDER CONFIGURATION
// ============================================================================

type Provider struct {
	Name            string       `yaml:"name"`
	Type            ProviderType `yaml:"type"`
	Token           string       `yaml:"token"`
	Secret          string       `yaml:"secret"`
	Model           string       `yaml:"model"`
	BaseURL         string       `yaml:"baseUrl"`          // e.g., gpt-4o-mini
	Version         string       `yaml:"version"`          // e.g., 2025-01-01-preview
	ProjectID       string       `yaml:"project_id"`       // e.g., 2025-01-01-preview
	Location        string       `yaml:"location"`         // e.g., 2025-01-01-preview
	CredentialsPath string       `yaml:"credentials_path"` // e.g., 2025-01-01-preview
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
}

type ServerType string

const (
	Stdio ServerType = "stdio"
	SSE   ServerType = "sse"
	Http  ServerType = "http"
)

// ============================================================================
// AGENT CONFIGURATION
// ============================================================================

type Agent struct {
	Name     string        `yaml:"name"`
	Settings Settings      `yaml:"settings"`
	Servers  []AgentServer `yaml:"servers"`
	Provider string        `yaml:"provider"`
}

type AgentServer struct {
	Name         string   `yaml:"name"`
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
}

// ============================================================================
// TEST RESULT
// ============================================================================

type Criteria struct {
	SuccessRate string `yaml:"success_rate"`
}

// ============================================================================
// AGENT CONFIGURATION
// ============================================================================

type Settings struct {
	Verbose       bool   `yaml:"verbose"`
	ToolTimeout   string `yaml:"tool_tool_timeout"`
	MaxIterations int    `yaml:"max_iterations"`
	TestDelay     string `yaml:"test_delay"`
}

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
	Params   map[string]string `yaml:"params,omitempty"`
	Sequence []string          `yaml:"sequence,omitempty"`
	Pattern  string            `yaml:"pattern,omitempty"`
	Count    int               `yaml:"count,omitempty"`
	Path     string            `yaml:"path,omitempty"`
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

	return Assertion{
		Type:     a.Type,
		Tool:     a.Tool,
		Value:    a.Value,
		Params:   params,
		Sequence: sequence,
		Pattern:  a.Pattern,
		Count:    a.Count,
		Path:     a.Path,
	}
}

// ============================================================================
// EXECUTION RESULT
// ============================================================================

type ExecutionResult struct {
	TestName      string
	AgentName     string
	ProviderType  ProviderType
	StartTime     time.Time
	EndTime       time.Time
	Messages      []Message
	ToolCalls     []ToolCall
	FinalOutput   string
	TokensUsed    int
	LatencyMs     int64
	Errors        []string
	MCPOperations MCPOperations
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
	Result     Result                 `json:"result,omitempty"`
}

type MCPOperations struct {
	ResourcesRead []string
	FilesCreated  []string
	FilesWritten  []string
	FilesDeleted  []string
	ToolsList     []string
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
	Type    string
	Passed  bool
	Message string
	Details map[string]interface{}
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
			templateContext[e.VariableName] = fmt.Sprint(res)
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
	results := make([]AssertionResult, 0, len(assertions))
	assertionsCopy := make([]Assertion, 0, len(assertions))

	for _, assertion := range assertions {
		assertionsCopy = append(assertionsCopy, assertion.Clone())
	}
	for _, assertion := range assertionsCopy {
		var result AssertionResult

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
		if tc.Name == a.Tool {
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
		Message: fmt.Sprintf("No hallucinated tools"),
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

	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		var parts []string
		for i := 0; i < rv.Len(); i++ {
			parts = append(parts, normalize(rv.Index(i).Interface()))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}

	return fmt.Sprint(v)
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

// ============================================================================
// REPORT GENERATOR
// ============================================================================

// ServerTestResult represents the result of a test on a specific server
type ServerTestResult struct {
	ServerName string
	AgentName  string
	Provider   ProviderType
	Passed     bool
	Duration   time.Duration
	Errors     []string
}

// TestComparison represents a test run across multiple servers
type TestComparison struct {
	TestName      string
	ServerResults map[string]ServerTestResult // key: server or agent name
	TotalRuns     int
	PassedRuns    int
	FailedRuns    int
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
type ReportGenerator struct{}

func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{}
}

// TestRun combines execution result with evaluated assertions
type TestRun struct {
	Execution    *ExecutionResult
	Assertions   []AssertionResult
	Passed       bool
	TestCriteria Criteria
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
			ServerName: run.Execution.AgentName,
			AgentName:  run.Execution.AgentName,
			Provider:   run.Execution.ProviderType,
			Passed:     run.Passed,
			Duration:   duration,
			Errors:     run.Execution.Errors,
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
	fmt.Println("═══════════════════════════════════════════════════════════════\n")

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
				result.Duration.Seconds())

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
	fmt.Println("═══════════════════════════════════════════════════════════════\n")

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
	fmt.Println("═══════════════════════════════════════════════════════════════\n")
}

func (rg *ReportGenerator) GenerateMarkdownReport(results []TestRun) string {
	var md string

	md += "# Test Results\n\n"
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
				result.Duration.Seconds())
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

func (rg *ReportGenerator) GenerateHTMLReport(results []TestRun) string {
	passed := 0
	failed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}

	// Group results by test name
	testGroups := make(map[string][]TestRun)
	for _, result := range results {
		testGroups[result.Execution.TestName] = append(testGroups[result.Execution.TestName], result)
	}

	// Generate agent statistics
	agentStats := generateAgentStats(results)

	comparisons := rg.GenerateComparisonSummary(results)

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Test Results</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1, h2 { color: #333; border-bottom: 3px solid #4CAF50; padding-bottom: 10px; }
        h2 { border-bottom: 2px solid #2196F3; margin-top: 40px; }
        .summary { display: flex; gap: 20px; margin: 20px 0; }
        .stat { padding: 15px 25px; border-radius: 6px; flex: 1; text-align: center; }
        .stat-total { background: #2196F3; color: white; }
        .stat-passed { background: #4CAF50; color: white; }
        .stat-failed { background: #f44336; color: white; }
        .stat-value { font-size: 32px; font-weight: bold; }
        .stat-label { font-size: 14px; opacity: 0.9; margin-top: 5px; }
        
        .agent-stats-section { margin: 30px 0; border: 1px solid #e0e0e0; border-radius: 8px; overflow: hidden; background: white; }
        .agent-stats-header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 20px; color: white; }
        .agent-stats-title { font-size: 20px; font-weight: bold; margin: 0; }
        .agent-stats-table { width: 100%; border-collapse: collapse; }
        .agent-stats-table th { background: #f5f5f5; padding: 14px 12px; text-align: left; font-weight: 600; color: #555; border-bottom: 2px solid #e0e0e0; font-size: 13px; text-transform: uppercase; letter-spacing: 0.5px; }
        .agent-stats-table td { padding: 14px 12px; border-bottom: 1px solid #f0f0f0; font-size: 14px; }
        .agent-stats-table tbody tr:hover { background: #fafafa; transition: background 0.2s; }
        .agent-name-cell { font-weight: 600; color: #333; }
        .provider-tag { background: #e3f2fd; color: #1976d2; padding: 4px 10px; border-radius: 12px; font-size: 11px; display: inline-block; font-weight: 500; }
        .test-count { color: #666; font-weight: 500; }
        .pass-count { color: #4CAF50; font-weight: 600; }
        .fail-count { color: #f44336; font-weight: 600; }
        .success-percentage { padding: 4px 10px; border-radius: 6px; font-weight: 600; font-size: 13px; display: inline-block; }
        .success-high { background: #c8e6c9; color: #2e7d32; }
        .success-medium { background: #fff9c4; color: #f57f17; }
        .success-low { background: #ffcdd2; color: #c62828; }
        .metric-cell { text-align: right; color: #666; font-variant-numeric: tabular-nums; }
        .duration-cell { text-align: right; font-family: 'Courier New', monospace; color: #555; }
        .tokens-cell { text-align: right; font-family: 'Courier New', monospace; color: #555; }
        
        .comparison-section { margin: 30px 0; border: 1px solid #e0e0e0; border-radius: 8px; overflow: hidden; }
        .comparison-header { background: #f5f5f5; padding: 15px 20px; border-bottom: 1px solid #e0e0e0; }
        .comparison-title { font-size: 18px; font-weight: bold; color: #333; }
        .success-rate { display: inline-block; padding: 4px 12px; border-radius: 6px; font-weight: 600; margin-left: 10px; font-size: 14px; }
        .success-rate.high { background: #c8e6c9; color: #2e7d32; }
        .success-rate.medium { background: #fff9c4; color: #f57f17; }
        .success-rate.low { background: #ffcdd2; color: #c62828; }
        .comparison-table { width: 100%; border-collapse: collapse; }
        .comparison-table th { background: #fafafa; padding: 12px; text-align: left; font-weight: 600; color: #555; border-bottom: 2px solid #e0e0e0; }
        .comparison-table td { padding: 12px; border-bottom: 1px solid #f0f0f0; }
        .comparison-table tr:hover { background: #fafafa; }
        .status-pass { color: #4CAF50; font-weight: 600; }
        .status-fail { color: #f44336; font-weight: 600; }
        .provider-badge { background: #e3f2fd; color: #1976d2; padding: 4px 10px; border-radius: 12px; font-size: 12px; display: inline-block; }
        .failed-servers { background: #ffebee; padding: 15px; margin: 0 20px 20px 20px; border-radius: 6px; border-left: 4px solid #f44336; }
        .failed-servers h4 { margin: 0 0 10px 0; color: #c62828; font-size: 14px; }
        .error-list { list-style: none; padding-left: 20px; margin: 5px 0; }
        .error-item { color: #666; font-size: 13px; margin: 3px 0; }
        
        .test-group { margin: 30px 0; }
        .test-name { font-size: 20px; font-weight: bold; color: #333; margin-bottom: 15px; }
        .test-result { background: #fafafa; padding: 15px; margin: 10px 0; border-left: 4px solid #ccc; border-radius: 4px; }
        .test-result.passed { border-left-color: #4CAF50; }
        .test-result.failed { border-left-color: #f44336; }
        .test-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
        .test-title { font-weight: 600; color: #555; }
        .duration { color: #666; font-size: 14px; }
        .assertions { margin-left: 20px; }
        .assertion { padding: 8px; margin: 5px 0; font-size: 14px; }
        .assertion.passed { color: #2e7d32; }
        .assertion.failed { color: #c62828; }
        .errors { background: #ffebee; padding: 10px; margin-top: 10px; border-radius: 4px; }
        .error-item { color: #c62828; margin: 5px 0; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🧪 Test Results</h1>
        <p><strong>Generated:</strong> ` + time.Now().Format(time.RFC3339) + `</p>
        
        <div class="summary">
            <div class="stat stat-total">
                <div class="stat-value">` + fmt.Sprintf("%d", passed+failed) + `</div>
                <div class="stat-label">Total Tests</div>
            </div>
            <div class="stat stat-passed">
                <div class="stat-value">` + fmt.Sprintf("%d", passed) + `</div>
                <div class="stat-label">Passed</div>
            </div>
            <div class="stat stat-failed">
                <div class="stat-value">` + fmt.Sprintf("%d", failed) + `</div>
                <div class="stat-label">Failed</div>
            </div>
        </div>
        
        <h2>🤖 Agent Performance Comparison</h2>
        <div class="agent-stats-section">
            <div class="agent-stats-header">
                <h3 class="agent-stats-title">📊 Performance Metrics by Agent</h3>
            </div>
            <table class="agent-stats-table">
                <thead>
                    <tr>
                        <th>Agent</th>
                        <th>Provider</th>
                        <th style="text-align: center;">Tests</th>
                        <th style="text-align: center;">Passed</th>
                        <th style="text-align: center;">Failed</th>
                        <th style="text-align: center;">Success Rate</th>
                        <th style="text-align: right;">Avg Duration</th>
                        <th style="text-align: right;">Total Tokens</th>
                        <th style="text-align: right;">Avg Tokens</th>
                    </tr>
                </thead>
                <tbody>`

	// Add agent statistics rows
	for _, stats := range agentStats {
		successRate := 0.0
		if stats.TotalTests > 0 {
			successRate = float64(stats.PassedTests) / float64(stats.TotalTests) * 100
		}

		rateClass := "success-high"
		if successRate < 100 && successRate >= 50 {
			rateClass = "success-medium"
		} else if successRate < 50 {
			rateClass = "success-low"
		}

		html += `
                    <tr>
                        <td class="agent-name-cell">` + stats.AgentName + `</td>
                        <td><span class="provider-tag">` + string(stats.Provider) + `</span></td>
                        <td class="metric-cell test-count">` + fmt.Sprintf("%d", stats.TotalTests) + `</td>
                        <td class="metric-cell pass-count">` + fmt.Sprintf("%d", stats.PassedTests) + `</td>
                        <td class="metric-cell fail-count">` + fmt.Sprintf("%d", stats.FailedTests) + `</td>
                        <td style="text-align: center;"><span class="success-percentage ` + rateClass + `">` + fmt.Sprintf("%.1f%%", successRate) + `</span></td>
                        <td class="duration-cell">` + fmt.Sprintf("%.2fs", stats.AvgDuration) + `</td>
                        <td class="tokens-cell">` + fmt.Sprintf("%s", formatNumber(stats.TotalTokens)) + `</td>
                        <td class="tokens-cell">` + fmt.Sprintf("%s", formatNumber(stats.AvgTokens)) + `</td>
                    </tr>`
	}

	html += `
                </tbody>
            </table>
        </div>
        
        <h2>🔄 Server Comparison Summary</h2>`

	// Rest of the HTML generation remains the same...
	for testName, comp := range comparisons {
		successRate := float64(comp.PassedRuns) / float64(comp.TotalRuns) * 100
		rateClass := "high"
		if successRate < 100 && successRate >= 50 {
			rateClass = "medium"
		} else if successRate < 50 {
			rateClass = "low"
		}

		html += `
        <div class="comparison-section">
            <div class="comparison-header">
                <span class="comparison-title">📋 ` + testName + `</span>
                <span class="success-rate ` + rateClass + `">` + fmt.Sprintf("%.0f%%", successRate) + `</span>
                <div style="font-size: 13px; color: #666; margin-top: 5px;">` + fmt.Sprintf("%d/%d servers passed", comp.PassedRuns, comp.TotalRuns) + `</div>
            </div>
            <table class="comparison-table">
                <thead>
                    <tr>
                        <th>Server/Agent</th>
                        <th>Provider</th>
                        <th>Status</th>
                        <th>Duration</th>
                    </tr>
                </thead>
                <tbody>`

		for serverName, result := range comp.ServerResults {
			statusClass := "status-pass"
			statusText := "✅ PASS"
			if !result.Passed {
				statusClass = "status-fail"
				statusText = "❌ FAIL"
			}

			html += `
                    <tr>
                        <td><strong>` + serverName + `</strong></td>
                        <td><span class="provider-badge">` + string(result.Provider) + `</span></td>
                        <td class="` + statusClass + `">` + statusText + `</td>
                        <td>` + fmt.Sprintf("%.2fs", result.Duration.Seconds()) + `</td>
                    </tr>`
		}

		html += `
                </tbody>
            </table>`

		if comp.FailedRuns > 0 {
			html += `
            <div class="failed-servers">
                <h4>❌ Failed Servers</h4>`

			for serverName, result := range comp.ServerResults {
				if !result.Passed {
					html += `<div style="margin-bottom: 10px;"><strong>` + serverName + `</strong> [` + string(result.Provider) + `]`
					if len(result.Errors) > 0 {
						html += `<ul class="error-list">`
						for _, err := range result.Errors {
							html += `<li class="error-item">• ` + err + `</li>`
						}
						html += `</ul>`
					}
					html += `</div>`
				}
			}

			html += `</div>`
		}

		html += `
        </div>`
	}

	html += `
        <h2>📊 Detailed Test Results</h2>`

	for testName, testRuns := range testGroups {
		html += `<div class="test-group">
            <div class="test-name">📋 ` + testName + `</div>`

		for _, run := range testRuns {
			status := "passed"
			icon := "✅"
			if !run.Passed {
				status = "failed"
				icon = "❌"
			}

			duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

			html += `<div class="test-result ` + status + `">
                <div class="test-header">
                    <div class="test-title">` + icon + ` ` + run.Execution.AgentName + `</div>
                    <div>
                        <span class="provider-badge">` + string(run.Execution.ProviderType) + `</span>
                        <span class="duration">` + fmt.Sprintf("%.2fs", duration.Seconds()) + `</span>
                    </div>
                </div>`

			if len(run.Assertions) > 0 {
				html += `<div class="assertions">`
				for _, assertion := range run.Assertions {
					assertStatus := "passed"
					assertIcon := "✓"
					if !assertion.Passed {
						assertStatus = "failed"
						assertIcon = "✗"
					}
					html += `<div class="assertion ` + assertStatus + `">` + assertIcon + ` ` + assertion.Type + `: ` + assertion.Message + `</div>`
				}
				html += `</div>`
			}

			if len(run.Execution.Errors) > 0 {
				html += `<div class="errors"><strong>Errors:</strong>`
				for _, err := range run.Execution.Errors {
					html += `<div class="error-item">• ` + err + `</div>`
				}
				html += `</div>`
			}

			html += `</div>`
		}

		html += `</div>`
	}

	html += `</div>
</body>
</html>`

	return html
}

func (rg *ReportGenerator) GenerateJSONReport(results []TestRun) string {
	comparisons := rg.GenerateComparisonSummary(results)

	// Create a structured report with comparison
	reportData := map[string]interface{}{
		"generated_at": time.Now().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"total":  len(results),
			"passed": countPassed(results),
			"failed": countFailed(results),
		},
		"comparison_summary": comparisons,
		"detailed_results":   results,
	}

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
