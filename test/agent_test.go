package tests

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tmc/langchaingo/llms"
)

func TestNewMCPAgent(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{"test_tool_1"},
		},
	}

	agent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	assert.NotNil(t, agent)
	assert.Equal(t, "test_agent", agent.Name)
	assert.Equal(t, "test_provider", agent.Provider)
	assert.Equal(t, 1, len(agent.McpServers))
	assert.Equal(t, 1, len(agent.MCPServerTools["test_server"]))
	assert.Equal(t, "test_tool_1", agent.MCPServerTools["test_server"][0].Name)
	assert.Contains(t, agent.AvailableTools, "test_tool_1")

	mockClient.AssertExpectations(t)
}

func TestNewMCPAgent_NoToolRestrictions(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{}, // Empty = all tools allowed
		},
	}

	agent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	assert.Equal(t, 2, len(agent.MCPServerTools["test_server"]))
	assert.Contains(t, agent.AvailableTools, "test_tool_1")
	assert.Contains(t, agent.AvailableTools, "test_tool_2")
}

func TestNewMCPAgent_ServerNotFound(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)

	agentServers := []model.AgentServer{
		{
			Name:         "nonexistent_server",
			AllowedTools: []string{},
		},
	}

	agent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{}, "test_provider", mockLLM)

	assert.NotNil(t, agent)
	assert.Equal(t, 0, len(agent.McpServers))
	assert.Equal(t, 0, len(agent.MCPServerTools))
}

func TestExecuteTool_Success(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	expectedResult := &mcp.CallToolResult{
		Result: mcp.Result{},
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Tool executed successfully",
			},
		},
		StructuredContent: nil,
		IsError:           false,
	}

	mockClient.On("CallTool", ctx, mock.MatchedBy(func(req mcp.CallToolRequest) bool {
		return req.Params.Name == "test_tool_1"
	})).Return(expectedResult, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{"test_tool_1"},
		},
	}

	agent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	arguments := `{"param1": "value1"}`
	result, err := agent.ExecuteTool(ctx, "test_tool_1", arguments)

	assert.NoError(t, err)
	assert.NotEmpty(t, result)

	var resultData mcp.CallToolResult
	err = json.Unmarshal([]byte(result), &resultData)
	assert.NoError(t, err)
	textContent := resultData.Content[0].(mcp.TextContent)
	assert.Equal(t, "text", textContent.Type)
	assert.Equal(t, "Tool executed successfully", textContent.Text)

	mockClient.AssertExpectations(t)
}

func TestExecuteTool_ToolNotFound(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)

	agent := &agent.MCPAgent{
		Name:           "test_agent",
		LLMModel:       mockLLM,
		ToolToServer:   make(map[string]string),
		MCPServerTools: make(map[string][]mcp.Tool),
	}

	_, err := agent.ExecuteTool(ctx, "nonexistent_tool", "{}")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool 'nonexistent_tool' not found")
}

func TestExecuteTool_InvalidJSON(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)

	agent := &agent.MCPAgent{
		Name:         "test_agent",
		LLMModel:     mockLLM,
		ToolToServer: map[string]string{"test_tool": "test_server"},
	}

	_, err := agent.ExecuteTool(ctx, "test_tool", "invalid json")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse arguments")
}

func TestExtractToolsFromAgent(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	agent := &agent.MCPAgent{
		MCPServerTools: map[string][]mcp.Tool{
			"server1": {
				{
					Name:        "tool1",
					Description: "First tool",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]interface{}{
							"param": map[string]interface{}{
								"type": "string",
							},
						},
						Required: []string{"param"},
					},
				},
			},
			"server2": {
				{
					Name:        "tool2",
					Description: "Second tool",
					InputSchema: mcp.ToolInputSchema{
						Type:       "object",
						Properties: map[string]interface{}{},
					},
				},
			},
		},
	}

	tools := agent.ExtractToolsFromAgent()

	assert.Equal(t, 2, len(tools))

	// Find tools by name instead of assuming order
	var tool1, tool2 *llms.Tool
	for i := range tools {
		if tools[i].Function.Name == "tool1" {
			tool1 = &tools[i]
		} else if tools[i].Function.Name == "tool2" {
			tool2 = &tools[i]
		}
	}

	assert.NotNil(t, tool1)
	assert.NotNil(t, tool2)
	assert.Equal(t, "First tool", tool1.Function.Description)

	// Check that required fields are preserved
	params := tool1.Function.Parameters.(map[string]interface{})
	required := params["required"].([]string)
	assert.Contains(t, required, "param")
}

func TestGenerateContentWithConfig_Success(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{"test_tool_1"},
		},
	}

	mcpAgent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	// Mock LLM response with no tool calls (final answer)
	mockLLM.On("GenerateContent", ctx, mock.Anything, mock.Anything).Return(&llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content:    "This is the final answer",
				StopReason: "stop",
				GenerationInfo: map[string]interface{}{
					"TotalTokens": 50,
				},
			},
		},
	}, nil)

	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: "Test question"},
			},
		},
	}

	config := agent.AgentConfig{
		MaxIterations: 5,
		Verbose:       false,
	}

	tools := mcpAgent.ExtractToolsFromAgent()
	result := mcpAgent.GenerateContentWithConfig(ctx, &msgs, config, tools)

	assert.Equal(t, "This is the final answer", result.FinalOutput)
	assert.Equal(t, 0, len(result.Errors))
	assert.Equal(t, 50, result.TokensUsed)

	mockLLM.AssertExpectations(t)
}

func TestGenerateContentWithConfig_WithToolCalls(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	toolResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Success",
			},
		},
	}

	mockClient.On("CallTool", ctx, mock.Anything).Return(toolResult, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{"test_tool_1"},
		},
	}

	mcpAgent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	// First call: LLM wants to use a tool
	mockLLM.On("GenerateContent", ctx, mock.Anything, mock.Anything).Return(&llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: "",
				ToolCalls: []llms.ToolCall{
					{
						ID: "call_1",
						FunctionCall: &llms.FunctionCall{
							Name:      "test_tool_1",
							Arguments: `{"param1": "test_value"}`,
						},
					},
				},
				GenerationInfo: map[string]interface{}{
					"TotalTokens": 30,
				},
			},
		},
	}, nil).Once()

	// Second call: LLM provides final answer
	mockLLM.On("GenerateContent", ctx, mock.Anything, mock.Anything).Return(&llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content:    "Final answer after using tool",
				StopReason: "stop",
				GenerationInfo: map[string]interface{}{
					"TotalTokens": 40,
				},
			},
		},
	}, nil).Once()

	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: "Test question requiring tool"},
			},
		},
	}

	config := agent.AgentConfig{
		MaxIterations: 5,
		Verbose:       false,
	}

	tools := mcpAgent.ExtractToolsFromAgent()
	result := mcpAgent.GenerateContentWithConfig(ctx, &msgs, config, tools)

	assert.Contains(t, result.FinalOutput, "Final answer after using tool")
	assert.Equal(t, 1, len(result.ToolCalls))
	assert.Equal(t, "test_tool_1", result.ToolCalls[0].Name)
	assert.Equal(t, 0, len(result.Errors))
	assert.Equal(t, 70, result.TokensUsed)

	mockLLM.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestGenerateContentWithConfig_MaxIterations(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)

	mcpAgent := &agent.MCPAgent{
		Name:           "test_agent",
		Provider:       "test_provider",
		LLMModel:       mockLLM,
		MCPServerTools: make(map[string][]mcp.Tool),
	}

	// Mock LLM to always return tool calls (never final answer)
	mockLLM.On("GenerateContent", ctx, mock.Anything, mock.Anything).Return(&llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: "",
				ToolCalls: []llms.ToolCall{
					{
						ID: "call_1",
						FunctionCall: &llms.FunctionCall{
							Name:      "some_tool",
							Arguments: "{}",
						},
					},
				},
			},
		},
	}, nil)

	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: "Test question"},
			},
		},
	}

	config := agent.AgentConfig{
		MaxIterations: 2,
		Verbose:       false,
	}

	result := mcpAgent.GenerateContentWithConfig(ctx, &msgs, config, []llms.Tool{})

	assert.Greater(t, len(result.Errors), 0)
	assert.Contains(t, result.Errors[len(result.Errors)-1], "maximum iterations")
}

func TestGenerateContentWithConfig_LLMError(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)

	mcpAgent := &agent.MCPAgent{
		Name:           "test_agent",
		Provider:       "test_provider",
		LLMModel:       mockLLM,
		MCPServerTools: make(map[string][]mcp.Tool),
	}

	mockLLM.On("GenerateContent", ctx, mock.Anything, mock.Anything).Return(
		nil, errors.New("LLM error"))

	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: "Test question"},
			},
		},
	}

	config := agent.AgentConfig{
		MaxIterations: 5,
		Verbose:       false,
	}

	result := mcpAgent.GenerateContentWithConfig(ctx, &msgs, config, []llms.Tool{})

	assert.Greater(t, len(result.Errors), 0)
	assert.Contains(t, result.Errors[0], "LLM generation error")
}

func TestGetTokenCount(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	tests := []struct {
		name     string
		response *llms.ContentResponse
		expected int
	}{
		{
			name: "OpenAI format",
			response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "test content",
						GenerationInfo: map[string]interface{}{
							"TotalTokens": 100,
						},
					},
				},
			},
			expected: 100,
		},
		{
			name: "Google format",
			response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "test content",
						GenerationInfo: map[string]interface{}{
							"total_tokens": 150,
						},
					},
				},
			},
			expected: 150,
		},
		{
			name: "Anthropic format",
			response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "test content",
						GenerationInfo: map[string]interface{}{
							"input_tokens":  50,
							"output_tokens": 75,
						},
					},
				},
			},
			expected: 125,
		},
		{
			name: "Fallback estimation",
			response: &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "This is a test content with some length.",
					},
				},
			},
			expected: 10, // 40 chars / 4 = 10
		},
		{
			name:     "Empty response",
			response: &llms.ContentResponse{Choices: []*llms.ContentChoice{}},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.GetTokenCount(tt.response)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateAndParseArguments(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "Valid JSON",
			input:     `{"key": "value"}`,
			expectErr: false,
		},
		{
			name:      "Empty string",
			input:     "",
			expectErr: false,
		},
		{
			name:      "Empty object",
			input:     "{}",
			expectErr: false,
		},
		{
			name:      "Invalid JSON",
			input:     `{"key": invalid}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := agent.ValidateAndParseArguments(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.input == "" || tt.input == "{}" {
					assert.Nil(t, result)
				} else {
					assert.NotNil(t, result)
				}
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "No truncation needed",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "Truncation needed",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is a ...",
		},
		{
			name:     "Exact length",
			input:    "exactly10c",
			maxLen:   10,
			expected: "exactly10c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteToolWithTimeout(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()
	mockLLM := new(MockLLMModel)
	mockClient := new(MockMCPClient)

	testTools := createTestTools()

	mockClient.On("ListTools", ctx, mock.Anything).Return(&mcp.ListToolsResult{
		Tools: testTools,
	}, nil)

	toolResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "success",
			},
		},
	}

	mockClient.On("CallTool", mock.Anything, mock.Anything).Return(toolResult, nil)

	mcpServer := createMockServer("test_server", testTools)
	mcpServer.Client = mockClient

	agentServers := []model.AgentServer{
		{
			Name:         "test_server",
			AllowedTools: []string{"test_tool_1"},
		},
	}

	mcpAgent := agent.NewMCPAgent(ctx, "test_agent", agentServers, []*server.MCPServer{mcpServer}, "test_provider", mockLLM)

	config := agent.AgentConfig{
		ToolTimeout: 5 * time.Second,
		Verbose:     false,
	}

	toolCall := llms.ToolCall{
		ID: "test_call",
		FunctionCall: &llms.FunctionCall{
			Name:      "test_tool_1",
			Arguments: `{"param1": "value"}`,
		},
	}

	result, response, err := mcpAgent.ExecuteToolWithTimeout(ctx, toolCall, config, 1, 1, 1)

	assert.NoError(t, err)
	assert.Equal(t, "test_tool_1", result.Name)
	assert.NotEmpty(t, response)

	mockClient.AssertExpectations(t)
}

func TestIsClarificationRequest(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Positive cases - should detect clarification requests
		{
			name:     "would you like me to",
			text:     "I found the file. Would you like me to read its contents?",
			expected: true,
		},
		{
			name:     "do you want me to",
			text:     "Do you want me to create this file for you?",
			expected: true,
		},
		{
			name:     "should i proceed",
			text:     "I'm ready to delete these files. Should I proceed?",
			expected: true,
		},
		{
			name:     "shall i proceed",
			text:     "The operation is ready. Shall I proceed with the changes?",
			expected: true,
		},
		{
			name:     "shall i continue",
			text:     "I've completed step 1. Shall I continue with step 2?",
			expected: true,
		},
		{
			name:     "would you prefer",
			text:     "Would you prefer JSON or YAML format for the output?",
			expected: true,
		},
		{
			name:     "do you prefer",
			text:     "Do you prefer to save this to a file or display it?",
			expected: true,
		},
		{
			name:     "please confirm",
			text:     "I will delete all files in /tmp. Please confirm before I proceed.",
			expected: true,
		},
		{
			name:     "please clarify",
			text:     "Please clarify which directory you want me to use.",
			expected: true,
		},
		{
			name:     "can you confirm",
			text:     "Can you confirm that this is the correct file path?",
			expected: true,
		},
		{
			name:     "can you clarify",
			text:     "Can you clarify what format you need?",
			expected: true,
		},
		{
			name:     "let me know if",
			text:     "I've prepared the script. Let me know if you want me to run it.",
			expected: true,
		},
		{
			name:     "is that correct",
			text:     "You want to rename the file to 'output.txt', is that correct?",
			expected: true,
		},
		{
			name:     "is this correct",
			text:     "The target directory is /home/user. Is this correct?",
			expected: true,
		},
		{
			name:     "would you like to",
			text:     "Would you like to see the full contents of the file?",
			expected: true,
		},
		{
			name:     "do you want to proceed",
			text:     "Do you want to proceed with this operation?",
			expected: true,
		},
		{
			name:     "case insensitive - uppercase",
			text:     "WOULD YOU LIKE ME TO help with that?",
			expected: true,
		},
		{
			name:     "case insensitive - mixed case",
			text:     "Should I Proceed with the deletion?",
			expected: true,
		},
		{
			name:     "pattern in middle of text",
			text:     "I analyzed the data and found 3 issues. Do you want me to fix them automatically or show you the details first?",
			expected: true,
		},
		// Negative cases - should NOT be detected as clarification
		{
			name:     "direct action statement",
			text:     "I have created the file successfully.",
			expected: false,
		},
		{
			name:     "information response",
			text:     "The file contains 150 lines of code.",
			expected: false,
		},
		{
			name:     "task completion",
			text:     "Done! I've updated the configuration file with the new settings.",
			expected: false,
		},
		{
			name:     "empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "unrelated question",
			text:     "What is the current directory?",
			expected: false,
		},
		{
			name:     "statement with similar words",
			text:     "I would like to note that the file was modified yesterday.",
			expected: false,
		},
		{
			name:     "past tense action",
			text:     "I proceeded to create the file as requested.",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with builtin patterns enabled (default behavior)
			result := agent.IsClarificationRequest(tt.text, true, nil)
			assert.Equal(t, tt.expected, result, "IsClarificationRequest(%q, true, nil) = %v, want %v", tt.text, result, tt.expected)
		})
	}
}

func TestIsClarificationRequest_CustomPatterns(t *testing.T) {
	// Test custom regex patterns
	customPattern := regexp.MustCompile(`(?i)¿te gustaría`)
	germanPattern := regexp.MustCompile(`(?i)möchten sie`)

	tests := []struct {
		name               string
		text               string
		useBuiltin         bool
		customPatterns     []*regexp.Regexp
		expected           bool
	}{
		// Custom patterns only (builtin disabled)
		{
			name:           "custom pattern matches - Spanish",
			text:           "¿Te gustaría que proceda con la operación?",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       true,
		},
		{
			name:           "custom pattern matches - German",
			text:           "Möchten Sie fortfahren?",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{germanPattern},
			expected:       true,
		},
		{
			name:           "custom pattern does not match",
			text:           "I have completed the task.",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       false,
		},
		{
			name:           "builtin disabled - builtin pattern should not match",
			text:           "Would you like me to proceed?",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       false,
		},
		// Builtin + custom patterns (additive mode)
		{
			name:           "additive - builtin matches",
			text:           "Would you like me to proceed?",
			useBuiltin:     true,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       true,
		},
		{
			name:           "additive - custom matches",
			text:           "¿Te gustaría continuar?",
			useBuiltin:     true,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       true,
		},
		{
			name:           "additive - neither matches",
			text:           "Task completed successfully.",
			useBuiltin:     true,
			customPatterns: []*regexp.Regexp{customPattern},
			expected:       false,
		},
		// Multiple custom patterns
		{
			name:           "multiple custom - first matches",
			text:           "¿Te gustaría proceder?",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{customPattern, germanPattern},
			expected:       true,
		},
		{
			name:           "multiple custom - second matches",
			text:           "Möchten Sie das bestätigen?",
			useBuiltin:     false,
			customPatterns: []*regexp.Regexp{customPattern, germanPattern},
			expected:       true,
		},
		// Empty/nil patterns
		{
			name:           "nil custom patterns with builtin",
			text:           "Would you like me to help?",
			useBuiltin:     true,
			customPatterns: nil,
			expected:       true,
		},
		{
			name:           "empty custom patterns with builtin",
			text:           "Would you like me to help?",
			useBuiltin:     true,
			customPatterns: []*regexp.Regexp{},
			expected:       true,
		},
		{
			name:           "nil custom patterns without builtin",
			text:           "Would you like me to help?",
			useBuiltin:     false,
			customPatterns: nil,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.IsClarificationRequest(tt.text, tt.useBuiltin, tt.customPatterns)
			assert.Equal(t, tt.expected, result, "IsClarificationRequest(%q, %v, patterns) = %v, want %v", tt.text, tt.useBuiltin, result, tt.expected)
		})
	}
}
