package summarizer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// sessionExtraction is the structured payload returned by the session LLM call.
// The summary field feeds Episode.Summary for the daily rollup chain.
// The entity/relation/decision/preference fields are intended for NexusStore
// ingestion.
type sessionExtraction struct {
	Summary     string              `json:"summary"`
	Importance  float32             `json:"importance"` // 0.0–1.0
	Entities    []extractedEntity   `json:"entities"`
	Relations   []extractedRelation `json:"relations"`
	Decisions   []extractedDecision `json:"decisions"`
	Preferences []extractedPref     `json:"preferences"`
}

type extractedEntity struct {
	Name string `json:"name"`
	Type string `json:"type"` // person | project | tool | concept | decision | preference
}

type extractedRelation struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	StillTrue bool   `json:"is_still_true"`
}

type extractedDecision struct {
	Entity     string  `json:"entity"`
	Decision   string  `json:"decision"`
	Confidence float32 `json:"confidence"`
}

type extractedPref struct {
	Who        string `json:"who"`
	Preference string `json:"preference"`
}

// SummarizeSessions fetches raw messages in [from, to), groups them into
// sessions by idle gap, then runs an LLM extraction pass on each.
// It also fetches the most recent prior session episode to provide continuity.
func (s *Summarizer) SummarizeSessions(ctx context.Context, from, to time.Time) error {
	msgs, err := s.store.GetMessagesBetween(ctx, []string{"user", "assistant"}, from, to)
	if err != nil {
		return fmt.Errorf("session summarizer: fetch messages: %w", err)
	}

	if len(msgs) == 0 {
		return nil
	}

	// Fetch the last session episode before this window for continuity context.
	prior, err := s.lastEpisodeBefore(ctx, ScopeSession, from, 1)
	if err != nil {
		logger.Log.Warn("session summarizer: could not fetch prior session", "err", err)
	}

	sessions := groupIntoSessions(msgs)

	for i, sess := range sessions {
		// Pass the immediately preceding episode as context.
		// For the first session in the batch, that's the store-fetched prior;
		// for subsequent ones, it's the previous session in this same batch
		// (not yet stored, so we use its rendered text inline).
		var prevSummary string
		if i == 0 && len(prior) > 0 {
			prevSummary = prior[0].Summary
		} else if i > 0 {
			// Use the in-memory rendered content of the previous session's
			// messages as a lightweight stand-in — avoids a round-trip.
			prevSummary = renderSession(sessions[i-1])
		}

		if err := s.summarizeAndStoreSession(ctx, sess, prevSummary); err != nil {
			return err
		}
	}

	return nil
}

func (s *Summarizer) summarizeAndStoreSession(ctx context.Context, sess msgSession, prevSummary string) error {
	userPrompt := buildSessionPrompt(sess, prevSummary)

	raw, err := s.callLLM(ctx, sessionSystemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("session summarizer: llm: %w", err)
	}

	var ext sessionExtraction
	if err := parseSessionJSON(raw, &ext); err != nil {
		// Degrade gracefully: store raw text as summary, use heuristic importance.
		logger.Log.Warn("session summarizer: JSON parse failed, degrading to raw text",
			"err", err,
			"session_start", sess.start.Format(time.RFC3339),
		)
		ext.Summary = strings.TrimSpace(raw)
		ext.Importance = computeSessionImportance(sess)
	}

	ep := &Episode{
		ID:         SessionEpisodeID(sess.start),
		Scope:      ScopeSession,
		Summary:    ext.Summary,
		Importance: ext.Importance,
		StartedAt:  sess.start,
		EndedAt:    sess.end,
		CreatedAt:  time.Now(),
	}

	s.ingestIntoNexus(ctx, ep.ID, ext)

	// Note: ingestIntoNexus logs individual failures and never returns an error —
	// Nexus ingestion is best-effort and must not block episode storage.
	// The SaveEpisode call below remains the critical path.
	return s.store.SaveEpisode(ctx, ep)
}

// buildSessionPrompt assembles the user-turn for the session extraction call.
// prevSummary is empty string when there is no prior context.
func buildSessionPrompt(sess msgSession, prevSummary string) string {
	var b strings.Builder

	if prevSummary != "" {
		b.WriteString("PREVIOUS SESSION CONTEXT:\n")
		b.WriteString(strings.TrimSpace(prevSummary))
		b.WriteString("\n\n---\n\n")
	}

	b.WriteString("CURRENT SESSION:\n")
	b.WriteString(renderSession(sess))

	return b.String()
}

// computeSessionImportance is a heuristic fallback used when JSON parsing fails.
func computeSessionImportance(sess msgSession) float32 {
	score := float32(len(sess.messages))
	if len(sess.agents) > 1 {
		score *= 1.2
	}
	if score > 50 {
		score = 50
	}
	return score / 50.0
}

func SessionEpisodeID(t time.Time) string {
	return "session:" + t.UTC().Format(time.RFC3339Nano)
}

// parseSessionJSON strips markdown fences and unmarshals the LLM response.
func parseSessionJSON(raw string, v any) error {
	s := strings.TrimSpace(raw)
	// Strip leading ```json or ``` fences the model occasionally emits.
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if i := strings.LastIndex(s, "}"); i >= 0 && i < len(s)-1 {
		s = s[:i+1]
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("parseSessionJSON: %w (raw: %.200s)", err, raw)
	}
	return nil
}

// sessionSystemPrompt instructs the LLM to extract structured memory from
// a session. The JSON schema is kept explicit so the model stays grounded.
const sessionSystemPrompt = `You are a memory extraction system for OnlyAgents, a personal AI agent runtime.

"user" is always the human. Named roles (executive, productivity_agent, etc.) are AI agents.

If a PREVIOUS SESSION CONTEXT block is provided, use it to understand continuity —
references like "that project" or "what we discussed earlier" should be resolved against it.
Do NOT summarise the previous context itself; only use it to inform the current session.

Extract the following from the CURRENT SESSION:
1. summary      — 3-5 sentences: what was discussed, decided, and left open
2. importance   — float 0.0–1.0 (how significant is this session for long-term memory)
3. entities     — people, projects, tools, concepts mentioned (name + type)
4. relations    — relationships between entities (subject, predicate, object, is_still_true)
5. decisions    — explicit decisions made (entity affected, decision text, confidence 0–1)
6. preferences  — preferences expressed by any participant (who, what preference)

Be conservative. Only extract what is explicitly stated.
Return ONLY a single JSON object matching this schema — no markdown fences, no explanation:

{
  "summary": "string",
  "importance": 0.0,
  "entities": [{"name": "string", "type": "string"}],
  "relations": [{"subject": "string", "predicate": "string", "object": "string", "is_still_true": true}],
  "decisions": [{"entity": "string", "decision": "string", "confidence": 0.0}],
  "preferences": [{"who": "string", "preference": "string"}]
}`
