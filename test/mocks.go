package tests

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/stretchr/testify/mock"
	"github.com/tmc/langchaingo/llms"
)

// dummy writer for logger
type DummyWriter struct{}

// NewDummyWriter creates a new DummyWriter instance
func NewDummyWriter() *DummyWriter {
	return &DummyWriter{}
}

// Write implements io.Writer interface and discards all data
func (d *DummyWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// Close implements io.Closer interface (if needed)
func (d *DummyWriter) Close() error {
	return nil
}

// MockServerFactory for testing
type MockServerFactory struct {
	CreateFunc func(ctx context.Context, config model.Server) (*server.MCPServer, error)
	CallCount  int
	LastConfig model.Server
}

func (m *MockServerFactory) NewMCPServer(ctx context.Context, config model.Server) (*server.MCPServer, error) {
	m.CallCount++
	m.LastConfig = config
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, config)
	}
	// Return a mock server by default
	return &server.MCPServer{}, nil
}

// MockMCPClient mocks the MCP client
type MockMCPClient struct {
	mock.Mock
}

func (m *MockMCPClient) Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) Ping(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListResourcesByPage(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListResourceTemplatesByPage(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListResourceTemplates(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) Subscribe(ctx context.Context, request mcp.SubscribeRequest) error {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) Unsubscribe(ctx context.Context, request mcp.UnsubscribeRequest) error {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListPromptsByPage(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListToolsByPage(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) SetLevel(ctx context.Context, request mcp.SetLevelRequest) error {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) Complete(ctx context.Context, request mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) Close() error {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	//TODO implement me
	panic("implement me")
}

func (m *MockMCPClient) ListTools(ctx context.Context, req mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mcp.ListToolsResult), args.Error(1)
}

func (m *MockMCPClient) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mcp.CallToolResult), args.Error(1)
}

// Test helper functions
func createMockServer(name string, tools []mcp.Tool) *server.MCPServer {
	return &server.MCPServer{
		Name:   name,
		Client: nil, // Will be set in tests
	}
}

func createTestTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "test_tool_1",
			Description: "A test tool",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"param1": map[string]interface{}{
						"type":        "string",
						"description": "First parameter",
					},
				},
				Required: []string{"param1"},
			},
		},
		{
			Name:        "test_tool_2",
			Description: "Another test tool",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]interface{}{},
			},
		},
	}
}

// MockLLMModel mocks the llms.Model interface
type MockLLMModel struct {
	mock.Mock
}

func (m *MockLLMModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	args := m.Called(ctx, messages, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llms.ContentResponse), args.Error(1)
}

func (m *MockLLMModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	args := m.Called(ctx, prompt, options)
	return args.String(0), args.Error(1)
}
