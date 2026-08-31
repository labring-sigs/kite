package utils

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// staticLoad returns a load func that always serves the same body and counts
// how many times it was invoked.
func staticLoad(body string, calls *int32) func(ctx context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		return []byte(body), nil
	}
}

func TestSWRCacheMissThenFreshHit(t *testing.T) {
	cache := NewSWRCache(50*time.Millisecond, time.Second, 8)
	var calls int32

	// First call must run the loader synchronously (miss).
	body, state, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v1", &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != SWRCacheMiss {
		t.Fatalf("expected miss, got %s", state)
	}
	if string(body) != "v1" {
		t.Fatalf("unexpected body %q", body)
	}

	// Second call inside the fresh window must be served from cache (hit)
	// without invoking the loader again.
	body, state, err = cache.Serve(context.Background(), "k", time.Second, staticLoad("v2", &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != SWRCacheFresh {
		t.Fatalf("expected fresh hit, got %s", state)
	}
	if string(body) != "v1" {
		t.Fatalf("expected cached body v1, got %q", body)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("loader invoked %d times, want 1", got)
	}
}

func TestSWRCacheStaleTriggersBackgroundRefresh(t *testing.T) {
	cache := NewSWRCache(30*time.Millisecond, 2*time.Second, 8)
	var calls int32

	if _, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v1", &calls)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the fresh window to lapse while staying inside the stale window.
	time.Sleep(60 * time.Millisecond)

	body, state, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v2", &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != SWRCacheStale {
		t.Fatalf("expected stale hit, got %s", state)
	}
	if string(body) != "v1" {
		t.Fatalf("stale serve should return the old body, got %q", body)
	}

	// The background refresh should complete shortly and store the new body.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body, state, _ = cache.Serve(context.Background(), "k", time.Second, staticLoad("v3", &calls))
		if state == SWRCacheFresh && string(body) == "v2" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("background refresh did not store new body; last body=%q state=%s", body, state)
}

func TestSWRCacheExpiresAfterStaleWindow(t *testing.T) {
	cache := NewSWRCache(20*time.Millisecond, 40*time.Millisecond, 8)

	if _, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v1", nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let both the fresh and stale windows lapse.
	time.Sleep(80 * time.Millisecond)

	_, state, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v2", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != SWRCacheMiss {
		t.Fatalf("expected entry to expire past stale window, got state %s", state)
	}
}

func TestSWRCacheLoadErrorIsNotCached(t *testing.T) {
	cache := NewSWRCache(time.Second, time.Minute, 8)
	wantErr := errors.New("backend down")

	_, _, err := cache.Serve(context.Background(), "k", time.Second, func(ctx context.Context) ([]byte, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected load error to propagate, got %v", err)
	}

	// A subsequent call must attempt the load again instead of serving a
	// cached failure.
	body, state, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("ok", nil))
	if err != nil || state != SWRCacheMiss || string(body) != "ok" {
		t.Fatalf("expected recovery after error, body=%q state=%s err=%v", body, state, err)
	}
}

func TestSWRCacheStaleRefreshFailureKeepsOldBody(t *testing.T) {
	cache := NewSWRCache(30*time.Millisecond, 2*time.Second, 8)

	if _, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v1", nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	// Trigger a stale serve whose background refresh fails; the old body must
	// remain available.
	if _, _, err := cache.Serve(context.Background(), "k", time.Second, func(ctx context.Context) ([]byte, error) {
		return nil, errors.New("refresh failed")
	}); err != nil {
		t.Fatalf("stale serve must not fail when refresh fails: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	body, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("v2", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "v1" {
		t.Fatalf("failed refresh must keep old body, got %q", body)
	}
}

func TestSWRCacheEvictsWhenFull(t *testing.T) {
	cache := NewSWRCache(time.Minute, time.Hour, 2)

	for _, key := range []string{"a", "b", "c"} {
		if _, _, err := cache.Serve(context.Background(), key, time.Second, staticLoad(key, nil)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	cache.mu.Lock()
	got := len(cache.entries)
	cache.mu.Unlock()
	if got != 2 {
		t.Fatalf("cache holds %d entries, want max 2", got)
	}
}

func TestSWRCacheServedBodiesAreCopies(t *testing.T) {
	cache := NewSWRCache(time.Minute, time.Hour, 8)

	body, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("abcdef", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Mutating the returned body must not corrupt the cached entry.
	body[0] = 'X'

	again, _, err := cache.Serve(context.Background(), "k", time.Second, staticLoad("other", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(again) != "abcdef" {
		t.Fatalf("cached entry was mutated through served copy: %q", again)
	}
}
