package sqlite

import (
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// DB is the SQLite-backed implementation of storage.Storage.
// Methods are split across conversations.go, memory.go, and productivity.go —
// all in this package, so *DB satisfies the full interface.
type DB struct {
	db *sqlx.DB
}

// New opens (or creates) the SQLite database at path, applies pending
// migrations, and returns a ready-to-use DB.
func New(path string) (*DB, error) {
	// _loc=auto: driver parses stored time strings respecting timezone info.
	// _busy_timeout=5000: wait up to 5 s before returning SQLITE_BUSY.
	dsn := fmt.Sprintf("file:%s?_loc=auto&_busy_timeout=5000", path)

	sqlxDB, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", path, err)
	}

	// SQLite supports only one concurrent writer; cap connections to avoid
	// "database is locked" errors under concurrent reads + occasional writes.
	sqlxDB.SetMaxOpenConns(1)

	store := &DB{db: sqlxDB}

	if err := store.applyPragmas(); err != nil {
		closeErr := sqlxDB.Close()
		return nil, fmt.Errorf("storage: pragmas: %w", errors.Join(err, closeErr))
	}

	if err := RunMigrations(sqlxDB); err != nil {
		closeErr := sqlxDB.Close()
		return nil, fmt.Errorf("storage: migrations: %w", errors.Join(err, closeErr))
	}

	logger.Log.Info("storage: SQLite ready", "path", path)
	return store, nil
}

func (d *DB) applyPragmas() error {
	for _, p := range []string{
		"PRAGMA journal_mode=WAL", // WAL gives better concurrent read perf
		"PRAGMA foreign_keys=ON",  // enforce FK constraints
	} {
		if _, err := d.db.Exec(p); err != nil {
			return fmt.Errorf("storage: pragma %q: %w", p, err)
		}
	}
	return nil
}

// Close releases the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// wrap adds a consistent "storage: <op>:" prefix to errors and returns nil
// when err is nil. Used by all store method implementations.
func wrap(err error, op string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage: %s: %w", op, err)
}
