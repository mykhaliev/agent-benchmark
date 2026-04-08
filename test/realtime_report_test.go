package tests

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mykhaliev/agent-benchmark/model"
	"github.com/mykhaliev/agent-benchmark/report"
)

// makeTestRun builds a minimal TestRun for realtime report tests.
func makeTestRun(name string, passed bool, latencyMs int64) model.TestRun {
	now := time.Now()
	return model.TestRun{
		Execution: &model.ExecutionResult{
			TestName:  name,
			AgentName: "test-agent",
			StartTime: now,
			EndTime:   now.Add(time.Duration(latencyMs) * time.Millisecond),
			LatencyMs: latencyMs,
		},
		Assertions: []model.AssertionResult{
			{Type: "contains", Passed: passed, Message: "output check"},
		},
		Passed: passed,
	}
}

// readJSONLFile reads all lines from a .jsonl file and returns them.
func readJSONLFile(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading %s: %v", path, err)
	}
	return lines
}

func TestRealtimeReporterOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	if err := r.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	r.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Open() should create the output file")
	}
}

func TestRealtimeReporterOpenInvalidPath(t *testing.T) {
	r := &report.RealtimeReporter{}
	err := r.Open("/nonexistent/dir/out.jsonl")
	if err == nil {
		t.Fatal("Open() should fail for an invalid directory")
	}
}

func TestRealtimeReporterWriteTestResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	if err := r.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	run := makeTestRun("my-test", true, 500)
	if err := r.WriteTestResult(run); err != nil {
		t.Fatalf("WriteTestResult() failed: %v", err)
	}
	r.Close()

	lines := readJSONLFile(t, path)
	// Expect: 1 test line + END
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	var row map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("first line is not valid JSON: %v", err)
	}

	var typ string
	if err := json.Unmarshal(row["type"], &typ); err != nil || typ != "test" {
		t.Errorf("expected type=test, got %q", typ)
	}
	if _, ok := row["data"]; !ok {
		t.Error("test line must have a 'data' field")
	}
}

func TestRealtimeReporterTestLineContainsAssertions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	if err := r.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	run := makeTestRun("assertion-test", false, 200)
	r.WriteTestResult(run)
	r.Close()

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "assertions") {
		t.Error("test line should contain assertions field")
	}
	if !strings.Contains(string(content), "assertion-test") {
		t.Error("test line should contain the test name")
	}
}

func TestRealtimeReporterTestLineContainsTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	if err := r.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	r.WriteTestResult(makeTestRun("ts-test", true, 300))
	r.Close()

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "startTime") && !strings.Contains(string(content), "start_time") {
		t.Error("test line should contain start time")
	}
	if !strings.Contains(string(content), "latencyMs") && !strings.Contains(string(content), "latency_ms") {
		t.Error("test line should contain latency")
	}
}

func TestRealtimeReporterWriteSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	if err := r.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	results := []model.TestRun{
		makeTestRun("t1", true, 100),
		makeTestRun("t2", false, 200),
		makeTestRun("t3", true, 150),
	}
	for _, res := range results {
		r.WriteTestResult(res)
	}
	if err := r.WriteSummary(results); err != nil {
		t.Fatalf("WriteSummary() failed: %v", err)
	}
	r.Close()

	lines := readJSONLFile(t, path)
	// 3 test lines + 1 summary line + END
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	summaryLine := lines[3]
	var row map[string]json.RawMessage
	if err := json.Unmarshal([]byte(summaryLine), &row); err != nil {
		t.Fatalf("summary line is not valid JSON: %v", err)
	}

	var typ string
	json.Unmarshal(row["type"], &typ)
	if typ != "summary" {
		t.Errorf("expected type=summary, got %q", typ)
	}

	var data map[string]json.RawMessage
	json.Unmarshal(row["data"], &data)

	var total, passed, failed int
	json.Unmarshal(data["total_tests"], &total)
	json.Unmarshal(data["passed"], &passed)
	json.Unmarshal(data["failed"], &failed)

	if total != 3 {
		t.Errorf("expected total_tests=3, got %d", total)
	}
	if passed != 2 {
		t.Errorf("expected passed=2, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("expected failed=1, got %d", failed)
	}

	var passRate float64
	json.Unmarshal(data["pass_rate"], &passRate)
	if passRate < 0.66 || passRate > 0.67 {
		t.Errorf("expected pass_rate≈0.667, got %f", passRate)
	}

	if _, ok := data["total_duration_ms"]; !ok {
		t.Error("summary should contain total_duration_ms")
	}
	if _, ok := data["generated_at"]; !ok {
		t.Error("summary should contain generated_at")
	}
}

func TestRealtimeReporterSummaryDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)

	results := []model.TestRun{
		makeTestRun("a", true, 100),
		makeTestRun("b", true, 200),
	}
	r.WriteSummary(results)
	r.Close()

	lines := readJSONLFile(t, path)
	var row map[string]json.RawMessage
	json.Unmarshal([]byte(lines[0]), &row)
	var data map[string]json.RawMessage
	json.Unmarshal(row["data"], &data)

	var totalDuration int64
	json.Unmarshal(data["total_duration_ms"], &totalDuration)
	if totalDuration != 300 {
		t.Errorf("expected total_duration_ms=300, got %d", totalDuration)
	}
}

func TestRealtimeReporterENDSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)
	r.WriteTestResult(makeTestRun("t1", true, 50))
	r.Close()

	lines := readJSONLFile(t, path)
	last := lines[len(lines)-1]
	if last != "END" {
		t.Errorf("last line must be END, got %q", last)
	}
}

func TestRealtimeReporterEmptySummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)
	r.WriteSummary([]model.TestRun{})
	r.Close()

	lines := readJSONLFile(t, path)
	var row map[string]json.RawMessage
	json.Unmarshal([]byte(lines[0]), &row)
	var data map[string]json.RawMessage
	json.Unmarshal(row["data"], &data)

	var passRate float64
	json.Unmarshal(data["pass_rate"], &passRate)
	if passRate != 0 {
		t.Errorf("expected pass_rate=0 for empty results, got %f", passRate)
	}
}

func TestRealtimeReporterMultipleTestsOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)

	names := []string{"first", "second", "third"}
	var results []model.TestRun
	for _, name := range names {
		run := makeTestRun(name, true, 100)
		r.WriteTestResult(run)
		results = append(results, run)
	}
	r.WriteSummary(results)
	r.Close()

	lines := readJSONLFile(t, path)
	for i, name := range names {
		if !strings.Contains(lines[i], name) {
			t.Errorf("line %d should contain test name %q", i, name)
		}
	}
}

func TestRealtimeReporterFileFlushesImmediately(t *testing.T) {
	// Verify data is on disk after WriteTestResult even before Close().
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)

	r.WriteTestResult(makeTestRun("flush-test", true, 10))

	// Read file without closing reporter
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.Contains(string(content), "flush-test") {
		t.Error("data should be flushed to disk immediately after WriteTestResult")
	}

	r.Close()
}

func TestRealtimeReporterLineStructure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	r := &report.RealtimeReporter{}
	r.Open(path)

	results := []model.TestRun{
		makeTestRun("t1", true, 100),
		makeTestRun("t2", false, 200),
	}
	for _, res := range results {
		r.WriteTestResult(res)
	}
	r.WriteSummary(results)
	r.Close()

	lines := readJSONLFile(t, path)

	// Validate each non-END line is valid JSON with type+data
	for i, line := range lines {
		if line == "END" {
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
			continue
		}
		if _, ok := row["type"]; !ok {
			t.Errorf("line %d missing 'type' field", i)
		}
		if _, ok := row["data"]; !ok {
			t.Errorf("line %d missing 'data' field", i)
		}
	}
}
