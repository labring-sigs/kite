package cluster

import (
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

func Test_applyNamespaceScope(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})

	t.Run("context namespace locks cluster to namespace scope", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{}
		cs := &ClientSet{Name: "test-cluster"}

		cs.applyNamespaceScope(" default ")

		assert.True(t, cs.NamespaceScoped)
		assert.Equal(t, "default", cs.Namespace)
	})

	t.Run("exempt namespace does not lock namespace scope", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{
			"ns-admin": {},
		}
		cs := &ClientSet{Name: "test-cluster"}

		cs.applyNamespaceScope("ns-admin")

		assert.False(t, cs.NamespaceScoped)
		assert.Equal(t, "ns-admin", cs.Namespace)
	})
}

func Test_clientOptionsForContextNamespace(t *testing.T) {
	originalExempt := common.NamespaceScopeExemptNamespaces
	t.Cleanup(func() {
		common.NamespaceScopeExemptNamespaces = originalExempt
	})

	t.Run("regular namespace gets a namespace-scoped informer cache", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{}

		options := clientOptionsForContextNamespace(" ns-a ")

		// The cache is no longer disabled; it is restricted to the context
		// namespace instead.
		assert.False(t, options.DisableCache)
		assert.Equal(t, []string{"ns-a"}, options.CacheNamespaces)
	})

	t.Run("exempt namespace keeps default cache behavior", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{
			"ns-admin": {},
		}

		options := clientOptionsForContextNamespace(" NS-ADMIN ")

		assert.False(t, options.DisableCache)
		assert.Empty(t, options.CacheNamespaces)
	})

	t.Run("empty namespace keeps default cache behavior", func(t *testing.T) {
		common.NamespaceScopeExemptNamespaces = map[string]struct{}{}

		options := clientOptionsForContextNamespace("")

		assert.False(t, options.DisableCache)
		assert.Empty(t, options.CacheNamespaces)
	})
}

func Test_shouldUpdateCluster(t *testing.T) {
	type args struct {
		cs      *ClientSet
		cluster *model.Cluster
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "enable/disable toggle, disable -> enable",
			args: args{
				cs:      nil,
				cluster: &model.Cluster{Name: "test", Enable: true},
			},
			want: true,
		},
		{
			name: "enable/disable toggle, enable -> disable",
			args: args{
				cs: &ClientSet{
					Name: "test",
				},
				cluster: &model.Cluster{Name: "test", Enable: false},
			},
			want: true,
		},
		{
			name: "disable cluster, keep disable",
			args: args{
				cs:      nil,
				cluster: &model.Cluster{Name: "test", Enable: false},
			},
			want: false,
		},
		{
			name: "invalid ClientSet(nil k8sClient), need update",
			args: args{
				cs: &ClientSet{
					Name:      "test",
					Version:   "v1.34.0",
					K8sClient: nil,
				},
				cluster: &model.Cluster{Name: "test", Enable: true},
			},
			want: true,
		},
		{
			name: "invalid ClientSet(nil k8sClient.ClientSet), need update",
			args: args{
				cs: &ClientSet{
					Name:    "test",
					Version: "v1.34.0",
					K8sClient: &kube.K8sClient{
						ClientSet: nil,
					},
				},
				cluster: &model.Cluster{Name: "test", Enable: true},
			},
			want: true,
		},
		{
			name: "k8s config change, need update",
			args: args{
				cs: &ClientSet{
					Name:    "test",
					Version: "v1.34.0",
					K8sClient: &kube.K8sClient{
						ClientSet: &kubernetes.Clientset{},
					},
					config: "test-config",
				},
				cluster: &model.Cluster{Name: "test", Enable: true, Config: model.SecretString("test-config-new")},
			},
			want: true,
		},
		{
			name: "prometheus url change, need update",
			args: args{
				cs: &ClientSet{
					Name:    "test",
					Version: "v1.34.0",
					K8sClient: &kube.K8sClient{
						ClientSet: &kubernetes.Clientset{},
					},
					prometheusURL: "test-prometheus-url",
				},
				cluster: &model.Cluster{Name: "test", Enable: true, PrometheusURL: "test-prometheus-url-new"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpdateCluster(tt.args.cs, tt.args.cluster); got != tt.want {
				t.Errorf("shouldUpdateCluster() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("k8s version change, need update", func(t *testing.T) {
		mockey.PatchConvey("mock ServerVersion change", t, func() {
			mockey.Mock((*discovery.DiscoveryClient).ServerVersion).
				Return(&version.Info{GitVersion: "v1.34.0"}, nil).Build()
			cs := &ClientSet{
				Name:    "test",
				Version: "v1.33.0",
				K8sClient: &kube.K8sClient{
					ClientSet: &kubernetes.Clientset{DiscoveryClient: &discovery.DiscoveryClient{}},
				},
			}
			cluster := &model.Cluster{Name: "test", Enable: true}

			got := shouldUpdateCluster(cs, cluster)
			assert.True(t, got, "expected update when k8s version changed")
		})
	})

	t.Run("same, skip update", func(t *testing.T) {
		mockey.PatchConvey("mock ServerVersion change", t, func() {
			mockey.Mock((*discovery.DiscoveryClient).ServerVersion).
				Return(&version.Info{GitVersion: "v1.34.0"}, nil).Build()
			cs := &ClientSet{
				Name:    "test",
				Version: "v1.34.0",
				K8sClient: &kube.K8sClient{
					ClientSet: &kubernetes.Clientset{DiscoveryClient: &discovery.DiscoveryClient{}},
				},
				config:        "test-config",
				prometheusURL: "test-prometheus-url",
			}
			cluster := &model.Cluster{
				Name:          "test",
				Enable:        true,
				Config:        model.SecretString("test-config"),
				PrometheusURL: "test-prometheus-url",
			}
			got := shouldUpdateCluster(cs, cluster)
			assert.False(t, got, "expected no update when all the same")
		})
	})
}

func TestClusterBuildBackoff(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cm := &ClusterManager{
		errors:       map[string]string{},
		errorBackoff: map[string]clusterBuildBackoff{},
	}
	cluster := &model.Cluster{
		Name:          "sealos-user-ns-a",
		Enable:        true,
		Config:        model.SecretString("kubeconfig-a"),
		PrometheusURL: "http://prometheus",
	}

	assert.False(t, cm.shouldSkipFailedBuild(cluster, now))

	cm.recordFailedBuild(cluster, now)

	assert.True(t, cm.shouldSkipFailedBuild(cluster, now.Add(time.Minute)))
	assert.False(t, cm.shouldSkipFailedBuild(cluster, now.Add(failedClusterBuildBackoff+time.Second)))

	changedCluster := *cluster
	changedCluster.Config = model.SecretString("kubeconfig-b")
	assert.False(t, cm.shouldSkipFailedBuild(&changedCluster, now.Add(time.Minute)))

	changedPrometheusCluster := *cluster
	changedPrometheusCluster.PrometheusURL = "http://prometheus-new"
	assert.False(t, cm.shouldSkipFailedBuild(&changedPrometheusCluster, now.Add(time.Minute)))

	changedInCluster := *cluster
	changedInCluster.InCluster = true
	assert.False(t, cm.shouldSkipFailedBuild(&changedInCluster, now.Add(time.Minute)))

	cm.mu.Lock()
	cm.errors[cluster.Name] = "failed"
	cm.mu.Unlock()
	cm.clearBuildFailure(cluster.Name)
	snapshot := cm.snapshotRuntimeState()
	assert.NotContains(t, snapshot.errors, cluster.Name)
	assert.False(t, cm.shouldSkipFailedBuild(cluster, now.Add(time.Minute)))
}

func TestSyncClustersClearsBuildFailureWhenClusterDisabled(t *testing.T) {
	originalListClusters := listClusters
	originalBuildClientSet := buildClientSetFunc
	originalNow := nowFunc
	t.Cleanup(func() {
		listClusters = originalListClusters
		buildClientSetFunc = originalBuildClientSet
		nowFunc = originalNow
	})

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cluster := &model.Cluster{
		Name:   "sealos-user-ns-a",
		Enable: false,
		Config: model.SecretString("kubeconfig-a"),
	}
	listClusters = func() ([]*model.Cluster, error) {
		return []*model.Cluster{cluster}, nil
	}
	nowFunc = func() time.Time {
		return now
	}

	cm := &ClusterManager{
		clusters:     map[string]*ClientSet{},
		errors:       map[string]string{cluster.Name: "cache sync failed"},
		errorBackoff: map[string]clusterBuildBackoff{},
	}
	cm.recordFailedBuild(cluster, now)

	buildCalls := 0
	buildClientSetFunc = func(cluster *model.Cluster) (*ClientSet, error) {
		buildCalls++
		return nil, errors.New("should not build disabled cluster")
	}

	require.NoError(t, syncClusters(cm))
	assert.Equal(t, 0, buildCalls)
	snapshot := cm.snapshotRuntimeState()
	assert.NotContains(t, snapshot.errors, cluster.Name)
	assert.False(t, cm.shouldSkipFailedBuild(cluster, now.Add(time.Minute)))
}

func TestSyncClustersFailedBuildBackoff(t *testing.T) {
	originalListClusters := listClusters
	originalBuildClientSet := buildClientSetFunc
	originalNow := nowFunc
	t.Cleanup(func() {
		listClusters = originalListClusters
		buildClientSetFunc = originalBuildClientSet
		nowFunc = originalNow
	})

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	currentCluster := &model.Cluster{
		Name:          "sealos-user-ns-a",
		Enable:        true,
		Config:        model.SecretString("kubeconfig-a"),
		PrometheusURL: "http://prometheus",
	}
	listClusters = func() ([]*model.Cluster, error) {
		return []*model.Cluster{currentCluster}, nil
	}
	nowFunc = func() time.Time {
		return now
	}

	cm := &ClusterManager{
		clusters:     map[string]*ClientSet{},
		errors:       map[string]string{},
		errorBackoff: map[string]clusterBuildBackoff{},
	}

	buildCalls := 0
	buildClientSetFunc = func(cluster *model.Cluster) (*ClientSet, error) {
		buildCalls++
		return nil, errors.New("cache sync failed")
	}

	require.NoError(t, syncClusters(cm))
	assert.Equal(t, 1, buildCalls)
	snapshot := cm.snapshotRuntimeState()
	assert.Equal(t, "cache sync failed", snapshot.errors[currentCluster.Name])
	assert.True(t, cm.shouldSkipFailedBuild(currentCluster, now.Add(time.Minute)))

	require.NoError(t, syncClusters(cm))
	assert.Equal(t, 1, buildCalls, "background sync should respect failed-build backoff")

	require.NoError(t, syncClustersWithOptions(cm, clusterSyncOptions{ignoreFailedBuildBackoff: true}))
	assert.Equal(t, 2, buildCalls, "manual sync should retry immediately")

	changedCluster := *currentCluster
	changedCluster.Config = model.SecretString("kubeconfig-b")
	currentCluster = &changedCluster

	require.NoError(t, syncClusters(cm))
	assert.Equal(t, 3, buildCalls, "config changes should bypass failed-build backoff")

	buildClientSetFunc = func(cluster *model.Cluster) (*ClientSet, error) {
		buildCalls++
		return &ClientSet{Name: cluster.Name, config: string(cluster.Config), prometheusURL: cluster.PrometheusURL}, nil
	}

	require.NoError(t, syncClustersWithOptions(cm, clusterSyncOptions{ignoreFailedBuildBackoff: true}))
	assert.Equal(t, 4, buildCalls)
	snapshot = cm.snapshotRuntimeState()
	assert.Contains(t, snapshot.clusters, currentCluster.Name)
	assert.NotContains(t, snapshot.errors, currentCluster.Name)
	assert.False(t, cm.shouldSkipFailedBuild(currentCluster, now.Add(time.Minute)))
}

func TestWaitForClusterRefreshIgnoresStaleError(t *testing.T) {
	cm := &ClusterManager{
		clusters: map[string]*ClientSet{},
		errors:   map[string]string{"sealos-tenant-c": "stale build error"},
	}

	assert.False(t, cm.WaitForCluster("sealos-tenant-c", 10*time.Millisecond))

	start := time.Now()
	assert.False(t, cm.WaitForClusterRefresh("sealos-tenant-c", 10*time.Millisecond))

	assert.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
}

func TestResolveClientSetForUser(t *testing.T) {
	cm := &ClusterManager{
		clusters: map[string]*ClientSet{
			"default-cluster":   {Name: "default-cluster"},
			"sealos-tenant-a":   {Name: "sealos-tenant-a"},
			"sealos-tenant-b":   {Name: "sealos-tenant-b"},
			"unrelated-cluster": {Name: "unrelated-cluster"},
		},
		defaultContext: "default-cluster",
	}

	user := model.User{
		Username: "sealos-user",
		Roles: []common.Role{
			{
				Name:       "sealos-role",
				Clusters:   []string{"sealos-tenant-.*"},
				Namespaces: []string{"*"},
				Resources:  []string{"*"},
				Verbs:      []string{"*"},
			},
		},
	}

	t.Run("fallback to accessible cluster when default cluster is inaccessible", func(t *testing.T) {
		got, err := cm.ResolveClientSetForUser(user, "")
		require.NoError(t, err)
		assert.Equal(t, "sealos-tenant-a", got.Name)
	})

	t.Run("deny explicit inaccessible cluster", func(t *testing.T) {
		_, err := cm.ResolveClientSetForUser(user, "default-cluster")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrClusterAccessDenied)
	})

	t.Run("allow explicit accessible cluster", func(t *testing.T) {
		got, err := cm.ResolveClientSetForUser(user, "sealos-tenant-b")
		require.NoError(t, err)
		assert.Equal(t, "sealos-tenant-b", got.Name)
	})

	t.Run("return no accessible cluster when user has no allowed cluster", func(t *testing.T) {
		noAccessUser := model.User{
			Username: "guest",
			Roles: []common.Role{
				{
					Name:       "guest",
					Clusters:   []string{"guest-only-cluster"},
					Namespaces: []string{"*"},
					Resources:  []string{"*"},
					Verbs:      []string{"*"},
				},
			},
		}
		_, err := cm.ResolveClientSetForUser(noAccessUser, "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNoAccessibleCluster)
	})

	t.Run("wait for explicitly requested cluster to finish syncing", func(t *testing.T) {
		originalLookup := getClusterByName
		getClusterByName = func(name string) (*model.Cluster, error) {
			return &model.Cluster{Name: name, Enable: true}, nil
		}
		t.Cleanup(func() {
			getClusterByName = originalLookup
		})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cm.mu.Lock()
			defer cm.mu.Unlock()
			cm.clusters["sealos-tenant-c"] = &ClientSet{Name: "sealos-tenant-c"}
		}()

		got, err := cm.ResolveClientSetForUser(user, "sealos-tenant-c")
		require.NoError(t, err)
		assert.Equal(t, "sealos-tenant-c", got.Name)
	})

	t.Run("return not found when requested cluster does not exist in db", func(t *testing.T) {
		originalLookup := getClusterByName
		getClusterByName = func(string) (*model.Cluster, error) {
			return nil, gorm.ErrRecordNotFound
		}
		t.Cleanup(func() {
			getClusterByName = originalLookup
		})

		_, err := cm.ResolveClientSetForUser(user, "sealos-tenant-missing")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrClusterNotFound)
	})
}
