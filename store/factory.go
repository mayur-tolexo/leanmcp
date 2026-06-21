package store

import "fmt"

// New returns a Store for the given backend type. "" and "memory" yield the
// in-memory store; "redis" requires redisURL. Unknown types are an error.
func New(typ, redisURL string) (Store, error) {
	switch typ {
	case "", "memory":
		return NewMemory(), nil
	case "redis":
		if redisURL == "" {
			return nil, fmt.Errorf("store type %q requires a redis url", typ)
		}
		return NewRedis(redisURL)
	default:
		return nil, fmt.Errorf("unknown store type %q", typ)
	}
}
