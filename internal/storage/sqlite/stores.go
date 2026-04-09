package sqlite

import (
	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
)

func NewRemindersStore(path string) (reminder.Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "reminders"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewCalendarStore(path string) (calendar.Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "calendar"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewNotesStore(path string) (notes.Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "notes"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func NewTasksStore(path string) (task.Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := RunMigrations(db, "tasks"); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}
