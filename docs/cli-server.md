# CLI Server Documentation

The CLI server type wraps command-line tools as MCP-like servers, allowing you to test CLI-based tools using agent-benchmark.

## Quick Start

```yaml
servers:
  - name: excel-cli
    type: cli
    command: excel-cli
    shell: powershell
    working_dir: "{{TEST_DIR}}"
    tool_prefix: excel
    help_commands:
      - "excel-cli --help"
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `command` | CLI executable to wrap (required) | - |
| `shell` | Shell to run commands in | `powershell` (Windows), `bash` (Unix) |
| `working_dir` | Working directory for commands | Current directory |
| `tool_prefix` | Prefix for generated tool name | `cli` (tool name: `cli_execute`) |
| `help_commands` | Commands to run at startup for CLI help | - |

### Shell Options

Supported shells:
- **Windows:** `powershell`, `pwsh`, `cmd`
- **Unix/Linux/macOS:** `bash`, `sh`, `zsh`

## Help Commands and Auto-Discovery

The `help_commands` option provides CLI help content to the LLM, so it knows how to use the CLI tool. This content is included in the tool description sent to the LLM.

### Basic Usage

```yaml
servers:
  - name: my-cli
    type: cli
    command: my-cli
    help_commands:
      - "my-cli --help"
```

### Auto-Discovery (for CLIs with Subcommands)

When you provide a **single help command**, agent-benchmark automatically discovers subcommands by:

1. Running the main help command (e.g., `my-cli --help`)
2. Parsing the output for a `COMMANDS:` section
3. Running `my-cli <subcommand> --help` for each discovered subcommand
4. Combining all help output into the tool description

This works well for CLIs that follow standard conventions like:

```
COMMANDS:
    session <FILE>    Manage sessions
    range <ACTION>    Work with ranges
    chart <ACTION>    Create and modify charts
```

**Example CLIs that work with auto-discovery:**
- CLIs built with [Spectre.Console.Cli](https://spectreconsole.net/cli/)
- CLIs built with [System.CommandLine](https://learn.microsoft.com/en-us/dotnet/standard/commandline/)
- Most CLIs that output a `COMMANDS:` or `Commands:` section in their help

### Explicit Subcommand Help (for Non-Standard CLIs)

If your CLI doesn't follow the `COMMANDS:` format, explicitly list all help commands:

```yaml
servers:
  - name: custom-cli
    type: cli
    command: custom-cli
    help_commands:
      - "custom-cli help"           # Main help
      - "custom-cli help create"    # Subcommand help
      - "custom-cli help delete"
      - "custom-cli help list"
```

### Limitations

| Limitation | Workaround |
|------------|------------|
| Auto-discovery only works for CLIs with a `COMMANDS:` section | Use explicit `help_commands` array |
| Subcommand discovery assumes `--help` flag | List explicit help commands for CLIs using `-h`, `help`, or `/?` |
| Help commands that fail are silently ignored | Server starts without help content |

## Tool Invocation

The CLI server exposes a single tool (`{prefix}_execute`) that accepts `args`:

```yaml
sessions:
  - name: CLI Tests
    tests:
      - name: List sheets
        prompt: "List all sheets in the workbook"
        assertions:
          - type: tool_called
            tool: excel_execute
          - type: tool_param_equals
            tool: excel_execute
            params:
              args: "sheet list --file workbook.xlsx"
```

The `args` parameter is passed directly to the CLI command:
```
{command} {args}
# e.g., excel-cli sheet list --file workbook.xlsx
```

## CLI-Specific Assertions

Special assertions for validating CLI output:

### cli_exit_code_equals

Check the CLI exit code:

```yaml
assertions:
  - type: cli_exit_code_equals
    tool: excel_execute
    value: "0"  # Success
```

### cli_stdout_contains

Check if stdout contains specific text:

```yaml
assertions:
  - type: cli_stdout_contains
    tool: excel_execute
    value: "Sheet1"
```

### cli_stdout_regex

Match stdout against a regex pattern:

```yaml
assertions:
  - type: cli_stdout_regex
    tool: excel_execute
    pattern: "Created.*successfully"
```

### cli_stderr_contains

Check if stderr contains specific text:

```yaml
assertions:
  - type: cli_stderr_contains
    tool: excel_execute
    value: "Warning:"
```

## Complete Example

```yaml
providers:
  - name: claude
    type: ANTHROPIC
    token: "{{ANTHROPIC_API_KEY}}"
    model: claude-sonnet-4-20250514

servers:
  - name: excel-cli
    type: cli
    command: excel-cli
    shell: powershell
    working_dir: "{{TEST_DIR}}"
    tool_prefix: excel
    help_commands:
      - "excel-cli --help"

agents:
  - name: excel-agent
    provider: claude
    system_prompt: |
      You are an Excel automation agent.
      Use the excel_execute tool to run CLI commands.
      Execute commands sequentially, one at a time.
    servers:
      - name: excel-cli

variables:
  test_file: "{{TEST_DIR}}/test-workbook.xlsx"

sessions:
  - name: Excel CLI Tests
    tests:
      - name: Create workbook
        prompt: "Create a new Excel workbook at {{test_file}}"
        assertions:
          - type: tool_called
            tool: excel_execute
          - type: cli_exit_code_equals
            tool: excel_execute
            value: "0"
          
      - name: Add data
        prompt: "Add 'Hello World' to cell A1 in the workbook"
        assertions:
          - type: tool_called
            tool: excel_execute
          - type: cli_stdout_contains
            tool: excel_execute
            value: "success"
```

## Best Practices

### 1. Use System Prompts for Sequential Execution

LLMs may try to parallelize CLI commands. Use a system prompt to enforce sequential execution:

```yaml
agents:
  - name: cli-agent
    provider: claude
    system_prompt: |
      Execute CLI commands ONE AT A TIME, sequentially.
      Wait for each command to complete before running the next.
      Do not try to run multiple commands in parallel.
```

### 2. Provide Comprehensive Help Content

The more help content the LLM has, the better it can construct correct commands:

```yaml
servers:
  - name: my-cli
    type: cli
    command: my-cli
    help_commands:
      - "my-cli --help"
      - "my-cli create --help"
      - "my-cli update --help"
```

### 3. Use Parameter Aliases in Your CLI

If LLMs consistently guess wrong parameter names, consider adding aliases to your CLI:

```csharp
// Spectre.Console.Cli example
[CommandOption("--source-range|--range <ADDRESS>")]
public string? SourceRange { get; set; }
```

### 4. Test with Multiple LLM Providers

Different LLMs interpret CLI help differently. Test with multiple providers to ensure robustness:

```yaml
providers:
  - name: claude
    type: ANTHROPIC
    # ...
  - name: gemini
    type: GOOGLE
    # ...

agents:
  - name: claude-agent
    provider: claude
    servers: [{ name: my-cli }]
  - name: gemini-agent
    provider: gemini
    servers: [{ name: my-cli }]
```

## Troubleshooting

### LLM uses wrong parameter names

**Problem:** LLM sends `--range` but CLI expects `--source-range`

**Solutions:**
1. Add parameter aliases to your CLI
2. Use explicit help commands showing correct parameter names
3. Include examples in system prompt

### Help content not loaded

**Problem:** Tool description doesn't include CLI help

**Check:**
1. Help command runs successfully: `my-cli --help`
2. Help command doesn't require interactive input
3. Shell is correct for your OS

### Auto-discovery misses subcommands

**Problem:** Some subcommands not discovered

**Cause:** CLI help doesn't use `COMMANDS:` section format

**Solution:** Use explicit `help_commands` array listing all subcommands
