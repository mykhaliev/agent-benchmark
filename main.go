package main

import (
	"flag"
	"fmt"
	"os"

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
	outputPath := flag.String("o", "", "Path to the output HTML reportType file")
	logPath := flag.String("l", "", "Path to the log file (if not set, logs to stdout)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	showVersion := flag.Bool("v", false, "Show version and exit")
	reportType := flag.String("reportType", "html", "Report type")

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

	// Validate report type
	if err := engine.ValidateReportType(*reportType); err != nil {
		logger.Logger.Error("Invalid reportType type", "error", err)
		os.Exit(1)
	}

	logger.Logger.Info("Starting application",
		"app", AppName,
		"config", *testPath,
		"output", *outputPath,
		"logfile", *logPath,
		"verbose", *verbose)

	engine.Run(testPath, verbose, suitePath, outputPath, reportType)
}
