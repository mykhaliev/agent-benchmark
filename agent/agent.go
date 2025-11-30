package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/life4/genesis/slices"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/tmc/langchaingo/llms"
)

const (
	DefaultMaxIterations = 10
	ResultPreviewLength  = 2000
	LongResultLength     = 10000
	ApproxTokenDivisor   = 4
)

type MCPAgent struct {
	Name           string                `json:"name"`
	MCPServerNames []model.AgentServer   `json:"mcp_servers"`
	MCPServerTools map[string][]mcp.Tool `json:"-"`
	ToolToServer   map[string]string     `json:"-"`
	McpServers     []*server.MCPServer   `json:"-"`
	Provider       string                `json:"provider"`
	LLMModel       llms.Model            `json:"-"`
	AvailableTools []string              `json:"-"`
}

type AgentConfig struct {
	MaxIterations        int
	AddNotFinalResponses bool
	Verbose              bool
	ToolTimeout          time.Duration
}

func NewMCPAgent(
	ctx context.Context,
	name string,
	mcpServersForAgent []model.AgentServer,
	servers []*server.MCPServer,
	provider string,
	llmModel llms.Model,
) *MCPAgent {
	ag := &MCPAgent{
		Name:           name,
		MCPServerNames: mcpServersForAgent,
		MCPServerTools: make(map[string][]mcp.Tool),
		ToolToServer:   make(map[string]string), // Initialize the new map
		McpServers:     make([]*server.MCPServer, 0),
		Provider:       provider,
		LLMModel:       llmModel,
	}

	logger.Logger.Info("Creating agent",
		"agent", name,
		"provider", provider,
		"servers_requested", len(mcpServersForAgent))

	for idx, srv := range mcpServersForAgent {
		logger.Logger.Debug("Processing MCP server",
			"index", idx+1,
			"total", len(mcpServersForAgent),
			"server_name", srv.Name)

		mcpServer, err := slices.Find(servers, func(s *server.MCPServer) bool {
			return s.Name == srv.Name
		})

		if err != nil {
			logger.Logger.Error("Server not found",
				"server", srv.Name,
				"agent", ag.Name,
				"error", err)
			continue
		}

		ag.McpServers = append(ag.McpServers, mcpServer)
		logger.Logger.Debug("Server found, listing tools", "server", srv.Name)

		toolsRes, err := mcpServer.Client.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			logger.Logger.Error("Failed to list tools",
				"server", srv.Name,
				"error", err)
			continue
		}

		if toolsRes == nil {
			logger.Logger.Warn("No tools response from server", "server", srv.Name)
			continue
		}

		logger.Logger.Debug("Server tools listed",
			"server", srv.Name,
			"total_tools", len(toolsRes.Tools))

		allowedTools := slices.Filter(toolsRes.Tools, func(tool mcp.Tool) bool {
			isAllowed := len(srv.AllowedTools) == 0 || slices.Contains(srv.AllowedTools, tool.Name)
			if !isAllowed {
				logger.Logger.Debug("Tool filtered out",
					"tool", tool.Name,
					"server", srv.Name)
			}
			return isAllowed
		})

		if len(allowedTools) == 0 {
			logger.Logger.Warn("No allowed tools for server", "server", srv.Name)
		}

		if _, ok := ag.MCPServerTools[srv.Name]; !ok {
			ag.MCPServerTools[srv.Name] = make([]mcp.Tool, 0)
		}
		ag.MCPServerTools[srv.Name] = append(ag.MCPServerTools[srv.Name], allowedTools...)

		// Populate the ToolToServer map
		for _, tool := range allowedTools {
			if existingServer, exists := ag.ToolToServer[tool.Name]; exists {
				logger.Logger.Warn("Tool name collision detected",
					"tool", tool.Name,
					"existing_server", existingServer,
					"new_server", srv.Name)
				// You could handle this by prefixing in this case, or using a different strategy
				// For now, we'll keep the first server that registered the tool
			} else {
				ag.ToolToServer[tool.Name] = srv.Name
			}
			ag.AvailableTools = append(ag.AvailableTools, tool.Name)
		}

		allowedToolNames := slices.Map(allowedTools, func(tool mcp.Tool) string {
			return tool.Name
		})
		logger.Logger.Info("Agent tools configured",
			"agent", ag.Name,
			"server", srv.Name,
			"tools", strings.Join(allowedToolNames, ", "))
	}

	logger.Logger.Info("Agent initialization complete",
		"agent", ag.Name,
		"servers", len(ag.McpServers),
		"total_tools", countTotalTools(ag.MCPServerTools),
		"unique_tool_names", len(ag.ToolToServer))

	return ag
}

func (m *MCPAgent) ExecuteTool(ctx context.Context, toolName, argumentsInJSON string) (string, error) {
	if m.LLMModel == nil {
		return "", fmt.Errorf("LLM model is not initialized")
	}

	// Look up the server for this tool
	serverName, exists := m.ToolToServer[toolName]
	if !exists {
		return "", fmt.Errorf("tool '%s' not found in any registered server", toolName)
	}

	arguments, err := validateAndParseArguments(argumentsInJSON)
	if err != nil {
		return "", fmt.Errorf("failed to parse arguments for tool '%s': %w", toolName, err)
	}

	toolServer, err := m.findServer(serverName)
	if err != nil {
		return "", fmt.Errorf("MCP server '%s' not found for tool '%s': %w", serverName, toolName, err)
	}

	if !m.isToolAllowed(serverName, toolName) {
		return "", fmt.Errorf("tool '%s' is not allowed on server '%s'", toolName, serverName)
	}

	if arguments == nil || arguments == "{}" {
		arguments = map[string]interface{}{}
	}
	result, err := toolServer.Client.CallTool(ctx, mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: struct {
			Name      string    `json:"name"`
			Arguments any       `json:"arguments,omitempty"`
			Meta      *mcp.Meta `json:"_meta,omitempty"`
		}{
			Name:      toolName,
			Arguments: arguments,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to call MCP tool '%s' on server '%s': %w", toolName, serverName, err)
	}

	marshaledResult, err := sonic.MarshalString(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP tool result: %w", err)
	}

	return marshaledResult, nil
}

func (m *MCPAgent) GenerateContentWithConfig(
	ctx context.Context,
	msgs *[]llms.MessageContent,
	config AgentConfig,
) model.ExecutionResult {
	startTime := time.Now()
	maxIterations := getMaxIterations(config.MaxIterations)

	if config.Verbose {
		logger.Logger.Info("Execution started",
			"agent", m.Name,
			"provider", m.Provider,
			"max_iterations", maxIterations,
			"available_tools", countTotalTools(m.MCPServerTools))
	}

	result := initializeExecutionResult(m.Name, m.Provider, startTime)
	recordUserMessages(msgs, &result, config.Verbose)

	tools := m.ExtractToolsFromAgent()
	if config.Verbose {
		logger.Logger.Debug("Tools extracted for LLM", "count", len(tools))
	}

	response := ""
	iteration := 0

	for iteration < maxIterations {
		iteration++

		if config.Verbose {
			logger.Logger.Debug("Starting LLM call",
				"iteration", iteration,
				"max_iterations", maxIterations)
		}

		if ctx.Err() != nil {
			errMsg := fmt.Sprintf("Context cancelled: %v", ctx.Err())
			result.Errors = append(result.Errors, errMsg)
			logger.Logger.Error("Context cancelled",
				"iteration", iteration,
				"error", ctx.Err())
			break
		}

		resp, err := m.LLMModel.GenerateContent(ctx, *msgs, llms.WithTools(tools))
		if err != nil {
			errMsg := fmt.Sprintf("LLM generation error (iteration %d): %v", iteration, err)
			result.Errors = append(result.Errors, errMsg)
			logger.Logger.Error("LLM generation failed",
				"iteration", iteration,
				"error", err)
			break
		}

		if len(resp.Choices) == 0 {
			errMsg := fmt.Sprintf("LLM returned no choices (iteration %d)", iteration)
			result.Errors = append(result.Errors, errMsg)
			logger.Logger.Error("No choices returned from LLM", "iteration", iteration)
			break
		}

		assistantText := resp.Choices[0].Content

		if strings.TrimSpace(assistantText) != "" {
			if config.Verbose {
				logger.Logger.Debug("Assistant response",
					"iteration", iteration,
					"text_preview", truncateString(assistantText, LongResultLength))
			}

			result.Messages = append(result.Messages, model.Message{
				Role:      "assistant",
				Content:   assistantText,
				Timestamp: time.Now(),
			})

			*msgs = append(*msgs, llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					llms.TextContent{Text: assistantText},
				},
			})
		}

		toolCalls := resp.Choices[0].ToolCalls

		if len(toolCalls) == 0 {
			response += assistantText
			if config.Verbose {
				logger.Logger.Debug("LLM finished conversation:",
					"reason", resp.Choices[0].StopReason,
					"content", resp.Choices[0].Content)
				logger.Logger.Info("Final answer received", "iteration", iteration)
			}
			break
		}

		if config.Verbose {
			logger.Logger.Debug("Processing tool calls",
				"iteration", iteration,
				"tool_count", len(toolCalls))
		}

		if config.AddNotFinalResponses {
			header := fmt.Sprintf("\n[Iteration %d: %d tool(s) to execute]\n", iteration, len(toolCalls))
			response += header
		}

		for toolIdx, suggestedTool := range toolCalls {
			if config.Verbose {
				logger.Logger.Debug("Executing tool",
					"iteration", iteration,
					"tool_index", toolIdx+1,
					"total_tools", len(toolCalls),
					"tool_name", suggestedTool.FunctionCall.Name,
					"arguments", truncateString(suggestedTool.FunctionCall.Arguments, LongResultLength))
			}

			if config.AddNotFinalResponses {
				response += fmt.Sprintf("\n[tool_usage %d/%d] %s\n",
					toolIdx+1, len(toolCalls), suggestedTool.FunctionCall.Name)
			}

			toolCall, toolRes, toolErr := m.executeToolWithTimeout(
				ctx, suggestedTool, config, iteration, toolIdx+1, len(toolCalls))

			if toolErr != nil {
				result.Errors = append(result.Errors, toolErr.Error())
			}

			result.ToolCalls = append(result.ToolCalls, toolCall)

			*msgs = append(*msgs, llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					suggestedTool,
				},
			})

			*msgs = append(*msgs, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						Name:       suggestedTool.FunctionCall.Name,
						ToolCallID: suggestedTool.ID,
						Content:    toolRes,
					},
				},
			})

			if config.AddNotFinalResponses {
				printRes := truncateString(toolRes, LongResultLength)
				response += fmt.Sprintf("\n[tool_response] %s\n", printRes)
			}
		}
	}

	if iteration >= maxIterations {
		msg := fmt.Sprintf("Reached maximum iterations (%d) without final answer", maxIterations)
		result.Errors = append(result.Errors, msg)
		logger.Logger.Warn("Max iterations reached",
			"max_iterations", maxIterations,
			"agent", m.Name)
	}

	result.FinalOutput = response
	result.EndTime = time.Now()
	result.LatencyMs = time.Since(startTime).Milliseconds()
	result.TokensUsed = len(response) / ApproxTokenDivisor

	if config.Verbose {
		logger.Logger.Info("Execution completed",
			"iterations", iteration,
			"max_iterations", maxIterations,
			"duration_ms", result.LatencyMs,
			"tool_calls", len(result.ToolCalls),
			"errors", len(result.Errors),
			"approx_tokens", result.TokensUsed)
	}

	return result
}

func (m *MCPAgent) GenerateContentAsStreaming(
	ctx context.Context,
	msgs *[]llms.MessageContent,
	config AgentConfig,
) (chan string, chan model.ExecutionResult) {
	streamingChan := make(chan string, 10)
	resultChan := make(chan model.ExecutionResult, 1)

	go func() {
		defer close(streamingChan)
		defer close(resultChan)

		startTime := time.Now()
		maxIterations := getMaxIterations(config.MaxIterations)

		if config.Verbose {
			logger.Logger.Info("Streaming execution started",
				"agent", m.Name,
				"provider", m.Provider,
				"max_iterations", maxIterations)
		}

		result := initializeExecutionResult(m.Name, m.Provider, startTime)
		recordUserMessages(msgs, &result, config.Verbose)

		tools := m.ExtractToolsFromAgent()
		if config.Verbose {
			logger.Logger.Debug("Tools extracted for streaming", "count", len(tools))
		}

		response := ""
		iteration := 0

		for iteration < maxIterations {
			iteration++

			if config.Verbose {
				logger.Logger.Debug("Starting streaming iteration",
					"iteration", iteration,
					"max_iterations", maxIterations)
			}

			if ctx.Err() != nil {
				errMsg := fmt.Sprintf("Context cancelled: %v", ctx.Err())
				result.Errors = append(result.Errors, errMsg)
				logger.Logger.Error("Streaming context cancelled",
					"iteration", iteration,
					"error", ctx.Err())
				streamingChan <- fmt.Sprintf("\n[Error] %s\n", errMsg)
				break
			}

			resp, err := m.LLMModel.GenerateContent(ctx, *msgs, llms.WithTools(tools), llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
				if isToolCallChunk(chunk) {
					if config.Verbose {
						logger.Logger.Debug("Filtered tool call chunk", "iteration", iteration)
					}
					return nil
				}

				streamingChan <- string(chunk)
				response += string(chunk)
				return nil
			}))

			if err != nil {
				errMsg := fmt.Sprintf("LLM generation error (iteration %d): %v", iteration, err)
				result.Errors = append(result.Errors, errMsg)
				logger.Logger.Error("Streaming LLM generation failed",
					"iteration", iteration,
					"error", err)
				streamingChan <- fmt.Sprintf("\n[Error] %s\n", errMsg)
				break
			}

			if len(resp.Choices) == 0 {
				errMsg := fmt.Sprintf("LLM returned no choices (iteration %d)", iteration)
				result.Errors = append(result.Errors, errMsg)
				logger.Logger.Error("No choices in streaming response", "iteration", iteration)
				break
			}

			assistantText := resp.Choices[0].Content

			if strings.TrimSpace(assistantText) != "" {
				if config.Verbose {
					logger.Logger.Debug("Streaming assistant response",
						"iteration", iteration,
						"text_preview", truncateString(assistantText, 150))
				}

				result.Messages = append(result.Messages, model.Message{
					Role:      "assistant",
					Content:   assistantText,
					Timestamp: time.Now(),
				})

				*msgs = append(*msgs, llms.MessageContent{
					Role: llms.ChatMessageTypeAI,
					Parts: []llms.ContentPart{
						llms.TextContent{Text: assistantText},
					},
				})
			}

			toolCalls := resp.Choices[0].ToolCalls

			if len(toolCalls) == 0 {
				if config.Verbose {
					logger.Logger.Info("Streaming final answer received", "iteration", iteration)
				}
				break
			}

			if config.Verbose {
				logger.Logger.Debug("Processing streaming tool calls",
					"iteration", iteration,
					"tool_count", len(toolCalls))
			}

			if config.AddNotFinalResponses {
				header := fmt.Sprintf("\n[Iteration %d: %d tool(s) to execute]\n", iteration, len(toolCalls))
				streamingChan <- header
				response += header
			}

			for toolIdx, suggestedTool := range toolCalls {
				if config.Verbose {
					logger.Logger.Debug("Executing streaming tool",
						"iteration", iteration,
						"tool_index", toolIdx+1,
						"total_tools", len(toolCalls),
						"tool_name", suggestedTool.FunctionCall.Name)
				}

				if config.AddNotFinalResponses {
					toolHeader := fmt.Sprintf("\n[tool_usage %d/%d] %s\n",
						toolIdx+1, len(toolCalls), suggestedTool.FunctionCall.Name)
					streamingChan <- toolHeader
					response += toolHeader
				}

				toolCall, toolRes, toolErr := m.executeToolWithTimeout(
					ctx, suggestedTool, config, iteration, toolIdx+1, len(toolCalls))

				if toolErr != nil {
					result.Errors = append(result.Errors, toolErr.Error())
					if config.AddNotFinalResponses {
						streamingChan <- fmt.Sprintf("\n[Error] %s\n", toolErr.Error())
					}
				}

				result.ToolCalls = append(result.ToolCalls, toolCall)

				*msgs = append(*msgs, llms.MessageContent{
					Role: llms.ChatMessageTypeAI,
					Parts: []llms.ContentPart{
						suggestedTool,
					},
				})

				*msgs = append(*msgs, llms.MessageContent{
					Role: llms.ChatMessageTypeTool,
					Parts: []llms.ContentPart{
						llms.ToolCallResponse{
							Name:       suggestedTool.FunctionCall.Name,
							ToolCallID: suggestedTool.ID,
							Content:    toolRes,
						},
					},
				})

				if config.AddNotFinalResponses {
					printRes := truncateString(toolRes, LongResultLength)
					toolResponse := fmt.Sprintf("\n[tool_response] %s\n", printRes)
					streamingChan <- toolResponse
					response += toolResponse
				}
			}
		}

		if iteration >= maxIterations {
			msg := fmt.Sprintf("Reached maximum iterations (%d) without final answer", maxIterations)
			result.Errors = append(result.Errors, msg)
			logger.Logger.Warn("Streaming max iterations reached",
				"max_iterations", maxIterations,
				"agent", m.Name)
			streamingChan <- fmt.Sprintf("\n[Warning] %s\n", msg)
		}

		result.FinalOutput = response
		result.EndTime = time.Now()
		result.LatencyMs = time.Since(startTime).Milliseconds()
		result.TokensUsed = len(response) / ApproxTokenDivisor

		if config.Verbose {
			logger.Logger.Info("Streaming execution completed",
				"iterations", iteration,
				"duration_ms", result.LatencyMs,
				"tool_calls", len(result.ToolCalls))
		}

		resultChan <- result
	}()

	return streamingChan, resultChan
}

func (m *MCPAgent) ExtractToolsFromAgent() []llms.Tool {
	result := make([]llms.Tool, 0)

	for _, agTs := range m.MCPServerTools {
		for _, agT := range agTs {
			params := map[string]interface{}{
				"type":       agT.InputSchema.Type,
				"properties": agT.InputSchema.Properties,
			}

			if agT.InputSchema.Required != nil && len(agT.InputSchema.Required) > 0 {
				params["required"] = agT.InputSchema.Required
			}

			// No longer prefixing tool names with server names
			t := llms.Tool{
				Type: "function",
				Function: &llms.FunctionDefinition{
					Name:        agT.Name,
					Description: agT.Description,
					Parameters:  params,
				},
			}

			result = append(result, t)
		}
	}

	return result
}

func validateAndParseArguments(argumentsInJSON string) (any, error) {
	if argumentsInJSON == "" || argumentsInJSON == "{}" {
		return nil, nil
	}

	var temp any
	if err := json.Unmarshal([]byte(argumentsInJSON), &temp); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return json.RawMessage(argumentsInJSON), nil
}

func (m *MCPAgent) findServer(serverName string) (*server.MCPServer, error) {
	return slices.Find(m.McpServers, func(srv *server.MCPServer) bool {
		return srv.Name == serverName
	})
}

func (m *MCPAgent) isToolAllowed(serverName, toolName string) bool {
	tools, exists := m.MCPServerTools[serverName]
	if !exists {
		return false
	}

	for _, tool := range tools {
		if tool.Name == toolName {
			return true
		}
	}
	return false
}

func (m *MCPAgent) executeToolWithTimeout(
	ctx context.Context,
	suggestedTool llms.ToolCall,
	config AgentConfig,
	iteration, toolIdx, totalTools int,
) (model.ToolCall, string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(suggestedTool.FunctionCall.Arguments), &params); err != nil {
		if config.Verbose {
			logger.Logger.Warn("Failed to parse tool arguments",
				"iteration", iteration,
				"tool_index", toolIdx,
				"total_tools", totalTools,
				"error", err)
		}
		params = make(map[string]interface{})
	}

	toolCall := model.ToolCall{
		Name:       suggestedTool.FunctionCall.Name,
		Parameters: params,
		Timestamp:  time.Now(),
	}

	toolCtx := ctx
	var cancel context.CancelFunc
	if config.ToolTimeout > 0 {
		toolCtx, cancel = context.WithTimeout(ctx, config.ToolTimeout)
		if config.Verbose {
			logger.Logger.Debug("Tool timeout set",
				"iteration", iteration,
				"tool_index", toolIdx,
				"timeout", config.ToolTimeout)
		}
	}

	toolRes, toolErr := m.ExecuteTool(
		toolCtx,
		suggestedTool.FunctionCall.Name,
		suggestedTool.FunctionCall.Arguments,
	)

	if cancel != nil {
		cancel()
	}

	if toolErr != nil {
		errMsg := fmt.Sprintf("Tool execution error (iteration %d, tool %s): %v",
			iteration, suggestedTool.FunctionCall.Name, toolErr)

		toolCall.Result = model.Result{
			Content: []model.ContentItem{
				{
					Type: "text",
					Text: errMsg,
				},
			},
		}

		logger.Logger.Error("Tool execution failed",
			"iteration", iteration,
			"tool_index", toolIdx,
			"tool_name", suggestedTool.FunctionCall.Name,
			"error", toolErr)

		return toolCall, errMsg, fmt.Errorf(errMsg)
	}

	var resultData model.Result
	if err := json.Unmarshal([]byte(toolRes), &resultData); err != nil {
		if config.Verbose {
			logger.Logger.Warn("Failed to parse tool result",
				"iteration", iteration,
				"tool_index", toolIdx,
				"error", err)
		}
		toolCall.Result = model.Result{
			Content: []model.ContentItem{
				{
					Type: "text",
					Text: "Failed to parse tool result",
				},
			},
		}
	} else {
		toolCall.Result = resultData
	}

	if config.Verbose {
		logger.Logger.Debug("Tool execution successful",
			"iteration", iteration,
			"tool_index", toolIdx,
			"result_preview", truncateString(toolRes, ResultPreviewLength))
	}

	return toolCall, toolRes, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getMaxIterations(configValue int) int {
	if configValue <= 0 {
		return DefaultMaxIterations
	}
	return configValue
}

func initializeExecutionResult(agentName, provider string, startTime time.Time) model.ExecutionResult {
	return model.ExecutionResult{
		AgentName:    agentName,
		ProviderType: model.ProviderType(provider),
		StartTime:    startTime,
		Messages:     make([]model.Message, 0),
		ToolCalls:    make([]model.ToolCall, 0),
		Errors:       make([]string, 0),
		MCPOperations: model.MCPOperations{
			ResourcesRead: make([]string, 0),
			FilesCreated:  make([]string, 0),
			FilesWritten:  make([]string, 0),
			FilesDeleted:  make([]string, 0),
			ToolsList:     make([]string, 0),
		},
	}
}

func recordUserMessages(msgs *[]llms.MessageContent, result *model.ExecutionResult, verbose bool) {
	userMsgCount := 0
	for _, msg := range *msgs {
		if msg.Role != llms.ChatMessageTypeHuman {
			continue
		}

		for _, part := range msg.Parts {
			textPart, ok := part.(llms.TextContent)
			if !ok {
				continue
			}

			result.Messages = append(result.Messages, model.Message{
				Role:      "user",
				Content:   textPart.Text,
				Timestamp: time.Now(),
			})
			userMsgCount++
		}
	}

	if verbose {
		logger.Logger.Debug("User messages recorded", "count", userMsgCount)
	}
}

func countTotalTools(toolsMap map[string][]mcp.Tool) int {
	total := 0
	for _, tools := range toolsMap {
		total += len(tools)
	}
	return total
}

func isToolCallChunk(chunk []byte) bool {
	var toolCallArray []interface{}
	if err := json.Unmarshal(chunk, &toolCallArray); err == nil && len(toolCallArray) > 0 {
		return true
	}

	var chunkData map[string]interface{}
	if err := json.Unmarshal(chunk, &chunkData); err == nil {
		if choices, ok := chunkData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if toolCalls, ok := choice["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
					return true
				}
			}
		}
	}

	return false
}
