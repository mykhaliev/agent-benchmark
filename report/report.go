// Package report provides HTML report generation using Go templates
package report

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mykhaliev/agent-benchmark/logger"
	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/version"
)

//go:embed templates/*.html templates/*.css
var templateFS embed.FS

// ReportData represents the data structure passed to the HTML template
type ReportData struct {
	CSS         template.CSS
	Version     string
	GeneratedAt string
	Summary     SummaryData
	AgentStats  []AgentStatsView
	Comparisons []ComparisonView
	TestGroups  []TestGroupView
	// Enhanced report data
	Matrix     MatrixView
	AgentNames []string
	TestNames  []string
	// Suite-level data
	IsSuiteRun bool
	SuiteName  string
	FileGroups []FileGroupView
	// Adaptive rendering flags
	ShowSuiteInfo       bool // Show suite name/info section
	ShowFileSections    bool // Show file grouping (multiple source files)
	ShowSessionSections bool // Show session grouping (multiple sessions)
	ShowAgentComparison bool // Show agent comparison matrix (multiple agents)
	ShowTestOverview    bool // Show test overview table (multiple tests)
	SessionGroups       []SessionGroupView
	TestOverview        []TestOverviewRow
}

// SummaryData holds overall test summary
type SummaryData struct {
	Total           int
	Passed          int
	Failed          int
	AgentCount      int
	PassRate        float64 // Percentage 0-100
	AvgTokensPassed int     // Average tokens used by passing tests
	AvgDuration     float64
}

// TestOverviewRow represents a single test in the overview table
type TestOverviewRow struct {
	TestName   string
	Passed     bool
	DurationMs float64
	TokensUsed int
	ToolCalls  int
	Assertions int
	ErrorCount int
}

// MatrixView represents the test × agent comparison matrix
type MatrixView struct {
	TestNames  []string
	AgentNames []string
	Cells      map[string]map[string]MatrixCell // [testName][agentName]
}

// MatrixCell represents a single cell in the comparison matrix
type MatrixCell struct {
	Passed     bool
	HasResult  bool
	DurationMs float64
	Tokens     int
}

// AgentStatsView is a view model for agent statistics
type AgentStatsView struct {
	Rank             int    // 1, 2, 3... or 0 for disqualified
	RankDisplay      string // "🥇", "🥈", "🥉", "4", "DQ"
	AgentName        string
	Provider         string
	TotalTests       int
	PassedTests      int
	FailedTests      int
	ErrorCount       int // Tests that had errors (subset of failed)
	SuccessRate      float64
	SuccessRateClass string
	AvgDuration      float64
	TotalTokens      int
	AvgTokens        int
	Efficiency       int    // Tokens per passed test (lower = better)
	EfficiencyStr    string // Display string ("125 tok/✓" or "—")
	IsDisqualified   bool   // 0% success rate
	RowClass         string // CSS class for row styling
}

// ComparisonView is a view model for test comparisons
type ComparisonView struct {
	TestName          string
	TotalRuns         int
	PassedRuns        int
	FailedRuns        int
	SuccessRate       float64
	SuccessRateClass  string
	ServerResultsList []ServerResultView
}

// ServerResultView is a view model for individual server results
type ServerResultView struct {
	ServerName string
	Provider   string
	Passed     bool
	DurationMs float64
	Errors     []string
}

// TestGroupView groups test runs by test name
type TestGroupView struct {
	TestName   string
	SourceFile string // Source file (for suite runs)
	Runs       []TestRunView
}

// FileGroupView groups test results by source file (for suite runs)
type FileGroupView struct {
	FileName         string
	TotalTests       int
	PassedTests      int
	FailedTests      int
	SuccessRate      float64
	SuccessRateClass string
	TestGroups       []TestGroupView
	SessionGroups    []SessionGroupView // Sessions within this file
}

// SessionGroupView groups test results by session
type SessionGroupView struct {
	SessionName      string
	SourceFile       string // Parent source file (for suite runs)
	TotalTests       int
	PassedTests      int
	FailedTests      int
	SuccessRate      float64
	SuccessRateClass string
	TestGroups       []TestGroupView
	SequenceDiagram  string // Mermaid diagram showing all tests in session
}

// TestRunView is a view model for individual test runs
type TestRunView struct {
	AgentName       string
	Provider        string
	Passed          bool
	DurationSeconds float64
	Assertions      []AssertionView
	Errors          []string
	// Enhanced fields for detailed view
	Prompt          string // The user prompt that was sent to the agent
	TokensUsed      int
	FinalOutput     string
	Messages        []MessageView
	ToolCalls          []ToolCallView           // Tool call timeline
	SequenceDiagram    string                   // Mermaid syntax
	RateLimitStats     *RateLimitStatsView      // Rate limiting and 429 stats
	ClarificationStats *ClarificationStatsView  // Clarification detection stats
}

// RateLimitStatsView is a view model for rate limit statistics
type RateLimitStatsView struct {
	ThrottleCount     int     // Number of times request was throttled
	ThrottleWaitSec   float64 // Total time spent waiting due to throttling (seconds)
	RateLimitHits     int     // Number of 429 errors received
	RetryCount        int     // Number of retry attempts made
	RetryWaitSec      float64 // Total time spent waiting for retries (seconds)
	RetrySuccessCount int     // Number of successful retries
}

// ClarificationStatsView is a view model for clarification detection display
type ClarificationStatsView struct {
	Count      int      // Number of clarification requests detected
	Iterations []int    // Which iterations had clarification requests
	Examples   []string // Sample text from clarification requests (truncated)
}


// MessageView is a view model for conversation messages
type MessageView struct {
	Role      string
	Content   string
	Timestamp string
}

// ToolCallView is a view model for tool invocations
type ToolCallView struct {
	Name       string
	Parameters string // JSON string
	Result     string // JSON string
	Timestamp  string
	DurationMs int64 // Execution time in milliseconds
}

// AssertionView is a view model for assertions
type AssertionView struct {
	Type    string
	Passed  bool
	Message string
	Details string // JSON string of assertion details
}

// Generator handles HTML report generation
type Generator struct {
	tmpl *template.Template
}

// NewGenerator creates a new report generator with embedded templates
func NewGenerator() (*Generator, error) {
	funcMap := template.FuncMap{
		"formatNumber": formatNumber,
		"getMatrixCell": func(cells map[string]map[string]MatrixCell, testName, agentName string) MatrixCell {
			if row, ok := cells[testName]; ok {
				if cell, ok := row[agentName]; ok {
					return cell
				}
			}
			return MatrixCell{HasResult: false}
		},
		"truncate": func(s string, max int) string {
			if len(s) <= max {
				return s
			}
			return s[:max-3] + "..."
		},
		"safeJSON": func(s string) template.HTML {
			// Return JSON as safe HTML to avoid double-escaping
			return template.HTML(s)
		},
		"prettyJSON": func(s string) template.HTML {
			// Pretty print JSON for display
			var obj interface{}
			if err := json.Unmarshal([]byte(s), &obj); err != nil {
				return template.HTML(s)
			}
			pretty, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return template.HTML(s)
			}
			return template.HTML(pretty)
		},
		"hasDetails": func(s string) bool {
			return s != "" && s != "{}" && s != "null"
		},
		"iterate": func(count int) []int {
			// Create a slice of integers from 0 to count-1 for range iteration
			result := make([]int, count)
			for i := 0; i < count; i++ {
				result[i] = i
			}
			return result
		},
		"add": func(a, b int) int {
			return a + b
		},
		"divFloat": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}

	tmpl, err := template.New("report.html").Funcs(funcMap).ParseFS(templateFS, "templates/report.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &Generator{tmpl: tmpl}, nil
}

// GenerateHTML generates an HTML report from test results
func (g *Generator) GenerateHTML(results []model.TestRun) (string, error) {
	data := buildReportData(results)

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateHTMLToFile generates an HTML report and writes it to a file
func (g *Generator) GenerateHTMLToFile(results []model.TestRun, outputPath string) error {
	html, err := g.GenerateHTML(results)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	return nil
}

// LoadResultsFromJSON loads test results from a JSON file
func LoadResultsFromJSON(jsonPath string) ([]model.TestRun, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	// The JSON structure has detailed_results containing the TestRun array
	var reportData struct {
		DetailedResults []model.TestRun `json:"detailed_results"`
	}

	if err := json.Unmarshal(data, &reportData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if len(reportData.DetailedResults) == 0 {
		return nil, fmt.Errorf("no test results found in JSON file")
	}

	return reportData.DetailedResults, nil
}

// GenerateReportFromJSON generates an HTML report from an existing JSON file
func GenerateReportFromJSON(jsonPath, outputPath string) error {
	results, err := LoadResultsFromJSON(jsonPath)
	if err != nil {
		return err
	}

	gen, err := NewGenerator()
	if err != nil {
		return err
	}

	if err := gen.GenerateHTMLToFile(results, outputPath); err != nil {
		return err
	}

	logger.Logger.Info("Report generated from JSON", "input", jsonPath, "output", outputPath)
	return nil
}

// buildReportData transforms TestRun results into the template view model
func buildReportData(results []model.TestRun) ReportData {
	passed := 0
	failed := 0
	totalTokensPassed := 0
	totalDuration := 0.0

	// Collect unique values for adaptive rendering
	sourceFiles := make(map[string]bool)
	sessions := make(map[string]bool)
	agents := make(map[string]bool)
	suiteName := ""

	for _, r := range results {
		if r.Execution.SourceFile != "" {
			sourceFiles[r.Execution.SourceFile] = true
		}
		if r.Execution.SessionName != "" {
			sessions[r.Execution.SessionName] = true
		}
		if r.Execution.SuiteName != "" {
			suiteName = r.Execution.SuiteName
		}
		agents[r.Execution.AgentName] = true
	}

	// Compute adaptive flags
	isSuiteRun := len(sourceFiles) > 0
	showSuiteInfo := suiteName != ""
	showFileSections := len(sourceFiles) > 1
	showSessionSections := len(sessions) > 1
	showAgentComparison := len(agents) > 1
	showTestOverview := len(results) > 1 && !showAgentComparison // Show for single-agent with multiple tests

	for _, r := range results {
		if r.Passed {
			passed++
			totalTokensPassed += r.Execution.TokensUsed
		} else {
			failed++
		}
		totalDuration += r.Execution.EndTime.Sub(r.Execution.StartTime).Seconds()
	}

	avgTokensPassed := 0
	if passed > 0 {
		avgTokensPassed = totalTokensPassed / passed
	}

	avgDuration := 0.0
	if len(results) > 0 {
		avgDuration = totalDuration / float64(len(results))
	}

	// Load CSS from embedded file
	cssBytes, err := templateFS.ReadFile("templates/report.css")
	if err != nil {
		cssBytes = []byte("/* CSS load error */")
	}

	matrix := buildMatrix(results)
	fileGroups := buildFileGroups(results)
	sessionGroups := buildSessionGroups(results)
	testOverview := buildTestOverview(results)

	totalTests := passed + failed
	passRate := 0.0
	if totalTests > 0 {
		passRate = float64(passed) / float64(totalTests) * 100
	}

	return ReportData{
		CSS:         template.CSS(cssBytes),
		Version:     version.Version,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Summary: SummaryData{
			Total:           totalTests,
			Passed:          passed,
			Failed:          failed,
			AgentCount:      len(agents),
			PassRate:        passRate,
			AvgTokensPassed: avgTokensPassed,
			AvgDuration:     avgDuration,
		},
		AgentStats:          buildAgentStats(results),
		Comparisons:         buildComparisons(results),
		TestGroups:          buildTestGroups(results),
		Matrix:              matrix,
		AgentNames:          matrix.AgentNames,
		TestNames:           matrix.TestNames,
		IsSuiteRun:          isSuiteRun,
		SuiteName:           suiteName,
		FileGroups:          fileGroups,
		ShowSuiteInfo:       showSuiteInfo,
		ShowFileSections:    showFileSections,
		ShowSessionSections: showSessionSections,
		ShowAgentComparison: showAgentComparison,
		ShowTestOverview:    showTestOverview,
		SessionGroups:       sessionGroups,
		TestOverview:        testOverview,
	}
}

func buildTestOverview(results []model.TestRun) []TestOverviewRow {
	rows := make([]TestOverviewRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, TestOverviewRow{
			TestName:   r.Execution.TestName,
			Passed:     r.Passed,
			DurationMs: float64(r.Execution.LatencyMs),
			TokensUsed: r.Execution.TokensUsed,
			ToolCalls:  len(r.Execution.ToolCalls),
			Assertions: len(r.Assertions),
			ErrorCount: len(r.Execution.Errors),
		})
	}
	return rows
}

func buildAgentStats(results []model.TestRun) []AgentStatsView {
	statsMap := make(map[string]*AgentStatsView)

	for _, result := range results {
		agentName := result.Execution.AgentName

		if _, exists := statsMap[agentName]; !exists {
			statsMap[agentName] = &AgentStatsView{
				AgentName: agentName,
				Provider:  string(result.Execution.ProviderType),
			}
		}

		stats := statsMap[agentName]
		stats.TotalTests++

		if result.Passed {
			stats.PassedTests++
		} else {
			stats.FailedTests++
			// Count errors separately
			if len(result.Execution.Errors) > 0 {
				stats.ErrorCount++
			}
		}

		stats.TotalTokens += result.Execution.TokensUsed
		duration := result.Execution.EndTime.Sub(result.Execution.StartTime).Seconds()
		stats.AvgDuration += duration
	}

	// Calculate averages and convert to slice
	statsList := make([]AgentStatsView, 0, len(statsMap))
	for _, stats := range statsMap {
		if stats.TotalTests > 0 {
			stats.AvgTokens = stats.TotalTokens / stats.TotalTests
			stats.AvgDuration = stats.AvgDuration / float64(stats.TotalTests)
			stats.SuccessRate = float64(stats.PassedTests) / float64(stats.TotalTests) * 100
			stats.SuccessRateClass = getSuccessRateClass(stats.SuccessRate)

			// Calculate efficiency (tokens per passed test)
			if stats.PassedTests > 0 {
				stats.Efficiency = stats.TotalTokens / stats.PassedTests
				stats.EfficiencyStr = fmt.Sprintf("%d tok/✓", stats.Efficiency)
			} else {
				stats.Efficiency = 0
				stats.EfficiencyStr = "—"
			}

			// Mark disqualified agents (0% success rate)
			if stats.SuccessRate == 0 {
				stats.IsDisqualified = true
				stats.RowClass = "leaderboard-row-dq"
			} else if stats.SuccessRate >= 100 {
				stats.RowClass = "leaderboard-row-perfect"
			} else if stats.SuccessRate >= 50 {
				stats.RowClass = "leaderboard-row-good"
			} else {
				stats.RowClass = "leaderboard-row-poor"
			}
		}
		statsList = append(statsList, *stats)
	}

	// Sort: qualified agents first by success rate, then efficiency, then speed
	// Disqualified agents go to the bottom
	sort.Slice(statsList, func(i, j int) bool {
		// Disqualified agents always rank last
		if statsList[i].IsDisqualified != statsList[j].IsDisqualified {
			return !statsList[i].IsDisqualified
		}
		// Higher success rate = better rank
		if statsList[i].SuccessRate != statsList[j].SuccessRate {
			return statsList[i].SuccessRate > statsList[j].SuccessRate
		}
		// Better efficiency (lower tokens/pass) = better rank
		if statsList[i].Efficiency != statsList[j].Efficiency && statsList[i].Efficiency > 0 && statsList[j].Efficiency > 0 {
			return statsList[i].Efficiency < statsList[j].Efficiency
		}
		// Faster = better rank
		if statsList[i].AvgDuration != statsList[j].AvgDuration {
			return statsList[i].AvgDuration < statsList[j].AvgDuration
		}
		// Alphabetical as final tiebreaker
		return statsList[i].AgentName < statsList[j].AgentName
	})

	// Assign ranks after sorting
	medals := []string{"🥇", "🥈", "🥉"}
	for i := range statsList {
		if statsList[i].IsDisqualified {
			statsList[i].Rank = 0
			statsList[i].RankDisplay = "DQ"
		} else {
			statsList[i].Rank = i + 1
			if i < 3 {
				statsList[i].RankDisplay = medals[i]
			} else {
				statsList[i].RankDisplay = fmt.Sprintf("%d", i+1)
			}
		}
	}

	return statsList
}

func buildComparisons(results []model.TestRun) []ComparisonView {
	compMap := make(map[string]*ComparisonView)

	for _, run := range results {
		testName := run.Execution.TestName

		if _, exists := compMap[testName]; !exists {
			compMap[testName] = &ComparisonView{
				TestName:          testName,
				ServerResultsList: []ServerResultView{},
			}
		}

		comp := compMap[testName]
		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

		serverResult := ServerResultView{
			ServerName: run.Execution.AgentName,
			Provider:   string(run.Execution.ProviderType),
			Passed:     run.Passed,
			DurationMs: float64(duration.Milliseconds()),
			Errors:     run.Execution.Errors,
		}

		comp.ServerResultsList = append(comp.ServerResultsList, serverResult)
		comp.TotalRuns++
		if run.Passed {
			comp.PassedRuns++
		} else {
			comp.FailedRuns++
		}
	}

	// Calculate success rates and convert to slice
	compList := make([]ComparisonView, 0, len(compMap))
	for _, comp := range compMap {
		if comp.TotalRuns > 0 {
			comp.SuccessRate = float64(comp.PassedRuns) / float64(comp.TotalRuns) * 100
			comp.SuccessRateClass = getSuccessRateClass(comp.SuccessRate)
		}
		compList = append(compList, *comp)
	}

	// Sort by test name for consistent output
	sort.Slice(compList, func(i, j int) bool {
		return compList[i].TestName < compList[j].TestName
	})

	return compList
}

func buildTestGroups(results []model.TestRun) []TestGroupView {
	groupMap := make(map[string]*TestGroupView)

	for _, run := range results {
		testName := run.Execution.TestName

		if _, exists := groupMap[testName]; !exists {
			groupMap[testName] = &TestGroupView{
				TestName: testName,
				Runs:     []TestRunView{},
			}
		}

		group := groupMap[testName]
		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

		assertions := make([]AssertionView, len(run.Assertions))
		for i, a := range run.Assertions {
			detailsJSON := ""
			if a.Details != nil {
				if b, err := json.Marshal(a.Details); err == nil {
					detailsJSON = string(b)
				}
			}
			assertions[i] = AssertionView{
				Type:    a.Type,
				Passed:  a.Passed,
				Message: a.Message,
				Details: detailsJSON,
			}
		}

		// Build message views
		messages := make([]MessageView, len(run.Execution.Messages))
		for i, m := range run.Execution.Messages {
			messages[i] = MessageView{
				Role:      m.Role,
				Content:   m.Content,
				Timestamp: m.Timestamp.Format("15:04:05.000"),
			}
		}

		// Build tool call views with relative timestamps
		toolCalls := make([]ToolCallView, len(run.Execution.ToolCalls))
		testStartTime := run.Execution.StartTime
		for i, tc := range run.Execution.ToolCalls {
			paramsJSON := ""
			if tc.Parameters != nil {
				if b, err := json.Marshal(tc.Parameters); err == nil {
					paramsJSON = string(b)
				}
			}
			resultJSON := ""
			if tc.Result.Content != nil {
				if b, err := json.Marshal(tc.Result.Content); err == nil {
					resultJSON = string(b)
				}
			}
			// Calculate relative time from test start
			relativeTime := ""
			if !tc.Timestamp.IsZero() && !testStartTime.IsZero() {
				elapsed := tc.Timestamp.Sub(testStartTime)
				if elapsed >= 0 {
					relativeTime = fmt.Sprintf("+%.2fs", elapsed.Seconds())
				} else {
					relativeTime = tc.Timestamp.Format("15:04:05")
				}
			} else {
				relativeTime = "-"
			}
			toolCalls[i] = ToolCallView{
				Name:       tc.Name,
				Parameters: paramsJSON,
				Result:     resultJSON,
				Timestamp:  relativeTime,
				DurationMs: tc.DurationMs,
			}
		}

		// Generate sequence diagram
		sequenceDiagram := buildSequenceDiagram(run)

		// Extract user prompt from messages
		prompt := ""
		for _, msg := range run.Execution.Messages {
			if msg.Role == "user" {
				prompt = msg.Content
				break
			}
		}

		runView := TestRunView{
			AgentName:       run.Execution.AgentName,
			Provider:        string(run.Execution.ProviderType),
			Passed:          run.Passed,
			DurationSeconds: duration.Seconds(),
			Assertions:      assertions,
			Errors:          run.Execution.Errors,
			Prompt:          prompt,
			TokensUsed:      run.Execution.TokensUsed,
			FinalOutput:     run.Execution.FinalOutput,
			Messages:           messages,
			ToolCalls:          toolCalls,
			SequenceDiagram:    sequenceDiagram,
			RateLimitStats:     buildRateLimitStatsView(run.Execution.RateLimitStats),
			ClarificationStats: buildClarificationStatsView(run.Execution.ClarificationStats),
		}

		group.Runs = append(group.Runs, runView)
	}

	// Convert to slice and sort
	groupList := make([]TestGroupView, 0, len(groupMap))
	for _, group := range groupMap {
		groupList = append(groupList, *group)
	}

	sort.Slice(groupList, func(i, j int) bool {
		return groupList[i].TestName < groupList[j].TestName
	})

	return groupList
}

// buildFileGroups groups test results by source file (for suite runs)
func buildFileGroups(results []model.TestRun) []FileGroupView {
	fileMap := make(map[string]*FileGroupView)
	fileTestMap := make(map[string]map[string]*TestGroupView) // [fileName][testName]

	for _, run := range results {
		sourceFile := run.Execution.SourceFile
		if sourceFile == "" {
			sourceFile = "default" // Single file run
		}

		// Initialize file group if needed
		if _, exists := fileMap[sourceFile]; !exists {
			fileMap[sourceFile] = &FileGroupView{
				FileName:   sourceFile,
				TestGroups: []TestGroupView{},
			}
			fileTestMap[sourceFile] = make(map[string]*TestGroupView)
		}

		fileGroup := fileMap[sourceFile]
		testName := run.Execution.TestName

		// Initialize test group within file if needed
		if _, exists := fileTestMap[sourceFile][testName]; !exists {
			fileTestMap[sourceFile][testName] = &TestGroupView{
				TestName:   testName,
				SourceFile: sourceFile,
				Runs:       []TestRunView{},
			}
		}

		// Update file-level stats
		fileGroup.TotalTests++
		if run.Passed {
			fileGroup.PassedTests++
		} else {
			fileGroup.FailedTests++
		}

		// Build test run view (same logic as buildTestGroups)
		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

		assertions := make([]AssertionView, len(run.Assertions))
		for i, a := range run.Assertions {
			detailsJSON := ""
			if a.Details != nil {
				if b, err := json.Marshal(a.Details); err == nil {
					detailsJSON = string(b)
				}
			}
			assertions[i] = AssertionView{
				Type:    a.Type,
				Passed:  a.Passed,
				Message: a.Message,
				Details: detailsJSON,
			}
		}

		// Extract user prompt from messages
		prompt := ""
		for _, msg := range run.Execution.Messages {
			if msg.Role == "user" {
				prompt = msg.Content
				break
			}
		}

		runView := TestRunView{
			AgentName:       run.Execution.AgentName,
			Provider:        string(run.Execution.ProviderType),
			Passed:          run.Passed,
			DurationSeconds: duration.Seconds(),
			Assertions:      assertions,
			Errors:          run.Execution.Errors,
			Prompt:          prompt,
			TokensUsed:         run.Execution.TokensUsed,
			FinalOutput:        run.Execution.FinalOutput,
			RateLimitStats:     buildRateLimitStatsView(run.Execution.RateLimitStats),
			ClarificationStats: buildClarificationStatsView(run.Execution.ClarificationStats),
		}

		fileTestMap[sourceFile][testName].Runs = append(fileTestMap[sourceFile][testName].Runs, runView)
	}

	// Build the final structure
	fileList := make([]FileGroupView, 0, len(fileMap))
	for fileName, fileGroup := range fileMap {
		// Calculate success rate
		if fileGroup.TotalTests > 0 {
			fileGroup.SuccessRate = float64(fileGroup.PassedTests) / float64(fileGroup.TotalTests) * 100
			fileGroup.SuccessRateClass = getSuccessRateClass(fileGroup.SuccessRate)
		}

		// Add test groups to file group
		for _, testGroup := range fileTestMap[fileName] {
			fileGroup.TestGroups = append(fileGroup.TestGroups, *testGroup)
		}

		// Sort test groups by name
		sort.Slice(fileGroup.TestGroups, func(i, j int) bool {
			return fileGroup.TestGroups[i].TestName < fileGroup.TestGroups[j].TestName
		})

		fileList = append(fileList, *fileGroup)
	}

	// Sort file groups by name
	sort.Slice(fileList, func(i, j int) bool {
		return fileList[i].FileName < fileList[j].FileName
	})

	return fileList
}

// buildSessionGroups groups test results by session
func buildSessionGroups(results []model.TestRun) []SessionGroupView {
	sessionMap := make(map[string]*SessionGroupView)
	sessionTestMap := make(map[string]map[string]*TestGroupView) // [sessionName][testName]
	sessionRuns := make(map[string][]model.TestRun)              // Collect runs for sequence diagrams

	for _, run := range results {
		sessionName := run.Execution.SessionName
		if sessionName == "" {
			sessionName = "default" // Single session run
		}

		// Collect runs for sequence diagram
		sessionRuns[sessionName] = append(sessionRuns[sessionName], run)

		// Initialize session group if needed
		if _, exists := sessionMap[sessionName]; !exists {
			sessionMap[sessionName] = &SessionGroupView{
				SessionName: sessionName,
				SourceFile:  run.Execution.SourceFile,
				TestGroups:  []TestGroupView{},
			}
			sessionTestMap[sessionName] = make(map[string]*TestGroupView)
		}

		sessionGroup := sessionMap[sessionName]
		testName := run.Execution.TestName

		// Initialize test group within session if needed
		if _, exists := sessionTestMap[sessionName][testName]; !exists {
			sessionTestMap[sessionName][testName] = &TestGroupView{
				TestName:   testName,
				SourceFile: run.Execution.SourceFile,
				Runs:       []TestRunView{},
			}
		}

		// Update session-level stats
		sessionGroup.TotalTests++
		if run.Passed {
			sessionGroup.PassedTests++
		} else {
			sessionGroup.FailedTests++
		}

		// Build test run view
		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)

		assertions := make([]AssertionView, len(run.Assertions))
		for i, a := range run.Assertions {
			detailsJSON := ""
			if a.Details != nil {
				if b, err := json.Marshal(a.Details); err == nil {
					detailsJSON = string(b)
				}
			}
			assertions[i] = AssertionView{
				Type:    a.Type,
				Passed:  a.Passed,
				Message: a.Message,
				Details: detailsJSON,
			}
		}

		// Extract user prompt from messages
		prompt := ""
		for _, msg := range run.Execution.Messages {
			if msg.Role == "user" {
				prompt = msg.Content
				break
			}
		}

		runView := TestRunView{
			AgentName:       run.Execution.AgentName,
			Provider:        string(run.Execution.ProviderType),
			Passed:          run.Passed,
			DurationSeconds: duration.Seconds(),
			Assertions:      assertions,
			Errors:          run.Execution.Errors,
			Prompt:          prompt,
			TokensUsed:         run.Execution.TokensUsed,
			FinalOutput:        run.Execution.FinalOutput,
			RateLimitStats:     buildRateLimitStatsView(run.Execution.RateLimitStats),
			ClarificationStats: buildClarificationStatsView(run.Execution.ClarificationStats),
		}

		sessionTestMap[sessionName][testName].Runs = append(sessionTestMap[sessionName][testName].Runs, runView)
	}

	// Build the final structure
	sessionList := make([]SessionGroupView, 0, len(sessionMap))
	for sessionName, sessionGroup := range sessionMap {
		// Calculate success rate
		if sessionGroup.TotalTests > 0 {
			sessionGroup.SuccessRate = float64(sessionGroup.PassedTests) / float64(sessionGroup.TotalTests) * 100
			sessionGroup.SuccessRateClass = getSuccessRateClass(sessionGroup.SuccessRate)
		}

		// Build session-level sequence diagram
		if runs, ok := sessionRuns[sessionName]; ok {
			sessionGroup.SequenceDiagram = buildSessionSequenceDiagram(runs)
		}

		// Add test groups to session group
		for _, testGroup := range sessionTestMap[sessionName] {
			sessionGroup.TestGroups = append(sessionGroup.TestGroups, *testGroup)
		}

		// Sort test groups by name
		sort.Slice(sessionGroup.TestGroups, func(i, j int) bool {
			return sessionGroup.TestGroups[i].TestName < sessionGroup.TestGroups[j].TestName
		})

		sessionList = append(sessionList, *sessionGroup)
	}

	// Sort session groups by name
	sort.Slice(sessionList, func(i, j int) bool {
		return sessionList[i].SessionName < sessionList[j].SessionName
	})

	return sessionList
}

func getSuccessRateClass(rate float64) string {
	if rate >= 100 {
		return "success-high"
	} else if rate >= 50 {
		return "success-medium"
	}
	return "success-low"
}

// buildRateLimitStatsView converts model.RateLimitStats to RateLimitStatsView
func buildRateLimitStatsView(stats *model.RateLimitStats) *RateLimitStatsView {
	if stats == nil {
		return nil
	}
	// Only return if there's something to report
	if stats.ThrottleCount == 0 && stats.RateLimitHits == 0 && stats.RetryCount == 0 {
		return nil
	}
	return &RateLimitStatsView{
		ThrottleCount:     stats.ThrottleCount,
		ThrottleWaitSec:   float64(stats.ThrottleWaitTimeMs) / 1000.0,
		RateLimitHits:     stats.RateLimitHits,
		RetryCount:        stats.RetryCount,
		RetryWaitSec:      float64(stats.RetryWaitTimeMs) / 1000.0,
		RetrySuccessCount: stats.RetrySuccessCount,
	}
}

// buildClarificationStatsView converts model.ClarificationStats to ClarificationStatsView
func buildClarificationStatsView(stats *model.ClarificationStats) *ClarificationStatsView {
	if stats == nil || stats.Count == 0 {
		return nil
	}
	return &ClarificationStatsView{
		Count:      stats.Count,
		Iterations: stats.Iterations,
		Examples:   stats.Examples,
	}
}

// buildMatrix creates a test×agent comparison matrix
func buildMatrix(results []model.TestRun) MatrixView {
	testSet := make(map[string]bool)
	agentSet := make(map[string]bool)
	cells := make(map[string]map[string]MatrixCell)

	for _, run := range results {
		testName := run.Execution.TestName
		agentName := run.Execution.AgentName

		testSet[testName] = true
		agentSet[agentName] = true

		if cells[testName] == nil {
			cells[testName] = make(map[string]MatrixCell)
		}

		duration := run.Execution.EndTime.Sub(run.Execution.StartTime)
		cells[testName][agentName] = MatrixCell{
			Passed:     run.Passed,
			HasResult:  true,
			DurationMs: float64(duration.Milliseconds()),
			Tokens:     run.Execution.TokensUsed,
		}
	}

	// Convert sets to sorted slices
	testNames := make([]string, 0, len(testSet))
	for name := range testSet {
		testNames = append(testNames, name)
	}
	sort.Strings(testNames)

	agentNames := make([]string, 0, len(agentSet))
	for name := range agentSet {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)

	return MatrixView{
		TestNames:  testNames,
		AgentNames: agentNames,
		Cells:      cells,
	}
}

// buildSequenceDiagram generates a Mermaid sequence diagram from a test run
func buildSequenceDiagram(run model.TestRun) string {
	var sb strings.Builder
	sb.WriteString("sequenceDiagram\n")
	sb.WriteString("    participant U as User\n")
	sb.WriteString("    participant A as Agent\n")
	sb.WriteString("    participant M as MCP Server\n")

	// Track if we've shown the initial prompt
	promptShown := false

	for _, msg := range run.Execution.Messages {
		// Escape special characters for Mermaid
		content := escapeMermaid(msg.Content)
		if len(content) > 50 {
			content = content[:47] + "..."
		}

		switch msg.Role {
		case "user":
			if !promptShown {
				sb.WriteString(fmt.Sprintf("    U->>A: %s\n", content))
				promptShown = true
			}
		case "assistant":
			// Check if this message triggered tool calls
			// Will be shown as part of tool call flow
		}
	}

	// Add tool calls with actual execution duration
	for _, tc := range run.Execution.ToolCalls {
		toolName := tc.Name

		// Use the actual measured execution duration
		if tc.DurationMs > 0 {
			sb.WriteString(fmt.Sprintf("    A->>M: %s() [%dms]\n", toolName, tc.DurationMs))
		} else {
			sb.WriteString(fmt.Sprintf("    A->>M: %s()\n", toolName))
		}

		// Check if there was a result
		if tc.Result.Content != nil && len(tc.Result.Content) > 0 {
			sb.WriteString("    M-->>A: result\n")
		}
	}

	// Final response with total tokens
	if run.Execution.FinalOutput != "" {
		output := escapeMermaid(run.Execution.FinalOutput)
		if len(output) > 40 {
			output = output[:37] + "..."
		}
		if run.Execution.TokensUsed > 0 {
			sb.WriteString(fmt.Sprintf("    A-->>U: %s [%d tokens]\n", output, run.Execution.TokensUsed))
		} else {
			sb.WriteString(fmt.Sprintf("    A-->>U: %s\n", output))
		}
	}

	return sb.String()
}

// escapeMermaid escapes special characters for Mermaid diagrams
func escapeMermaid(s string) string {
	// Replace characters that break Mermaid syntax
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "#", "")
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, ":", " -")
	s = strings.ReplaceAll(s, ">", "›")
	s = strings.ReplaceAll(s, "<", "‹")
	return s
}

// buildSessionSequenceDiagram generates a Mermaid diagram for an entire session
func buildSessionSequenceDiagram(runs []model.TestRun) string {
	if len(runs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("sequenceDiagram\n")
	sb.WriteString("    participant U as User\n")
	sb.WriteString("    participant A as Agent\n")
	sb.WriteString("    participant M as MCP Server\n")

	for i, run := range runs {
		testName := escapeMermaid(run.Execution.TestName)
		if len(testName) > 30 {
			testName = testName[:27] + "..."
		}

		// Add test boundary note
		if len(runs) > 1 {
			sb.WriteString(fmt.Sprintf("    note over U,M: Test %d - %s\n", i+1, testName))
		}

		// User prompt
		if len(run.Execution.Messages) > 0 {
			for _, msg := range run.Execution.Messages {
				if msg.Role == "user" {
					content := escapeMermaid(msg.Content)
					if len(content) > 40 {
						content = content[:37] + "..."
					}
					sb.WriteString(fmt.Sprintf("    U->>A: %s\n", content))
					break
				}
			}
		}

		// Tool calls
		for _, tc := range run.Execution.ToolCalls {
			sb.WriteString(fmt.Sprintf("    A->>M: %s()\n", tc.Name))
			if tc.Result.Content != nil && len(tc.Result.Content) > 0 {
				sb.WriteString("    M-->>A: result\n")
			}
		}

		// Final response
		if run.Execution.FinalOutput != "" {
			output := escapeMermaid(run.Execution.FinalOutput)
			if len(output) > 30 {
				output = output[:27] + "..."
			}
			status := "✓"
			if !run.Passed {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("    A-->>U: %s %s\n", status, output))
		}
	}

	return sb.String()
}

// formatNumber formats numbers with thousand separators
func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	result := ""
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
