package sqlite

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations applies any unapplied SQL migration files in order.
// It is safe to call on every startup — already-applied files are skipped.
func RunMigrations(db *sqlx.DB) error {
	// Bootstrap the migrations tracking table.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Collect already-applied versions.
	var applied []string
	if err := db.Select(&applied, `SELECT version FROM schema_migrations ORDER BY version`); err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	done := make(map[string]bool, len(applied))
	for _, v := range applied {
		done[v] = true
	}

	// List and sort migration files.
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") || done[name] {
			continue
		}

		sql, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		if _, err := db.Exec(string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}

		logger.Log.Info("storage: applied migration", "file", name)
	}
	return nil
}
