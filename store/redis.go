package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStore is a Redis-backed Store implementation.
type redisStore struct {
	client *redis.Client
}

// NewRedis parses redisURL and returns a Redis-backed Store. Returns an error
// if the URL is invalid.
func NewRedis(url string) (Store, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &redisStore{client: redis.NewClient(opts)}, nil
}

// Put marshals the Entry to JSON, stores it under a random handle with the
// given TTL, and returns the handle.
func (r *redisStore) Put(ctx context.Context, credHash string, raw []byte, ttlSeconds int) (string, error) {
	handle, err := newHandle()
	if err != nil {
		return "", err
	}

	entry := Entry{
		CredHash:  credHash,
		Raw:       raw,
		CreatedAt: time.Now().Unix(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("marshal entry: %w", err)
	}

	if err := r.client.Set(ctx, handle, data, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}
	return handle, nil
}

// Get fetches the entry for handle from Redis, maps redis.Nil to ErrNotFound,
// and unmarshals the JSON into an Entry.
func (r *redisStore) Get(ctx context.Context, handle string) (*Entry, error) {
	data, err := r.client.Get(ctx, handle).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal entry: %w", err)
	}
	return &entry, nil
}
