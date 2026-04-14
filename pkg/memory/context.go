package memory

import (
	"fmt"
	"strings"
)

// Context is the structured output of a recall query.
// Rendered into the agent's system prompt before each LLM call.
type Context struct {
	WakeUp   string      // identity snapshot — always included
	Episodes []*Episode  // relevant session summaries
	Facts    []*Relation // current Nexus facts for relevant entities
	Patterns []*Pattern  // applicable behavioral patterns
}

// Render formats MemoryContext into a prompt-injectable string.
// Returns empty string if there is nothing meaningful to inject.
func (mc *Context) Render() string {
	if mc == nil {
		return ""
	}
	var b strings.Builder

	if mc.WakeUp != "" {
		b.WriteString("## Context\n")
		b.WriteString(mc.WakeUp)
		b.WriteString("\n\n")
	}

	if len(mc.Facts) > 0 {
		b.WriteString("## Current Facts\n")
		for _, f := range mc.Facts {
			if f.ObjectLiteral != nil {
				fmt.Fprintf(&b, "- %s %s %s\n", f.SubjectID, f.Predicate, *f.ObjectLiteral)
			} else if f.ObjectID != nil {
				fmt.Fprintf(&b, "- %s %s %s\n", f.SubjectID, f.Predicate, *f.ObjectID)
			}
		}
		b.WriteString("\n")
	}

	if len(mc.Episodes) > 0 {
		b.WriteString("## Relevant History\n")
		for _, ep := range mc.Episodes {
			fmt.Fprintf(&b, "[%s] %s\n\n",
				ep.StartedAt.Format("Jan 2 3:04PM"),
				ep.Summary,
			)
		}
	}

	if len(mc.Patterns) > 0 {
		b.WriteString("## Behavioral Patterns\n")
		for _, p := range mc.Patterns {
			fmt.Fprintf(&b, "- %s\n", p.Description)
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
