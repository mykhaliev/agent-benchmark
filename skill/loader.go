package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mykhaliev/agent-benchmark/logger"
	"gopkg.in/yaml.v3"
)

const (
	SkillFileName    = "SKILL.md"
	ReferencesDir    = "references"
	MaxNameLength    = 64
	MaxDescLength    = 1024
	MaxCompatLength  = 500
)

// SkillMetadata represents the frontmatter of a SKILL.md file
// following the Agent Skills specification (agentskills.io/specification)
type SkillMetadata struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  string            `yaml:"allowed-tools,omitempty"`
	Version       string            `yaml:"version,omitempty"`
	Repository    string            `yaml:"repository,omitempty"`
	Documentation string            `yaml:"documentation,omitempty"`
	Tags          []string          `yaml:"tags,omitempty"`
}

// Skill represents a loaded Agent Skill
type Skill struct {
	Path       string            // Absolute path to skill directory
	Metadata   SkillMetadata     // Parsed frontmatter
	Content    string            // Full content (frontmatter + body) for injection
	Body       string            // Body content only (after frontmatter)
	References map[string]string // Cached references (filename -> content)
}

// LoadSkill loads and validates a skill from the given directory path.
// The path should point to a directory containing a SKILL.md file.
func LoadSkill(skillPath string) (*Skill, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(skillPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve skill path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("skill directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill path must be a directory: %s", absPath)
	}

	// Read SKILL.md
	skillFile := filepath.Join(absPath, SkillFileName)
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", SkillFileName, err)
	}

	// Parse frontmatter and body
	metadata, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", SkillFileName, err)
	}

	// Validate metadata
	if err := validateMetadata(metadata); err != nil {
		return nil, fmt.Errorf("invalid skill metadata: %w", err)
	}

	skill := &Skill{
		Path:       absPath,
		Metadata:   *metadata,
		Content:    string(content),
		Body:       body,
		References: make(map[string]string),
	}

	if logger.Logger != nil {
		logger.Logger.Info("Loaded skill",
			"name", metadata.Name,
			"path", absPath,
			"description_length", len(metadata.Description),
		)
	}

	return skill, nil
}

// parseFrontmatter extracts YAML frontmatter and body from a SKILL.md file.
// Frontmatter must be delimited by --- at the start and end.
func parseFrontmatter(content string) (*SkillMetadata, string, error) {
	// Handle different line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Check for frontmatter delimiter
	if !strings.HasPrefix(content, "---") {
		return nil, "", fmt.Errorf("SKILL.md must start with YAML frontmatter (---)")
	}

	// Find the closing delimiter
	endIndex := strings.Index(content[3:], "\n---")
	if endIndex == -1 {
		return nil, "", fmt.Errorf("SKILL.md frontmatter not properly closed (missing ---)")
	}

	// Extract frontmatter YAML (between the delimiters)
	frontmatterYAML := content[4 : endIndex+3]

	// Extract body (after the closing delimiter)
	bodyStart := endIndex + 3 + 4 // Skip past "\n---"
	body := ""
	if bodyStart < len(content) {
		body = strings.TrimPrefix(content[bodyStart:], "\n")
	}

	// Parse YAML
	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &metadata); err != nil {
		return nil, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return &metadata, body, nil
}

// validateMetadata validates skill metadata according to the Agent Skills spec
func validateMetadata(m *SkillMetadata) error {
	// Name is required
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Name validation: 1-64 chars, lowercase alphanumeric and hyphens
	if len(m.Name) > MaxNameLength {
		return fmt.Errorf("name must be 1-%d characters, got %d", MaxNameLength, len(m.Name))
	}

	namePattern := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("name must be lowercase alphanumeric with hyphens, no leading/trailing/consecutive hyphens: %s", m.Name)
	}

	// Description is required
	if m.Description == "" {
		return fmt.Errorf("description is required")
	}

	// Description validation: 1-1024 chars
	if len(m.Description) > MaxDescLength {
		return fmt.Errorf("description must be 1-%d characters, got %d", MaxDescLength, len(m.Description))
	}

	// Compatibility is optional but has max length
	if len(m.Compatibility) > MaxCompatLength {
		return fmt.Errorf("compatibility must be 1-%d characters, got %d", MaxCompatLength, len(m.Compatibility))
	}

	return nil
}

// ReadReference reads a reference file from the skill's references/ directory.
// The filename should be relative to the references/ directory.
func (s *Skill) ReadReference(filename string) (string, error) {
	// Check cache first
	if content, ok := s.References[filename]; ok {
		return content, nil
	}

	// Build path and validate it's within references/
	refPath := filepath.Join(s.Path, ReferencesDir, filename)
	absRefPath, err := filepath.Abs(refPath)
	if err != nil {
		return "", fmt.Errorf("invalid reference path: %w", err)
	}

	// Security: ensure the resolved path is within the skill's references directory
	refsDir := filepath.Join(s.Path, ReferencesDir)
	if !strings.HasPrefix(absRefPath, refsDir) {
		return "", fmt.Errorf("reference path escapes skill directory: %s", filename)
	}

	// Read file
	content, err := os.ReadFile(absRefPath)
	if err != nil {
		return "", fmt.Errorf("failed to read reference %s: %w", filename, err)
	}

	// Cache for future reads
	s.References[filename] = string(content)

	if logger.Logger != nil {
		logger.Logger.Debug("Read skill reference",
			"skill", s.Metadata.Name,
			"file", filename,
			"length", len(content),
		)
	}

	return string(content), nil
}

// ListReferences returns the list of available reference files in the skill's references/ directory.
func (s *Skill) ListReferences() ([]string, error) {
	refsDir := filepath.Join(s.Path, ReferencesDir)

	// Check if references directory exists
	if _, err := os.Stat(refsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list references: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// GetContentForInjection returns the skill content formatted for injection into the system prompt.
// This includes the full SKILL.md content.
func (s *Skill) GetContentForInjection() string {
	return s.Content
}
