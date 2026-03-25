package summarizer

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// canonicalEntityTypes is the authoritative set of entity_type values.
// The LLM may return anything; normalizeEntityType maps unknowns to "other".
var canonicalEntityTypes = map[string]bool{
	"person":       true,
	"place":        true,
	"preference":   true,
	"project":      true,
	"organization": true,
	"other":        true,
}

// normalizeEntityType lowercases and trims t, then maps it to the canonical
// set. Unknown values become "other".
func normalizeEntityType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if canonicalEntityTypes[t] {
		return t
	}
	return "other"
}

// clampConfidence ensures c is in [0.0, 1.0].
func clampConfidence(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// estimateTokens returns a rough token count using the 4-chars-per-token
// heuristic. Accurate enough for budget gating; not suitable for billing.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// dayBounds returns the UTC start and end of the calendar day that contains
// date when interpreted in loc. The returned range [from, to) covers exactly
// 24 hours and is suitable for half-open DB range queries.
func dayBounds(date time.Time, loc *time.Location) (from, to time.Time) {
	local := date.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start.UTC(), start.Add(24 * time.Hour).UTC()
}

// uniqueConvIDs returns the deduplicated conversation IDs from msgs, preserving
// first-seen order.
func uniqueConvIDs(msgs []*storage.Message) storage.JSONSlice[string] {
	seen := make(map[string]bool)
	var ids storage.JSONSlice[string]
	for _, m := range msgs {
		if !seen[m.ConversationID] {
			seen[m.ConversationID] = true
			ids = append(ids, m.ConversationID)
		}
	}
	return ids
}

// firstConvID returns the conversation ID of the first message, or "" if msgs
// is empty. Used as best-effort provenance for extracted facts.
func firstConvID(msgs []*storage.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].ConversationID
}

// parseJSON unmarshals raw into v after stripping any markdown code fences the
// LLM occasionally emits despite being instructed not to.
func parseJSON(raw string, v any) error {
	s := strings.TrimSpace(raw)
	// Strip leading non-JSON content (e.g. "```json\n").
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	// Strip trailing non-JSON content (e.g. "\n```").
	if i := strings.LastIndex(s, "}"); i >= 0 && i < len(s)-1 {
		s = s[:i+1]
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("parseJSON: %w (raw: %.200s)", err, raw)
	}
	return nil
}
