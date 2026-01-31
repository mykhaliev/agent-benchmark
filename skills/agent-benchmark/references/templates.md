# Template Helpers Reference

agent-benchmark uses Handlebars-style templates with custom helpers.

## Built-in Variables

### Static Variables (Available Everywhere)
| Variable | Description |
|----------|-------------|
| `{{TEST_DIR}}` | Directory containing the test YAML file |
| `{{TEMP_DIR}}` | System temp directory |
| `{{RUN_ID}}` | Unique UUID for this test run |
| `{{ANY_ENV_VAR}}` | Any environment variable |

### Runtime Variables (Prompts/Assertions Only)
| Variable | Description |
|----------|-------------|
| `{{AGENT_NAME}}` | Current agent name |
| `{{SESSION_NAME}}` | Current session name |
| `{{PROVIDER_NAME}}` | Provider being used |

## Random Values

### randomValue
```yaml
# Alphanumeric (default)
{{randomValue length=10}}  # aB3xY9kL2m

# Alphabetic only
{{randomValue type='ALPHABETIC' length=8}}

# Numeric only
{{randomValue type='NUMERIC' length=6}}

# UUID
{{randomValue type='UUID'}}

# Uppercase
{{randomValue type='ALPHABETIC' length=8 uppercase=true}}
```

Types: `ALPHANUMERIC`, `ALPHABETIC`, `NUMERIC`, `HEXADECIMAL`, `ALPHANUMERIC_AND_SYMBOLS`, `UUID`

### randomInt
```yaml
{{randomInt}}                    # 0-100
{{randomInt lower=1000 upper=9999}}
```

### randomDecimal
```yaml
{{randomDecimal}}                # 0.00-100.00
{{randomDecimal lower=10.5 upper=99.9}}
```

## Timestamps

### now
```yaml
# ISO8601 (default)
{{now}}  # 2024-01-15T14:30:00Z

# Unix epoch (seconds)
{{now format='unix'}}

# Unix epoch (milliseconds)
{{now format='epoch'}}

# Custom format
{{now format='yyyy-MM-dd HH:mm:ss'}}

# With offset
{{now offset='3 days'}}
{{now offset='-24 hours'}}
{{now offset='1 weeks'}}

# Combined
{{now format='yyyy-MM-dd' offset='7 days' timezone='UTC'}}
```

Offset units: `seconds`, `minutes`, `hours`, `days`, `weeks`, `months`, `years`

## Faker Data

### Names
```yaml
{{faker 'Name.first_name'}}   # John
{{faker 'Name.last_name'}}    # Smith
{{faker 'Name.full_name'}}    # John Smith
```

### Addresses
```yaml
{{faker 'Address.street'}}    # 123 Main St
{{faker 'Address.city'}}      # New York
{{faker 'Address.state'}}     # California
{{faker 'Address.postcode'}}  # 12345
```

### Internet
```yaml
{{faker 'Internet.email'}}    # john@example.com
{{faker 'Internet.username'}} # john_doe_123
{{faker 'Internet.url'}}      # https://example.com
{{faker 'Internet.ipv4'}}     # 192.168.1.1
```

### Company
```yaml
{{faker 'Company.name'}}      # Tech Corp
{{faker 'Company.profession'}} # Software Engineer
```

### Lorem
```yaml
{{faker 'Lorem.word'}}        # ipsum
{{faker 'Lorem.sentence'}}    # Lorem ipsum...
{{faker 'Lorem.paragraph'}}   # Full paragraph
```

### Finance
```yaml
{{faker 'Finance.credit_card'}}  # 4532-1234-5678-9010
{{faker 'Finance.currency'}}     # USD
```

### Misc
```yaml
{{faker 'Misc.uuid'}}         # 550e8400-e29b-...
{{faker 'Misc.boolean'}}      # true/false
{{faker 'Misc.date'}}         # 2024-01-15
```

## String Manipulation

### cut
```yaml
{{cut "Hello World" "World"}}  # Hello 
```

### replace
```yaml
{{replace "Hello World" "World" "Universe"}}  # Hello Universe
```

### substring
```yaml
{{substring "Hello World" start=0 end=5}}  # Hello
```

## Example Usage

```yaml
variables:
  filename: "test-{{randomValue length=8}}.txt"
  timestamp: "{{now format='unix'}}"
  user_email: "{{faker 'Internet.email'}}"
  output_path: "{{TEST_DIR}}/results/{{RUN_ID}}"

sessions:
  - name: Test Session
    tests:
      - name: Create timestamped file
        prompt: |
          Create file {{filename}} with timestamp {{timestamp}}
          Save to {{output_path}}
```
