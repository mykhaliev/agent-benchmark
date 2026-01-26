//go:build ignore
// +build ignore

// This script generates sample HTML reports for manual inspection.
// Run with: go run test/generate_reports.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/report"
)

var baseTime = time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)

// ============================================================================
// REUSABLE TEST RUN BUILDERS
// ============================================================================

// Agent configurations
type AgentConfig struct {
	Name     string
	Provider model.ProviderType
}

var (
	agentGemini  = AgentConfig{"gemini-agent", model.ProviderGoogle}
	agentClaude  = AgentConfig{"claude-agent", model.ProviderAnthropic}
	agentGPT     = AgentConfig{"gpt-agent", model.ProviderOpenAI}
	agentPhoenix = AgentConfig{"phoenix-agent", model.ProviderAzure}
	// Windows MCP test agents (based on real test data)
	agentGPT41 = AgentConfig{"gpt41-agent", model.ProviderAzure}
	agentGPT52 = AgentConfig{"gpt52-agent", model.ProviderAzure}
)

// buildTestRun creates a test run with the given parameters
func buildTestRun(testName string, agent AgentConfig, passed bool, opts TestRunOpts) model.TestRun {
	startTime := opts.StartTime
	if startTime.IsZero() {
		startTime = baseTime
	}
	endTime := startTime.Add(time.Duration(opts.DurationMs) * time.Millisecond)

	return model.TestRun{
		Execution: &model.ExecutionResult{
			TestName:           testName,
			AgentName:          agent.Name,
			ProviderType:       agent.Provider,
			SessionName:        opts.SessionName,
			SourceFile:         opts.SourceFile,
			StartTime:          startTime,
			EndTime:            endTime,
			Messages:           opts.Messages,
			ToolCalls:          opts.ToolCalls,
			FinalOutput:        opts.FinalOutput,
			LatencyMs:          int64(opts.DurationMs),
			TokensUsed:         opts.TokensUsed,
			Errors:             opts.Errors,
			RateLimitStats:     opts.RateLimitStats,
			ClarificationStats: opts.ClarificationStats,
		},
		Assertions: opts.Assertions,
		Passed:     passed,
	}
}

type TestRunOpts struct {
	StartTime          time.Time
	DurationMs         int
	SessionName        string
	SourceFile         string
	Messages           []model.Message
	ToolCalls          []model.ToolCall
	FinalOutput        string
	TokensUsed         int
	Errors             []string
	Assertions         []model.AssertionResult
	RateLimitStats     *model.RateLimitStats
	ClarificationStats *model.ClarificationStats
}

// Common tool calls
func toolWriteFile(offset time.Duration, durationMs int64) model.ToolCall {
	return model.ToolCall{
		Name:       "write_file",
		Parameters: map[string]interface{}{"path": "/tmp/test.txt", "content": "Hello World"},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"path":"/tmp/test.txt","bytes":11}`,
			}},
		},
	}
}

func toolReadFile(offset time.Duration, durationMs int64) model.ToolCall {
	return model.ToolCall{
		Name:       "read_file",
		Parameters: map[string]interface{}{"path": "/etc/config.yaml"},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"content":"server:\n  port: 8080\n  host: localhost"}`,
			}},
		},
	}
}

func toolBashCommand(offset time.Duration, durationMs int64, cmd string) model.ToolCall {
	return model.ToolCall{
		Name:       "run_bash",
		Parameters: map[string]interface{}{"command": cmd},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"exitCode":0,"stdout":"command completed"}`,
			}},
		},
	}
}

func toolPythonExec(offset time.Duration, durationMs int64, code string) model.ToolCall {
	return model.ToolCall{
		Name:       "execute_python",
		Parameters: map[string]interface{}{"code": code},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"output":"True","exitCode":0}`,
			}},
		},
	}
}

func toolDBConnect(offset time.Duration, durationMs int64, host string, port int) model.ToolCall {
	return model.ToolCall{
		Name:       "db_connect",
		Parameters: map[string]interface{}{"host": host, "port": port},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"connection":{"host":"%s","port":%d,"status":"connected"}}`, host, port),
			}},
		},
	}
}

func toolDBQuery(offset time.Duration, durationMs int64, sql string) model.ToolCall {
	return model.ToolCall{
		Name:       "db_query",
		Parameters: map[string]interface{}{"sql": sql},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"rows":[{"id":1,"name":"test"}],"rowCount":1}`,
			}},
		},
	}
}

func toolHTTPGet(offset time.Duration, durationMs int64, url string) model.ToolCall {
	return model.ToolCall{
		Name:       "http_get",
		Parameters: map[string]interface{}{"url": url},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"status":200,"body":"{\"message\":\"success\"}"}`,
			}},
		},
	}
}

func toolDeleteFile(offset time.Duration, durationMs int64, path string) model.ToolCall {
	return model.ToolCall{
		Name:       "delete_file",
		Parameters: map[string]interface{}{"path": path},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"deleted":"%s"}`, path),
			}},
		},
	}
}

func toolListFiles(offset time.Duration, durationMs int64, path string) model.ToolCall {
	return model.ToolCall{
		Name:       "list_files",
		Parameters: map[string]interface{}{"path": path},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"path":"%s","files":["file1.txt","file2.txt"]}`, path),
			}},
		},
	}
}

func toolGeneric(offset time.Duration, durationMs int64, name string, params map[string]interface{}) model.ToolCall {
	return model.ToolCall{
		Name:       name,
		Parameters: params,
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"tool":"%s","completed":true}`, name),
			}},
		},
	}
}

func toolGenericWithResult(offset time.Duration, durationMs int64, name string, params map[string]interface{}, resultJSON string) model.ToolCall {
	return model.ToolCall{
		Name:       name,
		Parameters: params,
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: resultJSON,
			}},
		},
	}
}

// Assertion for failed tool call expectation
func assertToolCalledFailed(expected, actual string) model.AssertionResult {
	return model.AssertionResult{Type: "tool_called", Passed: false, Message: fmt.Sprintf("Expected '%s' but got '%s'", expected, actual)}
}

func assertNoErrors() model.AssertionResult {
	return model.AssertionResult{Type: "no_error_messages", Passed: true, Message: "No errors occurred"}
}

func assertNoErrorsFailed(msg string) model.AssertionResult {
	return model.AssertionResult{Type: "no_error_messages", Passed: false, Message: msg}
}

func assertParamEquals(passed bool, msg string, details map[string]interface{}) model.AssertionResult {
	return model.AssertionResult{Type: "tool_param_equals", Passed: passed, Message: msg, Details: details}
}

// Common assertions
func assertToolCalled(tool string) model.AssertionResult {
	return model.AssertionResult{Type: "tool_called", Passed: true, Message: fmt.Sprintf("Tool '%s' was called", tool)}
}

func assertOutputContains(text string) model.AssertionResult {
	return model.AssertionResult{Type: "output_contains", Passed: true, Message: fmt.Sprintf("Output contains '%s'", text)}
}

func assertOutputContainsFailed(text string) model.AssertionResult {
	return model.AssertionResult{Type: "output_contains", Passed: false, Message: fmt.Sprintf("Output should contain '%s'", text)}
}

func assertMaxToolCalls(used, max int) model.AssertionResult {
	return model.AssertionResult{Type: "max_tool_calls", Passed: used <= max, Message: fmt.Sprintf("Used %d tool calls (max: %d)", used, max)}
}

// ============================================================================
// WINDOWS MCP TOOL HELPERS (realistic tool patterns from actual tests)
// ============================================================================

func toolWindowsApp(offset time.Duration, durationMs int64, program string, handle string) model.ToolCall {
	return model.ToolCall{
		Name:       "app",
		Parameters: map[string]interface{}{"programPath": program},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"w":{"h":"%s","t":"Untitled - Notepad","cn":"Notepad","pn":"Notepad","pid":12345,"b":[5,186,1654,870],"s":"normal","mi":1,"fg":true},"msg":"Launched '%s'. Window is focused and ready. Use this handle for all subsequent operations."}`, handle, program),
			}},
		},
	}
}

func toolWindowsUIType(offset time.Duration, durationMs int64, handle string, text string) model.ToolCall {
	return model.ToolCall{
		Name:       "ui_type",
		Parameters: map[string]interface{}{"windowHandle": handle, "controlType": "Document", "text": text},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"a":"type","items":[{"id":"2","n":"Text editor","t":"Document","c":[832,642,1],"e":true}],"n":1,"hint":"Use elementId='2' for subsequent actions.","diag":{"durationMs":147}}`,
			}},
		},
	}
}

func toolWindowsKeyboardType(offset time.Duration, durationMs int64, handle string, text string) model.ToolCall {
	return model.ToolCall{
		Name:       "keyboard_control",
		Parameters: map[string]interface{}{"action": "type", "windowHandle": handle, "text": text},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"cnt":%d,"tw":{"h":"%s","t":"*Hello - Notepad","pn":"Notepad","pid":12345}}`, len(text), handle),
			}},
		},
	}
}

func toolWindowsClose(offset time.Duration, durationMs int64, handle string, discard bool) model.ToolCall {
	params := map[string]interface{}{"action": "close", "handle": handle}
	if discard {
		params["discardChanges"] = true
	}
	return model.ToolCall{
		Name:       "window_management",
		Parameters: params,
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"w":{"h":"%s","t":"*Hello - Notepad","cn":"Notepad","pn":"Notepad","pid":12345,"b":[5,186,1654,870],"s":"normal","mi":1,"fg":true}}`, handle),
			}},
		},
	}
}

func toolWindowsList(offset time.Duration, durationMs int64, filter string, count int) model.ToolCall {
	return model.ToolCall{
		Name:       "window_management",
		Parameters: map[string]interface{}{"action": "list", "filter": filter},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"ws":[],"n":%d}`, count),
			}},
		},
	}
}

func toolWindowsUIRead(offset time.Duration, durationMs int64, handle string, text string) model.ToolCall {
	return model.ToolCall{
		Name:       "ui_read",
		Parameters: map[string]interface{}{"windowHandle": handle, "controlType": "Document"},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: fmt.Sprintf(`{"ok":true,"a":"get_text","txt":"%s","diag":{"durationMs":1}}`, text),
			}},
		},
	}
}

func toolWindowsScreenshot(offset time.Duration, durationMs int64, handle string, annotate bool) model.ToolCall {
	return model.ToolCall{
		Name:       "screenshot_control",
		Parameters: map[string]interface{}{"action": "capture", "target": "window", "windowHandle": handle, "annotate": annotate},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
		Result: model.Result{
			Content: []model.ContentItem{{
				Type: "text",
				Text: `{"ok":true,"ec":"success","msg":"Captured 1280x720 jpeg with 25 annotated elements","w":1280,"h":720,"ow":2560,"oh":1440,"fmt":"jpeg","n":25}`,
			}},
		},
	}
}

// Realistic Windows MCP assertion helpers
func assertAnyOf(passed bool, passedCount, totalCount int, children []model.AssertionResult) model.AssertionResult {
	return model.AssertionResult{
		Type:    "anyOf",
		Passed:  passed,
		Message: fmt.Sprintf("anyOf passed: %d of %d assertions passed", passedCount, totalCount),
		Details: map[string]interface{}{"children": children, "passed_count": passedCount, "failed_count": totalCount - passedCount},
	}
}

func assertToolCallOrder(passed bool, tools []string) model.AssertionResult {
	msg := fmt.Sprintf("Tools called in correct order: %v", tools)
	if !passed {
		msg = fmt.Sprintf("Tool order mismatch: expected %v", tools)
	}
	return model.AssertionResult{Type: "tool_call_order", Passed: passed, Message: msg}
}

func assertNoHallucinatedTools() model.AssertionResult {
	return model.AssertionResult{Type: "no_hallucinated_tools", Passed: true, Message: "No hallucinated tools"}
}

func assertNoClarificationQuestions() model.AssertionResult {
	return model.AssertionResult{Type: "no_clarification_questions", Passed: true, Message: "No clarification questions detected"}
}

func assertNoRateLimitErrors() model.AssertionResult {
	return model.AssertionResult{Type: "no_rate_limit_errors", Passed: true, Message: "No rate limit errors (HTTP 429)"}
}

func assertOutputRegex(passed bool, pattern string) model.AssertionResult {
	return model.AssertionResult{
		Type:    "output_regex",
		Passed:  passed,
		Message: fmt.Sprintf("Output matches regex '%s': %v", pattern, passed),
	}
}

func main() {
	gen, err := report.NewGenerator()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create generator: %v\n", err)
		os.Exit(1)
	}

	reporter := model.NewReportGenerator()

	outDir := "generated_reports"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// =====================================================================
	// SINGLE-AGENT ANALYSIS FIXTURES (01-04)
	// These use "fit for purpose" format - evaluating if the agent
	// accomplished the task appropriately, not comparing to others.
	// =====================================================================

	analysis01 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Assessment: PASS - Task completed successfully**

claude-agent correctly identified and used the write_file tool to create the requested configuration file. The approach was direct and efficient.

**Tool Usage Analysis**
- write_file called once with correct path parameter
- Content matches expected output exactly
- No unnecessary tool calls or retries

**Strengths**
- Efficient tool selection - went directly to filesystem operation
- Correct parameter formatting for the tool

**Considerations**
- Latency of 3.5s is acceptable for this simple file creation task`,
	}

	analysis02 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Assessment: PASS - Both tests completed successfully**

claude-agent handled the multi-test scenario well, correctly using different tools for different tasks.

**Tool Usage Patterns**
- Test 1 (file creation): Used write_file with appropriate parameters
- Test 2 (config reading): Used read_file to retrieve configuration

**Approach Quality**
- Tools selected match the task requirements
- No redundant operations between tests
- Session maintained appropriate state

**Efficiency Notes**
- Total execution time reasonable at ~8s for two operations
- Each tool call succeeded on first attempt`,
	}

	analysis03 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Assessment: PASS - Multi-session scenario handled correctly**

claude-agent maintained consistency across multiple sessions while adapting tool selection to each task.

**Session Analysis**
- Session 1: File operations using write_file and read_file
- Session 2: Database queries using db_connect and db_query
- Session 3: HTTP operations using http_get

**Tool Usage Patterns**
- Correctly switched tool families based on task domain
- Maintained context within sessions
- No cross-session state confusion

**Notable Behavior**
- Database connection established before query (correct ordering)
- HTTP call included proper URL formatting`,
	}

	analysis04 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Assessment: PASS - Multi-file test suite completed**

claude-agent demonstrated good versatility across different test files with varying requirements.

**Test File Analysis**
- basic.yaml: Simple file operations (PASSED)
- advanced.yaml: Database interaction (PASSED)
- edge-cases.yaml: Error handling scenarios (PASSED)

**Tool Selection Quality**
- Appropriate tools chosen for each test file's domain
- No tool misuse or incorrect parameter patterns
- Consistent behavior across different test contexts

**Performance Summary**
- Average latency: 4.2s per test
- No timeouts or retries needed
- Clean execution path through all files`,
	}

	// =====================================================================
	// MULTI-AGENT ANALYSIS FIXTURES (05-09)
	// These use comparison format - helping users choose between agents.
	// =====================================================================

	analysis05 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Verdict: gpt41-agent recommended for Notepad automation**

Testing Notepad automation across 3 agents revealed different approaches to text input and window management.

**Agent Comparison**

gpt41-agent (PASSED): Used semantic UI automation (ui_type) for text input. Clean approach that targets the Document control directly. Latency: 12.0s.

gpt52-agent (PASSED): Used raw keyboard input (keyboard_control). Faster but less semantic. Latency: 14.7s.

gemini-agent (FAILED): Task incomplete - took screenshot instead of closing Notepad. Window left open.

**Tool Usage Patterns**
- gpt41-agent: app → ui_type → ui_read → window_management(close) → window_management(list)
- gpt52-agent: app → keyboard_control → window_management(close) → window_management(list)
- gemini-agent: app → ui_type → screenshot_control (WRONG - should have closed window)

**Key Differentiation**
1. gpt41-agent used ui_type with controlType="Document" to target the text editor semantically
2. gpt52-agent used keyboard_control for raw text input - works but less reliable
3. gemini-agent failed to complete the task - verified the text but didn't close the window

**When to Choose Each**
- gpt41-agent: Best for UI automation requiring semantic element targeting
- gpt52-agent: Good when raw keyboard input is acceptable
- gemini-agent: Not recommended for window lifecycle management tasks`,
	}

	analysis06 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Verdict: gemini-agent recommended for speed; claude-agent for reliability**

Testing across 4 agents with 8 total tests showed diverse tool selection strategies.

**Agent Comparison**

claude-agent (100%): Used native filesystem tools (write_file, read_file). Slower but reliable approach with consistent results.

gemini-agent (100%): Leveraged bash commands for file operations. Fastest execution at 6.0s total.

gpt-agent (100%): Used Python execution for file handling. Middle ground in performance.

phoenix-agent (0%): FAILED - Used HTTP/GraphQL APIs instead of filesystem tools. Fundamentally wrong approach.

**Tool Usage Patterns**
- claude-agent: write_file, read_file (native tools)
- gemini-agent: run_bash with cat/echo commands
- gpt-agent: execute_python with open()/write()
- phoenix-agent: http_get, graphql_query (wrong tool family)

**Critical Finding**
phoenix-agent consistently chose network APIs for local file operations. This suggests poor tool selection logic for filesystem tasks.

**When to Choose Each**
- gemini-agent: When speed matters and bash is available
- claude-agent: When explicit tool tracing is needed
- gpt-agent: When Python environment is preferred
- phoenix-agent: Not recommended for filesystem tasks`,
	}

	analysis07 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Verdict: claude-agent or gemini-agent recommended**

Multi-session testing across 3 agents (9 tests total) tested session isolation and state management.

**Agent Comparison**

claude-agent (100%): Perfect performance across all sessions. Properly isolated state between session boundaries. Average latency 4.0s.

gemini-agent (100%): Consistent results with good session handling. Slightly slower at 4.5s average.

gpt-agent (67%): One failure in config reading session. Struggled with YAML parsing in session 2.

**Tool Usage Patterns**
- Session 1 (file ops): All agents used write_file correctly
- Session 2 (config): gpt-agent misinterpreted YAML structure
- Session 3 (validation): claude and gemini used read_file with proper content matching

**Session State Analysis**
- All agents properly isolated context between sessions
- No state bleeding detected
- gpt-agent's failure was tool usage, not state management

**When to Choose Each**
- claude-agent: First choice for multi-session workflows
- gemini-agent: Reliable alternative with similar capabilities
- gpt-agent: Avoid for configuration/YAML parsing tasks`,
	}

	analysis08 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Verdict: claude-agent is the clear winner for complex multi-file suites**

Full complexity test: 20 tests across 4 agents, multiple files and sessions.

**Agent Comparison**

claude-agent (100%): Perfect score across all test files. Efficient tool chains with an average of 2.1 tools per test. Handled errors gracefully with proper recovery.

gemini-agent (90%): One timeout on large file processing in advanced.yaml. Good overall but struggled with 50KB+ file operations.

gpt-agent (80%): Struggled with complex multi-step prompts. Used more tool calls than necessary (avg 3.8 per test).

phoenix-agent (70%): Inconsistent tool selection. Sometimes chose http_get when write_file was appropriate.

**Tool Usage Patterns**
- claude-agent: Minimal tool chains, direct approaches
- gemini-agent: bash-heavy approach, efficient but timeout-prone
- gpt-agent: Python wrappers added overhead
- phoenix-agent: Tool family confusion (network vs filesystem)

**Performance Insights**
- File size correlation: Larger files (50KB+) caused issues for gemini
- Tool chain length: claude's shorter chains = faster completion
- Error recovery: Only claude attempted alternative tools on failure

**When to Choose Each**
- claude-agent: Complex workflows, error-prone environments
- gemini-agent: Simple file ops, small files only
- gpt-agent: Python-native environments
- phoenix-agent: Not recommended without retraining`,
	}

	analysis09 := &agent.AISummaryResult{
		Success: true,
		Analysis: `**Overall Assessment: FAILED - Permission denied prevented task completion**

Single-agent failure case demonstrating error handling behavior.

**Failure Analysis**

The agent encountered a permission denied error when attempting to write to /tmp/test.txt. The agent did not attempt recovery strategies.

**Tool Usage Patterns**
- write_file called with path: /tmp/test.txt
- Parameters were correct but execution failed
- No fallback tools attempted (could have tried run_bash with sudo or alternative path)

**Error Chain**
1. write_file returned permission denied
2. Agent reported failure without retrying
3. Both assertions failed: tool_called (no successful call) and output_contains (no output generated)

**Root Cause**
Test environment configuration issue - /tmp directory lacked write permissions for the test user.

**Improvement Suggestions**
- For test authors: Verify environment permissions before test runs
- For agent: Consider adding retry logic with alternative approaches
- For assertions: Add specific error type assertions for better diagnostics`,
	}

	// Hierarchical test reports - progressive complexity:
	// Level 1: Single agent, single test (simplest case)
	// Level 2: Single agent, multiple tests (adds test variety)
	// Level 3: Single agent, multiple sessions (adds session grouping)
	// Level 4: Single agent, multiple files (adds file grouping)
	// Level 5: Multiple agents, single test (adds agent comparison)
	// Level 6: Multiple agents, multiple tests (full matrix)
	// Level 7: Multiple agents, multiple sessions (session grouping across agents)
	// Level 8: Multiple agents, multiple files (full complexity)
	// Bonus: Failed test with errors
	fixtures := []struct {
		name     string
		level    int
		results  []model.TestRun
		analysis *agent.AISummaryResult // nil means no analysis
	}{
		{"01_single_agent_single_test", 1, createSingleAgentOneTest(), analysis01},
		{"02_single_agent_multi_test", 2, createSingleAgentTwoTests(), analysis02},
		{"03_single_agent_multi_session", 3, createSingleAgentMultiSession(), analysis03},
		{"04_single_agent_multi_file", 4, createSingleAgentMultiFile(), analysis04},
		{"05_multi_agent_single_test", 5, createMultiAgentSingleTest(), analysis05},
		{"06_multi_agent_multi_test", 6, createMultiAgent(), analysis06},
		{"07_multi_agent_multi_session", 7, createMultiAgentMultiSession(), analysis07},
		{"08_multi_agent_multi_file", 8, createFullSuite(), analysis08},
		{"09_failed_with_errors", 0, createFailedTest(), analysis09},
	}

	fmt.Println("Generating hierarchical test reports...")
	fmt.Println("=========================================")
	for _, f := range fixtures {
		// Generate HTML report
		var html string
		var err error
		if f.analysis != nil {
			html, err = gen.GenerateHTMLWithAnalysis(f.results, f.analysis)
		} else {
			html, err = gen.GenerateHTML(f.results)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate HTML for %s: %v\n", f.name, err)
			continue
		}

		htmlPath := filepath.Join(outDir, f.name+".html")
		if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", htmlPath, err)
			continue
		}

		// Generate JSON report
		var jsonContent string
		if f.analysis != nil {
			analysisData := &model.AISummaryData{
				Success:  f.analysis.Success,
				Analysis: f.analysis.Analysis,
			}
			jsonContent = reporter.GenerateJSONReportWithAnalysis(f.results, analysisData)
		} else {
			jsonContent = reporter.GenerateJSONReport(f.results)
		}

		jsonPath := filepath.Join(outDir, f.name+".json")
		if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", jsonPath, err)
			continue
		}

		levelStr := ""
		if f.level > 0 {
			levelStr = fmt.Sprintf(" (Level %d)", f.level)
		}
		analysisStr := ""
		if f.analysis != nil {
			analysisStr = " [+analysis]"
		}
		fmt.Printf("✓ %s%s%s - HTML: %d bytes, JSON: %d bytes\n", f.name, levelStr, analysisStr, len(html), len(jsonContent))
	}
	fmt.Println("=========================================")
	fmt.Println("Done! Open generated_reports/*.html to view.")
	fmt.Println("JSON reports also generated in generated_reports/*.json")
}

func createSingleAgentOneTest() []model.TestRun {
	return []model.TestRun{
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs:  2340,
			Messages:    []model.Message{{Role: "user", Content: "Create a test file at /tmp/test.txt with the content 'Hello World'"}},
			ToolCalls:   []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
			FinalOutput: "[Iteration 1: 1 tool(s) to execute]\nDone!",
			TokensUsed:  325,
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
	}
}

func createSingleAgentTwoTests() []model.TestRun {
	return []model.TestRun{
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs:  2340,
			Messages:    []model.Message{{Role: "user", Content: "Create a test file at /tmp/test.txt with the content 'Hello World'"}},
			ToolCalls:   []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
			FinalOutput: "File created successfully at /tmp/test.txt",
			TokensUsed:  325,
			Assertions: []model.AssertionResult{
				assertToolCalled("write_file"),
				assertOutputContains("created"),
			},
		}),
		buildTestRun("Read configuration", agentGemini, true, TestRunOpts{
			StartTime:  baseTime.Add(3 * time.Second),
			DurationMs: 2500,
			Messages: []model.Message{
				{Role: "user", Content: "Read the configuration file at /etc/config.yaml and tell me the port number"},
				{Role: "assistant", Content: "I'll read the config file for you."},
			},
			ToolCalls:   []model.ToolCall{toolReadFile(3500*time.Millisecond, 82)},
			FinalOutput: "[Iteration 1: 1 tool(s) to execute]\nThe port number in the configuration is 8080.",
			TokensUsed:  198,
			Assertions: []model.AssertionResult{
				assertToolCalled("read_file"),
				assertOutputContains("8080"),
				assertMaxToolCalls(1, 3),
			},
		}),
	}
}

func createMultiAgent() []model.TestRun {
	createFilePrompt := "Create a test file at /tmp/test.txt with the content 'Hello World'"
	readConfigPrompt := "Read the configuration file at /etc/config.yaml and tell me the port number"

	// Claude uses filesystem MCP tools
	claudeFileTools := []model.ToolCall{
		toolGeneric(1*time.Second, 45, "stat", map[string]interface{}{"path": "/tmp"}),
		toolGeneric(2500*time.Millisecond, 82, "mkdir", map[string]interface{}{"path": "/tmp", "recursive": true}),
		toolGeneric(4500*time.Millisecond, 156, "write_file", map[string]interface{}{"path": "/tmp/test.txt", "content": "Hello World"}),
		toolGeneric(6*time.Second, 38, "cat", map[string]interface{}{"path": "/tmp/test.txt"}),
		toolGeneric(7*time.Second, 52, "ls", map[string]interface{}{"path": "/tmp", "long": true}),
	}

	// Gemini uses bash commands
	geminiBashTools := []model.ToolCall{
		toolGeneric(1*time.Second, 25, "bash", map[string]interface{}{"command": "test -d /tmp"}),
		toolGeneric(3*time.Second, 68, "bash", map[string]interface{}{"command": "echo 'Hello World' > /tmp/test.txt"}),
		toolGeneric(4500*time.Millisecond, 32, "bash", map[string]interface{}{"command": "cat /tmp/test.txt"}),
		toolGeneric(5500*time.Millisecond, 41, "bash", map[string]interface{}{"command": "stat /tmp/test.txt"}),
		toolGeneric(6*time.Second, 35, "bash", map[string]interface{}{"command": "ls -la /tmp/test.txt"}),
	}

	// GPT uses Python code execution
	gptPythonTools := []model.ToolCall{
		toolGeneric(1*time.Second, 185, "python", map[string]interface{}{"code": "import os; os.path.exists('/tmp')"}),
		toolGeneric(2500*time.Millisecond, 142, "python", map[string]interface{}{"code": "os.makedirs('/tmp', exist_ok=True)"}),
		toolGeneric(4500*time.Millisecond, 198, "python", map[string]interface{}{"code": "open('/tmp/test.txt', 'w').write('hello world')"}),
		toolGeneric(5*time.Second, 95, "python", map[string]interface{}{"code": "open('/tmp/test.txt').read()"}),
		toolGeneric(6*time.Second, 78, "python", map[string]interface{}{"code": "os.listdir('/tmp')"}),
	}

	// Phoenix (disqualified) uses random wrong API calls
	phoenixWrongTools := []model.ToolCall{
		toolGeneric(2*time.Second, 2500, "http_post", map[string]interface{}{"url": "http://files.api/create"}),
		toolGeneric(5*time.Second, 3200, "graphql", map[string]interface{}{"query": "mutation { createFile }"}),
		toolGeneric(9*time.Second, 1800, "database_insert", map[string]interface{}{"table": "files", "data": "{}"}),
		toolGeneric(11*time.Second, 950, "send_email", map[string]interface{}{"to": "admin", "subject": "help"}),
		toolGeneric(13*time.Second, 1250, "web_search", map[string]interface{}{"query": "how to create file"}),
	}

	return []model.TestRun{
		// Test 1: Create test file - each agent uses DIFFERENT tools/approaches
		buildTestRun("Create test file", agentClaude, true, TestRunOpts{
			DurationMs: 8000,
			Messages: []model.Message{
				{Role: "user", Content: createFilePrompt},
				{Role: "assistant", Content: "I'll create the file for you.", Timestamp: baseTime.Add(500 * time.Millisecond)},
				{Role: "assistant", Content: "Checking path exists.", Timestamp: baseTime.Add(2 * time.Second)},
				{Role: "assistant", Content: "Writing content.", Timestamp: baseTime.Add(4 * time.Second)},
				{Role: "assistant", Content: "Verifying file.", Timestamp: baseTime.Add(6 * time.Second)},
				{Role: "assistant", Content: "Done!", Timestamp: baseTime.Add(7500 * time.Millisecond)},
			},
			ToolCalls:   claudeFileTools,
			FinalOutput: "[Iteration 1: 2 tool(s) to execute]\n[Iteration 2: 2 tool(s) to execute]\n[Iteration 3: 1 tool(s) to execute]\nFile created successfully at /tmp/test.txt with content 'Hello World'",
			TokensUsed:  325,
			Assertions: []model.AssertionResult{
				assertToolCalled("write_file"),
				assertParamEquals(true, "Parameters match", nil),
			},
		}),
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs: 6500,
			Messages: []model.Message{
				{Role: "user", Content: createFilePrompt},
				{Role: "assistant", Content: "Creating file now.", Timestamp: baseTime.Add(400 * time.Millisecond)},
				{Role: "assistant", Content: "Using bash to create.", Timestamp: baseTime.Add(2 * time.Second)},
				{Role: "assistant", Content: "Verifying creation.", Timestamp: baseTime.Add(4 * time.Second)},
				{Role: "assistant", Content: "Checking permissions.", Timestamp: baseTime.Add(5 * time.Second)},
				{Role: "assistant", Content: "Complete.", Timestamp: baseTime.Add(6 * time.Second)},
			},
			ToolCalls:   geminiBashTools,
			FinalOutput: "[Iteration 1: 1 tool(s) to execute]\n[Iteration 2: 2 tool(s) to execute]\n[Iteration 3: 2 tool(s) to execute]\nDone! File created using bash.",
			TokensUsed:  256,
			Assertions: []model.AssertionResult{
				assertToolCalled("bash"),
				assertOutputContains("file creation"),
			},
		}),
		buildTestRun("Create test file", agentGPT, false, TestRunOpts{
			DurationMs: 7000,
			Messages: []model.Message{
				{Role: "user", Content: createFilePrompt},
				{Role: "assistant", Content: "Setting up file.", Timestamp: baseTime.Add(500 * time.Millisecond)},
				{Role: "assistant", Content: "Using Python for reliability.", Timestamp: baseTime.Add(2 * time.Second)},
				{Role: "assistant", Content: "Writing to disk.", Timestamp: baseTime.Add(4 * time.Second)},
				{Role: "assistant", Content: "Checking result.", Timestamp: baseTime.Add(5500 * time.Millisecond)},
				{Role: "assistant", Content: "File ready.", Timestamp: baseTime.Add(6500 * time.Millisecond)},
			},
			ToolCalls:   gptPythonTools,
			FinalOutput: "[Iteration 1: 2 tool(s) to execute]\n[Iteration 2: 3 tool(s) to execute]\nFile created via Python.",
			TokensUsed:  234,
			Assertions: []model.AssertionResult{
				assertToolCalled("python"),
				assertParamEquals(false, "Parameter mismatch", map[string]interface{}{"expected": "Hello World", "actual": "hello world"}),
			},
		}),
		// DISQUALIFIED: phoenix-agent fails all tests
		buildTestRun("Create test file", agentPhoenix, false, TestRunOpts{
			DurationMs: 15000,
			Messages: []model.Message{
				{Role: "user", Content: createFilePrompt},
				{Role: "assistant", Content: "I will try to create the file.", Timestamp: baseTime.Add(1 * time.Second)},
				{Role: "assistant", Content: "Error encountered.", Timestamp: baseTime.Add(4 * time.Second)},
				{Role: "assistant", Content: "Retrying...", Timestamp: baseTime.Add(8 * time.Second)},
				{Role: "assistant", Content: "Still failing.", Timestamp: baseTime.Add(12 * time.Second)},
				{Role: "assistant", Content: "Cannot complete task.", Timestamp: baseTime.Add(14 * time.Second)},
			},
			ToolCalls:   phoenixWrongTools,
			FinalOutput: "[Iteration 1: 1 tool(s) to execute]\n[Iteration 2: 1 tool(s) to execute]\n[Iteration 3: 3 tool(s) to execute]\nFailed to create file. I don't have the right tools.",
			TokensUsed:  567,
			Errors:      []string{"HTTP POST failed: connection refused", "GraphQL error: unknown mutation", "Database error: table not found"},
			Assertions: []model.AssertionResult{
				assertToolCalledFailed("write_file", "API calls"),
				assertParamEquals(false, "Path mismatch", map[string]interface{}{"expected": "/tmp/test.txt", "actual": "http://files.api/create"}),
				assertNoErrorsFailed("Multiple errors encountered"),
			},
		}),
		// Test 2: Read config - each agent consistent with their Test 1 approach
		// Claude: Uses filesystem MCP tools
		buildTestRun("Read config", agentClaude, true, TestRunOpts{
			StartTime:  baseTime.Add(20 * time.Second),
			DurationMs: 8000,
			Messages: []model.Message{
				{Role: "user", Content: readConfigPrompt},
				{Role: "assistant", Content: "Reading config file.", Timestamp: baseTime.Add(21 * time.Second)},
				{Role: "assistant", Content: "File found.", Timestamp: baseTime.Add(23 * time.Second)},
				{Role: "assistant", Content: "Parsing YAML.", Timestamp: baseTime.Add(25 * time.Second)},
				{Role: "assistant", Content: "Extracting port.", Timestamp: baseTime.Add(26 * time.Second)},
				{Role: "assistant", Content: "Port identified.", Timestamp: baseTime.Add(27 * time.Second)},
			},
			ToolCalls: []model.ToolCall{
				toolGeneric(21500*time.Millisecond, 38, "stat", map[string]interface{}{"path": "/etc/config.yaml"}),
				toolGeneric(23*time.Second, 52, "cat", map[string]interface{}{"path": "/etc/config.yaml"}),
				toolGeneric(24*time.Second, 28, "grep", map[string]interface{}{"pattern": "port", "file": "/etc/config.yaml"}),
				toolGeneric(25500*time.Millisecond, 35, "head", map[string]interface{}{"path": "/etc/config.yaml", "lines": 10}),
				toolGeneric(26500*time.Millisecond, 22, "wc", map[string]interface{}{"path": "/etc/config.yaml", "flag": "-l"}),
			},
			FinalOutput: "The port number is 8080.",
			TokensUsed:  198,
			Assertions:  []model.AssertionResult{assertOutputContains("8080")},
		}),
		// Gemini: Uses bash commands
		buildTestRun("Read config", agentGemini, true, TestRunOpts{
			StartTime:  baseTime.Add(20 * time.Second),
			DurationMs: 7000,
			Messages: []model.Message{
				{Role: "user", Content: readConfigPrompt},
				{Role: "assistant", Content: "Reading config.", Timestamp: baseTime.Add(21 * time.Second)},
				{Role: "assistant", Content: "Config loaded.", Timestamp: baseTime.Add(23 * time.Second)},
				{Role: "assistant", Content: "Parsing content.", Timestamp: baseTime.Add(24 * time.Second)},
				{Role: "assistant", Content: "Found port.", Timestamp: baseTime.Add(25 * time.Second)},
				{Role: "assistant", Content: "Done.", Timestamp: baseTime.Add(26 * time.Second)},
			},
			ToolCalls: []model.ToolCall{
				toolGeneric(21500*time.Millisecond, 32, "bash", map[string]interface{}{"command": "test -f /etc/config.yaml && echo exists"}),
				toolGeneric(22500*time.Millisecond, 45, "bash", map[string]interface{}{"command": "cat /etc/config.yaml"}),
				toolGeneric(24*time.Second, 28, "bash", map[string]interface{}{"command": "grep port /etc/config.yaml"}),
				toolGeneric(25*time.Second, 55, "bash", map[string]interface{}{"command": "awk -F: '{print $2}' /etc/config.yaml | tr -d ' '"}),
				toolGeneric(26*time.Second, 18, "bash", map[string]interface{}{"command": "echo 'Port: 8080'"}),
			},
			FinalOutput: "port: 8080",
			TokensUsed:  212,
			Assertions:  []model.AssertionResult{assertOutputContains("8080")},
		}),
		// GPT: Uses python code execution
		buildTestRun("Read config", agentGPT, true, TestRunOpts{
			StartTime:  baseTime.Add(20 * time.Second),
			DurationMs: 8000,
			Messages: []model.Message{
				{Role: "user", Content: readConfigPrompt},
				{Role: "assistant", Content: "Loading config file.", Timestamp: baseTime.Add(21 * time.Second)},
				{Role: "assistant", Content: "Reading content.", Timestamp: baseTime.Add(23 * time.Second)},
				{Role: "assistant", Content: "Processing YAML.", Timestamp: baseTime.Add(25 * time.Second)},
				{Role: "assistant", Content: "Extracting value.", Timestamp: baseTime.Add(26 * time.Second)},
				{Role: "assistant", Content: "Config parsed.", Timestamp: baseTime.Add(27 * time.Second)},
			},
			ToolCalls: []model.ToolCall{
				toolGeneric(22*time.Second, 165, "python", map[string]interface{}{"code": "import os; os.path.exists('/etc/config.yaml')"}),
				toolGeneric(24*time.Second, 142, "python", map[string]interface{}{"code": "with open('/etc/config.yaml') as f: content = f.read()"}),
				toolGeneric(25500*time.Millisecond, 198, "python", map[string]interface{}{"code": "import yaml; config = yaml.safe_load(content)"}),
				toolGeneric(26500*time.Millisecond, 85, "python", map[string]interface{}{"code": "port = config.get('port', None)"}),
				toolGeneric(27500*time.Millisecond, 72, "python", map[string]interface{}{"code": "print(f'Port: {port}')"}),
			},
			FinalOutput: "Config: port=8080",
			TokensUsed:  245,
			Assertions:  []model.AssertionResult{assertOutputContains("8080")},
		}),
		// DISQUALIFIED: phoenix-agent uses wrong API tools
		buildTestRun("Read config", agentPhoenix, false, TestRunOpts{
			StartTime:  baseTime.Add(20 * time.Second),
			DurationMs: 15000,
			Messages: []model.Message{
				{Role: "user", Content: readConfigPrompt},
				{Role: "assistant", Content: "Attempting to read config.", Timestamp: baseTime.Add(22 * time.Second)},
				{Role: "assistant", Content: "Error accessing file.", Timestamp: baseTime.Add(26 * time.Second)},
				{Role: "assistant", Content: "Trying different path.", Timestamp: baseTime.Add(29 * time.Second)},
				{Role: "assistant", Content: "Still failing.", Timestamp: baseTime.Add(32 * time.Second)},
				{Role: "assistant", Content: "Cannot read config.", Timestamp: baseTime.Add(34 * time.Second)},
			},
			ToolCalls: []model.ToolCall{
				toolGeneric(24*time.Second, 2850, "http_get", map[string]interface{}{"url": "http://config-service/api/v1/config"}),
				toolGeneric(27*time.Second, 2650, "graphql", map[string]interface{}{"query": "{ config { port } }", "endpoint": "http://localhost:4000"}),
				toolGeneric(30*time.Second, 980, "redis_get", map[string]interface{}{"key": "app:config:port"}),
				toolGeneric(31*time.Second, 1850, "database_query", map[string]interface{}{"sql": "SELECT port FROM configs WHERE id = 1"}),
				toolGeneric(33*time.Second, 1250, "consul_kv", map[string]interface{}{"path": "config/port"}),
			},
			FinalOutput: "Unable to read configuration file.",
			TokensUsed:  489,
			Errors:      []string{"HTTP request failed: connection refused", "GraphQL endpoint not available", "Redis connection timeout"},
			Assertions: []model.AssertionResult{
				assertToolCalledFailed("read_file", "API/database calls"),
				assertOutputContainsFailed("port number"),
				assertNoErrorsFailed("Multiple connection errors encountered"),
			},
		}),
	}
}

func createFailedTest() []model.TestRun {
	dbConnectPrompt := "Connect to the production database at prod-db.example.com"
	return []model.TestRun{
		buildTestRun("Connect to database", agentGemini, false, TestRunOpts{
			DurationMs: 5000,
			TokensUsed: 423,
			Messages: []model.Message{
				{Role: "user", Content: dbConnectPrompt},
				{Role: "assistant", Content: "I'll try to connect to the database..."},
				{Role: "assistant", Content: "Would you like me to try a different connection method?"},
			},
			ToolCalls: []model.ToolCall{
				{Name: "db_connect", Parameters: map[string]interface{}{"host": "prod-db.example.com"}, Timestamp: baseTime.Add(500 * time.Millisecond)},
			},
			FinalOutput: "Would you like me to try a different connection method?",
			Errors:      []string{"Connection refused", "Timeout after 5s", "LLM asked for clarification instead of acting (iteration 2): Would you like me to try a different connection method?"},
			Assertions: []model.AssertionResult{
				assertToolCalled("db_connect"),
				{Type: "no_error_messages", Passed: false, Message: "Errors encountered"},
			},
			ClarificationStats: &model.ClarificationStats{
				Count:      2,
				Iterations: []int{2, 4},
				Examples: []string{
					"Would you like me to try a different connection method?",
					"Should I proceed with the backup connection string instead?",
				},
			},
			RateLimitStats: &model.RateLimitStats{
				RateLimitHits:     3,
				RetryCount:        3,
				RetryWaitTimeMs:   6000,
				RetrySuccessCount: 2,
			},
		}),
	}
}

// createMultiAgentSingleTest creates a Notepad automation test run by multiple agents
// Based on real Windows MCP test patterns from notepad-ui-test.yaml
func createMultiAgentSingleTest() []model.TestRun {
	prompt := "1. Launch Notepad\n2. Type \"Hello World\"\n3. Close Notepad and click \"Don't Save\" if prompted\n4. Verify no Notepad window is open"

	// GPT41 uses ui_type approach (semantic UI automation)
	gpt41Tools := []model.ToolCall{
		toolWindowsApp(500*time.Millisecond, 585, "notepad.exe", "656544"),
		toolWindowsUIType(1500*time.Millisecond, 161, "656544", "Hello World"),
		toolWindowsUIRead(2000*time.Millisecond, 34, "656544", "Hello World"),
		toolWindowsClose(2500*time.Millisecond, 550, "656544", true),
		toolWindowsList(3500*time.Millisecond, 6, "Notepad", 0),
	}

	// GPT52 uses keyboard_control approach (raw keyboard input)
	gpt52Tools := []model.ToolCall{
		toolWindowsApp(500*time.Millisecond, 453, "notepad.exe", "397030"),
		toolWindowsKeyboardType(1500*time.Millisecond, 288, "397030", "Hello World"),
		toolWindowsClose(2500*time.Millisecond, 511, "397030", true),
		toolWindowsList(3200*time.Millisecond, 3, "Notepad", 0),
	}

	// Third agent fails - uses wrong tool (screenshot instead of close)
	failedTools := []model.ToolCall{
		toolWindowsApp(500*time.Millisecond, 600, "notepad.exe", "123456"),
		toolWindowsUIType(1500*time.Millisecond, 180, "123456", "Hello World"),
		toolWindowsScreenshot(2500*time.Millisecond, 250, "123456", true), // Wrong! Should close
	}

	return []model.TestRun{
		buildTestRun("Complete Notepad automation (discard)", agentGPT41, true, TestRunOpts{
			SourceFile:  "D:\\source\\mcp-windows\\tests\\Scenarios\\notepad-ui-test.yaml",
			SessionName: "Notepad Workflow - Discard",
			DurationMs:  12087,
			Messages: []model.Message{
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: "VERIFIED: No Notepad window open."},
			},
			ToolCalls:   gpt41Tools,
			FinalOutput: "1. Notepad was launched.\n2. \"Hello World\" was typed into Notepad.\n3. Proof: Text confirmed as \"Hello World\".\n4. Notepad closed with \"Don't Save\".\n5. Verification: No Notepad window is open.\n\nVERIFIED: No Notepad window open.",
			TokensUsed:  38595,
			Assertions: []model.AssertionResult{
				assertAnyOf(true, 1, 2, []model.AssertionResult{
					assertToolCallOrder(false, []string{"app", "keyboard_control"}),
					assertToolCallOrder(true, []string{"app", "ui_type"}),
				}),
				assertOutputRegex(true, "(?i)hello\\s*world"),
				assertOutputRegex(true, "(?i)(no.*(notepad|window).*(open|found)|verified)"),
				assertNoHallucinatedTools(),
				assertNoErrors(),
				assertNoClarificationQuestions(),
				assertNoRateLimitErrors(),
			},
		}),
		buildTestRun("Complete Notepad automation (discard)", agentGPT52, true, TestRunOpts{
			SourceFile:  "D:\\source\\mcp-windows\\tests\\Scenarios\\notepad-ui-test.yaml",
			SessionName: "Notepad Workflow - Discard",
			DurationMs:  14707,
			Messages: []model.Message{
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: "✅ **VERIFIED: No Notepad window open**"},
			},
			ToolCalls:   gpt52Tools,
			FinalOutput: "**Execution Report**\n\n1. **Notepad launched successfully.**\n2. **Typed text:** `Hello World`\n3. **Proof:** Window title changed to indicate text entry.\n4. **Notepad closed** with \"Don't Save\" selected.\n5. **Verification:** 0 Notepad windows found.\n\n✅ **VERIFIED: No Notepad window open**",
			TokensUsed:  32390,
			Assertions: []model.AssertionResult{
				assertAnyOf(true, 1, 2, []model.AssertionResult{
					assertToolCallOrder(true, []string{"app", "keyboard_control"}),
					assertToolCallOrder(false, []string{"app", "ui_type"}),
				}),
				assertOutputRegex(true, "(?i)hello\\s*world"),
				assertOutputRegex(true, "(?i)(no.*(notepad|window).*(open|found)|verified)"),
				assertNoHallucinatedTools(),
				assertNoErrors(),
				assertNoClarificationQuestions(),
				assertNoRateLimitErrors(),
			},
		}),
		buildTestRun("Complete Notepad automation (discard)", agentGemini, false, TestRunOpts{
			SourceFile:  "D:\\source\\mcp-windows\\tests\\Scenarios\\notepad-ui-test.yaml",
			SessionName: "Notepad Workflow - Discard",
			DurationMs:  8500,
			Messages: []model.Message{
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: "I've taken a screenshot of the Notepad window."},
			},
			ToolCalls:   failedTools,
			FinalOutput: "Screenshot captured. Notepad window is still open.",
			TokensUsed:  25000,
			Errors:      []string{"Task incomplete: Notepad window was not closed"},
			Assertions: []model.AssertionResult{
				assertAnyOf(true, 1, 2, []model.AssertionResult{
					assertToolCallOrder(false, []string{"app", "keyboard_control"}),
					assertToolCallOrder(true, []string{"app", "ui_type"}),
				}),
				assertOutputRegex(true, "(?i)screenshot"),
				assertOutputRegex(false, "(?i)(no.*(notepad|window).*(open|found)|verified)"),
				assertNoHallucinatedTools(),
				assertNoErrorsFailed("Task incomplete: Notepad window was not closed"),
				assertNoClarificationQuestions(),
				assertNoRateLimitErrors(),
			},
		}),
	}
}

func createSingleAgentMultiSession() []model.TestRun {
	// Prompts for each test
	createConfigPrompt := "Create a configuration file at /etc/config.yaml with key: value"
	readConfigPrompt := "Read the config file and verify its contents"
	connectDBPrompt := "Connect to the PostgreSQL database at localhost:5432"
	queryUsersPrompt := "Query the users table and return the count"

	return []model.TestRun{
		// Session 1: File Operations (same file)
		buildTestRun("Create config file", agentGemini, true, TestRunOpts{
			SessionName: "File Operations",
			SourceFile:  "tests/workflow.yaml",
			DurationMs:  1500,
			TokensUsed:  245,
			Messages: []model.Message{
				{Role: "user", Content: createConfigPrompt},
				{Role: "assistant", Content: "Config file created!"},
			},
			ToolCalls: []model.ToolCall{
				{Name: "write_file", Parameters: map[string]interface{}{"path": "/etc/config.yaml", "content": "key: value"}, Timestamp: baseTime.Add(500 * time.Millisecond)},
			},
			FinalOutput: "Config file created!",
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
		buildTestRun("Read config file", agentGemini, true, TestRunOpts{
			SessionName: "File Operations",
			SourceFile:  "tests/workflow.yaml",
			StartTime:   baseTime.Add(2 * time.Second),
			DurationMs:  1500,
			TokensUsed:  189,
			Messages: []model.Message{
				{Role: "user", Content: readConfigPrompt},
				{Role: "assistant", Content: "Config content: key: value"},
			},
			ToolCalls:   []model.ToolCall{toolReadFile(2500*time.Millisecond, 45)},
			FinalOutput: "Config content: key: value",
			Assertions:  []model.AssertionResult{assertOutputContains("key: value")},
		}),
		// Session 2: Database Operations (same file)
		buildTestRun("Connect to database", agentGemini, true, TestRunOpts{
			SessionName: "Database Operations",
			SourceFile:  "tests/workflow.yaml",
			StartTime:   baseTime.Add(5 * time.Second),
			DurationMs:  2000,
			TokensUsed:  312,
			Messages: []model.Message{
				{Role: "user", Content: connectDBPrompt},
				{Role: "assistant", Content: "Database connection established."},
			},
			ToolCalls:   []model.ToolCall{toolDBConnect(5500*time.Millisecond, 150, "localhost", 5432)},
			FinalOutput: "Database connection established.",
			Assertions:  []model.AssertionResult{assertToolCalled("db_connect")},
		}),
		buildTestRun("Query users table", agentGemini, true, TestRunOpts{
			SessionName: "Database Operations",
			SourceFile:  "tests/workflow.yaml",
			StartTime:   baseTime.Add(8 * time.Second),
			DurationMs:  1000,
			TokensUsed:  278,
			Messages: []model.Message{
				{Role: "user", Content: queryUsersPrompt},
				{Role: "assistant", Content: "Found 42 users in the database."},
			},
			ToolCalls:   []model.ToolCall{toolDBQuery(8200*time.Millisecond, 85, "SELECT * FROM users")},
			FinalOutput: "Found 42 users in the database.",
			Assertions:  []model.AssertionResult{assertOutputContains("42 users")},
		}),
	}
}

// createMultiAgentMultiSession creates multiple agents running tests across multiple sessions (no multi-file)
func createMultiAgentMultiSession() []model.TestRun {
	agents := []AgentConfig{agentClaude, agentGemini, agentGPT}
	var runs []model.TestRun

	// Session 1: Setup - 2 tests per agent
	// Test 1: Create workspace
	for i, agent := range agents {
		passed := true
		runs = append(runs, buildTestRun("Create workspace", agent, passed, TestRunOpts{
			SessionName: "Setup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(time.Duration(i) * 500 * time.Millisecond),
			DurationMs:  1500,
			TokensUsed:  180 + i*20,
			Messages: []model.Message{
				{Role: "user", Content: "Create a workspace directory structure"},
				{Role: "assistant", Content: "Workspace created at /workspace"},
			},
			ToolCalls:   []model.ToolCall{toolWriteFile(300*time.Millisecond, 35)},
			FinalOutput: "Workspace created at /workspace",
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}))
	}
	// Test 2: Initialize config
	for i, agent := range agents {
		passed := i != 2 // GPT fails
		finalOutput := "Config initialized successfully."
		if !passed {
			finalOutput = "Failed to initialize config."
		}
		runs = append(runs, buildTestRun("Initialize config", agent, passed, TestRunOpts{
			SessionName: "Setup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(2*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  2000,
			TokensUsed:  220 + i*30,
			Messages: []model.Message{
				{Role: "user", Content: "Initialize the configuration files"},
				{Role: "assistant", Content: finalOutput},
			},
			ToolCalls:   []model.ToolCall{toolWriteFile(2300*time.Millisecond, 55)},
			FinalOutput: finalOutput,
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}))
	}

	// Session 2: Processing - 3 tests per agent
	// Test 1: Connect database
	for i, agent := range agents {
		runs = append(runs, buildTestRun("Connect database", agent, true, TestRunOpts{
			SessionName: "Processing",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(5*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  2500,
			TokensUsed:  280 + i*25,
			Messages: []model.Message{
				{Role: "user", Content: "Connect to the PostgreSQL database"},
				{Role: "assistant", Content: "Connected to database successfully"},
			},
			ToolCalls:   []model.ToolCall{toolDBConnect(5300*time.Millisecond, 150, "localhost", 5432)},
			FinalOutput: "Connected to database successfully",
			Assertions:  []model.AssertionResult{assertToolCalled("db_connect")},
		}))
	}
	// Test 2: Query data
	for i, agent := range agents {
		runs = append(runs, buildTestRun("Query data", agent, true, TestRunOpts{
			SessionName: "Processing",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(8*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  1800,
			TokensUsed:  320 + i*20,
			Messages: []model.Message{
				{Role: "user", Content: "Query all records from the data table"},
				{Role: "assistant", Content: "Retrieved 1,234 records"},
			},
			ToolCalls:   []model.ToolCall{toolDBQuery(8200*time.Millisecond, 95, "SELECT * FROM data")},
			FinalOutput: "Retrieved 1,234 records",
			Assertions:  []model.AssertionResult{assertToolCalled("db_query")},
		}))
	}
	// Test 3: Generate report
	for i, agent := range agents {
		runs = append(runs, buildTestRun("Generate report", agent, true, TestRunOpts{
			SessionName: "Processing",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(11*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  3000,
			TokensUsed:  450 + i*30,
			Messages: []model.Message{
				{Role: "user", Content: "Generate a summary report from the queried data"},
				{Role: "assistant", Content: "Report generated at /reports/summary.pdf"},
			},
			ToolCalls:   []model.ToolCall{toolWriteFile(11500*time.Millisecond, 180)},
			FinalOutput: "Report generated at /reports/summary.pdf",
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}))
	}

	// Session 3: Cleanup - 2 tests per agent
	// Test 1: Delete temp files
	for i, agent := range agents {
		passed := i != 1 // Gemini fails
		finalOutput := "Temp files deleted."
		if !passed {
			finalOutput = "Some temp files could not be deleted."
		}
		runs = append(runs, buildTestRun("Delete temp files", agent, passed, TestRunOpts{
			SessionName: "Cleanup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(15*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  1200,
			TokensUsed:  140 + i*15,
			Messages: []model.Message{
				{Role: "user", Content: "Delete all temporary files"},
				{Role: "assistant", Content: finalOutput},
			},
			ToolCalls:   []model.ToolCall{toolGeneric(15300*time.Millisecond, 25, "delete_file", map[string]interface{}{"path": "/tmp/*"})},
			FinalOutput: finalOutput,
			Assertions:  []model.AssertionResult{assertToolCalled("delete_file")},
		}))
	}
	// Test 2: Close connections
	for i, agent := range agents {
		passed := i == 0 // Only Claude succeeds
		finalOutput := "All connections closed."
		assertMsg := "Output contains 'closed'"
		if !passed {
			finalOutput = "Warning: Some connections timed out."
			assertMsg = "Output does not contain 'closed'"
		}
		runs = append(runs, buildTestRun("Close connections", agent, passed, TestRunOpts{
			SessionName: "Cleanup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(17*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  1500,
			TokensUsed:  160 + i*20,
			Messages: []model.Message{
				{Role: "user", Content: "Close all database and network connections"},
				{Role: "assistant", Content: finalOutput},
			},
			ToolCalls:   []model.ToolCall{toolGeneric(17300*time.Millisecond, 40, "db_disconnect", map[string]interface{}{})},
			FinalOutput: finalOutput,
			Assertions: []model.AssertionResult{
				assertToolCalled("db_disconnect"),
				{Type: "output_contains", Passed: passed, Message: assertMsg},
			},
		}))
	}

	return runs
}

// createSingleAgentMultiFile creates a single agent running tests across multiple files with multiple sessions
func createSingleAgentMultiFile() []model.TestRun {
	// Prompts for each test
	createConfigPrompt := "Create a config file at /etc/app/config.yaml"
	validateConfigPrompt := "Validate the config syntax is valid YAML"
	connectDBPrompt := "Connect to the PostgreSQL database at localhost:5432"
	queryUsersPrompt := "Query the users table and return the count"
	testHealthPrompt := "Test the /health endpoint returns 200"
	testUsersAPIPrompt := "Test the /api/users endpoint returns user list"

	return []model.TestRun{
		// File 1: config_tests.yaml - Session: Configuration
		buildTestRun("Create config file", agentClaude, true, TestRunOpts{
			SessionName: "Configuration",
			SourceFile:  "tests/config_tests.yaml",
			DurationMs:  1800,
			TokensUsed:  256,
			Messages: []model.Message{
				{Role: "user", Content: createConfigPrompt},
				{Role: "assistant", Content: "Config file created at /etc/app/config.yaml"},
			},
			ToolCalls:   []model.ToolCall{toolWriteFile(500*time.Millisecond, 60)},
			FinalOutput: "Config file created at /etc/app/config.yaml",
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
		buildTestRun("Validate config syntax", agentClaude, true, TestRunOpts{
			SessionName: "Configuration",
			SourceFile:  "tests/config_tests.yaml",
			StartTime:   baseTime.Add(2 * time.Second),
			DurationMs:  1200,
			TokensUsed:  189,
			Messages: []model.Message{
				{Role: "user", Content: validateConfigPrompt},
				{Role: "assistant", Content: "Config syntax is valid YAML"},
			},
			ToolCalls:   []model.ToolCall{toolReadFile(2300*time.Millisecond, 40)},
			FinalOutput: "Config syntax is valid YAML",
			Assertions:  []model.AssertionResult{assertOutputContains("valid")},
		}),
		// File 2: database_tests.yaml - Session: Database
		buildTestRun("Connect to database", agentClaude, true, TestRunOpts{
			SessionName: "Database",
			SourceFile:  "tests/database_tests.yaml",
			StartTime:   baseTime.Add(5 * time.Second),
			DurationMs:  2500,
			TokensUsed:  312,
			Messages: []model.Message{
				{Role: "user", Content: connectDBPrompt},
				{Role: "assistant", Content: "Connected to PostgreSQL at localhost:5432"},
			},
			ToolCalls:   []model.ToolCall{toolDBConnect(5500*time.Millisecond, 180, "localhost", 5432)},
			FinalOutput: "Connected to PostgreSQL at localhost:5432",
			Assertions:  []model.AssertionResult{assertToolCalled("db_connect")},
		}),
		buildTestRun("Query users table", agentClaude, true, TestRunOpts{
			SessionName: "Database",
			SourceFile:  "tests/database_tests.yaml",
			StartTime:   baseTime.Add(8 * time.Second),
			DurationMs:  1100,
			TokensUsed:  245,
			Messages: []model.Message{
				{Role: "user", Content: queryUsersPrompt},
				{Role: "assistant", Content: "Query returned 156 users"},
			},
			ToolCalls:   []model.ToolCall{toolDBQuery(8200*time.Millisecond, 75, "SELECT COUNT(*) FROM users")},
			FinalOutput: "Query returned 156 users",
			Assertions:  []model.AssertionResult{assertOutputContains("users")},
		}),
		// File 3: api_tests.yaml - Session: API Validation
		buildTestRun("Test health endpoint", agentClaude, true, TestRunOpts{
			SessionName: "API Validation",
			SourceFile:  "tests/api_tests.yaml",
			StartTime:   baseTime.Add(12 * time.Second),
			DurationMs:  800,
			TokensUsed:  178,
			Messages: []model.Message{
				{Role: "user", Content: testHealthPrompt},
				{Role: "assistant", Content: "Health endpoint returned 200 OK"},
			},
			ToolCalls:   []model.ToolCall{toolGeneric(12300*time.Millisecond, 50, "http_get", map[string]interface{}{"url": "/health"})},
			FinalOutput: "Health endpoint returned 200 OK",
			Assertions:  []model.AssertionResult{assertOutputContains("200")},
		}),
		buildTestRun("Test users endpoint", agentClaude, false, TestRunOpts{
			SessionName: "API Validation",
			SourceFile:  "tests/api_tests.yaml",
			StartTime:   baseTime.Add(14 * time.Second),
			DurationMs:  1500,
			TokensUsed:  298,
			Errors:      []string{"Response timeout after 1000ms"},
			Messages: []model.Message{
				{Role: "user", Content: testUsersAPIPrompt},
				{Role: "assistant", Content: "Request timed out"},
			},
			ToolCalls:   []model.ToolCall{toolGeneric(14200*time.Millisecond, 1200, "http_get", map[string]interface{}{"url": "/api/users"})},
			FinalOutput: "Request timed out",
			Assertions: []model.AssertionResult{
				assertOutputContainsFailed("200"),
				assertNoErrorsFailed("Response timeout after 1000ms"),
			},
		}),
	}
}

// createFullSuite creates a comprehensive multi-agent, multi-session suite run (renamed to multi_agent_multi_file)
func createFullSuite() []model.TestRun {
	fileOpsPrompt := "Create a configuration file at /etc/app/config.yaml"
	readConfigPrompt := "Read the config file and verify its contents"
	dbConnectPrompt := "Connect to the PostgreSQL database at localhost:5432"
	dbQueryPrompt := "Query the users table and return the count"
	apiTestPrompt := "Call the /api/health endpoint and verify the response"
	cleanupPrompt := "Delete all temporary files created during the test"

	agents := []struct {
		name         string
		provider     model.ProviderType
		disqualified bool // If true, agent fails all tests
	}{
		{"claude-agent", model.ProviderAnthropic, false},
		{"gpt-agent", model.ProviderAzure, false},
		{"gemini-agent", model.ProviderGoogle, false},
		{"phoenix-agent", model.ProviderOpenAI, true}, // Disqualified: fails all tests
	}

	var runs []model.TestRun

	// Session 1: File Operations - all agents
	for i, agent := range agents {
		offset := time.Duration(i) * 500 * time.Millisecond

		// Test 1: Create config
		passed := !agent.disqualified
		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Create config file",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "File Operations",
				SourceFile:   "tests/file_ops.yaml",
				StartTime:    baseTime.Add(offset),
				EndTime:      baseTime.Add(offset + 1500*time.Millisecond),
				Messages:     []model.Message{{Role: "user", Content: fileOpsPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "write_file", Parameters: map[string]interface{}{"path": func() string {
						if agent.disqualified {
							return "/wrong/path/config.yaml"
						}
						return "/etc/app/config.yaml"
					}(), "content": "port: 8080\nenv: production"}, Timestamp: baseTime.Add(offset + 500*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "Failed to create configuration file."
					}
					return "Configuration file created successfully."
				}(),
				TokensUsed: 120 + i*15,
				LatencyMs:  1500,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"Permission denied", "Invalid path"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called"},
				{Type: "tool_param_contains", Passed: passed, Message: func() string {
					if passed {
						return "Path parameter is correct"
					}
					return "Path parameter is wrong"
				}()},
			},
			Passed: passed,
		})

		// Test 2: Read config
		passed2 := !agent.disqualified && i != 2 // gemini fails this one, mistral always fails
		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Read and verify config",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "File Operations",
				SourceFile:   "tests/file_ops.yaml",
				StartTime:    baseTime.Add(offset + 2*time.Second),
				EndTime:      baseTime.Add(offset + 3200*time.Millisecond),
				Messages:     []model.Message{{Role: "user", Content: readConfigPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "read_file", Parameters: map[string]interface{}{"path": func() string {
						if agent.disqualified {
							return "/wrong/path/config.yaml"
						}
						return "/etc/app/config.yaml"
					}()}, Timestamp: baseTime.Add(offset + 2500*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "Failed to read config file"
					}
					if passed2 {
						return "Config verified: port=8080, env=production"
					}
					return "Config file found but format unclear"
				}(),
				TokensUsed: 95 + i*10,
				LatencyMs:  1200,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"File not found"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "output_contains", Passed: passed2, Message: func() string {
					if passed2 {
						return "Output contains port number"
					}
					return "Output does not contain port number"
				}()},
			},
			Passed: passed2,
		})
	}

	// Session 2: Database Operations - all agents
	for i, agent := range agents {
		offset := time.Duration(i)*500*time.Millisecond + 5*time.Second

		// Test 3: Connect to DB
		passed := !agent.disqualified && i != 1 // gpt fails this one, mistral always fails
		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Connect to database",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "Database Operations",
				SourceFile:   "tests/db_ops.yaml",
				StartTime:    baseTime.Add(offset),
				EndTime:      baseTime.Add(offset + 2000*time.Millisecond),
				Messages:     []model.Message{{Role: "user", Content: dbConnectPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "db_connect", Parameters: map[string]interface{}{"host": func() string {
						if agent.disqualified {
							return "wrong-host"
						}
						return "localhost"
					}(), "port": 5432, "database": "testdb"}, Timestamp: baseTime.Add(offset + 800*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "Database connection failed: unknown host"
					}
					if passed {
						return "Connected to PostgreSQL database successfully."
					}
					return "Connection timeout after 30s"
				}(),
				TokensUsed: 145 + i*20,
				LatencyMs:  2000,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"Unknown host", "DNS resolution failed"}
					}
					if !passed {
						return []string{"Connection timeout"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'db_connect' was called"},
				{Type: "no_error_messages", Passed: passed, Message: func() string {
					if passed {
						return "No errors occurred"
					}
					return "Connection failed with error"
				}()},
			},
			Passed: passed,
		})

		// Test 4: Query users
		passed4 := !agent.disqualified
		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Query users table",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "Database Operations",
				SourceFile:   "tests/db_ops.yaml",
				StartTime:    baseTime.Add(offset + 3*time.Second),
				EndTime:      baseTime.Add(offset + 4*time.Second),
				Messages:     []model.Message{{Role: "user", Content: dbQueryPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "db_query", Parameters: map[string]interface{}{"sql": func() string {
						if agent.disqualified {
							return "SELEC * FORM users" // Intentional typo
						}
						return "SELECT COUNT(*) FROM users"
					}()}, Timestamp: baseTime.Add(offset + 3300*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "SQL syntax error near 'SELEC'"
					}
					return "Found 42 users in the database."
				}(),
				TokensUsed: 110 + i*12,
				LatencyMs:  1000,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"SQL syntax error"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "output_contains", Passed: passed4, Message: func() string {
					if passed4 {
						return "Output contains user count"
					}
					return "Output contains SQL error"
				}()},
			},
			Passed: passed4,
		})
	}

	// Session 3: API Testing - all agents
	for i, agent := range agents {
		offset := time.Duration(i)*500*time.Millisecond + 12*time.Second

		passed := !agent.disqualified
		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Health check API",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "API Testing",
				SourceFile:   "tests/api_tests.yaml",
				StartTime:    baseTime.Add(offset),
				EndTime:      baseTime.Add(offset + 800*time.Millisecond),
				Messages:     []model.Message{{Role: "user", Content: apiTestPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": func() string {
						if agent.disqualified {
							return "http://wrong-host:8080/api/status"
						}
						return "http://localhost:8080/api/health"
					}()}, Timestamp: baseTime.Add(offset + 300*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "Error: Connection refused"
					}
					return "API health check passed: {\"status\": \"healthy\", \"uptime\": \"24h\"}"
				}(),
				TokensUsed: 85 + i*8,
				LatencyMs:  800,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"Connection refused"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "output_contains", Passed: passed, Message: func() string {
					if passed {
						return "Output contains 'healthy'"
					}
					return "Output does not contain 'healthy'"
				}()},
			},
			Passed: passed,
		})
	}

	// Session 4: Cleanup - all agents
	for i, agent := range agents {
		offset := time.Duration(i)*500*time.Millisecond + 15*time.Second

		passed := !agent.disqualified
		if i == 0 { // claude fails cleanup (left a file behind)
			passed = false
		}

		runs = append(runs, model.TestRun{
			Execution: &model.ExecutionResult{
				TestName:     "Cleanup temp files",
				AgentName:    agent.name,
				ProviderType: agent.provider,
				SessionName:  "Cleanup",
				SourceFile:   "tests/cleanup.yaml",
				StartTime:    baseTime.Add(offset),
				EndTime:      baseTime.Add(offset + 1200*time.Millisecond),
				Messages:     []model.Message{{Role: "user", Content: cleanupPrompt}},
				ToolCalls: []model.ToolCall{
					{Name: "delete_file", Parameters: map[string]interface{}{"path": func() string {
						if agent.disqualified {
							return "/wrong/path/*"
						}
						return "/tmp/test_*"
					}()}, Timestamp: baseTime.Add(offset + 400*time.Millisecond)},
					{Name: "list_files", Parameters: map[string]interface{}{"path": func() string {
						if agent.disqualified {
							return "/wrong/path"
						}
						return "/tmp"
					}()}, Timestamp: baseTime.Add(offset + 800*time.Millisecond)},
				},
				FinalOutput: func() string {
					if agent.disqualified {
						return "Error: Path not found, cleanup failed"
					}
					if passed {
						return "All temporary files deleted successfully."
					}
					return "Cleanup completed but 1 file remains: /tmp/test_lock.pid"
				}(),
				TokensUsed: 130 + i*15,
				LatencyMs:  1200,
				Errors: func() []string {
					if agent.disqualified {
						return []string{"Path not found", "Cleanup failed"}
					}
					return nil
				}(),
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: !agent.disqualified, Message: func() string {
					if agent.disqualified {
						return "Tool called with wrong path"
					}
					return "Tool 'delete_file' was called"
				}()},
				{Type: "output_not_contains", Passed: passed, Message: func() string {
					if passed {
						return "No remaining files mentioned"
					}
					return "Output mentions remaining file or error"
				}(), Details: func() map[string]interface{} {
					if !passed {
						return map[string]interface{}{"expected": "no files remain", "actual": "files remain or error"}
					}
					return nil
				}()},
			},
			Passed: passed,
		})
	}

	return runs
}
