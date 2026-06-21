package store

import (
	"context"
	"sync"
	"time"
)

// memItem wraps an Entry with its absolute expiry time.
type memItem struct {
	entry     Entry
	expiresAt time.Time
}

// memStore is a mutex-guarded in-memory Store implementation.
type memStore struct {
	mu    sync.Mutex
	items map[string]memItem
}

// NewMemory returns an in-memory Store backed by a mutex-guarded map.
func NewMemory() Store {
	return &memStore{
		items: make(map[string]memItem),
	}
}

// Put generates a random handle, stores the entry with an absolute expiry, and
// returns the handle. The entry is bound to credHash for later verification.
func (m *memStore) Put(_ context.Context, credHash string, raw []byte, ttlSeconds int) (string, error) {
	handle, err := newHandle()
	if err != nil {
		return "", err
	}

	// Copy raw so callers cannot mutate the stored slice.
	rawCopy := make([]byte, len(raw))
	copy(rawCopy, raw)

	m.mu.Lock()
	m.items[handle] = memItem{
		entry: Entry{
			CredHash:  credHash,
			Raw:       rawCopy,
			CreatedAt: time.Now().Unix(),
		},
		expiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
	m.mu.Unlock()
	return handle, nil
}

// Get retrieves the entry for handle and returns ErrNotFound when the handle is
// missing or the TTL has elapsed. Expired entries are deleted on access.
// Entries are readable multiple times until they expire.
func (m *memStore) Get(_ context.Context, handle string) (*Entry, error) {
	m.mu.Lock()
	item, ok := m.items[handle]
	if !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	if time.Now().After(item.expiresAt) {
		// Remove the expired entry so subsequent calls see ErrNotFound immediately.
		delete(m.items, handle)
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	m.mu.Unlock()

	// Return a copy of the entry to prevent callers from mutating stored data.
	entryCopy := item.entry
	rawCopy := make([]byte, len(item.entry.Raw))
	copy(rawCopy, item.entry.Raw)
	entryCopy.Raw = rawCopy
	return &entryCopy, nil
}
