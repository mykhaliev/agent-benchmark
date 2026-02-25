package explorer

import (
	"fmt"

	"github.com/mykhaliev/agent-benchmark/generator"
	"github.com/mykhaliev/agent-benchmark/model"
)

// ExplorationMetadata is attached to each runtime-generated test to carry
// exploration context through to report rendering.
type ExplorationMetadata struct {
	Mode      string // always "exploration"
	Goal      string
	Iteration int
	PromptID  string
}

// RuntimeTestDefinition represents a dynamically generated test that is fully
// compatible with the existing engine executor.
type RuntimeTestDefinition struct {
	model.Test                     // fully compatible with executor input
	Metadata   ExplorationMetadata // exploration context attached for report encoding
}

// ExplorationTestAdapter converts TestIntents into RuntimeTestDefinitions.
type ExplorationTestAdapter struct{}

// Adapt builds a RuntimeTestDefinition from a TestIntent and exploration metadata.
//
// Session.Name  = "Exploration Goal: <goal>"         → renders as session header in report
// Test.Name     = "[Iter NN | prompt-NNN] <name>"    → renders as test group title in report
// Test.Agent    = agentName                           (the configured agent name)
func (a *ExplorationTestAdapter) Adapt(intent generator.TestIntent, reasoning string, iteration int, promptID, goal, agentName string) RuntimeTestDefinition {
	testName := fmt.Sprintf("[Iter %02d | %s] %s", iteration, promptID, intent.Name)

	var assertions []model.Assertion
	for _, c := range intent.Checks {
		assertions = append(assertions, generator.BuildAssertion(c))
	}

	return RuntimeTestDefinition{
		Test: model.Test{
			Name:         testName,
			Prompt:       intent.Prompt,
			Agent:        agentName,
			Assertions:   assertions,
			AllowedTools: intent.AllowedTools,
		},
		Metadata: ExplorationMetadata{
			Mode:      "exploration",
			Goal:      goal,
			Iteration: iteration,
			PromptID:  promptID,
		},
	}
}

// ToTestConfig converts the RuntimeTestDefinition into a model.TestConfiguration
// suitable for passing to engine.RunTests.
func (r *RuntimeTestDefinition) ToTestConfig(cfg *ExplorerConfig) model.TestConfiguration {
	sessionName := fmt.Sprintf("Exploration Goal: %s", r.Metadata.Goal)

	testCopy := r.Test // copy to avoid mutation

	return model.TestConfiguration{
		Providers: cfg.Providers,
		Servers:   cfg.Servers,
		Agents:    cfg.Agents,
		Sessions: []model.Session{
			{
				Name:  sessionName,
				Tests: []model.Test{testCopy},
			},
		},
		Settings:  cfg.Settings,
		Variables: cfg.Variables,
	}
}
