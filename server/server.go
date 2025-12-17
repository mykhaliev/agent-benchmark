package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/logger" // Adjust import path as needed
	"github.com/mykhaliev/agent-benchmark/model"
)

const (
	DefaultServerInitDelay = 30 * time.Second
	ProcessStartupDelay    = 300 * time.Millisecond
	MCPClientName          = "mcp-agents-go"
	MCPClientVersion       = "1.0.0"
	URLSchemeHTTP          = "http://"
	URLSchemeHTTPS         = "https://"
)

type MCPServer struct {
	Name         string              `json:"name"`
	Type         model.ServerType    `json:"type"`
	Command      string              `json:"command,omitempty"`
	URL          string              `json:"url,omitempty"`
	Headers      []string            `json:"headers,omitempty"`
	Client       mcpclient.MCPClient `json:"-"`
	ServerDelay  string
	ProcessDelay string
}

func NewMCPServer(ctx context.Context, serverConfig model.Server) (*MCPServer, error) {
	logger.Logger.Info("Creating MCP server",
		"server_name", serverConfig.Name,
		"server_type", serverConfig.Type,
	)

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	s := &MCPServer{
		Name:         serverConfig.Name,
		Type:         serverConfig.Type,
		Command:      serverConfig.Command,
		URL:          serverConfig.URL,
		Headers:      serverConfig.Headers,
		ServerDelay:  serverConfig.ServerDelay,
		ProcessDelay: serverConfig.ProcessDelay,
	}

	// Validate configuration
	if err := s.validate(); err != nil {
		logger.Logger.Error("Server configuration validation failed",
			"server_name", serverConfig.Name,
			"error", err,
		)
		return nil, fmt.Errorf("invalid server configuration for %s: %w", serverConfig.Name, err)
	}
	logger.Logger.Debug("Server configuration validated", "server_name", serverConfig.Name)

	// Create MCP client
	cli, err := s.createMCPClient(ctx)
	if err != nil {
		logger.Logger.Error("Failed to create MCP client",
			"server_name", serverConfig.Name,
			"error", err,
		)
		return nil, fmt.Errorf("failed to create MCP client for server %s: %w", serverConfig.Name, err)
	}
	logger.Logger.Debug("MCP client created", "server_name", serverConfig.Name)

	s.Client = cli

	initDelay := DefaultServerInitDelay
	if serverConfig.ServerDelay != "" {
		initDelay, err = time.ParseDuration(serverConfig.ServerDelay)
		if err != nil {
			logger.Logger.Error("Failed to parse server delay")
		}
	}
	// Initialize client with timeout
	initCtx, cancel := context.WithTimeout(ctx, initDelay)
	defer cancel()

	logger.Logger.Info("Initializing MCP client",
		"server_name", serverConfig.Name,
		"timeout", initDelay,
	)

	if err := s.initializeClient(initCtx); err != nil {
		logger.Logger.Error("MCP client initialization failed",
			"server_name", serverConfig.Name,
			"error", err,
		)
		s.cleanup()
		return nil, fmt.Errorf("failed to initialize MCP client for server %s: %w", serverConfig.Name, err)
	}

	logger.Logger.Info("MCP server successfully initialized", "server_name", serverConfig.Name)
	return s, nil
}

func (s *MCPServer) validate() error {
	if s.Name == "" {
		return fmt.Errorf("server name cannot be empty")
	}

	logger.Logger.Debug("Validating server configuration",
		"server_name", s.Name,
		"transport_type", s.Type,
	)

	switch s.Type {
	case model.Stdio:
		if s.Command == "" {
			return fmt.Errorf("command is required and cannot be empty for stdio/local server type")
		}

		if strings.TrimSpace(s.Command) == "" {
			return fmt.Errorf("command cannot be only whitespace")
		}

		commandParts := strings.Fields(s.Command)
		if len(commandParts) == 0 {
			return fmt.Errorf("command must contain at least an executable name")
		}

		logger.Logger.Debug("Stdio server configuration",
			"server_name", s.Name,
			"command", commandParts[0],
			"args_count", len(commandParts)-1,
		)

	case model.SSE:
		if s.URL == "" {
			return fmt.Errorf("URL is required for sse server type")
		}

		trimmedURL := strings.TrimSpace(s.URL)
		if trimmedURL != s.URL {
			return fmt.Errorf("URL contains leading or trailing whitespace")
		}

		if !strings.HasPrefix(s.URL, URLSchemeHTTP) && !strings.HasPrefix(s.URL, URLSchemeHTTPS) {
			return fmt.Errorf("invalid URL format: must start with http:// or https://, got: %s", s.URL)
		}

		logger.Logger.Debug("SSE server configuration",
			"server_name", s.Name,
			"url", s.URL,
			"headers_count", len(s.Headers),
		)

		if len(s.Headers) > 0 {
			for i, header := range s.Headers {
				if !strings.Contains(header, ":") {
					return fmt.Errorf("invalid header format at index %d: must contain ':' separator", i)
				}
			}
		}

	default:
		return fmt.Errorf("unsupported server type: %s (expected: stdio, local, or sse)", s.Type)
	}

	return nil
}

func (s *MCPServer) initializeClient(ctx context.Context) error {
	if s.Client == nil {
		return fmt.Errorf("client is nil, cannot initialize")
	}

	logger.Logger.Debug("Building initialization request", "server_name", s.Name)

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    MCPClientName,
		Version: MCPClientVersion,
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	logger.Logger.Debug("Sending initialize request",
		"server_name", s.Name,
		"protocol_version", initRequest.Params.ProtocolVersion,
	)

	response, err := s.Client.Initialize(ctx, initRequest)
	if err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	if response == nil {
		return fmt.Errorf("initialize response is nil")
	}

	logger.Logger.Info("Server initialization successful",
		"server_name", s.Name,
		"server_info_name", response.ServerInfo.Name,
		"server_info_version", response.ServerInfo.Version,
		"protocol_version", response.ProtocolVersion,
	)

	// Log server capabilities
	capabilities := []string{}
	if response.Capabilities.Tools != nil {
		capabilities = append(capabilities, "tools")
	}
	if response.Capabilities.Resources != nil {
		capabilities = append(capabilities, "resources")
	}
	if response.Capabilities.Prompts != nil {
		capabilities = append(capabilities, "prompts")
	}

	if len(capabilities) > 0 {
		logger.Logger.Debug("Server capabilities",
			"server_name", s.Name,
			"capabilities", capabilities,
		)
	}

	return nil
}

func (s *MCPServer) createMCPClient(ctx context.Context) (mcpclient.MCPClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	logger.Logger.Debug("Creating MCP client",
		"server_name", s.Name,
		"transport_type", s.Type,
	)
	if s.Type == model.Stdio {
		return s.createStdioClient()
	} else if s.Type == model.SSE {
		return s.createSSEClient(ctx)
	} else if s.Type == model.Http {
		return s.createStreamableHttpClient()
	}
	return nil, fmt.Errorf("unsupported transport type '%s' for server %s", s.Type, s.Name)
}

func (s *MCPServer) createStdioClient() (mcpclient.MCPClient, error) {
	logger.Logger.Debug("Creating stdio client", "server_name", s.Name)

	commandParts := strings.Fields(s.Command)
	if len(commandParts) == 0 {
		return nil, fmt.Errorf("command is empty after parsing")
	}

	command := commandParts[0]
	var args []string
	if len(commandParts) > 1 {
		args = commandParts[1:]
	}

	logger.Logger.Debug("Stdio client configuration",
		"server_name", s.Name,
		"command", command,
		"args", args,
	)

	var env []string

	stdioClient, err := mcpclient.NewStdioMCPClient(command, env, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdio client: %w", err)
	}

	logger.Logger.Debug("Waiting for process startup", "server_name", s.Name)
	processDelay := ProcessStartupDelay
	if s.ProcessDelay != "" {
		processDelay, err = time.ParseDuration(s.ProcessDelay)
		if err != nil {
			return nil, fmt.Errorf("failed to parse process delay")
		}
	}
	time.Sleep(processDelay)
	logger.Logger.Debug("Stdio client ready", "server_name", s.Name)
	return stdioClient, nil
}

func (s *MCPServer) createSSEClient(ctx context.Context) (mcpclient.MCPClient, error) {
	logger.Logger.Debug("Creating SSE client",
		"server_name", s.Name,
		"url", s.URL,
	)

	var options []transport.ClientOption

	if len(s.Headers) > 0 {
		headers := make(map[string]string)
		logger.Logger.Debug("Processing headers",
			"server_name", s.Name,
			"headers_count", len(s.Headers),
		)

		for i, header := range s.Headers {
			parts := strings.SplitN(header, ":", 2)
			if len(parts) != 2 {
				logger.Logger.Warn("Invalid header format, skipping",
					"server_name", s.Name,
					"header_index", i,
					"header", header,
				)
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "" {
				logger.Logger.Warn("Header with empty key, skipping",
					"server_name", s.Name,
					"header_index", i,
				)
				continue
			}

			headers[key] = value
			logger.Logger.Debug("Added header",
				"server_name", s.Name,
				"header_key", key,
			)
		}

		if len(headers) > 0 {
			options = append(options, transport.WithHeaders(headers))
			logger.Logger.Debug("Valid headers configured",
				"server_name", s.Name,
				"valid_headers_count", len(headers),
			)
		} else {
			logger.Logger.Warn("No valid headers after parsing", "server_name", s.Name)
		}
	}

	sseClient, err := client.NewSSEMCPClient(s.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE client: %w", err)
	}

	logger.Logger.Debug("Starting SSE client", "server_name", s.Name)

	if err := sseClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start SSE client: %w", err)
	}

	logger.Logger.Info("SSE client started successfully", "server_name", s.Name)
	return sseClient, nil
}

func (s *MCPServer) createStreamableHttpClient() (mcpclient.MCPClient, error) {
	logger.Logger.Debug("Creating Streamable HTTP client",
		"server_name", s.Name,
		"url", s.URL,
	)
	httpClient, err := mcpclient.NewStreamableHttpClient(s.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdio client: %w", err)
	}
	logger.Logger.Debug("Waiting for process startup", "server_name", s.Name)
	processDelay := ProcessStartupDelay
	if s.ProcessDelay != "" {
		processDelay, err = time.ParseDuration(s.ProcessDelay)
		if err != nil {
			return nil, fmt.Errorf("failed to parse process delay")
		}
	}
	time.Sleep(processDelay)
	logger.Logger.Debug("Stdio client ready", "server_name", s.Name)
	return httpClient, nil
}

func (s *MCPServer) cleanup() {
	if s.Client == nil {
		return
	}

	logger.Logger.Debug("Cleaning up server resources", "server_name", s.Name)

	if closer, ok := s.Client.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			logger.Logger.Warn("Error closing client",
				"server_name", s.Name,
				"error", err,
			)
		} else {
			logger.Logger.Debug("Client closed successfully", "server_name", s.Name)
		}
	} else {
		logger.Logger.Debug("Client does not implement Close()", "server_name", s.Name)
	}
}

func (s *MCPServer) Close() error {
	if s.Client == nil {
		return fmt.Errorf("client is nil, already closed or never initialized")
	}

	logger.Logger.Info("Closing MCP server", "server_name", s.Name)

	if closer, ok := s.Client.(interface{ Close() error }); ok {
		err := closer.Close()
		if err != nil {
			logger.Logger.Error("Failed to close server",
				"server_name", s.Name,
				"error", err,
			)
			return fmt.Errorf("failed to close server %s: %w", s.Name, err)
		}
		logger.Logger.Info("Server closed successfully", "server_name", s.Name)
		s.Client = nil
		return nil
	}

	logger.Logger.Warn("Client does not support closing", "server_name", s.Name)
	return fmt.Errorf("client does not implement Close() interface")
}

func (s *MCPServer) IsHealthy(ctx context.Context) bool {
	if s.Client == nil {
		logger.Logger.Debug("Health check failed: client is nil", "server_name", s.Name)
		return false
	}

	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.Client.ListTools(healthCtx, mcp.ListToolsRequest{})
	if err != nil {
		logger.Logger.Warn("Health check failed",
			"server_name", s.Name,
			"error", err,
		)
		return false
	}

	logger.Logger.Debug("Health check passed", "server_name", s.Name)
	return true
}

func (s *MCPServer) GetInfo() map[string]interface{} {
	info := map[string]interface{}{
		"name":      s.Name,
		"type":      s.Type,
		"transport": s.Type,
	}

	switch s.Type {
	case model.Stdio:
		info["command"] = s.Command
	case model.SSE:
		info["url"] = s.URL
		info["headers_count"] = len(s.Headers)
	}

	return info
}
