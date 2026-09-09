package cluster

import (
	"net/http"
	"strings"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
)

// sealosClusterPrefix marks cluster records that were auto-provisioned by the
// Sealos login flow (buildSealosClusterName).
const sealosClusterPrefix = "sealos-"

// IsSealosManagedCluster reports whether the cluster record belongs to the
// Sealos auto-provisioned set, whose kubeconfigs carry short-lived tokens
// that rotate whenever the desktop issues a fresh session.
func IsSealosManagedCluster(name string) bool {
	return strings.HasPrefix(name, sealosClusterPrefix)
}

// ParseSealosKubeconfig extracts the stable identity of a workspace
// kubeconfig (server, context, cluster, user and namespace — the embedded
// token is deliberately excluded because desktop sessions rotate it on every
// refresh) together with the current bearer token. An unparseable kubeconfig
// yields an empty identity, which callers treat as "changed" so they keep
// the full sync path.
func ParseSealosKubeconfig(kubeconfig string) (identity, token string) {
	cfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil || cfg == nil {
		return "", ""
	}
	ctx := cfg.Contexts[cfg.CurrentContext]
	if ctx == nil {
		return "", ""
	}
	server := ""
	if cl, ok := cfg.Clusters[ctx.Cluster]; ok && cl != nil {
		server = cl.Server
	}
	if auth, ok := cfg.AuthInfos[ctx.AuthInfo]; ok && auth != nil {
		token = auth.Token
	}
	identity = strings.Join([]string{server, cfg.CurrentContext, ctx.Cluster, ctx.AuthInfo, ctx.Namespace}, "\x00")
	return identity, token
}

// sealosClusterTokens holds the freshest bearer token per Sealos-managed
// cluster. Sealos workspace kubeconfigs rotate their embedded token on every
// desktop session refresh; keeping the token in this store lets the running
// clients pick up new tokens at login time WITHOUT tearing down and
// rebuilding the informer manager (a multi-second operation per rebuild).
var sealosClusterTokens sync.Map // map[string]string

// SetSealosClusterToken records the current token for a cluster. Empty tokens
// are ignored so a malformed update cannot blank out a working token.
func SetSealosClusterToken(name, token string) {
	if name == "" || token == "" {
		return
	}
	sealosClusterTokens.Store(name, token)
}

// DeleteSealosClusterToken drops the stored token when a cluster's client is
// torn down, so stale entries cannot accumulate or leak across cluster
// deletions.
func DeleteSealosClusterToken(name string) {
	sealosClusterTokens.Delete(name)
}

// sealosClusterToken returns the freshest token known for the cluster, or ""
// when none has been recorded yet.
func sealosClusterToken(name string) string {
	v, ok := sealosClusterTokens.Load(name)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// sealosTokenRoundTripper injects the cluster's freshest token into every
// request instead of relying on the (possibly rotated) token baked into the
// kubeconfig the client was built with.
type sealosTokenRoundTripper struct {
	clusterName string
	next        http.RoundTripper
}

func (rt *sealosTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if token := sealosClusterToken(rt.clusterName); token != "" {
		// Clone before mutating: requests must not be modified in place.
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return rt.next.RoundTrip(req)
}

// prepareSealosTokenInjection rewrites the REST config of a Sealos-managed
// cluster so authentication comes from the live token store rather than the
// static kubeconfig token. It also seeds the store from the kubeconfig so
// freshly built clients work immediately. It returns false (config untouched)
// when the cluster is not Sealos-managed or carries no static token to swap.
func prepareSealosTokenInjection(clusterName string, config *rest.Config) bool {
	if !IsSealosManagedCluster(clusterName) || config == nil {
		return false
	}
	// Only plain bearer-token kubeconfigs are eligible; exec plugins and
	// client certificates keep their original credential flow.
	if config.BearerToken == "" || config.ExecProvider != nil {
		return false
	}

	SetSealosClusterToken(clusterName, config.BearerToken)

	config.BearerToken = ""
	config.WrapTransport = wrapSealosTokenTransport(clusterName, config.WrapTransport)
	return true
}

// wrapSealosTokenTransport composes the token-injecting wrapper with any
// existing transport wrapper.
func wrapSealosTokenTransport(clusterName string, next transport.WrapperFunc) transport.WrapperFunc {
	return func(rt http.RoundTripper) http.RoundTripper {
		if next != nil {
			rt = next(rt)
		}
		return &sealosTokenRoundTripper{clusterName: clusterName, next: rt}
	}
}
