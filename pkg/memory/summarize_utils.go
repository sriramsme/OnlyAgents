package memory

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/message"
)

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

// groupIntoSessions partitions msgs into contiguous sessions separated by
// gaps of at least sessionGap. msgs must be ordered by CreatedAt ascending.
func groupIntoSessions(msgs []*message.Message) []msgSession {
	if len(msgs) == 0 {
		return nil
	}

	var sessions []msgSession
	cur := msgSession{
		start:    msgs[0].Timestamp.Time,
		end:      msgs[0].Timestamp.Time,
		messages: []*message.Message{msgs[0]},
	}
	if msgs[0].AgentID != "" {
		cur.agents = []string{msgs[0].AgentID}
	}

	agentSeen := map[string]bool{}
	if msgs[0].AgentID != "" {
		agentSeen[msgs[0].AgentID] = true
	}

	for _, m := range msgs[1:] {
		if m.Timestamp.Sub(cur.end) >= sessionGap {
			sessions = append(sessions, cur)
			cur = msgSession{
				start:    m.Timestamp.Time,
				end:      m.Timestamp.Time,
				messages: []*message.Message{m},
			}
			agentSeen = map[string]bool{}
			if m.AgentID != "" {
				agentSeen[m.AgentID] = true
				cur.agents = []string{m.AgentID}
			}
		} else {
			cur.end = m.Timestamp.Time
			cur.messages = append(cur.messages, m)
			if m.AgentID != "" && !agentSeen[m.AgentID] {
				agentSeen[m.AgentID] = true
				cur.agents = append(cur.agents, m.AgentID)
			}
		}
	}

	return append(sessions, cur)
}
