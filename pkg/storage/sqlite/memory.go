package sqlite

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── MemoryStore ───────────────────────────────────────────────────────────────

func (d *DB) SaveDailySummary(ctx context.Context, s *storage.DailySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO daily_summaries
			(id, agent_id, date, summary, key_events, topics, conversation_ids)
		VALUES
			(:id, :agent_id, :date, :summary, :key_events, :topics, :conversation_ids)
		ON CONFLICT(agent_id, date) DO UPDATE SET
			summary          = excluded.summary,
			key_events       = excluded.key_events,
			topics           = excluded.topics,
			conversation_ids = excluded.conversation_ids
	`, s)
	return wrap(err, "save daily summary")
}

func (d *DB) GetDailySummary(ctx context.Context, agentID string, date time.Time) (*storage.DailySummary, error) {
	var s storage.DailySummary
	// date() strips the time component so we match on calendar day.
	err := d.db.GetContext(ctx, &s, `
		SELECT * FROM daily_summaries
		WHERE agent_id = ? AND date(date) = date(?)
	`, agentID, storage.DBTime{Time: date})
	if err != nil {
		return nil, wrap(err, "get daily summary")
	}
	return &s, nil
}

func (d *DB) GetDailySummaries(ctx context.Context, agentID string, from, to time.Time) ([]*storage.DailySummary, error) {
	var summaries []*storage.DailySummary
	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM daily_summaries
		WHERE agent_id = ? AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, agentID, storage.DBTime{Time: from}, storage.DBTime{Time: to})
	return summaries, wrap(err, "get daily summaries")
}

func (d *DB) SaveWeeklySummary(ctx context.Context, s *storage.WeeklySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO weekly_summaries
			(id, agent_id, week_start, week_end, summary, themes, achievements)
		VALUES
			(:id, :agent_id, :week_start, :week_end, :summary, :themes, :achievements)
		ON CONFLICT(agent_id, week_start) DO UPDATE SET
			week_end     = excluded.week_end,
			summary      = excluded.summary,
			themes       = excluded.themes,
			achievements = excluded.achievements
	`, s)
	return wrap(err, "save weekly summary")
}

func (d *DB) GetWeeklySummaries(ctx context.Context, agentID string, from, to time.Time) ([]*storage.WeeklySummary, error) {
	var summaries []*storage.WeeklySummary
	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM weekly_summaries
		WHERE agent_id = ? AND week_start >= ? AND week_start <= ?
		ORDER BY week_start ASC
	`, agentID, storage.DBTime{Time: from}, storage.DBTime{Time: to})
	return summaries, wrap(err, "get weekly summaries")
}

func (d *DB) SaveMonthlySummary(ctx context.Context, s *storage.MonthlySummary) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO monthly_summaries
			(id, agent_id, year, month, summary, highlights, statistics)
		VALUES
			(:id, :agent_id, :year, :month, :summary, :highlights, :statistics)
		ON CONFLICT(agent_id, year, month) DO UPDATE SET
			summary    = excluded.summary,
			highlights = excluded.highlights,
			statistics = excluded.statistics
	`, s)
	return wrap(err, "save monthly summary")
}

func (d *DB) GetMonthlySummaries(ctx context.Context, agentID string, year int) ([]*storage.MonthlySummary, error) {
	var summaries []*storage.MonthlySummary
	err := d.db.SelectContext(ctx, &summaries, `
		SELECT * FROM monthly_summaries WHERE agent_id = ? AND year = ? ORDER BY month ASC
	`, agentID, year)
	return summaries, wrap(err, "get monthly summaries")
}

func (d *DB) SaveYearlyArchive(ctx context.Context, a *storage.YearlyArchive) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO yearly_archives
			(id, agent_id, year, summary, major_events, statistics)
		VALUES
			(:id, :agent_id, :year, :summary, :major_events, :statistics)
		ON CONFLICT(agent_id, year) DO UPDATE SET
			summary      = excluded.summary,
			major_events = excluded.major_events,
			statistics   = excluded.statistics
	`, a)
	return wrap(err, "save yearly archive")
}

func (d *DB) GetYearlyArchive(ctx context.Context, agentID string, year int) (*storage.YearlyArchive, error) {
	var a storage.YearlyArchive
	err := d.db.GetContext(ctx, &a, `
		SELECT * FROM yearly_archives WHERE agent_id = ? AND year = ?
	`, agentID, year)
	if err != nil {
		return nil, wrap(err, "get yearly archive")
	}
	return &a, nil
}

// ── FactStore ─────────────────────────────────────────────────────────────────

func (d *DB) UpsertFact(ctx context.Context, fact *storage.Fact) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO facts
			(id, agent_id, entity, entity_type, fact, confidence,
			 source_conversation_id, superseded_by, first_seen, last_confirmed)
		VALUES
			(:id, :agent_id, :entity, :entity_type, :fact, :confidence,
			 :source_conversation_id, :superseded_by, :first_seen, :last_confirmed)
		ON CONFLICT(id) DO UPDATE SET
			confidence     = excluded.confidence,
			superseded_by  = excluded.superseded_by,
			last_confirmed = excluded.last_confirmed
	`, fact)
	return wrap(err, "upsert fact")
}

func (d *DB) GetFacts(ctx context.Context, agentID string, entity string) ([]*storage.Fact, error) {
	var facts []*storage.Fact
	err := d.db.SelectContext(ctx, &facts, `
		SELECT * FROM facts
		WHERE agent_id = ? AND entity = ? AND superseded_by = ''
		ORDER BY last_confirmed DESC
	`, agentID, entity)
	return facts, wrap(err, "get facts")
}

// SearchFacts uses FTS5 to search across fact text and entity name.
func (d *DB) SearchFacts(ctx context.Context, agentID string, query string) ([]*storage.Fact, error) {
	var facts []*storage.Fact
	err := d.db.SelectContext(ctx, &facts, `
		SELECT f.* FROM facts f
		INNER JOIN facts_fts ON f.rowid = facts_fts.rowid
		WHERE facts_fts MATCH ? AND f.agent_id = ? AND f.superseded_by = ''
		ORDER BY rank
	`, query, agentID)
	return facts, wrap(err, "search facts")
}

func (d *DB) DeleteFact(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM facts WHERE id = ?`, id)
	return wrap(err, "delete fact")
}

// ── AgentStateStore ───────────────────────────────────────────────────────────

func (d *DB) GetAgentState(ctx context.Context, agentID string) (*storage.AgentState, error) {
	var state storage.AgentState
	err := d.db.GetContext(ctx, &state, `SELECT * FROM agent_state WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, wrap(err, "get agent state")
	}
	return &state, nil
}

func (d *DB) SaveAgentState(ctx context.Context, state *storage.AgentState) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO agent_state
			(agent_id, current_conversation_id, context, preferences, goals, last_active)
		VALUES
			(:agent_id, :current_conversation_id, :context, :preferences, :goals, :last_active)
		ON CONFLICT(agent_id) DO UPDATE SET
			current_conversation_id = excluded.current_conversation_id,
			context                 = excluded.context,
			preferences             = excluded.preferences,
			goals                   = excluded.goals,
			last_active             = excluded.last_active
	`, state)
	return wrap(err, "save agent state")
}
