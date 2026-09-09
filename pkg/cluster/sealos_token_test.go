package cluster

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// roundTripperFunc adapts a function to http.RoundTripper for capturing the
// Authorization header the wrapper injects.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestPrepareSealosTokenInjectionSwapsStaticTokenForStore(t *testing.T) {
	config := &rest.Config{
		Host:        "https://apiserver.example.com",
		BearerToken: "token-v1",
	}

	injected := prepareSealosTokenInjection("sealos-alice-ws-a", config)
	require.True(t, injected)
	// The static token must be stripped so the wrapper owns authentication.
	assert.Empty(t, config.BearerToken)
	// The store was seeded with the kubeconfig's token for the first requests.
	assert.Equal(t, "token-v1", sealosClusterToken("sealos-alice-ws-a"))
	require.NotNil(t, config.WrapTransport)

	// Build a client transport through the wrapper and capture the header.
	var seenAuth string
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seenAuth = r.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
	})
	rt := config.WrapTransport(base)

	// First request uses the seeded token.
	_, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, "https://apiserver.example.com/api", nil))
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-v1", seenAuth)

	// After a login records a rotated token, the SAME client uses it — no
	// rebuild required.
	SetSealosClusterToken("sealos-alice-ws-a", "token-v2")
	_, err = rt.RoundTrip(httptest.NewRequest(http.MethodGet, "https://apiserver.example.com/api", nil))
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-v2", seenAuth)
}

func TestPrepareSealosTokenInjectionSkipsNonSealosClusters(t *testing.T) {
	config := &rest.Config{
		Host:        "https://apiserver.example.com",
		BearerToken: "token-v1",
	}

	injected := prepareSealosTokenInjection("prod-cluster", config)
	assert.False(t, injected)
	// Non-Sealos clusters keep their static credential flow untouched.
	assert.Equal(t, "token-v1", config.BearerToken)
	assert.Nil(t, config.WrapTransport)
}

func TestPrepareSealosTokenInjectionRequiresStaticToken(t *testing.T) {
	config := &rest.Config{Host: "https://apiserver.example.com"}
	injected := prepareSealosTokenInjection("sealos-alice-ws-a", config)
	assert.False(t, injected)
	assert.Nil(t, config.WrapTransport)
}

// TestDeleteSealosClusterToken ensures teardown evicts the store entry so
// rotated tokens of removed clusters cannot linger.
func TestDeleteSealosClusterToken(t *testing.T) {
	SetSealosClusterToken("sealos-gone-ws", "token-x")
	require.Equal(t, "token-x", sealosClusterToken("sealos-gone-ws"))

	DeleteSealosClusterToken("sealos-gone-ws")
	assert.Empty(t, sealosClusterToken("sealos-gone-ws"))
}
