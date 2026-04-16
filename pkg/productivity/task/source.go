package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

type Source struct{ store Store }

func NewSource(s Store) memory.Source {
	return &Source{store: s}
}

func (s *Source) Name() string { return "tasks" }

func (s *Source) Search(ctx context.Context, query string, _ []float32, limit int) ([]memory.Result, error) {
	all, err := s.store.SearchTasks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("tasks source: %w", err)
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	results := make([]memory.Result, len(all))
	for i, t := range all {
		results[i] = memory.Result{
			Content:    formatTask(t),
			Score:      taskScore(t),
			SourceName: s.Name(),
			Metadata: map[string]any{
				"task_id":    t.ID,
				"status":     t.Status,
				"priority":   t.Priority,
				"project_id": t.ProjectID,
			},
		}
	}
	return results, nil
}

// formatTask renders a task as a compact LLM-readable string.
func formatTask(t *Task) string {
	var sb strings.Builder

	// status badge + priority
	fmt.Fprintf(&sb, "[%s][%s] %s", statusLabel(t.Status), t.Priority, t.Title)

	// due date if set
	if t.DueAt.Valid {
		due := t.DueAt.Time
		if due.Before(time.Now()) {
			fmt.Fprintf(&sb, " (overdue: %s)", due.Format("Jan 2"))
		} else {
			fmt.Fprintf(&sb, " (due %s)", due.Format("Jan 2"))
		}
	}

	// body truncated
	if t.Body != "" {
		body := t.Body
		if len(body) > 200 {
			body = body[:200] + "…"
		}
		sb.WriteString("\n")
		sb.WriteString(body)
	}

	return sb.String()
}

func statusLabel(status string) string {
	switch status {
	case "in_progress":
		return "in progress"
	default:
		return status
	}
}

// taskScore combines priority weight and due date urgency.
// Overdue high-priority tasks score near 1.0; low-priority no-due tasks score 0.3.
func taskScore(t *Task) float32 {
	base := priorityBase(t.Priority)

	if !t.DueAt.Valid {
		return base
	}

	due := t.DueAt.Time
	now := time.Now()

	if due.Before(now) {
		// overdue — boost toward 1.0 proportional to how late
		overdue := now.Sub(due)
		boost := float32(overdue) / float32(7*24*time.Hour) // 1 week = full boost
		if boost > 0.2 {
			boost = 0.2
		}
		return min32(base+boost, 1.0)
	}

	// upcoming — slight boost if within 48h
	until := due.Sub(now)
	if until <= 48*time.Hour {
		return min32(base+0.1, 1.0)
	}
	return base
}

func priorityBase(p string) float32 {
	switch p {
	case "high":
		return 0.8
	case "medium":
		return 0.55
	default:
		return 0.3
	}
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
