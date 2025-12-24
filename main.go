package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mykhaliev/agent-benchmark/engine"
	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/templates"
	"github.com/mykhaliev/agent-benchmark/version"
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
