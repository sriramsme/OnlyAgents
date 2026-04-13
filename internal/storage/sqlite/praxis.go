package sqlite

import (
	"context"
	"sort"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// ── row struct ────────────────────────────────────────────────────────────────

type patternRow struct {
	ID               string             `db:"id"`
	Description      string             `db:"description"`
	Embedding        []byte             `db:"embedding"`
	Confidence       float32            `db:"confidence"`
	ObservationCount int                `db:"observation_count"`
	FirstObservedAt  dbtypes.DBTime     `db:"first_observed_at"`
	LastObservedAt   dbtypes.DBTime     `db:"last_observed_at"`
	LastDecayedAt    dbtypes.NullDBTime `db:"last_decayed_at"`
	CreatedAt        dbtypes.DBTime     `db:"created_at"`
}

func toPatternRow(p *memory.Pattern) patternRow {
	return patternRow{
		ID:               p.ID,
		Description:      p.Description,
		Embedding:        encodeEmbedding(p.Embedding),
		Confidence:       p.Confidence,
		ObservationCount: p.ObservationCount,
		FirstObservedAt:  p.FirstObservedAt,
		LastObservedAt:   p.LastObservedAt,
		CreatedAt:        p.CreatedAt,
	}
}

func (r patternRow) toDomain() *memory.Pattern {
	return &memory.Pattern{
		ID:               r.ID,
		Description:      r.Description,
		Embedding:        decodeEmbedding(r.Embedding),
		Confidence:       r.Confidence,
		ObservationCount: r.ObservationCount,
		FirstObservedAt:  r.FirstObservedAt,
		LastObservedAt:   r.LastObservedAt,
		CreatedAt:        r.CreatedAt,
	}
}

// ── PraxisStore implementation ────────────────────────────────────────────────

func (d *DB) SavePattern(ctx context.Context, p *memory.Pattern) error {
	row := toPatternRow(p)
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO patterns
			(id, description, embedding, confidence, observation_count,
			 first_observed_at, last_observed_at, created_at)
		VALUES
			(:id, :description, :embedding, :confidence, :observation_count,
			 :first_observed_at, :last_observed_at, :created_at)
		ON CONFLICT(id) DO UPDATE SET
			description       = excluded.description,
			embedding         = excluded.embedding,
			confidence        = excluded.confidence,
			observation_count = excluded.observation_count,
			last_observed_at  = excluded.last_observed_at
	`, row)
	return wrap(err, "save pattern")
}

// UpdatePattern sets the confidence and last_observed_at for a known pattern.
// Called by Scribe after weekly extraction marks reinforcement or contradiction.
func (d *DB) UpdatePattern(ctx context.Context, id string, delta float32, lastSeen time.Time) error {
	_, err := d.db.ExecContext(ctx, `
        UPDATE patterns
        SET confidence       = MIN(1.0, MAX(0.0, confidence + ?)),
            observation_count = observation_count + 1,
            last_observed_at  = ?
        WHERE id = ?
    `, delta, dbtypes.DBTime{Time: lastSeen}, id)
	return wrap(err, "update pattern")
}

// SearchPatterns runs cosine similarity when an embedding is provided.
// Falls back to returning all patterns (up to limit) ordered by confidence when
// no embedding is available — the caller can filter further.
func (d *DB) SearchPatterns(ctx context.Context, embedding []float32, limit int) ([]*memory.Pattern, error) {
	if limit <= 0 {
		limit = 20
	}

	if len(embedding) > 0 {
		return d.searchPatternsByVector(ctx, embedding, limit)
	}
	return d.getAllPatternsCapped(ctx, limit)
}

func (d *DB) searchPatternsByVector(ctx context.Context, embedding []float32, limit int) ([]*memory.Pattern, error) {
	var rows []patternRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM patterns WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, wrap(err, "search patterns (vector load)")
	}

	type scored struct {
		row   patternRow
		score float32
	}
	results := make([]scored, 0, len(rows))
	for _, r := range rows {
		sim := cosine(embedding, decodeEmbedding(r.Embedding))
		results = append(results, scored{r, sim})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	out := make([]*memory.Pattern, len(results))
	for i, s := range results {
		out[i] = s.row.toDomain()
	}
	return out, nil
}

func (d *DB) getAllPatternsCapped(ctx context.Context, limit int) ([]*memory.Pattern, error) {
	var rows []patternRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM patterns ORDER BY confidence DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, wrap(err, "search patterns (no embedder fallback)")
	}
	return patternRowsToDomain(rows), nil
}

// GetAllPatterns returns every stored pattern. Used by Scribe during weekly
// extraction to compare the full pattern set against new episode summaries.
func (d *DB) GetAllPatterns(ctx context.Context) ([]*memory.Pattern, error) {
	var rows []patternRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM patterns ORDER BY confidence DESC
	`)
	if err != nil {
		return nil, wrap(err, "get all patterns")
	}
	return patternRowsToDomain(rows), nil
}

// DecayStalePatterns reduces the confidence of patterns that haven't been
// observed since staleBefore by multiplying by decayFactor (e.g. 0.9).
// Sets last_decayed_at to now so the cron job doesn't double-decay in the same run.
func (d *DB) DecayStalePatterns(ctx context.Context, staleBefore time.Time, decayFactor float32) error {
	now := dbtypes.DBTime{Time: time.Now().UTC()}
	_, err := d.db.ExecContext(ctx, `
		UPDATE patterns
		SET confidence     = MAX(0.0, confidence * ?),
		    last_decayed_at = ?
		WHERE last_observed_at < ?
		  AND (last_decayed_at IS NULL OR last_decayed_at < ?)
	`, decayFactor, now, dbtypes.DBTime{Time: staleBefore}, dbtypes.DBTime{Time: staleBefore})
	return wrap(err, "decay stale patterns")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func patternRowsToDomain(rows []patternRow) []*memory.Pattern {
	out := make([]*memory.Pattern, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out
}
