// Package tests provides realistic test fixtures for report generation tests
package tests

import (
	"time"

	"github.com/mykhaliev/agent-benchmark/model"
)

// fixtureBaseTime provides a consistent base time for all fixtures
var fixtureBaseTime = time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)

// createSingleAgentSingleTest returns a single-agent test with 5 steps
func createSingleAgentSingleTest() []model.TestRun {
	return []model.TestRun{
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(8500 * time.Millisecond),
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace with config, src folder, and initial files", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll set up the project workspace for you. Let me create the directory structure first.", Timestamp: fixtureBaseTime.Add(500 * time.Millisecond)},
					{Role: "assistant", Content: "Directory created. Now I'll add the configuration file.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Config ready. Creating the main source file.", Timestamp: fixtureBaseTime.Add(4 * time.Second)},
					{Role: "assistant", Content: "Source file created. Adding the README.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "All done! Your project workspace is ready.", Timestamp: fixtureBaseTime.Add(8 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{
						Name:       "create_directory",
						Parameters: map[string]interface{}{"path": "/workspace/myproject/src"},
						Timestamp:  fixtureBaseTime.Add(1 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created: /workspace/myproject/src"}}},
					},
					{
						Name:       "write_file",
						Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json", "content": "{\"name\": \"myproject\", \"version\": \"1.0.0\"}"},
						Timestamp:  fixtureBaseTime.Add(3 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created successfully"}}},
					},
					{
						Name:       "write_file",
						Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go", "content": "package main\n\nfunc main() {}"},
						Timestamp:  fixtureBaseTime.Add(5 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created successfully"}}},
					},
					{
						Name:       "write_file",
						Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md", "content": "# My Project\n\nA sample project."},
						Timestamp:  fixtureBaseTime.Add(7 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created successfully"}}},
					},
					{
						Name:       "list_files",
						Parameters: map[string]interface{}{"path": "/workspace/myproject"},
						Timestamp:  fixtureBaseTime.Add(8 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "config.json, README.md, src/"}}},
					},
				},
				FinalOutput: "Done! I've set up your project workspace with config.json, src/main.go, and README.md.",
				TokensUsed:  456,
				LatencyMs:   8500,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called", Details: map[string]interface{}{"tool": "create_directory"}},
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called", Details: map[string]interface{}{"tool": "write_file"}},
				{Type: "tool_called", Passed: true, Message: "Tool 'list_files' was called", Details: map[string]interface{}{"tool": "list_files"}},
				{Type: "output_contains", Passed: true, Message: "Output contains 'Done'", Details: map[string]interface{}{"expected": "Done", "found": true}},
			},
			Passed: true,
		},
	}
}

// createMultiAgentComparison returns a multi-agent comparison scenario (same test, different agents, each with 5 steps)
// Builds on single agent: same "Setup project workspace" test, but now comparing 3 agents
func createMultiAgentComparison() []model.TestRun {
	return []model.TestRun{
		// Gemini agent - passes (same as single agent test)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(8500 * time.Millisecond),
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace with config, src folder, and initial files", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll set up the project workspace for you.", Timestamp: fixtureBaseTime.Add(500 * time.Millisecond)},
					{Role: "assistant", Content: "Directory created. Adding config.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Config ready. Creating source file.", Timestamp: fixtureBaseTime.Add(4 * time.Second)},
					{Role: "assistant", Content: "Source file created. Adding README.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "All done! Workspace is ready.", Timestamp: fixtureBaseTime.Add(8 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(1 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json", "content": "{\"name\": \"myproject\"}"}, Timestamp: fixtureBaseTime.Add(3 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go", "content": "package main"}, Timestamp: fixtureBaseTime.Add(5 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md", "content": "# My Project"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(8 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "config.json, README.md, src/"}}}},
				},
				FinalOutput: "Done! Project workspace is set up with all files.",
				TokensUsed:  456,
				LatencyMs:   8500,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called"},
				{Type: "tool_param_equals", Passed: true, Message: "Parameters match expected values"},
			},
			Passed: true,
		},
		// Claude agent - passes but slower, different approach (5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "claude-agent",
				ProviderType: model.ProviderAnthropic,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(12000 * time.Millisecond),
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace with config, src folder, and initial files", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll help you set up the project workspace. First, let me check if the directory exists.", Timestamp: fixtureBaseTime.Add(1 * time.Second)},
					{Role: "assistant", Content: "Good, the path is clear. Creating the project structure now.", Timestamp: fixtureBaseTime.Add(3 * time.Second)},
					{Role: "assistant", Content: "Structure created. Now adding the configuration file.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Configuration complete. Creating the main source file.", Timestamp: fixtureBaseTime.Add(9 * time.Second)},
					{Role: "assistant", Content: "I've set up your complete project workspace.", Timestamp: fixtureBaseTime.Add(11 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace"}, Timestamp: fixtureBaseTime.Add(2 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Empty directory"}}}},
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(4 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json", "content": "{\"name\": \"myproject\", \"version\": \"1.0.0\"}"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go", "content": "package main\n\nfunc main() {}"}, Timestamp: fixtureBaseTime.Add(10 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md", "content": "# My Project\n\nA sample project."}, Timestamp: fixtureBaseTime.Add(11500 * time.Millisecond), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
				},
				FinalOutput: "I've successfully set up your project workspace with config.json, src/main.go, and README.md.",
				TokensUsed:  589,
				LatencyMs:   12000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called"},
				{Type: "tool_param_equals", Passed: true, Message: "Parameters match expected values"},
			},
			Passed: true,
		},
		// GPT agent - fails (wrong file content, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "gpt-agent",
				ProviderType: model.ProviderAzure,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(9500 * time.Millisecond),
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace with config, src folder, and initial files", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "Setting up the project now.", Timestamp: fixtureBaseTime.Add(500 * time.Millisecond)},
					{Role: "assistant", Content: "Creating directory structure.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Adding config file.", Timestamp: fixtureBaseTime.Add(4 * time.Second)},
					{Role: "assistant", Content: "Creating source files.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Project setup complete.", Timestamp: fixtureBaseTime.Add(9 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(1 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json", "content": "{}"}, Timestamp: fixtureBaseTime.Add(3 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go", "content": "package main"}, Timestamp: fixtureBaseTime.Add(5 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/readme.txt", "content": "My Project"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(9 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "config.json, readme.txt, src/"}}}},
				},
				FinalOutput: "Project created.",
				TokensUsed:  334,
				LatencyMs:   9500,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called"},
				{Type: "tool_param_equals", Passed: false, Message: "Parameter mismatch: expected README.md, got readme.txt", Details: map[string]interface{}{
					"expected": "README.md",
					"actual":   "readme.txt",
				}},
			},
			Passed: false,
		},
		// Mistral agent - DISQUALIFIED (fails all tests, 0% success rate)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "mistral-agent",
				ProviderType: model.ProviderOpenAI, // Using OpenAI as placeholder
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(25000 * time.Millisecond),
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace with config, src folder, and initial files", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I will try to set up the workspace.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Attempting to create directory...", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Error encountered, retrying...", Timestamp: fixtureBaseTime.Add(12 * time.Second)},
					{Role: "assistant", Content: "Still having issues with permissions.", Timestamp: fixtureBaseTime.Add(18 * time.Second)},
					{Role: "assistant", Content: "I was unable to complete the task.", Timestamp: fixtureBaseTime.Add(24 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(4 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Permission denied"}}}},
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(8 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Permission denied"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "sudo mkdir /workspace/myproject"}, Timestamp: fixtureBaseTime.Add(14 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: sudo not allowed"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/myproject/config.json"}, Timestamp: fixtureBaseTime.Add(20 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Directory does not exist"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace"}, Timestamp: fixtureBaseTime.Add(23 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Empty"}}}},
				},
				FinalOutput: "I was unable to set up the project workspace due to permission errors.",
				TokensUsed:  890,
				LatencyMs:   25000,
				Errors:      []string{"Permission denied", "sudo not allowed", "Directory does not exist"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "tool_called", Passed: false, Message: "Tool 'write_file' was not called with correct path"},
				{Type: "tool_param_equals", Passed: false, Message: "No files were created in the expected location", Details: map[string]interface{}{
					"expected": "/workspace/myproject",
					"actual":   "/tmp/myproject",
				}},
				{Type: "no_error_messages", Passed: false, Message: "Multiple errors encountered"},
			},
			Passed: false,
		},
	}
}

// createMultiSessionRun returns a scenario with multiple sessions, each test has 5 steps
// Builds on multi-agent: uses gemini-agent from previous tests, but adds session concept
func createMultiSessionRun() []model.TestRun {
	return []model.TestRun{
		// Session 1: Project Setup (same as single agent, now in a session context)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(8500 * time.Millisecond),
				SessionName:  "Project Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll set up the project workspace.", Timestamp: fixtureBaseTime.Add(500 * time.Millisecond)},
					{Role: "assistant", Content: "Directory created.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Adding configuration.", Timestamp: fixtureBaseTime.Add(4 * time.Second)},
					{Role: "assistant", Content: "Creating source file.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Workspace is ready.", Timestamp: fixtureBaseTime.Add(8 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(1 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json"}, Timestamp: fixtureBaseTime.Add(3 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go"}, Timestamp: fixtureBaseTime.Add(5 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(8 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "config.json, README.md, src/"}}}},
				},
				FinalOutput: "Project workspace created successfully.",
				TokensUsed:  456,
				LatencyMs:   8500,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "tool_called", Passed: true, Message: "Tool 'write_file' was called"},
			},
			Passed: true,
		},
		// Session 1: Add dependencies (continues project setup session)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Add project dependencies",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(10 * time.Second),
				EndTime:      fixtureBaseTime.Add(18 * time.Second),
				SessionName:  "Project Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Add go module and install dependencies", Timestamp: fixtureBaseTime.Add(10 * time.Second)},
					{Role: "assistant", Content: "I'll initialize the go module.", Timestamp: fixtureBaseTime.Add(11 * time.Second)},
					{Role: "assistant", Content: "Module initialized. Adding dependencies.", Timestamp: fixtureBaseTime.Add(13 * time.Second)},
					{Role: "assistant", Content: "Dependencies added. Tidying up.", Timestamp: fixtureBaseTime.Add(15 * time.Second)},
					{Role: "assistant", Content: "Running go mod tidy.", Timestamp: fixtureBaseTime.Add(16 * time.Second)},
					{Role: "assistant", Content: "Dependencies are ready.", Timestamp: fixtureBaseTime.Add(17 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go mod init myproject"}, Timestamp: fixtureBaseTime.Add(12 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "go.mod created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/go.mod"}, Timestamp: fixtureBaseTime.Add(13 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File updated"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get github.com/gin-gonic/gin"}, Timestamp: fixtureBaseTime.Add(14 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Package added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get github.com/stretchr/testify"}, Timestamp: fixtureBaseTime.Add(15 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Package added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go mod tidy"}, Timestamp: fixtureBaseTime.Add(17 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Tidied"}}}},
				},
				FinalOutput: "Go module initialized and dependencies installed.",
				TokensUsed:  320,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'run_command' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'dependencies'"},
			},
			Passed: true,
		},
		// Session 2: Database Setup
		{
			Execution: &model.ExecutionResult{
				TestName:     "Configure database connection",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(20 * time.Second),
				EndTime:      fixtureBaseTime.Add(28 * time.Second),
				SessionName:  "Database Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Set up database configuration", Timestamp: fixtureBaseTime.Add(20 * time.Second)},
					{Role: "assistant", Content: "I'll configure the database connection.", Timestamp: fixtureBaseTime.Add(21 * time.Second)},
					{Role: "assistant", Content: "Checking connection parameters.", Timestamp: fixtureBaseTime.Add(23 * time.Second)},
					{Role: "assistant", Content: "Creating db config file.", Timestamp: fixtureBaseTime.Add(25 * time.Second)},
					{Role: "assistant", Content: "Testing connection.", Timestamp: fixtureBaseTime.Add(26 * time.Second)},
					{Role: "assistant", Content: "Database configured.", Timestamp: fixtureBaseTime.Add(27 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "read_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json"}, Timestamp: fixtureBaseTime.Add(22 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "{\"name\": \"myproject\"}"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/db/config.go"}, Timestamp: fixtureBaseTime.Add(24 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/db/connection.go"}, Timestamp: fixtureBaseTime.Add(25 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go build ./db/..."}, Timestamp: fixtureBaseTime.Add(26 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Build successful"}}}},
					{Name: "db_connect", Parameters: map[string]interface{}{"host": "localhost", "port": 5432}, Timestamp: fixtureBaseTime.Add(27 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Connected"}}}},
				},
				FinalOutput: "Database configuration complete and connection verified.",
				TokensUsed:  380,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'db_connect' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'complete'"},
			},
			Passed: true,
		},
		// Session 2: Create tables
		{
			Execution: &model.ExecutionResult{
				TestName:     "Create database tables",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(30 * time.Second),
				EndTime:      fixtureBaseTime.Add(38 * time.Second),
				SessionName:  "Database Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Create the users and orders tables", Timestamp: fixtureBaseTime.Add(30 * time.Second)},
					{Role: "assistant", Content: "I'll create the database tables.", Timestamp: fixtureBaseTime.Add(31 * time.Second)},
					{Role: "assistant", Content: "Creating users table.", Timestamp: fixtureBaseTime.Add(33 * time.Second)},
					{Role: "assistant", Content: "Creating orders table.", Timestamp: fixtureBaseTime.Add(35 * time.Second)},
					{Role: "assistant", Content: "Adding foreign keys.", Timestamp: fixtureBaseTime.Add(36 * time.Second)},
					{Role: "assistant", Content: "Tables created successfully.", Timestamp: fixtureBaseTime.Add(37 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "query_database", Parameters: map[string]interface{}{"query": "CREATE TABLE users (id SERIAL PRIMARY KEY, name VARCHAR(255))"}, Timestamp: fixtureBaseTime.Add(32 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Table created"}}}},
					{Name: "query_database", Parameters: map[string]interface{}{"query": "CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INT)"}, Timestamp: fixtureBaseTime.Add(34 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Table created"}}}},
					{Name: "query_database", Parameters: map[string]interface{}{"query": "ALTER TABLE orders ADD FOREIGN KEY (user_id) REFERENCES users(id)"}, Timestamp: fixtureBaseTime.Add(35 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Constraint added"}}}},
					{Name: "query_database", Parameters: map[string]interface{}{"query": "CREATE INDEX idx_orders_user ON orders(user_id)"}, Timestamp: fixtureBaseTime.Add(36 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Index created"}}}},
					{Name: "query_database", Parameters: map[string]interface{}{"query": "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"}, Timestamp: fixtureBaseTime.Add(37 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "users, orders"}}}},
				},
				FinalOutput: "Created users and orders tables with proper relationships.",
				TokensUsed:  420,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'query_database' was called"},
				{Type: "output_regex", Passed: true, Message: "Output matches pattern 'users.*orders'"},
			},
			Passed: true,
		},
	}
}

// createSuiteRun returns a full suite run with multiple files, sessions, and agents (5 steps each)
// Builds on all previous: combines multi-agent + multi-session concepts
func createSuiteRun() []model.TestRun {
	return []model.TestRun{
		// File 1: project-setup.yaml - Project Setup session (gemini-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(8500 * time.Millisecond),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Project Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "Setting up project workspace.", Timestamp: fixtureBaseTime.Add(500 * time.Millisecond)},
					{Role: "assistant", Content: "Directory created.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Adding config.", Timestamp: fixtureBaseTime.Add(4 * time.Second)},
					{Role: "assistant", Content: "Creating source.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Done.", Timestamp: fixtureBaseTime.Add(8 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(1 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json"}, Timestamp: fixtureBaseTime.Add(3 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go"}, Timestamp: fixtureBaseTime.Add(5 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(8 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "config.json, README.md, src/"}}}},
				},
				FinalOutput: "Project workspace created.",
				TokensUsed:  456,
				LatencyMs:   8500,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "no_error_messages", Passed: true, Message: "No errors encountered"},
			},
			Passed: true,
		},
		// File 1: project-setup.yaml - Project Setup session (claude-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "claude-agent",
				ProviderType: model.ProviderAnthropic,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(12000 * time.Millisecond),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Project Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll set up the project.", Timestamp: fixtureBaseTime.Add(1 * time.Second)},
					{Role: "assistant", Content: "Checking path.", Timestamp: fixtureBaseTime.Add(3 * time.Second)},
					{Role: "assistant", Content: "Creating structure.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Adding files.", Timestamp: fixtureBaseTime.Add(9 * time.Second)},
					{Role: "assistant", Content: "Workspace ready.", Timestamp: fixtureBaseTime.Add(11 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace"}, Timestamp: fixtureBaseTime.Add(2 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Empty"}}}},
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/workspace/myproject/src"}, Timestamp: fixtureBaseTime.Add(5 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Directory created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/config.json"}, Timestamp: fixtureBaseTime.Add(8 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/src/main.go"}, Timestamp: fixtureBaseTime.Add(10 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/README.md"}, Timestamp: fixtureBaseTime.Add(11500 * time.Millisecond), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "File created"}}}},
				},
				FinalOutput: "I've set up the project workspace.",
				TokensUsed:  589,
				LatencyMs:   12000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'create_directory' was called"},
				{Type: "no_error_messages", Passed: true, Message: "No errors encountered"},
			},
			Passed: true,
		},
		// File 1: project-setup.yaml - Dependencies session (gemini-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Add project dependencies",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(15 * time.Second),
				EndTime:      fixtureBaseTime.Add(23 * time.Second),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Dependencies",
				Messages: []model.Message{
					{Role: "user", Content: "Add dependencies", Timestamp: fixtureBaseTime.Add(15 * time.Second)},
					{Role: "assistant", Content: "Initializing module.", Timestamp: fixtureBaseTime.Add(16 * time.Second)},
					{Role: "assistant", Content: "Adding gin.", Timestamp: fixtureBaseTime.Add(18 * time.Second)},
					{Role: "assistant", Content: "Adding testify.", Timestamp: fixtureBaseTime.Add(20 * time.Second)},
					{Role: "assistant", Content: "Running tidy.", Timestamp: fixtureBaseTime.Add(21 * time.Second)},
					{Role: "assistant", Content: "Dependencies ready.", Timestamp: fixtureBaseTime.Add(22 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go mod init"}, Timestamp: fixtureBaseTime.Add(17 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "go.mod created"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get gin"}, Timestamp: fixtureBaseTime.Add(18 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get testify"}, Timestamp: fixtureBaseTime.Add(19 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get gorm"}, Timestamp: fixtureBaseTime.Add(20 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go mod tidy"}, Timestamp: fixtureBaseTime.Add(22 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Tidied"}}}},
				},
				FinalOutput: "Dependencies installed.",
				TokensUsed:  289,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'run_command' was called"},
			},
			Passed: true,
		},
		// File 1: project-setup.yaml - Dependencies session (claude-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Add project dependencies",
				AgentName:    "claude-agent",
				ProviderType: model.ProviderAnthropic,
				StartTime:    fixtureBaseTime.Add(15 * time.Second),
				EndTime:      fixtureBaseTime.Add(25 * time.Second),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Dependencies",
				Messages: []model.Message{
					{Role: "user", Content: "Add dependencies", Timestamp: fixtureBaseTime.Add(15 * time.Second)},
					{Role: "assistant", Content: "I'll add the dependencies.", Timestamp: fixtureBaseTime.Add(16 * time.Second)},
					{Role: "assistant", Content: "Reading go.mod.", Timestamp: fixtureBaseTime.Add(18 * time.Second)},
					{Role: "assistant", Content: "Adding packages.", Timestamp: fixtureBaseTime.Add(20 * time.Second)},
					{Role: "assistant", Content: "Verifying.", Timestamp: fixtureBaseTime.Add(23 * time.Second)},
					{Role: "assistant", Content: "Done.", Timestamp: fixtureBaseTime.Add(24 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "read_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/go.mod"}, Timestamp: fixtureBaseTime.Add(17 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "module myproject"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get gin"}, Timestamp: fixtureBaseTime.Add(19 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go get testify"}, Timestamp: fixtureBaseTime.Add(21 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Added"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "go mod tidy"}, Timestamp: fixtureBaseTime.Add(22 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Tidied"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/workspace/myproject"}, Timestamp: fixtureBaseTime.Add(24 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "go.mod, go.sum, src/"}}}},
				},
				FinalOutput: "Dependencies are set up.",
				TokensUsed:  378,
				LatencyMs:   10000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'run_command' was called"},
			},
			Passed: true,
		},
		// File 2: api-tests.yaml - API Operations session (gemini-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Fetch user profile",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(30 * time.Second),
				EndTime:      fixtureBaseTime.Add(38 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "User API",
				Messages: []model.Message{
					{Role: "user", Content: "Get user profile for user 123", Timestamp: fixtureBaseTime.Add(30 * time.Second)},
					{Role: "assistant", Content: "Fetching user.", Timestamp: fixtureBaseTime.Add(31 * time.Second)},
					{Role: "assistant", Content: "Checking auth.", Timestamp: fixtureBaseTime.Add(33 * time.Second)},
					{Role: "assistant", Content: "Making request.", Timestamp: fixtureBaseTime.Add(35 * time.Second)},
					{Role: "assistant", Content: "Parsing response.", Timestamp: fixtureBaseTime.Add(36 * time.Second)},
					{Role: "assistant", Content: "Done.", Timestamp: fixtureBaseTime.Add(37 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "read_file", Parameters: map[string]interface{}{"path": "/workspace/myproject/.env"}, Timestamp: fixtureBaseTime.Add(32 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "API_KEY=xxx"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/auth/token"}, Timestamp: fixtureBaseTime.Add(33 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "token: abc123"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/123"}, Timestamp: fixtureBaseTime.Add(35 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "{\"name\": \"John Doe\"}"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/user_123.json"}, Timestamp: fixtureBaseTime.Add(36 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Saved"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/123/preferences"}, Timestamp: fixtureBaseTime.Add(37 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "{\"theme\": \"dark\"}"}}}},
				},
				FinalOutput: "User profile: John Doe, john@example.com",
				TokensUsed:  434,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'http_get' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'John Doe'"},
				{Type: "max_latency_ms", Passed: true, Message: "Latency 8000ms < 10000ms"},
			},
			Passed: true,
		},
		// File 2: api-tests.yaml - API Operations session (claude-agent, 5 steps - FAILS latency)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Fetch user profile",
				AgentName:    "claude-agent",
				ProviderType: model.ProviderAnthropic,
				StartTime:    fixtureBaseTime.Add(30 * time.Second),
				EndTime:      fixtureBaseTime.Add(42 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "User API",
				Messages: []model.Message{
					{Role: "user", Content: "Get user profile for user 123", Timestamp: fixtureBaseTime.Add(30 * time.Second)},
					{Role: "assistant", Content: "I'll fetch the user profile.", Timestamp: fixtureBaseTime.Add(32 * time.Second)},
					{Role: "assistant", Content: "Getting auth token.", Timestamp: fixtureBaseTime.Add(35 * time.Second)},
					{Role: "assistant", Content: "Fetching profile.", Timestamp: fixtureBaseTime.Add(38 * time.Second)},
					{Role: "assistant", Content: "Getting preferences.", Timestamp: fixtureBaseTime.Add(40 * time.Second)},
					{Role: "assistant", Content: "Profile retrieved.", Timestamp: fixtureBaseTime.Add(41 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/health"}, Timestamp: fixtureBaseTime.Add(33 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "OK"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/auth/token"}, Timestamp: fixtureBaseTime.Add(36 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "token: abc123"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/123"}, Timestamp: fixtureBaseTime.Add(38 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "{\"name\": \"John Doe\"}"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/123/preferences"}, Timestamp: fixtureBaseTime.Add(40 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "{\"theme\": \"dark\"}"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/123/avatar"}, Timestamp: fixtureBaseTime.Add(41 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "https://avatar.url"}}}},
				},
				FinalOutput: "The user profile shows: Name: John Doe, Email: john@example.com",
				TokensUsed:  489,
				LatencyMs:   12000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'http_get' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'John Doe'"},
				{Type: "max_latency_ms", Passed: false, Message: "Latency 12000ms > 10000ms", Details: map[string]interface{}{
					"actual": 12000,
					"max":    10000,
				}},
			},
			Passed: false,
		},
		// File 2: api-tests.yaml - Error handling (gemini-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Handle 404 gracefully",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime.Add(45 * time.Second),
				EndTime:      fixtureBaseTime.Add(53 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Error Handling",
				Messages: []model.Message{
					{Role: "user", Content: "Try to get non-existent user 99999", Timestamp: fixtureBaseTime.Add(45 * time.Second)},
					{Role: "assistant", Content: "Fetching user.", Timestamp: fixtureBaseTime.Add(46 * time.Second)},
					{Role: "assistant", Content: "Got 404.", Timestamp: fixtureBaseTime.Add(48 * time.Second)},
					{Role: "assistant", Content: "Checking if deleted.", Timestamp: fixtureBaseTime.Add(50 * time.Second)},
					{Role: "assistant", Content: "Logging error.", Timestamp: fixtureBaseTime.Add(51 * time.Second)},
					{Role: "assistant", Content: "User not found.", Timestamp: fixtureBaseTime.Add(52 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999"}, Timestamp: fixtureBaseTime.Add(47 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999/history"}, Timestamp: fixtureBaseTime.Add(49 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "query_database", Parameters: map[string]interface{}{"query": "SELECT deleted_at FROM users WHERE id=99999"}, Timestamp: fixtureBaseTime.Add(50 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "No rows"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/errors.log", "content": "User 99999 not found"}, Timestamp: fixtureBaseTime.Add(51 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Logged"}}}},
					{Name: "http_post", Parameters: map[string]interface{}{"url": "/api/audit/log"}, Timestamp: fixtureBaseTime.Add(52 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Audit logged"}}}},
				},
				FinalOutput: "User not found. The requested user ID 99999 does not exist.",
				TokensUsed:  345,
				LatencyMs:   8000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'http_get' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'not found'"},
				{Type: "no_error_messages", Passed: true, Message: "No errors encountered"},
			},
			Passed: true,
		},
		// File 2: api-tests.yaml - Error handling (claude-agent, 5 steps)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Handle 404 gracefully",
				AgentName:    "claude-agent",
				ProviderType: model.ProviderAnthropic,
				StartTime:    fixtureBaseTime.Add(45 * time.Second),
				EndTime:      fixtureBaseTime.Add(54 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Error Handling",
				Messages: []model.Message{
					{Role: "user", Content: "Try to get non-existent user 99999", Timestamp: fixtureBaseTime.Add(45 * time.Second)},
					{Role: "assistant", Content: "I'll try to fetch that user.", Timestamp: fixtureBaseTime.Add(46 * time.Second)},
					{Role: "assistant", Content: "Received a 404 error.", Timestamp: fixtureBaseTime.Add(49 * time.Second)},
					{Role: "assistant", Content: "Checking related data.", Timestamp: fixtureBaseTime.Add(51 * time.Second)},
					{Role: "assistant", Content: "Verifying status.", Timestamp: fixtureBaseTime.Add(52 * time.Second)},
					{Role: "assistant", Content: "User doesn't exist.", Timestamp: fixtureBaseTime.Add(53 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999"}, Timestamp: fixtureBaseTime.Add(48 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999/profile"}, Timestamp: fixtureBaseTime.Add(50 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999/settings"}, Timestamp: fixtureBaseTime.Add(51 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/search?id=99999"}, Timestamp: fixtureBaseTime.Add(52 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "[]"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/result.json"}, Timestamp: fixtureBaseTime.Add(53 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Saved"}}}},
				},
				FinalOutput: "I received a 404 error. The user with ID 99999 was not found in the system.",
				TokensUsed:  398,
				LatencyMs:   9000,
				Errors:      []string{},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'http_get' was called"},
				{Type: "output_contains", Passed: true, Message: "Output contains 'not found'"},
				{Type: "no_error_messages", Passed: true, Message: "No errors encountered"},
			},
			Passed: true,
		},
		// DISQUALIFIED AGENT: mistral-agent fails all tests
		// File 1: project-setup.yaml - Project Setup session (mistral-agent - FAILS)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Setup project workspace",
				AgentName:    "mistral-agent",
				ProviderType: model.ProviderOpenAI,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(20000 * time.Millisecond),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Project Setup",
				Messages: []model.Message{
					{Role: "user", Content: "Set up a new project workspace", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I will attempt to set up the workspace.", Timestamp: fixtureBaseTime.Add(2 * time.Second)},
					{Role: "assistant", Content: "Creating directory...", Timestamp: fixtureBaseTime.Add(5 * time.Second)},
					{Role: "assistant", Content: "Error encountered.", Timestamp: fixtureBaseTime.Add(10 * time.Second)},
					{Role: "assistant", Content: "Retrying...", Timestamp: fixtureBaseTime.Add(15 * time.Second)},
					{Role: "assistant", Content: "Failed to complete.", Timestamp: fixtureBaseTime.Add(19 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/wrong/path"}, Timestamp: fixtureBaseTime.Add(3 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Invalid path"}}}},
					{Name: "create_directory", Parameters: map[string]interface{}{"path": "/wrong/path"}, Timestamp: fixtureBaseTime.Add(7 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Invalid path"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "mkdir -p /wrong"}, Timestamp: fixtureBaseTime.Add(12 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Permission denied"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/test.txt"}, Timestamp: fixtureBaseTime.Add(16 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Wrong location"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/wrong"}, Timestamp: fixtureBaseTime.Add(18 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Not found"}}}},
				},
				FinalOutput: "Failed to set up workspace.",
				TokensUsed:  678,
				LatencyMs:   20000,
				Errors:      []string{"Invalid path", "Permission denied"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: false, Message: "Tool called with wrong parameters"},
				{Type: "no_error_messages", Passed: false, Message: "Errors encountered"},
			},
			Passed: false,
		},
		// File 1: project-setup.yaml - Dependencies session (mistral-agent - FAILS)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Add project dependencies",
				AgentName:    "mistral-agent",
				ProviderType: model.ProviderOpenAI,
				StartTime:    fixtureBaseTime.Add(15 * time.Second),
				EndTime:      fixtureBaseTime.Add(30 * time.Second),
				SourceFile:   "tests/project-setup.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Dependencies",
				Messages: []model.Message{
					{Role: "user", Content: "Add dependencies", Timestamp: fixtureBaseTime.Add(15 * time.Second)},
					{Role: "assistant", Content: "Adding dependencies.", Timestamp: fixtureBaseTime.Add(17 * time.Second)},
					{Role: "assistant", Content: "Running npm instead of go.", Timestamp: fixtureBaseTime.Add(20 * time.Second)},
					{Role: "assistant", Content: "Command failed.", Timestamp: fixtureBaseTime.Add(24 * time.Second)},
					{Role: "assistant", Content: "Trying pip.", Timestamp: fixtureBaseTime.Add(27 * time.Second)},
					{Role: "assistant", Content: "Unable to complete.", Timestamp: fixtureBaseTime.Add(29 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "npm init"}, Timestamp: fixtureBaseTime.Add(18 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Not a Node.js project"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "npm install"}, Timestamp: fixtureBaseTime.Add(21 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: package.json not found"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "pip install"}, Timestamp: fixtureBaseTime.Add(25 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Not a Python project"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/deps.txt"}, Timestamp: fixtureBaseTime.Add(27 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Written to wrong location"}}}},
					{Name: "list_files", Parameters: map[string]interface{}{"path": "/tmp"}, Timestamp: fixtureBaseTime.Add(28 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "deps.txt"}}}},
				},
				FinalOutput: "Could not add dependencies.",
				TokensUsed:  534,
				LatencyMs:   15000,
				Errors:      []string{"Wrong package manager", "Project type mismatch"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: false, Message: "Used npm instead of go"},
				{Type: "no_error_messages", Passed: false, Message: "Multiple errors"},
			},
			Passed: false,
		},
		// File 2: api-tests.yaml - User API (mistral-agent - FAILS)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Fetch user profile",
				AgentName:    "mistral-agent",
				ProviderType: model.ProviderOpenAI,
				StartTime:    fixtureBaseTime.Add(30 * time.Second),
				EndTime:      fixtureBaseTime.Add(45 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "User API",
				Messages: []model.Message{
					{Role: "user", Content: "Get user profile for user 123", Timestamp: fixtureBaseTime.Add(30 * time.Second)},
					{Role: "assistant", Content: "Getting user.", Timestamp: fixtureBaseTime.Add(32 * time.Second)},
					{Role: "assistant", Content: "Wrong endpoint.", Timestamp: fixtureBaseTime.Add(36 * time.Second)},
					{Role: "assistant", Content: "Trying another.", Timestamp: fixtureBaseTime.Add(40 * time.Second)},
					{Role: "assistant", Content: "Still failing.", Timestamp: fixtureBaseTime.Add(43 * time.Second)},
					{Role: "assistant", Content: "Cannot fetch user.", Timestamp: fixtureBaseTime.Add(44 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/wrong/endpoint"}, Timestamp: fixtureBaseTime.Add(34 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/v2/users"}, Timestamp: fixtureBaseTime.Add(37 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "API version not supported"}}}},
					{Name: "http_post", Parameters: map[string]interface{}{"url": "/api/users"}, Timestamp: fixtureBaseTime.Add(39 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Method not allowed"}}}},
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/users/123"}, Timestamp: fixtureBaseTime.Add(42 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Missing /api prefix"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/error.log"}, Timestamp: fixtureBaseTime.Add(44 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Logged"}}}},
				},
				FinalOutput: "Failed to fetch user profile.",
				TokensUsed:  612,
				LatencyMs:   15000,
				Errors:      []string{"Wrong endpoint", "API version error", "Method not allowed"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: false, Message: "Wrong API endpoint used"},
				{Type: "output_contains", Passed: false, Message: "Output does not contain user data"},
				{Type: "max_latency_ms", Passed: false, Message: "Latency 15000ms > 10000ms"},
			},
			Passed: false,
		},
		// File 2: api-tests.yaml - Error handling (mistral-agent - FAILS)
		{
			Execution: &model.ExecutionResult{
				TestName:     "Handle 404 gracefully",
				AgentName:    "mistral-agent",
				ProviderType: model.ProviderOpenAI,
				StartTime:    fixtureBaseTime.Add(45 * time.Second),
				EndTime:      fixtureBaseTime.Add(60 * time.Second),
				SourceFile:   "tests/api-tests.yaml",
				SuiteName:    "Integration Suite",
				SessionName:  "Error Handling",
				Messages: []model.Message{
					{Role: "user", Content: "Try to get non-existent user 99999", Timestamp: fixtureBaseTime.Add(45 * time.Second)},
					{Role: "assistant", Content: "Fetching user.", Timestamp: fixtureBaseTime.Add(47 * time.Second)},
					{Role: "assistant", Content: "Got error.", Timestamp: fixtureBaseTime.Add(50 * time.Second)},
					{Role: "assistant", Content: "Throwing exception.", Timestamp: fixtureBaseTime.Add(53 * time.Second)},
					{Role: "assistant", Content: "Crash.", Timestamp: fixtureBaseTime.Add(57 * time.Second)},
					{Role: "assistant", Content: "Unhandled error.", Timestamp: fixtureBaseTime.Add(59 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{Name: "http_get", Parameters: map[string]interface{}{"url": "/api/users/99999"}, Timestamp: fixtureBaseTime.Add(48 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "404 Not Found"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "throw Error"}, Timestamp: fixtureBaseTime.Add(51 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Invalid command"}}}},
					{Name: "http_delete", Parameters: map[string]interface{}{"url": "/api/users/99999"}, Timestamp: fixtureBaseTime.Add(54 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Not authorized"}}}},
					{Name: "run_command", Parameters: map[string]interface{}{"cmd": "exit 1"}, Timestamp: fixtureBaseTime.Add(56 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Process exited"}}}},
					{Name: "write_file", Parameters: map[string]interface{}{"path": "/tmp/crash.log"}, Timestamp: fixtureBaseTime.Add(58 * time.Second), Result: model.Result{Content: []model.ContentItem{{Type: "text", Text: "Crash logged"}}}},
				},
				FinalOutput: "Error: Unhandled exception occurred.",
				TokensUsed:  489,
				LatencyMs:   15000,
				Errors:      []string{"Unhandled exception", "Invalid command", "Not authorized"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'http_get' was called"},
				{Type: "output_contains", Passed: false, Message: "Output should indicate graceful handling"},
				{Type: "no_error_messages", Passed: false, Message: "Should not throw exceptions"},
			},
			Passed: false,
		},
	}
}

// createFailedTestWithErrors returns a test run with actual errors (5 steps)
func createFailedTestWithErrors() []model.TestRun {
	return []model.TestRun{
		{
			Execution: &model.ExecutionResult{
				TestName:     "Connect to database",
				AgentName:    "gemini-agent",
				ProviderType: model.ProviderGoogle,
				StartTime:    fixtureBaseTime,
				EndTime:      fixtureBaseTime.Add(15 * time.Second),
				Messages: []model.Message{
					{Role: "user", Content: "Connect to the production database and verify the connection", Timestamp: fixtureBaseTime},
					{Role: "assistant", Content: "I'll connect to the production database.", Timestamp: fixtureBaseTime.Add(1 * time.Second)},
					{Role: "assistant", Content: "Reading connection config.", Timestamp: fixtureBaseTime.Add(3 * time.Second)},
					{Role: "assistant", Content: "Attempting connection.", Timestamp: fixtureBaseTime.Add(6 * time.Second)},
					{Role: "assistant", Content: "Connection failed, retrying.", Timestamp: fixtureBaseTime.Add(10 * time.Second)},
					{Role: "assistant", Content: "Unable to establish connection.", Timestamp: fixtureBaseTime.Add(14 * time.Second)},
				},
				ToolCalls: []model.ToolCall{
					{
						Name:       "read_file",
						Parameters: map[string]interface{}{"path": "/etc/db/config.yaml"},
						Timestamp:  fixtureBaseTime.Add(2 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "host: prod-db.example.com\nport: 5432"}}},
					},
					{
						Name:       "run_command",
						Parameters: map[string]interface{}{"cmd": "ping prod-db.example.com"},
						Timestamp:  fixtureBaseTime.Add(4 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Host unreachable"}}},
					},
					{
						Name:       "db_connect",
						Parameters: map[string]interface{}{"host": "prod-db.example.com", "port": 5432},
						Timestamp:  fixtureBaseTime.Add(7 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Connection refused"}}},
					},
					{
						Name:       "db_connect",
						Parameters: map[string]interface{}{"host": "prod-db.example.com", "port": 5432, "retry": true},
						Timestamp:  fixtureBaseTime.Add(11 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error: Connection timed out"}}},
					},
					{
						Name:       "write_file",
						Parameters: map[string]interface{}{"path": "/var/log/db_errors.log", "content": "Connection failed to prod-db.example.com"},
						Timestamp:  fixtureBaseTime.Add(14 * time.Second),
						Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Error logged"}}},
					},
				},
				FinalOutput: "I was unable to connect to the database. The connection was refused after multiple attempts.",
				TokensUsed:  410,
				LatencyMs:   15000,
				Errors:      []string{"Connection refused: prod-db.example.com:5432", "Connection timed out after 5s", "Host unreachable"},
			},
			Assertions: []model.AssertionResult{
				{Type: "tool_called", Passed: true, Message: "Tool 'db_connect' was called"},
				{Type: "no_error_messages", Passed: false, Message: "Errors encountered: Connection refused, Timeout, Host unreachable"},
			},
			Passed: false,
		},
	}
}

// createLargeScaleRun returns a larger dataset for performance/stress testing (5 steps per agent)
func createLargeScaleRun() []model.TestRun {
	agents := []struct {
		name     string
		provider model.ProviderType
	}{
		{"gemini-flash", model.ProviderGoogle},
		{"gemini-pro", model.ProviderVertex},
		{"claude-sonnet", model.ProviderAnthropic},
		{"gpt-4o", model.ProviderAzure},
		{"gpt-4o-mini", model.ProviderOpenAI},
	}

	tests := []string{
		"Create user",
		"Update user profile",
		"Delete user",
		"List all users",
		"Search users by email",
		"Reset password",
		"Verify email",
		"Generate API key",
		"Revoke API key",
		"Export user data",
	}

	// 5 steps per test
	stepNames := []string{"validate_input", "check_permissions", "execute_action", "log_audit", "send_notification"}

	var results []model.TestRun
	for testIdx, testName := range tests {
		for agentIdx, agent := range agents {
			startTime := fixtureBaseTime.Add(time.Duration(testIdx*10+agentIdx) * time.Second)
			duration := time.Duration(8000+agentIdx*500) * time.Millisecond
			passed := (testIdx+agentIdx)%7 != 0 // ~14% failure rate

			// Generate 5 tool calls for each test
			toolCalls := make([]model.ToolCall, 5)
			messages := make([]model.Message, 6)
			messages[0] = model.Message{Role: "user", Content: testName, Timestamp: startTime}

			for step := 0; step < 5; step++ {
				stepTime := startTime.Add(time.Duration(step+1) * time.Second)
				toolCalls[step] = model.ToolCall{
					Name:       stepNames[step],
					Parameters: map[string]interface{}{"action": testName, "step": step + 1},
					Timestamp:  stepTime,
					Result:     model.Result{Content: []model.ContentItem{{Type: "text", Text: "Step completed"}}},
				}
				messages[step+1] = model.Message{
					Role:      "assistant",
					Content:   "Executing " + stepNames[step],
					Timestamp: stepTime.Add(100 * time.Millisecond),
				}
			}

			results = append(results, model.TestRun{
				Execution: &model.ExecutionResult{
					TestName:     testName,
					AgentName:    agent.name,
					ProviderType: agent.provider,
					StartTime:    startTime,
					EndTime:      startTime.Add(duration),
					SourceFile:   "tests/user-management.yaml",
					SuiteName:    "User Management Suite",
					SessionName:  "CRUD Operations",
					Messages:     messages,
					ToolCalls:    toolCalls,
					FinalOutput:  testName + " completed successfully.",
					TokensUsed:   200 + testIdx*20 + agentIdx*10,
					LatencyMs:    int64(duration.Milliseconds()),
					Errors:       []string{},
				},
				Assertions: []model.AssertionResult{
					{Type: "tool_called", Passed: true, Message: "Tool 'validate_input' was called"},
					{Type: "tool_called", Passed: true, Message: "Tool 'execute_action' was called"},
					{Type: "no_error_messages", Passed: passed, Message: func() string {
						if passed {
							return "No errors"
						}
						return "Simulated failure"
					}()},
				},
				Passed: passed,
			})
		}
	}
	return results
}
