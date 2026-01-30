package analysis

import (
	"regexp"
	"sort"
)

// ErrorSignature represents a normalized error pattern with metadata.
type ErrorSignature struct {
	Pattern            string   `json:"pattern"`
	Count              int      `json:"count"`
	FirstSeenMs        int64    `json:"first_seen_ms"`
	LastSeenMs         int64    `json:"last_seen_ms"`
	AffectedOperations []string `json:"affected_operations"`
	AffectedTools      []string `json:"affected_tools"`
	SampleError        string   `json:"sample_error"`
}

// ErrorLog represents an error log entry for signature extraction.
type ErrorLog struct {
	TimestampMs int64
	Operation   string
	ToolName    string
	ErrorType   string
}

// Regex patterns for error normalization.
// Order matters: more specific patterns should come before more general ones.
var (
	uuidPattern      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	ipPattern        = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	pathPattern      = regexp.MustCompile(`/[a-zA-Z0-9/_.-]+`)
	numberPattern    = regexp.MustCompile(`\d+`)
)

// NormalizeError normalizes an error message by replacing dynamic values with placeholders.
// The order of replacements is important:
// 1. UUIDs (most specific)
// 2. Timestamps (before numbers consume the digits)
// 3. IP addresses (before numbers consume the digits)
// 4. File paths
// 5. Numbers (most general)
func NormalizeError(msg string) string {
	msg = uuidPattern.ReplaceAllString(msg, "<UUID>")
	msg = timestampPattern.ReplaceAllString(msg, "<TS>")
	msg = ipPattern.ReplaceAllString(msg, "<IP>")
	msg = pathPattern.ReplaceAllString(msg, "<PATH>")
	msg = numberPattern.ReplaceAllString(msg, "<NUM>")
	return msg
}

// signatureData holds intermediate data during signature extraction.
type signatureData struct {
	count       int
	firstSeenMs int64
	lastSeenMs  int64
	operations  map[string]struct{}
	tools       map[string]struct{}
	sampleError string
}

// ExtractSignatures extracts and ranks error signatures from a list of error logs.
// Returns the top N signatures sorted by count descending.
func ExtractSignatures(errors []ErrorLog, topN int) []ErrorSignature {
	if len(errors) == 0 {
		return []ErrorSignature{}
	}

	// Group errors by normalized pattern
	signatures := make(map[string]*signatureData)

	for _, err := range errors {
		if err.ErrorType == "" {
			continue
		}

		pattern := NormalizeError(err.ErrorType)

		sig, ok := signatures[pattern]
		if !ok {
			sig = &signatureData{
				count:       0,
				firstSeenMs: err.TimestampMs,
				lastSeenMs:  err.TimestampMs,
				operations:  make(map[string]struct{}),
				tools:       make(map[string]struct{}),
				sampleError: err.ErrorType,
			}
			signatures[pattern] = sig
		}

		sig.count++

		if err.TimestampMs < sig.firstSeenMs {
			sig.firstSeenMs = err.TimestampMs
		}
		if err.TimestampMs > sig.lastSeenMs {
			sig.lastSeenMs = err.TimestampMs
		}

		if err.Operation != "" {
			sig.operations[err.Operation] = struct{}{}
		}
		if err.ToolName != "" {
			sig.tools[err.ToolName] = struct{}{}
		}
	}

	// Convert to slice for sorting
	result := make([]ErrorSignature, 0, len(signatures))
	for pattern, sig := range signatures {
		operations := make([]string, 0, len(sig.operations))
		for op := range sig.operations {
			operations = append(operations, op)
		}
		sort.Strings(operations)

		tools := make([]string, 0, len(sig.tools))
		for tool := range sig.tools {
			tools = append(tools, tool)
		}
		sort.Strings(tools)

		result = append(result, ErrorSignature{
			Pattern:            pattern,
			Count:              sig.count,
			FirstSeenMs:        sig.firstSeenMs,
			LastSeenMs:         sig.lastSeenMs,
			AffectedOperations: operations,
			AffectedTools:      tools,
			SampleError:        sig.sampleError,
		})
	}

	// Sort by count descending, then by pattern for deterministic ordering
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Pattern < result[j].Pattern
	})

	// Return top N
	if topN > 0 && len(result) > topN {
		result = result[:topN]
	}

	return result
}
