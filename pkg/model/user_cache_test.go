package model

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxh326/kite/pkg/common"
)

// stubUserLoader returns a loader that serves a fixed user and counts how
// many times it was called, so tests can observe cache hits without a
// database.
func stubUserLoader(user *User, calls *int32) func(uint64) (*User, error) {
	return func(id uint64) (*User, error) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		return user, nil
	}
}

func TestGetUserByIDCachedServesFromCache(t *testing.T) {
	invalidateUserByIDCache(1)
	var calls int32
	loader := stubUserLoader(&User{Username: "alice", OIDCGroups: SliceString{"dev"}}, &calls)

	// First call misses and loads from the "database".
	user, err := getUserByIDCached(1, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("unexpected user %q", user.Username)
	}

	// Second call within the TTL must be served from the cache.
	if _, err := getUserByIDCached(1, loader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("loader called %d times, want 1", got)
	}
}

func TestGetUserByIDCachedReturnsPrivateCopies(t *testing.T) {
	invalidateUserByIDCache(2)
	loader := stubUserLoader(&User{Username: "bob", OIDCGroups: SliceString{"dev"}}, nil)

	first, err := getUserByIDCached(2, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Mutating the returned user (as RequireAuth does when attaching roles)
	// must not leak into the cached entry.
	first.Roles = append(first.Roles, common.Role{Name: "admin"})
	first.OIDCGroups[0] = "mutated"

	second, err := getUserByIDCached(2, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(second.Roles) != 0 {
		t.Fatalf("cached entry was polluted with per-request roles")
	}
	if second.OIDCGroups[0] != "dev" {
		t.Fatalf("cached OIDC groups were mutated: %v", second.OIDCGroups)
	}
}

func TestGetUserByIDCachedInvalidation(t *testing.T) {
	invalidateUserByIDCache(3)
	var calls int32
	loader := stubUserLoader(&User{Username: "carol"}, &calls)

	if _, err := getUserByIDCached(3, loader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	invalidateUserByIDCache(3)
	if _, err := getUserByIDCached(3, loader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("loader called %d times, want 2 after invalidation", got)
	}
}

func TestGetUserByIDCachedDoesNotCacheErrors(t *testing.T) {
	invalidateUserByIDCache(4)
	var calls int32
	wantErr := errors.New("db unavailable")
	loader := func(uint64) (*User, error) {
		atomic.AddInt32(&calls, 1)
		if atomic.LoadInt32(&calls) == 1 {
			return nil, wantErr
		}
		return &User{Username: "dave"}, nil
	}

	if _, err := getUserByIDCached(4, loader); !errors.Is(err, wantErr) {
		t.Fatalf("expected load error, got %v", err)
	}
	user, err := getUserByIDCached(4, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "dave" {
		t.Fatalf("unexpected user %q", user.Username)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("loader called %d times, want 2 (error must not be cached)", got)
	}
}

func TestGetUserByIDCachedExpiresAfterTTL(t *testing.T) {
	invalidateUserByIDCache(5)
	var calls int32
	loader := stubUserLoader(&User{Username: "erin"}, &calls)

	if _, err := getUserByIDCached(5, loader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Force the entry to look expired by rewriting it with a past deadline.
	// The key must be uint64 to match how GetUserByIDCached stores entries.
	userByIDCache.Store(uint64(5), userByIDCacheEntry{
		user:      User{Username: "erin"},
		expiresAt: time.Now().Add(-time.Second),
	})
	if _, err := getUserByIDCached(5, loader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("loader called %d times, want 2 after TTL expiry", got)
	}
}
