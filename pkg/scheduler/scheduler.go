package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Scheduler is the single cron runtime for the whole system.
// System jobs (memory summarization, digest) and user-created jobs both
// run through here.
type Scheduler struct {
	cron *cron.Cron
	jobs []Job
	mu   sync.Mutex
	bus  chan<- core.Event // write-only, no kernel import needed
}

func New(bus chan<- core.Event) *Scheduler {
	return &Scheduler{
		cron: cron.New(),
		bus:  bus,
	}
}

// Register adds a system job. Call before Start.
func (s *Scheduler) Register(job Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
}

// Start runs catch-up for any CatchUpJob, registers all jobs with the cron
// engine, then starts the scheduler. Call once at kernel boot.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	for _, job := range s.jobs {
		if cu, ok := job.(CatchUpJob); ok {
			if err := cu.CatchUp(ctx); err != nil {
				errs = append(errs, fmt.Errorf("catch-up failed for %s: %w", job.Name(), err))
			}
		}

		j := job
		if _, err := s.cron.AddFunc(j.Schedule(), func() {
			if err := j.Run(ctx); err != nil {
				logger.Log.Error("scheduler: job failed", "job", j.Name(), "err", err)
			}
		}); err != nil {
			errs = append(errs, fmt.Errorf("register failed for %s (%s): %w",
				j.Name(), j.Schedule(), err))
		}
	}

	s.cron.Start()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Stop shuts the cron engine down gracefully.
func (s *Scheduler) Stop() error {
	s.cron.Stop()
	logger.Log.Info("scheduler: stopped")
	return nil
}

func (s *Scheduler) RegisterDynamic(job *storage.CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.cron.AddFunc(job.Schedule, func() {
		logger.Log.Info("scheduler: firing cron job", "job", job.Name)
		// emit WorkflowInstantiate event — kernel picks it up,
		// deep-copies the template, calls engine.SubmitWorkflow
		s.onFire(job)
	})
	if err != nil {
		logger.Log.Error("scheduler: failed to register dynamic job", "job", job.Name, "err", err)
	}
}

func (s *Scheduler) onFire(job *storage.CronJob) {
	var evt core.Event
	if err := json.Unmarshal([]byte(job.EventPayload), &evt); err != nil {
		logger.Log.Error("scheduler: failed to deserialize event", "job", job.Name, "err", err)
		return
	}
	evt.Type = core.EventType(job.EventType)
	evt.CorrelationID = uuid.NewString()
	s.bus <- evt
}
