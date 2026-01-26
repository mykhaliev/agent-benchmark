package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/report"
	"github.com/mykhaliev/agent-benchmark/templates"
	"github.com/mykhaliev/agent-benchmark/version"
	"github.com/tmc/langchaingo/llms"
)

const (
	AppName = "agent-bench"
)

func main() {
	testPath := flag.String("f", "", "Path to the test configuration file (YAML/JSON)")
	suitePath := flag.String("s", "", "Path to the suite configuration file (YAML/JSON)")
	reportFileName := flag.String("o", "", "Report file name (without extension)")
	logPath := flag.String("l", "", "Path to the log file (if not set, logs to stdout)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	showVersion := flag.Bool("v", false, "Show version and exit")
	reportTypes := flag.String("reportType", "html", "Report type(s) (comma-separated): html, json, markdown, txt")
	generateFromJSON := flag.String("generate-report", "", "Generate report from existing JSON results file (use with -f to get AI summary config)")

	flag.Parse()

	fmt.Printf("Version: %s\nCommit: %s\nBuildDate: %s\n",
		version.Version, version.Commit, version.BuildDate)
	if *showVersion {
		return
	}

	// Initialize Logger
	logWriter, logFile, err := logger.SetupLogWriter(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to setup logging: %v\n", err)
		os.Exit(1)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	logger.SetupLogger(logWriter, *verbose)
	templates.NewTemplateEngine()

	// Handle report generation from JSON
	if *generateFromJSON != "" {
		outputPath := *reportFileName
		if outputPath == "" {
			// Default: same name as input but with .html extension
			base := strings.TrimSuffix(*generateFromJSON, filepath.Ext(*generateFromJSON))
			outputPath = base + ".html"
		} else if !strings.HasSuffix(outputPath, ".html") {
			outputPath = outputPath + ".html"
		}

		fmt.Printf("Generating HTML report from: %s\n", *generateFromJSON)

		// Load JSON to get the test_file path
		reportData, err := report.LoadFullReportFromJSON(*generateFromJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load JSON: %v\n", err)
			os.Exit(1)
		}

		var judgeLLM llms.Model

		// Use test_file from JSON to get AI summary configuration
		if reportData.TestFile != "" {
			testConfig, err := model.ParseTestConfig(reportData.TestFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to parse test config %s: %v\n", reportData.TestFile, err)
			} else if testConfig.AISummary.Enabled {
				judgeProvider := testConfig.AISummary.JudgeProvider
				if judgeProvider == "" {
					fmt.Fprintf(os.Stderr, "Warning: AI summary enabled but no judge_provider specified\n")
				} else {
					ctx := context.Background()
					staticCtx := engine.CreateStaticTemplateContext(reportData.TestFile, nil)

					// Find the provider config
					var targetProvider *model.Provider
					if judgeProvider == "$self" && len(testConfig.Providers) > 0 {
						targetProvider = &testConfig.Providers[0]
					} else {
						for i := range testConfig.Providers {
							if testConfig.Providers[i].Name == judgeProvider {
								targetProvider = &testConfig.Providers[i]
								break
							}
						}
					}

					if targetProvider != nil {
						providers, err := engine.InitProviders(ctx, []model.Provider{*targetProvider}, staticCtx)
						if err != nil {
							fmt.Fprintf(os.Stderr, "Warning: Failed to initialize AI summary provider: %v\n", err)
						} else {
							for _, llm := range providers {
								judgeLLM = llm
								break
							}
						}
					}
				}
			}
		}

		// Generate HTML with AI summary (if judgeLLM is available)
		ctx := context.Background()
		if err := report.GenerateReportFromJSONWithSummary(ctx, *generateFromJSON, outputPath, judgeLLM); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to generate report: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Report generated: %s\n", outputPath)
		return
	}

	// Validate input
	if *testPath == "" && *suitePath == "" {
		fmt.Fprintf(os.Stderr, "Error: -f <test-file> or -s <suite-file> is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Parse and validate report types
	reportTypesArray := parseReportTypes(*reportTypes)
	if len(reportTypesArray) == 0 {
		logger.Logger.Error("No valid report types specified")
		os.Exit(1)
	}

	for _, rt := range reportTypesArray {
		if err := engine.ValidateReportType(rt); err != nil {
			logger.Logger.Error("Invalid reportType", "type", rt, "error", err)
			os.Exit(1)
		}
	}

	logger.Logger.Info("Starting application",
		"app", AppName,
		"config", *testPath,
		"suite", *suitePath,
		"output", *reportFileName,
		"reportTypes", strings.Join(reportTypesArray, ", "),
		"logfile", *logPath,
		"verbose", *verbose)

	engine.Run(testPath, verbose, suitePath, reportFileName, reportTypesArray)
}

func parseReportTypes(reportTypes string) []string {
	parts := strings.Split(reportTypes, ",")
	seen := make(map[string]bool)
	result := make([]string, 0, len(parts))

	for _, rt := range parts {
		trimmed := strings.TrimSpace(rt)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}

	return result
}
