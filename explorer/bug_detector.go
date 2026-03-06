package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/tmc/langchaingo/llms"
)

const bugDetectorSystemPrompt = `You are a server-side error analyst for MCP (Model Context Protocol) tool responses.

Your job is to detect server-side bugs in tool responses: crashes, timeouts, stack traces, malformed output, or error keywords. Do NOT flag normal errors that are part of expected business logic — only flag genuine server-side failures.

Valid bug_type values:
- EMPTY_RESPONSE: The tool returned no content at all
- TIMEOUT: The response indicates a timeout or deadline exceeded
- SERVER_CRASH: The response indicates the server process crashed or was killed
- STACKTRACE_RETURNED: The response contains a language stack trace (Python, Go, Java, etc.)
- MALFORMED_JSON: The response looks like JSON but is syntactically invalid
- UNEXPECTED_TEXT_RESPONSE: The response contains error keywords (internal server error, unhandled exception, fatal error, etc.)
- SCHEMA_MISMATCH: The response content type is not the standard MCP "text" type

Respond with ONLY a JSON object, no markdown:
{"is_bug": true/false, "bug_type": "<one of the valid types above or empty string>", "explanation": "<concise human-readable description>"}`

// llmBugAnalysis is the structured JSON response from the bug-detection LLM.
type llmBugAnalysis struct {
	IsBug       bool   `json:"is_bug"`
	BugType     string `json:"bug_type"`
	Explanation string `json:"explanation"`
}

// AnalyzeToolCallWithLLM uses an LLM to detect server-side errors in a single
// tool call response. It returns the detected BugType, a human-readable
// explanation, whether a bug was detected, and any error encountered.
//
// Pre-flight: empty Content slices are detected without an LLM call.
// Fallback: if the LLM call or JSON parse fails, (no-bug, nil) is returned
// so that exploration continues uninterrupted.
func AnalyzeToolCallWithLLM(
	ctx context.Context,
	llm llms.Model,
	tc model.ToolCall,
	totalTokens *int,
) (BugType, string, bool, error) {
	// Pre-flight: empty response check avoids an unnecessary LLM call.
	if len(tc.Result.Content) == 0 && len(tc.Result.StructuredContent.Result) == 0 {
		return BugTypeEmptyResponse, "tool returned empty response with no content", true, nil
	}

	raw := extractRawText(tc.Result)

	msgs := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: bugDetectorSystemPrompt}},
		},
		{
			Role:  llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{llms.TextContent{Text: buildBugDetectorPrompt(tc, raw)}},
		},
	}

	resp, err := llm.GenerateContent(ctx, msgs)
	if err != nil {
		logger.Logger.Warn("LLM bug analysis call failed", "tool", tc.Name, "error", err)
		return "", "", false, nil
	}

	if totalTokens != nil {
		*totalTokens += agent.GetTokenCount(resp)
	}

	rawContent := ""
	for _, choice := range resp.Choices {
		if choice.Content != "" {
			rawContent = choice.Content
			break
		}
	}
	if rawContent == "" {
		logger.Logger.Warn("LLM bug analysis returned empty content", "tool", tc.Name)
		return "", "", false, nil
	}

	jsonStr := ExtractJSONFromResponse(rawContent)
	var analysis llmBugAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		logger.Logger.Warn("Failed to parse LLM bug analysis JSON", "tool", tc.Name, "error", err)
		return "", "", false, nil
	}

	if !analysis.IsBug {
		return "", "", false, nil
	}

	return BugType(analysis.BugType), analysis.Explanation, true, nil
}

// buildBugDetectorPrompt constructs the human-turn message for the bug-detection LLM.
func buildBugDetectorPrompt(tc model.ToolCall, raw string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("TOOL NAME: %s\n\n", tc.Name))

	if len(tc.Parameters) > 0 {
		paramsJSON, err := json.Marshal(tc.Parameters)
		if err == nil {
			params := string(paramsJSON)
			if len(params) > 500 {
				params = params[:500] + "...(truncated)"
			}
			sb.WriteString(fmt.Sprintf("TOOL PARAMETERS:\n%s\n\n", params))
		}
	}

	if len(tc.Result.Content) > 0 {
		sb.WriteString(fmt.Sprintf("CONTENT TYPE: %s\n\n", tc.Result.Content[0].Type))
	}

	responseText := raw
	if len(responseText) > 2000 {
		responseText = responseText[:2000] + "...(truncated)"
	}
	sb.WriteString(fmt.Sprintf("RAW RESPONSE:\n%s\n\n", responseText))
	sb.WriteString("Analyze the response above. Is this a server-side bug? Respond with only a JSON object.")

	return sb.String()
}

// extractRawText joins all Content[].Text values from a Result into a single
// string. Most MCP tools return a single content item; this handles the general
// case.
func extractRawText(r model.Result) string {
	if len(r.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Content))
	for _, item := range r.Content {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}
