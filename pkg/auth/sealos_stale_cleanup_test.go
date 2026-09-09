package auth

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

// setupStaleCleanupTestDB swaps model.DB for an in-memory sqlite database
// with every table the sweep touches, restoring the original afterwards.
func setupStaleCleanupTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	t.Cleanup(func() {
		model.DB = previousDB
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Cluster{},
		&model.Role{},
		&model.RoleAssignment{},
		&model.ResourceHistory{},
	))
	model.DB = db
}

// seedSealosUser creates one auto-provisioned Sealos user together with its
// auto role, role assignment and clusters, mirroring what SealosLogin
// upserts on every login.
func seedSealosUser(t *testing.T, userID string, workspaces []string, lastLoginAt *time.Time, createdAt time.Time) model.User {
	t.Helper()

	user := model.User{
		Username:    buildSealosUsername(userID),
		Provider:    sealosProvider,
		Sub:         sealosProvider + ":" + userID,
		Enabled:     true,
		LastLoginAt: lastLoginAt,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	// GORM manages CreatedAt itself; force the requested value for the test.
	require.NoError(t, model.DB.Model(&user).Update("created_at", createdAt).Error)

	clusterNames := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		clusterName := buildSealosClusterName(userID, ws)
		clusterNames = append(clusterNames, clusterName)
		require.NoError(t, model.DB.Create(&model.Cluster{
			Name:   clusterName,
			Enable: true,
		}).Error)
	}

	// The auto role only keeps the most recent cluster, exactly like
	// ensureSealosRole does on sequential logins.
	role := model.Role{
		Name:       buildSealosRoleName(userID),
		Clusters:   clusterNames[len(clusterNames)-1:],
		Namespaces: []string{workspaces[len(workspaces)-1]},
		Resources:  []string{"*"},
		Verbs:      []string{"*"},
	}
	require.NoError(t, model.DB.Create(&role).Error)
	require.NoError(t, model.DB.Create(&model.RoleAssignment{
		RoleID:      role.ID,
		SubjectType: model.SubjectTypeUser,
		Subject:     user.Username,
	}).Error)
	return user
}

func countRows(t *testing.T, m any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(m).Count(&count).Error)
	return count
}

func TestSweepStaleSealosUsers(t *testing.T) {
	setupStaleCleanupTestDB(t)

	now := time.Now()
	fortyDaysAgo := now.AddDate(0, 0, -40)
	recentLogin := now.Add(-time.Hour)

	// Stale user with two workspaces; the older one is an orphan that only
	// the name-prefix match can find.
	staleLogin := fortyDaysAgo
	seedSealosUser(t, "staleuser1", []string{"ws-a", "ws-b"}, &staleLogin, fortyDaysAgo)
	// Ghost user: auto-provisioned but never logged in.
	seedSealosUser(t, "ghostuser2", []string{"ws-c"}, nil, fortyDaysAgo)
	// Active user that must survive the sweep.
	seedSealosUser(t, "freshuser3", []string{"ws-d"}, &recentLogin, now.AddDate(0, 0, -5))
	// Password admin with a manually managed cluster; never touched.
	admin := model.User{Username: "admin", Provider: "password", Enabled: true}
	require.NoError(t, model.DB.Create(&admin).Error)
	require.NoError(t, model.DB.Create(&model.Cluster{Name: "prod", Enable: true}).Error)

	removed, err := SweepStaleSealosUsers(now, 30, staleSweepBatchSize)
	require.NoError(t, err)
	assert.Equal(t, 2, removed, "stale user + ghost user should be swept")

	// The two inactive users are gone; the active one and the admin remain.
	var remaining []model.User
	require.NoError(t, model.DB.Find(&remaining).Error)
	remainingNames := map[string]bool{}
	for _, u := range remaining {
		remainingNames[u.Username] = true
	}
	assert.False(t, remainingNames[buildSealosUsername("staleuser1")])
	assert.False(t, remainingNames[buildSealosUsername("ghostuser2")])
	assert.True(t, remainingNames[buildSealosUsername("freshuser3")])
	assert.True(t, remainingNames["admin"])

	// All clusters of the swept users are gone — including the orphan that
	// the role no longer referenced — while others survive.
	var remainingClusters []model.Cluster
	require.NoError(t, model.DB.Find(&remainingClusters).Error)
	remainingClusterNames := map[string]bool{}
	for _, cl := range remainingClusters {
		remainingClusterNames[cl.Name] = true
	}
	assert.False(t, remainingClusterNames[buildSealosClusterName("staleuser1", "ws-a")])
	assert.False(t, remainingClusterNames[buildSealosClusterName("staleuser1", "ws-b")])
	assert.False(t, remainingClusterNames[buildSealosClusterName("ghostuser2", "ws-c")])
	assert.True(t, remainingClusterNames[buildSealosClusterName("freshuser3", "ws-d")])
	assert.True(t, remainingClusterNames["prod"])

	// Auto roles and assignments of swept users are gone; the active user's
	// role and assignment correctly remain.
	assert.Equal(t, int64(1), countRows(t, &model.Role{}))
	assert.Equal(t, int64(1), countRows(t, &model.RoleAssignment{}))
}

// TestSweepStaleSealosUsersLongestPrefixOwnership proves the collision fix:
// a stale user "a" must not sweep the active user "a-b"'s cluster even though
// "sealos-a-b-c" starts with the raw "sealos-a-" prefix.
func TestSweepStaleSealosUsersLongestPrefixOwnership(t *testing.T) {
	setupStaleCleanupTestDB(t)

	now := time.Now()
	fortyDaysAgo := now.AddDate(0, 0, -40)
	recentLogin := now.Add(-time.Hour)

	// Stale user "a" owning sealos-a-x.
	staleLogin := fortyDaysAgo
	seedSealosUser(t, "a", []string{"x"}, &staleLogin, fortyDaysAgo)
	// Active user "a-b" owning sealos-a-b-c — a raw "sealos-a-" prefix match
	// that must NOT be swept with user "a".
	seedSealosUser(t, "a-b", []string{"c"}, &recentLogin, now.AddDate(0, 0, -5))

	removed, err := SweepStaleSealosUsers(now, 30, staleSweepBatchSize)
	require.NoError(t, err)
	assert.Equal(t, 1, removed, "only stale user a should be swept")

	var clusters []model.Cluster
	require.NoError(t, model.DB.Find(&clusters).Error)
	require.Len(t, clusters, 1)
	assert.Equal(t, buildSealosClusterName("a-b", "c"), clusters[0].Name,
		"the active longer-prefix user's cluster must survive")

	var users []model.User
	require.NoError(t, model.DB.Find(&users).Error)
	require.Len(t, users, 1)
	assert.Equal(t, buildSealosUsername("a-b"), users[0].Username)
}

// TestSweepStaleSealosUsersSkipsNonSealosRows ensures the sweep never
// touches password users even when they are old and inactive.
func TestSweepStaleSealosUsersSkipsNonSealosRows(t *testing.T) {
	setupStaleCleanupTestDB(t)

	now := time.Now()
	old := now.AddDate(0, 0, -90)
	oldLogin := old
	require.NoError(t, model.DB.Create(&model.User{
		Username:    "old-password-user",
		Provider:    "password",
		Enabled:     true,
		LastLoginAt: &oldLogin,
	}).Error)

	removed, err := SweepStaleSealosUsers(now, 30, staleSweepBatchSize)
	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.Equal(t, int64(1), countRows(t, &model.User{}))
}
