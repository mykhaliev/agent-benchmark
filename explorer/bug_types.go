package explorer

// BugType classifies the kind of unexpected MCP tool response detected.
type BugType string

const (
	BugTypeSchemaMismatch         BugType = "SCHEMA_MISMATCH"
	BugTypeMalformedJSON          BugType = "MALFORMED_JSON"
	BugTypeStacktraceReturned     BugType = "STACKTRACE_RETURNED"
	BugTypeUnexpectedTextResponse BugType = "UNEXPECTED_TEXT_RESPONSE"
	BugTypeEmptyResponse          BugType = "EMPTY_RESPONSE"
	BugTypeTimeout                BugType = "TIMEOUT"
	BugTypeServerCrash            BugType = "SERVER_CRASH"
)
