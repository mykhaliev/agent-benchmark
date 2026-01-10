package tests

import (
	"context"
	"encoding/json"
	"errors"
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

// TestCheckClarificationWithLLM tests the LLM-based clarification detection function
func TestCheckClarificationWithLLM(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()

	tests := []struct {
		name           string
		llmResponse    string
		expectedResult bool
	}{
		{
			name:           "LLM returns YES - is clarification",
			llmResponse:    "YES",
			expectedResult: true,
		},
		{
			name:           "LLM returns YES with explanation",
			llmResponse:    "YES, this is asking for confirmation",
			expectedResult: true,
		},
		{
			name:           "LLM returns NO - not clarification",
			llmResponse:    "NO",
			expectedResult: false,
		},
		{
			name:           "LLM returns NO with explanation",
			llmResponse:    "NO, this is a direct action",
			expectedResult: false,
		},
		{
			name:           "LLM returns lowercase yes",
			llmResponse:    "yes",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := new(MockLLMModel)
			mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(
				&llms.ContentResponse{
					Choices: []*llms.ContentChoice{
						{Content: tt.llmResponse},
					},
				}, nil,
			)

			result := agent.CheckClarificationWithLLM(ctx, mockLLM, "Some response text")
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestCheckClarificationWithLLM_NilLLM(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()

	// Should return false when LLM is nil
	result := agent.CheckClarificationWithLLM(ctx, nil, "Some response")
	assert.False(t, result)
}

func TestCheckClarificationWithLLM_LLMError(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()

	mockLLM := new(MockLLMModel)
	mockLLM.On("GenerateContent", mock.Anything, mock.Anything, mock.Anything).Return(
		nil, errors.New("LLM error"),
	)

	// Should return false when LLM returns an error
	result := agent.CheckClarificationWithLLM(ctx, mockLLM, "Some response")
	assert.False(t, result)
}
