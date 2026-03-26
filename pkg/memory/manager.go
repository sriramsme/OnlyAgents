package memory

import (
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory/summarizer"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// MemoryManager builds the memory jobs and registers them with the shared
// scheduler. It no longer owns a cron instance.
type MemoryManager struct {
	store      storage.Storage
	summarizer *summarizer.Summarizer
}

func NewMemoryManager(store storage.Storage, llmClient llm.Client, tz string) *MemoryManager {
	return &MemoryManager{
		store:      store,
		summarizer: summarizer.New(store, llmClient, tz),
	}
}

// RegisterJobs registers all memory-related jobs with the provided scheduler.
// Call this during kernel boot, before scheduler.Start.
func (mm *MemoryManager) RegisterJobs(s *scheduler.Scheduler) {
	s.Register(&dailySummaryJob{summarizer: mm.summarizer, store: mm.store})
	s.Register(&weeklySummaryJob{summarizer: mm.summarizer, store: mm.store})
	s.Register(&monthlySummaryJob{summarizer: mm.summarizer, store: mm.store})
	s.Register(&yearlySummaryJob{summarizer: mm.summarizer, store: mm.store})
}
