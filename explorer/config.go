// Package explorer implements the exploratory testing mode (-e flag).
// It uses an LLM to iteratively decide what tests to run, executes them via
// the existing engine, and feeds the results into the standard report pipeline.
package explorer

import (
	"fmt"
	"os"

	"github.com/mykhaliev/agent-benchmark/model"
	"gopkg.in/yaml.v3"
)

// ExplorerConfig is the top-level structure for an explorer config file.
// It mirrors GeneratorConfig and TestConfiguration: top-level providers/servers/agents
// sections define the infrastructure, while explorer.agent references an agent by name.
type ExplorerConfig struct {
	Providers []model.Provider  `yaml:"providers"`
	Servers   []model.Server    `yaml:"servers"`
	Agents    []model.Agent     `yaml:"agents"`
	Variables map[string]string `yaml:"variables,omitempty"`
	Settings  model.Settings    `yaml:"settings"`
	Explorer  ExplorerSettings  `yaml:"explorer"`
}

// ExplorerSettings controls the exploratory test execution behaviour.
type ExplorerSettings struct {
	Goal            string `yaml:"goal"`               // Required: what the explorer is trying to test
	MaxIterations   int    `yaml:"max_iterations"`     // Default: 10
	StopOnPassCount int    `yaml:"stop_on_pass_count"` // 0 = disabled: stop after N consecutive passes
	MaxRetries      int    `yaml:"max_retries"`        // Default: 3: LLM retries per iteration
	Agent           string `yaml:"agent"`              // Name reference to an agent defined in top-level agents
	MaxTokens       int    `yaml:"max_tokens"`         // 0 = unlimited; stop exploration loop if cumulative tokens exceed this
}

func (s *ExplorerSettings) applyDefaults() {
	if s.MaxIterations <= 0 {
		s.MaxIterations = 10
	}
	if s.MaxRetries <= 0 {
		s.MaxRetries = 3
	}
}

// ParseExplorerConfig reads and unmarshals an explorer config YAML file,
// applying defaults for any omitted settings.
func ParseExplorerConfig(path string) (*ExplorerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read explorer config %q: %w", path, err)
	}

	var cfg ExplorerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse explorer config %q: %w", path, err)
	}

	if cfg.Explorer.Goal == "" {
		return nil, fmt.Errorf("explorer.goal is required")
	}

	if len(cfg.Agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}

	// Default explorer.agent to the first agent when not set.
	if cfg.Explorer.Agent == "" {
		cfg.Explorer.Agent = cfg.Agents[0].Name
	}

	// Verify the referenced agent exists.
	found := false
	for _, a := range cfg.Agents {
		if a.Name == cfg.Explorer.Agent {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("explorer.agent %q not found in agents", cfg.Explorer.Agent)
	}

	cfg.Explorer.applyDefaults()

	return &cfg, nil
}
