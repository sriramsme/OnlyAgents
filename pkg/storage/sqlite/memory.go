package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── MemoryStore ───────────────────────────────────────────────────────────────

func (d *DB) SaveDailySummary(ctx context.Context, s *storage.DailySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO daily_summaries
			(id, date, summary, key_events, topics, conversation_ids)
		VALUES
			(:id, :date, :summary, :key_events, :topics, :conversation_ids)
		ON CONFLICT(date) DO UPDATE SET
			summary          = excluded.summary,
			key_events       = excluded.key_events,
			topics           = excluded.topics,
			conversation_ids = excluded.conversation_ids
	`, s)
	return wrap(err, "save daily summary")
}

func (d *DB) GetDailySummary(ctx context.Context, date time.Time) (*storage.DailySummary, error) {
	var s storage.DailySummary

	start := date.Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)

	err := d.db.GetContext(ctx, &s, `
		SELECT * FROM daily_summaries
		WHERE date >= ? AND date < ?
	`, dbtypes.DBTime{Time: start}, dbtypes.DBTime{Time: end})
	if err != nil {
		return nil, wrap(err, "get daily summary")
	}
	return &s, nil
}

func (d *DB) GetDailySummaries(ctx context.Context, from, to time.Time) ([]*storage.DailySummary, error) {
	var summaries []*storage.DailySummary

	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM daily_summaries
		WHERE date >= ? AND date < ?
		ORDER BY date ASC
	`, dbtypes.DBTime{Time: from}, dbtypes.DBTime{Time: to})

	return summaries, wrap(err, "get daily summaries")
}

func (d *DB) SaveWeeklySummary(ctx context.Context, s *storage.WeeklySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO weekly_summaries
			(id, week_start, week_end, summary, themes, achievements)
		VALUES
			(:id, :week_start, :week_end, :summary, :themes, :achievements)
		ON CONFLICT(week_start) DO UPDATE SET
			week_end     = excluded.week_end,
			summary      = excluded.summary,
			themes       = excluded.themes,
			achievements = excluded.achievements
	`, s)
	return wrap(err, "save weekly summary")
}

func (d *DB) GetWeeklySummaries(ctx context.Context, from, to time.Time) ([]*storage.WeeklySummary, error) {
	var summaries []*storage.WeeklySummary
	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM weekly_summaries
		WHERE week_start >= ? AND week_start <= ?
		ORDER BY week_start ASC
	`, dbtypes.DBTime{Time: from}, dbtypes.DBTime{Time: to})
	return summaries, wrap(err, "get weekly summaries")
}

func (d *DB) SaveMonthlySummary(ctx context.Context, s *storage.MonthlySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO monthly_summaries
			(id, year, month, summary, highlights, statistics)
		VALUES
			(:id, :year, :month, :summary, :highlights, :statistics)
		ON CONFLICT(year, month) DO UPDATE SET
			summary    = excluded.summary,
			highlights = excluded.highlights,
			statistics = excluded.statistics
	`, s)
	return wrap(err, "save monthly summary")
}

func (d *DB) GetMonthlySummaries(ctx context.Context, year int) ([]*storage.MonthlySummary, error) {
	var summaries []*storage.MonthlySummary
	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM monthly_summaries WHERE year = ? ORDER BY month ASC
	`, year)
	return summaries, wrap(err, "get monthly summaries")
}

func (d *DB) SaveYearlyArchive(ctx context.Context, a *storage.YearlyArchive) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO yearly_archives
			(id, year, summary, major_events, statistics)
		VALUES
			(:id, :year, :summary, :major_events, :statistics)
		ON CONFLICT(year) DO UPDATE SET
			summary      = excluded.summary,
			major_events = excluded.major_events,
			statistics   = excluded.statistics
	`, a)
	return wrap(err, "save yearly archive")
}

func (d *DB) GetYearlyArchive(ctx context.Context, year int) (*storage.YearlyArchive, error) {
	var a storage.YearlyArchive
	err := d.db.GetContext(ctx, &a, `
		SELECT * FROM yearly_archives WHERE year = ?
	`, year)
	if err != nil {
		return nil, wrap(err, "get yearly archive")
	}
	return &a, nil
}

// ── FactStore ─────────────────────────────────────────────────────────────────

func (d *DB) InsertFact(ctx context.Context, fact *storage.Fact) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO facts
			(entity, fact, entity_type, confidence,
			 source_conversation_id, superseded_by, first_seen, last_confirmed)
		VALUES
			(:entity, :fact, :entity_type, :confidence,
			 :source_conversation_id, :superseded_by, :first_seen, :last_confirmed)
	`, fact)

	return wrap(err, "insert fact")
}

func (d *DB) GetFacts(ctx context.Context, entity string) ([]*storage.Fact, error) {
	var facts []*storage.Fact
	err := d.db.SelectContext(ctx, &facts, `
		SELECT * FROM facts
		WHERE entity = ? AND superseded_by = ''
		ORDER BY last_confirmed DESC
	`, entity)
	return facts, wrap(err, "get facts")
}

// SearchFacts uses FTS5 to search across fact text and entity name.
func (d *DB) SearchFacts(ctx context.Context, query string) ([]*storage.Fact, error) {
	var facts []*storage.Fact
	err := d.db.SelectContext(ctx, &facts, `
		SELECT f.* FROM facts f
		INNER JOIN facts_fts ON f.rowid = facts_fts.rowid
		WHERE facts_fts MATCH ? AND f.superseded_by = ''
		ORDER BY rank
	`, query)
	return facts, wrap(err, "search facts")
}

func (d *DB) DeleteFact(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM facts WHERE id = ?`, id)
	return wrap(err, "delete fact")
}

func (d *DB) GetFactByKey(ctx context.Context, entity, fact string) (*storage.Fact, error) {
	var f storage.Fact
	err := d.db.GetContext(ctx, &f, `
        SELECT * FROM facts
        WHERE entity = ? AND fact = ?
    `, entity, fact)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // not found, not an error
	}
	if err != nil {
		return nil, wrap(err, "get fact by key")
	}
	return &f, nil
}

func (d *DB) GetActiveFactsByEntity(ctx context.Context, entity string) ([]*storage.Fact, error) {
	var facts []*storage.Fact
	err := d.db.SelectContext(ctx, &facts, `
		SELECT * FROM facts
		WHERE entity = ? AND superseded_by = ''
		ORDER BY last_confirmed DESC
	`, entity)
	return facts, wrap(err, "get active facts by entity")
}
