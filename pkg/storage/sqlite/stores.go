package sqlite

import (
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

func NewReminderStore(path string) (storage.ReminderStore, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "reminders"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewCalendarStore(path string) (storage.CalendarStore, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "calendar"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewNotesStore(path string) (storage.NoteStore, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "notes"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewTasksStore(path string) (storage.TaskStore, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "tasks"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}
