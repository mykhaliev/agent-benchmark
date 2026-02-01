package tests

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CLI Server Tests
// ============================================================================

func TestNewCLIServer(t *testing.T) {
	t.Run("Valid CLI server configuration", func(t *testing.T) {
		ctx := context.Background()
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		assert.NotNil(t, srv)
		assert.Equal(t, "test-cli", srv.Name)
		assert.Equal(t, "echo", srv.Command)
		defer srv.Close()
	})

	t.Run("CLI server with custom shell", func(t *testing.T) {
		ctx := context.Background()
		
		shell := "bash"
		if runtime.GOOS == "windows" {
			shell = "powershell"
		}
		
		config := model.Server{
			Name:    "test-cli-shell",
			Type:    model.CLI,
			Command: "echo",
			Shell:   shell,
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		assert.Equal(t, shell, srv.Shell)
		defer srv.Close()
	})

	t.Run("CLI server with nil context", func(t *testing.T) {
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
		}

		_, err := server.NewCLIServer(nil, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context cannot be nil")
	})

	t.Run("CLI server without command", func(t *testing.T) {
		ctx := context.Background()
		config := model.Server{
			Name: "test-cli",
			Type: model.CLI,
		}

		_, err := server.NewCLIServer(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command is required")
	})

	t.Run("CLI server with invalid shell", func(t *testing.T) {
		ctx := context.Background()
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
			Shell:   "invalid-shell",
		}

		_, err := server.NewCLIServer(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported shell")
	})

	t.Run("CLI server with non-existent working directory", func(t *testing.T) {
		ctx := context.Background()
		config := model.Server{
			Name:       "test-cli",
			Type:       model.CLI,
			Command:    "echo",
			WorkingDir: "/non/existent/path/12345",
		}

		_, err := server.NewCLIServer(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "working directory does not exist")
	})
}

func TestCLIServerExecute(t *testing.T) {
	ctx := context.Background()

	t.Run("Execute simple echo command", func(t *testing.T) {
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		defer srv.Close()

		exec, err := srv.Execute(ctx, "hello world")
		require.NoError(t, err)
		assert.Equal(t, 0, exec.ExitCode)
		assert.Contains(t, exec.Stdout, "hello")
	})

	t.Run("Execute command with non-zero exit code", func(t *testing.T) {
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "exit",
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		defer srv.Close()

		exec, err := srv.Execute(ctx, "1")
		require.NoError(t, err)
		assert.NotEqual(t, 0, exec.ExitCode)
	})

	t.Run("Track multiple executions", func(t *testing.T) {
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		defer srv.Close()

		_, _ = srv.Execute(ctx, "first")
		_, _ = srv.Execute(ctx, "second")

		executions := srv.GetExecutions()
		assert.Len(t, executions, 2)
		assert.Contains(t, executions[0].FullCmd, "first")
		assert.Contains(t, executions[1].FullCmd, "second")
	})

	t.Run("Clear executions", func(t *testing.T) {
		config := model.Server{
			Name:    "test-cli",
			Type:    model.CLI,
			Command: "echo",
		}

		srv, err := server.NewCLIServer(ctx, config)
		require.NoError(t, err)
		defer srv.Close()

		_, _ = srv.Execute(ctx, "test")
		assert.Len(t, srv.GetExecutions(), 1)

		srv.ClearExecutions()
		assert.Len(t, srv.GetExecutions(), 0)
	})
}

func TestCLIClientInterface(t *testing.T) {
	ctx := context.Background()
	config := model.Server{
		Name:       "test-cli",
		Type:       model.CLI,
		Command:    "echo",
		ToolPrefix: "test",
	}

	srv, err := server.NewCLIServer(ctx, config)
	require.NoError(t, err)
	defer srv.Close()

	client := srv.GetClient()

	t.Run("Initialize returns valid result", func(t *testing.T) {
		result, err := client.Initialize(ctx, mcp.InitializeRequest{})
		require.NoError(t, err)
		assert.Equal(t, "cli-server", result.ServerInfo.Name)
	})

	t.Run("ListTools returns CLI execute tool", func(t *testing.T) {
		result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		assert.Len(t, result.Tools, 1)
		assert.Equal(t, "test_execute", result.Tools[0].Name)
	})

	t.Run("CallTool executes command", func(t *testing.T) {
		request := mcp.CallToolRequest{}
		request.Params.Name = "test_execute"
		request.Params.Arguments = map[string]interface{}{
			"args": "hello from tool",
		}

		result, err := client.CallTool(ctx, request)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Len(t, result.Content, 1)
	})

	t.Run("Ping succeeds", func(t *testing.T) {
		err := client.Ping(ctx)
		assert.NoError(t, err)
	})

	t.Run("ListResources returns empty", func(t *testing.T) {
		result, err := client.ListResources(ctx, mcp.ListResourcesRequest{})
		require.NoError(t, err)
		assert.Empty(t, result.Resources)
	})

	t.Run("ListPrompts returns empty", func(t *testing.T) {
		result, err := client.ListPrompts(ctx, mcp.ListPromptsRequest{})
		require.NoError(t, err)
		assert.Empty(t, result.Prompts)
	})
}

func TestCLIServerViaMCPServer(t *testing.T) {
	// Skip this test when logger is not initialized
	// This test requires full integration with NewMCPServer which has logger calls
	t.Skip("Skipping integration test that requires logger initialization")
	
	ctx := context.Background()
	config := model.Server{
		Name:    "test-cli",
		Type:    model.CLI,
		Command: "echo",
	}

	// Test that CLI server can be created via NewMCPServer
	mcpServer, err := server.NewMCPServer(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, mcpServer)
	assert.Equal(t, model.CLI, mcpServer.Type)
	defer mcpServer.Close()

	// Test that tools are accessible via the MCP client interface
	tools, err := mcpServer.Client.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	assert.Len(t, tools.Tools, 1)
}

func TestCLIServerWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := model.Server{
		Name:    "test-cli",
		Type:    model.CLI,
		Command: "echo",
	}

	srv, err := server.NewCLIServer(ctx, config)
	require.NoError(t, err)
	defer srv.Close()

	// Quick command should succeed
	exec, err := srv.Execute(ctx, "fast")
	require.NoError(t, err)
	// Just verify we got a result - exit code may vary by platform
	assert.NotNil(t, exec)
	assert.Contains(t, exec.FullCmd, "fast")
}

func TestCLIServerWorkingDirectory(t *testing.T) {
	ctx := context.Background()
	
	// Use system temp directory as working directory
	tempDir := os.TempDir()
	
	config := model.Server{
		Name:       "test-cli",
		Type:       model.CLI,
		Command:    "echo",
		WorkingDir: tempDir,
	}

	srv, err := server.NewCLIServer(ctx, config)
	require.NoError(t, err)
	assert.Equal(t, tempDir, srv.WorkingDir)
	defer srv.Close()
}

// TestCLIServerHelpAutoDiscovery tests the automatic help discovery feature
func TestCLIServerHelpAutoDiscovery(t *testing.T) {
	t.Run("Auto-discovers help when no help_command configured", func(t *testing.T) {
		ctx := context.Background()
		
		// Use a command that supports --help (git is commonly available)
		// Skip if git is not available
		config := model.Server{
			Name:    "test-cli-autodiscover",
			Type:    model.CLI,
			Command: "git",
			// No HelpCommand or HelpCommands - should auto-discover
		}

		srv, err := server.NewCLIServer(ctx, config)
		if err != nil {
			t.Skipf("git not available: %v", err)
		}
		defer srv.Close()

		// Get tools from the server
		client := srv.GetClient()
		tools, err := client.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.Len(t, tools.Tools, 1)

		// Tool description should contain auto-discovered help content
		// (git --help should return usage information)
		description := tools.Tools[0].Description
		assert.Contains(t, description, "command", "Auto-discovered help should contain CLI usage info")
	})

	t.Run("Respects disable_help_auto_discovery flag", func(t *testing.T) {
		ctx := context.Background()
		
		config := model.Server{
			Name:                     "test-cli-no-autodiscover",
			Type:                     model.CLI,
			Command:                  "git",
			DisableHelpAutoDiscovery: true, // Explicitly disable
			// No HelpCommand or HelpCommands - but auto-discovery is disabled
		}

		srv, err := server.NewCLIServer(ctx, config)
		if err != nil {
			t.Skipf("git not available: %v", err)
		}
		defer srv.Close()

		// Get tools from the server
		client := srv.GetClient()
		tools, err := client.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.Len(t, tools.Tools, 1)

		// Tool description should NOT contain help content (just the basic description)
		description := tools.Tools[0].Description
		// Should be simple: "Execute git CLI command with arguments."
		// Not contain help content like "usage:" or "commands:"
		assert.Equal(t, "Execute git CLI command with arguments.", description)
	})
}
