package handlers

import (
	"strings"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/utils"
)

// Response-level stale-while-revalidate caches for the hot dashboard
// endpoints. They mirror the resource summary-list cache so that the initial
// page load can be answered from memory instead of repeating the same
// Kubernetes list calls and Prometheus queries on every poll.
const (
	// overviewCacheTTL matches the summary-list cache freshness; the overview
	// page polls every 30s so a 10s fresh window absorbs bursts of parallel
	// mounts and window-focus refetches.
	overviewCacheTTL        = 10 * time.Second
	overviewCacheStaleTTL   = 60 * time.Second
	overviewRefreshTimeout  = 15 * time.Second
	overviewCacheHeader     = "X-Kite-Overview-Cache"
	overviewCacheMaxEntries = 256

	// resourceUsageHistoryTTL is slightly longer than the overview TTL
	// because Prometheus range queries are the most expensive call on the
	// dashboard and a 15s shift in the time window is imperceptible on a
	// chart that polls every 30s.
	resourceUsageHistoryTTL        = 15 * time.Second
	resourceUsageHistoryStaleTTL   = 60 * time.Second
	resourceUsageRefreshTimeout    = 20 * time.Second
	resourceUsageHistoryHeader     = "X-Kite-Resource-Usage-Cache"
	resourceUsageCacheMaxEntries   = 512
	sidebarCRDCacheTTL             = 60 * time.Second
	sidebarCRDCacheStaleTTL        = 5 * time.Minute
	sidebarCRDRefreshTimeout       = 10 * time.Second
	sidebarCRDCacheHeader          = "X-Kite-Sidebar-CRD-Cache"
	sidebarCRDCacheMaxEntries      = 64
	responseCacheKeyFieldSeparator = "\x00"
)

var (
	// overviewResponseCache caches serialized /overview responses per
	// cluster+user.
	overviewResponseCache = utils.NewSWRCache(overviewCacheTTL, overviewCacheStaleTTL, overviewCacheMaxEntries)
	// resourceUsageHistoryCache caches serialized Prometheus usage-history
	// responses per cluster+user+duration+instance.
	resourceUsageHistoryCache = utils.NewSWRCache(resourceUsageHistoryTTL, resourceUsageHistoryStaleTTL, resourceUsageCacheMaxEntries)
	// sidebarCRDResponseCache caches serialized builtin-CRD discovery
	// responses per cluster; CRD presence is cluster-level state and does not
	// depend on the requesting user.
	sidebarCRDResponseCache = utils.NewSWRCache(sidebarCRDCacheTTL, sidebarCRDCacheStaleTTL, sidebarCRDCacheMaxEntries)
)

// clusterCacheIdentity returns the parts of a cache key that identify the
// target cluster: its configured name plus the API server host so two
// same-named clusters pointing at different servers never share entries.
func clusterCacheIdentity(cs *cluster.ClientSet) []string {
	host := ""
	if cs.K8sClient != nil && cs.K8sClient.Configuration != nil {
		host = cs.K8sClient.Configuration.Host
	}
	return []string{cs.Name, host}
}

// overviewCacheKey scopes overview entries by cluster and user because role
// visibility can change what the caller is allowed to see.
func overviewCacheKey(cs *cluster.ClientSet, user model.User) string {
	parts := append(clusterCacheIdentity(cs), user.Key())
	return strings.Join(parts, responseCacheKeyFieldSeparator)
}

// resourceUsageHistoryCacheKey scopes usage-history entries by everything
// that influences the PromQL the handler builds.
func resourceUsageHistoryCacheKey(cs *cluster.ClientSet, user model.User, duration, instance string) string {
	parts := append(clusterCacheIdentity(cs), user.Key(), duration, instance)
	return strings.Join(parts, responseCacheKeyFieldSeparator)
}

// sidebarCRDCacheKey scopes builtin-CRD entries by cluster and user. Today
// every caller of a cluster shares its stored credentials, so responses are
// identical per cluster record; keying by user as well keeps the cache
// correct if credential handling ever becomes per-user.
func sidebarCRDCacheKey(cs *cluster.ClientSet, user model.User) string {
	parts := append(clusterCacheIdentity(cs), user.Key())
	return strings.Join(parts, responseCacheKeyFieldSeparator)
}
