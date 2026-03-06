package explorer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/generator"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/tmc/langchaingo/llms"
	"gopkg.in/yaml.v3"
)

// sessionsWrapper is a helper for marshalling sessions to YAML test files.
type sessionsWrapper struct {
	Sessions []model.Session `yaml:"sessions"`
}

// Run is the main entry point for exploratory testing mode (-e flag).
func Run(ctx context.Context, configPath, outputDir, reportFileName string, reportTypes []string) {
	cfg, err := ParseExplorerConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load explorer config: %v\n", err)
		os.Exit(1)
	}

	logger.Logger.Info("Explorer config loaded",
		"goal", cfg.Explorer.Goal,
		"max_iterations", cfg.Explorer.MaxIterations,
		"stop_on_pass_count", cfg.Explorer.StopOnPassCount,
		"max_retries", cfg.Explorer.MaxRetries,
		"agent", cfg.Explorer.Agent,
	)

	// Build static template context (env vars, TEST_DIR, user variables).
	templateCtx := engine.CreateStaticTemplateContext(configPath, cfg.Variables)

	// Initialise providers.
	providers, err := engine.InitProviders(ctx, cfg.Providers, templateCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialise providers: %v\n", err)
		os.Exit(1)
	}

	// Initialise MCP servers.
	var mcpServers map[string]*server.MCPServer
	if len(cfg.Servers) > 0 {
		mcpServers, err = engine.InitServers(ctx, cfg.Servers, templateCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to initialise servers: %v\n", err)
			os.Exit(1)
		}
		defer engine.CleanupServers(mcpServers)
	}

	// Initialise agents.
	agents, err := engine.InitAgents(ctx, cfg.Agents, mcpServers, providers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialise agents: %v\n", err)
		os.Exit(1)
	}

	// Find the referenced agent definition.
	agentDef := findAgentByName(cfg.Agents, cfg.Explorer.Agent)

	// Get the decision LLM from the configured agent's provider.
	explorerLLM := providers[agentDef.Provider]

	// Build server names list from the agent's configured servers.
	serverNames := make([]string, 0, len(agentDef.Servers))
	for _, s := range agentDef.Servers {
		serverNames = append(serverNames, s.Name)
	}

	// Fetch tool descriptions for use in the decision prompt.
	tools, err := fetchToolDescriptions(ctx, serverNames, mcpServers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to fetch tool descriptions: %v\n", err)
		os.Exit(1)
	}

	// Apply defaults.
	if outputDir == "" {
		outputDir = "./explorer_results"
	}
	if reportFileName == "" {
		reportFileName = "exploration_report"
	}

	// Run the exploration loop.
	timestamp := time.Now().Format("20060102_150405")
	registry := NewPromptRegistry()
	adapter := &ExplorationTestAdapter{}
	allResults, testDefs := runExplorationLoop(ctx, cfg, explorerLLM, agents, providers, tools, registry, adapter, cfg.Explorer.Agent, configPath)

	// Write output folder with YAML exports and reports.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory %q: %v\n", outputDir, err)
		os.Exit(1)
	}
	if err := writeExplorationOutput(outputDir, configPath, cfg, testDefs, allResults, reportFileName, reportTypes, timestamp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write exploration output: %v\n", err)
		os.Exit(1)
	}

	engine.PrintTestSummary(allResults)

	if engine.HasFailures(allResults) {
		logger.Logger.Warn("Exploration completed with failures")
		os.Exit(1)
	}
	logger.Logger.Info("Exploration completed successfully")
}

// writeExplorationOutput creates a timestamped subdirectory, writes the
// exploration tests as YAML, generates reports, and prints the summary tree.
func writeExplorationOutput(
	outputDir, configPath string,
	cfg *ExplorerConfig,
	testDefs []RuntimeTestDefinition,
	allResults []model.TestRun,
	reportFileName string,
	reportTypes []string,
	timestamp string,
) error {
	subdir := filepath.Join(outputDir, "explorer_"+timestamp)
	if err := os.MkdirAll(subdir, 0755); err != nil {
		return fmt.Errorf("failed to create output subdirectory %q: %w", subdir, err)
	}

	// Collect all tests into a single session.
	tests := make([]model.Test, 0, len(testDefs))
	for _, rtd := range testDefs {
		tests = append(tests, rtd.Test)
	}

	goalSlug := engine.Slugify(cfg.Explorer.Goal)
	sessionsFile := goalSlug + ".yaml"

	sessionName := fmt.Sprintf("Exploration Goal: %s", cfg.Explorer.Goal)
	sessionsBytes, err := yaml.Marshal(sessionsWrapper{
		Sessions: []model.Session{
			{Name: sessionName, Tests: tests},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal exploration sessions: %w", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, sessionsFile), sessionsBytes, 0644); err != nil {
		return fmt.Errorf("failed to write sessions file %q: %w", sessionsFile, err)
	}

	// Build and write suite.yaml.
	suiteName := fmt.Sprintf("Exploration %s", timestamp)
	suiteYAML, err := engine.BuildSuiteYAML(configPath, []string{sessionsFile}, suiteName, "explorer")
	if err != nil {
		return fmt.Errorf("failed to build suite YAML: %w", err)
	}
	suiteFile := filepath.Join(subdir, "suite.yaml")
	if err := os.WriteFile(suiteFile, []byte(suiteYAML), 0644); err != nil {
		return fmt.Errorf("failed to write suite.yaml: %w", err)
	}

	// Generate reports into the folder.
	for _, rt := range reportTypes {
		reportPath := filepath.Join(subdir, reportFileName+"."+rt)
		if err := engine.GenerateReports(allResults, rt, reportPath, nil, configPath); err != nil {
			logger.Logger.Error("Failed to generate report", "type", rt, "error", err)
		}
	}

	// Count passes, failures, and bug findings.
	passed, failed, bugCount := 0, 0, 0
	for _, r := range allResults {
		if r.Passed {
			passed++
		} else {
			failed++
		}
		if r.Execution != nil {
			bugCount += len(r.Execution.BugFindings)
		}
	}
	total := len(allResults)

	headerLine := fmt.Sprintf("Exploration run — %d tests, %d passed, %d failed, %d MCP bugs detected",
		total, passed, failed, bugCount)
	engine.PrintOutputTree(subdir, headerLine, []string{sessionsFile}, []int{len(testDefs)})

	if len(reportTypes) > 0 {
		fmt.Println("Reports:")
		for _, rt := range reportTypes {
			fmt.Printf("  %s\n", filepath.ToSlash(filepath.Join(subdir, reportFileName+"."+rt)))
		}
		fmt.Println()
	}

	engine.PrintRunCommand(suiteFile)
	return nil
}

// findAgentByName returns the agent definition with the given name.
// ParseExplorerConfig guarantees the name exists, so this should never panic.
func findAgentByName(agents []model.Agent, name string) model.Agent {
	for _, a := range agents {
		if a.Name == name {
			return a
		}
	}
	panic(fmt.Sprintf("agent %q not found — this should have been caught by ParseExplorerConfig", name))
}

// runExplorationLoop is the core exploration loop. It calls the explorer LLM
// on each iteration to decide the next test, executes it via engine.RunTests,
// and appends results to the accumulated slice.
func runExplorationLoop(
	ctx context.Context,
	cfg *ExplorerConfig,
	explorerLLM llms.Model,
	agents map[string]*agent.MCPAgent,
	providers map[string]llms.Model,
	tools []mcp.Tool,
	registry *PromptRegistry,
	adapter *ExplorationTestAdapter,
	agentName string,
	configPath string,
) ([]model.TestRun, []RuntimeTestDefinition) {
	var allResults []model.TestRun
	var allRTDs []RuntimeTestDefinition
	var history []IterationContext

	// Parse settings once.
	toolTimeout := engine.ParseTimeout(cfg.Settings.ToolTimeout)
	maxIterations := engine.GetMaxIterations(cfg.Settings.MaxIterations)
	suiteName := fmt.Sprintf("Exploration: %s", cfg.Explorer.Goal)

	// Build validation inputs once (outside the loop — tools don't change per iteration).
	toolsByAgent := map[string][]mcp.Tool{"_all": tools}
	agentNames := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}
	sessionVars := make(map[string]bool, len(cfg.Variables))
	for k := range cfg.Variables {
		sessionVars[k] = true
	}

	consecutivePasses := 0
	totalTokens := 0

	for iter := 1; iter <= cfg.Explorer.MaxIterations; iter++ {
		logger.Logger.Info("Exploration iteration", "iteration", iter, "total", cfg.Explorer.MaxIterations)

		// 1. Build decision prompt.
		msgs, promptText := BuildDecisionPrompt(cfg, tools, history, iter)

		// 2. Call explorer LLM with retries.
		var dec *ExplorerDecision
		for attempt := 1; attempt <= cfg.Explorer.MaxRetries; attempt++ {
			resp, err := explorerLLM.GenerateContent(ctx, msgs)
			if err != nil {
				logger.Logger.Warn("Explorer LLM call failed", "attempt", attempt, "error", err)
				continue
			}
			totalTokens += agent.GetTokenCount(resp)

			rawContent := ""
			for _, choice := range resp.Choices {
				if choice.Content != "" {
					rawContent = choice.Content
					break
				}
			}
			if rawContent == "" {
				logger.Logger.Warn("Explorer LLM returned empty response", "attempt", attempt)
				continue
			}

			d, err := ParseExplorerDecision(rawContent)
			if err != nil {
				logger.Logger.Warn("Failed to parse decision", "attempt", attempt, "error", err)
				continue
			}

			// Validate the TestIntent and attempt repair if needed.
			errs := generator.ValidateTestIntent(d.TestIntent, toolsByAgent, agentNames, sessionVars)
			if len(errs) > 0 {
				repaired, repairErr := generator.RepairTestIntent(ctx, explorerLLM, d.TestIntent, errs,
					toolsByAgent, agentNames, cfg.Explorer.MaxRetries, cfg.Explorer.MaxTokens, &totalTokens)
				if repairErr == nil {
					rerrs := generator.ValidateTestIntent(repaired, toolsByAgent, agentNames, sessionVars)
					if len(rerrs) == 0 {
						d.TestIntent = repaired
						errs = nil
					}
				}
			}
			if len(errs) > 0 {
				logger.Logger.Warn("Validation failed after repair", "attempt", attempt, "errors", errs)
				continue
			}

			dec = d
			break
		}

		if dec == nil {
			logger.Logger.Error("Failed to get valid decision after retries", "iteration", iter)
			// Add a synthetic failed run so the iteration is visible in the report.
			history = append(history, IterationContext{
				Iteration: iter,
				TestName:  fmt.Sprintf("[Iter %02d] decision-failed", iter),
				Prompt:    "(could not get decision from LLM)",
				Passed:    false,
				Summary:   "LLM decision failed after all retries",
			})
			continue
		}

		// 3. Register prompt in registry.
		promptID := registry.Register(iter, promptText, dec.Reasoning)

		// 4. Build RuntimeTestDefinition.
		rtd := adapter.Adapt(dec.TestIntent, dec.Reasoning, iter, promptID, cfg.Explorer.Goal, agentName)

		// 5. Convert to TestConfiguration.
		testConfig := rtd.ToTestConfig(cfg)

		// 6. Execute test via existing engine.
		testResults := engine.RunTests(
			ctx,
			&testConfig,
			agents,
			providers,
			maxIterations,
			toolTimeout,
			0, // no test delay
			0, // no session delay
			configPath,
			suiteName,
		)

		// 7. Prepend exploration metadata as a system message in each result.
		metadataMsg := buildMetadataMessage(iter, promptID, cfg.Explorer.Goal, promptText, dec.Reasoning)
		for i := range testResults {
			if testResults[i].Execution != nil {
				testResults[i].Execution.Messages = append(
					[]model.Message{metadataMsg},
					testResults[i].Execution.Messages...,
				)
			}
		}

		// 8. Detect MCP bugs in tool call responses and store on execution results.
		mcpAgent := agents[agentName]
		for i := range testResults {
			if testResults[i].Execution == nil {
				continue
			}
			for _, tc := range testResults[i].Execution.ToolCalls {
				bugType, msg, isBug, err := AnalyzeToolCallWithLLM(ctx, explorerLLM, tc, &totalTokens)
				if err != nil {
					logger.Logger.Warn("LLM bug analysis failed", "tool", tc.Name, "error", err)
					continue
				}
				if !isBug {
					continue
				}
				serverName := ""
				if mcpAgent != nil {
					serverName = mcpAgent.ToolToServer[tc.Name]
				}
				testResults[i].Execution.BugFindings = append(
					testResults[i].Execution.BugFindings,
					model.BugFinding{
						ToolName:    tc.Name,
						BugType:     string(bugType),
						Explanation: msg,
						ServerName:  serverName,
					},
				)
			}
		}

		// Accumulate results and RTDs.
		for _, tr := range testResults {
			if tr.Execution != nil {
				totalTokens += tr.Execution.TokensUsed
			}
		}
		allResults = append(allResults, testResults...)
		allRTDs = append(allRTDs, rtd)

		if cfg.Explorer.MaxTokens > 0 && totalTokens >= cfg.Explorer.MaxTokens {
			logger.Logger.Warn("Max token limit reached, stopping exploration",
				"max_tokens", cfg.Explorer.MaxTokens,
				"tokens_used", totalTokens,
				"completed_iterations", iter)
			break
		}

		// 9. Update history for next iteration.
		passed := len(testResults) > 0 && testResults[0].Passed
		summary := buildIterationSummary(testResults)
		history = append(history, IterationContext{
			Iteration: iter,
			TestName:  rtd.Name,
			Prompt:    dec.Prompt,
			Passed:    passed,
			Summary:   summary,
		})

		// 10. Check convergence.
		if passed {
			consecutivePasses++
		} else {
			consecutivePasses = 0
		}
		if cfg.Explorer.StopOnPassCount > 0 && consecutivePasses >= cfg.Explorer.StopOnPassCount {
			logger.Logger.Info("Stop condition reached",
				"consecutive_passes", consecutivePasses,
				"threshold", cfg.Explorer.StopOnPassCount)
			break
		}

		logger.Logger.Info("Iteration complete",
			"iteration", iter,
			"passed", passed,
			"consecutive_passes", consecutivePasses)

		// Small pause between iterations to avoid rate limiting.
		time.Sleep(500 * time.Millisecond)
	}

	return allResults, allRTDs
}

// buildMetadataMessage creates the system message prepended to each result's
// Messages slice to make exploration metadata visible in the report conversation view.
func buildMetadataMessage(iter int, promptID, goal, promptText, reasoning string) model.Message {
	var sb strings.Builder
	sb.WriteString("=== EXPLORATION METADATA ===\n")
	sb.WriteString(fmt.Sprintf("Mode:      exploration\n"))
	sb.WriteString(fmt.Sprintf("Goal:      %s\n", goal))
	sb.WriteString(fmt.Sprintf("Iteration: %d\n", iter))
	sb.WriteString(fmt.Sprintf("PromptID:  %s\n", promptID))
	if reasoning != "" {
		sb.WriteString(fmt.Sprintf("\nExplorer LLM Reasoning:\n%s\n", reasoning))
	}
	sb.WriteString(fmt.Sprintf("\nDecision Prompt Sent to Explorer LLM:\n%s\n", promptText))
	sb.WriteString("===========================")

	return model.Message{
		Role:      "system",
		Content:   sb.String(),
		Timestamp: time.Now(),
	}
}

// buildIterationSummary creates a short summary string from test results for
// inclusion in the history context passed to subsequent LLM decisions.
func buildIterationSummary(results []model.TestRun) string {
	if len(results) == 0 {
		return "no results"
	}
	r := results[0]
	if r.Execution == nil {
		return "execution error"
	}

	var parts []string
	if r.Execution.FinalOutput != "" {
		output := r.Execution.FinalOutput
		if len(output) > 200 {
			output = output[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("output: %q", output))
	}
	if len(r.Execution.ToolCalls) > 0 {
		toolNames := make([]string, 0, len(r.Execution.ToolCalls))
		for _, tc := range r.Execution.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		parts = append(parts, fmt.Sprintf("tools called: %s", strings.Join(toolNames, ", ")))
	}
	if len(r.Execution.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("errors: %s", strings.Join(r.Execution.Errors, "; ")))
	}
	if len(parts) == 0 {
		return "completed"
	}
	return strings.Join(parts, "; ")
}

// fetchToolDescriptions collects all tools from the named MCP servers.
func fetchToolDescriptions(
	ctx context.Context,
	serverNames []string,
	mcpServers map[string]*server.MCPServer,
) ([]mcp.Tool, error) {
	var tools []mcp.Tool
	seen := make(map[string]bool)

	for _, name := range serverNames {
		mcpSrv, ok := mcpServers[name]
		if !ok {
			continue
		}

		res, err := mcpSrv.Client.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to list tools for server %q: %w", name, err)
		}

		for _, t := range res.Tools {
			if !seen[t.Name] {
				seen[t.Name] = true
				tools = append(tools, t)
			}
		}
	}

	return tools, nil
}
