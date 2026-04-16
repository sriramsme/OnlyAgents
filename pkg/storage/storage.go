package storage

import (
	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

type Storage interface {
	conversation.Store
	message.Store
	task.Store
	reminder.Store
	calendar.Store
	notes.Store
	workflow.Store
	scheduler.Store
	memory.EpisodeStore
	memory.NexusStore
	memory.PraxisStore

	Close() error
}
