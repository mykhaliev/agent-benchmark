# AI Summary

The `ai_summary` feature provides an LLM-generated interpretation of test results, helping you quickly understand patterns, failures, and actionable improvements.

## Configuration

Add `ai_summary` to your test configuration (single test file) or suite configuration:

```yaml
# In a test file (test.yaml)
ai_summary:
  enabled: true
  judge_provider: "$self"  # Use same provider as agents

# In a suite file (suite.yaml)
ai_summary:
  enabled: true
  judge_provider: "gpt-4o"  # Use a specific provider
```

## Architecture: Late-Binding

AI summaries are generated **at report time**, not during test execution. This design has several benefits:

1. **Iterate on prompts**: You can regenerate HTML reports with updated AI summary prompts without re-running expensive tests
2. **Pure test data**: JSON output contains only test results, making it reproducible and cacheable
3. **Flexible models**: Use different models for summary generation on existing results

### How It Works

1. Test execution writes `report.json` with a `test_file` field pointing to the original YAML
2. When generating HTML/MD reports, the tool reads `test_file` from JSON
3. AI summary configuration is loaded from the original YAML file
4. Fresh AI summary is generated and embedded in HTML/MD output

### JSON Structure

```json
{
  "test_file": "path/to/test.yaml",
  "detailed_results": [...],
  "summary": {...}
}
```

**Note:** The JSON output never contains `ai_summary`. It is always generated fresh during HTML/MD report generation.

## Regenerating Reports

You can regenerate HTML reports from existing JSON results:

```bash
# Regenerate HTML with fresh AI summary
agent-benchmark -generate-report results.json -o report.html
```

The tool automatically:
1. Reads `test_file` from the JSON
2. Loads AI summary configuration from that YAML file
3. Generates a fresh summary using the configured provider

This is useful when:
- You want to try different AI summary prompts
- The original AI summary failed due to rate limits
- You want to use a different model for analysis

## Judge Provider Options

The `judge_provider` field specifies which LLM generates the analysis:

| Value | Behavior |
|-------|----------|
| `"$self"` | Uses the same provider as the first agent in the test run |
| `"<provider-name>"` | Uses a specific provider defined in your `providers` section |

### Example with Separate Analysis Provider

```yaml
providers:
  - name: gpt-4o-mini
    type: openai
    model: gpt-4o-mini
  
  - name: gpt-4o
    type: openai
    model: gpt-4o

agents:
  - name: fast-agent
    provider: gpt-4o-mini
    # ... agent config

ai_summary:
  enabled: true
  judge_provider: gpt-4o  # Use a more capable model for analysis
```

## Recommended Models

For best analysis quality, we recommend using capable reasoning models:

| Provider | Recommended Models |
|----------|-------------------|
| OpenAI | `gpt-4o`, `gpt-4.1` |
| Azure OpenAI | `gpt-4o`, `gpt-4.1` |
| Anthropic | `claude-sonnet-4-20250514`, `claude-3-5-sonnet-20241022` |

**Note:** Using `$self` is cost-effective when your agents already use capable models. For runs with cheaper/faster models (e.g., `gpt-4o-mini`), consider specifying a more capable model for analysis.

## Output

The AI summary appears in:

1. **Console output** - Immediately after test completion, before report generation
2. **HTML report** - As a prominent "AI Summary" section at the top of the report

### Analysis Content

The LLM-generated analysis includes:

- **Verdict**: Direct assessment of test outcomes
- **Trade-offs**: What worked vs. what didn't across different agents/tests
- **Notable Observations**: Unexpected behaviors or interesting patterns
- **Failure Analysis**: Root causes and common failure patterns (when applicable)
- **Recommendations**: Actionable improvements for prompts, tools, or test design

## Example Output

```
🤖 AI Summary
────────────────────────────────────────
## Verdict
4 of 5 tests passed (80%). GPT-4.1 shows consistent tool usage 
but struggles with implicit requirements.

## Trade-offs
GPT-4o excels at rapid execution but occasionally skips verification.
GPT-4.1 is thorough but asks for clarification more often.

## Notable Observations
Both models correctly identified the need for window capture before 
recording, demonstrating good tool understanding.

## Failure Analysis
GPT-4.1 failed "create_slicer" because it asked for a slicer name 
instead of letting Excel auto-generate one. Tool documentation 
doesn't explicitly state auto-generation behavior.

## Recommendations
1. Update excel_pivottable tool docs to clarify auto-naming
2. Add `no_clarification_questions` assertion to catch this pattern
────────────────────────────────────────
```

## Cost Considerations

AI summary makes a single LLM call with:
- **Input**: ~2000-5000 tokens (test results summary)
- **Output**: ~200-500 tokens (interpretation)

For typical test runs, expect ~$0.01-$0.03 per summary with GPT-4o pricing.

## Timeout

AI summary has a 90-second timeout. For very large test suites (100+ tests), the summary may be truncated. Consider running analysis on smaller test batches if you hit timeout issues.
