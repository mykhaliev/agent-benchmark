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
	Name                     string           `json:"name"`
	Type                     model.ServerType `json:"type"`
	Command                  string           `json:"command"`
	Shell                    string           `json:"shell"`
	WorkingDir               string           `json:"working_dir"`
	ToolPrefix               string           `json:"tool_prefix"`
	HelpCommand              string           `json:"help_command"`                // DEPRECATED: Use help_commands instead
	HelpCommands             []string         `json:"help_commands"`               // Commands to run at startup for help content
	DisableHelpAutoDiscovery bool             `json:"disable_help_auto_discovery"` // If true, don't auto-discover help

	// tools discovered or configured for this CLI
	tools []mcp.Tool
	// Track command executions for assertions
	executions []CLIExecution
	// Help content from running help_command(s) at startup
	helpContent string
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
		Name:                     serverConfig.Name,
		Type:                     serverConfig.Type,
		Command:                  serverConfig.Command,
		Shell:                    serverConfig.Shell,
		WorkingDir:               serverConfig.WorkingDir,
		ToolPrefix:               serverConfig.ToolPrefix,
		HelpCommand:              serverConfig.HelpCommand,
		HelpCommands:             serverConfig.HelpCommands,
		DisableHelpAutoDiscovery: serverConfig.DisableHelpAutoDiscovery,
		executions:               []CLIExecution{},
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

	// Run help_command(s) if configured to get CLI help content
	// Support both single help_command (backwards compat) and help_commands array
	helpCommands := s.HelpCommands
	if len(helpCommands) == 0 && s.HelpCommand != "" {
		helpCommands = []string{s.HelpCommand}
	}

	if len(helpCommands) > 0 {
		// Explicit help commands configured - use them
		s.helpContent = s.runHelpCommands(ctx, helpCommands)
		if logger.Logger != nil {
			if s.helpContent != "" {
				logger.Logger.Info("CLI help content loaded",
					"server_name", serverConfig.Name,
					"help_commands_count", len(helpCommands),
					"content_length", len(s.helpContent),
				)
			} else {
				logger.Logger.Warn("CLI help commands returned no content",
					"server_name", serverConfig.Name,
					"help_commands_count", len(helpCommands),
				)
			}
		}
	} else {
		// No explicit help commands - try auto-discovery (unless disabled)
		if !s.DisableHelpAutoDiscovery {
			if logger.Logger != nil {
				logger.Logger.Debug("No help_command configured, attempting auto-discovery",
					"server_name", serverConfig.Name,
				)
			}
			s.helpContent = s.tryAutoDiscoverHelp(ctx)
			if s.helpContent != "" {
				// Auto-discovered content - now try to discover subcommands
				subcommands := s.parseSubcommands(s.helpContent)
				if len(subcommands) > 0 {
					if logger.Logger != nil {
						logger.Logger.Debug("Auto-discovered subcommands from help output",
							"server_name", serverConfig.Name,
							"count", len(subcommands),
							"subcommands", subcommands,
						)
					}

					var allHelp strings.Builder
					allHelp.WriteString(s.helpContent)

					// Run help for each subcommand
					for _, subcmd := range subcommands {
						subHelpCmd := s.Command + " " + subcmd + " --help"
						subHelp := s.runSingleHelpCommand(ctx, subHelpCmd)
						if subHelp != "" {
							allHelp.WriteString("\n\n")
							allHelp.WriteString("=== " + subcmd + " ===\n")
							allHelp.WriteString(subHelp)
						}
					}
					s.helpContent = allHelp.String()
				}
			}
		} else if logger.Logger != nil {
			logger.Logger.Debug("Help auto-discovery disabled",
				"server_name", serverConfig.Name,
			)
		}
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

	// Build tool description, including help content if available
	description := fmt.Sprintf("Execute %s CLI command with arguments.", s.Command)
	if s.helpContent != "" {
		description = fmt.Sprintf("Execute %s CLI command with arguments.\n\nAvailable commands and options:\n%s", s.Command, s.helpContent)
	}

	tool := mcp.Tool{
		Name:        toolName,
		Description: description,
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

// tryAutoDiscoverHelp attempts to discover help content by trying common help patterns
// Returns the help content from the first successful pattern, or empty string if none work
func (s *CLIServer) tryAutoDiscoverHelp(ctx context.Context) string {
	// Common help patterns to try, in order of preference
	// Most CLIs support at least one of these
	helpPatterns := []string{
		"--help",
		"-h",
		"help",
	}

	// Add Windows-specific pattern
	if runtime.GOOS == "windows" {
		helpPatterns = append(helpPatterns, "/?")
	}

	for _, pattern := range helpPatterns {
		helpCmd := s.Command + " " + pattern
		if logger.Logger != nil {
			logger.Logger.Debug("Trying auto-discover help pattern",
				"server_name", s.Name,
				"pattern", pattern,
				"command", helpCmd,
			)
		}

		content := s.runSingleHelpCommand(ctx, helpCmd)
		if content != "" {
			if logger.Logger != nil {
				logger.Logger.Info("Auto-discovered help content",
					"server_name", s.Name,
					"pattern", pattern,
					"content_length", len(content),
				)
			}
			return content
		}
	}

	if logger.Logger != nil {
		logger.Logger.Debug("Auto-discovery found no help content",
			"server_name", s.Name,
			"patterns_tried", helpPatterns,
		)
	}
	return ""
}

// runHelpCommands executes help commands and automatically discovers subcommands
// If help_commands is empty but help_command is set, it will:
// 1. Run the initial help command
// 2. Parse the output to find COMMANDS section
// 3. Automatically run help for each discovered subcommand
func (s *CLIServer) runHelpCommands(ctx context.Context, helpCommands []string) string {
	if len(helpCommands) == 0 {
		return ""
	}

	// Run the first (main) help command
	mainHelp := s.runSingleHelpCommand(ctx, helpCommands[0])
	if mainHelp == "" {
		return ""
	}

	var allHelp strings.Builder
	allHelp.WriteString(mainHelp)

	// If only one help command provided, try to auto-discover subcommands
	if len(helpCommands) == 1 {
		subcommands := s.parseSubcommands(mainHelp)
		if len(subcommands) > 0 {
			if logger.Logger != nil {
				logger.Logger.Debug("Auto-discovered subcommands from help output",
					"count", len(subcommands),
					"subcommands", subcommands,
				)
			}

			// Run help for each subcommand
			for _, subcmd := range subcommands {
				// Build subcommand help command (e.g., "excelcli.exe session --help")
				subHelpCmd := s.Command + " " + subcmd + " --help"
				subHelp := s.runSingleHelpCommand(ctx, subHelpCmd)
				if subHelp != "" {
					allHelp.WriteString("\n\n")
					allHelp.WriteString("=== " + subcmd + " ===\n")
					allHelp.WriteString(subHelp)
				}
			}
		}
	} else {
		// Multiple help commands explicitly provided - run them all
		for i := 1; i < len(helpCommands); i++ {
			output := s.runSingleHelpCommand(ctx, helpCommands[i])
			if output != "" {
				allHelp.WriteString("\n\n--- Help Command ")
				allHelp.WriteString(fmt.Sprintf("%d", i+1))
				allHelp.WriteString(" ---\n")
				allHelp.WriteString(output)
			}
		}
	}

	return allHelp.String()
}

// parseSubcommands extracts subcommand names from CLI help output
// Looks for patterns like:
//
//	COMMANDS:
//	    session <FILE>    Description
//	    range <ACTION>    Description
func (s *CLIServer) parseSubcommands(helpOutput string) []string {
	var subcommands []string
	lines := strings.Split(helpOutput, "\n")

	inCommandsSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of COMMANDS section
		if strings.HasPrefix(trimmed, "COMMANDS:") || trimmed == "COMMANDS" {
			inCommandsSection = true
			continue
		}

		// Detect end of COMMANDS section (next section header or empty line after commands)
		if inCommandsSection {
			// Section headers typically end with ":"
			if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
				break
			}

			// Parse command line - typically "    command <args>    Description"
			if trimmed != "" && !strings.HasPrefix(trimmed, "-") {
				// Extract first word (the command name)
				parts := strings.Fields(trimmed)
				if len(parts) > 0 {
					cmd := parts[0]
					// Skip if it looks like a flag or option
					if !strings.HasPrefix(cmd, "-") && !strings.HasPrefix(cmd, "<") {
						subcommands = append(subcommands, cmd)
					}
				}
			}
		}
	}

	return subcommands
}

// runSingleHelpCommand executes a single help command and returns its output
func (s *CLIServer) runSingleHelpCommand(ctx context.Context, helpCmd string) string {
	if helpCmd == "" {
		return ""
	}

	// Create a timeout context for help command (10 seconds max)
	helpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build the shell command
	var cmd *exec.Cmd
	switch strings.ToLower(s.Shell) {
	case "powershell", "pwsh":
		shellExe := "powershell"
		if s.Shell == "pwsh" {
			shellExe = "pwsh"
		}
		cmd = exec.CommandContext(helpCtx, shellExe, "-NoProfile", "-NonInteractive", "-Command", helpCmd)
	case "cmd":
		cmd = exec.CommandContext(helpCtx, "cmd", "/C", helpCmd)
	case "bash", "sh", "zsh":
		cmd = exec.CommandContext(helpCtx, s.Shell, "-c", helpCmd)
	default:
		if logger.Logger != nil {
			logger.Logger.Warn("Unsupported shell for help command", "shell", s.Shell)
		}
		return ""
	}

	cmd.Dir = s.WorkingDir

	// Capture stdout
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // Ignore stderr for help commands

	// Run the command
	err := cmd.Run()
	if err != nil {
		if logger.Logger != nil {
			logger.Logger.Warn("help command failed",
				"command", helpCmd,
				"error", err,
			)
		}
		// Return empty string on error - don't fail server startup
		return ""
	}

	return strings.TrimSpace(stdout.String())
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
	// Arguments can be either map[string]interface{} or json.RawMessage depending on caller
	args := ""
	if request.Params.Arguments != nil {
		switch v := request.Params.Arguments.(type) {
		case map[string]interface{}:
			if argsVal, exists := v["args"]; exists {
				args = fmt.Sprintf("%v", argsVal)
			}
		case json.RawMessage:
			// Unmarshal the raw JSON to extract args
			var argsMap map[string]interface{}
			if err := json.Unmarshal(v, &argsMap); err == nil {
				if argsVal, exists := argsMap["args"]; exists {
					args = fmt.Sprintf("%v", argsVal)
				}
			}
		case []byte:
			// Also handle []byte (which json.RawMessage is an alias for)
			var argsMap map[string]interface{}
			if err := json.Unmarshal(v, &argsMap); err == nil {
				if argsVal, exists := argsMap["args"]; exists {
					args = fmt.Sprintf("%v", argsVal)
				}
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
