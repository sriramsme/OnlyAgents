package kernel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
)

func (k *Kernel) handleCronJobScheduled(evt core.Event) {
	payload, ok := evt.Payload.(core.CronJobScheduledPayload)
	if !ok {
		k.logger.Error("invalid cron job payload", "correlation_id", evt.CorrelationID)
		return
	}

	// Marshal the inner event to JSON for storage.
	eventJSON, err := json.Marshal(payload.Event)
	if err != nil {
		k.logger.Error("failed to marshal cron event",
			"job", payload.Name,
			"err", err)
		return
	}
	schedule := payload.Schedule
	if !strings.HasPrefix(schedule, "TZ=") && k.user.Identity.Timezone != "" {
		schedule = fmt.Sprintf("TZ=%s %s", k.user.Identity.Timezone, schedule)
	}
	job := &scheduler.CronJob{
		ID:           payload.ID,
		Name:         payload.Name,
		Schedule:     schedule,
		Enabled:      true,
		EventType:    string(payload.Event.Type),
		EventPayload: string(eventJSON),
		CreatedAt:    dbtypes.DBTime{Time: time.Now()},
		UpdatedAt:    dbtypes.DBTime{Time: time.Now()},
	}

	if err := k.store.SaveCronJob(k.ctx, job); err != nil {
		k.logger.Error("failed to save cron job",
			"job", payload.Name,
			"err", err)
		return
	}

	k.scheduler.RegisterDynamic(job)
	k.logger.Info("cron job scheduled",
		"job", payload.Name,
		"schedule", payload.Schedule,
		"event_type", payload.Event.Type)
}
