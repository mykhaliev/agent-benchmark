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
	Agent              string   `yaml:"agent"`                 // agent whose LLM is used for generation (defaults to first agent)
	TestCount          int      `yaml:"test_count"`            // Number of tests to generate (default 5)
	Complexity         string   `yaml:"complexity"`            // simple | medium | complex (default "medium")
	IncludeEdgeCases   bool     `yaml:"include_edge_cases"`    // Whether to include edge case tests (default false)
	MaxStepsPerTest    int      `yaml:"max_steps_per_test"`    // Max tool-call steps per test (default 5)
	MaxRetries         int      `yaml:"max_retries"`           // Max LLM generation attempts per phase (default 3)
	Tools              []string `yaml:"tools,omitempty"`       // Allowlist of tool names; empty means all tools
	MaxTokens          int      `yaml:"max_tokens"`            // 0 = unlimited; stop retrying if cumulative tokens exceed this
	Goal               string   `yaml:"goal"`                  // User prompt for the generator
	MaxIterations      int      `yaml:"max_iterations"`        // Max llm conversation iterations
	PlanChunkSize      int      `yaml:"plan_chunk_size"`       // Max tests per plan chunk (0 = use default of 5)
	PlanChunkMaxTokens int      `yaml:"plan_chunk_max_tokens"` // Max output tokens per plan chunk LLM call (0 = auto)
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
	if s.MaxRetries <= 0 {
		s.MaxRetries = 3
	}
	if s.PlanChunkSize <= 0 {
		s.PlanChunkSize = 5
	}
	if s.PlanChunkMaxTokens <= 0 {
		s.PlanChunkMaxTokens = max(4096, s.PlanChunkSize*800)
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

	// Default generator agent to the first agent when not set.
	if cfg.Generator.Agent == "" && len(cfg.Agents) > 0 {
		cfg.Generator.Agent = cfg.Agents[0].Name
	}

	return &cfg, nil
}
