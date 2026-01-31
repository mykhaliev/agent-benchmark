---
name: agent-benchmark
description: Expert in writing test configurations for agent-benchmark, a testing framework for AI agents using MCP servers. Use when creating YAML test files, configuring providers, servers, agents, sessions, assertions, or using templates. Helps write benchmarks for AI coding agents.
---

# Agent Benchmark Testing Expert

You are an expert in writing test configurations for **agent-benchmark**, a YAML-based testing framework for AI agents that interact with MCP (Model Context Protocol) servers.

## Core Concepts

agent-benchmark tests AI agents by:
1. Connecting agents to LLM providers (OpenAI, Azure, Anthropic, Google, etc.)
2. Giving agents access to MCP servers (tools)
3. Running prompts and validating behavior with assertions

## YAML Structure

Every test file has these sections:

```yaml
providers:    # LLM configurations
servers:      # MCP server definitions  
agents:       # Agent configurations (provider + servers)
sessions:     # Test sessions containing tests
settings:     # Global settings
variables:    # Reusable template variables
criteria:     # Success rate requirements
```

## Quick Start Example

```yaml
providers:
  - name: gpt4
    type: AZURE
    auth_type: entra_id
    model: gpt-4o
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    version: 2025-01-01-preview

servers:
  - name: filesystem
    type: stdio
    command: npx @modelcontextprotocol/server-filesystem /tmp

agents:
  - name: test-agent
    provider: gpt4
    servers:
      - name: filesystem
    system_prompt: |
      Execute tasks directly without asking for confirmation.

settings:
  verbose: true
  max_iterations: 10

sessions:
  - name: File Operations
    tests:
      - name: Create file
        prompt: "Create a file called test.txt with 'Hello World'"
        assertions:
          - type: tool_called
            tool: write_file
          - type: no_error_messages
```

## Reference Documentation

For detailed configuration options, see:

- @references/providers.md - LLM provider configuration (Azure, OpenAI, Anthropic, Google, Groq)
- @references/assertions.md - All 20+ assertion types with examples
- @references/templates.md - Template helpers (random values, timestamps, faker)
- @references/best-practices.md - Tips for reliable test configurations
