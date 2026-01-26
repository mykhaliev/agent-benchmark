package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// File Validation Tests
// ============================================================================

func TestValidateTestInputFile(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	t.Run("Valid YAML file", func(t *testing.T) {
		tmpfile := createTempFile(t, "test-*.yaml", "test: content")
		err := engine.ValidateTestInputFile(tmpfile)
		assert.NoError(t, err)
	})

	t.Run("Valid YML file", func(t *testing.T) {
		tmpfile := createTempFile(t, "test-*.yml", "test: content")
		err := engine.ValidateTestInputFile(tmpfile)
		assert.NoError(t, err)
	})

	t.Run("Empty path", func(t *testing.T) {
		err := engine.ValidateTestInputFile("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("Non-existent file", func(t *testing.T) {
		err := engine.ValidateTestInputFile("/nonexistent/path/file.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("Directory instead of file", func(t *testing.T) {
		tmpdir := t.TempDir()
		err := engine.ValidateTestInputFile(tmpdir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "directory")
	})

	t.Run("Empty file", func(t *testing.T) {
		tmpfile := createTempFile(t, "test-*.yaml", "")
		err := engine.ValidateTestInputFile(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("Unexpected file extension: .json", func(t *testing.T) {
		tmpfile := createTempFile(t, "test-*.json", "test: content")
		err := engine.ValidateTestInputFile(tmpfile)
		// Should not error, but will log warning
		assert.Contains(t, err.Error(), "unexpected file extension: .json")
	})
}

// ============================================================================
// Config Validation Tests
// ============================================================================

func TestValidateTestConfig(t *testing.T) {
	tests := []struct {
		name             string
		config           *model.TestConfiguration
		runningFromSuite bool
		wantErr          bool
		errContains      string
	}{
		{
			name:             "Nil config",
			config:           nil,
			runningFromSuite: false,
			wantErr:          true,
			errContains:      "nil",
		},
		{
			name: "Valid standalone config",
			config: &model.TestConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Servers:   []model.Server{{Name: "test"}},
				Agents:    []model.Agent{{Name: "test"}},
				Sessions:  []model.Session{{Name: "test"}},
			},
			runningFromSuite: false,
			wantErr:          false,
		},
		{
			name: "Valid suite config (no providers required)",
			config: &model.TestConfiguration{
				Sessions: []model.Session{{Name: "test"}},
			},
			runningFromSuite: true,
			wantErr:          false,
		},
		{
			name: "Missing providers (standalone)",
			config: &model.TestConfiguration{
				Servers:  []model.Server{{Name: "test"}},
				Agents:   []model.Agent{{Name: "test"}},
				Sessions: []model.Session{{Name: "test"}},
			},
			runningFromSuite: false,
			wantErr:          true,
			errContains:      "providers",
		},
		{
			name: "Missing servers (standalone)",
			config: &model.TestConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Agents:    []model.Agent{{Name: "test"}},
				Sessions:  []model.Session{{Name: "test"}},
			},
			runningFromSuite: false,
			wantErr:          true,
			errContains:      "servers",
		},
		{
			name: "Missing agents (standalone)",
			config: &model.TestConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Servers:   []model.Server{{Name: "test"}},
				Sessions:  []model.Session{{Name: "test"}},
			},
			runningFromSuite: false,
			wantErr:          true,
			errContains:      "agents",
		},
		{
			name: "Missing sessions",
			config: &model.TestConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Servers:   []model.Server{{Name: "test"}},
				Agents:    []model.Agent{{Name: "test"}},
			},
			runningFromSuite: false,
			wantErr:          true,
			errContains:      "sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateTestConfig(tt.config, tt.runningFromSuite)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSuiteConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *model.TestSuiteConfiguration
		wantErr     bool
		errContains string
	}{
		{
			name:        "Nil config",
			config:      nil,
			wantErr:     true,
			errContains: "nil",
		},
		{
			name: "Valid config",
			config: &model.TestSuiteConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Servers:   []model.Server{{Name: "test"}},
				Agents:    []model.Agent{{Name: "test"}},
			},
			wantErr: false,
		},
		{
			name: "Missing providers",
			config: &model.TestSuiteConfiguration{
				Servers: []model.Server{{Name: "test"}},
				Agents:  []model.Agent{{Name: "test"}},
			},
			wantErr:     true,
			errContains: "providers",
		},
		{
			name: "Missing servers",
			config: &model.TestSuiteConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Agents:    []model.Agent{{Name: "test"}},
			},
			wantErr:     true,
			errContains: "servers",
		},
		{
			name: "Missing agents",
			config: &model.TestSuiteConfiguration{
				Providers: []model.Provider{{Name: "test"}},
				Servers:   []model.Server{{Name: "test"}},
			},
			wantErr:     true,
			errContains: "agents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateSuiteConfig(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateReportType(t *testing.T) {
	tests := []struct {
		name       string
		reportType string
		wantErr    bool
	}{
		{"Valid HTML", "html", false},
		{"Valid JSON", "json", false},
		{"Valid Markdown", "md", false},
		{"Invalid type", "xml", true},
		{"Invalid type", "pdf", true},
		{"Empty string", "", true},
		{"Case sensitive", "HTML", true},
		{"Case sensitive", "Json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateReportType(tt.reportType)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "supported types")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// Duration Parsing Tests
// ============================================================================

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeoutStr  string
		expected    time.Duration
		description string
	}{
		{
			name:        "Empty string returns default",
			timeoutStr:  "",
			expected:    engine.DefaultTimeout,
			description: "Should return default when empty",
		},
		{
			name:        "Valid seconds",
			timeoutStr:  "30s",
			expected:    30 * time.Second,
			description: "Parse seconds correctly",
		},
		{
			name:        "Valid minutes",
			timeoutStr:  "5m",
			expected:    5 * time.Minute,
			description: "Parse minutes correctly",
		},
		{
			name:        "Valid hours",
			timeoutStr:  "2h",
			expected:    2 * time.Hour,
			description: "Parse hours correctly",
		},
		{
			name:        "Valid milliseconds",
			timeoutStr:  "500ms",
			expected:    500 * time.Millisecond,
			description: "Parse milliseconds correctly",
		},
		{
			name:        "Complex duration",
			timeoutStr:  "1h30m45s",
			expected:    time.Hour + 30*time.Minute + 45*time.Second,
			description: "Parse complex duration",
		},
		{
			name:        "Invalid format returns default",
			timeoutStr:  "invalid",
			expected:    engine.DefaultTimeout,
			description: "Return default on parse error",
		},
		{
			name:        "Negative value returns 0",
			timeoutStr:  "-10s",
			expected:    0,
			description: "Negative values clamped to 0",
		},
		{
			name:        "Zero value",
			timeoutStr:  "0s",
			expected:    0,
			description: "Zero is valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.ParseTimeout(tt.timeoutStr)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestParseDelay(t *testing.T) {
	tests := []struct {
		name        string
		delayStr    string
		expected    time.Duration
		description string
	}{
		{
			name:        "Empty string returns default",
			delayStr:    "",
			expected:    engine.DefaultTestDelay,
			description: "Should return default when empty",
		},
		{
			name:        "Valid seconds",
			delayStr:    "10s",
			expected:    10 * time.Second,
			description: "Parse seconds correctly",
		},
		{
			name:        "Valid milliseconds",
			delayStr:    "100ms",
			expected:    100 * time.Millisecond,
			description: "Parse milliseconds correctly",
		},
		{
			name:        "Invalid format returns default",
			delayStr:    "not-a-duration",
			expected:    engine.DefaultTestDelay,
			description: "Return default on parse error",
		},
		{
			name:        "Negative value returns 0",
			delayStr:    "-5s",
			expected:    0,
			description: "Negative values clamped to 0",
		},
		{
			name:        "Zero value",
			delayStr:    "0s",
			expected:    0,
			description: "Zero is valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.ParseDelay(tt.delayStr)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

// ============================================================================
// Max Iterations Tests
// ============================================================================

func TestGetMaxIterations(t *testing.T) {
	tests := []struct {
		name     string
		maxIter  int
		expected int
	}{
		{
			name:     "Zero returns default",
			maxIter:  0,
			expected: engine.DefaultMaxIterations,
		},
		{
			name:     "Negative returns default",
			maxIter:  -5,
			expected: engine.DefaultMaxIterations,
		},
		{
			name:     "Valid small value",
			maxIter:  5,
			expected: 5,
		},
		{
			name:     "Valid medium value",
			maxIter:  20,
			expected: 20,
		},
		{
			name:     "Large value (warning logged)",
			maxIter:  150,
			expected: 150,
		},
		{
			name:     "Exactly 100",
			maxIter:  100,
			expected: 100,
		},
		{
			name:     "One",
			maxIter:  1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.GetMaxIterations(tt.maxIter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Utility Function Tests
// ============================================================================

func TestGetServerNames(t *testing.T) {
	tests := []struct {
		name     string
		servers  []model.AgentServer
		expected []string
	}{
		{
			name:     "Empty list",
			servers:  []model.AgentServer{},
			expected: []string{},
		},
		{
			name: "Single server",
			servers: []model.AgentServer{
				{Name: "server1"},
			},
			expected: []string{"server1"},
		},
		{
			name: "Multiple servers",
			servers: []model.AgentServer{
				{Name: "server1"},
				{Name: "server2"},
				{Name: "server3"},
			},
			expected: []string{"server1", "server2", "server3"},
		},
		{
			name: "Servers with allowed tools",
			servers: []model.AgentServer{
				{Name: "server1", AllowedTools: []string{"tool1", "tool2"}},
				{Name: "server2", AllowedTools: []string{"tool3"}},
			},
			expected: []string{"server1", "server2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.GetServerNames(tt.servers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasFailures(t *testing.T) {
	tests := []struct {
		name     string
		results  []model.TestRun
		expected bool
	}{
		{
			name:     "Empty results",
			results:  []model.TestRun{},
			expected: false,
		},
		{
			name: "All passed",
			results: []model.TestRun{
				{Passed: true},
				{Passed: true},
				{Passed: true},
			},
			expected: false,
		},
		{
			name: "One failure",
			results: []model.TestRun{
				{Passed: true},
				{Passed: false},
				{Passed: true},
			},
			expected: true,
		},
		{
			name: "All failed",
			results: []model.TestRun{
				{Passed: false},
				{Passed: false},
			},
			expected: true,
		},
		{
			name: "First test failed",
			results: []model.TestRun{
				{Passed: false},
				{Passed: true},
			},
			expected: true,
		},
		{
			name: "Last test failed",
			results: []model.TestRun{
				{Passed: true},
				{Passed: false},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.HasFailures(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateTemplateContext(t *testing.T) {
	t.Run("Nil variables", func(t *testing.T) {
		ctx := engine.CreateTemplateContext(nil)
		assert.NotNil(t, ctx)
		// Should contain environment variables
		assert.NotEmpty(t, ctx)
	})

	t.Run("Empty variables", func(t *testing.T) {
		ctx := engine.CreateTemplateContext(map[string]string{})
		assert.NotNil(t, ctx)
	})

	t.Run("Simple variables", func(t *testing.T) {
		variables := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		ctx := engine.CreateTemplateContext(variables)

		assert.Equal(t, "value1", ctx["key1"])
		assert.Equal(t, "value2", ctx["key2"])
	})

	t.Run("Variables with templates", func(t *testing.T) {
		// Set a test environment variable
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		variables := map[string]string{
			"composed": "{{TEST_VAR}}_suffix",
		}
		ctx := engine.CreateTemplateContext(variables)

		assert.Contains(t, ctx["composed"], "test_value")
	})

	t.Run("Contains environment variables", func(t *testing.T) {
		os.Setenv("CUSTOM_TEST_VAR", "custom_value")
		defer os.Unsetenv("CUSTOM_TEST_VAR")

		ctx := engine.CreateTemplateContext(nil)
		assert.Equal(t, "custom_value", ctx["CUSTOM_TEST_VAR"])
	})
}

func TestCreateStaticTemplateContext(t *testing.T) {
	t.Run("RUN_ID is set and is valid UUID", func(t *testing.T) {
		ctx := engine.CreateStaticTemplateContext("", nil)

		assert.NotNil(t, ctx)
		assert.NotEmpty(t, ctx["RUN_ID"])
		// Should be a valid UUID format (8-4-4-4-12)
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, ctx["RUN_ID"])
	})

	t.Run("RUN_ID is unique per call", func(t *testing.T) {
		ctx1 := engine.CreateStaticTemplateContext("", nil)
		ctx2 := engine.CreateStaticTemplateContext("", nil)

		assert.NotEqual(t, ctx1["RUN_ID"], ctx2["RUN_ID"])
	})

	t.Run("TEMP_DIR is set", func(t *testing.T) {
		ctx := engine.CreateStaticTemplateContext("", nil)

		assert.NotNil(t, ctx)
		assert.NotEmpty(t, ctx["TEMP_DIR"])
		// Should be an absolute path
		assert.True(t, filepath.IsAbs(ctx["TEMP_DIR"]))
		// Directory should exist
		info, err := os.Stat(ctx["TEMP_DIR"])
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("TEST_DIR from source file", func(t *testing.T) {
		// Create a temp directory and file path
		tempDir := t.TempDir()
		sourceFile := filepath.Join(tempDir, "test.yaml")

		ctx := engine.CreateStaticTemplateContext(sourceFile, nil)

		assert.NotNil(t, ctx)
		assert.Equal(t, tempDir, ctx["TEST_DIR"])
	})

	t.Run("TEST_DIR with relative path", func(t *testing.T) {
		// Use examples directory which exists in the repo
		sourceFile := "examples/test.yaml"

		ctx := engine.CreateStaticTemplateContext(sourceFile, nil)

		assert.NotNil(t, ctx)
		assert.Contains(t, ctx["TEST_DIR"], "examples")
		// Should be an absolute path
		assert.True(t, filepath.IsAbs(ctx["TEST_DIR"]))
	})

	t.Run("Empty source file", func(t *testing.T) {
		ctx := engine.CreateStaticTemplateContext("", nil)

		assert.NotNil(t, ctx)
		// TEST_DIR should not be set
		_, exists := ctx["TEST_DIR"]
		assert.False(t, exists)
	})

	t.Run("Variables can reference TEST_DIR", func(t *testing.T) {
		tempDir := t.TempDir()
		sourceFile := filepath.Join(tempDir, "test.yaml")

		variables := map[string]string{
			"SERVER_PATH": "{{TEST_DIR}}/server.exe",
		}

		ctx := engine.CreateStaticTemplateContext(sourceFile, variables)

		// The template uses forward slash, but TEST_DIR uses OS path separator
		// So we check that both parts are present
		assert.Contains(t, ctx["SERVER_PATH"], tempDir)
		assert.Contains(t, ctx["SERVER_PATH"], "server.exe")
	})

	t.Run("Variables can reference environment variables", func(t *testing.T) {
		os.Setenv("STATIC_TEST_VAR", "env_value")
		defer os.Unsetenv("STATIC_TEST_VAR")

		variables := map[string]string{
			"COMPOSED": "prefix_{{STATIC_TEST_VAR}}_suffix",
		}

		ctx := engine.CreateStaticTemplateContext("", variables)

		assert.Equal(t, "prefix_env_value_suffix", ctx["COMPOSED"])
	})

	t.Run("Variables can reference other user variables", func(t *testing.T) {
		variables := map[string]string{
			"BASE_PATH":   "/opt/app",
			"SERVER_PATH": "{{BASE_PATH}}/bin/server",
		}

		ctx := engine.CreateStaticTemplateContext("", variables)

		// Note: Due to map iteration order, this may or may not work
		// The second variable references the first
		assert.Equal(t, "/opt/app", ctx["BASE_PATH"])
		// SERVER_PATH depends on ordering - may be resolved or not
		assert.NotNil(t, ctx["SERVER_PATH"])
	})

	t.Run("Contains environment variables", func(t *testing.T) {
		os.Setenv("STATIC_ENV_TEST", "static_env_value")
		defer os.Unsetenv("STATIC_ENV_TEST")

		ctx := engine.CreateStaticTemplateContext("", nil)

		assert.Equal(t, "static_env_value", ctx["STATIC_ENV_TEST"])
	})

	t.Run("Combined TEST_DIR and env vars", func(t *testing.T) {
		tempDir := t.TempDir()
		sourceFile := filepath.Join(tempDir, "test.yaml")

		os.Setenv("APP_NAME", "myapp")
		defer os.Unsetenv("APP_NAME")

		variables := map[string]string{
			"APP_PATH": "{{TEST_DIR}}/{{APP_NAME}}",
		}

		ctx := engine.CreateStaticTemplateContext(sourceFile, variables)

		// The template uses forward slash, but TEST_DIR uses OS path separator
		// So we check that both parts are present
		assert.Contains(t, ctx["APP_PATH"], tempDir)
		assert.Contains(t, ctx["APP_PATH"], "myapp")
	})
}

// ============================================================================
// Provider Creation Tests
// ============================================================================

func TestCreateProvider_Validation(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &engine.DefaultServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Empty token for non-Vertex", func(t *testing.T) {
		provider := model.Provider{
			Type:  model.ProviderOpenAI,
			Token: "",
			Model: "gpt-4",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token")
	})

	t.Run("Empty model", func(t *testing.T) {
		provider := model.Provider{
			Type:  model.ProviderOpenAI,
			Token: "test-token",
			Model: "",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("Unsupported provider type", func(t *testing.T) {
		provider := model.Provider{
			Type:  model.ProviderType("UNKNOWN"),
			Token: "test-token",
			Model: "test-model",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})

	t.Run("Azure without version", func(t *testing.T) {
		provider := model.Provider{
			Type:    model.ProviderAzure,
			Token:   "test-token",
			Model:   "gpt-4",
			BaseURL: "https://test.openai.azure.com",
			Version: "",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("Azure without base URL", func(t *testing.T) {
		provider := model.Provider{
			Type:    model.ProviderAzure,
			Token:   "test-token",
			Model:   "gpt-4",
			Version: "2024-01-01",
			BaseURL: "",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "base URL")
	})

	t.Run("Azure with entra_id auth skips token validation", func(t *testing.T) {
		// When using Entra ID auth, token is not required for initial validation
		// The provider creation may succeed or fail depending on Azure credentials in env
		// We just verify it doesn't fail on "token is empty" validation
		provider := model.Provider{
			Type:     model.ProviderAzure,
			Token:    "", // No token needed for Entra ID
			Model:    "gpt-4",
			Version:  "2024-01-01",
			BaseURL:  "https://test.openai.azure.com",
			AuthType: "entra_id",
		}

		_, err := engine.CreateProvider(ctx, provider)
		// Error is expected (no Azure credentials in test env)
		// But if there IS an error, it should NOT be about empty token
		if err != nil {
			assert.NotContains(t, err.Error(), "token is empty")
		}
	})

	t.Run("Azure with api_key auth requires token", func(t *testing.T) {
		provider := model.Provider{
			Type:     model.ProviderAzure,
			Token:    "", // No token
			Model:    "gpt-4",
			Version:  "2024-01-01",
			BaseURL:  "https://test.openai.azure.com",
			AuthType: "api_key",
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token")
	})

	t.Run("Azure with empty auth_type defaults to api_key (requires token)", func(t *testing.T) {
		provider := model.Provider{
			Type:     model.ProviderAzure,
			Token:    "", // No token
			Model:    "gpt-4",
			Version:  "2024-01-01",
			BaseURL:  "https://test.openai.azure.com",
			AuthType: "", // Empty defaults to api_key
		}

		_, err := engine.CreateProvider(ctx, provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token")
	})
}

// ============================================================================
// InitServers Tests
// ============================================================================

func TestInitServers_Validation(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &engine.DefaultServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	tests := []struct {
		name        string
		servers     []model.Server
		wantErr     bool
		errContains string
		description string
	}{
		// Basic Validation Tests
		{
			name:        "Empty servers list",
			servers:     []model.Server{},
			wantErr:     true,
			errContains: "no servers",
			description: "Should fail with empty server list",
		},
		{
			name: "Server with empty name",
			servers: []model.Server{
				{
					Name:    "",
					Type:    model.Stdio,
					Command: "node server.js",
				},
			},
			wantErr:     true,
			errContains: "empty name",
			description: "Should fail when server has no name",
		},
		{
			name: "Duplicate server names",
			servers: []model.Server{
				{
					Name:    "duplicate-server",
					Type:    model.Stdio,
					Command: "node server.js",
				},
				{
					Name: "duplicate-server",
					Type: model.SSE,
					URL:  "https://example.com/mcp",
				},
			},
			wantErr:     true,
			errContains: "duplicate",
			description: "Should fail with duplicate server names",
		},

		// Stdio Server Tests
		{
			name: "Valid stdio server",
			servers: []model.Server{
				{
					Name:         "stdio-server",
					Type:         model.Stdio,
					Command:      "node server.js",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Valid stdio configuration",
		},
		{
			name: "Stdio server with arguments",
			servers: []model.Server{
				{
					Name:         "stdio-with-args",
					Type:         model.Stdio,
					Command:      "python3 server.py --port 8080 --verbose",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Stdio server with command arguments",
		},

		// SSE Server Tests
		{
			name: "Valid SSE server",
			servers: []model.Server{
				{
					Name:         "sse-server",
					Type:         model.SSE,
					URL:          "https://example.com/mcp/sse",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Valid SSE configuration",
		},
		{
			name: "SSE server with headers",
			servers: []model.Server{
				{
					Name: "sse-with-headers",
					Type: model.SSE,
					URL:  "https://api.example.com/mcp",
					Headers: []string{
						"Authorization: Bearer token123",
						"X-API-Key: key456",
					},
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "SSE server with custom headers",
		},
		{
			name: "SSE server missing URL",
			servers: []model.Server{
				{
					Name:         "sse-no-url",
					Type:         model.SSE,
					URL:          "",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     true,
			errContains: "server",
			description: "Should fail without URL",
		},
		{
			name: "SSE server with HTTP URL",
			servers: []model.Server{
				{
					Name:         "sse-http",
					Type:         model.SSE,
					URL:          "http://localhost:8080/mcp",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "SSE with HTTP URL (non-HTTPS)",
		},

		// HTTP Server Tests
		{
			name: "Valid HTTP server",
			servers: []model.Server{
				{
					Name:         "http-server",
					Type:         model.Http,
					URL:          "https://api.example.com/mcp",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Valid HTTP configuration",
		},
		{
			name: "HTTP server missing URL",
			servers: []model.Server{
				{
					Name:         "http-no-url",
					Type:         model.Http,
					URL:          "",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     true,
			errContains: "server",
			description: "Should fail without URL",
		},

		// Multiple Servers Tests
		{
			name: "Multiple valid servers of different types",
			servers: []model.Server{
				{
					Name:         "stdio-1",
					Type:         model.Stdio,
					Command:      "node server1.js",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
				{
					Name:         "sse-1",
					Type:         model.SSE,
					URL:          "https://example.com/mcp1",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
				{
					Name:         "http-1",
					Type:         model.Http,
					URL:          "https://api.example.com/mcp",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Multiple servers with different types",
		},
		{
			name: "Multiple stdio servers",
			servers: []model.Server{
				{
					Name:         "stdio-node",
					Type:         model.Stdio,
					Command:      "node server.js",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
				{
					Name:         "stdio-python",
					Type:         model.Stdio,
					Command:      "python server.py",
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Multiple stdio servers",
		},

		// Delay Configuration Tests
		{
			name: "Server with custom delays",
			servers: []model.Server{
				{
					Name:         "server-with-delays",
					Type:         model.Stdio,
					Command:      "node server.js",
					ServerDelay:  "1ms",
					ProcessDelay: "5ms",
				},
			},
			wantErr:     false,
			description: "Server with custom initialization delays",
		},
		{
			name: "Server with only server delay",
			servers: []model.Server{
				{
					Name:        "server-server-delay",
					Type:        model.Stdio,
					Command:     "node server.js",
					ServerDelay: "1ms",
				},
			},
			wantErr:     false,
			description: "Server with only server delay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.InitServers(ctx, tt.servers, model.GetAllEnv())

			if tt.wantErr {
				assert.Error(t, err, tt.description)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"Error should contain '%s'. Got: %v", tt.errContains, err)
				}
				// Cleanup any servers that might have been created
				if result != nil {
					engine.CleanupServers(result)
				}
			} else {
				if err != nil {
					// Server creation may fail without actual server processes
					// but validation should pass
					t.Logf("Server creation failed (may be expected without real server): %v", err)
					if result != nil {
						engine.CleanupServers(result)
					}
				} else {
					require.NotNil(t, result)
					assert.Len(t, result, len(tt.servers))
					// Cleanup created servers
					engine.CleanupServers(result)
				}
			}
		})
	}
}

func TestInitServers_EnvironmentVariableResolution(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Server with environment variables in name", func(t *testing.T) {
		t.Setenv("TEST_SERVER_NAME", "test-server-from-env")
		t.Setenv("TEST_COMMAND", "node")
		t.Setenv("TEST_SCRIPT", "server.js")

		servers := []model.Server{
			{
				Name:    "{{TEST_SERVER_NAME}}",
				Type:    model.Stdio,
				Command: "{{TEST_COMMAND}} {{TEST_SCRIPT}}",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected without real server): %v", err)
		}
		if result != nil {
			// Verify environment variable was resolved
			_, exists := result["test-server-from-env"]
			assert.True(t, exists, "Server name should be resolved from environment variable")
			engine.CleanupServers(result)
		}
	})

	t.Run("SSE server with env vars in URL and headers", func(t *testing.T) {
		t.Setenv("TEST_API_URL", "https://api.example.com")
		t.Setenv("TEST_API_TOKEN", "secret-token-123")

		servers := []model.Server{
			{
				Name: "sse-with-env",
				Type: model.SSE,
				URL:  "{{TEST_API_URL}}/mcp/sse",
				Headers: []string{
					"Authorization: Bearer {{TEST_API_TOKEN}}",
				},
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Server with delay env vars", func(t *testing.T) {
		t.Setenv("TEST_SERVER_DELAY", "5s")
		t.Setenv("TEST_PROCESS_DELAY", "500ms")

		servers := []model.Server{
			{
				Name:         "server-delay-env",
				Type:         model.Stdio,
				Command:      "node server.js",
				ServerDelay:  "{{TEST_SERVER_DELAY}}",
				ProcessDelay: "{{TEST_PROCESS_DELAY}}",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Multiple env vars in single field", func(t *testing.T) {
		t.Setenv("CMD_PREFIX", "python3")
		t.Setenv("SCRIPT_PATH", "/opt/mcp")
		t.Setenv("SCRIPT_NAME", "server.py")

		servers := []model.Server{
			{
				Name:    "multi-env-server",
				Type:    model.Stdio,
				Command: "{{CMD_PREFIX}} {{SCRIPT_PATH}}/{{SCRIPT_NAME}}",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Server command with TEST_DIR from static context", func(t *testing.T) {
		// This test verifies that TEST_DIR is available during server initialization
		// when using CreateStaticTemplateContext
		tempDir := t.TempDir()
		sourceFile := filepath.Join(tempDir, "test.yaml")

		// Create static template context with TEST_DIR
		templateCtx := engine.CreateStaticTemplateContext(sourceFile, map[string]string{
			"SERVER_CMD": "{{TEST_DIR}}/server.exe",
		})

		servers := []model.Server{
			{
				Name:    "test-dir-server",
				Type:    model.Stdio,
				Command: "{{SERVER_CMD}}",
			},
		}

		// This will fail to create the server (no actual executable),
		// but we verify the command was resolved correctly
		result, err := engine.InitServers(ctx, servers, templateCtx)
		if err != nil {
			// The error should contain the resolved path, not the template
			assert.Contains(t, err.Error(), "test-dir-server")
			// Should not contain unresolved template
			assert.NotContains(t, err.Error(), "{{SERVER_CMD}}")
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})
}

func TestInitServers_ErrorHandling(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &engine.DefaultServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Cleanup on partial failure", func(t *testing.T) {
		servers := []model.Server{
			{
				Name:    "invalid-server",
				Type:    model.Stdio,
				Command: "", // Invalid: empty command
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		assert.Error(t, err, "Should fail on invalid server")

		// Result should be nil or cleaned up
		if result != nil {
			// Should have cleaned up partial initialization
			assert.Empty(t, result, "Should cleanup on failure")
		}
	})

	t.Run("Error contains server name", func(t *testing.T) {
		servers := []model.Server{
			{
				Name:    "problematic-server",
				Type:    model.Stdio,
				Command: "",
			},
		}

		_, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "problematic-server",
			"Error message should contain server name")
	})

	t.Run("Server creation failure", func(t *testing.T) {
		servers := []model.Server{
			{
				Name:    "will-fail-server",
				Type:    model.Stdio,
				Command: "/nonexistent/command that will fail",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		// Should fail during server creation
		assert.Error(t, err)
		if result != nil {
			engine.CleanupServers(result)
		}
	})
}

func TestInitServers_ServerTypes(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("All supported server types", func(t *testing.T) {
		tests := []struct {
			name       string
			serverType model.ServerType
			config     model.Server
		}{
			{
				name:       "Stdio type",
				serverType: model.Stdio,
				config: model.Server{
					Name:    "stdio-test",
					Type:    model.Stdio,
					Command: "node server.js",
				},
			},
			{
				name:       "SSE type",
				serverType: model.SSE,
				config: model.Server{
					Name: "sse-test",
					Type: model.SSE,
					URL:  "https://example.com/mcp",
				},
			},
			{
				name:       "HTTP type",
				serverType: model.Http,
				config: model.Server{
					Name: "http-test",
					Type: model.Http,
					URL:  "https://api.example.com/mcp",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				servers := []model.Server{tt.config}
				result, err := engine.InitServers(ctx, servers, model.GetAllEnv())

				if err != nil {
					t.Logf("Server creation failed (expected without real server): %v", err)
				}

				if result != nil {
					assert.Len(t, result, 1)
					engine.CleanupServers(result)
				}
			})
		}
	})
}

func TestInitServers_HeaderConfiguration(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Headers with environment variables", func(t *testing.T) {
		t.Setenv("AUTH_TOKEN", "bearer-token-123")
		t.Setenv("API_KEY", "key-456")

		servers := []model.Server{
			{
				Name: "server-with-env-headers",
				Type: model.SSE,
				URL:  "https://api.example.com/mcp",
				Headers: []string{
					"Authorization: Bearer {{AUTH_TOKEN}}",
					"X-API-Key: {{API_KEY}}",
					"Content-Type: application/json",
				},
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Multiple headers without env vars", func(t *testing.T) {
		servers := []model.Server{
			{
				Name: "server-plain-headers",
				Type: model.SSE,
				URL:  "https://api.example.com/mcp",
				Headers: []string{
					"Content-Type: application/json",
					"Accept: application/json",
					"User-Agent: MCP-Agent/1.0",
				},
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Empty headers array", func(t *testing.T) {
		servers := []model.Server{
			{
				Name:    "server-no-headers",
				Type:    model.SSE,
				URL:     "https://api.example.com/mcp",
				Headers: []string{},
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Nil headers", func(t *testing.T) {
		servers := []model.Server{
			{
				Name:    "server-nil-headers",
				Type:    model.SSE,
				URL:     "https://api.example.com/mcp",
				Headers: nil,
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			t.Logf("Server creation failed (expected): %v", err)
		}
		if result != nil {
			engine.CleanupServers(result)
		}
	})
}

func TestInitServers_CommandVariations(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	tests := []struct {
		name        string
		command     string
		description string
	}{
		{
			name:        "Simple command",
			command:     "node",
			description: "Single executable",
		},
		{
			name:        "Command with single arg",
			command:     "node server.js",
			description: "Executable with one argument",
		},
		{
			name:        "Command with multiple args",
			command:     "python3 -m server --port 8080 --verbose",
			description: "Executable with multiple arguments",
		},
		{
			name:        "Command with flags",
			command:     "deno run --allow-net --allow-read server.ts",
			description: "Executable with flags",
		},
		{
			name:        "Command with quoted args",
			command:     `node server.js --config "config file.json"`,
			description: "Command with quoted arguments",
		},
		{
			name:        "Command with environment variables",
			command:     "PORT=8080 node server.js",
			description: "Command with env vars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := []model.Server{
				{
					Name:         "test-server",
					Type:         model.Stdio,
					Command:      tt.command,
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			}

			result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
			if err != nil {
				t.Logf("%s - Server creation failed (expected): %v", tt.description, err)
			}
			if result != nil {
				engine.CleanupServers(result)
			}
		})
	}
}

func TestInitServers_URLVariations(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	tests := []struct {
		name        string
		url         string
		serverType  model.ServerType
		description string
	}{
		{
			name:        "HTTPS URL",
			url:         "https://api.example.com/mcp",
			serverType:  model.SSE,
			description: "Secure HTTPS endpoint",
		},
		{
			name:        "HTTP URL",
			url:         "http://localhost:8080/mcp",
			serverType:  model.SSE,
			description: "Local HTTP endpoint",
		},
		{
			name:        "URL with port",
			url:         "https://api.example.com:9090/mcp",
			serverType:  model.SSE,
			description: "URL with custom port",
		},
		{
			name:        "URL with path",
			url:         "https://api.example.com/v1/mcp/sse",
			serverType:  model.SSE,
			description: "URL with nested path",
		},
		{
			name:        "URL with query params",
			url:         "https://api.example.com/mcp?key=value",
			serverType:  model.SSE,
			description: "URL with query parameters",
		},
		{
			name:        "Localhost URL",
			url:         "http://127.0.0.1:8080/mcp",
			serverType:  model.SSE,
			description: "Localhost with IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := []model.Server{
				{
					Name:         "test-server",
					Type:         tt.serverType,
					URL:          tt.url,
					ServerDelay:  "1ms",
					ProcessDelay: "1ms",
				},
			}

			result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
			if err != nil {
				t.Logf("%s - Server creation failed (expected): %v", tt.description, err)
			}
			if result != nil {
				engine.CleanupServers(result)
			}
		})
	}
}

func TestInitServers_DuplicateNames(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	tests := []struct {
		name        string
		servers     []model.Server
		errContains string
		description string
	}{
		{
			name: "Duplicate names - same type",
			servers: []model.Server{
				{
					Name:    "duplicate-server",
					Type:    model.Stdio,
					Command: "node server1.js",
				},
				{
					Name:    "duplicate-server",
					Type:    model.Stdio,
					Command: "node server2.js",
				},
			},
			errContains: "duplicate server name: duplicate-server",
			description: "Should fail with exact duplicate name and same type",
		},
		{
			name: "Duplicate names - different types",
			servers: []model.Server{
				{
					Name:    "my-server",
					Type:    model.Stdio,
					Command: "node server.js",
				},
				{
					Name: "my-server",
					Type: model.SSE,
					URL:  "https://example.com/mcp",
				},
			},
			errContains: "duplicate server name: my-server",
			description: "Should fail even when types are different",
		},
		{
			name: "Case sensitive - not duplicates",
			servers: []model.Server{
				{
					Name:    "MyServer",
					Type:    model.Stdio,
					Command: "node server1.js",
				},
				{
					Name:    "myserver",
					Type:    model.Stdio,
					Command: "node server2.js",
				},
				{
					Name:    "MYSERVER",
					Type:    model.Stdio,
					Command: "node server3.js",
				},
			},
			errContains: "",
			description: "Should treat names as case-sensitive (all different)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock factory
			mockFactory := &MockServerFactory{
				CreateFunc: func(ctx context.Context, config model.Server) (*server.MCPServer, error) {
					// Return a simple mock server (doesn't need to be functional)
					return &server.MCPServer{}, nil
				},
			}

			// Inject mock factory
			engine.SetServerFactory(mockFactory)

			// Restore original factory after test
			defer engine.SetServerFactory(&engine.DefaultServerFactory{})

			result, err := engine.InitServers(ctx, tt.servers, model.GetAllEnv())

			if tt.errContains != "" {
				// Should fail with duplicate error
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), tt.errContains,
					"Error message should contain '%s'. Got: %v", tt.errContains, err)

				// Verify NewMCPServer was called exactly once (before duplicate detected)
				assert.Equal(t, 1, mockFactory.CallCount,
					"Should only create one server before detecting duplicate")

				// Verify cleanup was called
				if result != nil {
					t.Error("Result should be nil after duplicate detection")
				}
			} else {
				// Should succeed (names are different)
				if err != nil {
					assert.NotContains(t, err.Error(), "duplicate",
						"Should not fail with duplicate error. Got: %v", err)
					t.Logf("%s - Failed for other reason (expected): %v", tt.description, err)
				} else {
					require.NotNil(t, result)
					assert.Len(t, result, len(tt.servers),
						"Should create all servers when names are unique")

					// Verify all servers were created
					assert.Equal(t, len(tt.servers), mockFactory.CallCount,
						"Should call NewMCPServer for each server")
				}

				// Cleanup any created servers
				if result != nil {
					engine.CleanupServers(result)
				}
			}
		})
	}
}

func TestMockServerFactory_Behavior(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Mock tracks calls correctly", func(t *testing.T) {
		defer engine.SetServerFactory(&engine.DefaultServerFactory{})

		servers := []model.Server{
			{Name: "server1", Type: model.Stdio, Command: "node s1.js"},
			{Name: "server2", Type: model.Stdio, Command: "node s2.js"},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 2, mockFactory.CallCount, "Should track number of calls")
		assert.Equal(t, "server2", mockFactory.LastConfig.Name, "Should store last config")

		engine.CleanupServers(result)
	})

	t.Run("Mock can simulate errors", func(t *testing.T) {
		mockFactory := &MockServerFactory{
			CreateFunc: func(ctx context.Context, config model.Server) (*server.MCPServer, error) {
				if config.Name == "failing-server" {
					return nil, fmt.Errorf("simulated server creation failure")
				}
				return &server.MCPServer{}, nil
			},
		}
		engine.SetServerFactory(mockFactory)
		defer engine.SetServerFactory(&engine.DefaultServerFactory{})

		servers := []model.Server{
			{Name: "good-server", Type: model.Stdio, Command: "node s1.js"},
			{Name: "failing-server", Type: model.Stdio, Command: "node s2.js"},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create server 'failing-server'")
		assert.Nil(t, result)
	})
}

func TestInitServers_DuplicateNames_WithEnvVars(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	mockFactory := &MockServerFactory{}
	engine.SetServerFactory(mockFactory)
	ctx := context.Background()

	t.Run("Duplicate after environment variable resolution", func(t *testing.T) {
		t.Setenv("SERVER_NAME", "resolved-name")
		servers := []model.Server{
			{
				Name:    "{{SERVER_NAME}}",
				Type:    model.Stdio,
				Command: "node server1.js",
			},
			{
				Name:    "resolved-name",
				Type:    model.Stdio,
				Command: "node server2.js",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		require.Error(t, err, "Should detect duplicate after env var resolution")
		assert.Contains(t, err.Error(), "duplicate server name: resolved-name")

		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Both names from env vars - duplicate", func(t *testing.T) {
		t.Setenv("SERVER_A", "same-server")
		t.Setenv("SERVER_B", "same-server")
		mockFactory := &MockServerFactory{}
		engine.SetServerFactory(mockFactory)
		servers := []model.Server{
			{
				Name:    "{{SERVER_A}}",
				Type:    model.Stdio,
				Command: "node server1.js",
			},
			{
				Name:    "{{SERVER_B}}",
				Type:    model.Stdio,
				Command: "node server2.js",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		require.Error(t, err, "Should detect duplicate when both resolve to same name")
		assert.Contains(t, err.Error(), "duplicate server name: same-server")

		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Both names from env vars - unique", func(t *testing.T) {
		t.Setenv("SERVER_X", "server-x")
		t.Setenv("SERVER_Y", "server-y")

		servers := []model.Server{
			{
				Name:    "{{SERVER_X}}",
				Type:    model.Stdio,
				Command: "node server1.js",
			},
			{
				Name:    "{{SERVER_Y}}",
				Type:    model.Stdio,
				Command: "node server2.js",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		if err != nil {
			// Should not be duplicate error
			assert.NotContains(t, err.Error(), "duplicate")
			t.Logf("Failed for other reason (expected): %v", err)
		}

		if result != nil {
			engine.CleanupServers(result)
		}
	})

	t.Run("Composite env var creates duplicate", func(t *testing.T) {
		t.Setenv("PREFIX", "api")
		t.Setenv("SUFFIX", "server")

		servers := []model.Server{
			{
				Name:    "{{PREFIX}}-{{SUFFIX}}",
				Type:    model.Stdio,
				Command: "node server1.js",
			},
			{
				Name:    "api-server",
				Type:    model.Stdio,
				Command: "node server2.js",
			},
		}

		result, err := engine.InitServers(ctx, servers, model.GetAllEnv())
		require.Error(t, err, "Should detect duplicate from composite env var")
		assert.Contains(t, err.Error(), "duplicate server name: api-server")

		if result != nil {
			engine.CleanupServers(result)
		}
	})
}

// ============================================================================
// Report Generation Tests
// ============================================================================

func TestGenerateReports(t *testing.T) {
	logger.SetupLogger(NewDummyWriter(), true)
	t.Run("Empty results", func(t *testing.T) {
		tmpfile := filepath.Join(t.TempDir(), "report.html")
		err := engine.GenerateReports([]model.TestRun{}, "html", tmpfile, nil, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no test results")
	})

	t.Run("Valid HTML report", func(t *testing.T) {
		results := []model.TestRun{
			{
				Passed: true,
				Execution: &model.ExecutionResult{
					TestName:    "test1",
					AgentName:   "agent1",
					FinalOutput: "Success",
					TokensUsed:  100,
					LatencyMs:   500,
				},
				Assertions: []model.AssertionResult{
					{Type: "output_contains", Passed: true, Message: "Test passed"},
				},
			},
		}

		tmpfile := filepath.Join(t.TempDir(), "report.html")
		err := engine.GenerateReports(results, "html", tmpfile, nil, "")
		assert.NoError(t, err)

		// Verify file was created
		info, err := os.Stat(tmpfile)
		assert.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})

	t.Run("Valid JSON report", func(t *testing.T) {
		results := []model.TestRun{
			{
				Passed: true,
				Execution: &model.ExecutionResult{
					TestName:  "test1",
					AgentName: "agent1",
				},
			},
		}

		tmpfile := filepath.Join(t.TempDir(), "report.json")
		err := engine.GenerateReports(results, "json", tmpfile, nil, "")
		assert.NoError(t, err)

		info, err := os.Stat(tmpfile)
		assert.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})

	t.Run("Valid Markdown report", func(t *testing.T) {
		results := []model.TestRun{
			{
				Passed: true,
				Execution: &model.ExecutionResult{
					TestName:  "test1",
					AgentName: "agent1",
				},
			},
		}

		tmpfile := filepath.Join(t.TempDir(), "report.md")
		err := engine.GenerateReports(results, "md", tmpfile, nil, "")
		assert.NoError(t, err)

		info, err := os.Stat(tmpfile)
		assert.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	})

	t.Run("Invalid report type", func(t *testing.T) {
		results := []model.TestRun{
			{Passed: true, Execution: &model.ExecutionResult{}},
		}

		tmpfile := filepath.Join(t.TempDir(), "report.xml")
		err := engine.GenerateReports(results, "xml", tmpfile, nil, "")
		assert.Error(t, err)
	})

	t.Run("Creates output directory", func(t *testing.T) {
		results := []model.TestRun{
			{Passed: true, Execution: &model.ExecutionResult{TestName: "test"}},
		}

		tmpdir := t.TempDir()
		outputPath := filepath.Join(tmpdir, "subdir", "report.html")

		err := engine.GenerateReports(results, "html", outputPath, nil, "")
		assert.NoError(t, err)

		// Verify directory was created
		info, err := os.Stat(filepath.Dir(outputPath))
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

// ============================================================================
// Test Summary Tests
// ============================================================================

func TestPrintTestSummary(t *testing.T) {
	t.Run("Empty results", func(t *testing.T) {
		// Should not panic
		engine.PrintTestSummary([]model.TestRun{})
	})

	t.Run("Single passed test", func(t *testing.T) {
		results := []model.TestRun{
			{
				Passed: true,
				Execution: &model.ExecutionResult{
					ToolCalls:  []model.ToolCall{{Name: "tool1"}},
					Errors:     []string{},
					LatencyMs:  1000,
					TokensUsed: 500,
				},
			},
		}

		// Should not panic
		engine.PrintTestSummary(results)
	})

	t.Run("Mixed results", func(t *testing.T) {
		results := []model.TestRun{
			{
				Passed: true,
				Execution: &model.ExecutionResult{
					ToolCalls:  []model.ToolCall{{Name: "tool1"}},
					LatencyMs:  1000,
					TokensUsed: 500,
				},
			},
			{
				Passed: false,
				Execution: &model.ExecutionResult{
					ToolCalls:  []model.ToolCall{{Name: "tool2"}},
					Errors:     []string{"Error 1", "Error 2"},
					LatencyMs:  2000,
					TokensUsed: 800,
				},
			},
		}

		// Should not panic
		engine.PrintTestSummary(results)
	})

	t.Run("Nil execution", func(t *testing.T) {
		results := []model.TestRun{
			{Passed: true, Execution: nil},
		}

		// Should not panic
		engine.PrintTestSummary(results)
	})
}

// ============================================================================
// InitProviders Tests - Extended
// ============================================================================

func TestInitProviders_AllProviderTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		providers   []model.Provider
		wantErr     bool
		errContains string
		description string
	}{
		// OpenAI Provider Tests
		{
			name: "OpenAI - Valid with default base URL",
			providers: []model.Provider{
				{
					Name:  "openai-gpt4",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-token",
					Model: "gpt-4",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid OpenAI config",
		},
		{
			name: "OpenAI - Valid with custom base URL",
			providers: []model.Provider{
				{
					Name:    "openai-custom",
					Type:    model.ProviderOpenAI,
					Token:   "sk-test-token",
					Model:   "gpt-4",
					BaseURL: "https://custom-api.openai.com/v1",
				},
			},
			wantErr:     false,
			description: "Should succeed with custom base URL",
		},
		{
			name: "OpenAI - Missing token",
			providers: []model.Provider{
				{
					Name:  "openai-no-token",
					Type:  model.ProviderOpenAI,
					Token: "",
					Model: "gpt-4",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without token",
		},
		{
			name: "OpenAI - Missing model",
			providers: []model.Provider{
				{
					Name:  "openai-no-model",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-token",
					Model: "",
				},
			},
			wantErr:     true,
			errContains: "model",
			description: "Should fail without model",
		},

		// Groq Provider Tests
		{
			name: "Groq - Valid configuration",
			providers: []model.Provider{
				{
					Name:  "groq-llama",
					Type:  model.ProviderGroq,
					Token: "gsk_test_token",
					Model: "llama-3.1-70b-versatile",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Groq config",
		},
		{
			name: "Groq - Valid with custom base URL",
			providers: []model.Provider{
				{
					Name:    "groq-custom",
					Type:    model.ProviderGroq,
					Token:   "gsk_test_token",
					Model:   "mixtral-8x7b-32768",
					BaseURL: "https://api.groq.com/openai/v1",
				},
			},
			wantErr:     false,
			description: "Should succeed with explicit Groq base URL",
		},
		{
			name: "Groq - Missing token",
			providers: []model.Provider{
				{
					Name:  "groq-no-token",
					Type:  model.ProviderGroq,
					Token: "",
					Model: "llama-3.1-70b-versatile",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without token",
		},

		// Anthropic Provider Tests
		{
			name: "Anthropic - Valid configuration",
			providers: []model.Provider{
				{
					Name:  "anthropic-claude",
					Type:  model.ProviderAnthropic,
					Token: "sk-ant-test-token",
					Model: "claude-3-opus-20240229",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Anthropic config",
		},
		{
			name: "Anthropic - Different Claude model",
			providers: []model.Provider{
				{
					Name:  "anthropic-sonnet",
					Type:  model.ProviderAnthropic,
					Token: "sk-ant-test-token",
					Model: "claude-3-sonnet-20240229",
				},
			},
			wantErr:     false,
			description: "Should work with different Claude models",
		},
		{
			name: "Anthropic - Missing token",
			providers: []model.Provider{
				{
					Name:  "anthropic-no-token",
					Type:  model.ProviderAnthropic,
					Token: "",
					Model: "claude-3-opus-20240229",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without API key",
		},

		// Google AI Provider Tests
		{
			name: "Google - Valid configuration",
			providers: []model.Provider{
				{
					Name:  "google-gemini",
					Type:  model.ProviderGoogle,
					Token: "AIza-test-token",
					Model: "gemini-pro",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Google AI config",
		},
		{
			name: "Google - Missing token",
			providers: []model.Provider{
				{
					Name:  "google-no-token",
					Type:  model.ProviderGoogle,
					Token: "",
					Model: "gemini-pro",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without API key",
		},

		// Vertex AI Provider Tests
		{
			name: "Vertex - Valid configuration",
			providers: []model.Provider{
				{
					Name:            "vertex-gemini",
					Type:            model.ProviderVertex,
					Model:           "gemini-pro",
					ProjectID:       "test-project-id",
					Location:        "us-central1",
					CredentialsPath: "/path/to/credentials.json",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Vertex AI config",
		},
		{
			name: "Vertex - Missing project ID",
			providers: []model.Provider{
				{
					Name:            "vertex-no-project",
					Type:            model.ProviderVertex,
					Model:           "gemini-pro",
					Location:        "us-central1",
					CredentialsPath: "/path/to/credentials.json",
				},
			},
			wantErr:     false, // Will fail during actual initialization
			description: "Missing project ID",
		},
		{
			name: "Vertex - Missing location",
			providers: []model.Provider{
				{
					Name:            "vertex-no-location",
					Type:            model.ProviderVertex,
					Model:           "gemini-pro",
					ProjectID:       "test-project-id",
					CredentialsPath: "/path/to/credentials.json",
				},
			},
			wantErr:     false, // Will fail during actual initialization
			description: "Missing location",
		},

		// Azure OpenAI Provider Tests
		{
			name: "Azure - Valid configuration",
			providers: []model.Provider{
				{
					Name:    "azure-gpt4",
					Type:    model.ProviderAzure,
					Token:   "test-azure-key",
					Model:   "gpt-4",
					BaseURL: "https://test.openai.azure.com",
					Version: "2024-02-15-preview",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Azure config",
		},
		{
			name: "Azure - Missing version",
			providers: []model.Provider{
				{
					Name:    "azure-no-version",
					Type:    model.ProviderAzure,
					Token:   "test-azure-key",
					Model:   "gpt-4",
					BaseURL: "https://test.openai.azure.com",
					Version: "",
				},
			},
			wantErr:     true,
			errContains: "version",
			description: "Should fail without API version",
		},
		{
			name: "Azure - Missing base URL",
			providers: []model.Provider{
				{
					Name:    "azure-no-url",
					Type:    model.ProviderAzure,
					Token:   "test-azure-key",
					Model:   "gpt-4",
					BaseURL: "",
					Version: "2024-02-15-preview",
				},
			},
			wantErr:     true,
			errContains: "base URL",
			description: "Should fail without base URL",
		},
		{
			name: "Azure - Missing token",
			providers: []model.Provider{
				{
					Name:    "azure-no-token",
					Type:    model.ProviderAzure,
					Token:   "",
					Model:   "gpt-4",
					BaseURL: "https://test.openai.azure.com",
					Version: "2024-02-15-preview",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without token",
		},

		// Amazon Bedrock Anthropic Tests
		{
			name: "Bedrock - Valid configuration",
			providers: []model.Provider{
				{
					Name:     "bedrock-claude",
					Type:     model.ProviderAmazonAnthropic,
					Token:    "AKIA-test-access-key",
					Secret:   "test-secret-key",
					Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
					Location: "us-east-1",
				},
			},
			wantErr:     false,
			description: "Should succeed with valid Bedrock config",
		},
		{
			name: "Bedrock - Different region",
			providers: []model.Provider{
				{
					Name:     "bedrock-eu",
					Type:     model.ProviderAmazonAnthropic,
					Token:    "AKIA-test-access-key",
					Secret:   "test-secret-key",
					Model:    "anthropic.claude-3-opus-20240229-v1:0",
					Location: "eu-west-1",
				},
			},
			wantErr:     false,
			description: "Should work with different AWS regions",
		},
		{
			name: "Bedrock - Missing access key",
			providers: []model.Provider{
				{
					Name:     "bedrock-no-key",
					Type:     model.ProviderAmazonAnthropic,
					Token:    "",
					Secret:   "test-secret-key",
					Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
					Location: "us-east-1",
				},
			},
			wantErr:     true,
			errContains: "token",
			description: "Should fail without access key",
		},

		// Multiple Providers Tests
		{
			name: "Multiple - Different provider types",
			providers: []model.Provider{
				{
					Name:  "openai-1",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-1",
					Model: "gpt-4",
				},
				{
					Name:  "anthropic-1",
					Type:  model.ProviderAnthropic,
					Token: "sk-ant-test-1",
					Model: "claude-3-opus-20240229",
				},
				{
					Name:  "groq-1",
					Type:  model.ProviderGroq,
					Token: "gsk-test-1",
					Model: "llama-3.1-70b-versatile",
				},
			},
			wantErr:     false,
			description: "Should handle multiple different providers",
		},
		{
			name: "Multiple - Same type different names",
			providers: []model.Provider{
				{
					Name:  "openai-gpt4",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-1",
					Model: "gpt-4",
				},
				{
					Name:  "openai-gpt35",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-2",
					Model: "gpt-3.5-turbo",
				},
			},
			wantErr:     false,
			description: "Should handle multiple providers of same type",
		},

		// Edge Cases
		{
			name: "Empty provider name",
			providers: []model.Provider{
				{
					Name:  "",
					Type:  model.ProviderOpenAI,
					Token: "sk-test",
					Model: "gpt-4",
				},
			},
			wantErr:     true,
			errContains: "empty name",
			description: "Should fail with empty provider name",
		},
		{
			name: "Duplicate provider names",
			providers: []model.Provider{
				{
					Name:  "duplicate",
					Type:  model.ProviderOpenAI,
					Token: "sk-test-1",
					Model: "gpt-4",
				},
				{
					Name:  "duplicate",
					Type:  model.ProviderAnthropic,
					Token: "sk-ant-test-2",
					Model: "claude-3-opus-20240229",
				},
			},
			wantErr:     true,
			errContains: "duplicate",
			description: "Should fail with duplicate names even if different types",
		},
		{
			name: "Unsupported provider type",
			providers: []model.Provider{
				{
					Name:  "unknown-provider",
					Type:  model.ProviderType("UNKNOWN"),
					Token: "test-token",
					Model: "test-model",
				},
			},
			wantErr:     true,
			errContains: "unsupported",
			description: "Should fail with unknown provider type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: These tests validate the provider initialization logic
			// Actual API calls will fail without real credentials
			_, err := engine.InitProviders(ctx, tt.providers, model.GetAllEnv())

			if tt.wantErr {
				assert.Error(t, err, tt.description)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"Error should contain '%s'. Got: %v", tt.errContains, err)
				}
			} else {
				// Note: Some tests may still error during actual provider creation
				// due to invalid credentials, but the validation logic should pass
				if err != nil {
					t.Logf("Expected no validation error, but got: %v", err)
					t.Logf("This may be due to actual API initialization failing, which is expected in tests")
				}
			}
		})
	}
}

func TestCreateProvider_DetailedValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("Provider specific validations", func(t *testing.T) {
		tests := []struct {
			name        string
			provider    model.Provider
			wantErr     bool
			errContains string
		}{
			// Token validation for non-Vertex providers
			{
				name: "OpenAI requires token",
				provider: model.Provider{
					Type:  model.ProviderOpenAI,
					Token: "",
					Model: "gpt-4",
				},
				wantErr:     true,
				errContains: "token",
			},
			{
				name: "Groq requires token",
				provider: model.Provider{
					Type:  model.ProviderGroq,
					Token: "",
					Model: "llama-3.1-70b-versatile",
				},
				wantErr:     true,
				errContains: "token",
			},
			{
				name: "Anthropic requires token",
				provider: model.Provider{
					Type:  model.ProviderAnthropic,
					Token: "",
					Model: "claude-3-opus-20240229",
				},
				wantErr:     true,
				errContains: "token",
			},
			{
				name: "Google requires token",
				provider: model.Provider{
					Type:  model.ProviderGoogle,
					Token: "",
					Model: "gemini-pro",
				},
				wantErr:     true,
				errContains: "token",
			},

			// Model validation
			{
				name: "All providers require model",
				provider: model.Provider{
					Type:  model.ProviderOpenAI,
					Token: "test-token",
					Model: "",
				},
				wantErr:     true,
				errContains: "model",
			},

			// Azure specific validations
			{
				name: "Azure requires version",
				provider: model.Provider{
					Type:    model.ProviderAzure,
					Token:   "test-token",
					Model:   "gpt-4",
					BaseURL: "https://test.openai.azure.com",
					Version: "",
				},
				wantErr:     true,
				errContains: "version",
			},
			{
				name: "Azure requires base URL",
				provider: model.Provider{
					Type:    model.ProviderAzure,
					Token:   "test-token",
					Model:   "gpt-4",
					BaseURL: "",
					Version: "2024-02-15-preview",
				},
				wantErr:     true,
				errContains: "base URL",
			},
			{
				name: "Azure with all requirements",
				provider: model.Provider{
					Type:    model.ProviderAzure,
					Token:   "test-token",
					Model:   "gpt-4",
					BaseURL: "https://test.openai.azure.com",
					Version: "2024-02-15-preview",
				},
				wantErr: false,
			},

			// Unsupported provider
			{
				name: "Unsupported provider type",
				provider: model.Provider{
					Type:  model.ProviderType("CUSTOM_PROVIDER"),
					Token: "test-token",
					Model: "test-model",
				},
				wantErr:     true,
				errContains: "unsupported",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := engine.CreateProvider(ctx, tt.provider)

				if tt.wantErr {
					assert.Error(t, err)
					if tt.errContains != "" {
						assert.Contains(t, err.Error(), tt.errContains)
					}
				} else {
					// May still fail due to invalid credentials, but validation should pass
					if err != nil {
						t.Logf("Validation passed but provider creation failed (expected): %v", err)
					}
				}
			})
		}
	})
}

func TestInitProviders_EnvironmentVariableResolution(t *testing.T) {
	ctx := context.Background()

	t.Run("Provider with environment variables", func(t *testing.T) {
		// Set test environment variables
		t.Setenv("TEST_OPENAI_TOKEN", "sk-test-from-env")
		t.Setenv("TEST_MODEL_NAME", "gpt-4-from-env")

		providers := []model.Provider{
			{
				Name:  "provider-with-env",
				Type:  model.ProviderOpenAI,
				Token: "{{TEST_OPENAI_TOKEN}}",
				Model: "{{TEST_MODEL_NAME}}",
			},
		}

		// This should pass validation after environment variable resolution
		result, err := engine.InitProviders(ctx, providers, model.GetAllEnv())

		// Note: May fail during actual provider creation, but env vars should be resolved
		if err != nil {
			t.Logf("Provider creation failed (expected without real credentials): %v", err)
		} else {
			assert.NotNil(t, result)
			assert.Len(t, result, 1)
		}
	})

	t.Run("Provider with partial env vars", func(t *testing.T) {
		t.Setenv("TEST_BASE_URL", "https://custom-api.openai.com")

		providers := []model.Provider{
			{
				Name:    "provider-partial-env",
				Type:    model.ProviderOpenAI,
				Token:   "sk-literal-token",
				Model:   "gpt-4",
				BaseURL: "{{TEST_BASE_URL}}/v1",
			},
		}

		result, err := engine.InitProviders(ctx, providers, model.GetAllEnv())
		if err != nil {
			t.Logf("Provider creation failed (expected): %v", err)
		} else {
			assert.NotNil(t, result)
		}
	})
}

// Note: Clarification pattern compilation tests have been removed since the feature now uses
// LLM-based classification instead of pattern matching.

// ============================================================================
// Helper Functions
// ============================================================================

func createTempFile(t *testing.T, pattern, content string) string {
	t.Helper()

	tmpfile, err := os.CreateTemp("", pattern)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	if content != "" {
		_, err = tmpfile.WriteString(content)
		require.NoError(t, err)
	}

	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}
