// Package generator implements the test generation mode (-g flag).
// It reads a generator config file, connects to MCP servers, fetches tool
// descriptions, and uses an LLM to produce a ready-to-run test suite directory.
package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/agent"
	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/tmc/langchaingo/llms"
	"gopkg.in/yaml.v3"
)

// Run is the main entry point for test generation mode.
// It orchestrates config loading, server init, tool fetching, LLM generation,
// validation, and output (suite directory or stdout for dry-run).
func Run(ctx context.Context, configPath, outputDir string, dryRun bool, seed int64) {
	cfg, err := ParseGeneratorConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load generator config: %v\n", err)
		os.Exit(1)
	}

	logger.Logger.Info("Generator config loaded",
		"providers", len(cfg.Providers),
		"servers", len(cfg.Servers),
		"agents", len(cfg.Agents),
		"test_count", cfg.Generator.TestCount,
		"complexity", cfg.Generator.Complexity,
	)

	// Build static template context (env vars, TEST_DIR, user variables).
	templateCtx := engine.CreateStaticTemplateContext(configPath, cfg.Variables)

	// Initialise LLM providers.
	providers, err := engine.InitProviders(ctx, cfg.Providers, templateCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialise providers: %v\n", err)
		os.Exit(1)
	}

	// Resolve the generator agent → provider → LLM.
	var generatorAgentProvider string
	for _, a := range cfg.Agents {
		if a.Name == cfg.Generator.Agent {
			generatorAgentProvider = a.Provider
			break
		}
	}
	if generatorAgentProvider == "" {
		fmt.Fprintf(os.Stderr, "Error: generator agent %q not found in agents\n", cfg.Generator.Agent)
		os.Exit(1)
	}
	generatorLLM, ok := providers[generatorAgentProvider]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: provider %q (from generator agent %q) not found in providers\n", generatorAgentProvider, cfg.Generator.Agent)
		os.Exit(1)
	}

	// Initialise MCP servers (only those referenced by agents).
	var mcpServers map[string]*server.MCPServer
	if len(cfg.Servers) > 0 {
		mcpServers, err = engine.InitServers(ctx, cfg.Servers, templateCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to initialise servers: %v\n", err)
			os.Exit(1)
		}
		defer engine.CleanupServers(mcpServers)
	}

	// Fetch tool descriptions from MCP servers for each agent.
	toolsByAgent, err := fetchToolDescriptions(ctx, cfg.Agents, mcpServers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to fetch tool descriptions: %v\n", err)
		os.Exit(1)
	}

	// Apply tool allowlist from generator settings.
	if len(cfg.Generator.Tools) > 0 {
		toolsByAgent = filterToolsByName(toolsByAgent, cfg.Generator.Tools)
		for agentName, tools := range toolsByAgent {
			logger.Logger.Info("Effective tools after filtering",
				"agent", agentName,
				"tools", len(tools))
		}
	}

	// Build the list of known agent names for validation.
	agentNames := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}

	// Generate sessions YAML with retry.
	sessionsYAML, err := generateWithRetry(ctx, generatorLLM, cfg, toolsByAgent, agentNames, seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: test generation failed after %d attempts: %v\n", cfg.Generator.MaxRetries, err)
		os.Exit(1)
	}

	timestamp := time.Now().Format("20060102_150405")

	if dryRun {
		printDryRun(sessionsYAML, configPath, timestamp)
		return
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory %q: %v\n", outputDir, err)
		os.Exit(1)
	}

	if err := writeMultiFileOutput(outputDir, configPath, sessionsYAML, timestamp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write output: %v\n", err)
		os.Exit(1)
	}
}

// writeMultiFileOutput creates a timestamped subdirectory under outputDir,
// writes one YAML file per session plus a suite.yaml that ties them together,
// and prints the file tree and run command to stdout.
func writeMultiFileOutput(outputDir, configPath, sessionsYAML, timestamp string) error {
	var wrapper sessionsWrapper
	if err := yaml.Unmarshal([]byte(sessionsYAML), &wrapper); err != nil {
		return fmt.Errorf("failed to parse sessions YAML: %w", err)
	}

	subdir := filepath.Join(outputDir, fmt.Sprintf("generated_%s", timestamp))
	if err := os.MkdirAll(subdir, 0755); err != nil {
		return fmt.Errorf("failed to create output subdirectory %q: %w", subdir, err)
	}

	// Write one file per session.
	filenames := make([]string, 0, len(wrapper.Sessions))
	testFiles := make([]string, 0, len(wrapper.Sessions))
	testCounts := make([]int, 0, len(wrapper.Sessions))
	for _, session := range wrapper.Sessions {
		filename := engine.Slugify(session.Name) + ".yaml"
		filenames = append(filenames, filename)
		testFiles = append(testFiles, filename)
		testCounts = append(testCounts, len(session.Tests))

		sessionBytes, err := yaml.Marshal(sessionsWrapper{Variables: wrapper.Variables, Sessions: []model.Session{session}})
		if err != nil {
			return fmt.Errorf("failed to marshal session %q: %w", session.Name, err)
		}
		if err := os.WriteFile(filepath.Join(subdir, filename), sessionBytes, 0644); err != nil {
			return fmt.Errorf("failed to write session file %q: %w", filename, err)
		}
	}

	// Build and write suite.yaml.
	suiteName := fmt.Sprintf("Generated %s", timestamp)
	suiteYAML, err := engine.BuildSuiteYAML(configPath, testFiles, suiteName, "generator")
	if err != nil {
		return fmt.Errorf("failed to build suite YAML: %w", err)
	}
	suiteFile := filepath.Join(subdir, "suite.yaml")
	if err := os.WriteFile(suiteFile, []byte(suiteYAML), 0644); err != nil {
		return fmt.Errorf("failed to write suite file: %w", err)
	}

	totalTests := 0
	for _, c := range testCounts {
		totalTests += c
	}
	headerLine := fmt.Sprintf("Generated test suite — %d sessions, %d tests total", len(wrapper.Sessions), totalTests)
	engine.PrintOutputTree(subdir, headerLine, filenames, testCounts)
	engine.PrintRunCommand(suiteFile)
	return nil
}

// printDryRun prints the would-be file contents to stdout with clear separators.
func printDryRun(sessionsYAML, configPath, timestamp string) {
	var wrapper sessionsWrapper
	if err := yaml.Unmarshal([]byte(sessionsYAML), &wrapper); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to parse sessions for dry-run: %v\n", err)
		return
	}

	testFiles := make([]string, 0, len(wrapper.Sessions))
	filenames := make([]string, 0, len(wrapper.Sessions))
	for _, session := range wrapper.Sessions {
		filename := engine.Slugify(session.Name) + ".yaml"
		filenames = append(filenames, filename)
		testFiles = append(testFiles, filename)
	}

	suiteName := fmt.Sprintf("Generated %s", timestamp)
	suiteYAML, err := engine.BuildSuiteYAML(configPath, testFiles, suiteName, "generator")
	if err != nil {
		suiteYAML = fmt.Sprintf("# (error building suite YAML: %v)\n", err)
	}

	fmt.Println("=== suite.yaml ===")
	fmt.Print(suiteYAML)

	for i, session := range wrapper.Sessions {
		fmt.Printf("\n=== %s ===\n", filenames[i])
		sessionBytes, err := yaml.Marshal(sessionsWrapper{Variables: wrapper.Variables, Sessions: []model.Session{session}})
		if err != nil {
			fmt.Printf("# (error marshalling session: %v)\n", err)
		} else {
			fmt.Print(string(sessionBytes))
		}
	}
}

// fetchToolDescriptions iterates over agents and their configured servers,
// calling ListTools on each connected MCP server.
func fetchToolDescriptions(
	ctx context.Context,
	agents []model.Agent,
	mcpServers map[string]*server.MCPServer,
) (map[string][]mcp.Tool, error) {
	toolsByAgent := make(map[string][]mcp.Tool)

	for _, ag := range agents {
		var agentTools []mcp.Tool
		seen := make(map[string]bool)

		for _, agSrv := range ag.Servers {
			srv, ok := mcpServers[agSrv.Name]
			if !ok {
				logger.Logger.Warn("Server not found for agent", "agent", ag.Name, "server", agSrv.Name)
				continue
			}

			res, err := srv.Client.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				return nil, fmt.Errorf("failed to list tools for server %q (agent %q): %w",
					agSrv.Name, ag.Name, err)
			}

			for _, t := range res.Tools {
				if !seen[t.Name] {
					seen[t.Name] = true
					agentTools = append(agentTools, t)
				}
			}
		}

		toolsByAgent[ag.Name] = agentTools
		logger.Logger.Info("Fetched tool descriptions",
			"agent", ag.Name,
			"tools", len(agentTools))
	}

	return toolsByAgent, nil
}

// generatePlanChunk runs a single LLM call to produce a chunk of the JSON test plan.
// It returns the fence-stripped JSON string, the token count, and any error.
func generatePlanChunk(
	ctx context.Context,
	llm llms.Model,
	cfg *GeneratorConfig,
	toolsByAgent map[string][]mcp.Tool,
	chunkSize int,
	alreadyPlanned []planTest,
) (string, int, error) {
	conversation := BuildPlanChunkPrompt(cfg, toolsByAgent, chunkSize, alreadyPlanned)

	resp, err := llm.GenerateContent(ctx, conversation, llms.WithMaxTokens(cfg.Generator.PlanChunkMaxTokens))
	if err != nil {
		return "", 0, fmt.Errorf("plan chunk LLM call failed: %w", err)
	}

	totalTokens := agent.GetTokenCount(resp)

	if len(resp.Choices) == 0 {
		return "", totalTokens, fmt.Errorf("plan chunk LLM returned no choices")
	}

	raw := resp.Choices[0].Content
	if isTokenLimitStopReason(resp.Choices[0].StopReason) {
		logger.Logger.Debug("Plan chunk response was token-limit truncated — JSON may be incomplete; retrying",
			"chunk_size", chunkSize, "stop_reason", resp.Choices[0].StopReason)
	}
	planJSON := ExtractJSONFromResponse(raw)

	return planJSON, totalTokens, nil
}

// mergePlanChunks merges multiple planWrapper values into one,
// combining tests from chunks that share the same session name.
func mergePlanChunks(chunks []planWrapper) planWrapper {
	var merged planWrapper
	sessionIndex := make(map[string]int) // session name → index in merged.Sessions
	for _, chunk := range chunks {
		for _, sess := range chunk.Sessions {
			if idx, ok := sessionIndex[sess.Name]; ok {
				merged.Sessions[idx].Tests = append(merged.Sessions[idx].Tests, sess.Tests...)
			} else {
				sessionIndex[sess.Name] = len(merged.Sessions)
				merged.Sessions = append(merged.Sessions, sess)
			}
		}
	}
	return merged
}

// isTokenLimitStopReason reports whether the LLM stop reason indicates it was
// cut off by a token limit. Handles provider-specific values:
// OpenAI/Groq "length", Anthropic/Google "max_tokens", Vertex AI "FinishReasonMaxTokens".
func isTokenLimitStopReason(reason string) bool {
	lower := strings.ToLower(reason)
	return lower == "length" ||
		strings.Contains(lower, "max_tokens") ||
		strings.Contains(lower, "maxtokens") // Vertex AI: "FinishReasonMaxTokens"
}

// continueJSONResponse resumes a truncated LLM response through multi-turn conversation.
// It uses StopReason to detect genuine token-limit truncation rather than JSON parse errors.
// partial must be the fence-stripped JSON (jsonStr), not the raw response.
// Returns the extracted JSON string on success.
func continueJSONResponse(
	ctx context.Context,
	llm llms.Model,
	conversation []llms.MessageContent,
	partial string,
	maxContinuations int,
	maxTokens int,
	totalTokens *int,
) (string, error) {
	accumulated := partial
	for i := 0; i < maxContinuations; i++ {
		if maxTokens > 0 && *totalTokens >= maxTokens {
			return "", fmt.Errorf("token limit exceeded: %d tokens used, limit is %d", *totalTokens, maxTokens)
		}

		tail := accumulated
		if len(tail) > 60 {
			tail = tail[len(tail)-60:]
		}
		contPrompt := fmt.Sprintf(
			"Your JSON response was cut off by the token limit. Resume it by outputting ONLY"+
				" what comes after: %q\nContinue the JSON from exactly where it was cut off."+
				" Output only the missing portion — do not repeat what was already shown and"+
				" do not add any prefix or code fences.",
			tail,
		)

		cont := append(append([]llms.MessageContent(nil), conversation...),
			llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{llms.TextContent{Text: accumulated}},
			},
			llms.MessageContent{
				Role:  llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{llms.TextContent{Text: contPrompt}},
			},
		)
		resp, err := llm.GenerateContent(ctx, cont)
		if err != nil {
			return "", fmt.Errorf("continuation call %d failed: %w", i+1, err)
		}
		*totalTokens += agent.GetTokenCount(resp)
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("continuation call %d returned no choices", i+1)
		}

		accumulated += ExtractJSONFromResponse(resp.Choices[0].Content)

		if !isTokenLimitStopReason(resp.Choices[0].StopReason) {
			// LLM finished naturally — accumulated content should be complete.
			jsonStr := ExtractJSONFromResponse(accumulated)
			var probe interface{}
			if json.Unmarshal([]byte(jsonStr), &probe) == nil {
				return jsonStr, nil
			}
			logger.Logger.Debug("JSON output after conversation had finished", "json", jsonStr)
			return "", fmt.Errorf("continuation call %d: LLM finished but JSON is invalid", i+1)
		}
		// Still hitting token limit — loop for another continuation.
	}
	return "", fmt.Errorf("JSON still incomplete after %d continuations", maxContinuations)
}

// generateTestIntent generates a single TestIntent for one plan test scenario.
// It retries up to cfg.Generator.MaxRetries times; each failed attempt includes
// one repair sub-attempt. JSON parse failures are tracked across attempts:
// after 2 consecutive parse failures a parse-repair prompt is used to request a
// simpler response. Validation errors from the most recent failure are injected
// into every subsequent generation prompt so the LLM can correct them.
func generateTestIntent(
	ctx context.Context,
	llm llms.Model,
	pt planTest,
	sessionName, agentName string,
	agentNames []string,
	toolsByAgent map[string][]mcp.Tool,
	sessionVars map[string]bool,
	cfg *GeneratorConfig,
	totalTokens *int,
) (TestIntent, error) {
	maxRetries := cfg.Generator.MaxRetries
	maxTokens := cfg.Generator.MaxTokens

	var lastParseErrCount int       // consecutive JSON parse failures
	var lastValidationErrs []string // errors from the most recent failed validation

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if maxTokens > 0 && *totalTokens >= maxTokens {
			return TestIntent{}, fmt.Errorf(
				"token limit exceeded: %d tokens used, limit is %d",
				*totalTokens, maxTokens,
			)
		}

		conversation := BuildTestIntentPrompt(pt, sessionName, agentName, agentNames, toolsByAgent, sessionVars, cfg, lastValidationErrs)
		resp, err := llm.GenerateContent(ctx, conversation)
		if err != nil {
			if ctx.Err() != nil {
				return TestIntent{}, ctx.Err()
			}
			logger.Logger.Warn("Intent generation failed", "test", pt.Name, "attempt", attempt, "error", err)
			continue
		}

		*totalTokens += agent.GetTokenCount(resp)

		if len(resp.Choices) == 0 {
			logger.Logger.Warn("Intent generation returned no choices", "test", pt.Name, "attempt", attempt)
			continue
		}

		raw := resp.Choices[0].Content
		jsonStr := ExtractJSONFromResponse(raw)

		// If the LLM was cut off by a token limit, attempt continuation conversation.
		if isTokenLimitStopReason(resp.Choices[0].StopReason) {
			if continued, contErr := continueJSONResponse(ctx, llm, conversation,
				jsonStr, cfg.Generator.MaxRetries, maxTokens, totalTokens); contErr == nil {
				jsonStr = continued
			} else {
				logger.Logger.Debug("Intent continuation failed", "test", pt.Name,
					"attempt", attempt, "stop_reason", resp.Choices[0].StopReason, "error", contErr)
			}
		}

		var intent TestIntent
		if err := json.Unmarshal([]byte(jsonStr), &intent); err != nil {
			logger.Logger.Debug("Intent JSON parse failed", "test", pt.Name, "attempt", attempt, "error", err)
			logger.Logger.Debug("Intent JSON parse response", "response", jsonStr)
			lastParseErrCount++
			if lastParseErrCount >= 2 {
				// Two or more consecutive parse failures: ask the LLM for a simpler response.
				repaired, repairErr := repairTestIntentParse(ctx, llm, pt, sessionName, toolsByAgent, agentNames, cfg.Generator.MaxRetries, maxTokens, totalTokens)
				if repairErr == nil {
					repairErrs := ValidateTestIntent(repaired, toolsByAgent, agentNames, sessionVars)
					if len(repairErrs) == 0 {
						return repaired, nil
					}
					// Parse repair produced valid JSON but validation failed; carry errors forward.
					lastParseErrCount = 0
					lastValidationErrs = repairErrs
					logger.Logger.Debug("Parse repair validation failed", "test", pt.Name, "attempt", attempt, "errors", len(repairErrs))
				} else {
					logger.Logger.Debug("Parse repair failed", "test", pt.Name, "attempt", attempt, "error", repairErr)
					lastParseErrCount = 0 // allow two new main attempts before triggering repair again
				}
			}
			continue
		}

		lastParseErrCount = 0

		// Session name from the plan is authoritative.
		intent.SessionName = sessionName

		errs := ValidateTestIntent(intent, toolsByAgent, agentNames, sessionVars)
		if len(errs) == 0 {
			return intent, nil
		}

		logger.Logger.Debug("Intent validation failed", "test", pt.Name, "attempt", attempt, "errors", len(errs))
		logger.Logger.Debug("Error", "info", errs)

		// Carry validation errors into the next generation attempt.
		lastValidationErrs = errs

		// One repair attempt per generation attempt.
		repaired, repairErr := RepairTestIntent(ctx, llm, intent, errs, toolsByAgent, agentNames, cfg.Generator.MaxRetries, maxTokens, totalTokens)
		if repairErr == nil {
			repairErrs := ValidateTestIntent(repaired, toolsByAgent, agentNames, sessionVars)
			if len(repairErrs) == 0 {
				return repaired, nil
			}
			lastValidationErrs = repairErrs
			logger.Logger.Debug("Intent repair validation failed", "test", pt.Name, "attempt", attempt, "errors", len(repairErrs))
		} else {
			logger.Logger.Debug("Intent repair failed", "test", pt.Name, "attempt", attempt, "error", repairErr)
		}
	}

	return TestIntent{}, fmt.Errorf("failed to generate intent for %q after %d attempts", pt.Name, maxRetries)
}

// repairTestIntentParse asks the LLM to produce a simpler, shorter TestIntent after
// previous attempts generated truncated or unparseable JSON responses.
func repairTestIntentParse(
	ctx context.Context,
	llm llms.Model,
	pt planTest,
	sessionName string,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
	maxContinuations int,
	maxTokens int,
	totalTokens *int,
) (TestIntent, error) {
	if maxTokens > 0 && *totalTokens >= maxTokens {
		return TestIntent{}, fmt.Errorf(
			"token limit exceeded: %d tokens used, limit is %d",
			*totalTokens, maxTokens,
		)
	}

	conversation := BuildTestIntentParseRepairPrompt(pt, toolsByAgent, agentNames)
	resp, err := llm.GenerateContent(ctx, conversation)
	if err != nil {
		return TestIntent{}, fmt.Errorf("parse repair LLM call failed: %w", err)
	}

	*totalTokens += agent.GetTokenCount(resp)

	if len(resp.Choices) == 0 {
		return TestIntent{}, fmt.Errorf("parse repair LLM returned no choices")
	}

	raw := resp.Choices[0].Content
	jsonStr := ExtractJSONFromResponse(raw)

	// If the LLM was cut off by a token limit, attempt continuation conversation.
	if isTokenLimitStopReason(resp.Choices[0].StopReason) {
		if continued, contErr := continueJSONResponse(ctx, llm, conversation,
			jsonStr, maxContinuations, maxTokens, totalTokens); contErr == nil {
			jsonStr = continued
		}
	}

	var repaired TestIntent
	if err := json.Unmarshal([]byte(jsonStr), &repaired); err != nil {
		return TestIntent{}, fmt.Errorf("parse repair JSON parse failed: %w", err)
	}

	// Session name from the plan is authoritative.
	repaired.SessionName = sessionName

	return repaired, nil
}

// RepairTestIntent asks the LLM to fix a TestIntent that failed validation.
// It makes a single LLM call and returns the repaired intent (or an error).
func RepairTestIntent(
	ctx context.Context,
	llm llms.Model,
	intent TestIntent,
	errs []string,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
	maxContinuations int,
	maxTokens int,
	totalTokens *int,
) (TestIntent, error) {
	if maxTokens > 0 && *totalTokens >= maxTokens {
		return TestIntent{}, fmt.Errorf(
			"token limit exceeded: %d tokens used, limit is %d",
			*totalTokens, maxTokens,
		)
	}

	conversation := BuildTestIntentRepairPrompt(intent, errs, toolsByAgent, agentNames)
	resp, err := llm.GenerateContent(ctx, conversation)
	if err != nil {
		return TestIntent{}, fmt.Errorf("repair LLM call failed: %w", err)
	}

	*totalTokens += agent.GetTokenCount(resp)

	if len(resp.Choices) == 0 {
		return TestIntent{}, fmt.Errorf("repair LLM returned no choices")
	}

	raw := resp.Choices[0].Content
	jsonStr := ExtractJSONFromResponse(raw)

	// If the LLM was cut off by a token limit, attempt continuation conversation.
	if isTokenLimitStopReason(resp.Choices[0].StopReason) {
		if continued, contErr := continueJSONResponse(ctx, llm, conversation,
			jsonStr, maxContinuations, maxTokens, totalTokens); contErr == nil {
			jsonStr = continued
		}
	}

	var repaired TestIntent
	if err := json.Unmarshal([]byte(jsonStr), &repaired); err != nil {
		return TestIntent{}, fmt.Errorf("repair JSON parse failed: %w", err)
	}

	// Preserve authoritative session name from original.
	repaired.SessionName = intent.SessionName

	return repaired, nil
}

// buildInitialSessionVars copies variable keys from cfg.Variables into a bool map
// so that ValidateTestIntent can check forward references.
func buildInitialSessionVars(vars map[string]string) map[string]bool {
	sessionVars := make(map[string]bool, len(vars))
	for k := range vars {
		sessionVars[k] = true
	}
	return sessionVars
}

// generateWithRetry orchestrates three sequential phases:
//
//  1. Plan phase: a focused LLM call produces a compact JSON test plan,
//     retried up to MaxRetries times. Hard failure if all attempts are exhausted.
//
//  2. Intent phase: for each test scenario in the plan, generateTestIntent is
//     called. Each call retries up to MaxRetries times with one repair sub-attempt
//     per generation attempt. Hard failure if any test's retries are exhausted.
//
//  3. Build phase: BuildSessions constructs model.Session values deterministically
//     and yaml.Marshal serializes them. ValidateSessions is run as a safety-net
//     (warns on errors but does not fail — the builder is deterministic).
func generateWithRetry(
	ctx context.Context,
	llm llms.Model,
	cfg *GeneratorConfig,
	toolsByAgent map[string][]mcp.Tool,
	agentNames []string,
	seed int64,
) (string, error) {

	if seed != 0 {
		rand.New(rand.NewSource(seed)) //nolint:gosec
	}

	startTime := time.Now()
	totalTokens := 0

	// ── Phase 1: Chunked plan generation ─────────────────────────────────────
	planChunkSize := cfg.Generator.PlanChunkSize
	if planChunkSize <= 0 {
		planChunkSize = 5
	}

	logger.Logger.Info("Generating test plan (phase 1)",
		"total_tests", cfg.Generator.TestCount,
		"chunk_size", planChunkSize,
	)

	var allPlanTests []planTest // flat list of all planned tests (for dedup context)
	var planChunks []planWrapper
	chunkNum := 0
	totalChunks := (cfg.Generator.TestCount + planChunkSize - 1) / planChunkSize

	for len(allPlanTests) < cfg.Generator.TestCount {
		remaining := cfg.Generator.TestCount - len(allPlanTests)
		currentChunkSize := remaining
		if currentChunkSize > planChunkSize {
			currentChunkSize = planChunkSize
		}
		chunkNum++

		logger.Logger.Info("Generating plan chunk",
			"chunk", chunkNum, "of", totalChunks,
			"tests_in_chunk", currentChunkSize,
			"planned_so_far", len(allPlanTests),
		)

		var chunkPlan string
		for attempt := 1; attempt <= cfg.Generator.MaxRetries; attempt++ {
			if cfg.Generator.MaxTokens > 0 && totalTokens >= cfg.Generator.MaxTokens {
				return "", fmt.Errorf("token limit exceeded: %d tokens used, limit is %d",
					totalTokens, cfg.Generator.MaxTokens)
			}

			planJSON, n, err := generatePlanChunk(ctx, llm, cfg, toolsByAgent, currentChunkSize, allPlanTests)
			totalTokens += n
			if err != nil {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				logger.Logger.Debug("Plan chunk generation failed", "chunk", chunkNum, "attempt", attempt, "error", err)
				continue
			}

			planErrs := ValidatePlan(planJSON, toolsByAgent)
			if len(planErrs) == 0 {
				chunkPlan = planJSON
				break
			}
			logger.Logger.Debug("Plan chunk validation failed", "chunk", chunkNum, "attempt", attempt, "errors", len(planErrs))
			logger.Logger.Debug("Error", "info", planErrs)
		}

		if chunkPlan == "" {
			return "", fmt.Errorf("plan chunk %d failed after %d attempts", chunkNum, cfg.Generator.MaxRetries)
		}

		var chunkData planWrapper
		if err := json.Unmarshal([]byte(chunkPlan), &chunkData); err != nil {
			return "", fmt.Errorf("failed to parse plan chunk %d JSON: %w", chunkNum, err)
		}
		planChunks = append(planChunks, chunkData)

		// Accumulate planned tests for dedup context in subsequent chunks.
		for _, sess := range chunkData.Sessions {
			allPlanTests = append(allPlanTests, sess.Tests...)
		}

		// Safety: stop if LLM produced zero tests to avoid infinite loop.
		testsInChunk := 0
		for _, sess := range chunkData.Sessions {
			testsInChunk += len(sess.Tests)
		}
		if testsInChunk == 0 {
			logger.Logger.Warn("Plan chunk produced no tests, stopping early", "chunk", chunkNum)
			break
		}
	}

	if len(allPlanTests) == 0 {
		return "", fmt.Errorf("plan phase produced no tests after %d chunks", chunkNum)
	}

	// Serialize merged plan back to JSON for Phase 2.
	mergedPlan := mergePlanChunks(planChunks)

	logger.Logger.Info("Plan phase complete",
		"tests_planned", len(allPlanTests),
		"sessions", len(mergedPlan.Sessions),
		"tokens_used", totalTokens,
	)
	planBytes, err := json.Marshal(mergedPlan)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged plan: %w", err)
	}
	plan := string(planBytes)

	// ── Phase 2: Generate one TestIntent per test ─────────────────────────────
	var planData planWrapper
	if err := json.Unmarshal([]byte(plan), &planData); err != nil {
		return "", fmt.Errorf("failed to parse validated plan JSON: %w", err)
	}

	// Count total intents for progress reporting.
	totalIntents := 0
	for _, s := range planData.Sessions {
		totalIntents += len(s.Tests)
	}
	intentNum := 0

	var allIntents []TestIntent
	for _, planSess := range planData.Sessions {
		sessionVars := buildInitialSessionVars(cfg.Variables)

		logger.Logger.Info("Generating session intents",
			"session", planSess.Name,
			"tests", len(planSess.Tests))

		for _, pt := range planSess.Tests {
			intentNum++
			logger.Logger.Info("Generating test intent",
				"number", intentNum,
				"total", totalIntents,
				"session", planSess.Name,
				"test", pt.Name)

			intent, err := generateTestIntent(
				ctx, llm, pt, planSess.Name,
				cfg.Generator.Agent, agentNames, toolsByAgent, sessionVars,
				cfg, &totalTokens,
			)
			if err != nil {
				return "", err
			}

			allIntents = append(allIntents, intent)

			// Add extractor outputs to session scope for subsequent tests.
			for _, ex := range intent.Extractors {
				if ex.VariableName != "" {
					sessionVars[ex.VariableName] = true
				}
			}
		}
	}

	logger.Logger.Info("Building test files (phase 3)")

	// ── Phase 3: Deterministic build + serialize ──────────────────────────────
	sessions := BuildSessions(allIntents, cfg.Variables)
	wrapper := sessionsWrapper{Variables: cfg.Variables, Sessions: sessions}

	yamlBytes, err := yaml.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sessions: %w", err)
	}

	candidate := strings.TrimSpace(string(yamlBytes))

	// Safety-net: should always pass since the builder is deterministic.
	if finalErrs := ValidateSessions(candidate, agentNames, toolsByAgent); len(finalErrs) > 0 {
		logger.Logger.Warn("Final ValidateSessions errors after builder", "errors", finalErrs)
	}

	logger.Logger.Info("Generation complete",
		"sessions", len(sessions),
		"tests", len(allIntents),
		"tokens_used", totalTokens,
		"elapsed", time.Since(startTime).Round(time.Second),
	)

	return candidate, nil
}

// filterToolsByName filters toolsByAgent to only include tools whose names
// appear in the allowlist. An empty allowlist returns toolsByAgent unchanged.
// Tools in the allowlist that are not found in any agent are logged as warnings.
func filterToolsByName(toolsByAgent map[string][]mcp.Tool, allowlist []string) map[string][]mcp.Tool {
	if len(allowlist) == 0 {
		return toolsByAgent
	}

	allowed := make(map[string]bool, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = true
	}

	filtered := make(map[string][]mcp.Tool, len(toolsByAgent))
	for agentName, tools := range toolsByAgent {
		var kept []mcp.Tool
		for _, t := range tools {
			if allowed[t.Name] {
				kept = append(kept, t)
				delete(allowed, t.Name) // mark as found
			}
		}
		filtered[agentName] = kept
	}

	// Warn about any allowlisted names that were never found.
	for name := range allowed {
		logger.Logger.Warn("Tool in generator.tools not found in any server", "tool", name)
	}

	return filtered
}
