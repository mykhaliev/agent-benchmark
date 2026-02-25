package generator

import (
	"fmt"
	"os"

	"github.com/mykhaliev/agent-benchmark/model"
	"gopkg.in/yaml.v3"
)

// GeneratorConfig is the top-level structure for a generator config file.
// It mirrors TestConfiguration but omits Sessions and adds a Generator section.
type GeneratorConfig struct {
	Providers []model.Provider  `yaml:"providers"`
	Servers   []model.Server    `yaml:"servers"`
	Agents    []model.Agent     `yaml:"agents"`
	Variables map[string]string `yaml:"variables,omitempty"`
	Settings  model.Settings    `yaml:"settings"`
	Generator GeneratorSettings `yaml:"generator"`
}

// GeneratorSettings controls the test generation behaviour.
type GeneratorSettings struct {
	Provider         string   `yaml:"provider"`           // LLM to use for generation (defaults to first agent's provider)
	TestCount        int      `yaml:"test_count"`         // Number of tests to generate (default 5)
	Complexity       string   `yaml:"complexity"`         // simple | medium | complex (default "medium")
	IncludeEdgeCases bool     `yaml:"include_edge_cases"` // Whether to include edge case tests (default false)
	MaxStepsPerTest  int      `yaml:"max_steps_per_test"` // Max tool-call steps per test (default 5)
	Tools            []string `yaml:"tools,omitempty"`    // Allowlist of tool names; empty means all tools
}

func (s *GeneratorSettings) applyDefaults() {
	if s.TestCount <= 0 {
		s.TestCount = 5
	}
	if s.Complexity == "" {
		s.Complexity = "medium"
	}
	if s.MaxStepsPerTest <= 0 {
		s.MaxStepsPerTest = 5
	}
}

// ParseGeneratorConfig reads and unmarshals a generator config YAML file,
// applying defaults for any omitted generator settings.
func ParseGeneratorConfig(path string) (*GeneratorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read generator config %q: %w", path, err)
	}

	var cfg GeneratorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse generator config %q: %w", path, err)
	}

	cfg.Generator.applyDefaults()

	// Default generator provider to the first agent's provider when not set.
	if cfg.Generator.Provider == "" && len(cfg.Agents) > 0 {
		cfg.Generator.Provider = cfg.Agents[0].Provider
	}

	return &cfg, nil
}
