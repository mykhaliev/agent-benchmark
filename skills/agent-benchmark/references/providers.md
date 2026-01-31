# LLM Provider Configuration

## Provider Types

| Type | Provider | SDK Used |
|------|----------|----------|
| `OPENAI` | OpenAI API | Azure SDK |
| `AZURE` | Azure OpenAI | Azure SDK |
| `ANTHROPIC` | Anthropic Claude | Anthropic SDK |
| `AMAZON-ANTHROPIC` | Bedrock Claude | Anthropic SDK (Bedrock mode) |
| `GOOGLE` | Google AI (Gemini) | Google GenAI SDK |
| `VERTEX` | Vertex AI | Google GenAI SDK |
| `GROQ` | Groq | AWS SDK |

## Azure OpenAI (Recommended for Enterprise)

### With Entra ID (Passwordless)
```yaml
providers:
  - name: azure-gpt4
    type: AZURE
    auth_type: entra_id  # Uses DefaultAzureCredential
    model: gpt-4o
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    version: 2025-01-01-preview
```

### With API Key
```yaml
providers:
  - name: azure-gpt4
    type: AZURE
    auth_type: api_key
    token: "{{AZURE_OPENAI_API_KEY}}"
    model: gpt-4o
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    version: 2025-01-01-preview
```

## OpenAI
```yaml
providers:
  - name: openai-gpt4
    type: OPENAI
    token: "{{OPENAI_API_KEY}}"
    model: gpt-4o
```

## Anthropic Claude
```yaml
providers:
  - name: claude
    type: ANTHROPIC
    token: "{{ANTHROPIC_API_KEY}}"
    model: claude-sonnet-4-20250514
```

## Amazon Bedrock (Claude)
```yaml
providers:
  - name: bedrock-claude
    type: AMAZON-ANTHROPIC
    model: anthropic.claude-3-5-sonnet-20241022-v2:0
    # Uses AWS credentials from environment
```

## Google AI (Gemini)
```yaml
providers:
  - name: gemini
    type: GOOGLE
    token: "{{GOOGLE_API_KEY}}"
    model: gemini-2.0-flash
```

## Vertex AI
```yaml
providers:
  - name: vertex
    type: VERTEX
    project_id: "your-gcp-project"
    location: "us-central1"
    credentials_path: "/path/to/service-account.json"
    model: gemini-2.0-flash
```

## Rate Limiting

Proactively throttle requests to avoid hitting API quotas:

```yaml
providers:
  - name: azure-gpt4
    type: AZURE
    auth_type: entra_id
    model: gpt-4o
    baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
    version: 2025-01-01-preview
    rate_limits:
      tpm: 30000  # Tokens per minute
      rpm: 60     # Requests per minute
    retry:
      retry_on_429: true  # Retry on rate limit errors
      max_retries: 3
```

## Environment Variables

Always use template syntax for secrets:
```yaml
token: "{{OPENAI_API_KEY}}"      # References $OPENAI_API_KEY
baseUrl: "{{AZURE_OPENAI_ENDPOINT}}"
```

Set variables before running:
```bash
export AZURE_OPENAI_ENDPOINT="https://your-resource.openai.azure.com"
agent-benchmark -f tests.yaml
```
