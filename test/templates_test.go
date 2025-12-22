package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/aymerick/raymond"
	"github.com/mykhaliev/agent-benchmark/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var templateEngine = templates.NewTemplateEngine()

func TestRandomValueHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "Default alphanumeric",
			template: `{{randomValue}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 10, "Default length should be 10")
				assert.Regexp(t, `^[a-zA-Z0-9]+$`, result)
			},
		},
		{
			name:     "Custom length",
			template: `{{randomValue length=20}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 20)
			},
		},
		{
			name:     "Alphabetic type",
			template: `{{randomValue type="ALPHABETIC" length=15}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 15)
				assert.Regexp(t, `^[a-zA-Z]+$`, result)
			},
		},
		{
			name:     "Numeric type",
			template: `{{randomValue type="NUMERIC" length=8}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 8)
				assert.Regexp(t, `^[0-9]+$`, result)
			},
		},
		{
			name:     "Hexadecimal type",
			template: `{{randomValue type="HEXADECIMAL" length=16}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 16)
				assert.Regexp(t, `^[0-9a-f]+$`, result)
			},
		},
		{
			name:     "UUID type",
			template: `{{randomValue type="UUID"}}`,
			validate: func(t *testing.T, result string) {
				assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, result)
			},
		},
		{
			name:     "Uppercase flag",
			template: `{{randomValue type="ALPHABETIC" length=10 uppercase=true}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 10)
				assert.Regexp(t, `^[A-Z]+$`, result)
			},
		},
		{
			name:     "Alphanumeric with symbols",
			template: `{{{randomValue type="ALPHANUMERIC_AND_SYMBOLS" length=12}}}`,
			validate: func(t *testing.T, result string) {
				assert.Len(t, result, 12)
				// Should contain at least alphanumeric chars
				assert.Regexp(t, `[a-zA-Z0-9]`, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(nil)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestRandomIntHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "Default range",
			template: `{{randomInt}}`,
			validate: func(t *testing.T, result string) {
				var num int
				_, err := fmt.Sscanf(result, "%d", &num)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, num, 0)
				assert.LessOrEqual(t, num, 100)
			},
		},
		{
			name:     "Custom range",
			template: `{{randomInt lower=10 upper=20}}`,
			validate: func(t *testing.T, result string) {
				var num int
				_, err := fmt.Sscanf(result, "%d", &num)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, num, 10)
				assert.LessOrEqual(t, num, 20)
			},
		},
		{
			name:     "Negative range",
			template: `{{randomInt lower=-50 upper=-10}}`,
			validate: func(t *testing.T, result string) {
				var num int
				_, err := fmt.Sscanf(result, "%d", &num)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, num, -50)
				assert.LessOrEqual(t, num, -10)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(nil)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestRandomDecimalHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "Default range",
			template: `{{randomDecimal}}`,
			validate: func(t *testing.T, result string) {
				var num float64
				_, err := fmt.Sscanf(result, "%f", &num)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, num, 0.0)
				assert.LessOrEqual(t, num, 100.0)
			},
		},
		{
			name:     "Custom range",
			template: `{{randomDecimal lower=1.5 upper=10.5}}`,
			validate: func(t *testing.T, result string) {
				var num float64
				_, err := fmt.Sscanf(result, "%f", &num)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, num, 1.5)
				assert.LessOrEqual(t, num, 10.5)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(nil)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestNowHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "Default ISO8601",
			template: `{{now}}`,
			validate: func(t *testing.T, result string) {
				_, err := time.Parse(time.RFC3339, result)
				assert.NoError(t, err)
			},
		},
		{
			name:     "Unix timestamp",
			template: `{{now format="unix"}}`,
			validate: func(t *testing.T, result string) {
				var timestamp int64
				_, err := fmt.Sscanf(result, "%d", &timestamp)
				require.NoError(t, err)
				assert.Greater(t, timestamp, int64(0))
			},
		},
		{
			name:     "Epoch milliseconds",
			template: `{{now format="epoch"}}`,
			validate: func(t *testing.T, result string) {
				var timestamp int64
				_, err := fmt.Sscanf(result, "%d", &timestamp)
				require.NoError(t, err)
				assert.Greater(t, timestamp, int64(0))
			},
		},
		{
			name:     "Custom format",
			template: `{{now format="yyyy-MM-dd"}}`,
			validate: func(t *testing.T, result string) {
				_, err := time.Parse("2006-01-02", result)
				assert.NoError(t, err)
			},
		},
		{
			name:     "With offset",
			template: `{{now offset="1 days" format="yyyy-MM-dd"}}`,
			validate: func(t *testing.T, result string) {
				parsed, err := time.Parse("2006-01-02", result)
				require.NoError(t, err)

				today := time.Now().Format("2006-01-02")
				todayParsed, _ := time.Parse("2006-01-02", today)

				diff := parsed.Sub(todayParsed)
				assert.InDelta(t, 24*time.Hour, diff, float64(time.Hour))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(nil)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestFakerHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		validate func(t *testing.T, result string)
	}{
		{
			name:     "Name.first_name",
			template: `{{faker "Name.first_name"}}`,
			validate: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
				assert.Regexp(t, `^[A-Za-z]+$`, result)
			},
		},
		{
			name:     "Internet.email",
			template: `{{faker "Internet.email"}}`,
			validate: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
				assert.Contains(t, result, "@")
			},
		},
		{
			name:     "Address.city",
			template: `{{faker "Address.city"}}`,
			validate: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
			},
		},
		{
			name:     "Phone.number",
			template: `{{faker "Phone.number"}}`,
			validate: func(t *testing.T, result string) {
				assert.NotEmpty(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(nil)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestCutHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		context  map[string]string
		expected string
	}{
		{
			name:     "Remove substring",
			template: `{{cut text "World"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hello ",
		},
		{
			name:     "Remove multiple occurrences",
			template: `{{cut text "o"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hell Wrld",
		},
		{
			name:     "Nothing to remove",
			template: `{{cut text "xyz"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hello World",
		},
		{
			name:     "Empty removal",
			template: `{{cut text ""}}`,
			context:  map[string]string{"text": "Hello"},
			expected: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReplaceHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		context  map[string]string
		expected string
	}{
		{
			name:     "Simple replacement",
			template: `{{replace text "World" "Universe"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hello Universe",
		},
		{
			name:     "Multiple replacements",
			template: `{{replace text "o" "0"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hell0 W0rld",
		},
		{
			name:     "No match",
			template: `{{replace text "xyz" "abc"}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstringHelper(t *testing.T) {
	tests := []struct {
		name     string
		template string
		context  map[string]string
		expected string
	}{
		{
			name:     "Start only",
			template: `{{substring text start=6}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "World",
		},
		{
			name:     "Start and end",
			template: `{{substring text start=0 end=5}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "Hello",
		},
		{
			name:     "Middle substring",
			template: `{{substring text start=3 end=8}}`,
			context:  map[string]string{"text": "Hello World"},
			expected: "lo Wo",
		},
		{
			name:     "Out of bounds clamped",
			template: `{{substring text start=0 end=100}}`,
			context:  map[string]string{"text": "Hello"},
			expected: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := raymond.Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Exec(tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseOffset(t *testing.T) {
	tests := []struct {
		name     string
		offset   string
		expected time.Duration
		wantErr  bool
	}{
		{"3 days", "3 days", 3 * 24 * time.Hour, false},
		{"Negative days", "-2 days", -2 * 24 * time.Hour, false},
		{"Hours", "5 hours", 5 * time.Hour, false},
		{"Minutes", "30 minutes", 30 * time.Minute, false},
		{"Seconds", "45 seconds", 45 * time.Second, false},
		{"Singular form", "1 day", 1 * 24 * time.Hour, false},
		{"Invalid format", "invalid", 0, true},
		{"Unknown unit", "5 fortnights", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := templates.ParseOffset(tt.offset)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJavaToGoDateFormat(t *testing.T) {
	tests := []struct {
		name       string
		javaFormat string
		goFormat   string
	}{
		{"Year", "yyyy", "2006"},
		{"Short year", "yy", "06"},
		{"Month number", "MM", "01"},
		{"Day", "dd", "02"},
		{"Hour 24", "HH", "15"},
		{"Hour 12", "hh", "03"},
		{"Minute", "mm", "04"},
		{"Second", "ss", "05"},
		{"Complete format", "yyyy-MM-dd HH:mm:ss", "2006-01-02 15:04:05"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := templates.JavaToGoDateFormat(tt.javaFormat)
			assert.Equal(t, tt.goFormat, result)
		})
	}
}
