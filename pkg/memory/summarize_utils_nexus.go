package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// ingestIntoNexus processes the entities, relations, decisions, and
// preferences from a session extraction and writes them into NexusStore.
//
// Entity deduplication is batched: one FTS lookup per entity (cheap SQL),
// then a SINGLE LLM call to confirm all candidate matches at once.
func (s *Summarizer) ingestIntoNexus(ctx context.Context, episodeID string, ext sessionExtraction) {
	nameToID := s.resolveEntities(ctx, ext.Entities, episodeID)

	entityIDs := make([]string, 0, len(nameToID))
	for _, id := range nameToID {
		entityIDs = append(entityIDs, id)
	}
	if len(entityIDs) > 0 {
		if err := s.store.LinkEpisodeEntities(ctx, episodeID, entityIDs); err != nil {
			logger.Log.Warn("nexus: link episode entities failed", "err", err)
		}
	}

	for _, er := range ext.Relations {
		rel, ok := buildRelation(er.Subject, er.Predicate, er.Object, er.StillTrue, episodeID, nameToID)
		if !ok {
			logger.Log.Warn("nexus: skipping relation — subject not resolved",
				"subject", er.Subject, "predicate", er.Predicate)
			continue
		}
		if err := s.store.SaveRelation(ctx, rel); err != nil {
			logger.Log.Warn("nexus: save relation failed", "err", err)
		}
	}

	for _, d := range ext.Decisions {
		rel, ok := buildLiteralRelation(d.Entity, "decided", d.Decision, d.Confidence, episodeID, nameToID)
		if !ok {
			logger.Log.Warn("nexus: skipping decision — entity not resolved", "entity", d.Entity)
			continue
		}
		if err := s.store.SaveRelation(ctx, rel); err != nil {
			logger.Log.Warn("nexus: save decision relation failed", "err", err)
		}
	}

	for _, p := range ext.Preferences {
		rel, ok := buildLiteralRelation(p.Who, "prefers", p.Preference, 1.0, episodeID, nameToID)
		if !ok {
			logger.Log.Warn("nexus: skipping preference — entity not resolved", "who", p.Who)
			continue
		}
		if err := s.store.SaveRelation(ctx, rel); err != nil {
			logger.Log.Warn("nexus: save preference relation failed", "err", err)
		}
	}
}

type slot struct {
	extracted  extractedEntity
	candidates []*Entity
}

// resolveEntities deduplicates all extracted entities in a single batched LLM
// call, then upserts new ones. Returns a name→entityID map for relation wiring.
//
// Steps:
//  1. FTS lookup for every extracted entity — N cheap SQL queries, no LLM
//  2. One LLM call covering all entities that have candidates
//  3. Confirmed matches → alias write; unmatched → UpsertEntity
func (s *Summarizer) resolveEntities(ctx context.Context, entities []extractedEntity, sourceEpisodeID string) map[string]string {
	if len(entities) == 0 {
		return nil
	}

	// Step 1 — FTS lookups (SQL only, no LLM).
	slots := make([]slot, 0, len(entities))
	for _, ee := range entities {
		candidates, err := s.store.FindSimilarEntities(ctx, ee.Name)
		if err != nil {
			logger.Log.Warn("nexus: FTS lookup failed", "entity", ee.Name, "err", err)
		}
		slots = append(slots, slot{ee, candidates})
	}

	// Step 2 — single batched LLM call for all slots that have candidates.
	confirmed := s.batchConfirmMatches(ctx, slots)
	// confirmed[i] is the matched *Entity for slots[i], or nil.

	// Step 3 — alias writes or inserts.
	nameToID := make(map[string]string, len(slots))
	for i, sl := range slots {
		if match := confirmed[i]; match != nil {
			if err := s.writeAlias(ctx, match.ID, sl.extracted.Name, sourceEpisodeID); err != nil {
				logger.Log.Warn("nexus: add alias failed",
					"entity_id", match.ID, "alias", sl.extracted.Name, "err", err)
			}
			nameToID[sl.extracted.Name] = match.ID
			continue
		}

		entity := &Entity{
			ID:            newID(),
			CanonicalName: sl.extracted.Name,
			Type:          EntityType(sl.extracted.Type),
			CreatedAt:     time.Now().UTC(),
		}
		saved, err := s.store.UpsertEntity(ctx, entity)
		if err != nil {
			logger.Log.Warn("nexus: upsert entity failed", "entity", sl.extracted.Name, "err", err)
			continue
		}
		nameToID[sl.extracted.Name] = saved.ID
	}

	return nameToID
}

// dedupSlot is a compacted view of slots that actually need LLM confirmation.
type dedupSlot struct {
	slotIndex  int // index back into the original slots slice
	name       string
	candidates []*Entity
}

// batchConfirmMatches sends ONE LLM call for all entities that have FTS
// candidates. Returns a slice parallel to slots: confirmed[i] is the matched
// entity for slots[i], or nil if no match or no candidates.
func (s *Summarizer) batchConfirmMatches(ctx context.Context, slots []slot) []*Entity {
	results := make([]*Entity, len(slots))

	var toConfirm []dedupSlot
	for i, sl := range slots {
		if len(sl.candidates) > 0 {
			toConfirm = append(toConfirm, dedupSlot{i, sl.extracted.Name, sl.candidates})
		}
	}
	if len(toConfirm) == 0 {
		return results
	}

	prompt := buildBatchDedupPrompt(toConfirm)
	raw, err := s.callLLM(ctx, entityDeduplicationSystemPrompt, prompt)
	if err != nil {
		logger.Log.Warn("nexus: batch dedup LLM call failed", "err", err)
		return results
	}

	parsed := parseBatchDedupResponse(raw)
	for _, ds := range toConfirm {
		candidateIdx, ok := parsed[ds.slotIndex]
		if !ok || candidateIdx < 0 || candidateIdx >= len(ds.candidates) {
			continue
		}
		results[ds.slotIndex] = ds.candidates[candidateIdx]
	}
	return results
}

// buildBatchDedupPrompt renders all entities-with-candidates for the model.
//
// Requested response format:
//
//	{"0": 1, "3": -1, "5": 0}
//
// Key = slot index (string), value = 0-based candidate index or -1 for no match.
func buildBatchDedupPrompt(items []dedupSlot) string {
	var b strings.Builder
	b.WriteString("For each numbered entity, identify which candidate (if any) is the same real-world entity.\n\n")

	for _, item := range items {
		fmt.Printf("Entity %d: %q\nCandidates:\n", item.slotIndex, item.name)
		for j, c := range item.candidates {
			fmt.Printf("  [%d] %q (type: %s)\n", j, c.CanonicalName, c.Type)
		}
		b.WriteString("\n")
	}

	b.WriteString("Reply with a single JSON object. Keys are entity numbers (as strings). " +
		"Values are the matching candidate index (0-based), or -1 if none match.\n" +
		"Example: {\"0\": 1, \"3\": -1}\nNo explanation. No markdown.")

	return b.String()
}

// parseBatchDedupResponse unmarshals {"slotIndex": candidateIndex} from the
// model's response. Returns an empty map on any parse failure.
func parseBatchDedupResponse(raw string) map[int]int {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if i := strings.LastIndex(s, "}"); i >= 0 && i < len(s)-1 {
		s = s[:i+1]
	}

	var strMap map[string]int
	if err := json.Unmarshal([]byte(s), &strMap); err != nil {
		logger.Log.Warn("nexus: batch dedup parse failed", "err", err, "raw", raw)
		return nil
	}

	out := make(map[int]int, len(strMap))
	for k, v := range strMap {
		var idx int
		if _, err := fmt.Sscanf(k, "%d", &idx); err == nil {
			out[idx] = v
		}
	}
	return out
}

// writeAlias inserts a name variant into entity_aliases so it is searchable
// by future FTS dedup passes. Duplicate pairs are silently ignored.
func (s *Summarizer) writeAlias(ctx context.Context, entityID, alias, sourceEpisodeID string) error {
	return s.store.AddAlias(ctx, entityID, alias, sourceEpisodeID)
}

const entityDeduplicationSystemPrompt = `You are a named-entity deduplication assistant.
Determine whether extracted entity names refer to the same real-world entities as known candidates.
Reply with a single JSON object mapping entity numbers (string keys) to candidate indices (integer values).
Use -1 when there is no match. No explanation. No markdown.`

// ── relation builders ─────────────────────────────────────────────────────────

// buildRelation constructs a Relation where the object may be another known
// entity (resolved via nameToID) or falls back to an object literal.
func buildRelation(
	subject, predicate, object string,
	isStillTrue bool,
	episodeID string,
	nameToID map[string]string,
) (*Relation, bool) {
	subjectID, ok := nameToID[subject]
	if !ok {
		return nil, false
	}

	rel := &Relation{
		ID:              newID(),
		SubjectID:       subjectID,
		Predicate:       predicate,
		Confidence:      1.0,
		ValidFrom:       time.Now().UTC(),
		SourceEpisodeID: &episodeID,
		CreatedAt:       time.Now().UTC(),
	}

	if !isStillTrue {
		now := time.Now().UTC()
		rel.ValidUntil = &now
	}

	if objectID, found := nameToID[object]; found {
		rel.ObjectID = &objectID
	} else {
		rel.ObjectLiteral = &object
	}

	return rel, true
}

// buildLiteralRelation constructs a Relation whose object is always a string
// literal. Used for decisions and preferences.
func buildLiteralRelation(
	subject, predicate, literal string,
	confidence float32,
	episodeID string,
	nameToID map[string]string,
) (*Relation, bool) {
	subjectID, ok := nameToID[subject]
	if !ok {
		return nil, false
	}

	rel := &Relation{
		ID:              newID(),
		SubjectID:       subjectID,
		Predicate:       predicate,
		ObjectLiteral:   &literal,
		Confidence:      confidence,
		ValidFrom:       time.Now().UTC(),
		SourceEpisodeID: &episodeID,
		CreatedAt:       time.Now().UTC(),
	}
	return rel, true
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
