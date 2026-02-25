package explorer

import (
	"fmt"
	"sync"
)

// PromptEntry records one exploration decision made by the explorer LLM.
type PromptEntry struct {
	PromptID  string // "prompt-001"
	Iteration int
	Prompt    string // Full text sent to explorer LLM
	Reasoning string // LLM reasoning extracted from response
}

// PromptRegistry stores all prompts sent during an exploration run in order.
// It is safe for concurrent use.
type PromptRegistry struct {
	mu      sync.Mutex
	entries []PromptEntry
}

// NewPromptRegistry creates an empty PromptRegistry.
func NewPromptRegistry() *PromptRegistry {
	return &PromptRegistry{}
}

// Register records a new prompt + reasoning pair for the given iteration and
// returns the assigned prompt ID (e.g. "prompt-001").
func (r *PromptRegistry) Register(iteration int, prompt, reasoning string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := fmt.Sprintf("prompt-%03d", len(r.entries)+1)
	r.entries = append(r.entries, PromptEntry{
		PromptID:  id,
		Iteration: iteration,
		Prompt:    prompt,
		Reasoning: reasoning,
	})
	return id
}

// All returns a snapshot of all registered entries in insertion order.
func (r *PromptRegistry) All() []PromptEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]PromptEntry, len(r.entries))
	copy(result, r.entries)
	return result
}
