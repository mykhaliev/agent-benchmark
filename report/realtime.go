package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mykhaliev/agent-benchmark/model"
)

// RealtimeReporter writes test results as NDJSON (newline-delimited JSON) in real-time.
// Each line is a JSON object with a "type" field: "test" or "summary".
// The final line is the literal string "END" (non-JSON sentinel for parsers).
//
// Format:
//
//	{"type":"test","data":{...model.TestRun...}}
//	{"type":"summary","data":{...realtimeSummary...}}
//	END
type RealtimeReporter struct {
	file   *os.File
	writer *bufio.Writer
	mu     sync.Mutex
}

type realtimeLine struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type realtimeSummary struct {
	TotalTests      int     `json:"total_tests"`
	Passed          int     `json:"passed"`
	Failed          int     `json:"failed"`
	PassRate        float64 `json:"pass_rate"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	GeneratedAt     string  `json:"generated_at"`
}

// Open creates (or truncates) the output file at path.
func (r *RealtimeReporter) Open(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("realtime reporter: failed to open %s: %w", path, err)
	}
	r.file = f
	r.writer = bufio.NewWriter(f)
	return nil
}

// WriteTestResult serializes a single TestRun as a "test" line and flushes immediately.
func (r *RealtimeReporter) WriteTestResult(result model.TestRun) error {
	return r.writeLine(realtimeLine{Type: "test", Data: result})
}

// WriteSummary computes aggregate stats from all results and writes a "summary" line.
func (r *RealtimeReporter) WriteSummary(results []model.TestRun) error {
	total := len(results)
	passed := 0
	var totalDurationMs int64
	for _, res := range results {
		if res.Passed {
			passed++
		}
		if res.Execution != nil {
			totalDurationMs += res.Execution.LatencyMs
		}
	}
	failed := total - passed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total)
	}
	summary := realtimeSummary{
		TotalTests:      total,
		Passed:          passed,
		Failed:          failed,
		PassRate:        passRate,
		TotalDurationMs: totalDurationMs,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	return r.writeLine(realtimeLine{Type: "summary", Data: summary})
}

// Close writes the "END" sentinel, flushes the buffer, and closes the file.
func (r *RealtimeReporter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := fmt.Fprintln(r.writer, "END"); err != nil {
		return fmt.Errorf("realtime reporter: failed to write END sentinel: %w", err)
	}
	if err := r.writer.Flush(); err != nil {
		return fmt.Errorf("realtime reporter: failed to flush: %w", err)
	}
	return r.file.Close()
}

func (r *RealtimeReporter) writeLine(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("realtime reporter: failed to marshal line: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.writer.Write(data); err != nil {
		return err
	}
	if err := r.writer.WriteByte('\n'); err != nil {
		return err
	}
	return r.writer.Flush()
}
