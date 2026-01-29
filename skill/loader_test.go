package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSkill_ValidSkill tests loading a well-formed skill
func TestLoadSkill_ValidSkill(t *testing.T) {
	// Create a temporary skill directory
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	// Create a valid SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for unit testing.
license: MIT
version: 1.0.0
tags:
  - test
  - example
---

# Test Skill

This is a test skill body.

## Usage

Use this skill for testing.
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Load the skill
	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	// Verify metadata
	if skill.Metadata.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", skill.Metadata.Name)
	}
	if skill.Metadata.Description != "A test skill for unit testing." {
		t.Errorf("Expected description 'A test skill for unit testing.', got '%s'", skill.Metadata.Description)
	}
	if skill.Metadata.License != "MIT" {
		t.Errorf("Expected license 'MIT', got '%s'", skill.Metadata.License)
	}
	if skill.Metadata.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", skill.Metadata.Version)
	}
	if len(skill.Metadata.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(skill.Metadata.Tags))
	}

	// Verify body extraction
	if skill.Body == "" {
		t.Error("Body should not be empty")
	}
	if skill.Content == "" {
		t.Error("Content should not be empty")
	}
}

// TestLoadSkill_MissingName tests that missing name is rejected
func TestLoadSkill_MissingName(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "invalid-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
description: A skill without a name.
---

# Invalid Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	_, err := LoadSkill(skillDir)
	if err == nil {
		t.Error("Expected error for missing name, got nil")
	}
}

// TestLoadSkill_MissingDescription tests that missing description is rejected
func TestLoadSkill_MissingDescription(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "invalid-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
name: no-description
---

# Invalid Skill
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	_, err := LoadSkill(skillDir)
	if err == nil {
		t.Error("Expected error for missing description, got nil")
	}
}

// TestLoadSkill_InvalidName tests that invalid name format is rejected
func TestLoadSkill_InvalidName(t *testing.T) {
	testCases := []struct {
		name        string
		skillName   string
		shouldError bool
	}{
		{"uppercase", "Test-Skill", true},
		{"leading hyphen", "-test-skill", true},
		{"trailing hyphen", "test-skill-", true},
		{"consecutive hyphens", "test--skill", true},
		{"valid simple", "test", false},
		{"valid with hyphen", "test-skill", false},
		{"valid with numbers", "test-skill-123", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			skillDir := filepath.Join(tempDir, "test-skill")
			if err := os.Mkdir(skillDir, 0755); err != nil {
				t.Fatalf("Failed to create skill directory: %v", err)
			}

			skillContent := "---\nname: " + tc.skillName + "\ndescription: Test description.\n---\n# Body\n"
			skillPath := filepath.Join(skillDir, "SKILL.md")
			if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
				t.Fatalf("Failed to write SKILL.md: %v", err)
			}

			_, err := LoadSkill(skillDir)
			if tc.shouldError && err == nil {
				t.Errorf("Expected error for name '%s', got nil", tc.skillName)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("Expected no error for name '%s', got: %v", tc.skillName, err)
			}
		})
	}
}

// TestLoadSkill_NoFrontmatter tests that missing frontmatter is rejected
func TestLoadSkill_NoFrontmatter(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "invalid-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `# No Frontmatter

This skill has no YAML frontmatter.
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	_, err := LoadSkill(skillDir)
	if err == nil {
		t.Error("Expected error for missing frontmatter, got nil")
	}
}

// TestLoadSkill_NonExistentPath tests loading from non-existent path
func TestLoadSkill_NonExistentPath(t *testing.T) {
	_, err := LoadSkill("/nonexistent/path/to/skill")
	if err == nil {
		t.Error("Expected error for non-existent path, got nil")
	}
}

// TestLoadSkill_FileNotDirectory tests loading from a file instead of directory
func TestLoadSkill_FileNotDirectory(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	_, err := LoadSkill(filePath)
	if err == nil {
		t.Error("Expected error for file path (not directory), got nil")
	}
}

// TestSkill_ReadReference tests reading reference files
func TestSkill_ReadReference(t *testing.T) {
	// Create skill with references
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create SKILL.md
	skillContent := `---
name: test-skill
description: A skill with references.
---

# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create reference file
	refContent := "# Guide\n\nThis is a guide."
	if err := os.WriteFile(filepath.Join(refsDir, "guide.md"), []byte(refContent), 0644); err != nil {
		t.Fatalf("Failed to write reference: %v", err)
	}

	// Load skill
	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	// Read reference
	content, err := skill.ReadReference("guide.md")
	if err != nil {
		t.Fatalf("ReadReference failed: %v", err)
	}
	if content != refContent {
		t.Errorf("Expected reference content '%s', got '%s'", refContent, content)
	}

	// Verify caching
	content2, err := skill.ReadReference("guide.md")
	if err != nil {
		t.Fatalf("Second ReadReference failed: %v", err)
	}
	if content2 != content {
		t.Error("Cached content mismatch")
	}
}

// TestSkill_ReadReference_NotFound tests reading non-existent reference
func TestSkill_ReadReference_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A skill without references.
---

# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	_, err = skill.ReadReference("nonexistent.md")
	if err == nil {
		t.Error("Expected error for non-existent reference, got nil")
	}
}

// TestSkill_ReadReference_PathTraversal tests path traversal prevention
func TestSkill_ReadReference_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	skillContent := `---
name: test-skill
description: A skill for path traversal testing.
---

# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	// Try path traversal
	_, err = skill.ReadReference("../SKILL.md")
	if err == nil {
		t.Error("Expected error for path traversal, got nil")
	}
}

// TestSkill_ListReferences tests listing reference files
func TestSkill_ListReferences(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	refsDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	skillContent := `---
name: test-skill
description: A skill with multiple references.
---

# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create reference files
	refs := []string{"guide.md", "api.md", "examples.md"}
	for _, ref := range refs {
		if err := os.WriteFile(filepath.Join(refsDir, ref), []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to write reference %s: %v", ref, err)
		}
	}

	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	listed, err := skill.ListReferences()
	if err != nil {
		t.Fatalf("ListReferences failed: %v", err)
	}
	if len(listed) != len(refs) {
		t.Errorf("Expected %d references, got %d", len(refs), len(listed))
	}
}

// TestSkill_ListReferences_NoRefsDir tests listing when references/ doesn't exist
func TestSkill_ListReferences_NoRefsDir(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "test-skill")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A skill without references directory.
---

# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	skill, err := LoadSkill(skillDir)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	listed, err := skill.ListReferences()
	if err != nil {
		t.Fatalf("ListReferences failed: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("Expected 0 references, got %d", len(listed))
	}
}
