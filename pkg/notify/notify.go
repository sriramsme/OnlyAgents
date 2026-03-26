package notify

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type Notifier struct {
	store storage.Storage
	loc   *time.Location
	bus   chan<- core.Event
}

func New(store storage.Storage, bus chan<- core.Event, tz string) (*Notifier, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	return &Notifier{store: store, loc: loc, bus: bus}, nil
}

func (n *Notifier) RegisterJobs(s *scheduler.Scheduler) {
	s.Register(&reminderNotifierJob{store: n.store, bus: n.bus})
	s.Register(&dailyDigestJob{store: n.store, bus: n.bus, loc: n.loc})
}
