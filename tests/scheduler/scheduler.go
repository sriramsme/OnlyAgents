package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// stubJob is a minimal Job for testing — no catch-up.
type stubJob struct {
	name     string
	schedule string
	ran      atomic.Bool
}

func (j *stubJob) Name() string     { return j.name }
func (j *stubJob) Schedule() string { return j.schedule }
func (j *stubJob) Run(_ context.Context) error {
	j.ran.Store(true)
	return nil
}

// stubCatchUpJob adds CatchUp tracking on top of stubJob.
type stubCatchUpJob struct {
	stubJob
	caughtUp   atomic.Bool
	catchUpErr error
}

func (j *stubCatchUpJob) CatchUp(_ context.Context) error {
	j.caughtUp.Store(true)
	return j.catchUpErr
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRegister_JobsAreStored(t *testing.T) {
	s := scheduler.New(make(chan<- core.Event, 1)) // bus unused in this test
	job := &stubJob{name: "test_job", schedule: "@every 1h"}
	s.Register(job)
	// If Register panics or drops the job, Start will show it.
	// We just assert no panic here; wiring is tested via Start tests.
}

func TestStart_RunsCatchUpBeforeCron(t *testing.T) {
	bus := make(chan core.Event, 10)
	s := scheduler.New(bus)

	job := &stubCatchUpJob{}
	job.name = "catch_up_job"
	job.schedule = "@every 1h" // won't fire in test window
	s.Register(job)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	s.Start(ctx)
	defer s.Stop()

	// CatchUp must have been called synchronously during Start.
	if !job.caughtUp.Load() {
		t.Error("expected CatchUp to be called during Start")
	}
}

func TestStart_NonCatchUpJobSkipsCatchUp(t *testing.T) {
	bus := make(chan core.Event, 10)
	s := scheduler.New(bus)

	job := &stubJob{name: "plain_job", schedule: "@every 1h"}
	s.Register(job)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	s.Start(ctx)
	defer s.Stop()

	// stubJob has no CatchUp — nothing to assert except no panic.
}

func TestStart_CatchUpErrorDoesNotStopOtherJobs(t *testing.T) {
	bus := make(chan core.Event, 10)
	s := scheduler.New(bus)

	failing := &stubCatchUpJob{}
	failing.name = "failing_catch_up"
	failing.schedule = "@every 1h"
	failing.catchUpErr = context.DeadlineExceeded // simulate error

	ok := &stubCatchUpJob{}
	ok.name = "ok_catch_up"
	ok.schedule = "@every 1h"

	s.Register(failing)
	s.Register(ok)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	s.Start(ctx)
	defer s.Stop()

	if !failing.caughtUp.Load() {
		t.Error("failing job's CatchUp should still have been called")
	}
	if !ok.caughtUp.Load() {
		t.Error("ok job's CatchUp should have been called despite previous failure")
	}
}

func TestStop_DoesNotPanic(t *testing.T) {
	s := scheduler.New(make(chan core.Event, 1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.Stop() // should not panic or block
}
