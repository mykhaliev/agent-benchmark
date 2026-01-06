package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/report"
)

func TestNewGenerator(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}
	if gen == nil {
		t.Fatal("NewGenerator() returned nil")
	}
}

func TestGenerateHTML(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	results := createSampleTestRuns()

	html, err := gen.GenerateHTML(results)
	if err != nil {
		t.Fatalf("GenerateHTML() failed: %v", err)
	}

	// Verify HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain DOCTYPE")
	}
	if !strings.Contains(html, "<title>Test Results") {
		t.Error("HTML should contain title")
	}
	if !strings.Contains(html, "Test Results") {
		t.Error("HTML should contain 'Test Results' heading")
	}

	// Verify CSS is embedded
	if !strings.Contains(html, "<style>") {
		t.Error("HTML should contain embedded styles")
	}
	if !strings.Contains(html, ".container") {
		t.Error("HTML should contain CSS class definitions")
	}

	// Verify data is rendered
	if !strings.Contains(html, "test-agent") {
		t.Error("HTML should contain agent name")
	}
	if !strings.Contains(html, "openai") {
		t.Error("HTML should contain provider name")
	}
	if !strings.Contains(html, "Sample Test") {
		t.Error("HTML should contain test name")
	}
}

func TestGenerateHTMLEmptyResults(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	results := []model.TestRun{}

	html, err := gen.GenerateHTML(results)
	if err != nil {
		t.Fatalf("GenerateHTML() with empty results failed: %v", err)
	}

	// Should still generate valid HTML
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain DOCTYPE even with empty results")
	}
	if !strings.Contains(html, "Total Tests") {
		t.Error("HTML should contain summary section")
	}
}

func TestGenerateHTMLToFile(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	results := createSampleTestRuns()
	outputPath := filepath.Join(t.TempDir(), "test-report.html")

	err = gen.GenerateHTMLToFile(results, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTMLToFile() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Output file was not created")
	}

	// Verify file content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "<!DOCTYPE html>") {
		t.Error("Output file should contain valid HTML")
	}
}

func TestLoadResultsFromJSON(t *testing.T) {
	// Create a temporary JSON file
	jsonContent := `{
		"agent_benchmark_version": "v0.1.0",
		"generated_at": "2026-01-02T10:00:00Z",
		"summary": {"total": 1, "passed": 1, "failed": 0},
		"detailed_results": [
			{
				"execution": {
					"testName": "JSON Test",
					"agentName": "test-agent",
					"providerType": "openai",
					"startTime": "2026-01-02T10:00:00Z",
					"endTime": "2026-01-02T10:00:05Z",
					"messages": [],
					"toolCalls": [],
					"finalOutput": "Success",
					"tokensUsed": 100,
					"latencyMs": 5000,
					"errors": []
				},
				"assertions": [],
				"passed": true,
				"testCriteria": {}
			}
		]
	}`

	jsonPath := filepath.Join(t.TempDir(), "test-results.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test JSON: %v", err)
	}

	results, err := report.LoadResultsFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("LoadResultsFromJSON() failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if results[0].Execution.TestName != "JSON Test" {
		t.Errorf("Expected test name 'JSON Test', got '%s'", results[0].Execution.TestName)
	}
}

func TestLoadResultsFromJSONInvalidFile(t *testing.T) {
	_, err := report.LoadResultsFromJSON("/nonexistent/file.json")
	if err == nil {
		t.Error("LoadResultsFromJSON() should fail for nonexistent file")
	}
}

func TestLoadResultsFromJSONInvalidJSON(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(jsonPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := report.LoadResultsFromJSON(jsonPath)
	if err == nil {
		t.Error("LoadResultsFromJSON() should fail for invalid JSON")
	}
}

func TestLoadResultsFromJSONEmptyResults(t *testing.T) {
	jsonContent := `{
		"agent_benchmark_version": "v0.1.0",
		"detailed_results": []
	}`

	jsonPath := filepath.Join(t.TempDir(), "empty-results.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test JSON: %v", err)
	}

	_, err := report.LoadResultsFromJSON(jsonPath)
	if err == nil {
		t.Error("LoadResultsFromJSON() should fail for empty results")
	}
}

func TestGenerateReportFromJSON(t *testing.T) {
	// Create a temporary JSON file
	jsonContent := `{
		"agent_benchmark_version": "v0.1.0",
		"generated_at": "2026-01-02T10:00:00Z",
		"summary": {"total": 1, "passed": 1, "failed": 0},
		"detailed_results": [
			{
				"execution": {
					"testName": "End-to-End Test",
					"agentName": "e2e-agent",
					"providerType": "anthropic",
					"startTime": "2026-01-02T10:00:00Z",
					"endTime": "2026-01-02T10:00:10Z",
					"messages": [],
					"toolCalls": [],
					"finalOutput": "Done",
					"tokensUsed": 500,
					"latencyMs": 10000,
					"errors": []
				},
				"assertions": [
					{"type": "contains", "passed": true, "message": "Output contains 'Done'", "details": {}}
				],
				"passed": true,
				"testCriteria": {}
			}
		]
	}`

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.html")

	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test JSON: %v", err)
	}

	err := report.GenerateReportFromJSON(jsonPath, outputPath)
	if err != nil {
		t.Fatalf("GenerateReportFromJSON() failed: %v", err)
	}

	// Verify output file
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	htmlContent := string(content)
	if !strings.Contains(htmlContent, "End-to-End Test") {
		t.Error("HTML should contain test name")
	}
	if !strings.Contains(htmlContent, "e2e-agent") {
		t.Error("HTML should contain agent name")
	}
	if !strings.Contains(htmlContent, "anthropic") {
		t.Error("HTML should contain provider name")
	}
}

func TestHTMLContainsAllSections(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	// Use multi-agent fixture to ensure all sections are visible
	results := createMultiAgentComparison()
	html, err := gen.GenerateHTML(results)
	if err != nil {
		t.Fatalf("GenerateHTML() failed: %v", err)
	}

	// Verify all major sections are present (multi-agent shows all sections)
	sections := []string{
		"Agent Leaderboard",
		"Comparison Matrix",
		"Detailed Test Results",
		"Total Tests",
		"Passed",
		"Failed",
	}

	for _, section := range sections {
		if !strings.Contains(html, section) {
			t.Errorf("HTML should contain section: %s", section)
		}
	}
}

func TestHTMLPassedFailedStyling(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	// Create results with both passed and failed tests
	results := []model.TestRun{
		{
			Execution: &model.ExecutionResult{
				TestName:     "Passed Test",
				AgentName:    "agent-1",
				ProviderType: "openai",
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(5 * time.Second),
				TokensUsed:   100,
			},
			Passed: true,
		},
		{
			Execution: &model.ExecutionResult{
				TestName:     "Failed Test",
				AgentName:    "agent-2",
				ProviderType: "anthropic",
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(5 * time.Second),
				TokensUsed:   200,
				Errors:       []string{"Something went wrong"},
			},
			Passed: false,
		},
	}

	html, err := gen.GenerateHTML(results)
	if err != nil {
		t.Fatalf("GenerateHTML() failed: %v", err)
	}

	// Verify pass/fail styling classes are used
	if !strings.Contains(html, "passed") || !strings.Contains(html, "✅") {
		t.Error("HTML should contain pass status styling")
	}
	if !strings.Contains(html, "failed") || !strings.Contains(html, "❌") {
		t.Error("HTML should contain fail status styling")
	}
}

// TestReportFixtures verifies all fixture functions produce valid data
func TestReportFixtures(t *testing.T) {
	fixtures := map[string]func() []model.TestRun{
		"SingleAgentSingleTest": createSingleAgentSingleTest,
		"MultiAgentComparison":  createMultiAgentComparison,
		"MultiSessionRun":       createMultiSessionRun,
		"SuiteRun":              createSuiteRun,
		"FailedTestWithErrors":  createFailedTestWithErrors,
		"LargeScaleRun":         createLargeScaleRun,
	}

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			results := fixture()
			if len(results) == 0 {
				t.Errorf("%s: expected non-empty results", name)
			}

			for i, run := range results {
				if run.Execution == nil {
					t.Errorf("%s[%d]: Execution should not be nil", name, i)
					continue
				}
				if run.Execution.TestName == "" {
					t.Errorf("%s[%d]: TestName should not be empty", name, i)
				}
				if run.Execution.AgentName == "" {
					t.Errorf("%s[%d]: AgentName should not be empty", name, i)
				}
			}
		})
	}
}

// TestGenerateHTMLWithFixtures generates HTML reports from each fixture
func TestGenerateHTMLWithFixtures(t *testing.T) {
	gen, err := report.NewGenerator()
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	fixtures := map[string]func() []model.TestRun{
		"single_agent":  createSingleAgentSingleTest,
		"multi_agent":   createMultiAgentComparison,
		"multi_session": createMultiSessionRun,
		"suite_run":     createSuiteRun,
		"failed_errors": createFailedTestWithErrors,
		"large_scale":   createLargeScaleRun,
	}

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			results := fixture()

			html, err := gen.GenerateHTML(results)
			if err != nil {
				t.Fatalf("Failed to generate HTML: %v", err)
			}

			if len(html) == 0 {
				t.Error("Generated HTML should not be empty")
			}

			t.Logf("Generated %s report: %d bytes", name, len(html))
		})
	}
}

// Helper function to create sample test runs
func createSampleTestRuns() []model.TestRun {
	return []model.TestRun{
		{
			Execution: &model.ExecutionResult{
				TestName:     "Sample Test",
				AgentName:    "test-agent",
				ProviderType: "openai",
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(5 * time.Second),
				TokensUsed:   1000,
				FinalOutput:  "Test completed successfully",
			},
			Assertions: []model.AssertionResult{
				{
					Type:    "contains",
					Passed:  true,
					Message: "Output contains expected text",
				},
			},
			Passed: true,
		},
	}
}
