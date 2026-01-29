package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
)

// CLIServer wraps CLI commands as MCP-like tools.
// It allows testing CLI-based tools (like excel-cli) using the same
// agent-benchmark framework used for MCP servers.
type CLIServer struct {
	Name       string           `json:"name"`
	Type       model.ServerType `json:"type"`
	Command    string           `json:"command"`
	Shell      string           `json:"shell"`
	WorkingDir string           `json:"working_dir"`
	ToolPrefix string           `json:"tool_prefix"`

	// tools discovered or configured for this CLI
	tools []mcp.Tool
	// Track command executions for assertions
	executions []CLIExecution
}

// CLIExecution records a CLI command execution for assertion evaluation
type CLIExecution struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	FullCmd    string            `json:"full_cmd"`
	ExitCode   int               `json:"exit_code"`
	Stdout     string            `json:"stdout"`
	Stderr     string            `json:"stderr"`
	DurationMs int64             `json:"duration_ms"`
	Timestamp  time.Time         `json:"timestamp"`
	Params     map[string]string `json:"params"` // Parsed parameters if applicable
}

// CLIClient implements a minimal interface compatible with agent expectations.
// It wraps CLI commands and presents them as MCP tools.
type CLIClient struct {
	server *CLIServer
}

// NewCLIServer creates a CLI server that wraps command-line tools.
func NewCLIServer(ctx context.Context, serverConfig model.Server) (*CLIServer, error) {
	if logger.Logger != nil {
		logger.Logger.Info("Creating CLI server",
			"server_name", serverConfig.Name,
			"command", serverConfig.Command,
		)
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	s := &CLIServer{
		Name:       serverConfig.Name,
		Type:       serverConfig.Type,
		Command:    serverConfig.Command,
		Shell:      serverConfig.Shell,
		WorkingDir: serverConfig.WorkingDir,
		ToolPrefix: serverConfig.ToolPrefix,
		executions: []CLIExecution{},
	}

	// Set default shell based on OS
	if s.Shell == "" {
		if runtime.GOOS == "windows" {
			s.Shell = "powershell"
		} else {
			s.Shell = "bash"
		}
	}

	// Set default working directory
	if s.WorkingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		s.WorkingDir = cwd
	}

	// Validate configuration
	if err := s.validate(); err != nil {
		if logger.Logger != nil {
			logger.Logger.Error("CLI server configuration validation failed",
				"server_name", serverConfig.Name,
				"error", err,
			)
		}
		return nil, fmt.Errorf("invalid CLI server configuration for %s: %w", serverConfig.Name, err)
	}

	// Create default tool (the CLI itself as a single tool)
	s.tools = s.createDefaultTools()

	if logger.Logger != nil {
		logger.Logger.Info("CLI server successfully initialized",
			"server_name", serverConfig.Name,
			"shell", s.Shell,
			"working_dir", s.WorkingDir,
			"tools_count", len(s.tools),
		)
	}

	return s, nil
}

func (s *CLIServer) validate() error {
	if s.Name == "" {
		return fmt.Errorf("server name cannot be empty")
	}

	if s.Command == "" {
		return fmt.Errorf("command is required for CLI server type")
	}

	// Validate shell
	validShells := map[string]bool{
		"powershell": true,
		"pwsh":       true,
		"cmd":        true,
		"bash":       true,
		"sh":         true,
		"zsh":        true,
	}
	if !validShells[strings.ToLower(s.Shell)] {
		return fmt.Errorf("unsupported shell: %s (supported: powershell, pwsh, cmd, bash, sh, zsh)", s.Shell)
	}

	// Validate working directory exists
	if _, err := os.Stat(s.WorkingDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", s.WorkingDir)
	}

	return nil
}

// createDefaultTools creates the default tool representing the CLI command
func (s *CLIServer) createDefaultTools() []mcp.Tool {
	toolName := "cli_execute"
	if s.ToolPrefix != "" {
		toolName = s.ToolPrefix + "_execute"
	}

	tool := mcp.Tool{
		Name:        toolName,
		Description: fmt.Sprintf("Execute %s CLI command with arguments", s.Command),
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"args": map[string]interface{}{
					"type":        "string",
					"description": "Command-line arguments to pass to the CLI",
				},
			},
			Required: []string{},
		},
	}

	return []mcp.Tool{tool}
}

// GetClient returns a CLIClient that wraps this server
func (s *CLIServer) GetClient() *CLIClient {
	return &CLIClient{server: s}
}

// Execute runs the CLI command with the given arguments
func (s *CLIServer) Execute(ctx context.Context, args string) (*CLIExecution, error) {
	startTime := time.Now()

	// Build the full command
	fullCmd := s.Command
	if args != "" {
		fullCmd = fullCmd + " " + args
	}

	if logger.Logger != nil {
		logger.Logger.Debug("Executing CLI command",
			"server_name", s.Name,
			"full_cmd", fullCmd,
			"shell", s.Shell,
			"working_dir", s.WorkingDir,
		)
	}

	// Build the shell command
	var cmd *exec.Cmd
	switch strings.ToLower(s.Shell) {
	case "powershell", "pwsh":
		shellExe := "powershell"
		if s.Shell == "pwsh" {
			shellExe = "pwsh"
		}
		cmd = exec.CommandContext(ctx, shellExe, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd", "/C", fullCmd)
	case "bash", "sh", "zsh":
		cmd = exec.CommandContext(ctx, s.Shell, "-c", fullCmd)
	default:
		return nil, fmt.Errorf("unsupported shell: %s", s.Shell)
	}

	cmd.Dir = s.WorkingDir

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()

	execution := &CLIExecution{
		Command:    s.Command,
		Args:       strings.Fields(args),
		FullCmd:    fullCmd,
		ExitCode:   0,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: time.Since(startTime).Milliseconds(),
		Timestamp:  startTime,
		Params:     parseArgsToParams(args),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			execution.ExitCode = exitError.ExitCode()
		} else {
			// Command failed to run at all
			execution.ExitCode = -1
			execution.Stderr = err.Error()
		}
	}

	// Record the execution
	s.executions = append(s.executions, *execution)

	if logger.Logger != nil {
		logger.Logger.Debug("CLI command completed",
			"server_name", s.Name,
			"exit_code", execution.ExitCode,
			"duration_ms", execution.DurationMs,
			"stdout_len", len(execution.Stdout),
			"stderr_len", len(execution.Stderr),
		)
	}

	return execution, nil
}

// GetExecutions returns all recorded command executions
func (s *CLIServer) GetExecutions() []CLIExecution {
	return s.executions
}

// ClearExecutions clears the execution history
func (s *CLIServer) ClearExecutions() {
	s.executions = []CLIExecution{}
}

// Close cleans up the CLI server resources
func (s *CLIServer) Close() error {
	if logger.Logger != nil {
		logger.Logger.Info("Closing CLI server", "server_name", s.Name)
	}
	s.executions = []CLIExecution{}
	return nil
}

// parseArgsToParams attempts to parse CLI arguments into a parameter map
// Supports formats like: --key value, --key=value, -k value
func parseArgsToParams(args string) map[string]string {
	params := make(map[string]string)
	parts := strings.Fields(args)

	for i := 0; i < len(parts); i++ {
		part := parts[i]

		// Handle --key=value format
		if strings.HasPrefix(part, "--") && strings.Contains(part, "=") {
			kv := strings.SplitN(part[2:], "=", 2)
			if len(kv) == 2 {
				params[kv[0]] = kv[1]
			}
			continue
		}

		// Handle --key value format
		if strings.HasPrefix(part, "--") {
			key := part[2:]
			if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
				params[key] = parts[i+1]
				i++
			} else {
				params[key] = "true" // Flag without value
			}
			continue
		}

		// Handle -k value format (single character flags)
		if strings.HasPrefix(part, "-") && len(part) == 2 {
			key := part[1:]
			if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
				params[key] = parts[i+1]
				i++
			} else {
				params[key] = "true"
			}
			continue
		}
	}

	return params
}

// ============================================================================
// CLIClient - MCP-like interface for CLI commands
// ============================================================================

// ListTools returns the tools (CLI commands) available
func (c *CLIClient) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{
		Tools: c.server.tools,
	}, nil
}

// ListToolsByPage returns tools with pagination (same as ListTools for CLI)
func (c *CLIClient) ListToolsByPage(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return c.ListTools(ctx, request)
}

// CallTool executes a CLI command
func (c *CLIClient) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract args from the request
	args := ""
	if request.Params.Arguments != nil {
		if argsMap, ok := request.Params.Arguments.(map[string]interface{}); ok {
			if argsVal, exists := argsMap["args"]; exists {
				args = fmt.Sprintf("%v", argsVal)
			}
		}
	}

	execution, err := c.server.Execute(ctx, args)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error executing command: %s", err.Error()),
				},
			},
			IsError: true,
		}, nil
	}

	// Build result content
	resultContent := struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}{
		ExitCode: execution.ExitCode,
		Stdout:   execution.Stdout,
		Stderr:   execution.Stderr,
	}

	resultJSON, _ := json.Marshal(resultContent)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(resultJSON),
			},
		},
		IsError: execution.ExitCode != 0,
	}, nil
}

// Initialize is a no-op for CLI servers (no MCP handshake needed)
func (c *CLIClient) Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	return &mcp.InitializeResult{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ServerInfo: mcp.Implementation{
			Name:    "cli-server",
			Version: "1.0.0",
		},
		Capabilities: mcp.ServerCapabilities{
			Tools: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{},
		},
	}, nil
}

// Ping is a no-op for CLI servers
func (c *CLIClient) Ping(ctx context.Context) error {
	return nil
}

// ListResources returns empty for CLI servers (no resources)
func (c *CLIClient) ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return &mcp.ListResourcesResult{
		Resources: []mcp.Resource{},
	}, nil
}

// ListResourcesByPage returns empty for CLI servers (no resources)
func (c *CLIClient) ListResourcesByPage(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return c.ListResources(ctx, request)
}

// ListResourceTemplates returns empty for CLI servers (no resource templates)
func (c *CLIClient) ListResourceTemplates(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return &mcp.ListResourceTemplatesResult{
		ResourceTemplates: []mcp.ResourceTemplate{},
	}, nil
}

// ListResourceTemplatesByPage returns empty for CLI servers (no resource templates)
func (c *CLIClient) ListResourceTemplatesByPage(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return c.ListResourceTemplates(ctx, request)
}

// ReadResource returns error for CLI servers (no resources)
func (c *CLIClient) ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return nil, fmt.Errorf("CLI server does not support resources")
}

// Subscribe is not supported by CLI servers
func (c *CLIClient) Subscribe(ctx context.Context, request mcp.SubscribeRequest) error {
	return fmt.Errorf("CLI server does not support subscriptions")
}

// Unsubscribe is not supported by CLI servers
func (c *CLIClient) Unsubscribe(ctx context.Context, request mcp.UnsubscribeRequest) error {
	return fmt.Errorf("CLI server does not support subscriptions")
}

// ListPrompts returns empty for CLI servers (no prompts)
func (c *CLIClient) ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return &mcp.ListPromptsResult{
		Prompts: []mcp.Prompt{},
	}, nil
}

// ListPromptsByPage returns empty for CLI servers (no prompts)
func (c *CLIClient) ListPromptsByPage(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return c.ListPrompts(ctx, request)
}

// GetPrompt returns error for CLI servers (no prompts)
func (c *CLIClient) GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return nil, fmt.Errorf("CLI server does not support prompts")
}

// SetLevel is a no-op for CLI servers
func (c *CLIClient) SetLevel(ctx context.Context, request mcp.SetLevelRequest) error {
	return nil
}

// Complete returns empty completions for CLI servers
func (c *CLIClient) Complete(ctx context.Context, request mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return &mcp.CompleteResult{
		Completion: struct {
			Values  []string `json:"values"`
			Total   int      `json:"total,omitempty"`
			HasMore bool     `json:"hasMore,omitempty"`
		}{
			Values: []string{},
		},
	}, nil
}

// OnNotification registers a notification handler (no-op for CLI)
func (c *CLIClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	// CLI servers don't send notifications
}

// Close cleans up the CLI client
func (c *CLIClient) Close() error {
	return c.server.Close()
}
