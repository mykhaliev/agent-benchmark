package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
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

// ClarificationLevel defines the severity level for clarification detection logging
type ClarificationLevel string

const (
	ClarificationLevelInfo    ClarificationLevel = "info"
	ClarificationLevelWarning ClarificationLevel = "warning"
	ClarificationLevelError   ClarificationLevel = "error"
)

type AgentConfig struct {
	MaxIterations                 int
	AddNotFinalResponses          bool
	Verbose                       bool
	ToolTimeout                   time.Duration
	ClarificationDetectionEnabled bool
	ClarificationDetectionLevel   ClarificationLevel
	ClarificationJudgeLLM         llms.Model // LLM used to classify if a response is asking for clarification
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

	arguments, err := ValidateAndParseArguments(argumentsInJSON)
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
	tools []llms.Tool,
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

	// Initialize ClarificationStats when detection is enabled
	// This allows assertions to distinguish "enabled but no clarifications found" from "not enabled"
	if config.ClarificationDetectionEnabled {
		result.ClarificationStats = &model.ClarificationStats{
			Iterations: []int{},
			Examples:   []string{},
		}
	}

	recordUserMessages(msgs, &result, config.Verbose)

	if config.Verbose {
		logger.Logger.Debug("Tools extracted for LLM", "count", len(tools))
	}

	response := ""
	iteration := 0
	tokens := 0
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
					"text_preview", TruncateString(assistantText, LongResultLength))
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
		tokens += GetTokenCount(resp)
		if len(toolCalls) == 0 {
			response += assistantText
			// Check if LLM is asking for clarification instead of acting (using LLM-based detection)
			if config.ClarificationDetectionEnabled && CheckClarificationWithLLM(ctx, config.ClarificationJudgeLLM, assistantText) {
				recordClarificationRequest(config.ClarificationDetectionLevel, iteration, assistantText, &result)
			}
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
					"arguments", TruncateString(suggestedTool.FunctionCall.Arguments, LongResultLength))
			}

			if config.AddNotFinalResponses {
				response += fmt.Sprintf("\n[tool_usage %d/%d] %s\n",
					toolIdx+1, len(toolCalls), suggestedTool.FunctionCall.Name)
			}

			toolCall, toolRes, toolErr := m.ExecuteToolWithTimeout(
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
				printRes := TruncateString(toolRes, LongResultLength)
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
	result.TokensUsed = tokens

	// Collect rate limit stats if the LLM provides them
	result.RateLimitStats = m.collectRateLimitStats()

	if config.Verbose {
		logger.Logger.Info("Execution completed",
			"iterations", iteration,
			"max_iterations", maxIterations,
			"duration_ms", result.LatencyMs,
			"tool_calls", len(result.ToolCalls),
			"errors", len(result.Errors),
			"approx_tokens", result.TokensUsed)
		if result.RateLimitStats != nil && (result.RateLimitStats.ThrottleCount > 0 || result.RateLimitStats.RateLimitHits > 0) {
			logger.Logger.Info("Rate limit stats",
				"throttle_count", result.RateLimitStats.ThrottleCount,
				"throttle_wait_ms", result.RateLimitStats.ThrottleWaitTimeMs,
				"rate_limit_hits", result.RateLimitStats.RateLimitHits,
				"retry_count", result.RateLimitStats.RetryCount,
				"retry_success", result.RateLimitStats.RetrySuccessCount)
		}
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

		// Initialize ClarificationStats when detection is enabled
		// This allows assertions to distinguish "enabled but no clarifications found" from "not enabled"
		if config.ClarificationDetectionEnabled {
			result.ClarificationStats = &model.ClarificationStats{
				Iterations: []int{},
				Examples:   []string{},
			}
		}

		recordUserMessages(msgs, &result, config.Verbose)

		tools := m.ExtractToolsFromAgent()
		if config.Verbose {
			logger.Logger.Debug("Tools extracted for streaming", "count", len(tools))
		}

		response := ""
		iteration := 0
		tokens := 0
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
						"text_preview", TruncateString(assistantText, 150))
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
			tokens += GetTokenCount(resp)
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

				toolCall, toolRes, toolErr := m.ExecuteToolWithTimeout(
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
					printRes := TruncateString(toolRes, LongResultLength)
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
		result.TokensUsed = tokens

		// Collect rate limit stats if the LLM provides them
		result.RateLimitStats = m.collectRateLimitStats()

		if config.Verbose {
			logger.Logger.Info("Streaming execution completed",
				"iterations", iteration,
				"duration_ms", result.LatencyMs,
				"tool_calls", len(result.ToolCalls))
			if result.RateLimitStats != nil && (result.RateLimitStats.ThrottleCount > 0 || result.RateLimitStats.RateLimitHits > 0) {
				logger.Logger.Info("Rate limit stats",
					"throttle_count", result.RateLimitStats.ThrottleCount,
					"throttle_wait_ms", result.RateLimitStats.ThrottleWaitTimeMs,
					"rate_limit_hits", result.RateLimitStats.RateLimitHits,
					"retry_count", result.RateLimitStats.RetryCount,
					"retry_success", result.RateLimitStats.RetrySuccessCount)
			}
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

			if len(agT.InputSchema.Required) > 0 {
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

func ValidateAndParseArguments(argumentsInJSON string) (any, error) {
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

func (m *MCPAgent) ExecuteToolWithTimeout(
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

	// Measure actual tool execution time
	execStart := time.Now()
	toolRes, toolErr := m.ExecuteTool(
		toolCtx,
		suggestedTool.FunctionCall.Name,
		suggestedTool.FunctionCall.Arguments,
	)
	toolCall.DurationMs = time.Since(execStart).Milliseconds()

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

		return toolCall, errMsg, fmt.Errorf("%s", errMsg)
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
			"result_preview", TruncateString(toolRes, ResultPreviewLength))
	}

	return toolCall, toolRes, nil
}

func TruncateString(s string, maxLen int) string {
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
	}
}

// RateLimitStatsProvider is an interface for LLMs that can provide rate limit statistics
type RateLimitStatsProvider interface {
	GetStats() model.RateLimitStats
	ResetStats()
}

// collectRateLimitStats retrieves rate limit stats from the LLM if it supports them
func (m *MCPAgent) collectRateLimitStats() *model.RateLimitStats {
	if provider, ok := m.LLMModel.(RateLimitStatsProvider); ok {
		stats := provider.GetStats()
		// Only include if there's something to report
		if stats.ThrottleCount > 0 || stats.RateLimitHits > 0 || stats.RetryCount > 0 {
			return &stats
		}
	}
	return nil
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

// GetTokenCount extracts total token count from a ContentResponse
// If provider is unknown or token info is unavailable, it estimates using len(response)/4
func GetTokenCount(response *llms.ContentResponse) int {
	if len(response.Choices) == 0 {
		return 0
	}

	choice := response.Choices[0]
	genInfo := choice.GenerationInfo

	// Try to parse based on common provider keys
	if genInfo != nil {
		// Try OpenAI format
		if v := extractInt(genInfo["TotalTokens"]); v > 0 {
			return v
		}
		if v := extractInt(genInfo["total_tokens"]); v > 0 {
			return v
		}

		// Try prompt + completion tokens
		promptTokens := extractInt(genInfo["PromptTokens"])
		completionTokens := extractInt(genInfo["CompletionTokens"])
		if promptTokens > 0 || completionTokens > 0 {
			return promptTokens + completionTokens
		}
		promptTokens = extractInt(genInfo["prompt_tokens"])
		completionTokens = extractInt(genInfo["completion_tokens"])
		if promptTokens > 0 || completionTokens > 0 {
			return promptTokens + completionTokens
		}

		// Try Anthropic format (sum of input + output)
		inputTokens := extractInt(genInfo["input_tokens"])
		outputTokens := extractInt(genInfo["output_tokens"])
		if inputTokens > 0 || outputTokens > 0 {
			return inputTokens + outputTokens
		}
	}

	// Fallback: estimate using len(response)/4
	return len(choice.Content) / ApproxTokenDivisor
}

// extractInt safely extracts an integer from an any/interface{} value
// Returns 0 if the value cannot be converted to int
func extractInt(v any) int {
	if v == nil {
		return 0
	}

	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}

// clarificationJudgePrompt is the system prompt for the LLM that classifies responses
const clarificationJudgePrompt = `Classify if an AI assistant is asking for user input BEFORE completing a task.

Answer YES if the assistant:
- Asks "Would you like me to...", "Should I proceed...", "Do you want me to..."
- Asks "Which would you prefer?", "What format?", "Which option?"
- Requests confirmation before doing something: "Do you want me to proceed?"
- Asks for missing information: "What should the filename be?"
- Says "I'm about to..." and then asks for permission

Answer NO if the response:
- STARTS with ✅ or "Done!" or "Complete" or "Successfully" (these indicate COMPLETED work)
- Uses past tense to describe actions: "created", "added", "completed", "saved", "loaded"
- Contains a summary of what was accomplished
- Ends with "Let me know if..." AFTER describing completed work

CRITICAL RULE: If response STARTS with ✅, answer NO. The checkmark means the task is done.

Examples:
- "Would you like me to create the file?" → YES
- "Should I proceed with the analysis?" → YES  
- "I'm about to delete files. Proceed?" → YES
- "✅ File created. Let me know if you need more." → NO (starts with ✅)
- "Done! Here's what I did: ..." → NO (completed work)
- "✅ Setup Complete... Let me know if you'd like to proceed" → NO (starts with ✅, already completed)

Respond ONLY "YES" or "NO".`

// CheckClarificationWithLLM uses an LLM to determine if the response is asking for clarification.
// This is more accurate than pattern matching as it can understand context, nuance, and multiple languages.
// Returns true if the response is detected as a clarification request.
func CheckClarificationWithLLM(ctx context.Context, judgeLLM llms.Model, responseText string) bool {
	if judgeLLM == nil {
		logger.Logger.Warn("Clarification judge LLM is nil, skipping detection")
		return false
	}

	// Create a context with timeout to avoid blocking too long
	judgeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Prepare the messages for classification
	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: clarificationJudgePrompt},
			},
		},
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: fmt.Sprintf("Classify this AI assistant response:\n\n%s", responseText)},
			},
		},
	}

	// Call the judge LLM
	resp, err := judgeLLM.GenerateContent(judgeCtx, msgs)
	if err != nil {
		logger.Logger.Warn("Clarification detection LLM call failed", "error", err)
		return false
	}

	if len(resp.Choices) == 0 {
		logger.Logger.Warn("Clarification detection LLM returned no choices")
		return false
	}

	// Parse the response - looking for YES or NO
	answer := strings.TrimSpace(strings.ToUpper(resp.Choices[0].Content))

	// Handle responses that might have extra text
	if strings.HasPrefix(answer, "YES") {
		return true
	}

	return false
}

// logClarificationRequest logs the clarification request at the configured level
// and optionally adds it to the result errors based on the level.
func logClarificationRequest(level ClarificationLevel, iteration int, text string, result *model.ExecutionResult) {
	preview := TruncateString(text, 200)
	msg := fmt.Sprintf("LLM asked for clarification instead of acting (iteration %d): %s", iteration, preview)

	switch level {
	case ClarificationLevelInfo:
		logger.Logger.Info("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		// Info level does not add to errors
	case ClarificationLevelError:
		logger.Logger.Error("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		result.Errors = append(result.Errors, msg)
	default: // warning is the default
		logger.Logger.Warn("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		result.Errors = append(result.Errors, msg)
	}
}

// recordClarificationRequest logs, records stats, and optionally adds to errors
func recordClarificationRequest(level ClarificationLevel, iteration int, text string, result *model.ExecutionResult) {
	preview := TruncateString(text, 200)
	msg := fmt.Sprintf("LLM asked for clarification instead of acting (iteration %d): %s", iteration, preview)

	// Initialize stats if needed
	if result.ClarificationStats == nil {
		result.ClarificationStats = &model.ClarificationStats{
			Iterations: []int{},
			Examples:   []string{},
		}
	}

	// Record the detection
	result.ClarificationStats.Count++
	result.ClarificationStats.Iterations = append(result.ClarificationStats.Iterations, iteration)
	// Keep up to 3 examples
	if len(result.ClarificationStats.Examples) < 3 {
		result.ClarificationStats.Examples = append(result.ClarificationStats.Examples, preview)
	}

	// Log and optionally add to errors based on level
	switch level {
	case ClarificationLevelInfo:
		logger.Logger.Info("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		// Info level does not add to errors
	case ClarificationLevelError:
		logger.Logger.Error("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		result.Errors = append(result.Errors, msg)
	default: // warning is the default
		logger.Logger.Warn("LLM asked for clarification instead of acting",
			"iteration", iteration,
			"response_preview", preview)
		result.Errors = append(result.Errors, msg)
	}
}

// ============================================================================
// AI SUMMARY - LLM-GENERATED EXECUTIVE SUMMARY
// ============================================================================

// aiSummaryPrompt is embedded from prompts/ai_summary.md at compile time.
// Edit that file to modify the prompt - changes require recompilation.
//
//go:embed prompts/ai_summary.md
var aiSummaryPrompt string

// AISummaryResult contains the generated analysis or error information
type AISummaryResult struct {
	Success   bool   `json:"success"`
	Analysis  string `json:"analysis,omitempty"`  // Markdown content if successful
	Error     string `json:"error,omitempty"`     // Error message if failed
	Retryable bool   `json:"retryable,omitempty"` // Whether the error is retryable
	Guidance  string `json:"guidance,omitempty"`  // Actionable suggestion for the user
}

// GenerateAISummary uses an LLM to generate an executive summary of test results.
// It takes the full test results and produces a markdown analysis.
// Returns an AISummaryResult with either the analysis or error information.
func GenerateAISummary(ctx context.Context, judgeLLM llms.Model, results []model.TestRun) AISummaryResult {
	if judgeLLM == nil {
		return AISummaryResult{
			Success:   false,
			Error:     "AI summary LLM is nil",
			Retryable: false,
			Guidance:  "Configure a valid judge_provider in ai_summary settings. Use '$self' to reuse an agent's provider, or specify a provider name.",
		}
	}

	// Create a context with 60-second timeout for large test suites
	analysisCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Prepare a summary of the test results for the LLM
	resultsSummary := prepareResultsSummary(results)

	// Prepare the messages for analysis
	msgs := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: aiSummaryPrompt},
			},
		},
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: fmt.Sprintf("Analyze these test results:\n\n%s", resultsSummary)},
			},
		},
	}

	// Call the analysis LLM
	resp, err := judgeLLM.GenerateContent(analysisCtx, msgs)
	if err != nil {
		// Check if it's a context timeout
		if ctx.Err() == context.DeadlineExceeded || analysisCtx.Err() == context.DeadlineExceeded {
			return AISummaryResult{
				Success:   false,
				Error:     "Analysis timed out after 60 seconds",
				Retryable: true,
				Guidance:  "The test results may be too large. Try running with fewer tests, or use a model with higher throughput.",
			}
		}
		return AISummaryResult{
			Success:   false,
			Error:     fmt.Sprintf("LLM call failed: %v", err),
			Retryable: true,
			Guidance:  "Check API connectivity and credentials. Ensure the judge_provider is correctly configured.",
		}
	}

	if len(resp.Choices) == 0 {
		return AISummaryResult{
			Success:   false,
			Error:     "LLM returned no response",
			Retryable: true,
			Guidance:  "The model may be overloaded. Try again, or use a different judge_provider.",
		}
	}

	analysis := resp.Choices[0].Content
	if strings.TrimSpace(analysis) == "" {
		return AISummaryResult{
			Success:   false,
			Error:     "LLM returned empty analysis",
			Retryable: true,
			Guidance:  "The model returned an empty response. Try a different model with better instruction following.",
		}
	}

	return AISummaryResult{
		Success:  true,
		Analysis: analysis,
	}
}

// prepareResultsSummary creates a structured summary of test results for the LLM
func prepareResultsSummary(results []model.TestRun) string {
	if len(results) == 0 {
		return "No test results available."
	}

	var sb strings.Builder

	// Count unique agents first to determine evaluation context
	agentSet := make(map[string]bool)
	for _, r := range results {
		agentSet[r.Execution.AgentName] = true
	}
	agentCount := len(agentSet)

	// Evaluation context header
	if agentCount == 1 {
		sb.WriteString("## Evaluation Context: SINGLE AGENT\n")
		sb.WriteString("Use the **Single-Agent Evaluation** template (fit for purpose analysis).\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("## Evaluation Context: MULTIPLE AGENTS (%d)\n", agentCount))
		sb.WriteString("Use the **Multi-Agent Comparison** template (which to choose analysis).\n\n")
	}

	// Overall stats
	total := len(results)
	passed := 0
	failed := 0
	totalDuration := time.Duration(0)
	totalTokens := 0

	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
		totalDuration += r.Execution.EndTime.Sub(r.Execution.StartTime)
		totalTokens += r.Execution.TokensUsed
	}

	sb.WriteString(fmt.Sprintf("## Test Run Overview\n"))
	sb.WriteString(fmt.Sprintf("- Total Tests: %d\n", total))
	sb.WriteString(fmt.Sprintf("- Passed: %d (%.1f%%)\n", passed, float64(passed)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("- Failed: %d (%.1f%%)\n", failed, float64(failed)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("- Total Duration: %.1fs\n", totalDuration.Seconds()))
	sb.WriteString(fmt.Sprintf("- Total Tokens: %d\n\n", totalTokens))

	// Agent breakdown with efficiency metrics
	type agentMetrics struct {
		total, passed, failed int
		provider              string
		totalTokens           int
		totalDurationMs       int64
	}
	agentStats := make(map[string]*agentMetrics)

	for _, r := range results {
		stats, exists := agentStats[r.Execution.AgentName]
		if !exists {
			stats = &agentMetrics{provider: string(r.Execution.ProviderType)}
			agentStats[r.Execution.AgentName] = stats
		}
		stats.total++
		if r.Passed {
			stats.passed++
		} else {
			stats.failed++
		}
		stats.totalTokens += r.Execution.TokensUsed
		stats.totalDurationMs += r.Execution.LatencyMs
	}

	sb.WriteString("## Agent Performance\n")
	for agent, stats := range agentStats {
		passRate := float64(stats.passed) / float64(stats.total) * 100
		avgTokens := float64(stats.totalTokens) / float64(stats.total)
		avgDuration := float64(stats.totalDurationMs) / float64(stats.total) / 1000.0 // Convert to seconds

		// Calculate tokens per successful test (efficiency metric)
		tokensPerSuccess := 0.0
		if stats.passed > 0 {
			tokensPerSuccess = float64(stats.totalTokens) / float64(stats.passed)
		}

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", agent, stats.provider))
		sb.WriteString(fmt.Sprintf("- Pass Rate: %.1f%% (%d/%d)\n", passRate, stats.passed, stats.total))
		sb.WriteString(fmt.Sprintf("- Total Tokens: %d\n", stats.totalTokens))
		sb.WriteString(fmt.Sprintf("- Avg Tokens/Test: %.0f\n", avgTokens))
		if stats.passed > 0 {
			sb.WriteString(fmt.Sprintf("- Tokens/Success: %.0f (efficiency metric)\n", tokensPerSuccess))
		}
		sb.WriteString(fmt.Sprintf("- Avg Duration: %.2fs\n", avgDuration))
		sb.WriteString(fmt.Sprintf("- Failed: %d\n\n", stats.failed))
	}

	// Tool usage analysis - helps understand strategy differences
	sb.WriteString("## Tool Usage Patterns\n")
	for _, r := range results {
		if len(r.Execution.ToolCalls) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s (Agent: %s) - %s\n",
			r.Execution.TestName, r.Execution.AgentName,
			map[bool]string{true: "PASSED", false: "FAILED"}[r.Passed]))

		// Count tool calls and errors by tool name
		toolCounts := make(map[string]int)
		toolErrors := make(map[string][]string)
		toolParams := make(map[string][]string) // Track parameters for each tool
		for _, tc := range r.Execution.ToolCalls {
			toolCounts[tc.Name]++

			// Capture key parameters (truncated for readability)
			paramsJSON, _ := json.Marshal(tc.Parameters)
			params := string(paramsJSON)
			if len(params) > 150 {
				params = params[:150] + "..."
			}
			toolParams[tc.Name] = append(toolParams[tc.Name], params)

			// Check if result contains error indicators
			if len(tc.Result.Content) > 0 {
				for _, content := range tc.Result.Content {
					text := content.Text
					// Look for common error patterns in tool results
					if strings.Contains(text, `"ok":false`) ||
						strings.Contains(text, `"error"`) ||
						strings.Contains(text, `"et":"`) { // error type field
						// Extract error type if present
						errType := "unknown"
						if idx := strings.Index(text, `"et":"`); idx != -1 {
							end := strings.Index(text[idx+6:], `"`)
							if end != -1 {
								errType = text[idx+6 : idx+6+end]
							}
						} else if idx := strings.Index(text, `"ec":"`); idx != -1 {
							end := strings.Index(text[idx+6:], `"`)
							if end != -1 {
								errType = text[idx+6 : idx+6+end]
							}
						}
						toolErrors[tc.Name] = append(toolErrors[tc.Name], errType)
					}
				}
			}
		}

		// Show tool usage summary
		sb.WriteString("- Tools used: ")
		toolList := make([]string, 0, len(toolCounts))
		for tool, count := range toolCounts {
			errCount := len(toolErrors[tool])
			if errCount > 0 {
				toolList = append(toolList, fmt.Sprintf("%s×%d (%d errors)", tool, count, errCount))
			} else {
				toolList = append(toolList, fmt.Sprintf("%s×%d", tool, count))
			}
		}
		sb.WriteString(strings.Join(toolList, ", ") + "\n")

		// Show key tool parameters (important for understanding strategy differences)
		sb.WriteString("- Key tool parameters:\n")
		for tool, params := range toolParams {
			// Show first call's params (or unique params if they differ)
			if len(params) > 0 {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", tool, params[0]))
			}
		}

		// Show error types if any
		for tool, errs := range toolErrors {
			errCounts := make(map[string]int)
			for _, e := range errs {
				errCounts[e]++
			}
			for errType, count := range errCounts {
				sb.WriteString(fmt.Sprintf("  - %s error '%s' occurred %d times\n", tool, errType, count))
			}
		}

		sb.WriteString("\n")
	}

	// Detailed results with final outputs
	sb.WriteString("## Test Details (with final outputs)\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("### %s (Agent: %s) - %s\n",
			r.Execution.TestName, r.Execution.AgentName,
			map[bool]string{true: "PASSED", false: "FAILED"}[r.Passed]))

		// Show failed assertions
		for _, a := range r.Assertions {
			if !a.Passed {
				sb.WriteString(fmt.Sprintf("- **FAILED %s**: %s\n", a.Type, a.Message))
			}
		}

		// Show errors
		if len(r.Execution.Errors) > 0 {
			sb.WriteString("- Errors:\n")
			for _, e := range r.Execution.Errors {
				if len(e) > 200 {
					e = e[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("  - %s\n", e))
			}
		}

		// Show clarification stats if relevant
		if r.Execution.ClarificationStats != nil && r.Execution.ClarificationStats.Count > 0 {
			sb.WriteString(fmt.Sprintf("- Clarification requests: %d\n", r.Execution.ClarificationStats.Count))
			if len(r.Execution.ClarificationStats.Examples) > 0 {
				sb.WriteString("  - Example: " + r.Execution.ClarificationStats.Examples[0] + "\n")
			}
		}

		// Show final assistant message (crucial for understanding agent behavior)
		for i := len(r.Execution.Messages) - 1; i >= 0; i-- {
			msg := r.Execution.Messages[i]
			if msg.Role == "assistant" && msg.Content != "" {
				content := msg.Content
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("- Final output: \"%s\"\n", content))
				break
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
