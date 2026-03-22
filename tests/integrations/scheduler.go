package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// TestScheduler_EndToEnd verifies the full path:
//
//	user request → submitCronJob → RegisterDynamic → fires → event on bus
//
// Uses "@every 1s" so we don't wait for a real cron window.
func TestScheduler_EndToEnd(t *testing.T) {
	bus := make(chan core.Event, 10)
	sched := scheduler.New(bus)

	// Simulate a cron job created by the exec agent for a single-agent recurring task.
	innerEvent := core.Event{
		Type:    core.AgentExecute,
		AgentID: "researcher",
		// CorrelationID intentionally left blank — scheduler assigns a fresh one.
	}
	eventJSON, err := json.Marshal(innerEvent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	job := &storage.CronJob{
		ID:           uuid.NewString(),
		Name:         "daily newspaper",
		Schedule:     "@every 1s",
		Enabled:      true,
		EventType:    string(core.AgentExecute),
		EventPayload: string(eventJSON),
		CreatedAt:    storage.DBTime{Time: time.Now()},
		UpdatedAt:    storage.DBTime{Time: time.Now()},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sched.RegisterDynamic(job)
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := sched.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	// Collect two fires to confirm the job repeats, not just fires once.
	var received []core.Event
	for len(received) < 2 {
		select {
		case evt := <-bus:
			received = append(received, evt)
		case <-ctx.Done():
			t.Fatalf("only received %d fires before timeout, wanted 2", len(received))
		}
	}

	// Both events must be AgentExecute routed to the researcher.
	for i, evt := range received {
		if evt.Type != core.AgentExecute {
			t.Errorf("fire %d: expected AgentExecute, got %s", i, evt.Type)
		}
		if evt.AgentID != "researcher" {
			t.Errorf("fire %d: expected agent_id 'researcher', got %s", i, evt.AgentID)
		}
		if evt.CorrelationID == "" {
			t.Errorf("fire %d: expected a fresh correlation ID", i)
		}
	}

	// Each fire must have a unique correlation ID.
	if received[0].CorrelationID == received[1].CorrelationID {
		t.Error("each fire should have a distinct correlation ID")
	}
}

// TestScheduler_WorkflowFire verifies workflow events are re-emitted correctly.
func TestScheduler_WorkflowFire(t *testing.T) {
	bus := make(chan core.Event, 10)
	sched := scheduler.New(bus)

	wfPayload := workflow.WorkflowPayload{
		Workflow: workflow.WorkflowDefinition{
			ID:   uuid.NewString(),
			Name: "weekly report",
		},
	}
	innerEvent := core.Event{
		Type:    core.WorkflowSubmitted,
		AgentID: "executive",
		Payload: wfPayload,
	}
	eventJSON, err := json.Marshal(innerEvent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	job := &storage.CronJob{
		ID:           uuid.NewString(),
		Name:         "weekly report workflow",
		Schedule:     "@every 1s",
		Enabled:      true,
		EventType:    string(core.WorkflowSubmitted),
		EventPayload: string(eventJSON),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sched.RegisterDynamic(job)
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := sched.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	select {
	case evt := <-bus:
		if evt.Type != core.WorkflowSubmitted {
			t.Errorf("expected WorkflowSubmitted, got %s", evt.Type)
		}
	case <-ctx.Done():
		t.Fatal("workflow job never fired")
	}
}
