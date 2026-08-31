package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func testRESTConfig() *rest.Config {
	return &rest.Config{
		Host:        "https://127.0.0.1:6443",
		BearerToken: "test-token",
	}
}

func TestNewClientWithOptionsDisablesCache(t *testing.T) {
	client, err := NewClientWithOptions(testRESTConfig(), ClientOptions{DisableCache: true})
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Stop("test")
	})

	assert.False(t, client.CacheEnabled)
	assert.NotNil(t, client.Client)
	assert.NotNil(t, client.ClientSet)
}

func TestNewClientHonorsDisableCacheEnvironment(t *testing.T) {
	t.Setenv("DISABLE_CACHE", "true")

	client, err := NewClient(testRESTConfig())
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Stop("test")
	})

	assert.False(t, client.CacheEnabled)
	assert.NotNil(t, client.Client)
	assert.NotNil(t, client.ClientSet)
}

// TestClusterScopedCacheExclusionsResolveAgainstScheme guarantees every type
// in the namespace-scoped cache bypass list is registered in the runtime
// scheme. An unregistered type would make the delegating client fail at
// construction time for every namespace-scoped cluster, so this guards the
// invariant without needing a live API server.
func TestClusterScopedCacheExclusionsResolveAgainstScheme(t *testing.T) {
	exclusions := clusterScopedCacheExclusions()
	require.NotEmpty(t, exclusions)

	for _, obj := range exclusions {
		gvks, _, err := runtimeScheme.ObjectKinds(obj)
		require.NoError(t, err, "excluded type %T must be registered in the scheme", obj)
		require.NotEmpty(t, gvks, "excluded type %T must have a GVK", obj)
	}
}
