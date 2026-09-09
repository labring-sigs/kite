package auth

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

// buildWorkspaceKubeconfig renders a workspace-style kubeconfig with the
// given parts so identity extraction can be tested field by field.
func buildWorkspaceKubeconfig(server, clusterName, userName, namespace, token string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
    namespace: %s
  name: %s
current-context: %s
users:
- name: %s
  user:
    token: %s
`, server, clusterName, clusterName, userName, namespace, clusterName, clusterName, userName, token)
}

func TestParseSealosKubeconfigIdentityIgnoresToken(t *testing.T) {
	kcV1 := buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-a", "token-v1")
	kcV2 := buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-a", "token-v2")

	identityV1, tokenV1 := parseSealosKubeconfig(kcV1)
	identityV2, tokenV2 := parseSealosKubeconfig(kcV2)

	// A token rotation must not change the identity: role sync and client
	// rebuilds are skipped in that case.
	assert.Equal(t, identityV1, identityV2)
	assert.Equal(t, "token-v1", tokenV1)
	assert.Equal(t, "token-v2", tokenV2)
}

func TestParseSealosKubeconfigIdentityChangesWithParts(t *testing.T) {
	base := buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-a", "token")
	baseIdentity, _ := parseSealosKubeconfig(base)
	require.NotEmpty(t, baseIdentity)

	cases := map[string]string{
		"different server":    buildWorkspaceKubeconfig("https://other.example.com", "ws-a", "alice", "ws-a", "token"),
		"different namespace": buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-b", "token"),
		"different user":      buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "bob", "ws-a", "token"),
	}
	for name, kc := range cases {
		identity, _ := parseSealosKubeconfig(kc)
		assert.NotEqual(t, baseIdentity, identity, name)
	}

	// Unparseable input yields an empty identity (treated as changed).
	invalidIdentity, _ := parseSealosKubeconfig("not a kubeconfig")
	assert.Empty(t, invalidIdentity)
}

// setupKubeconfigTestDB swaps model.DB for an in-memory sqlite database with
// the cluster table, restoring the original afterwards.
func setupKubeconfigTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	t.Cleanup(func() {
		model.DB = originalDB
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Cluster{}))
	model.DB = db
}

func TestUpsertSealosClusterIdentityChangeSemantics(t *testing.T) {
	setupKubeconfigTestDB(t)

	kcV1 := buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-a", "token-v1")
	kcV2 := buildWorkspaceKubeconfig("https://apiserver.example.com", "ws-a", "alice", "ws-a", "token-v2")
	kcOtherServer := buildWorkspaceKubeconfig("https://other.example.com", "ws-a", "alice", "ws-a", "token-v1")
	clusterName := "sealos-alice-ws-a"

	// First upsert creates the record and counts as an identity change.
	changed, err := upsertSealosCluster(clusterName, kcV1, "ws-a")
	require.NoError(t, err)
	assert.True(t, changed)

	// Same kubeconfig again: no identity change.
	changed, err = upsertSealosCluster(clusterName, kcV1, "ws-a")
	require.NoError(t, err)
	assert.False(t, changed)

	// Rotated token: identity is unchanged, but the stored config must be
	// refreshed so restarts still get the fresh token.
	changed, err = upsertSealosCluster(clusterName, kcV2, "ws-a")
	require.NoError(t, err)
	assert.False(t, changed)
	stored, err := model.GetClusterByName(clusterName)
	require.NoError(t, err)
	assert.Equal(t, kcV2, string(stored.Config))

	// A different apiserver: identity changed, full sync path.
	changed, err = upsertSealosCluster(clusterName, kcOtherServer, "ws-a")
	require.NoError(t, err)
	assert.True(t, changed)
}
