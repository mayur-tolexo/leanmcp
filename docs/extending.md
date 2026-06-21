# Implementing a custom cache backend

leanmcp stores compacted-result payloads behind an opaque handle via the `store.Store`
interface. If Redis and the built-in in-memory backend do not fit your deployment you can
supply any implementation that satisfies the two-method interface.

## The Store interface

```go
// store package: github.com/mayur-tolexo/leanmcp/store

type Store interface {
    // Put stores raw bound to credHash and returns an opaque handle.
    Put(ctx context.Context, credHash string, raw []byte, ttlSeconds int) (handle string, err error)
    // Get returns the Entry for handle, or ErrNotFound when missing or expired.
    Get(ctx context.Context, handle string) (*Entry, error)
}
```

### Entry fields

```go
type Entry struct {
    CredHash  string // keyed hash of the caller credential that stored the entry
    Raw       []byte // the full, uncompressed tool result
    CreatedAt int64  // Unix timestamp (seconds) at storage time
}
```

### Contract

- **Handles are opaque.** callers treat them as random strings; do not encode meaning in them.
- **ErrNotFound on miss or expiry.** Return `store.ErrNotFound` (not a generic error) when the
  handle does not exist or its TTL has elapsed. leanmcp uses this sentinel to fall back to the
  upstream rather than returning an error to the model.
- **Expire entries by ttlSeconds.** The caller supplies the TTL on `Put`; the backend must honour
  it and stop returning the entry after that duration. Entries may be evicted earlier under memory
  pressure but must never be returned after their TTL.
- **Never log Raw or credentials.** `Raw` contains full tool-call payloads which may include PII;
  `credHash` is derived from a caller secret. Do not write either to logs or traces.

## Minimal compilable example

```go
package main

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "sync"
    "time"

    "github.com/mayur-tolexo/leanmcp/store"
)

// expiringEntry holds a cached raw result with an absolute expiry wall-clock time.
type expiringEntry struct {
    entry   store.Entry
    expiresAt time.Time
}

// MapStore is a minimal in-process Store backed by a sync.Map. It is suitable
// for single-instance deployments or testing; it does not share state across
// replicas.
type MapStore struct {
    mu sync.Mutex
    m  map[string]expiringEntry
}

// newHandle returns a 16-byte random hex string suitable for use as a handle.
func newHandle() (string, error) {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return hex.EncodeToString(b), nil
}

// Put stores raw under a fresh handle with the given TTL and returns the handle.
func (s *MapStore) Put(_ context.Context, credHash string, raw []byte, ttlSeconds int) (string, error) {
    handle, err := newHandle()
    if err != nil {
        return "", err
    }
    s.mu.Lock()
    s.m[handle] = expiringEntry{
        entry: store.Entry{
            CredHash:  credHash,
            Raw:       raw,
            CreatedAt: time.Now().Unix(),
        },
        expiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
    }
    s.mu.Unlock()
    return handle, nil
}

// Get returns the Entry for handle or store.ErrNotFound when absent or expired.
func (s *MapStore) Get(_ context.Context, handle string) (*store.Entry, error) {
    s.mu.Lock()
    e, ok := s.m[handle]
    s.mu.Unlock()
    if !ok || time.Now().After(e.expiresAt) {
        return nil, store.ErrNotFound
    }
    return &e.entry, nil
}

// Verify MapStore satisfies the Store interface at compile time.
var _ store.Store = (*MapStore)(nil)
```

## Wiring a custom backend

Pass your implementation directly to `proxy.NewServer` in `cmd/leanmcp/main.go`:

```go
myStore := &MapStore{m: make(map[string]expiringEntry)}
srv := proxy.NewServer(cfg, upstream.New(cfg.UpstreamMCPURL), myStore, cfg.CacheSecret, m)
```

`store.New` (the factory used in the default `main`) is a convenience wrapper; custom backends
bypass it entirely and are passed straight to `proxy.NewServer`.

## Auth is not an extension point

leanmcp never interprets or validates the caller's credential — it extracts the raw value from
the Authorization header, hashes it with the `cache_secret` HMAC key to form `credHash`, and
forwards the credential verbatim to the upstream. A custom backend receives only the hash, not
the credential itself, and need not implement any auth logic.
