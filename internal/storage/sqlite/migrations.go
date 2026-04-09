package sqlite

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations applies any unapplied SQL migration files in order.
// It is safe to call on every startup — already-applied files are skipped.
func RunMigrations(db *sqlx.DB, prefixes ...string) error {
	if err := ensureMigrationTable(db); err != nil {
		return err
	}

	applied, err := getAppliedMigrations(db)
	if err != nil {
		return err
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()

		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		if applied[name] {
			continue
		}

		if !matchesPrefix(name, prefixes) {
			continue
		}

		if err := applyMigration(db, name); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(db *sqlx.DB, name string) error {
	sqlBytes, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	if _, err := db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}

	if _, err := db.Exec(
		`INSERT INTO schema_migrations (version) VALUES (?)`,
		name,
	); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}

	return nil
}

func matchesPrefix(name string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	// extract domain: "007_notes.sql" → "notes"
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return false
	}
	domain := parts[1]
	for _, p := range prefixes {
		if domain == p {
			return true
		}
	}
	return false
}

func ensureMigrationTable(db *sqlx.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func getAppliedMigrations(db *sqlx.DB) (map[string]bool, error) {
	var rows []string

	err := db.Select(&rows, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}

	done := make(map[string]bool, len(rows))
	for _, v := range rows {
		done[v] = true
	}

	return done, nil
}
