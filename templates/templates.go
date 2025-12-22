package templates

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aymerick/raymond"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
)

const (
	alphanumericChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	alphabeticChars   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	numericChars      = "0123456789"
	hexChars          = "0123456789abcdef"
	symbolChars       = "!@#$%^&*()_+-=[]{}|;:,.<>?"
)

type TemplateEngine struct{}

var (
	templateEngineInstance *TemplateEngine
	templateEngineOnce     sync.Once
)

// NewTemplateEngine returns the singleton instance of TemplateEngine
func NewTemplateEngine() *TemplateEngine {
	templateEngineOnce.Do(func() {
		// Register helpers only once during initialization
		RegisterHelpers()
		templateEngineInstance = &TemplateEngine{}
	})
	return templateEngineInstance
}

// RegisterHelpers registers custom Handlebars helpers
func RegisterHelpers() {
	// Register random value helper
	raymond.RegisterHelper("randomValue", func(options *raymond.Options) string {
		// Get type from hash arguments (default: ALPHANUMERIC)
		randomType := strings.ToUpper(options.HashStr("type"))
		if randomType == "" {
			randomType = "ALPHANUMERIC"
		}

		// Handle UUID separately
		if randomType == "UUID" {
			return uuid.New().String()
		}

		// Get length from hash arguments (default: 10)
		length := 10
		if lengthVal := options.HashProp("length"); lengthVal != nil {
			switch v := lengthVal.(type) {
			case int:
				length = v
			case int64:
				length = int(v)
			case float64:
				length = int(v)
			case string:
				fmt.Sscanf(v, "%d", &length)
			}
		}

		// Get uppercase flag (default: false)
		uppercase := false
		if uppercaseVal := options.HashProp("uppercase"); uppercaseVal != nil {
			uppercase = raymond.IsTrue(uppercaseVal)
		}

		// Generate random string based on type
		var result string
		switch randomType {
		case "ALPHANUMERIC":
			result = generateRandomString(alphanumericChars, length)
		case "ALPHABETIC":
			result = generateRandomString(alphabeticChars, length)
		case "NUMERIC":
			result = generateRandomString(numericChars, length)
		case "HEXADECIMAL":
			result = generateRandomString(hexChars, length)
		case "ALPHANUMERIC_AND_SYMBOLS":
			chars := alphanumericChars + symbolChars
			result = generateRandomString(chars, length)
		default:
			result = generateRandomString(alphanumericChars, length)
		}

		// Apply uppercase if requested
		if uppercase {
			result = strings.ToUpper(result)
		}

		return result
	})
	// Register randomInt helper
	raymond.RegisterHelper("randomInt", func(options *raymond.Options) string {
		// Default bounds
		lower := 0
		upper := 100

		// Get lower bound if specified
		if lowerVal := options.HashProp("lower"); lowerVal != nil {
			lower = toInt(lowerVal)
		}

		// Get upper bound if specified
		if upperVal := options.HashProp("upper"); upperVal != nil {
			upper = toInt(upperVal)
		}

		// Ensure lower <= upper
		if lower > upper {
			lower, upper = upper, lower
		}

		// Generate random integer in range [lower, upper]
		rangeSize := upper - lower + 1
		num, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
		if err != nil {
			return "0"
		}

		result := int(num.Int64()) + lower
		return fmt.Sprintf("%d", result)
	})
	// Register randomDecimal helper
	raymond.RegisterHelper("randomDecimal", func(options *raymond.Options) string {
		// Default bounds
		lower := 0.0
		upper := 100.0

		// Get lower bound if specified
		if lowerVal := options.HashProp("lower"); lowerVal != nil {
			lower = toFloat(lowerVal)
		}

		// Get upper bound if specified
		if upperVal := options.HashProp("upper"); upperVal != nil {
			upper = toFloat(upperVal)
		}

		// Ensure lower <= upper
		if lower > upper {
			lower, upper = upper, lower
		}

		// Generate random decimal in range [lower, upper]
		rangeSize := upper - lower

		// Generate random bytes for precision
		randomBytes := make([]byte, 8)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return "0"
		}

		// Convert to uint64 and normalize to [0, 1]
		var randomUint64 uint64
		for i := 0; i < 8; i++ {
			randomUint64 = (randomUint64 << 8) | uint64(randomBytes[i])
		}

		// Normalize to [0, 1]
		normalized := float64(randomUint64) / float64(^uint64(0))

		// Scale to [lower, upper]
		result := lower + (normalized * rangeSize)

		return fmt.Sprintf("%.2f", result)
	})
	// current timestamp helper
	raymond.RegisterHelper("now", func(options *raymond.Options) string {
		// Start with current time
		now := time.Now().UTC()

		// Apply offset if provided
		if offsetStr := options.HashStr("offset"); offsetStr != "" {
			offset, err := ParseOffset(offsetStr)
			if err == nil {
				now = now.Add(offset)
			}
		}

		// Apply timezone if provided
		if tzStr := options.HashStr("timezone"); tzStr != "" {
			if loc, err := time.LoadLocation(tzStr); err == nil {
				now = now.In(loc)
			}
		}

		// Apply format if provided
		format := options.HashStr("format")
		switch format {
		case "epoch":
			// UNIX epoch time in milliseconds
			return fmt.Sprintf("%d", now.UnixMilli())
		case "unix":
			// UNIX timestamp in seconds
			return fmt.Sprintf("%d", now.Unix())
		case "":
			// Default ISO8601 format
			return now.Format(time.RFC3339)
		default:
			// Custom format - convert Java SimpleDateFormat to Go format
			goFormat := JavaToGoDateFormat(format)
			return now.Format(goFormat)
		}
	})
	// faker helper
	raymond.RegisterHelper("faker", func(key string) string {
		r := gofakeit.New(0)

		parts := strings.Split(key, ".")
		category := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}
		switch category {
		case "Name":
			switch sub {
			case "first_name":
				return r.FirstName()
			case "last_name":
				return r.LastName()
			case "full_name":
				return r.Name()
			case "prefix":
				return r.NamePrefix()
			case "suffix":
				return r.NameSuffix()
			}
		case "Address":
			switch sub {
			case "street":
				return r.Street()
			case "street_name":
				return r.StreetName()
			case "street_number":
				return r.StreetNumber()
			case "city":
				return r.City()
			case "state":
				return r.State()
			case "state_abbrev":
				return r.StateAbr()
			case "country":
				return r.Country()
			case "country_code":
				return r.CountryAbr()
			case "postcode":
				return r.Zip()
			}
		case "Phone":
			switch sub {
			case "number":
				return r.Phone()
			case "number_formatted":
				return r.PhoneFormatted()
			}
		case "Internet":
			switch sub {
			case "email":
				return r.Email()
			case "username":
				return r.Username()
			case "url":
				return r.URL()
			case "ipv4":
				return r.IPv4Address()
			case "ipv6":
				return r.IPv6Address()
			case "mac":
				return r.MacAddress()
			}
		case "Company":
			switch sub {
			case "name":
				return r.Company()
			case "suffix":
				return r.CompanySuffix()
			case "profession":
				return r.JobTitle()
			}
		case "Lorem":
			switch sub {
			case "word":
				return r.Word()
			case "sentence":
				return r.Sentence(5)
			case "paragraph":
				return r.Paragraph(1, 3, 5, " ")
			}
		case "Finance":
			switch sub {
			case "credit_card":
				return r.CreditCardNumber(nil)
			case "currency":
				return r.CurrencyShort()
			}
		case "Misc":
			switch sub {
			case "uuid":
				return r.UUID()
			case "boolean":
				return fmt.Sprintf("%t", r.Bool())
			case "date":
				return r.Date().Format("2006-01-02")
			case "time":
				return r.Date().Format("15:04:05")
			case "timestamp":
				return fmt.Sprintf("%d", r.Date().Unix())
			case "digit":
				return r.Digit()
			}
		}
		return ""
	})
	// cut helper
	raymond.RegisterHelper("cut", func(value interface{}, toRemove interface{}, options *raymond.Options) raymond.SafeString {
		if value == nil {
			return raymond.SafeString("")
		}
		content := raymond.Str(value)
		if content == "" {
			return raymond.SafeString("")
		}

		removal := raymond.Str(toRemove)
		if removal == "" {
			// Nothing to remove
			return raymond.SafeString(content)
		}

		result := strings.ReplaceAll(content, removal, "")
		return raymond.SafeString(result)
	})
	// replace helper
	raymond.RegisterHelper("replace", func(value interface{}, old interface{}, newVal interface{}, options *raymond.Options) raymond.SafeString {
		if value == nil {
			return raymond.SafeString("")
		}

		content := raymond.Str(value)
		if content == "" {
			return raymond.SafeString("")
		}

		oldStr := raymond.Str(old)
		newStr := raymond.Str(newVal)

		if oldStr == "" {
			// Nothing to replace
			return raymond.SafeString(content)
		}

		result := strings.ReplaceAll(content, oldStr, newStr)
		return raymond.SafeString(result)
	})
	// substring helper
	raymond.RegisterHelper("substring", func(value interface{}, options *raymond.Options) raymond.SafeString {
		if value == nil {
			return ""
		}

		content := raymond.Str(value)
		length := len(content)
		if length == 0 {
			return ""
		}

		// Parse start index from hash
		startIndex := 0
		if startVal := options.HashProp("start"); startVal != nil {
			switch v := startVal.(type) {
			case int:
				startIndex = v
			case int64:
				startIndex = int(v)
			case float64:
				startIndex = int(v)
			case string:
				if parsed, err := strconv.Atoi(v); err == nil {
					startIndex = parsed
				}
			}
		}

		// Parse optional end index from hash
		endIndex := length
		if endVal := options.HashProp("end"); endVal != nil {
			switch v := endVal.(type) {
			case int:
				endIndex = v
			case int64:
				endIndex = int(v)
			case float64:
				endIndex = int(v)
			case string:
				if parsed, err := strconv.Atoi(v); err == nil {
					endIndex = parsed
				}
			}
		}

		// Clamp indices
		if startIndex < 0 {
			startIndex = 0
		} else if startIndex > length {
			startIndex = length
		}

		if endIndex < startIndex {
			endIndex = startIndex
		} else if endIndex > length {
			endIndex = length
		}

		return raymond.SafeString(content[startIndex:endIndex])
	})
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(charset string, length int) string {
	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return ""
		}
		result[i] = charset[num.Int64()]
	}

	return string(result)
}

// toInt converts various types to int
func toInt(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var result int
		fmt.Sscanf(v, "%d", &result)
		return result
	default:
		return 0
	}
}

// toFloat converts various types to float64
func toFloat(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		var result float64
		fmt.Sscanf(v, "%f", &result)
		return result
	default:
		return 0.0
	}
}

// ParseOffset parses offset strings like "3 days", "-24 seconds", "1 years"
func ParseOffset(offset string) (time.Duration, error) {
	parts := strings.Fields(strings.TrimSpace(offset))
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid offset format")
	}

	value, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	unit := strings.ToLower(parts[1])
	// Handle both singular and plural
	unit = strings.TrimSuffix(unit, "s")

	switch unit {
	case "second":
		return time.Duration(value) * time.Second, nil
	case "minute":
		return time.Duration(value) * time.Minute, nil
	case "hour":
		return time.Duration(value) * time.Hour, nil
	case "day":
		return time.Duration(value) * 24 * time.Hour, nil
	case "week":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	case "month":
		// Approximate: 30 days
		return time.Duration(value) * 30 * 24 * time.Hour, nil
	case "year":
		// Approximate: 365 days
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown time unit: %s", unit)
	}
}

// JavaToGoDateFormat converts Java SimpleDateFormat to Go time format
func JavaToGoDateFormat(javaFormat string) string {
	// Common Java SimpleDateFormat patterns to Go equivalents
	replacements := map[string]string{
		"yyyy": "2006",
		"yy":   "06",
		"MMMM": "January",
		"MMM":  "Jan",
		"MM":   "01",
		"M":    "1",
		"dd":   "02",
		"d":    "2",
		"HH":   "15",
		"H":    "15", // Go doesn't have single digit 24-hour, use 15
		"hh":   "03",
		"h":    "3",
		"mm":   "04",
		"m":    "4",
		"ss":   "05",
		"s":    "5",
		"SSS":  "000", // milliseconds
		"SS":   "00",
		"S":    "0",
		"a":    "PM",
		"z":    "MST",
		"Z":    "-0700",
		"EEEE": "Monday",
		"EEE":  "Mon",
	}

	result := javaFormat

	// Sort by length (longest first) to avoid replacing substrings incorrectly
	patterns := make([]string, 0, len(replacements))
	for pattern := range replacements {
		patterns = append(patterns, pattern)
	}

	// Simple sort by length
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			if len(patterns[i]) < len(patterns[j]) {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}

	for _, pattern := range patterns {
		result = strings.ReplaceAll(result, pattern, replacements[pattern])
	}

	return result
}
