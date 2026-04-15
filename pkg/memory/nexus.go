package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// nexusResolver handles entity deduplication and relation writing.
type nexusResolver struct {
	store NexusStore
	llm   llm.Client
}

func newNexusResolver(store NexusStore, llm llm.Client) *nexusResolver {
	return &nexusResolver{store: store, llm: llm}
}

type NexusInput struct {
	Entities  []extractedEntity
	Relations []extractedRelation
}

// ingestIntoNexus processes the entities, relations, decisions, and
// preferences from a session extraction and writes them into NexusStore.
//
// Entity deduplication is batched: one FTS lookup per entity (cheap SQL),
// then a SINGLE LLM call to confirm all candidate matches at once.
func (r *nexusResolver) ingest(ctx context.Context, episodeID string, input NexusInput) {
	nameToID := r.resolveEntities(ctx, input.Entities, episodeID)

	entityIDs := make([]string, 0, len(nameToID))
	for _, id := range nameToID {
		entityIDs = append(entityIDs, id)
	}
	if len(entityIDs) > 0 {
		if err := r.store.LinkEpisodeEntities(ctx, episodeID, entityIDs); err != nil {
			logger.Log.Warn("nexus: link episode entities failed", "err", err)
		}
	}

	for _, rel := range input.Relations {
		r.ingestRelation(ctx, rel, episodeID, nameToID)
	}
}

func (nr *nexusResolver) ingestRelation(ctx context.Context, rel extractedRelation, episodeID string, nameToID map[string]string) {
	subjectID, ok := nameToID[rel.Subject]
	if !ok {
		logger.Log.Warn("nexus: skipping relation — subject not resolved",
			"subject", rel.Subject, "predicate", rel.Predicate)
		return
	}

	r := &Relation{
		ID:              newID(),
		SubjectID:       subjectID,
		Predicate:       rel.Predicate,
		Confidence:      1.0,
		ValidFrom:       dbtypes.DBTime{Time: time.Now().UTC()},
		SourceEpisodeID: &episodeID,
	}

	// Determine if object is a known entity or a literal.
	if objectID, ok := nameToID[rel.Object]; ok {
		r.ObjectID = &objectID
	} else {
		r.ObjectLiteral = &rel.Object
	}

	if !rel.StillTrue {
		now := dbtypes.NullDBTime{Time: time.Now().UTC(), Valid: true}
		r.ValidUntil = now
	}

	if err := nr.store.SaveRelation(ctx, r); err != nil {
		logger.Log.Warn("nexus: save relation failed", "err", err)
	}
}

type slot struct {
	extracted  extractedEntity
	candidates []*Entity
}

// dedupSlot is a compacted view of slots that actually need LLM confirmation.
type dedupSlot struct {
	slotIndex  int // index back into the original slots slice
	name       string
	candidates []*Entity
}

// resolveEntities deduplicates all extracted entities in a single batched LLM
// call, then upserts new ones. Returns a name→entityID map for relation wiring.
//
// Steps:
//  1. FTS lookup for every extracted entity — N cheap SQL queries, no LLM
//  2. One LLM call covering all entities that have candidates
//  3. Confirmed matches → alias write; unmatched → UpsertEntity
func (nr *nexusResolver) resolveEntities(ctx context.Context, entities []extractedEntity, sourceEpisodeID string) map[string]string {
	if len(entities) == 0 {
		return nil
	}

	// Step 1 — FTS lookups (SQL only, no LLM).
	slots := make([]slot, 0, len(entities))
	for _, ee := range entities {
		candidates, err := nr.store.FindSimilarEntities(ctx, ee.Name)
		if err != nil {
			logger.Log.Warn("nexus: FTS lookup failed", "entity", ee.Name, "err", err)
		}
		slots = append(slots, slot{ee, candidates})
	}

	// Step 2 — single batched LLM call for all slots that have candidates.
	confirmed := nr.batchConfirmMatches(ctx, slots)
	// confirmed[i] is the matched *Entity for slots[i], or nil.

	// Step 3 — alias writes or inserts.
	nameToID := make(map[string]string, len(slots))
	for i, sl := range slots {
		if match := confirmed[i]; match != nil {
			if err := nr.writeAlias(ctx, match.ID, sl.extracted.Name, sourceEpisodeID); err != nil {
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
			CreatedAt:     dbtypes.DBTime{Time: time.Now().UTC()},
		}
		saved, err := nr.store.UpsertEntity(ctx, entity)
		if err != nil {
			logger.Log.Warn("nexus: upsert entity failed", "entity", sl.extracted.Name, "err", err)
			continue
		}
		nameToID[sl.extracted.Name] = saved.ID
	}

	return nameToID
}

// writeAlias inserts a name variant into entity_aliases so it is searchable
// by future FTS dedup passes. Duplicate pairs are silently ignored.
func (nr *nexusResolver) writeAlias(ctx context.Context, entityID, alias, sourceEpisodeID string) error {
	return nr.store.AddAlias(ctx, entityID, alias, sourceEpisodeID)
}

// batchConfirmMatches sends ONE LLM call for all entities that have FTS
// candidates. Returns a slice parallel to slots: confirmed[i] is the matched
// entity for slots[i], or nil if no match or no candidates.
func (nr *nexusResolver) batchConfirmMatches(ctx context.Context, slots []slot) []*Entity {
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
	raw, err := callLLM(ctx, nr.llm, entityDeduplicationSystemPrompt, prompt)
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
// Requested response format: {"0": 1, "3": -1, "5": 0}
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

const entityDeduplicationSystemPrompt = `You are a named-entity deduplication assistant.
Determine whether extracted entity names refer to the same real-world entities as known candidates.
Reply with a single JSON object mapping entity numbers (string keys) to candidate indices (integer values).
Use -1 when there is no match. No explanation. No markdown.`
