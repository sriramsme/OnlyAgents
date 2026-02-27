package logger

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var Log = slog.New(slog.NewTextHandler(os.Stdout, nil))

// Timing configuration
var (
	TimingEnabled       = true  // Toggle all timing
	TimingDetailedLLM   = false // Show individual LLM calls (vs aggregated)
	TimingDetailedTools = false // Show individual tool calls (vs aggregated)
)

// Global timing tracker
var Timing = NewTimingTracker()

func Initialize(level string, format string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Shorten timestamp
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("15:04:05"))
				}
			}
			if a.Key == "correlation_id" {
				return slog.Attr{}
			}
			return a
		},
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	Log = slog.New(handler)
	slog.SetDefault(Log)
}

func With(args ...any) *slog.Logger {
	return Log.With(args...)
}

// SetTimingDetail configures timing detail level from env vars
func SetTimingDetail(llmDetailed, toolDetailed bool) {
	TimingDetailedLLM = llmDetailed
	TimingDetailedTools = toolDetailed
}

// ====================
// Timing Tracker
// ====================

type TimingTracker struct {
	mu      sync.RWMutex
	timings map[string]*RequestTiming
}

type RequestTiming struct {
	mu            sync.Mutex
	CorrelationID string
	StartTime     time.Time
	Phases        []PhaseRecord
	phaseStack    []string // Track nested phases
}

type PhaseRecord struct {
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Metadata  map[string]any
	Level     int // Nesting level for display
}

func NewTimingTracker() *TimingTracker {
	t := &TimingTracker{
		timings: make(map[string]*RequestTiming),
	}
	// Auto-cleanup every 5 minutes
	go t.cleanup()
	return t
}

// StartPhase begins timing a phase
func (t *TimingTracker) StartPhase(correlationID, phase string) {
	if !TimingEnabled || correlationID == "" {
		return
	}

	t.mu.Lock()
	rt, exists := t.timings[correlationID]
	if !exists {
		rt = &RequestTiming{
			CorrelationID: correlationID,
			StartTime:     time.Now(),
			Phases:        make([]PhaseRecord, 0, 16),
			phaseStack:    make([]string, 0, 8),
		}
		t.timings[correlationID] = rt
	}
	t.mu.Unlock()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Push to stack for nesting
	rt.phaseStack = append(rt.phaseStack, phase)

	// Record the phase start time
	rt.Phases = append(rt.Phases, PhaseRecord{
		Name:      phase,
		StartTime: time.Now(),
		Level:     len(rt.phaseStack) - 1,
	})
}

// EndPhase ends timing a phase
func (t *TimingTracker) EndPhase(correlationID, phase string) time.Duration {
	if !TimingEnabled || correlationID == "" {
		return 0
	}

	t.mu.RLock()
	rt, exists := t.timings[correlationID]
	t.mu.RUnlock()

	if !exists {
		return 0
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Find matching phase in stack
	stackIdx := -1
	for i := len(rt.phaseStack) - 1; i >= 0; i-- {
		if rt.phaseStack[i] == phase {
			stackIdx = i
			break
		}
	}

	if stackIdx == -1 {
		// Phase not found in stack - might be called out of order
		return 0
	}

	// Calculate nesting level
	level := stackIdx

	// Remove from stack
	rt.phaseStack = rt.phaseStack[:stackIdx]

	// Find corresponding start in phases
	var startTime time.Time
	for i := len(rt.Phases) - 1; i >= 0; i-- {
		if rt.Phases[i].Name == phase && rt.Phases[i].EndTime.IsZero() {
			startTime = rt.Phases[i].StartTime
			// Update the existing record
			rt.Phases[i].EndTime = time.Now()
			rt.Phases[i].Duration = rt.Phases[i].EndTime.Sub(startTime)
			rt.Phases[i].Level = level
			return rt.Phases[i].Duration
		}
	}

	// If not found (shouldn't happen), create new record
	now := time.Now()
	duration := time.Duration(0) // Unknown start time
	rt.Phases = append(rt.Phases, PhaseRecord{
		Name:      phase,
		StartTime: now,
		EndTime:   now,
		Duration:  duration,
		Level:     level,
	})

	return duration
}

// EndPhaseWithMetadata ends timing with extra data
func (t *TimingTracker) EndPhaseWithMetadata(correlationID, phase string, metadata map[string]any) time.Duration {
	duration := t.EndPhase(correlationID, phase)

	if metadata == nil || correlationID == "" {
		return duration
	}

	t.mu.RLock()
	rt, exists := t.timings[correlationID]
	t.mu.RUnlock()

	if !exists {
		return duration
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Add metadata to most recent matching phase
	for i := len(rt.Phases) - 1; i >= 0; i-- {
		if rt.Phases[i].Name == phase {
			rt.Phases[i].Metadata = metadata
			break
		}
	}

	return duration
}

// RecordPhase records a completed phase (when you have start/end times)
func (t *TimingTracker) RecordPhase(correlationID, phase string, startTime, endTime time.Time, metadata map[string]any) {
	if !TimingEnabled || correlationID == "" {
		return
	}

	t.mu.Lock()
	rt, exists := t.timings[correlationID]
	if !exists {
		rt = &RequestTiming{
			CorrelationID: correlationID,
			StartTime:     time.Now(),
			Phases:        make([]PhaseRecord, 0, 16),
			phaseStack:    make([]string, 0, 8),
		}
		t.timings[correlationID] = rt
	}
	t.mu.Unlock()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.Phases = append(rt.Phases, PhaseRecord{
		Name:      phase,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
		Metadata:  metadata,
		Level:     len(rt.phaseStack),
	})
}

// LogSummary outputs timing summary for a correlation ID
func (t *TimingTracker) LogSummary(correlationID string) {
	if !TimingEnabled || correlationID == "" {
		return
	}

	t.mu.RLock()
	rt, exists := t.timings[correlationID]
	t.mu.RUnlock()

	if !exists {
		return
	}

	rt.mu.Lock()
	phases := make([]PhaseRecord, len(rt.Phases))
	copy(phases, rt.Phases)
	startTime := rt.StartTime
	rt.mu.Unlock()

	// Calculate total duration
	totalDuration := time.Since(startTime)

	// Aggregate phases by category
	aggregated := t.aggregatePhases(phases)

	// Check if we should use compact (JSON-friendly) output
	// We'll detect this by checking if the handler is JSON
	handler := Log.Handler()
	isJSON := false
	if handlerType := fmt.Sprintf("%T", handler); strings.Contains(handlerType, "JSON") {
		isJSON = true
	}

	if isJSON {
		// Structured output for JSON logs
		t.logStructured(correlationID, totalDuration, aggregated)
	} else {
		// Pretty output for text logs
		t.logPretty(correlationID, totalDuration, aggregated)
	}

	// Cleanup
	t.mu.Lock()
	delete(t.timings, correlationID)
	t.mu.Unlock()
}

// logStructured outputs timing as structured log entries (JSON-friendly)
func (t *TimingTracker) logStructured(correlationID string, totalDuration time.Duration, phases []PhaseRecord) {
	// Sort by start time
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].StartTime.Before(phases[j].StartTime)
	})

	// Log summary header
	Log.Info("timing summary",
		"correlation_id", correlationID[:8],
		"total_duration_ms", totalDuration.Milliseconds(),
		"total_duration", formatDuration(totalDuration))

	// Log each phase
	for _, phase := range phases {
		attrs := []any{
			"correlation_id", correlationID[:8],
			"phase", phase.Name,
			"duration_ms", phase.Duration.Milliseconds(),
			"duration", formatDuration(phase.Duration),
			"level", phase.Level,
		}

		// Add metadata
		if len(phase.Metadata) > 0 {
			for k, v := range phase.Metadata {
				attrs = append(attrs, k, v)
			}
		}

		Log.Info("timing phase", attrs...)
	}
}

// logPretty outputs timing as a pretty tree (text-friendly)
func (t *TimingTracker) logPretty(correlationID string, totalDuration time.Duration, phases []PhaseRecord) {
	// Build output
	var b strings.Builder

	// Add slog-style prefix
	timestamp := time.Now().Format("15:04:05")
	b.WriteString(fmt.Sprintf("time=%s level=INFO msg=\"\n", timestamp))
	b.WriteString(fmt.Sprintf("╭─ Timing Summary [%s]\n", correlationID[:8]))
	b.WriteString(fmt.Sprintf("│ Total: %s\n", formatDuration(totalDuration)))
	b.WriteString("├─────────────────────────\n")

	// Sort by start time
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].StartTime.Before(phases[j].StartTime)
	})

	for _, phase := range phases {
		indent := strings.Repeat("  ", phase.Level)
		b.WriteString(fmt.Sprintf("│ %s├─ %s: %s", indent, phase.Name, formatDuration(phase.Duration)))

		// Add metadata if present
		if len(phase.Metadata) > 0 {
			var meta []string
			for k, v := range phase.Metadata {
				meta = append(meta, fmt.Sprintf("%s=%v", k, v))
			}
			sort.Strings(meta)
			b.WriteString(fmt.Sprintf(" (%s)", strings.Join(meta, ", ")))
		}
		b.WriteString("\n")
	}

	b.WriteString("╰─────────────────────────\"")

	// Write directly to stdout to preserve newlines
	//nolint:errcheck
	fmt.Fprintln(os.Stdout, b.String())
}

// aggregatePhases combines similar phases based on detail flags
func (t *TimingTracker) aggregatePhases(phases []PhaseRecord) []PhaseRecord {
	if len(phases) == 0 {
		return phases
	}

	result := make([]PhaseRecord, 0, len(phases))
	aggregationMap := make(map[string]*PhaseRecord)

	for _, phase := range phases {
		aggKey, shouldAggregate := classifyAggregationKey(phase.Name)

		if !shouldAggregate {
			result = append(result, phase)
			continue
		}

		if existing, found := aggregationMap[aggKey]; found {
			updateAggregate(existing, phase)
			continue
		}

		agg := phase
		agg.Name = aggKey
		aggregationMap[aggKey] = &agg
	}

	for _, agg := range aggregationMap {
		result = append(result, *agg)
	}

	return result
}
func updateAggregate(existing *PhaseRecord, phase PhaseRecord) {
	existing.Duration += phase.Duration

	if existing.EndTime.Before(phase.EndTime) {
		existing.EndTime = phase.EndTime
	}

	existing.Metadata = mergeMetadata(existing.Metadata, phase.Metadata)
}

func mergeMetadata(dst, src map[string]any) map[string]any {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any)
	}

	for k, v := range src {
		if ev, ok := dst[k].(int); ok {
			if pv, ok := v.(int); ok {
				dst[k] = ev + pv
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

func classifyAggregationKey(phaseName string) (string, bool) {
	if !TimingDetailedLLM && strings.Contains(phaseName, "_llm_") {
		parts := strings.Split(phaseName, "_llm_")
		if len(parts) > 0 {
			return parts[0] + "_llm_total", true
		}
	}

	if !TimingDetailedTools && strings.Contains(phaseName, "_tool_") {
		parts := strings.Split(phaseName, "_tool_")
		if len(parts) > 0 {
			return parts[0] + "_tool_total", true
		}
	}

	return "", false
}

// cleanup removes old timing entries
func (t *TimingTracker) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for id, rt := range t.timings {
			rt.mu.Lock()
			age := now.Sub(rt.StartTime)
			rt.mu.Unlock()

			// Remove entries older than 10 minutes
			if age > 10*time.Minute {
				delete(t.timings, id)
			}
		}
		t.mu.Unlock()
	}
}

// formatDuration formats duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Microseconds()))
	} else if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
