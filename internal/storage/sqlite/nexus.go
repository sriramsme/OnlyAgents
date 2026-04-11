package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// ── row structs ───────────────────────────────────────────────────────────────

type entityRow struct {
	ID            string         `db:"id"`
	CanonicalName string         `db:"canonical_name"`
	Type          string         `db:"type"`
	CreatedAt     dbtypes.DBTime `db:"created_at"`
}

func toEntityRow(e *memory.Entity) entityRow {
	return entityRow{
		ID:            e.ID,
		CanonicalName: e.CanonicalName,
		Type:          string(e.Type),
		CreatedAt:     dbtypes.DBTime{Time: e.CreatedAt},
	}
}

func (r entityRow) toDomain() *memory.Entity {
	return &memory.Entity{
		ID:            r.ID,
		CanonicalName: r.CanonicalName,
		Type:          memory.EntityType(r.Type),
		CreatedAt:     r.CreatedAt.Time,
	}
}

type relationRow struct {
	ID              string             `db:"id"`
	SubjectID       string             `db:"subject_id"`
	Predicate       string             `db:"predicate"`
	ObjectID        sql.NullString     `db:"object_id"`
	ObjectLiteral   sql.NullString     `db:"object_literal"`
	Confidence      float32            `db:"confidence"`
	ValidFrom       dbtypes.DBTime     `db:"valid_from"`
	ValidUntil      dbtypes.NullDBTime `db:"valid_until"`
	SourceEpisodeID sql.NullString     `db:"source_episode_id"`
	CreatedAt       dbtypes.DBTime     `db:"created_at"`

	SubjectName string `db:"subject_name"`
	ObjectName  string `db:"object_name"`
}

func toRelationRow(r *memory.Relation) relationRow {
	row := relationRow{
		ID:         r.ID,
		SubjectID:  r.SubjectID,
		Predicate:  r.Predicate,
		Confidence: r.Confidence,
		ValidFrom:  dbtypes.DBTime{Time: r.ValidFrom},
		CreatedAt:  dbtypes.DBTime{Time: r.CreatedAt},
	}
	if r.ObjectID != nil {
		row.ObjectID = sql.NullString{String: *r.ObjectID, Valid: true}
	}
	if r.ObjectLiteral != nil {
		row.ObjectLiteral = sql.NullString{String: *r.ObjectLiteral, Valid: true}
	}
	if r.ValidUntil != nil {
		row.ValidUntil = dbtypes.NullDBTime{Time: *r.ValidUntil, Valid: true}
	}
	if r.SourceEpisodeID != nil {
		row.SourceEpisodeID = sql.NullString{String: *r.SourceEpisodeID, Valid: true}
	}
	return row
}

func (r relationRow) toDomain() *memory.Relation {
	rel := &memory.Relation{
		ID:          r.ID,
		SubjectID:   r.SubjectID,
		Predicate:   r.Predicate,
		Confidence:  r.Confidence,
		ValidFrom:   r.ValidFrom.Time,
		CreatedAt:   r.CreatedAt.Time,
		SubjectName: r.SubjectName,
		ObjectName:  r.ObjectName,
	}
	if r.ObjectID.Valid {
		rel.ObjectID = &r.ObjectID.String
	}
	if r.ObjectLiteral.Valid {
		rel.ObjectLiteral = &r.ObjectLiteral.String
	}
	if r.ValidUntil.Valid {
		rel.ValidUntil = &r.ValidUntil.Time
	}
	if r.SourceEpisodeID.Valid {
		rel.SourceEpisodeID = &r.SourceEpisodeID.String
	}
	return rel
}

// ── NexusStore implementation ─────────────────────────────────────────────────

// UpsertEntity inserts the entity if it doesn't exist, otherwise updates
// canonical_name and type. The canonical name is also written as an alias
// so it participates in FTS deduplication searches.
func (d *DB) UpsertEntity(ctx context.Context, e *memory.Entity) (*memory.Entity, error) {
	row := toEntityRow(e)
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO entities (id, canonical_name, type, created_at)
		VALUES (:id, :canonical_name, :type, :created_at)
		ON CONFLICT(id) DO UPDATE SET
			canonical_name = excluded.canonical_name,
			type           = excluded.type
	`, row)
	if err != nil {
		return nil, wrap(err, "upsert entity")
	}

	// Keep canonical name in alias table so FTS covers it.
	_, err = d.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO entity_aliases (entity_id, alias, created_at)
		VALUES (?, ?, ?)
	`, e.ID, e.CanonicalName, dbtypes.DBTime{Time: e.CreatedAt})
	if err != nil {
		return nil, wrap(err, "upsert entity alias (canonical)")
	}

	return e, nil
}

// FindSimilarEntities searches the alias FTS table and falls back to a LIKE
// on canonical_name. Returns deduplicated entity candidates for LLM confirmation.
func (d *DB) FindSimilarEntities(ctx context.Context, name string) ([]*memory.Entity, error) {
	// FTS match on aliases — use MATCH with the sanitised name.
	ftsQuery := sanitiseFTSQuery(name)

	var rows []entityRow

	// FTS path.
	ftsRows, err := d.findEntitiesByAliasFTS(ctx, ftsQuery)
	if err != nil {
		return nil, err
	}
	rows = append(rows, ftsRows...)

	// Direct LIKE fallback on canonical_name to catch what FTS may miss.
	likeRows, err := d.findEntitiesByCanonicalLike(ctx, name)
	if err != nil {
		return nil, err
	}
	rows = append(rows, likeRows...)

	return deduplicateEntityRows(rows), nil
}

func (d *DB) findEntitiesByAliasFTS(ctx context.Context, ftsQuery string) ([]entityRow, error) {
	if ftsQuery == "" {
		return nil, nil
	}
	var rows []entityRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT e.*
		FROM entity_aliases_fts f
		JOIN entity_aliases ea ON f.rowid = ea.rowid
		JOIN entities e ON e.id = ea.entity_id
		WHERE entity_aliases_fts MATCH ?
		LIMIT 10
	`, ftsQuery)
	if err != nil {
		return nil, wrap(err, "find similar entities (FTS)")
	}
	return rows, nil
}

func (d *DB) findEntitiesByCanonicalLike(ctx context.Context, name string) ([]entityRow, error) {
	var rows []entityRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM entities
		WHERE canonical_name LIKE ?
		LIMIT 10
	`, "%"+name+"%")
	if err != nil {
		return nil, wrap(err, "find similar entities (LIKE)")
	}
	return rows, nil
}

func (d *DB) SaveRelation(ctx context.Context, r *memory.Relation) error {
	row := toRelationRow(r)
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO relations
			(id, subject_id, predicate, object_id, object_literal,
			 confidence, valid_from, valid_until, source_episode_id, created_at)
		VALUES
			(:id, :subject_id, :predicate, :object_id, :object_literal,
			 :confidence, :valid_from, :valid_until, :source_episode_id, :created_at)
	`, row)
	return wrap(err, "save relation")
}

// InvalidateRelation sets valid_until on a relation, marking it as no longer true.
func (d *DB) InvalidateRelation(ctx context.Context, id string, endedAt time.Time) error {
	result, err := d.db.ExecContext(ctx, `
		UPDATE relations SET valid_until = ? WHERE id = ? AND valid_until IS NULL
	`, dbtypes.DBTime{Time: endedAt}, id)
	if err != nil {
		return wrap(err, "invalidate relation")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return wrap(err, "invalidate relation")
	}
	if n == 0 {
		return fmt.Errorf("relation %s not found or already invalidated", id)
	}
	return nil
}

// QueryEntity returns relations where the entity is the subject.
// If asOf is nil, only currently-valid relations are returned (valid_until IS NULL).
// If asOf is set, returns relations valid at that point in time.
func (d *DB) QueryEntity(ctx context.Context, entityID string, asOf *time.Time) ([]*memory.Relation, error) {
	var rows []relationRow
	var err error

	if asOf == nil {
		err = d.db.SelectContext(ctx, &rows, `
    SELECT r.*,
        s.canonical_name as subject_name,
        o.canonical_name as object_name
    FROM relations r
    JOIN entities s ON s.id = r.subject_id
    LEFT JOIN entities o ON o.id = r.object_id
    WHERE r.subject_id = ? AND r.valid_until IS NULL
    ORDER BY r.valid_from DESC
`, entityID)
	} else {
		t := dbtypes.DBTime{Time: *asOf}
		err = d.db.SelectContext(ctx, &rows, `
    SELECT r.*,
        s.canonical_name as subject_name,
        o.canonical_name as object_name
    FROM relations r
    JOIN entities s ON s.id = r.subject_id
    LEFT JOIN entities o ON o.id = r.object_id
    WHERE r.subject_id = ?
      AND r.valid_from <= ?
      AND (r.valid_until IS NULL OR r.valid_until > ?)
    ORDER BY r.valid_from DESC
`, entityID, t, t)
	}
	if err != nil {
		return nil, wrap(err, "query entity")
	}
	return relationRowsToDomain(rows), nil
}

// Timeline returns all relations for an entity across all time, ordered
// chronologically. Used for historical inspection.
func (d *DB) Timeline(ctx context.Context, entityID string) ([]*memory.Relation, error) {
	var rows []relationRow
	err := d.db.SelectContext(ctx, &rows, `
		SELECT * FROM relations
		WHERE subject_id = ? OR object_id = ?
		ORDER BY valid_from ASC
	`, entityID, entityID)
	if err != nil {
		return nil, wrap(err, "timeline")
	}
	return relationRowsToDomain(rows), nil
}

// LinkEpisodeEntities writes rows into the episode_entities join table.
// Duplicate pairs are silently ignored.
func (d *DB) LinkEpisodeEntities(ctx context.Context, episodeID string, entityIDs []string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	// Build a single multi-row insert for efficiency.
	placeholders := make([]string, len(entityIDs))
	args := make([]any, 0, len(entityIDs)*2)
	for i, eid := range entityIDs {
		placeholders[i] = "(?, ?)"
		args = append(args, episodeID, eid)
	}
	query := "INSERT OR IGNORE INTO episode_entities (episode_id, entity_id) VALUES " +
		strings.Join(placeholders, ", ")
	_, err := d.db.ExecContext(ctx, query, args...)
	return wrap(err, "link episode entities")
}

func (d *DB) AddAlias(ctx context.Context, entityID, alias, sourceEpisodeID string) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO entity_aliases (entity_id, alias, source_episode_id)
		VALUES (?, ?, ?)
	`, entityID, alias, sourceEpisodeID)
	return wrap(err, "add alias")
}

func (d *DB) GetEpisodeEntityIDs(ctx context.Context, episodeID string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT entity_id FROM episode_entities
		WHERE episode_id = ?
	`, episodeID)
	if err != nil {
		return nil, wrap(err, "get episode entity IDs")
	}
	var out []string
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		if err != nil {
			return nil, wrap(err, "get episode entity IDs")
		}
		out = append(out, id)
	}
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func relationRowsToDomain(rows []relationRow) []*memory.Relation {
	out := make([]*memory.Relation, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out
}

// deduplicateEntityRows returns unique entities by ID, preserving first-seen order.
func deduplicateEntityRows(rows []entityRow) []*memory.Entity {
	seen := make(map[string]struct{}, len(rows))
	out := make([]*memory.Entity, 0, len(rows))
	for _, r := range rows {
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		out = append(out, r.toDomain())
	}
	return out
}

// sanitiseFTSQuery strips characters that would break an FTS5 MATCH expression.
func sanitiseFTSQuery(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
