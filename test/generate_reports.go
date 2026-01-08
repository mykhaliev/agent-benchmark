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
	}
}

func toolReadFile(offset time.Duration, durationMs int64) model.ToolCall {
	return model.ToolCall{
		Name:       "read_file",
		Parameters: map[string]interface{}{"path": "/etc/config.yaml"},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolBashCommand(offset time.Duration, durationMs int64, cmd string) model.ToolCall {
	return model.ToolCall{
		Name:       "run_bash",
		Parameters: map[string]interface{}{"command": cmd},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolPythonExec(offset time.Duration, durationMs int64, code string) model.ToolCall {
	return model.ToolCall{
		Name:       "execute_python",
		Parameters: map[string]interface{}{"code": code},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolDBConnect(offset time.Duration, durationMs int64, host string, port int) model.ToolCall {
	return model.ToolCall{
		Name:       "db_connect",
		Parameters: map[string]interface{}{"host": host, "port": port},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolDBQuery(offset time.Duration, durationMs int64, sql string) model.ToolCall {
	return model.ToolCall{
		Name:       "db_query",
		Parameters: map[string]interface{}{"sql": sql},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolHTTPGet(offset time.Duration, durationMs int64, url string) model.ToolCall {
	return model.ToolCall{
		Name:       "http_get",
		Parameters: map[string]interface{}{"url": url},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolDeleteFile(offset time.Duration, durationMs int64, path string) model.ToolCall {
	return model.ToolCall{
		Name:       "delete_file",
		Parameters: map[string]interface{}{"path": path},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolListFiles(offset time.Duration, durationMs int64, path string) model.ToolCall {
	return model.ToolCall{
		Name:       "list_files",
		Parameters: map[string]interface{}{"path": path},
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
	}
}

func toolGeneric(offset time.Duration, durationMs int64, name string, params map[string]interface{}) model.ToolCall {
	return model.ToolCall{
		Name:       name,
		Parameters: params,
		Timestamp:  baseTime.Add(offset),
		DurationMs: durationMs,
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

func main() {
	gen, err := report.NewGenerator()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create generator: %v\n", err)
		os.Exit(1)
	}

	outDir := "generated_reports"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Hierarchical test reports - from simple to complex:
	// Level 1: Single agent, single test (simplest case)
	// Level 2: Single agent, multiple tests (test overview table)
	// Level 3: Multiple agents, single test (leaderboard focus)
	// Level 4: Multiple agents, multiple tests (full matrix)
	// Level 5: Single agent, multiple sessions (session grouping with flow diagrams)
	// Level 6: Multiple agents, multiple sessions (session grouping, no flow diagrams)
	// Level 7: Full suite (multiple agents, sessions, files)
	// Bonus: Failed test with errors
	fixtures := []struct {
		name    string
		level   int
		results []model.TestRun
	}{
		{"01_single_agent_single_test", 1, createSingleAgentOneTest()},
		{"02_single_agent_multi_test", 2, createSingleAgentTwoTests()},
		{"03_multi_agent_single_test", 3, createMultiAgentSingleTest()},
		{"04_multi_agent_multi_test", 4, createMultiAgent()},
		{"05_single_agent_multi_session", 5, createSingleAgentMultiSession()},
		{"06_multi_agent_multi_session", 6, createMultiAgentMultiSession()},
		{"07_single_agent_multi_file", 7, createSingleAgentMultiFile()},
		{"08_multi_agent_multi_file", 8, createFullSuite()},
		{"09_failed_with_errors", 0, createFailedTest()},
	}

	fmt.Println("Generating hierarchical test reports...")
	fmt.Println("=========================================")
	for _, f := range fixtures {
		html, err := gen.GenerateHTML(f.results)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate %s: %v\n", f.name, err)
			continue
		}

		outPath := filepath.Join(outDir, f.name+".html")
		if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", outPath, err)
			continue
		}
		levelStr := ""
		if f.level > 0 {
			levelStr = fmt.Sprintf(" (Level %d)", f.level)
		}
		fmt.Printf("✓ %s%s - %d bytes\n", f.name, levelStr, len(html))
	}
	fmt.Println("=========================================")
	fmt.Println("Done! Open generated_reports/*.html to view.")
}

func createSingleAgentOneTest() []model.TestRun {
	return []model.TestRun{
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs: 2340,
			Messages:   []model.Message{{Role: "user", Content: "Create a test file at /tmp/test.txt with the content 'Hello World'"}},
			ToolCalls:  []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
			FinalOutput: "Done!",
			TokensUsed:  325,
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
	}
}

func createSingleAgentTwoTests() []model.TestRun {
	return []model.TestRun{
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs: 2340,
			Messages:   []model.Message{{Role: "user", Content: "Create a test file at /tmp/test.txt with the content 'Hello World'"}},
			ToolCalls:  []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
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
			FinalOutput: "The port number in the configuration is 8080.",
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
			FinalOutput: "File created successfully at /tmp/test.txt with content 'Hello World'",
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
			FinalOutput: "Done! File created using bash.",
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
			FinalOutput: "File created via Python.",
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
			FinalOutput: "Failed to create file. I don't have the right tools.",
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
	return []model.TestRun{
		buildTestRun("Connect to database", agentGemini, false, TestRunOpts{
			DurationMs: 5000,
			TokensUsed: 423,
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

// createMultiAgentSingleTest creates a single test run by multiple agents
func createMultiAgentSingleTest() []model.TestRun {
	prompt := "Create a test file at /tmp/test.txt with the content 'Hello World'"
	return []model.TestRun{
		buildTestRun("Create test file", agentClaude, true, TestRunOpts{
			DurationMs:  3500,
			Messages:    []model.Message{{Role: "user", Content: prompt}},
			ToolCalls:   []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
			FinalOutput: "File created successfully!",
			TokensUsed:  289,
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
		buildTestRun("Create test file", agentGemini, true, TestRunOpts{
			DurationMs:  4200,
			Messages:    []model.Message{{Role: "user", Content: prompt}},
			ToolCalls:   []model.ToolCall{toolWriteFile(600*time.Millisecond, 52)},
			FinalOutput: "Done! File created at /tmp/test.txt",
			TokensUsed:  312,
			Assertions:  []model.AssertionResult{assertToolCalled("write_file")},
		}),
		buildTestRun("Create test file", agentGPT, false, TestRunOpts{
			DurationMs:  5100,
			Messages:    []model.Message{{Role: "user", Content: prompt}},
			ToolCalls:   []model.ToolCall{toolWriteFile(800*time.Millisecond, 68)},
			FinalOutput: "I wrote 'hello world' to the file.",
			TokensUsed:  345,
			Errors:      []string{"Content mismatch: expected 'Hello World', got 'hello world'"},
			Assertions: []model.AssertionResult{
				assertToolCalled("write_file"),
				assertParamEquals(false, "Content case mismatch", map[string]interface{}{"expected": "Hello World", "actual": "hello world"}),
			},
		}),
	}
}

func createSingleAgentMultiSession() []model.TestRun {
	return []model.TestRun{
		// Session 1: File Operations (same file)
		buildTestRun("Create config file", agentGemini, true, TestRunOpts{
			SessionName: "File Operations",
			SourceFile:  "tests/workflow.yaml",
			DurationMs:  1500,
			TokensUsed:  245,
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

	// Session 1: Setup
	for i, agent := range agents {
		passed := i != 2 // GPT fails
		runs = append(runs, buildTestRun("Initialize workspace", agent, passed, TestRunOpts{
			SessionName: "Setup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(time.Duration(i) * 500 * time.Millisecond),
			DurationMs:  2000,
			TokensUsed:  200 + i*30,
			ToolCalls:   []model.ToolCall{toolWriteFile(500*time.Millisecond, 45)},
			FinalOutput: func() string {
				if passed {
					return "Workspace initialized successfully."
				}
				return "Failed to initialize workspace."
			}(),
			Assertions: []model.AssertionResult{assertToolCalled("write_file")},
		}))
	}

	// Session 2: Processing
	for i, agent := range agents {
		passed := true
		runs = append(runs, buildTestRun("Process data", agent, passed, TestRunOpts{
			SessionName: "Processing",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(5*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  3000,
			TokensUsed:  350 + i*25,
			ToolCalls:   []model.ToolCall{toolDBQuery(5500*time.Millisecond, 120, "SELECT * FROM data")},
			FinalOutput: "Data processed successfully.",
			Assertions:  []model.AssertionResult{assertToolCalled("db_query")},
		}))
	}

	// Session 3: Cleanup
	for i, agent := range agents {
		passed := i == 0 // Only Claude succeeds
		runs = append(runs, buildTestRun("Cleanup resources", agent, passed, TestRunOpts{
			SessionName: "Cleanup",
			SourceFile:  "tests/multi_session.yaml",
			StartTime:   baseTime.Add(10*time.Second + time.Duration(i)*500*time.Millisecond),
			DurationMs:  1500,
			TokensUsed:  150 + i*20,
			ToolCalls:   []model.ToolCall{toolGeneric(10500*time.Millisecond, 30, "delete_file", map[string]interface{}{"path": "/tmp/data"})},
			FinalOutput: func() string {
				if passed {
					return "Cleanup completed."
				}
				return "Partial cleanup - some files remain."
			}(),
			Assertions: []model.AssertionResult{
				assertToolCalled("delete_file"),
				{Type: "output_contains", Passed: passed, Message: func() string {
					if passed {
						return "Output contains 'completed'"
					}
					return "Output does not contain 'completed'"
				}()},
			},
		}))
	}

	return runs
}

// createSingleAgentMultiFile creates a single agent running tests across multiple files with multiple sessions
func createSingleAgentMultiFile() []model.TestRun {
	return []model.TestRun{
		// File 1: config_tests.yaml - Session: Configuration
		buildTestRun("Create config file", agentClaude, true, TestRunOpts{
			SessionName: "Configuration",
			SourceFile:  "tests/config_tests.yaml",
			DurationMs:  1800,
			TokensUsed:  256,
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
				TestName:    "Create config file",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "File Operations",
				SourceFile:  "tests/file_ops.yaml",
				StartTime:   baseTime.Add(offset),
				EndTime:     baseTime.Add(offset + 1500*time.Millisecond),
				Messages:    []model.Message{{Role: "user", Content: fileOpsPrompt}},
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
				TokensUsed:  120 + i*15,
				LatencyMs:   1500,
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
				TestName:    "Read and verify config",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "File Operations",
				SourceFile:  "tests/file_ops.yaml",
				StartTime:   baseTime.Add(offset + 2*time.Second),
				EndTime:     baseTime.Add(offset + 3200*time.Millisecond),
				Messages:    []model.Message{{Role: "user", Content: readConfigPrompt}},
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
				TestName:    "Connect to database",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "Database Operations",
				SourceFile:  "tests/db_ops.yaml",
				StartTime:   baseTime.Add(offset),
				EndTime:     baseTime.Add(offset + 2000*time.Millisecond),
				Messages:    []model.Message{{Role: "user", Content: dbConnectPrompt}},
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
				TestName:    "Query users table",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "Database Operations",
				SourceFile:  "tests/db_ops.yaml",
				StartTime:   baseTime.Add(offset + 3*time.Second),
				EndTime:     baseTime.Add(offset + 4*time.Second),
				Messages:    []model.Message{{Role: "user", Content: dbQueryPrompt}},
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
				TestName:    "Health check API",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "API Testing",
				SourceFile:  "tests/api_tests.yaml",
				StartTime:   baseTime.Add(offset),
				EndTime:     baseTime.Add(offset + 800*time.Millisecond),
				Messages:    []model.Message{{Role: "user", Content: apiTestPrompt}},
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
				TestName:    "Cleanup temp files",
				AgentName:   agent.name,
				ProviderType: agent.provider,
				SessionName: "Cleanup",
				SourceFile:  "tests/cleanup.yaml",
				StartTime:   baseTime.Add(offset),
				EndTime:     baseTime.Add(offset + 1200*time.Millisecond),
				Messages:    []model.Message{{Role: "user", Content: cleanupPrompt}},
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
