# Agent Skills

## Overview

Agent Skills inject domain-specific knowledge into agents via SKILL.md files following the [Agent Skills specification](https://agentskills.io/specification). This enables agents to have specialized knowledge about tools, APIs, or domains without cluttering test configurations.

## How It Works

1. **Skill Loading**: The engine loads the SKILL.md file from the specified path
2. **System Prompt Injection**: Skill content is prepended to the agent's system prompt
3. **Reference Tools**: If the skill has a `references/` directory, built-in tools are automatically added for on-demand access

## Configuration

Add the `skill` block to your agent configuration:

```yaml
agents:
  - name: skilled-agent
    provider: azure-openai
    skill:
      path: "./skills/my-skill"  # Path to skill directory
    system_prompt: |
      Additional instructions here...
```

### Configuration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Path to skill directory containing SKILL.md |

## Skill Directory Structure

```
my-skill/
├── SKILL.md              # Required: Skill definition with frontmatter + body
└── references/           # Optional: Additional reference files
    ├── guide.md
    └── api.md
```

## SKILL.md Format

Skills must have YAML frontmatter with required fields:

```markdown
---
name: my-skill                    # Required: lowercase, hyphens allowed
description: What this skill does # Required: max 1024 chars
license: MIT                      # Optional
version: 1.0.0                    # Optional
tags:
  - example
---

# Skill Content

Instructions for the agent go here...
```

### Validation Rules

- **name**: Required, lowercase letters and hyphens only, 1-64 characters
- **description**: Required, max 1024 characters
- No leading/trailing hyphens or consecutive hyphens in name

## Progressive Disclosure

Following the Agent Skills specification, content is loaded progressively:

1. **Metadata**: `name` and `description` available for skill matching
2. **SKILL.md body**: Full content injected when skill activates
3. **References**: Files in `references/` available on-demand via built-in tools

### Built-in Tools

When a skill has a `references/` directory with files, two built-in tools are automatically added:

| Tool | Description |
|------|-------------|
| `list_skill_references` | Lists available reference files in the skill's `references/` directory |
| `read_skill_reference` | Reads a specific reference file by filename |

This allows agents to discover and load additional documentation as needed, rather than injecting all content upfront.

**Why built-in tools?**

Real agents (GitHub Copilot, etc.) use their existing `read_file` tools for progressive disclosure. But in agent-benchmark:

- We can't assume the agent has file tools configured
- We can't assume file tools can access the skill directory
- We want to test **skill comprehension**, not MCP configuration

Built-in tools guarantee reference access regardless of MCP server setup.

## Template Variables

When a skill is loaded, these template variables are available:

| Variable | Description |
|----------|-------------|
| `{{SKILL_DIR}}` | Absolute path to the skill directory |

## Combining with System Prompt

When both `skill` and `system_prompt` are specified, they are combined in order:

1. Skill content (from SKILL.md body) is injected first
2. Custom system_prompt is appended after

```yaml
agents:
  - name: expert-agent
    provider: azure-openai
    skill:
      path: "./skills/excel-automation"
    system_prompt: |
      Additional context for this specific test:
      - Focus on performance optimization
      - Prefer batch operations
```

## Example

### Skill Definition (./skills/demo-skill/SKILL.md)

```markdown
---
name: demo-skill
description: Demonstrates Agent Skills features
---

# Demo Skill

Always greet with "Demo Skill activated!"

## Instructions

1. Be helpful and concise
2. Use available tools when needed

For detailed guidelines, see the references.
```

### Test Configuration

```yaml
agents:
  - name: skilled-agent
    provider: azure-openai
    skill:
      path: "./skills/demo-skill"
    servers:
      - name: mcp-server

sessions:
  - name: "Skill Tests"
    tests:
      - name: "Verify skill loaded"
        prompt: "What skill do you have?"
        assertions:
          - type: output_contains
            value: "Demo Skill"

      - name: "Read references"
        prompt: "List your skill references"
        assertions:
          - type: tool_called
            tool: list_skill_references
```

## See Also

- [Agent Skills Specification](https://agentskills.io/specification)
- [examples/agent-skills-test.yaml](../examples/agent-skills-test.yaml) - Full example
- [skills/agent-benchmark/](../skills/agent-benchmark/) - Skill for writing agent-benchmark configs
