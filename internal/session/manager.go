package session

import (
	"sync"
	"time"
)

// Manager serializes message processing per user to prevent race conditions
// when multiple messages arrive simultaneously for the same phone number.
type Manager struct {
	mu      sync.Mutex
	mutexes map[string]*userLock
}

type userLock struct {
	mu       sync.Mutex
	lastUsed time.Time
}

func NewManager() *Manager {
	return &Manager{
		mutexes: make(map[string]*userLock),
	}
}

// WithLock executes fn while holding the per-phone mutex.
// Concurrent messages from the same phone are serialized; different phones run in parallel.
func (m *Manager) WithLock(phone string, fn func() error) error {
	m.mu.Lock()
	ul, ok := m.mutexes[phone]
	if !ok {
		ul = &userLock{}
		m.mutexes[phone] = ul
	}
	m.mu.Unlock()

	ul.mu.Lock()
	defer ul.mu.Unlock()

	ul.lastUsed = time.Now()
	return fn()
}

// Cleanup removes locks not used within maxAge to prevent memory leaks.
func (m *Manager) Cleanup(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for phone, ul := range m.mutexes {
		if now.Sub(ul.lastUsed) > maxAge {
			delete(m.mutexes, phone)
		}
	}
}
