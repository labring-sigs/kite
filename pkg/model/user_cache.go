package model

import (
	"sync"
	"time"
)

// userByIDCacheTTL bounds how long request authentication may rely on a
// cached user row before it is re-read from the database. The dashboard fires
// bursts of concurrent API requests, each of which used to run an identical
// GetUserByID query; a short TTL collapses those bursts while keeping
// administrative changes (disable, role edits) visible within seconds. Every
// mutating function in user.go also invalidates the entry explicitly, so the
// TTL is only a backstop.
const userByIDCacheTTL = 15 * time.Second

// userByIDCacheEntry stores a user row by value so cached data can never be
// mutated through a returned pointer.
type userByIDCacheEntry struct {
	user      User
	expiresAt time.Time
}

// userByIDCache maps user IDs to their cached row.
var userByIDCache sync.Map

// cloneUserCopy returns a private copy of the user, including its slice
// fields, so callers can attach per-request data (e.g. resolved roles)
// without polluting the cached value.
func cloneUserCopy(user User) *User {
	cloned := user
	cloned.OIDCGroups = append(SliceString(nil), user.OIDCGroups...)
	return &cloned
}

// getUserByIDCached implements the cache lookup with an injectable loader so
// tests can count database hits without a real database.
func getUserByIDCached(id uint64, load func(uint64) (*User, error)) (*User, error) {
	if cached, ok := userByIDCache.Load(id); ok {
		entry := cached.(userByIDCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return cloneUserCopy(entry.user), nil
		}
		// Drop the expired entry so a concurrent store cannot revive it.
		userByIDCache.Delete(id)
	}

	user, err := load(id)
	if err != nil {
		return nil, err
	}
	userByIDCache.Store(id, userByIDCacheEntry{
		user:      *cloneUserCopy(*user),
		expiresAt: time.Now().Add(userByIDCacheTTL),
	})
	return cloneUserCopy(*user), nil
}

// GetUserByIDCached returns the user for id, serving short-lived in-memory
// copies to avoid a database round-trip on every authenticated request.
// Callers receive a private copy; mutations must go through the model
// functions, which invalidate the cache.
func GetUserByIDCached(id uint64) (*User, error) {
	return getUserByIDCached(id, GetUserByID)
}

// invalidateUserByIDCache drops the cached row for id. It is called by every
// user-mutating model function so administrative changes take effect
// immediately instead of waiting for the TTL.
func invalidateUserByIDCache(id uint64) {
	userByIDCache.Delete(id)
}
