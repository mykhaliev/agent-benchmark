package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/googleai/vertex"
	"github.com/tmc/langchaingo/llms/openai"
)

const (
	DefaultMaxIterations = 10
	DefaultTimeout       = 0 * time.Second
	DefaultTestDelay     = 0 * time.Second
)

func Run(testPath *string, verbose *bool, suitePath *string, outputPath *string, reportType *string) {
	// Run tests
	results := make([]model.TestRun, 0)

	var criteria model.Criteria
	if *testPath != "" {
		// Create a NEW context for each test file
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Validate input file exists
		if err := ValidateTestInputFile(*testPath); err != nil {
			logger.Logger.Error("Invalid input file", "error", err)
			os.Exit(1)
		}
		// Load and validate test configuration
		logger.Logger.Info("Loading test configuration")
		testConfig, err := model.ParseTestConfig(*testPath)
		if err != nil {
			logger.Logger.Error("Failed to parse configuration", "error", err)
			os.Exit(1)
		}
		// Override verbose setting if command line flag is set
		if *verbose {
			testConfig.Settings.Verbose = true
		}
		if err := ValidateTestConfig(testConfig, false); err != nil {
			logger.Logger.Error("Invalid configuration", "error", err)
			os.Exit(1)
		}
		totalTests := 0
		for _, session := range testConfig.Sessions {
			totalTests += len(session.Tests)
		}

		logger.Logger.Info("Configuration loaded",
			"providers", len(testConfig.Providers),
			"servers", len(testConfig.Servers),
			"agents", len(testConfig.Agents),
			"sessions", len(testConfig.Sessions),
			"tests", totalTests)

		// Initialize components using the passed context
		providers, err := InitProviders(ctx, testConfig.Providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize providers", "error", err)
			os.Exit(1)
		}

		mcpServers, err := InitServers(ctx, testConfig.Servers)
		if err != nil {
			logger.Logger.Error("Failed to initialize servers", "error", err)
			os.Exit(1)
		}
		// Cleanup servers when this test file completes
		defer CleanupServers(mcpServers)

		agents, err := initAgents(ctx, testConfig.Agents, mcpServers, providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize agents", "error", err)
			os.Exit(1)
		}

		// Parse settings
		toolTimeout := ParseTimeout(testConfig.Settings.ToolTimeout)
		testDelay := ParseDelay(testConfig.Settings.TestDelay)
		maxIterations := GetMaxIterations(testConfig.Settings.MaxIterations)

		logger.Logger.Info("Test settings configured",
			"max_iterations", maxIterations,
			"tool_timeout", toolTimeout,
			"test_delay", testDelay,
			"verbose", testConfig.Settings.Verbose)

		// Run tests
		logger.Logger.Info("Starting test execution")
		testResults := runTests(ctx, testConfig, agents, maxIterations, toolTimeout, testDelay)
		results = append(results, testResults...)
		if len(testResults) > 0 {
			criteria = testResults[0].TestCriteria
		}
	}

	if *suitePath != "" {
		if err := ValidateTestInputFile(*suitePath); err != nil {
			logger.Logger.Error("Invalid input file", "error", err)
			os.Exit(1)
		}

		logger.Logger.Info("Loading test suite configuration")
		testSuiteConfig, err := model.ParseSuiteConfig(*suitePath)
		if err != nil {
			logger.Logger.Error("Failed to parse suite configuration", "error", err)
			os.Exit(1)
		}
		if err := ValidateSuiteConfig(testSuiteConfig); err != nil {
			logger.Logger.Error("Invalid configuration", "error", err)
			os.Exit(1)
		}

		if testSuiteConfig == nil || testSuiteConfig.TestFiles == nil {
			logger.Logger.Error("No test files found in suite configuration")
			os.Exit(1)
		}
		// Create a suite level context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		logger.Logger.Info("Running test suite", "name", testSuiteConfig.Name)
		// Initialize components using the passed context
		providers, err := InitProviders(ctx, testSuiteConfig.Providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize providers", "error", err)
			os.Exit(1)
		}

		mcpServers, err := InitServers(ctx, testSuiteConfig.Servers)
		if err != nil {
			logger.Logger.Error("Failed to initialize servers", "error", err)
			os.Exit(1)
		}
		// Cleanup servers when this test file completes
		defer CleanupServers(mcpServers)

		agents, err := initAgents(ctx, testSuiteConfig.Agents, mcpServers, providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize agents", "error", err)
			os.Exit(1)
		}

		// Parse settings
		toolTimeout := ParseTimeout(testSuiteConfig.Settings.ToolTimeout)
		testDelay := ParseDelay(testSuiteConfig.Settings.TestDelay)
		maxIterations := GetMaxIterations(testSuiteConfig.Settings.MaxIterations)

		logger.Logger.Info("Test settings configured",
			"max_iterations", maxIterations,
			"tool_timeout", toolTimeout,
			"test_delay", testDelay,
			"verbose", testSuiteConfig.Settings.Verbose)

		for _, testFile := range testSuiteConfig.TestFiles {
			// Validate input file exists
			if err := ValidateTestInputFile(testFile); err != nil {
				logger.Logger.Error("Invalid input file", "error", err)
				os.Exit(1)
			}
			// Load and validate test configuration
			logger.Logger.Info("Loading test configuration")
			testConfig, err := model.ParseTestConfig(testFile)
			if err != nil {
				logger.Logger.Error("Failed to parse configuration", "error", err)
				os.Exit(1)
			}
			// Override verbose setting if command line flag is set
			if *verbose {
				testConfig.Settings.Verbose = true
			}
			// override settings
			testConfig.Settings = testSuiteConfig.Settings
			// override variables
			if testSuiteConfig.Variables != nil {
				testConfig.Variables = testSuiteConfig.Variables
			}
			if err := ValidateTestConfig(testConfig, true); err != nil {
				logger.Logger.Error("Invalid configuration", "error", err)
				os.Exit(1)
			}

			totalTests := 0
			for _, session := range testConfig.Sessions {
				totalTests += len(session.Tests)
			}

			logger.Logger.Info("Configuration loaded",
				"providers", len(testConfig.Providers),
				"servers", len(testConfig.Servers),
				"agents", len(testConfig.Agents),
				"sessions", len(testConfig.Sessions),
				"tests", totalTests)
			// Run tests
			logger.Logger.Info("Starting test execution")
			testResults := runTests(ctx, testConfig, agents, maxIterations, toolTimeout, testDelay)
			results = append(results, testResults...)
		}
		criteria = testSuiteConfig.TestCriteria
	}

	// Generate and save reports
	logger.Logger.Info("Generating reports")
	if *outputPath == "" {
		*outputPath = "report." + *reportType
	}
	if err := GenerateReports(results, *reportType, *outputPath); err != nil {
		logger.Logger.Error("Failed to generate reports", "error", err)
		os.Exit(1)
	}

	// Exit with appropriate code
	if criteria.SuccessRate == "" {
		if HasFailures(results) {
			logger.Logger.Warn("Tests completed with failures")
			os.Exit(1)
		}
	} else {
		successRate, err := strconv.ParseFloat(criteria.SuccessRate, 64)
		if err != nil {
			logger.Logger.Error("Failed to parse criteria success rate", "error", err)
			if HasFailures(results) {
				logger.Logger.Warn("Tests completed with failures")
				os.Exit(1)
			}
		}
		passedTests := 0
		failedTests := 0
		for _, result := range results {
			if result.Passed {
				passedTests++
			} else {
				failedTests++
			}
		}
		passRate := float64(passedTests) / float64(len(results))
		if successRate <= passRate {
			logger.Logger.Info("Tests suite success rate matched", "criteria", successRate, "actual", passRate)
			os.Exit(0)
		} else {
			logger.Logger.Warn("Tests suite success rate not matched", "criteria", successRate, "actual", passRate)
			os.Exit(1)
		}
	}
	logger.Logger.Info("All tests passed successfully")
	os.Exit(0)
}

func ValidateTestInputFile(path string) error {
	if path == "" {
		return fmt.Errorf("input file path is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		return fmt.Errorf("cannot access file %s: %w", path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}

	if info.Size() == 0 {
		return fmt.Errorf("file is empty: %s", path)
	}

	// Check file extension
	ext := filepath.Ext(path)
	if ext != ".yaml" && ext != ".yml" {
		logger.Logger.Warn("Unexpected file extension", "extension", ext, "expected", ".yaml, .yml")
		return fmt.Errorf("unexpected file extension: %s", ext)
	}

	return nil
}

func ValidateTestConfig(config *model.TestConfiguration, runningFromSuite bool) error {
	if config == nil {
		return fmt.Errorf("configuration is nil")
	}

	if !runningFromSuite {
		if len(config.Providers) == 0 {
			return fmt.Errorf("no providers configured")
		}

		if len(config.Servers) == 0 {
			return fmt.Errorf("no servers configured")
		}

		if len(config.Agents) == 0 {
			return fmt.Errorf("no agents configured")
		}
	}
	if len(config.Sessions) == 0 {
		return fmt.Errorf("no sessions configured")
	}

	return nil
}

func ValidateSuiteConfig(config *model.TestSuiteConfiguration) error {
	if config == nil {
		return fmt.Errorf("configuration is nil")
	}

	if len(config.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	if len(config.Servers) == 0 {
		return fmt.Errorf("no servers configured")
	}

	if len(config.Agents) == 0 {
		return fmt.Errorf("no agents configured")
	}

	return nil
}

func ValidateReportType(reportType string) error {
	if reportType != "json" && reportType != "html" && reportType != "md" {
		return fmt.Errorf("unknown type %s, supported types are: json, html, md", reportType)
	}
	return nil
}

func InitProviders(ctx context.Context, providerConfigs []model.Provider) (map[string]llms.Model, error) {
	if len(providerConfigs) == 0 {
		return nil, fmt.Errorf("no providers to initialize")
	}

	logger.Logger.Info("Initializing providers", "count", len(providerConfigs))
	providers := make(map[string]llms.Model)

	for i, p := range providerConfigs {
		// resolve ENV variables
		envs := model.GetAllEnv()
		// replace ENVs in config
		p.Name = model.RenderTemplate(p.Name, envs)
		p.Token = model.RenderTemplate(p.Token, envs)
		p.Model = model.RenderTemplate(p.Model, envs)
		p.BaseURL = model.RenderTemplate(p.BaseURL, envs)
		p.Version = model.RenderTemplate(p.Version, envs)
		p.ProjectID = model.RenderTemplate(p.ProjectID, envs)
		p.Location = model.RenderTemplate(p.Location, envs)
		p.CredentialsPath = model.RenderTemplate(p.CredentialsPath, envs)
		logger.Logger.Debug("Initializing provider",
			"index", i+1,
			"total", len(providerConfigs),
			"name", p.Name,
			"type", p.Type,
			"model", p.Model)

		if p.Name == "" {
			return nil, fmt.Errorf("provider at index %d has empty name", i)
		}

		if _, exists := providers[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name: %s", p.Name)
		}

		llmModel, err := CreateProvider(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider '%s': %w", p.Name, err)
		}

		providers[p.Name] = llmModel
		logger.Logger.Info("Provider initialized", "name", p.Name)
	}

	logger.Logger.Info("All providers initialized", "count", len(providers))
	return providers, nil
}

func CreateProvider(ctx context.Context, p model.Provider) (llms.Model, error) {
	if p.Type != model.ProviderVertex && p.Token == "" {
		return nil, fmt.Errorf("provider token is empty")
	}

	if p.Model == "" {
		return nil, fmt.Errorf("provider model is empty")
	}

	var llmModel llms.Model
	var err error

	switch p.Type {
	case model.ProviderGroq:
		opts := []openai.Option{
			openai.WithToken(p.Token),
			openai.WithModel(p.Model),
		}
		if p.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(p.BaseURL))
			logger.Logger.Debug("Using custom base URL", "url", p.BaseURL)
		} else {
			opts = append(opts, openai.WithBaseURL("https://api.groq.com/openai/v1"))
		}
		llmModel, err = openai.New(opts...)
	case model.ProviderGoogle:
		llmModel, err = googleai.New(
			ctx,
			googleai.WithAPIKey(p.Token),
			googleai.WithDefaultModel(p.Model),
		)
	case model.ProviderVertex:
		llmModel, err = vertex.New(
			ctx,
			googleai.WithDefaultModel(p.Model),
			googleai.WithCloudProject(p.ProjectID),
			googleai.WithCloudLocation(p.Location),
			googleai.WithCredentialsFile(p.CredentialsPath),
		)
	case model.ProviderAnthropic:
		opts := []anthropic.Option{
			anthropic.WithModel(p.Model),
			anthropic.WithToken(p.Token),
		}
		llmModel, err = anthropic.New(opts...)
	case model.ProviderAmazonAnthropic:
		cfg, err := config.LoadDefaultConfig(context.Background(),
			config.WithRegion(p.Location),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				p.Token,
				p.Secret,
				"",
			)),
		)
		if err != nil {
			log.Fatal(err)
		}
		brc := bedrockruntime.NewFromConfig(cfg)
		llmModel, err = bedrock.New(
			bedrock.WithClient(brc),
			bedrock.WithModel(p.Model),
		)
	case model.ProviderOpenAI:
		opts := []openai.Option{
			openai.WithToken(p.Token),
			openai.WithModel(p.Model),
		}

		if p.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(p.BaseURL))
			logger.Logger.Debug("Using custom base URL", "url", p.BaseURL)
		}

		llmModel, err = openai.New(opts...)

	case model.ProviderAzure:
		if p.Version == "" {
			return nil, fmt.Errorf("Azure provider requires version")
		}

		opts := []openai.Option{
			openai.WithToken(p.Token),
			openai.WithModel(p.Model),
			openai.WithAPIType(openai.APITypeAzure),
			openai.WithAPIVersion(p.Version),
		}

		if p.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(p.BaseURL))
			logger.Logger.Debug("Using Azure base URL", "url", p.BaseURL)
		} else {
			return nil, fmt.Errorf("Azure provider requires base URL")
		}

		llmModel, err = openai.New(opts...)

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", p.Type)
	}

	if err != nil {
		return nil, err
	}

	if llmModel == nil {
		return nil, fmt.Errorf("provider created but model is nil")
	}

	return llmModel, nil
}

// ServerFactory creates MCP servers
type ServerFactory interface {
	NewMCPServer(ctx context.Context, config model.Server) (*server.MCPServer, error)
}

// DefaultServerFactory is the production implementation
type DefaultServerFactory struct{}

func (f *DefaultServerFactory) NewMCPServer(ctx context.Context, config model.Server) (*server.MCPServer, error) {
	return server.NewMCPServer(ctx, config)
}

// Package-level variable for dependency injection
var serverFactory ServerFactory = &DefaultServerFactory{}

// SetServerFactory allows tests to inject mock factories
func SetServerFactory(factory ServerFactory) {
	serverFactory = factory
}

func InitServers(ctx context.Context, serverConfigs []model.Server) (map[string]*server.MCPServer, error) {
	if len(serverConfigs) == 0 {
		return nil, fmt.Errorf("no servers to initialize")
	}

	logger.Logger.Info("Initializing servers", "count", len(serverConfigs))
	servers := make(map[string]*server.MCPServer)

	for i, s := range serverConfigs {
		// resolve ENV variables
		envs := model.GetAllEnv()
		// replace ENVs in config
		s.Name = model.RenderTemplate(s.Name, envs)
		s.Command = model.RenderTemplate(s.Command, envs)
		s.URL = model.RenderTemplate(s.URL, envs)
		s.ServerDelay = model.RenderTemplate(s.ServerDelay, envs)
		s.ProcessDelay = model.RenderTemplate(s.ProcessDelay, envs)
		if s.Headers != nil {
			for k := range s.Headers {
				s.Headers[k] = model.RenderTemplate(s.Headers[k], envs)
			}
		}
		s.Command = model.RenderTemplate(s.Command, envs)

		logger.Logger.Debug("Initializing server",
			"index", i+1,
			"total", len(serverConfigs),
			"name", s.Name,
			"type", s.Type)

		if s.Name == "" {
			return nil, fmt.Errorf("server at index %d has empty name", i)
		}

		if _, exists := servers[s.Name]; exists {
			return nil, fmt.Errorf("duplicate server name: %s", s.Name)
		}

		// Use the factory instead of direct call
		mcpServer, err := serverFactory.NewMCPServer(ctx, s)
		if err != nil {
			CleanupServers(servers)
			return nil, fmt.Errorf("failed to create server '%s': %w", s.Name, err)
		}
		servers[s.Name] = mcpServer
		logger.Logger.Info("Server initialized", "name", s.Name)
	}

	logger.Logger.Info("All servers initialized", "count", len(servers))
	return servers, nil
}

func initAgents(
	ctx context.Context,
	agentConfigs []model.Agent,
	mcpServers map[string]*server.MCPServer,
	providers map[string]llms.Model,
) (map[string]*agent.MCPAgent, error) {
	if len(agentConfigs) == 0 {
		return nil, fmt.Errorf("no agents to initialize")
	}

	logger.Logger.Info("Initializing agents", "count", len(agentConfigs))
	agents := make(map[string]*agent.MCPAgent)

	for i, a := range agentConfigs {
		logger.Logger.Debug("Initializing agent",
			"index", i+1,
			"total", len(agentConfigs),
			"name", a.Name,
			"provider", a.Provider)

		if a.Name == "" {
			return nil, fmt.Errorf("agent at index %d has empty name", i)
		}

		if _, exists := agents[a.Name]; exists {
			return nil, fmt.Errorf("duplicate agent name: %s", a.Name)
		}

		// Get provider
		llmModel, ok := providers[a.Provider]
		if !ok {
			return nil, fmt.Errorf("provider '%s' not found for agent '%s'", a.Provider, a.Name)
		}

		if llmModel == nil {
			return nil, fmt.Errorf("provider '%s' is nil for agent '%s'", a.Provider, a.Name)
		}

		if len(a.Servers) == 0 {
			return nil, fmt.Errorf("agent '%s' has no servers configured", a.Name)
		}

		// Build agent server list
		agentServers := make([]model.AgentServer, 0, len(a.Servers))
		agentMCPServers := make([]*server.MCPServer, 0, len(a.Servers))

		for _, srv := range a.Servers {
			if srv.Name == "" {
				return nil, fmt.Errorf("agent '%s' has server with empty name", a.Name)
			}

			mcpServer, ok := mcpServers[srv.Name]
			if !ok {
				return nil, fmt.Errorf("server '%s' not found for agent '%s'", srv.Name, a.Name)
			}

			if mcpServer == nil {
				return nil, fmt.Errorf("server '%s' is nil for agent '%s'", srv.Name, a.Name)
			}

			agentServers = append(agentServers, model.AgentServer{
				Name:         srv.Name,
				AllowedTools: srv.AllowedTools,
			})
			agentMCPServers = append(agentMCPServers, mcpServer)
		}

		logger.Logger.Debug("Agent server configuration",
			"agent", a.Name,
			"server_count", len(agentServers),
			"servers", GetServerNames(agentServers))

		// Create agent
		mcpAgent := agent.NewMCPAgent(
			ctx,
			a.Name,
			agentServers,
			agentMCPServers,
			a.Provider,
			llmModel,
		)

		if mcpAgent == nil {
			return nil, fmt.Errorf("failed to create agent '%s': agent is nil", a.Name)
		}

		agents[a.Name] = mcpAgent
		logger.Logger.Info("Agent initialized", "name", a.Name)
	}

	logger.Logger.Info("All agents initialized", "count", len(agents))
	return agents, nil
}

func runTests(
	ctx context.Context,
	testConfig *model.TestConfiguration,
	agents map[string]*agent.MCPAgent,
	maxIterations int,
	toolTimeout time.Duration,
	testDelay time.Duration,
) []model.TestRun {
	results := make([]model.TestRun, 0)

	// Calculate total tests across all sessions and agents
	totalTests := 0
	for _, session := range testConfig.Sessions {
		totalTests += len(agents) * len(session.Tests)
	}
	testCount := 0

	logger.Logger.Info("Running tests",
		"total_tests", totalTests,
		"agents", len(agents),
		"sessions", len(testConfig.Sessions))

	for _, agentConfig := range agents {
		ag, ok := agents[agentConfig.Name]
		if !ok {
			logger.Logger.Warn("Agent not found, skipping tests", "agent", agentConfig.Name)
			continue
		}

		logger.Logger.Info("Starting tests for agent",
			"agent", agentConfig.Name,
			"total", len(agents))

		allAgentTools := ag.ExtractToolsFromAgent()
		// Iterate through sessions
		for sessionIdx, session := range testConfig.Sessions {
			logger.Logger.Info("Starting session",
				"session", session.Name,
				"agent", agentConfig.Name,
				"index", sessionIdx+1,
				"total", len(testConfig.Sessions))

			// Reload variables and reset message history for each session
			templateCtx := CreateTemplateContext(testConfig.Variables)

			// Initialize fresh message history for this session
			msgs := make([]llms.MessageContent, 0)
			sessionTools := allAgentTools // Don't mutate original
			if session.AllowedTools != nil {
				sessionTools = make([]llms.Tool, 0)
				for i := range allAgentTools { // Filter from allAgentTools
					for _, allowedTool := range session.AllowedTools {
						if allAgentTools[i].Function.Name == allowedTool {
							sessionTools = append(sessionTools, allAgentTools[i])
						}
					}
				}
			}

			// Run tests within this session
			for testIdx, test := range session.Tests {
				testCount++

				if test.Name == "" {
					logger.Logger.Warn("Test has no name", "index", testIdx)
				}

				logger.Logger.Info("Running test",
					"test", test.Name,
					"number", testCount,
					"total", totalTests,
					"agent", agentConfig.Name,
					"session", session.Name)

				testTools := sessionTools // Start from session tools
				if test.AllowedTools != nil {
					testTools = make([]llms.Tool, 0)
					for i := range sessionTools { // Filter from sessionTools
						for _, allowedTool := range test.AllowedTools {
							if sessionTools[i].Function.Name == allowedTool {
								testTools = append(testTools, sessionTools[i])
							}
						}
					}
				}
				// Start delay
				if test.StartDelay != "" {
					startDelay := ParseDelay(test.StartDelay)
					logger.Logger.Debug("Delaying the test start", "delay", startDelay)
					time.Sleep(startDelay)
				}

				// Transform prompt with template context
				prompt := model.RenderTemplate(test.Prompt, templateCtx)
				logger.Logger.Debug("Test prompt prepared", "prompt", prompt)

				// Create message from test prompt
				msgs = append(msgs, llms.MessageContent{
					Role: llms.ChatMessageTypeHuman,
					Parts: []llms.ContentPart{
						llms.TextContent{Text: prompt},
					},
				})

				// Execute test
				startTime := time.Now()
				executionResult := ag.GenerateContentWithConfig(ctx, &msgs, agent.AgentConfig{
					MaxIterations:        maxIterations,
					ToolTimeout:          toolTimeout,
					AddNotFinalResponses: true,
					Verbose:              testConfig.Settings.Verbose,
				}, testTools)
				executionResult.TestName = test.Name
				duration := time.Since(startTime)

				logger.Logger.Info("Test execution completed",
					"test", test.Name,
					"duration", duration,
					"tool_calls", len(executionResult.ToolCalls),
					"errors", len(executionResult.Errors))
				//extract variables
				if test.Extractors != nil {
					for _, extractor := range test.Extractors {
						extractor.Extract(&executionResult, templateCtx)
					}
				}
				// Evaluate assertions
				logger.Logger.Debug("Evaluating assertions", "count", len(test.Assertions))
				evaluator := model.NewAssertionEvaluator(&executionResult, templateCtx, ag.AvailableTools)
				assertions := evaluator.Evaluate(test.Assertions)

				// Check if all assertions passed
				allPassed := true
				passedCount := 0
				for _, a := range assertions {
					if a.Passed {
						passedCount++
					} else {
						allPassed = false
					}
				}

				logger.Logger.Info("Assertion results",
					"test", test.Name,
					"passed", passedCount,
					"total", len(assertions))

				// Create test run
				testRun := model.TestRun{
					Execution:    &executionResult,
					Assertions:   assertions,
					Passed:       allPassed,
					TestCriteria: testConfig.TestCriteria,
				}

				results = append(results, testRun)

				if allPassed {
					logger.Logger.Info("Test PASSED", "test", test.Name)
				} else {
					logger.Logger.Warn("Test FAILED", "test", test.Name)
				}

				// Delay between tests if configured
				if testDelay > 0 && testCount < totalTests {
					logger.Logger.Debug("Waiting before next test", "delay", testDelay)
					time.Sleep(testDelay)
				}
			}

			logger.Logger.Info("Session completed",
				"session", session.Name,
				"agent", agentConfig.Name)
		}
	}

	return results
}

func CreateTemplateContext(variables map[string]string) map[string]string {
	templateCtx := model.GetAllEnv()

	if variables == nil {
		return templateCtx
	}
	// Pre-transform variables if they contain templates
	for k, v := range variables {
		templateCtx[k] = model.RenderTemplate(v, templateCtx)
	}
	return templateCtx
}

func GenerateReports(results []model.TestRun, reportType, outputPath string) error {
	if len(results) == 0 {
		return fmt.Errorf("no test results to generate report")
	}

	reporter := model.NewReportGenerator()

	// Generate console report
	fmt.Println("\n" + strings.Repeat("=", 80))
	reporter.GenerateConsoleReport(results)
	// Print summary
	PrintTestSummary(results)
	report := ""
	switch reportType {
	case "json":
		report = reporter.GenerateJSONReport(results)
	case "html":
		report = reporter.GenerateHTMLReport(results)
	case "md":
		report = reporter.GenerateMarkdownReport(results)
	default:
		return fmt.Errorf("Unknown report type")
	}

	if report == "" {
		return fmt.Errorf("generated HTML report is empty")
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write report to file
	err := os.WriteFile(outputPath, []byte(report), logger.FilePermission)
	if err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	// Verify file was written
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("failed to verify output file: %w", err)
	}

	logger.Logger.Info("Report generated successfully", "size", info.Size())
	return nil
}

func CleanupServers(servers map[string]*server.MCPServer) {
	if len(servers) == 0 {
		return
	}

	logger.Logger.Info("Shutting down servers", "count", len(servers))

	for name, srv := range servers {
		if srv == nil {
			continue
		}

		logger.Logger.Debug("Closing server", "name", name)
		if err := srv.Close(); err != nil {
			logger.Logger.Warn("Error closing server", "name", name, "error", err)
		} else {
			logger.Logger.Debug("Server closed", "name", name)
		}
	}

	logger.Logger.Info("Server cleanup completed")
}

func PrintTestSummary(results []model.TestRun) {
	if len(results) == 0 {
		logger.Logger.Info("No tests were run")
		return
	}

	totalTests := len(results)
	passedTests := 0
	failedTests := 0
	totalToolCalls := 0
	totalErrors := 0
	var totalDuration int64
	totalTokens := 0

	for _, result := range results {
		if result.Passed {
			passedTests++
		} else {
			failedTests++
		}

		if result.Execution != nil {
			totalToolCalls += len(result.Execution.ToolCalls)
			totalErrors += len(result.Execution.Errors)
			totalDuration += result.Execution.LatencyMs
			totalTokens += result.Execution.TokensUsed
		}
	}

	passRate := float64(passedTests) / float64(totalTests) * 100
	failRate := float64(failedTests) / float64(totalTests) * 100
	avgDuration := totalDuration / int64(totalTests)

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("[Summary] Test Execution Summary")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  Total Tests:      %d\n", totalTests)
	fmt.Printf("  Passed:           %d (%.1f%%)\n", passedTests, passRate)
	fmt.Printf("  Failed:           %d (%.1f%%)\n", failedTests, failRate)
	fmt.Printf("  Total Tool Calls: %d\n", totalToolCalls)
	fmt.Printf("  Total Errors:     %d\n", totalErrors)
	fmt.Printf("  Total Duration:   %dms (avg: %dms per test)\n", totalDuration, avgDuration)
	fmt.Printf("  Total Tokens:     %d\n", totalTokens)
	fmt.Println(strings.Repeat("=", 80))

	logger.Logger.Info("Test execution summary",
		"total_tests", totalTests,
		"passed", passedTests,
		"failed", failedTests,
		"pass_rate", fmt.Sprintf("%.1f%%", passRate),
		"tool_calls", totalToolCalls,
		"errors", totalErrors,
		"total_duration_ms", totalDuration,
		"avg_duration_ms", avgDuration,
		"tokens", totalTokens)
}

func HasFailures(results []model.TestRun) bool {
	for _, result := range results {
		if !result.Passed {
			return true
		}
	}
	return false
}

func ParseTimeout(timeoutStr string) time.Duration {
	if timeoutStr == "" {
		return DefaultTimeout
	}

	dur, err := time.ParseDuration(timeoutStr)
	if err != nil {
		logger.Logger.Warn("Invalid timeout, using default",
			"timeout", timeoutStr,
			"default", DefaultTimeout,
			"error", err)
		return DefaultTimeout
	}

	if dur < 0 {
		logger.Logger.Warn("Negative timeout, using 0", "timeout", dur)
		return 0
	}

	return dur
}

func ParseDelay(delayStr string) time.Duration {
	if delayStr == "" {
		return DefaultTestDelay
	}

	dur, err := time.ParseDuration(delayStr)
	if err != nil {
		logger.Logger.Warn("Invalid delay, using default",
			"delay", delayStr,
			"default", DefaultTestDelay,
			"error", err)
		return DefaultTestDelay
	}

	if dur < 0 {
		logger.Logger.Warn("Negative delay, using 0", "delay", dur)
		return 0
	}

	return dur
}

func GetMaxIterations(maxIter int) int {
	if maxIter <= 0 {
		return DefaultMaxIterations
	}

	if maxIter > 100 {
		logger.Logger.Warn("Max iterations is very high, consider reducing", "max_iterations", maxIter)
	}

	return maxIter
}

func GetServerNames(servers []model.AgentServer) []string {
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return names
}
