package explorer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/generator"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/tmc/langchaingo/llms"
)

func TestMain(m *testing.M) {
	logger.SetupLogger(io.Discard, false)
	os.Exit(m.Run())
}

// ============================================================================
// TestParseExplorerConfig
// ============================================================================

func TestParseExplorerConfig_Valid(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o-mini
    token: "sk-test"

agents:
  - name: test-agent
    provider: test-llm

explorer:
  goal: "Explore edge cases in file system operations"
  max_iterations: 5
  max_retries: 2
  agent: test-agent
`
	cfg, err := parseExplorerConfigFromBytes([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Explorer.Goal != "Explore edge cases in file system operations" {
		t.Errorf("unexpected goal: %q", cfg.Explorer.Goal)
	}
	if cfg.Explorer.MaxIterations != 5 {
		t.Errorf("expected MaxIterations=5, got %d", cfg.Explorer.MaxIterations)
	}
	if cfg.Explorer.MaxRetries != 2 {
		t.Errorf("expected MaxRetries=2, got %d", cfg.Explorer.MaxRetries)
	}
	if cfg.Explorer.Agent != "test-agent" {
		t.Errorf("unexpected agent: %q", cfg.Explorer.Agent)
	}
}

func TestParseExplorerConfig_Defaults(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o
    token: sk-x

agents:
  - name: test-agent
    provider: test-llm

explorer:
  goal: "Test something"
`
	cfg, err := parseExplorerConfigFromBytes([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Explorer.MaxIterations != 10 {
		t.Errorf("expected default MaxIterations=10, got %d", cfg.Explorer.MaxIterations)
	}
	if cfg.Explorer.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries=3, got %d", cfg.Explorer.MaxRetries)
	}
	// Agent should default to the first agent name.
	if cfg.Explorer.Agent != "test-agent" {
		t.Errorf("expected default agent=test-agent, got %q", cfg.Explorer.Agent)
	}
}

func TestParseExplorerConfig_MissingGoal(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o
    token: sk-x

agents:
  - name: test-agent
    provider: test-llm

explorer:
  max_iterations: 5
`
	_, err := parseExplorerConfigFromBytes([]byte(content))
	if err == nil {
		t.Fatal("expected error for missing goal")
	}
	if !strings.Contains(err.Error(), "goal") {
		t.Errorf("error should mention 'goal': %v", err)
	}
}

func TestParseExplorerConfig_MaxTokens(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o
    token: sk-x

agents:
  - name: test-agent
    provider: test-llm

explorer:
  goal: "Test token limit config"
  max_tokens: 2500
`
	cfg, err := parseExplorerConfigFromBytes([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Explorer.MaxTokens != 2500 {
		t.Errorf("expected MaxTokens=2500, got %d", cfg.Explorer.MaxTokens)
	}
}

func TestParseExplorerConfig_MaxTokensDefaultZero(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o
    token: sk-x

agents:
  - name: test-agent
    provider: test-llm

explorer:
  goal: "Test unlimited tokens by default"
`
	cfg, err := parseExplorerConfigFromBytes([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Explorer.MaxTokens != 0 {
		t.Errorf("expected default MaxTokens=0 (unlimited), got %d", cfg.Explorer.MaxTokens)
	}
}

func TestParseExplorerConfig_MissingAgent(t *testing.T) {
	content := `
providers:
  - name: test-llm
    type: OPENAI
    model: gpt-4o
    token: sk-x

explorer:
  goal: "something"
`
	_, err := parseExplorerConfigFromBytes([]byte(content))
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

// ============================================================================
// TestBuildDecisionPrompt
// ============================================================================

func TestBuildDecisionPrompt_ContainsGoalAndTools(t *testing.T) {
	cfg := &ExplorerConfig{
		Explorer: ExplorerSettings{
			Goal:          "Test file operations",
			MaxIterations: 5,
			MaxRetries:    3,
		},
	}

	msgs, promptText := BuildDecisionPrompt(cfg, nil, nil, 1)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(promptText, "Test file operations") {
		t.Error("prompt should contain the goal")
	}
	if !strings.Contains(promptText, "CURRENT ITERATION: 1 of 5") {
		t.Error("prompt should contain iteration info")
	}
}

func TestBuildDecisionPrompt_ContainsHistory(t *testing.T) {
	cfg := &ExplorerConfig{
		Explorer: ExplorerSettings{
			Goal:          "My goal",
			MaxIterations: 10,
		},
	}
	history := []IterationContext{
		{Iteration: 1, TestName: "First Test", Prompt: "Do something", Passed: true, Summary: "worked fine"},
		{Iteration: 2, TestName: "Second Test", Prompt: "Do another thing", Passed: false, Summary: "error occurred"},
	}

	_, promptText := BuildDecisionPrompt(cfg, nil, history, 3)

	if !strings.Contains(promptText, "First Test") {
		t.Error("prompt should contain history test name")
	}
	if !strings.Contains(promptText, "PASSED") {
		t.Error("prompt should contain PASSED status")
	}
	if !strings.Contains(promptText, "FAILED") {
		t.Error("prompt should contain FAILED status")
	}
}

func TestBuildDecisionPrompt_JSONFormatInstructions(t *testing.T) {
	if !strings.Contains(explorerSystemPrompt, "JSON") {
		t.Error("system prompt should mention JSON output format")
	}
}

// ============================================================================
// TestExtractJSONFromResponse
// ============================================================================

func TestExtractJSONFromResponse_Plain(t *testing.T) {
	input := `{"name": "foo", "prompt": "bar", "checks": [], "reasoning": "baz"}`
	result := ExtractJSONFromResponse(input)
	if result != input {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestExtractJSONFromResponse_WithCodeFence(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	result := ExtractJSONFromResponse(input)
	if result != `{"key": "value"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestExtractJSONFromResponse_WithPlainFence(t *testing.T) {
	input := "```\n{\"key\": \"value\"}\n```"
	result := ExtractJSONFromResponse(input)
	if result != `{"key": "value"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

// ============================================================================
// TestParseExplorerDecision
// ============================================================================

func TestParseExplorerDecision_Valid(t *testing.T) {
	input := `{
		"name": "List files in /tmp",
		"goal": "verify directory listing",
		"session_name": "exploration",
		"prompt": "List all files in the /tmp directory",
		"checks": [{"type": "tool_called", "tool": "list_directory"}],
		"reasoning": "Testing basic file listing"
	}`

	d, err := ParseExplorerDecision(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Name != "List files in /tmp" {
		t.Errorf("unexpected Name: %q", d.Name)
	}
	if d.Prompt != "List all files in the /tmp directory" {
		t.Errorf("unexpected Prompt: %q", d.Prompt)
	}
	if len(d.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(d.Checks))
	}
	if d.Reasoning != "Testing basic file listing" {
		t.Errorf("unexpected Reasoning: %q", d.Reasoning)
	}
}

func TestParseExplorerDecision_MissingName(t *testing.T) {
	input := `{"prompt": "p", "checks": [{"type": "no_error_messages"}], "reasoning": "r"}`
	_, err := ParseExplorerDecision(input)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseExplorerDecision_MissingPrompt(t *testing.T) {
	input := `{"name": "t", "checks": [{"type": "no_error_messages"}], "reasoning": "r"}`
	_, err := ParseExplorerDecision(input)
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestParseExplorerDecision_NoChecks(t *testing.T) {
	// ParseExplorerDecision allows empty checks — ValidateTestIntent handles that.
	input := `{"name": "t", "prompt": "p", "checks": [], "reasoning": "r"}`
	d, err := ParseExplorerDecision(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Name != "t" {
		t.Errorf("unexpected name: %q", d.Name)
	}
}

func TestParseExplorerDecision_Malformed(t *testing.T) {
	_, err := ParseExplorerDecision("not json at all")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseExplorerDecision_AllCheckTypes(t *testing.T) {
	// All 20 valid check types (minus combinators) should parse without error.
	cases := []struct {
		name  string
		check string
	}{
		{"tool_called", `{"type": "tool_called", "tool": "my_tool"}`},
		{"tool_not_called", `{"type": "tool_not_called", "tool": "my_tool"}`},
		{"tool_call_count", `{"type": "tool_call_count", "tool": "my_tool", "count": 2}`},
		{"tool_call_order", `{"type": "tool_call_order", "sequence": ["tool_a", "tool_b"]}`},
		{"tool_param_equals", `{"type": "tool_param_equals", "tool": "my_tool", "params": {"key": "val"}}`},
		{"tool_param_matches_regex", `{"type": "tool_param_matches_regex", "tool": "my_tool", "params": {"key": ".*"}}`},
		{"tool_result_matches_json", `{"type": "tool_result_matches_json", "tool": "my_tool", "path": "$.id", "value": "1"}`},
		{"output_contains", `{"type": "output_contains", "value": "hello"}`},
		{"output_not_contains", `{"type": "output_not_contains", "value": "error"}`},
		{"output_regex", `{"type": "output_regex", "pattern": "\\d+"}`},
		{"no_error_messages", `{"type": "no_error_messages"}`},
		{"no_hallucinated_tools", `{"type": "no_hallucinated_tools"}`},
		{"no_clarification_questions", `{"type": "no_clarification_questions"}`},
		{"no_rate_limit_errors", `{"type": "no_rate_limit_errors"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(
				`{"name": "t", "prompt": "p", "checks": [%s], "reasoning": "r"}`,
				tc.check,
			)
			if _, err := ParseExplorerDecision(input); err != nil {
				t.Errorf("unexpected error for type %q: %v", tc.name, err)
			}
		})
	}
}

// ============================================================================
// TestRuntimeTestDefinitionMetadata
// ============================================================================

func TestRuntimeTestDefinitionMetadata_NamesEncodeContext(t *testing.T) {
	adapter := &ExplorationTestAdapter{}
	intent := generator.TestIntent{
		Name:   "Read a file",
		Prompt: "Read /tmp/test.txt",
		Checks: []generator.Check{
			{Type: "output_contains", Value: "hello"},
		},
	}

	rtd := adapter.Adapt(intent, "Basic read test", 3, "prompt-003", "Explore file system", "my-agent")

	// Test name must encode iteration and promptID.
	if !strings.Contains(rtd.Name, "[Iter 03 | prompt-003]") {
		t.Errorf("test name should contain iteration and promptID: %q", rtd.Name)
	}
	if !strings.Contains(rtd.Name, "Read a file") {
		t.Errorf("test name should contain original name: %q", rtd.Name)
	}

	// Agent must be the configured agent name.
	if rtd.Agent != "my-agent" {
		t.Errorf("agent should be %q, got %q", "my-agent", rtd.Agent)
	}

	// Metadata fields.
	if rtd.Metadata.Iteration != 3 {
		t.Errorf("expected Iteration=3, got %d", rtd.Metadata.Iteration)
	}
	if rtd.Metadata.PromptID != "prompt-003" {
		t.Errorf("expected PromptID=prompt-003, got %q", rtd.Metadata.PromptID)
	}
	if rtd.Metadata.Goal != "Explore file system" {
		t.Errorf("unexpected Goal: %q", rtd.Metadata.Goal)
	}
	if rtd.Metadata.Mode != "exploration" {
		t.Errorf("expected Mode=exploration, got %q", rtd.Metadata.Mode)
	}
}

func TestRuntimeTestDefinitionMetadata_SessionNameContainsGoal(t *testing.T) {
	adapter := &ExplorationTestAdapter{}
	intent := generator.TestIntent{
		Name:   "Test",
		Prompt: "Do something",
		Checks: []generator.Check{
			{Type: "no_error_messages"},
		},
	}
	rtd := adapter.Adapt(intent, "", 1, "prompt-001", "My exploration goal", "my-agent")

	cfg := &ExplorerConfig{
		Providers: []model.Provider{
			{Name: "test-llm", Type: "OPENAI", Model: "gpt-4o"},
		},
		Agents: []model.Agent{
			{Name: "my-agent", Provider: "test-llm"},
		},
		Explorer: ExplorerSettings{
			Goal:  "My exploration goal",
			Agent: "my-agent",
		},
	}
	testConfig := rtd.ToTestConfig(cfg)

	if len(testConfig.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(testConfig.Sessions))
	}
	if !strings.Contains(testConfig.Sessions[0].Name, "My exploration goal") {
		t.Errorf("session name should contain goal: %q", testConfig.Sessions[0].Name)
	}
}

// ============================================================================
// TestPromptRegistrySequential
// ============================================================================

func TestPromptRegistrySequential(t *testing.T) {
	r := NewPromptRegistry()

	id1 := r.Register(1, "prompt1", "reasoning1")
	id2 := r.Register(2, "prompt2", "reasoning2")
	id3 := r.Register(3, "prompt3", "reasoning3")

	if id1 != "prompt-001" {
		t.Errorf("expected prompt-001, got %q", id1)
	}
	if id2 != "prompt-002" {
		t.Errorf("expected prompt-002, got %q", id2)
	}
	if id3 != "prompt-003" {
		t.Errorf("expected prompt-003, got %q", id3)
	}

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	if all[0].Iteration != 1 || all[1].Iteration != 2 || all[2].Iteration != 3 {
		t.Error("entries should be in insertion order")
	}
}

// ============================================================================
// TestMetadataInjectedIntoMessages
// ============================================================================

func TestMetadataInjectedIntoMessages(t *testing.T) {
	msg := buildMetadataMessage(2, "prompt-002", "Explore FS", "the decision prompt text", "my reasoning")

	if msg.Role != "system" {
		t.Errorf("expected role=system, got %q", msg.Role)
	}
	if !strings.Contains(msg.Content, "EXPLORATION METADATA") {
		t.Error("content should contain EXPLORATION METADATA header")
	}
	if !strings.Contains(msg.Content, "Explore FS") {
		t.Error("content should contain goal")
	}
	if !strings.Contains(msg.Content, "prompt-002") {
		t.Error("content should contain promptID")
	}
	if !strings.Contains(msg.Content, "my reasoning") {
		t.Error("content should contain reasoning")
	}
	if msg.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

// ============================================================================
// Integration smoke: Adapt + ToTestConfig + name encoding
// ============================================================================

func TestSmokeAdaptToTestConfig(t *testing.T) {
	adapter := &ExplorationTestAdapter{}
	intent := generator.TestIntent{
		Name:   "Write file",
		Prompt: "Write hello to /tmp/smoke.txt",
		Checks: []generator.Check{
			{Type: "tool_called", Tool: "write_file"},
		},
	}
	rtd := adapter.Adapt(intent, "smoke test", 1, "prompt-001", "Smoke test goal", "fs-explorer")

	// Validate test name format.
	expectedPrefix := "[Iter 01 | prompt-001]"
	if !strings.HasPrefix(rtd.Name, expectedPrefix) {
		t.Errorf("expected name to start with %q, got %q", expectedPrefix, rtd.Name)
	}

	// Validate ToTestConfig produces sensible output.
	cfg := &ExplorerConfig{
		Providers: []model.Provider{
			{Name: "gemini-flash", Type: "GOOGLE", Model: "gemini-2.0-flash", Token: "tok"},
		},
		Agents: []model.Agent{
			{Name: "fs-explorer", Provider: "gemini-flash"},
		},
		Explorer: ExplorerSettings{
			Goal:  "Smoke test goal",
			Agent: "fs-explorer",
		},
	}
	tc := rtd.ToTestConfig(cfg)

	if len(tc.Providers) != 1 || tc.Providers[0].Name != "gemini-flash" {
		t.Errorf("provider not correctly set up: %+v", tc.Providers)
	}
	if len(tc.Agents) != 1 || tc.Agents[0].Name != "fs-explorer" {
		t.Errorf("agent not correctly set up: %+v", tc.Agents)
	}
	if len(tc.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(tc.Sessions))
	}
	if len(tc.Sessions[0].Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(tc.Sessions[0].Tests))
	}
}

// ============================================================================
// TestRunExplorationLoop_TokenLimit
// ============================================================================

// mockExplorerLLM is a test double for llms.Model used in explorer loop tests.
type mockExplorerLLM struct {
	responses []*llms.ContentResponse
	callIdx   int
}

func (m *mockExplorerLLM) Call(_ context.Context, _ string, _ ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *mockExplorerLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses configured")
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// validDecisionJSON returns a JSON string for a valid ExplorerDecision.
func validDecisionJSON() string {
	return `{
		"name": "Token limit test",
		"goal": "verify token limit",
		"session_name": "exploration",
		"prompt": "Do something simple",
		"checks": [{"type": "no_error_messages"}],
		"reasoning": "testing token limit"
	}`
}

func TestRunExplorationLoop_TokenLimitStopsAfterFirstIteration(t *testing.T) {
	// Each LLM call returns 100 tokens; MaxTokens is 50 → loop should stop after iter 1.
	decisionJSON := validDecisionJSON()
	mock := &mockExplorerLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{
				Content:        decisionJSON,
				GenerationInfo: map[string]any{"TotalTokens": 100},
			}}},
			{Choices: []*llms.ContentChoice{{
				Content:        decisionJSON,
				GenerationInfo: map[string]any{"TotalTokens": 100},
			}}},
		},
	}

	cfg := &ExplorerConfig{
		Providers: []model.Provider{{Name: "test-llm", Type: "OPENAI", Model: "gpt-4o"}},
		Agents:    []model.Agent{{Name: "test-agent", Provider: "test-llm"}},
		Explorer: ExplorerSettings{
			Goal:          "Test token limit behaviour",
			MaxIterations: 5,
			MaxRetries:    3,
			MaxTokens:     50, // less than 100 tokens used per decision call
		},
	}

	registry := NewPromptRegistry()
	adapter := &ExplorationTestAdapter{}

	// Pass empty agents/providers — RunTests returns empty results, which is fine for this test.
	_, rtds := runExplorationLoop(
		context.Background(),
		cfg,
		mock,
		map[string]*agent.MCPAgent{},
		map[string]llms.Model{},
		nil,
		registry,
		adapter,
		"test-agent",
		"test-config.yaml",
	)

	// Only 1 iteration should have completed (token limit hit after iter 1).
	if len(rtds) != 1 {
		t.Errorf("expected 1 completed iteration (token limit), got %d", len(rtds))
	}
}

func TestRunExplorationLoop_TokenLimitZeroRunsAllIterations(t *testing.T) {
	// MaxTokens==0 means unlimited; all 3 iterations should complete.
	decisionJSON := validDecisionJSON()
	responses := make([]*llms.ContentResponse, 3)
	for i := range responses {
		responses[i] = &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				Content:        decisionJSON,
				GenerationInfo: map[string]any{"TotalTokens": 999999},
			}},
		}
	}
	mock := &mockExplorerLLM{responses: responses}

	cfg := &ExplorerConfig{
		Providers: []model.Provider{{Name: "test-llm", Type: "OPENAI", Model: "gpt-4o"}},
		Agents:    []model.Agent{{Name: "test-agent", Provider: "test-llm"}},
		Explorer: ExplorerSettings{
			Goal:          "Test unlimited tokens",
			MaxIterations: 3,
			MaxRetries:    3,
			MaxTokens:     0, // unlimited
		},
	}

	registry := NewPromptRegistry()
	adapter := &ExplorationTestAdapter{}

	_, rtds := runExplorationLoop(
		context.Background(),
		cfg,
		mock,
		map[string]*agent.MCPAgent{},
		map[string]llms.Model{},
		nil,
		registry,
		adapter,
		"test-agent",
		"test-config.yaml",
	)

	if len(rtds) != 3 {
		t.Errorf("expected 3 completed iterations (unlimited tokens), got %d", len(rtds))
	}
}

// ============================================================================
// Helpers
// ============================================================================

// parseExplorerConfigFromBytes is a test-only helper that writes to a temp file
// and parses it with ParseExplorerConfig.
func parseExplorerConfigFromBytes(data []byte) (*ExplorerConfig, error) {
	f, err := os.CreateTemp("", "explorer-test-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	return ParseExplorerConfig(f.Name())
}

// Ensure time import is used.
var _ = time.Now

// ============================================================================
// TestAnalyzeToolCallWithLLM
// ============================================================================

func makeTC(text string) model.ToolCall {
	return model.ToolCall{
		Name:      "test_tool",
		Timestamp: time.Now(),
		Result: model.Result{
			Content: []model.ContentItem{{Type: "text", Text: text}},
		},
	}
}

func makeBugAnalysisResponse(isBug bool, bugType, explanation string) *llms.ContentResponse {
	j := fmt.Sprintf(`{"is_bug": %v, "bug_type": %q, "explanation": %q}`,
		isBug, bugType, explanation)
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: j}},
	}
}

func TestAnalyzeToolCallWithLLM_EmptyResponse_PrefligthNoCAll(t *testing.T) {
	// Empty content → pre-flight catches it without calling the LLM.
	tc := model.ToolCall{
		Name:      "test_tool",
		Timestamp: time.Now(),
		Result:    model.Result{},
	}
	// mock with no responses configured — if called it would error.
	mock := &mockExplorerLLM{}
	bugType, msg, isBug, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBug {
		t.Fatal("expected bug detected")
	}
	if bugType != BugTypeEmptyResponse {
		t.Fatalf("expected EMPTY_RESPONSE, got %s", bugType)
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if mock.callIdx != 0 {
		t.Fatal("LLM should not have been called for empty response")
	}
}

func TestAnalyzeToolCallWithLLM_LLMDetectsBug(t *testing.T) {
	tc := makeTC("panic: runtime error: index out of range")
	mock := &mockExplorerLLM{
		responses: []*llms.ContentResponse{
			makeBugAnalysisResponse(true, "STACKTRACE_RETURNED", "Response contains a Go panic stacktrace"),
		},
	}
	bugType, msg, isBug, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBug {
		t.Fatal("expected bug detected")
	}
	if bugType != BugTypeStacktraceReturned {
		t.Fatalf("expected STACKTRACE_RETURNED, got %s", bugType)
	}
	if msg == "" {
		t.Fatal("expected non-empty explanation")
	}
}

func TestAnalyzeToolCallWithLLM_LLMSaysNoBug(t *testing.T) {
	tc := makeTC("The result is 42")
	mock := &mockExplorerLLM{
		responses: []*llms.ContentResponse{
			makeBugAnalysisResponse(false, "", "Normal successful response"),
		},
	}
	_, _, isBug, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBug {
		t.Fatal("expected no bug for clean response")
	}
}

func TestAnalyzeToolCallWithLLM_LLMCallFails_ReturnsFalse(t *testing.T) {
	tc := makeTC("some response")
	// mock has no responses → GenerateContent returns error
	mock := &mockExplorerLLM{}
	_, _, isBug, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, nil)
	// fallback: no error returned to caller, no bug flagged
	if err != nil {
		t.Fatalf("expected nil error (fallback), got: %v", err)
	}
	if isBug {
		t.Fatal("expected no bug on LLM failure (fallback)")
	}
}

func TestAnalyzeToolCallWithLLM_MalformedLLMJSON_ReturnsFalse(t *testing.T) {
	tc := makeTC("some response")
	mock := &mockExplorerLLM{
		responses: []*llms.ContentResponse{
			{Choices: []*llms.ContentChoice{{Content: "not valid json at all"}}},
		},
	}
	_, _, isBug, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, nil)
	if err != nil {
		t.Fatalf("expected nil error (fallback), got: %v", err)
	}
	if isBug {
		t.Fatal("expected no bug when LLM returns malformed JSON (fallback)")
	}
}

func TestAnalyzeToolCallWithLLM_TokensAccumulated(t *testing.T) {
	tc := makeTC("something")
	mock := &mockExplorerLLM{
		responses: []*llms.ContentResponse{
			{
				Choices: []*llms.ContentChoice{{
					Content:        `{"is_bug": false, "bug_type": "", "explanation": "fine"}`,
					GenerationInfo: map[string]any{"TotalTokens": 42},
				}},
			},
		},
	}
	var tokens int
	_, _, _, err := AnalyzeToolCallWithLLM(context.Background(), mock, tc, &tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens != 42 {
		t.Errorf("expected 42 tokens accumulated, got %d", tokens)
	}
}

// ============================================================================
// TestBugFindings — model.BugFinding stored on ExecutionResult
// ============================================================================

func TestBugFindings_StoredOnExecutionResult(t *testing.T) {
	findings := []model.BugFinding{
		{ToolName: "tool_a", BugType: string(BugTypeEmptyResponse), Explanation: "empty", ServerName: "srv1"},
		{ToolName: "tool_b", BugType: string(BugTypeTimeout), Explanation: "timeout", ServerName: "srv1"},
		{ToolName: "tool_a", BugType: string(BugTypeMalformedJSON), Explanation: "bad json", ServerName: "srv2"},
	}

	result := model.ExecutionResult{BugFindings: findings}

	if len(result.BugFindings) != 3 {
		t.Fatalf("expected 3 bug findings, got %d", len(result.BugFindings))
	}
	if result.BugFindings[0].ToolName != "tool_a" {
		t.Errorf("unexpected tool name: %s", result.BugFindings[0].ToolName)
	}
	if result.BugFindings[1].BugType != string(BugTypeTimeout) {
		t.Errorf("unexpected bug type: %s", result.BugFindings[1].BugType)
	}
	if result.BugFindings[2].ServerName != "srv2" {
		t.Errorf("unexpected server name: %s", result.BugFindings[2].ServerName)
	}
}
