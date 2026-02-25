package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mykhaliev/agent-benchmark/model"
	"gopkg.in/yaml.v3"
)

// infraConfig is a minimal struct used by BuildSuiteYAML to read shared
// infrastructure fields from a config file without importing generator or
// explorer packages.
type infraConfig struct {
	Providers []model.Provider  `yaml:"providers"`
	Servers   []model.Server    `yaml:"servers"`
	Agents    []model.Agent     `yaml:"agents"`
	Variables map[string]string `yaml:"variables,omitempty"`
	Settings  model.Settings    `yaml:"settings"`
}

// Slugify converts a name to a safe filename component.
// For example: "File Operations (edge)" → "file-operations-edge"
func Slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	s := b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "session"
	}
	return s
}

// BuildSuiteYAML reads the original config file at configPath, removes the
// sectionToRemove key and the "sessions" key, then builds and serialises a
// TestSuiteConfiguration referencing the given testFiles.
func BuildSuiteYAML(configPath string, testFiles []string, suiteName, sectionToRemove string) (string, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	var topLevel map[string]interface{}
	if err := yaml.Unmarshal(raw, &topLevel); err != nil {
		return "", fmt.Errorf("failed to parse config file: %w", err)
	}
	delete(topLevel, sectionToRemove)
	delete(topLevel, "sessions")

	infraBytes, err := yaml.Marshal(topLevel)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal infrastructure config: %w", err)
	}

	var cfg infraConfig
	if err := yaml.Unmarshal(infraBytes, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse infrastructure config: %w", err)
	}

	suite := model.TestSuiteConfiguration{
		Name:      suiteName,
		TestFiles: testFiles,
		Providers: cfg.Providers,
		Servers:   cfg.Servers,
		Agents:    cfg.Agents,
		Settings:  cfg.Settings,
		Variables: cfg.Variables,
	}

	suiteBytes, err := yaml.Marshal(suite)
	if err != nil {
		return "", fmt.Errorf("failed to marshal suite config: %w", err)
	}

	return string(suiteBytes), nil
}

// PrintRunCommand prints the command to run a suite file.
func PrintRunCommand(suiteFile string) {
	fmt.Printf("Run with:\n  ./agent-benchmark -s %s -verbose\n\n",
		filepath.ToSlash(suiteFile))
}

// PrintOutputTree prints a visual directory tree for a generated/explored output folder.
// headerLine is printed before the tree.
// filenames and testCounts must have the same length.
func PrintOutputTree(subdir, headerLine string, filenames []string, testCounts []int) {
	fmt.Printf("\n%s\n\n", headerLine)
	fmt.Printf("  %s/\n", filepath.Base(subdir))
	fmt.Printf("  ├── suite.yaml\n")

	for i, filename := range filenames {
		connector := "├──"
		if i == len(filenames)-1 {
			connector = "└──"
		}
		count := 0
		if i < len(testCounts) {
			count = testCounts[i]
		}
		plural := "tests"
		if count == 1 {
			plural = "test"
		}
		fmt.Printf("  %s %-40s (%d %s)\n", connector, filename, count, plural)
	}
	fmt.Println()
}
