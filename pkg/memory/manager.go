package memory

// Manager orchestrates memory operations including summarization and retrieval.
type Manager struct {
	store EpisodeStore
}

// NewManager creates a new memory manager.
func NewManager(store EpisodeStore) *Manager {
	return &Manager{
		store: store,
	}
}
