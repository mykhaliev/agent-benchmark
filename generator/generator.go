// Package generator implements the test generation mode (-g flag).
// It reads a generator config file, connects to MCP servers, fetches tool
// descriptions, and uses an LLM to produce a ready-to-run test YAML file.
package generator

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/server"
	"github.com/tmc/langchaingo/llms"
	"gopkg.in/yaml.v3"
)

const maxRetries = 3

// Run is the main entry point for test generation mode.
// It orchestrates config loading, server init, tool fetching, LLM generation,
// validation, and output (file or stdout for dry-run).
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

	// Select the generator LLM.
	generatorLLM, ok := providers[cfg.Generator.Provider]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: generator provider %q not found in providers\n", cfg.Generator.Provider)
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

	// Build the list of known agent names for validation.
	agentNames := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		agentNames = append(agentNames, a.Name)
	}

	// Generate sessions YAML with retry.
	sessionsYAML, err := generateWithRetry(ctx, generatorLLM, cfg, toolsByAgent, agentNames, seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: test generation failed after %d attempts: %v\n", maxRetries, err)
		os.Exit(1)
	}

	// Combine original config (minus generator section) with generated sessions.
	fullYAML, err := combineOutput(configPath, sessionsYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to combine output: %v\n", err)
		os.Exit(1)
	}

	if dryRun {
		fmt.Println(fullYAML)
		return
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory %q: %v\n", outputDir, err)
		os.Exit(1)
	}

	// Write to a timestamped file.
	timestamp := time.Now().Format("20060102_150405")
	outFile := filepath.Join(outputDir, fmt.Sprintf("generated_test_%s.yaml", timestamp))
	if err := os.WriteFile(outFile, []byte(fullYAML), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated test configuration: %s\n", outFile)
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

// generateWithRetry calls the LLM up to maxRetries times, feeding back
// validation errors on each failed attempt.
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

	var prevErrors []string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Logger.Info("Generating tests", "attempt", attempt, "max", maxRetries)

		msgs := BuildGenerationPrompt(cfg, toolsByAgent, seed, attempt, prevErrors)

		resp, err := llm.GenerateContent(ctx, msgs)
		if err != nil {
			logger.Logger.Warn("LLM generation error", "attempt", attempt, "error", err)
			prevErrors = []string{fmt.Sprintf("LLM call failed: %v", err)}
			continue
		}

		rawContent := ""
		for _, choice := range resp.Choices {
			if choice.Content != "" {
				rawContent = choice.Content
				break
			}
		}

		if rawContent == "" {
			prevErrors = []string{"LLM returned empty response"}
			continue
		}

		sessionsYAML := ExtractYAMLFromResponse(rawContent)

		// Validate the generated sessions.
		errs := ValidateSessions(sessionsYAML, agentNames)
		if len(errs) == 0 {
			logger.Logger.Info("Sessions generated and validated successfully", "attempt", attempt)
			return sessionsYAML, nil
		}

		logger.Logger.Warn("Generated sessions failed validation",
			"attempt", attempt,
			"errors", len(errs))
		for _, e := range errs {
			logger.Logger.Debug("Validation error", "error", e)
		}
		prevErrors = errs
	}

	return "", fmt.Errorf("all %d generation attempts failed; last errors: %v", maxRetries, prevErrors)
}

// combineOutput reads the original generator config file, removes the
// "generator:" section from it, and appends the generated sessions block.
func combineOutput(configPath, sessionsYAML string) (string, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	// Strip the generator: section by parsing and re-marshalling without it.
	var topLevel map[string]interface{}
	if err := yaml.Unmarshal(raw, &topLevel); err != nil {
		return "", fmt.Errorf("failed to parse config file: %w", err)
	}
	delete(topLevel, "generator")

	infraBytes, err := yaml.Marshal(topLevel)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal infrastructure config: %w", err)
	}

	// Ensure the sessions YAML starts with "sessions:".
	if !strings.HasPrefix(strings.TrimSpace(sessionsYAML), "sessions:") {
		sessionsYAML = "sessions:\n" + sessionsYAML
	}

	return strings.TrimSpace(string(infraBytes)) + "\n\n" + sessionsYAML + "\n", nil
}
