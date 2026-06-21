// Package store caches full raw tool results behind opaque handles so a model can
// expand them on demand. Each entry is bound to a credential hash and expires via TTL.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// ErrNotFound is returned when a handle is missing or expired.
var ErrNotFound = errors.New("handle not found")

// Entry is a cached raw result plus the credential hash permitted to redeem it.
type Entry struct {
	CredHash  string `json:"cred_hash"`
	Raw       []byte `json:"raw"`
	CreatedAt int64  `json:"created_at"`
}

// Store persists Entry values under a handle with a TTL.
type Store interface {
	// Put stores raw bound to credHash under a fresh random handle and returns it.
	Put(ctx context.Context, credHash string, raw []byte, ttlSeconds int) (handle string, err error)
	// Get returns the entry for handle, or ErrNotFound.
	Get(ctx context.Context, handle string) (*Entry, error)
}

// newHandle returns a random 16-byte hex handle.
func newHandle() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
