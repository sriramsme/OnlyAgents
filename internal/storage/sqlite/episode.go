package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// ── row struct ────────────────────────────────────────────────────────────────

type episodeRow struct {
	ID         string         `db:"id"`
	Scope      string         `db:"scope"`
	Summary    string         `db:"summary"`
	Embedding  []byte         `db:"embedding"`
	Importance float32        `db:"importance"`
	StartedAt  dbtypes.DBTime `db:"started_at"`
	EndedAt    dbtypes.DBTime `db:"ended_at"`
	CreatedAt  dbtypes.DBTime `db:"created_at"`
}

func toEpisodeRow(e *memory.Episode) episodeRow {
	return episodeRow{
		ID:         e.ID,
		Scope:      string(e.Scope),
		Summary:    e.Summary,
		Embedding:  encodeEmbedding(e.Embedding),
		Importance: e.Importance,
		StartedAt:  e.StartedAt,
		EndedAt:    e.EndedAt,
		CreatedAt:  e.CreatedAt,
	}
}

func (r episodeRow) toDomain() *memory.Episode {
	return &memory.Episode{
		ID:         r.ID,
		Scope:      memory.EpisodeScope(r.Scope),
		Summary:    r.Summary,
		Embedding:  decodeEmbedding(r.Embedding),
		Importance: r.Importance,
		StartedAt:  r.StartedAt,
		EndedAt:    r.EndedAt,
		CreatedAt:  r.CreatedAt,
	}
}

// ── EpisodeStore implementation ───────────────────────────────────────────────

func (d *DB) SaveEpisode(ctx context.Context, e *memory.Episode) error {
	row := toEpisodeRow(e)
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO episodes (id, scope, summary, embedding, importance, started_at, ended_at, created_at)
		VALUES (:id, :scope, :summary, :embedding, :importance, :started_at, :ended_at, :created_at)
		ON CONFLICT(id) DO UPDATE SET
			summary    = excluded.summary,
			embedding  = excluded.embedding,
			importance = excluded.importance,
			ended_at   = excluded.ended_at
	`, row)
	return wrap(err, "save episode")
}

func (d *DB) GetEpisode(ctx context.Context, id string) (*memory.Episode, error) {
	var row episodeRow
	err := d.db.GetContext(ctx, &row, `SELECT * FROM episodes WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("episode %s not found", id)
		}
		return nil, wrap(err, "get episode")
	}
	return row.toDomain(), nil
}

// SearchEpisodes runs vector search when an embedding is provided, otherwise
// falls back to scope/time filters ordered by importance.
func (d *DB) SearchEpisodes(ctx context.Context, q memory.EpisodeQuery) ([]*memory.Episode, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}

	if len(q.Embedding) > 0 {
		return d.searchEpisodesByVector(ctx, q, limit)
	}
	return d.searchEpisodesByFilter(ctx, q, limit)
}

// searchEpisodesByVector loads candidates (optionally pre-filtered by scope/time),
// computes cosine similarity in Go, and returns the top-limit results.
func (d *DB) searchEpisodesByVector(ctx context.Context, q memory.EpisodeQuery, limit int) ([]*memory.Episode, error) {
	where, args := episodeFilterClauses(q)

	query := `SELECT * FROM episodes`
	clauses := where

	// Always require embedding
	clauses = append(clauses, "embedding IS NOT NULL")

	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	var rows []episodeRow
	if err := d.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, wrap(err, "search episodes (vector load)")
	}

	type scored struct {
		row   episodeRow
		score float32
	}
	results := make([]scored, 0, len(rows))
	for _, r := range rows {
		sim := cosine(q.Embedding, decodeEmbedding(r.Embedding))
		results = append(results, scored{r, sim})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	episodes := make([]*memory.Episode, len(results))
	for i, s := range results {
		episodes[i] = s.row.toDomain()
	}
	return episodes, nil
}

// searchEpisodesByFilter is the FTS-less fallback: filter by scope/time,
// order by importance descending.
func (d *DB) searchEpisodesByFilter(ctx context.Context, q memory.EpisodeQuery, limit int) ([]*memory.Episode, error) {
	where, args := episodeFilterClauses(q)

	query := `SELECT * FROM episodes`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY importance DESC, started_at DESC LIMIT ?"
	args = append(args, limit)

	var rows []episodeRow
	if err := d.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, wrap(err, "search episodes (filter)")
	}
	return episodeRowsToDomain(rows), nil
}

func (d *DB) GetEpisodesByScope(ctx context.Context, scope memory.EpisodeScope, from, to time.Time) ([]*memory.Episode, error) {
	var rows []episodeRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM episodes
		WHERE scope = ? AND started_at >= ? AND ended_at <= ?
		ORDER BY started_at ASC
	`, string(scope), dbtypes.DBTime{Time: from}, dbtypes.DBTime{Time: to})
	if err != nil {
		return nil, wrap(err, "get episodes by scope")
	}
	return episodeRowsToDomain(rows), nil
}

func (d *DB) PruneEpisodes(ctx context.Context, before time.Time, maxImportance float32) (int, error) {
	result, err := d.db.ExecContext(ctx, `
		DELETE FROM episodes
		WHERE ended_at < ? AND importance <= ?
	`, dbtypes.DBTime{Time: before}, maxImportance)
	if err != nil {
		return 0, wrap(err, "prune episodes")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, wrap(err, "prune episodes")
	}
	return int(n), nil
}

func (d *DB) LastSessionEpisodeEndedAt(ctx context.Context) (time.Time, error) {
	var lastEnd dbtypes.NullDBTime

	err := d.db.GetContext(ctx, &lastEnd, `
		SELECT MAX(ended_at) FROM episodes
		WHERE scope = ?
	`, string(memory.ScopeSession))
	if err != nil {
		return time.Time{}, wrap(err, "last session episode ended at")
	}

	if !lastEnd.Valid {
		return time.Unix(0, 0), nil // 1970-01-01
	}

	return lastEnd.Time, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// episodeFilterClauses builds WHERE clause fragments from an EpisodeQuery,
// excluding the embedding (handled separately).
func episodeFilterClauses(q memory.EpisodeQuery) (clauses []string, args []any) {
	if q.Scope != nil {
		clauses = append(clauses, "scope = ?")
		args = append(args, string(*q.Scope))
	}
	if q.From != nil {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, *q.From)
	}
	if q.To != nil {
		clauses = append(clauses, "ended_at <= ?")
		args = append(args, *q.To)
	}
	return
}

func episodeRowsToDomain(rows []episodeRow) []*memory.Episode {
	out := make([]*memory.Episode, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out
}
