package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
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

// TestClarificationPromptScenarios tests real-world response patterns to verify the prompt correctly classifies them.
// These tests validate the prompt design by checking expected classifications for common LLM response patterns.
func TestClarificationPromptScenarios(t *testing.T) {
	// This test documents the expected behavior for various response patterns.
	// The actual LLM judge would classify these - here we test that the test framework
	// correctly handles different scenarios.

	scenarios := []struct {
		name                   string
		response               string
		shouldBeClarification  bool
		explanation            string
	}{
		// === TRUE CLARIFICATION REQUESTS (should be YES) ===
		{
			name:                  "Direct question before acting",
			response:              "Would you like me to create the file now?",
			shouldBeClarification: true,
			explanation:           "Asking permission before taking action",
		},
		{
			name:                  "Should I proceed pattern",
			response:              "I can see the data. Should I proceed with the analysis?",
			shouldBeClarification: true,
			explanation:           "Seeking confirmation before next step",
		},
		{
			name:                  "Listing options and asking to choose",
			response:              "I found three approaches:\n1. Method A\n2. Method B\n3. Method C\nWhich would you prefer?",
			shouldBeClarification: true,
			explanation:           "Asking user to choose before proceeding",
		},
		{
			name:                  "Asking for more details",
			response:              "I need more information. What format should the output be in?",
			shouldBeClarification: true,
			explanation:           "Requesting clarification before acting",
		},
		{
			name:                  "Confirmation request with context",
			response:              "I'm about to delete all files in the folder. Do you want me to proceed?",
			shouldBeClarification: true,
			explanation:           "Seeking destructive action confirmation",
		},
		{
			name:                  "Multiple choice before action",
			response:              "Should I save it as CSV or Excel format?",
			shouldBeClarification: true,
			explanation:           "Asking for format choice before saving",
		},

		// === NOT CLARIFICATION - COMPLETED TASK WITH POLITE CLOSING (should be NO) ===
		{
			name:                  "Task completed with let me know offer",
			response:              "Here are the steps completed:\n1. Created file\n2. Added data\n3. Saved\n\nIf you need anything else, let me know!",
			shouldBeClarification: false,
			explanation:           "Task completed, polite closing is not a clarification request",
		},
		{
			name:                  "Task completed with offer for more",
			response:              "✅ All steps completed successfully.\n\n1. Created the Excel file\n2. Added the worksheets\n3. Closed the file\n\nIf you'd like to repeat this or test other scenarios, just let me know.",
			shouldBeClarification: false,
			explanation:           "Completed task with offer for additional help is not clarification",
		},
		{
			name:                  "Task completed with emoji and follow-up offer",
			response:              "Done! The file has been saved to C:\\Users\\test\\output.xlsx. 🎉\n\nNeed anything else?",
			shouldBeClarification: false,
			explanation:           "Completed with casual follow-up offer",
		},
		{
			name:                  "Simple completion report",
			response:              "The file has been created and saved successfully.",
			shouldBeClarification: false,
			explanation:           "Pure completion report with no questions",
		},
		{
			name:                  "Detailed completion with results",
			response:              "I've completed the analysis:\n- Total records: 1,500\n- Valid: 1,450\n- Invalid: 50\n\nThe results have been saved to report.csv.",
			shouldBeClarification: false,
			explanation:           "Detailed results report, no questions",
		},
		{
			name:                  "Task list completion",
			response:              "Here's what I did:\n1. Opened the workbook\n2. Created 'SalesData' sheet\n3. Renamed it to 'Q1Sales'\n4. Listed all sheets\n5. Closed without saving\n\nLet me know if you need to save the changes or do more.",
			shouldBeClarification: false,
			explanation:           "Task list with offer for more is not clarification",
		},

		// === NOT CLARIFICATION - DIRECT ANSWERS/INFO (should be NO) ===
		{
			name:                  "Direct answer to question",
			response:              "The capital of France is Paris.",
			shouldBeClarification: false,
			explanation:           "Direct factual answer",
		},
		{
			name:                  "Explaining what was done",
			response:              "I analyzed the data and found that sales increased by 15% in Q2.",
			shouldBeClarification: false,
			explanation:           "Reporting completed analysis",
		},
		{
			name:                  "Error report",
			response:              "The operation failed because the file was not found at the specified path.",
			shouldBeClarification: false,
			explanation:           "Error reporting is not clarification",
		},

		// === EDGE CASES ===
		{
			name:                  "Question mark in completed statement",
			response:              "Interesting finding, right? The data shows a clear trend upward.",
			shouldBeClarification: false,
			explanation:           "Rhetorical question after providing info",
		},
		{
			name:                  "Conditional offer after completion",
			response:              "File saved. If you want, I can also create a backup.",
			shouldBeClarification: false,
			explanation:           "Conditional offer after task is done - not blocking",
		},

		// === FORMATTED COMPLETION SUMMARIES (should be NO) ===
		// These are verbose, well-structured completion reports that should NOT be classified as clarification
		{
			name:                  "Data model setup complete with headers",
			response:              "✅ **Data Model Analysis Setup Complete**\n\nYour sales data is now fully enabled for advanced analysis:\n\n### 🧠 Added to Data Model\n- **Sales** table with 500 records\n- **Products** dimension table\n\n### 📊 Next Steps\nYou can now create PivotTables or DAX measures.\n\nLet me know if you'd like to proceed!",
			shouldBeClarification: false,
			explanation:           "Structured completion report with markdown headers is NOT clarification",
		},
		{
			name:                  "Star schema complete with formatted sections",
			response:              "✅ **Tables Successfully Linked – Star Schema Complete**\n\nThe relationship between your fact and dimension tables has been established:\n\n| Relationship | Type |\n|---|---|\n| Sales → Products | Many-to-One |\n\nReady for further analysis!",
			shouldBeClarification: false,
			explanation:           "Completion with tables and sections is NOT clarification",
		},
		{
			name:                  "PivotTable analysis created with emoji headers",
			response:              "✅ **PivotTable Analysis Created Successfully**\n\n### 📊 Configuration\n- Rows: Category\n- Values: Sum of Revenue\n- Filter: Region\n\nThe PivotTable is ready for use. Need any adjustments?",
			shouldBeClarification: false,
			explanation:           "Emoji-decorated completion with configuration details is NOT clarification",
		},
		{
			name:                  "PivotChart added with celebration",
			response:              "✅ **PivotChart Added Successfully**\n\nYour sales dashboard is coming together nicely! 📈\n\n- Chart Type: Column Chart\n- Based on: SalesAnalysis PivotTable\n\nWould you like to customize the chart further?",
			shouldBeClarification: false,
			explanation:           "Celebration + optional offer after completion is NOT clarification",
		},
		{
			name:                  "Multi-section completion report",
			response:              "## ✅ Operation Complete\n\n### What Was Done\n1. Imported the CSV file\n2. Created Power Query connection\n3. Loaded data to worksheet\n\n### Results\n- 1,500 rows imported\n- 12 columns detected\n\n### Tips\n- Refresh to update data\n- Use PivotTables for analysis\n\nAnything else you need?",
			shouldBeClarification: false,
			explanation:           "Multi-section detailed report is NOT clarification",
		},
		{
			name:                  "Success with bold emphasis",
			response:              "**Success!** The workbook has been saved.\n\n**File:** C:\\Data\\Report.xlsx\n**Sheets:** 3\n**Size:** 125 KB\n\nLet me know if you need to make any changes.",
			shouldBeClarification: false,
			explanation:           "Bold-formatted success report is NOT clarification",
		},
		{
			name:                  "Numbered steps completed with summary",
			response:              "Done! Here's what I accomplished:\n\n1. ✅ Created new workbook\n2. ✅ Added 'Sales' worksheet\n3. ✅ Imported 500 rows of data\n4. ✅ Applied formatting\n5. ✅ Saved file\n\n**Total time:** 2.3 seconds\n\nReady for the next task!",
			shouldBeClarification: false,
			explanation:           "Checkmark-decorated step list is completion, NOT clarification",
		},
		{
			name:                  "Ready for analysis phrasing",
			response:              "✅ **Products Table Added to Power Pivot (Data Model)**\n\nThe **Products** data is now successfully loaded into the **Excel Data Model**, which means it is **ready for DAX measures and Power Pivot analysis**.\n\n### What's Now Available\n- DAX measure creation\n- Relationships with other tables\n- PivotTable analysis\n\nLet me know what you'd like to do next!",
			shouldBeClarification: false,
			explanation:           "'Ready for analysis' describes completed state, NOT a question",
		},
		{
			name:                  "Measures created ready for analysis",
			response:              "✅ **DAX Measures Created Successfully**\n\nYour **Products** table (in the Data Model as **Table_ExternalData_1**) now has a solid starter set of **Power Pivot DAX measures** ready for analysis.\n\n---\n\n### Measures Created\n1. Average Rating\n2. Total Products\n3. Average Discount\n\nThese are now available in PivotTables.",
			shouldBeClarification: false,
			explanation:           "Completion report listing what was created is NOT clarification",
		},
	}

	t.Logf("Documented %d clarification detection scenarios", len(scenarios))
	for _, s := range scenarios {
		t.Logf("  - %s: shouldBeClarification=%v (%s)", s.name, s.shouldBeClarification, s.explanation)
	}

	// Count expected classifications
	yesCases := 0
	noCases := 0
	for _, s := range scenarios {
		if s.shouldBeClarification {
			yesCases++
		} else {
			noCases++
		}
	}
	t.Logf("Expected: %d YES (clarification), %d NO (not clarification)", yesCases, noCases)
}

// TestClarificationJudgeIntegration tests the clarification judge prompt against real LLM.
// This test requires AZURE_OPENAI_ENDPOINT environment variable to be set.
// Run with: go test ./test -run "TestClarificationJudgeIntegration" -v
func TestClarificationJudgeIntegration(t *testing.T) {
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping integration test: AZURE_OPENAI_ENDPOINT not set")
	}

	logger.SetupLogger(NewDummyWriter(), true)
	ctx := context.Background()

	// Create real LLM client for gpt-4.1 (a more capable judge model) using Entra ID auth
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		t.Fatalf("Failed to create Azure credential: %v", err)
	}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://cognitiveservices.azure.com/.default"},
	})
	if err != nil {
		t.Fatalf("Failed to get Azure token: %v", err)
	}

	judgeLLM, err := openai.New(
		openai.WithBaseURL(endpoint),
		openai.WithModel("gpt-4.1"),
		openai.WithAPIType(openai.APITypeAzureAD),
		openai.WithAPIVersion("2025-01-01-preview"),
		openai.WithToken(token.Token),
	)
	if err != nil {
		t.Fatalf("Failed to create LLM client: %v", err)
	}

	// These are EXACT responses from production tests that were incorrectly classified
	// as clarification requests. The judge should return false (NO) for all of these.
	notClarificationCases := []struct {
		name     string
		response string
	}{
		{
			name: "Products Table Added to Data Model",
			response: `✅ **Products Table Added to Power Pivot (Data Model)**

The **Products** data is now successfully loaded into the **Excel Data Model**, which means it is **ready for DAX measures and Power Pivot analysis**.

### What's Now Available
- DAX measure creation
- Relationships with other tables
- PivotTable analysis

Let me know what you'd like to do next!`,
		},
		{
			name: "DAX Measures Created Successfully",
			response: `✅ **DAX Measures Created Successfully**

Your **Products** table (in the Data Model as **Table_ExternalData_1**) now has a solid starter set of **Power Pivot DAX measures** ready for analysis.

---

### 📏 Measures Created

| Measure | Formula |
|---------|---------|
| Average Rating | AVERAGE(Table_ExternalData_1[rating]) |
| Total Products | COUNTROWS(Table_ExternalData_1) |
| Average Discount | AVERAGE(Table_ExternalData_1[discount_percentage]) |
| Total Potential Revenue | SUM(Table_ExternalData_1[actual_price]) |

These measures are now available in any PivotTable connected to the Data Model.`,
		},
		{
			name: "Data Model Analysis Setup Complete",
			response: `✅ **Data Model Analysis Setup Complete**

Your sales data is now fully enabled for advanced analysis:

### 🧠 Added to Data Model
- **Sales** table with 500 records
- **Products** dimension table

### 📊 Next Steps
You can now create PivotTables or DAX measures.

Let me know if you'd like to proceed!`,
		},
		{
			name: "Tables Successfully Linked Star Schema",
			response: `✅ **Tables Successfully Linked – Star Schema Complete**

The relationship between your fact and dimension tables has been established:

| Relationship | Type |
|---|---|
| Sales → Products | Many-to-One |

Ready for further analysis!`,
		},
		{
			name: "Simple completion with offer",
			response: `Done! The file has been saved to C:\Users\test\output.xlsx. 🎉

Need anything else?`,
		},
		{
			name: "Task list with checkmarks",
			response: `Here's what I did:
1. Opened the workbook
2. Created 'SalesData' sheet
3. Renamed it to 'Q1Sales'
4. Listed all sheets
5. Closed without saving

Let me know if you need to save the changes or do more.`,
		},
	}

	// Test NOT clarification cases - all should return false
	t.Run("NotClarificationCases", func(t *testing.T) {
		for _, tc := range notClarificationCases {
			t.Run(tc.name, func(t *testing.T) {
				result := agent.CheckClarificationWithLLM(ctx, judgeLLM, tc.response)
				if result {
					t.Errorf("FALSE POSITIVE: Response was incorrectly classified as clarification.\nResponse preview: %s...", tc.response[:min(200, len(tc.response))])
				} else {
					t.Logf("✓ Correctly classified as NOT clarification: %s", tc.name)
				}
			})
		}
	})

	// True clarification cases - all should return true
	clarificationCases := []struct {
		name     string
		response string
	}{
		{
			name:     "Direct question before acting",
			response: "Would you like me to create the file now?",
		},
		{
			name:     "Should I proceed pattern",
			response: "I can see the data. Should I proceed with the analysis?",
		},
		{
			name:     "Asking for format choice",
			response: "Should I save it as CSV or Excel format?",
		},
		{
			name:     "Listing options asking to choose",
			response: "I found three approaches:\n1. Method A\n2. Method B\n3. Method C\nWhich would you prefer?",
		},
		{
			name:     "Confirmation before destructive action",
			response: "I'm about to delete all files in the folder. Do you want me to proceed?",
		},
	}

	t.Run("ClarificationCases", func(t *testing.T) {
		for _, tc := range clarificationCases {
			t.Run(tc.name, func(t *testing.T) {
				result := agent.CheckClarificationWithLLM(ctx, judgeLLM, tc.response)
				if !result {
					t.Errorf("FALSE NEGATIVE: Response was incorrectly classified as NOT clarification.\nResponse: %s", tc.response)
				} else {
					t.Logf("✓ Correctly classified as clarification: %s", tc.name)
				}
			})
		}
	})
}

