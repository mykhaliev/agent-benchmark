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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/google/uuid"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/report"
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

func Run(testPath *string, verbose *bool, suitePath *string, reportFileName *string, reportTypes []string) {
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

		// Create static template context early - includes env vars, TEST_DIR, user variables
		// This enables templates like {{TEST_DIR}}/server.exe in server commands
		staticCtx := CreateStaticTemplateContext(*testPath, testConfig.Variables)

		// Initialize components using the passed context
		providers, err := InitProviders(ctx, testConfig.Providers, staticCtx)
		if err != nil {
			logger.Logger.Error("Failed to initialize providers", "error", err)
			os.Exit(1)
		}

		// Collect required servers from agents
		requiredServers := getRequiredServers(testConfig.Agents, testConfig.Servers)
		// Initialize only required servers
		mcpServers, err := InitServers(ctx, requiredServers, staticCtx)
		if err != nil {
			logger.Logger.Error("Failed to initialize servers", "error", err)
			os.Exit(1)
		}
		defer CleanupServers(mcpServers)

		agents, err := initAgents(ctx, testConfig.Agents, mcpServers, providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize agents", "error", err)
			os.Exit(1)
		}

		// Parse settings
		toolTimeout := ParseTimeout(testConfig.Settings.ToolTimeout)
		testDelay := ParseDelay(testConfig.Settings.TestDelay)
		sessionDelay := ParseDelay(testConfig.Settings.SessionDelay)
		maxIterations := GetMaxIterations(testConfig.Settings.MaxIterations)

		logger.Logger.Info("Test settings configured",
			"max_iterations", maxIterations,
			"tool_timeout", toolTimeout,
			"test_delay", testDelay,
			"session_delay", sessionDelay,
			"verbose", testConfig.Settings.Verbose)

		// Run tests
		logger.Logger.Info("Starting test execution")
		testResults := runTests(ctx, testConfig, agents, providers, maxIterations, toolTimeout, testDelay, sessionDelay, *testPath, "")
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

		// Create static template context early - includes env vars, TEST_DIR, user variables
		// For suite, TEST_DIR is relative to the suite file (not individual test files)
		// Test-level variables are not part of the static context.
		staticCtx := CreateStaticTemplateContext(*suitePath, testSuiteConfig.Variables)

		// Initialize components using the passed context
		providers, err := InitProviders(ctx, testSuiteConfig.Providers, staticCtx)
		if err != nil {
			logger.Logger.Error("Failed to initialize providers", "error", err)
			os.Exit(1)
		}

		// Collect required servers from agents
		requiredServers := getRequiredServers(testSuiteConfig.Agents, testSuiteConfig.Servers)

		// Initialize only required servers
		mcpServers, err := InitServers(ctx, requiredServers, staticCtx)
		if err != nil {
			logger.Logger.Error("Failed to initialize servers", "error", err)
			os.Exit(1)
		}
		defer CleanupServers(mcpServers)

		agents, err := initAgents(ctx, testSuiteConfig.Agents, mcpServers, providers)
		if err != nil {
			logger.Logger.Error("Failed to initialize agents", "error", err)
			os.Exit(1)
		}

		// Parse settings
		toolTimeout := ParseTimeout(testSuiteConfig.Settings.ToolTimeout)
		testDelay := ParseDelay(testSuiteConfig.Settings.TestDelay)
		sessionDelay := ParseDelay(testSuiteConfig.Settings.SessionDelay)
		maxIterations := GetMaxIterations(testSuiteConfig.Settings.MaxIterations)

		logger.Logger.Info("Test settings configured",
			"max_iterations", maxIterations,
			"tool_timeout", toolTimeout,
			"test_delay", testDelay,
			"session_delay", sessionDelay,
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
			switch testSuiteConfig.Settings.VariablePolicy {
			case model.MergeTestPriority:
				testConfig.Variables = MergeVariables(testConfig.Variables, testSuiteConfig.Variables)
			case model.MergeSuitePriority:
				testConfig.Variables = MergeVariables(testSuiteConfig.Variables, testConfig.Variables)
			case model.TestOnly:
				break
			case model.SuiteOnly, "":
				fallthrough
			default:
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
			testResults := runTests(ctx, testConfig, agents, providers, maxIterations, toolTimeout, testDelay, sessionDelay, testFile, testSuiteConfig.Name)
			results = append(results, testResults...)
		}
		criteria = testSuiteConfig.TestCriteria
	}

	// AI Summary (optional LLM-powered executive summary)
	var aiSummaryResult *agent.AISummaryResult
	aiSummaryConfig := getAISummaryConfig(*testPath, *suitePath)
	if aiSummaryConfig != nil && aiSummaryConfig.Enabled {
		logger.Logger.Info("Generating AI summary")

		// Create a context for AI summary
		analysisBaseCtx := context.Background()

		// Resolve judge LLM for AI summary
		var judgeLLM llms.Model
		judgeProvider := aiSummaryConfig.JudgeProvider
		if judgeProvider == "" {
			logger.Logger.Error("AI summary enabled but judge_provider not specified")
		} else if judgeProvider == "$self" {
			// "$self" means use the same provider as the first agent that ran
			// Extract from first test result
			if len(results) > 0 {
				firstProvider := string(results[0].Execution.ProviderType)
				logger.Logger.Debug("Using first agent's provider for AI summary", "provider", firstProvider)
				// Re-initialize just this provider for analysis
				staticCtx := CreateStaticTemplateContext(*testPath, nil)
				if *suitePath != "" {
					staticCtx = CreateStaticTemplateContext(*suitePath, nil)
				}
				// Load config to get provider settings
				var providerConfig []model.Provider
				if *testPath != "" {
					if tc, err := model.ParseTestConfig(*testPath); err == nil {
						providerConfig = tc.Providers
					}
				} else if *suitePath != "" {
					if sc, err := model.ParseSuiteConfig(*suitePath); err == nil {
						providerConfig = sc.Providers
					}
				}
				for _, p := range providerConfig {
					if p.Name == firstProvider {
						initProviders, err := InitProviders(analysisBaseCtx, []model.Provider{p}, staticCtx)
						if err == nil {
							judgeLLM = initProviders[p.Name]
						}
						break
					}
				}
			}
		} else {
			// Look up the specified provider by name and initialize it
			staticCtx := CreateStaticTemplateContext(*testPath, nil)
			if *suitePath != "" {
				staticCtx = CreateStaticTemplateContext(*suitePath, nil)
			}
			var providerConfig []model.Provider
			if *testPath != "" {
				if tc, err := model.ParseTestConfig(*testPath); err == nil {
					providerConfig = tc.Providers
				}
			} else if *suitePath != "" {
				if sc, err := model.ParseSuiteConfig(*suitePath); err == nil {
					providerConfig = sc.Providers
				}
			}
			for _, p := range providerConfig {
				if p.Name == judgeProvider {
					initProviders, err := InitProviders(analysisBaseCtx, []model.Provider{p}, staticCtx)
					if err == nil {
						judgeLLM = initProviders[p.Name]
						logger.Logger.Debug("Using separate provider for AI summary", "judge_provider", judgeProvider)
					} else {
						logger.Logger.Error("Failed to initialize judge provider", "error", err)
					}
					break
				}
			}
			if judgeLLM == nil {
				logger.Logger.Error("AI summary judge provider not found", "judge_provider", judgeProvider)
			}
		}

		if judgeLLM != nil {
			analysisCtx, cancel := context.WithTimeout(analysisBaseCtx, 90*time.Second)
			analysisResult := agent.GenerateAISummary(analysisCtx, judgeLLM, results)
			cancel()
			aiSummaryResult = &analysisResult
			if analysisResult.Success {
				logger.Logger.Info("AI summary completed successfully")
			} else {
				logger.Logger.Warn("AI summary failed", "error", analysisResult.Error)
			}
		}
	}

	// Generate and save reports
	logger.Logger.Info("Generating reports")

	// Determine report output directory
	// Default to test_results folder in the test file's directory
	var reportDir string
	if *reportFileName == "" {
		var testDir string
		if *testPath != "" {
			absPath, err := filepath.Abs(*testPath)
			if err == nil {
				testDir = filepath.Dir(absPath)
			}
		} else if *suitePath != "" {
			absPath, err := filepath.Abs(*suitePath)
			if err == nil {
				testDir = filepath.Dir(absPath)
			}
		}
		if testDir != "" {
			reportDir = filepath.Join(testDir, "test_results")
			// Create the directory if it doesn't exist
			if err := os.MkdirAll(reportDir, 0755); err != nil {
				logger.Logger.Error("Failed to create test_results directory", "error", err)
				os.Exit(1)
			}
			*reportFileName = filepath.Join(reportDir, "report")
		} else {
			*reportFileName = "report"
		}
	}

	for _, rt := range reportTypes {
		reportFileNameWithExt := *reportFileName + "." + rt
		// Determine source test file path for JSON metadata
		configFilePath := ""
		if *testPath != "" {
			configFilePath = *testPath
		} else if *suitePath != "" {
			configFilePath = *suitePath
		}
		if err := GenerateReports(results, rt, reportFileNameWithExt, aiSummaryResult, configFilePath); err != nil {
			logger.Logger.Error("Failed to generate reports", "error", err)
			os.Exit(1)
		}
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

func getRequiredServers(agents []model.Agent, allServers []model.Server) []model.Server {
	// Collect unique server names used by agents
	usedServerNames := make(map[string]bool)
	for _, agent := range agents {
		for _, server := range agent.Servers {
			usedServerNames[server.Name] = true
		}
	}

	// Filter servers to only those actually used
	requiredServers := make([]model.Server, 0)
	unusedCount := 0

	for _, server := range allServers {
		if usedServerNames[server.Name] {
			requiredServers = append(requiredServers, server)
		} else {
			logger.Logger.Warn("Server defined but not used by any agent, will not be initialized",
				"server_name", server.Name,
				"server_type", server.Type)
			unusedCount++
		}
	}

	logger.Logger.Debug("Filtered servers",
		"total_defined", len(allServers),
		"required", len(requiredServers),
		"unused", unusedCount)

	return requiredServers
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

func InitProviders(ctx context.Context, providerConfigs []model.Provider, templateCtx map[string]string) (map[string]llms.Model, error) {
	if len(providerConfigs) == 0 {
		return nil, fmt.Errorf("no providers to initialize")
	}

	logger.Logger.Info("Initializing providers", "count", len(providerConfigs))
	providers := make(map[string]llms.Model)

	for i, p := range providerConfigs {
		// Use provided template context (includes env vars, TEST_DIR, user variables)
		// replace templates in config
		p.Name = model.RenderTemplate(p.Name, templateCtx)
		p.Token = model.RenderTemplate(p.Token, templateCtx)
		p.Model = model.RenderTemplate(p.Model, templateCtx)
		p.BaseURL = model.RenderTemplate(p.BaseURL, templateCtx)
		p.Version = model.RenderTemplate(p.Version, templateCtx)
		p.ProjectID = model.RenderTemplate(p.ProjectID, templateCtx)
		p.Location = model.RenderTemplate(p.Location, templateCtx)
		p.CredentialsPath = model.RenderTemplate(p.CredentialsPath, templateCtx)
		p.AuthType = model.RenderTemplate(p.AuthType, templateCtx)
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
	// Token validation: required for all providers except Vertex and Azure with Entra ID auth
	isEntraIdAuth := p.Type == model.ProviderAzure && strings.ToLower(p.AuthType) == "entra_id"
	if p.Type != model.ProviderVertex && !isEntraIdAuth && p.Token == "" {
		return nil, fmt.Errorf("provider token is empty")
	}

	if p.Model == "" {
		return nil, fmt.Errorf("provider model is empty")
	}

	// Create custom HTTP client for Retry-After header capture if retry is enabled
	var retryAfterClient *RetryAfterHTTPClient
	if p.Retry.RetryOn429 {
		retryAfterClient = NewRetryAfterHTTPClient(nil)
		logger.Logger.Debug("Created Retry-After HTTP client for header capture", "provider", p.Name)
	}

	var llmModel llms.Model
	var err error

	switch p.Type {
	case model.ProviderGroq:
		opts := []openai.Option{
			openai.WithToken(p.Token),
			openai.WithModel(p.Model),
		}
		if retryAfterClient != nil {
			opts = append(opts, openai.WithHTTPClient(retryAfterClient))
		}
		if p.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(p.BaseURL))
			logger.Logger.Debug("Using custom base URL", "url", p.BaseURL)
		} else {
			opts = append(opts, openai.WithBaseURL("https://api.groq.com/openai/v1"))
		}
		llmModel, err = openai.New(opts...)
	case model.ProviderGoogle:
		googleOpts := []googleai.Option{
			googleai.WithAPIKey(p.Token),
			googleai.WithDefaultModel(p.Model),
		}
		if retryAfterClient != nil {
			googleOpts = append(googleOpts, googleai.WithHTTPClient(retryAfterClient.wrapped))
		}
		llmModel, err = googleai.New(ctx, googleOpts...)
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
		if retryAfterClient != nil {
			opts = append(opts, anthropic.WithHTTPClient(retryAfterClient))
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
		if retryAfterClient != nil {
			opts = append(opts, openai.WithHTTPClient(retryAfterClient))
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

		if p.BaseURL == "" {
			return nil, fmt.Errorf("Azure provider requires base URL")
		}

		opts := []openai.Option{
			openai.WithModel(p.Model),
			openai.WithAPIVersion(p.Version),
			openai.WithBaseURL(p.BaseURL),
		}
		if retryAfterClient != nil {
			opts = append(opts, openai.WithHTTPClient(retryAfterClient))
		}
		logger.Logger.Debug("Using Azure base URL", "url", p.BaseURL)

		// Handle authentication type: "entra_id" uses DefaultAzureCredential, otherwise use API key
		if strings.ToLower(p.AuthType) == "entra_id" {
			logger.Logger.Debug("Using Entra ID authentication for Azure provider")
			cred, err := azidentity.NewDefaultAzureCredential(nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create Azure credential: %w", err)
			}
			// Get token for Azure OpenAI scope
			token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
				Scopes: []string{"https://cognitiveservices.azure.com/.default"},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get Azure token: %w", err)
			}
			// Use APITypeAzureAD to send token as Bearer token in Authorization header
			opts = append(opts, openai.WithAPIType(openai.APITypeAzureAD))
			opts = append(opts, openai.WithToken(token.Token))
		} else {
			// Default to API key authentication (backward compatible)
			// Use APITypeAzure to send token as api-key header
			if p.Token == "" {
				return nil, fmt.Errorf("Azure provider requires token when using api_key authentication")
			}
			opts = append(opts, openai.WithAPIType(openai.APITypeAzure))
			opts = append(opts, openai.WithToken(p.Token))
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

	// Wrap with rate limiter and/or retry handler if configured
	if NeedsLLMWrapper(p.RateLimits, p.Retry) {
		logger.Logger.Info("Wrapping provider with rate limiter/retry handler",
			"name", p.Name,
			"tpm", p.RateLimits.TPM,
			"rpm", p.RateLimits.RPM,
			"retry_on_429", p.Retry.RetryOn429)
		rateLimitedLLM := NewRateLimitedLLM(llmModel, p.RateLimits, p.Retry, p.Model)

		// If we created a custom HTTP client for Retry-After header capture, link it
		if retryAfterClient != nil {
			rateLimitedLLM.SetRetryAfterProvider(retryAfterClient)
			logger.Logger.Debug("Retry-After HTTP header capture enabled for provider", "name", p.Name)
		}

		llmModel = rateLimitedLLM
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

func InitServers(ctx context.Context, serverConfigs []model.Server, templateCtx map[string]string) (map[string]*server.MCPServer, error) {
	if len(serverConfigs) == 0 {
		return nil, fmt.Errorf("no servers to initialize")
	}

	logger.Logger.Info("Initializing servers", "count", len(serverConfigs))
	servers := make(map[string]*server.MCPServer)

	for i, s := range serverConfigs {
		// Use provided template context (includes env vars, TEST_DIR, user variables)
		// replace templates in config
		s.Name = model.RenderTemplate(s.Name, templateCtx)
		s.Command = model.RenderTemplate(s.Command, templateCtx)
		s.URL = model.RenderTemplate(s.URL, templateCtx)
		s.ServerDelay = model.RenderTemplate(s.ServerDelay, templateCtx)
		s.ProcessDelay = model.RenderTemplate(s.ProcessDelay, templateCtx)
		if s.Headers != nil {
			for k := range s.Headers {
				s.Headers[k] = model.RenderTemplate(s.Headers[k], templateCtx)
			}
		}

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
	providers map[string]llms.Model,
	maxIterations int,
	toolTimeout time.Duration,
	testDelay time.Duration,
	sessionDelay time.Duration,
	sourceFile string, // Source test file (empty for single file runs)
	suiteName string, // Suite name (empty for single file runs)
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

	// Build a map of agent definitions for quick lookup
	agentDefMap := make(map[string]model.Agent)
	for _, a := range testConfig.Agents {
		agentDefMap[a.Name] = a
	}

	// Build a map of provider configurations for quick lookup
	providerDefMap := make(map[string]model.Provider)
	for _, p := range testConfig.Providers {
		providerDefMap[p.Name] = p
	}

	for _, agentConfig := range agents {
		ag, ok := agents[agentConfig.Name]
		if !ok {
			logger.Logger.Warn("Agent not found, skipping tests", "agent", agentConfig.Name)
			continue
		}

		// Find the original agent config from testConfig.Agents to get system_prompt
		var originalAgentConfig *model.Agent
		for i := range testConfig.Agents {
			if testConfig.Agents[i].Name == agentConfig.Name {
				originalAgentConfig = &testConfig.Agents[i]
				break
			}
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

			// Create static template context with TEST_DIR, env vars, and user variables
			templateCtx := CreateStaticTemplateContext(sourceFile, testConfig.Variables)
			// Add runtime variables for this session
			templateCtx["AGENT_NAME"] = agentConfig.Name
			templateCtx["SESSION_NAME"] = session.Name
			templateCtx["PROVIDER_NAME"] = ag.Provider

			// Initialize fresh message history for this session
			msgs := make([]llms.MessageContent, 0)

			// Add system prompt if configured for this agent
			if originalAgentConfig != nil && originalAgentConfig.SystemPrompt != "" {
				systemPrompt := model.RenderTemplate(originalAgentConfig.SystemPrompt, templateCtx)
				msgs = append(msgs, llms.MessageContent{
					Role: llms.ChatMessageTypeSystem,
					Parts: []llms.ContentPart{
						llms.TextContent{Text: systemPrompt},
					},
				})
				logger.Logger.Debug("System prompt added", "length", len(systemPrompt))
			}

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

				// Get agent definition for config
				agentDef := agentDefMap[agentConfig.Name]

				// Resolve judge LLM for clarification detection
				var judgeLLM llms.Model
				if agentDef.ClarificationDetection.Enabled {
					judgeProvider := agentDef.ClarificationDetection.JudgeProvider
					if judgeProvider == "" {
						logger.Logger.Error("Clarification detection enabled but judge_provider not specified",
							"agent", agentConfig.Name)
					} else if judgeProvider == "$self" {
						// Use the agent's own LLM as the judge
						judgeLLM = ag.LLMModel
						logger.Logger.Debug("Using agent's LLM as clarification judge", "agent", agentConfig.Name)
					} else {
						// Look up the specified provider
						if providerLLM, ok := providers[judgeProvider]; ok {
							judgeLLM = providerLLM
							logger.Logger.Debug("Using separate provider for clarification judge",
								"agent", agentConfig.Name,
								"judge_provider", judgeProvider)
						} else {
							logger.Logger.Error("Clarification judge provider not found",
								"agent", agentConfig.Name,
								"judge_provider", judgeProvider)
						}
					}
				}

				// Execute test
				startTime := time.Now()
				executionResult := ag.GenerateContentWithConfig(ctx, &msgs, agent.AgentConfig{
					MaxIterations:                 maxIterations,
					ToolTimeout:                   toolTimeout,
					AddNotFinalResponses:          true,
					Verbose:                       testConfig.Settings.Verbose,
					ClarificationDetectionEnabled: agentDef.ClarificationDetection.Enabled,
					ClarificationDetectionLevel:   agent.ClarificationLevel(agentDef.ClarificationDetection.Level),
					ClarificationJudgeLLM:         judgeLLM,
				}, testTools)
				executionResult.TestName = test.Name
				executionResult.SourceFile = sourceFile
				executionResult.SuiteName = suiteName
				executionResult.SessionName = session.Name

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

			// Delay between sessions if configured (allows external processes like Excel to clean up)
			if sessionDelay > 0 && sessionIdx < len(testConfig.Sessions)-1 {
				logger.Logger.Info("Waiting before next session", "delay", sessionDelay)
				time.Sleep(sessionDelay)
			}
		}
	}

	return results
}

// CreateStaticTemplateContext creates a template context with all "static" variables
// that are available before test execution begins. This includes:
// - Environment variables
// - TEST_DIR (directory containing the source file)
// - User-defined variables from the config
//
// This context is used during provider and server initialization,
// enabling templates like {{TEST_DIR}}/server.exe in server commands.
// Runtime variables (AGENT_NAME, SESSION_NAME, PROVIDER_NAME) are added later
// during test execution via CreateTemplateContext.
func CreateStaticTemplateContext(sourceFile string, variables map[string]string) map[string]string {
	templateCtx := model.GetAllEnv()

	// Add RUN_ID: unique identifier for this test run (UUID v4)
	// Useful for creating unique file names, directories, etc.
	templateCtx["RUN_ID"] = uuid.New().String()

	// Add TEMP_DIR: system temporary directory (cross-platform)
	// Windows: %TEMP% or %TMP%, Linux/macOS: /tmp or $TMPDIR
	templateCtx["TEMP_DIR"] = os.TempDir()

	// Add TEST_DIR: absolute path to the directory containing the source file
	// Enables relative path references in templates (e.g., {{TEST_DIR}}/data)
	if sourceFile != "" {
		absPath, err := filepath.Abs(sourceFile)
		if err == nil {
			templateCtx["TEST_DIR"] = filepath.Dir(absPath)
		}
	}

	if variables == nil {
		return templateCtx
	}
	// Pre-transform variables if they contain templates
	// This allows variables to reference other variables or TEST_DIR
	for k, v := range variables {
		templateCtx[k] = model.RenderTemplate(v, templateCtx)
	}
	return templateCtx
}

// CreateTemplateContext creates the full template context for test execution.
// It builds on static context and adds runtime variables.
// This function is kept for backward compatibility.
func CreateTemplateContext(variables map[string]string) map[string]string {
	return CreateStaticTemplateContext("", variables)
}

func GenerateReports(results []model.TestRun, reportType, outputPath string, aiSummary *agent.AISummaryResult, testFilePath string) error {
	if len(results) == 0 {
		return fmt.Errorf("no test results to generate report")
	}

	reporter := model.NewReportGenerator()
	reporter.TestFile = testFilePath

	// Generate console report
	fmt.Println("\n" + strings.Repeat("=", 80))
	reporter.GenerateConsoleReport(results)
	// Print summary
	PrintTestSummary(results)

	// Print AI summary if available
	if aiSummary != nil && aiSummary.Success {
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Println("AI SUMMARY (LLM-Generated)")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Println(aiSummary.Analysis)
		fmt.Println(strings.Repeat("=", 80))
	}

	reportContent := ""
	switch reportType {
	case "json":
		// Convert agent.AISummaryResult to model.AISummaryData (avoiding circular import)
		var analysisData *model.AISummaryData
		if aiSummary != nil {
			analysisData = &model.AISummaryData{
				Success:   aiSummary.Success,
				Analysis:  aiSummary.Analysis,
				Error:     aiSummary.Error,
				Retryable: aiSummary.Retryable,
				Guidance:  aiSummary.Guidance,
			}
		}
		reportContent = reporter.GenerateJSONReportWithAnalysis(results, analysisData)
	case "html":
		// Use the new template-based HTML generator
		gen, err := report.NewGenerator()
		if err != nil {
			return fmt.Errorf("failed to create report generator: %w", err)
		}
		// Pass AI summary to HTML generator
		htmlContent, err := gen.GenerateHTMLWithAnalysis(results, aiSummary)
		if err != nil {
			return fmt.Errorf("failed to generate HTML report: %w", err)
		}
		reportContent = htmlContent
	case "md":
		reportContent = reporter.GenerateMarkdownReport(results)
	default:
		return fmt.Errorf("Unknown report type")
	}

	if reportContent == "" {
		return fmt.Errorf("generated report is empty")
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write report to file
	err := os.WriteFile(outputPath, []byte(reportContent), logger.FilePermission)
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

func MergeVariables(primary map[string]string, secondary map[string]string) map[string]string {
	merged := make(map[string]string)
	if secondary != nil {
		for key, value := range secondary {
			merged[key] = value
		}
	}
	if primary != nil {
		for key, value := range primary {
			merged[key] = value
		}
	}
	return merged
}

// getAISummaryConfig retrieves the AISummary configuration from either
// a single test file or a suite configuration file.
func getAISummaryConfig(testPath, suitePath string) *model.AISummary {
	// Try suite config first
	if suitePath != "" {
		suiteConfig, err := model.ParseSuiteConfig(suitePath)
		if err == nil && suiteConfig.AISummary.Enabled {
			return &suiteConfig.AISummary
		}
	}

	// Fall back to test config
	if testPath != "" {
		testConfig, err := model.ParseTestConfig(testPath)
		if err == nil && testConfig.AISummary.Enabled {
			return &testConfig.AISummary
		}
	}

	return nil
}
