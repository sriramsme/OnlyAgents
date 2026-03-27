package agents

import (
	"sync"
)

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

func (a *Agent) turnLockFor(sessionID string) *sync.Mutex {
	v, _ := a.executeMu.LoadOrStore(sessionID, &sync.Mutex{})
	return v.(*sync.Mutex)
}
