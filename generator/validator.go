package generator

import (
	"fmt"
	"strings"

	"github.com/mykhaliev/agent-benchmark/model"
	"gopkg.in/yaml.v3"
)

// sessionsWrapper is a helper for unmarshalling only the sessions block.
type sessionsWrapper struct {
	Sessions []model.Session `yaml:"sessions"`
}

// ValidateSessions parses the YAML content (which must contain a "sessions:" key)
// and validates it against the known agent names and assertion types.
// Returns a list of human-readable error strings; an empty list means the content is valid.
func ValidateSessions(yamlContent string, knownAgents []string) []string {
	var errs []string

	var wrapper sessionsWrapper
	if err := yaml.Unmarshal([]byte(yamlContent), &wrapper); err != nil {
		return []string{fmt.Sprintf("YAML parse error: %v", err)}
	}

	if len(wrapper.Sessions) == 0 {
		return []string{"no sessions found in generated output"}
	}

	agentSet := make(map[string]bool, len(knownAgents))
	for _, a := range knownAgents {
		agentSet[a] = true
	}

	assertionTypeSet := make(map[string]bool, len(validAssertionTypes))
	for _, t := range validAssertionTypes {
		assertionTypeSet[t] = true
	}

	for si, session := range wrapper.Sessions {
		sessionLabel := fmt.Sprintf("session[%d](%q)", si, session.Name)

		if session.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: missing name", sessionLabel))
		}
		if len(session.Tests) == 0 {
			errs = append(errs, fmt.Sprintf("%s: has no tests", sessionLabel))
		}

		for ti, test := range session.Tests {
			testLabel := fmt.Sprintf("%s/test[%d](%q)", sessionLabel, ti, test.Name)

			if test.Name == "" {
				errs = append(errs, fmt.Sprintf("%s: missing name", testLabel))
			}
			if test.Prompt == "" {
				errs = append(errs, fmt.Sprintf("%s: missing prompt", testLabel))
			}
			if test.Agent == "" {
				errs = append(errs, fmt.Sprintf("%s: missing agent field", testLabel))
			} else if len(agentSet) > 0 && !agentSet[test.Agent] {
				errs = append(errs, fmt.Sprintf(
					"%s: unknown agent %q (valid: %s)",
					testLabel, test.Agent, strings.Join(knownAgents, ", ")))
			}

			for ai, assertion := range test.Assertions {
				assertionLabel := fmt.Sprintf("%s/assertion[%d]", testLabel, ai)
				if assertion.Type == "" {
					errs = append(errs, fmt.Sprintf("%s: missing type", assertionLabel))
				} else if !assertionTypeSet[assertion.Type] {
					errs = append(errs, fmt.Sprintf(
						"%s: unknown assertion type %q", assertionLabel, assertion.Type))
				}
			}
		}
	}

	return errs
}
