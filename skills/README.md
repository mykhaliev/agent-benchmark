# Agent Benchmark Skills

This folder contains [Agent Skills](https://agentskills.io/) for AI coding assistants (VS Code Copilot, Claude, etc.) that help write agent-benchmark configurations.

## Available Skills

### agent-benchmark
Helps write YAML test configurations for agent-benchmark. Includes guidance on:
- Provider configuration (Azure, OpenAI, Anthropic, Google, etc.)
- Server and agent setup
- Writing assertions (20+ types)
- Template helpers (random values, timestamps, faker)
- Best practices for reliable benchmarks

## Installation

### Option 1: Project Skills (Recommended)
Copy the skill to your project's `.github/skills/` folder:

```bash
# From your project root
mkdir -p .github/skills
cp -r /path/to/agent-benchmark/skills/agent-benchmark .github/skills/
```

The skill will automatically be available when working in that project.

### Option 2: Personal Skills
Copy to your personal skills folder for use across all projects:

**Windows:**
```powershell
Copy-Item -Recurse .\skills\agent-benchmark "$env:USERPROFILE\.copilot\skills\"
```

**Linux/macOS:**
```bash
cp -r ./skills/agent-benchmark ~/.copilot/skills/
```

## Usage

Once installed, AI assistants will automatically use the skill when you ask about:
- Writing agent-benchmark tests
- Configuring providers, servers, or agents
- Using assertions or templates
- Best practices for AI agent testing

### Example Prompts
- "Help me write a test configuration for testing an agent with Azure OpenAI"
- "What assertions should I use to verify the agent created a file?"
- "How do I configure rate limiting for my provider?"

## Requirements

- VS Code with GitHub Copilot (Insiders recommended for Agent Skills support)
- Enable `chat.useAgentSkills` setting in VS Code

## Skill Structure

```
agent-benchmark/
├── SKILL.md              # Main skill instructions
└── references/           # Detailed reference docs
    ├── providers.md      # Provider configuration
    ├── assertions.md     # All assertion types
    ├── templates.md      # Template helpers
    └── best-practices.md # Tips and patterns
```
