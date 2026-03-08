package auth

import (
	"sync"
	"time"
)

const (
	sessionTTL      = 30 * 24 * time.Hour // 30 days inactivity expiry
	cleanupInterval = 1 * time.Hour
)

type session struct {
	createdAt  time.Time
	lastSeenAt time.Time
}

// SessionStore holds active sessions in memory.
// Sessions are lost on server restart — this is intentional for simplicity.
// The user re-logs in after a restart; sessions are cheap to create.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*session // token → session
}

func newSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*session),
	}
}

// start launches a background goroutine that periodically removes expired sessions.
// It stops when the provided done channel is closed.
func (s *SessionStore) start(done <-chan struct{}) {
	ticker := time.NewTicker(cleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.cleanup()
			case <-done:
				return
			}
		}
	}()
}

func (s *SessionStore) create(token string) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &session{
		createdAt:  now,
		lastSeenAt: now,
	}
}

// validate returns true if the token exists and is not expired.
// Updates lastSeenAt on success (sliding expiry).
func (s *SessionStore) validate(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Since(sess.lastSeenAt) > sessionTTL {
		delete(s.sessions, token)
		return false
	}
	sess.lastSeenAt = time.Now()
	return true
}

func (s *SessionStore) delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// invalidateAll removes every active session.
// Called after a password change to force re-login on all devices.
func (s *SessionStore) invalidateAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = make(map[string]*session)
}

func (s *SessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if time.Since(sess.lastSeenAt) > sessionTTL {
			delete(s.sessions, token)
		}
	}
}

func (s *SessionStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
