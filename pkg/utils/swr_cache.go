package utils

import (
	"context"
	"sync"
	"time"
)

// SWRCacheState describes how a served entry relates to the cache.
type SWRCacheState string

const (
	// SWRCacheFresh means the body was served from a fresh (non-expired) cache entry.
	SWRCacheFresh SWRCacheState = "hit"
	// SWRCacheStale means the body was served from an expired entry that is
	// still inside its stale window; a background refresh has been scheduled.
	SWRCacheStale SWRCacheState = "stale"
	// SWRCacheMiss means no usable entry existed and load ran synchronously.
	SWRCacheMiss SWRCacheState = "miss"
)

// swrCacheEntry holds one cached response body together with its freshness
// timestamps and a single-flight refresh marker.
type swrCacheEntry struct {
	body       []byte    // serialized response payload
	expiresAt  time.Time // end of the fresh window
	staleUntil time.Time // end of the stale-while-revalidate window
	refreshing bool      // true while a background refresh is in flight
}

// SWRCache is a small in-memory stale-while-revalidate cache for serialized
// response bodies. It is modeled on the resource summary-list cache so that
// hot read endpoints (overview, prometheus history, sidebar discovery) can
// answer from memory instead of repeating identical backend round-trips.
type SWRCache struct {
	ttl        time.Duration // fresh lifetime of an entry
	staleTTL   time.Duration // total lifetime during which stale data may be served
	maxEntries int           // soft cap; one random entry is evicted when full

	mu      sync.Mutex
	entries map[string]swrCacheEntry
}

// NewSWRCache creates a cache whose entries stay fresh for ttl and may be
// served stale until staleTTL after they were stored. A maxEntries value <= 0
// disables caching entirely.
func NewSWRCache(ttl, staleTTL time.Duration, maxEntries int) *SWRCache {
	return &SWRCache{
		ttl:        ttl,
		staleTTL:   staleTTL,
		maxEntries: maxEntries,
		entries:    map[string]swrCacheEntry{},
	}
}

// Serve returns the response body for key.
//
//   - Fresh hit: the cached body is returned with SWRCacheFresh.
//   - Stale hit: the cached body is returned with SWRCacheStale and exactly
//     one background refresh is started; the refresh calls load with a
//     detached context bounded by refreshTimeout so a slow backend cannot
//     block the caller.
//   - Miss: load runs synchronously with the caller's context and its result
//     is cached on success.
//
// load must return the fully serialized response body; errors are propagated
// and never cached.
func (c *SWRCache) Serve(ctx context.Context, key string, refreshTimeout time.Duration, load func(ctx context.Context) ([]byte, error)) ([]byte, SWRCacheState, error) {
	now := time.Now()
	if body, state, ok := c.get(key, now); ok {
		if state == SWRCacheStale {
			c.refresh(key, refreshTimeout, load)
		}
		return body, state, nil
	}

	body, err := load(ctx)
	if err != nil {
		return nil, SWRCacheMiss, err
	}
	c.set(key, body, now)
	return body, SWRCacheMiss, nil
}

// get returns a copy of the cached body together with its freshness state.
// Entries past their stale window are treated as absent and removed.
func (c *SWRCache) get(key string, now time.Time) ([]byte, SWRCacheState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, "", false
	}
	if !entry.staleUntil.After(now) {
		delete(c.entries, key)
		return nil, "", false
	}
	if entry.expiresAt.After(now) {
		return cloneSWRCacheBytes(entry.body), SWRCacheFresh, true
	}
	return cloneSWRCacheBytes(entry.body), SWRCacheStale, true
}

// set stores body under key, refreshing both freshness windows. When the
// cache is full, one arbitrary entry is evicted to keep memory bounded; the
// exact eviction order does not matter because every entry is cheap to
// recompute.
func (c *SWRCache) set(key string, body []byte, now time.Time) {
	if c.maxEntries <= 0 || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		for evictKey := range c.entries {
			delete(c.entries, evictKey)
			break
		}
	}

	c.entries[key] = swrCacheEntry{
		body:       cloneSWRCacheBytes(body),
		expiresAt:  now.Add(c.ttl),
		staleUntil: now.Add(c.staleTTL),
	}
}

// refresh starts a single-flight background refresh for a stale entry. If
// another goroutine is already refreshing this key, the call is a no-op.
func (c *SWRCache) refresh(key string, refreshTimeout time.Duration, load func(ctx context.Context) ([]byte, error)) {
	if !c.markRefreshing(key, time.Now()) {
		return
	}

	go func() {
		defer c.finishRefresh(key)
		// The refresh must outlive the request that triggered it, so it runs
		// on a detached context with its own timeout.
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()

		body, err := load(ctx)
		if err != nil {
			// Keep serving the existing stale entry; it will be retried on
			// the next request once it crosses the stale window again.
			return
		}
		c.set(key, body, time.Now())
	}()
}

// markRefreshing atomically claims the refresh slot for key. It returns false
// when the entry vanished, expired beyond its stale window, or is already
// being refreshed.
func (c *SWRCache) markRefreshing(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || !entry.staleUntil.After(now) || entry.refreshing {
		return false
	}
	entry.refreshing = true
	c.entries[key] = entry
	return true
}

// finishRefresh releases the refresh slot so future stale hits may refresh
// the entry again.
func (c *SWRCache) finishRefresh(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return
	}
	entry.refreshing = false
	c.entries[key] = entry
}

// Clear drops every cached entry; mainly useful in tests.
func (c *SWRCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]swrCacheEntry{}
}

// cloneSWRCacheBytes copies payload bytes so callers can never mutate data
// that is still referenced by the cache.
func cloneSWRCacheBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
