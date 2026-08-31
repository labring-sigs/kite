package cluster

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/prometheus"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

type ClientSet struct {
	Name       string
	Version    string // Kubernetes version
	K8sClient  *kube.K8sClient
	PromClient *prometheus.Client

	DiscoveredPrometheusURL string
	NamespaceScoped         bool
	Namespace               string
	config                  string
	prometheusURL           string
}

type ClusterManager struct {
	syncMu         sync.Mutex
	mu             sync.RWMutex
	clusters       map[string]*ClientSet
	errors         map[string]string
	errorBackoff   map[string]clusterBuildBackoff
	defaultContext string
}

type clusterBuildBackoff struct {
	configHash string
	retryAfter time.Time
}

var (
	ErrClusterNotFound     = errors.New("cluster not found")
	ErrClusterAccessDenied = errors.New("cluster access denied")
	ErrNoAccessibleCluster = errors.New("no accessible cluster")
	getClusterByName       = model.GetClusterByName
	listClusters           = model.ListClusters
	buildClientSetFunc     = buildClientSet
	nowFunc                = time.Now
)

const (
	failedClusterBuildBackoff = 5 * time.Minute
	// maxConcurrentClusterBuilds caps how many clusters are built in parallel
	// during a sync pass. Building is I/O heavy (API discovery, informer cache
	// sync), so unbounded parallelism can overwhelm the local machine and the
	// cluster API servers on large fleets.
	maxConcurrentClusterBuilds = 8
)

func createClientSetInCluster(name, prometheusURL string) (*ClientSet, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return newClientSet(name, config, prometheusURL)
}

func createClientSetFromConfig(name, content, prometheusURL string) (*ClientSet, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(content))
	if err != nil {
		klog.Warningf("Failed to create REST config for cluster %s: %v", name, err)
		return nil, err
	}
	contextNamespace := parseCurrentContextNamespace(content)
	cs, err := newClientSet(name, restConfig, prometheusURL, clientOptionsForContextNamespace(contextNamespace))
	if err != nil {
		return nil, err
	}
	cs.applyNamespaceScope(contextNamespace)
	cs.config = content

	return cs, nil
}

func parseCurrentContextNamespace(content string) string {
	cfg, err := clientcmd.Load([]byte(content))
	if err != nil {
		klog.Warningf("Failed to parse kubeconfig while detecting namespace scope: %v", err)
		return ""
	}
	if cfg.CurrentContext == "" {
		return ""
	}
	ctx, ok := cfg.Contexts[cfg.CurrentContext]
	if !ok || ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.Namespace)
}

func clientOptionsForContextNamespace(contextNamespace string) kube.ClientOptions {
	contextNamespace = strings.TrimSpace(contextNamespace)
	if contextNamespace == "" || common.IsNamespaceScopeExempt(contextNamespace) {
		return kube.ClientOptions{}
	}
	// Namespace-scoped contexts (Sealos workspaces) previously disabled the
	// informer cache entirely, so every resource read hit the API server.
	// Keep the cache but restrict it to the workspace namespace: namespaced
	// reads are served locally, cluster-scoped types keep using direct calls.
	return kube.ClientOptions{CacheNamespaces: []string{contextNamespace}}
}

func (cs *ClientSet) applyNamespaceScope(contextNamespace string) {
	contextNamespace = strings.TrimSpace(contextNamespace)
	if contextNamespace == "" {
		return
	}
	// Keep context namespace as preferred namespace even when scope lock is exempted.
	cs.Namespace = contextNamespace
	if common.IsNamespaceScopeExempt(contextNamespace) {
		klog.Infof("Cluster %s context namespace %s is exempt from namespace scope lock", cs.Name, contextNamespace)
		return
	}

	// Honor kubeconfig current-context namespace directly:
	// when context namespace is set, Kite keeps this cluster namespace-scoped.
	cs.NamespaceScoped = true
	klog.Infof("Cluster %s namespace locked by kubeconfig context: %s", cs.Name, cs.Namespace)

	if cs.K8sClient == nil || cs.K8sClient.ClientSet == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := cs.K8sClient.ClientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 1})
	if err == nil {
		klog.Warningf("Cluster %s credentials can list cluster-wide pods, but Kite keeps namespace scope from kubeconfig context: %s", cs.Name, cs.Namespace)
		return
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		klog.Infof("Cluster %s cluster-scope probe denied, namespace scope remains locked: %s", cs.Name, cs.Namespace)
		return
	}
	klog.Warningf("Namespace scope probe failed for cluster %s: %v", cs.Name, err)
}

func newClientSet(name string, k8sConfig *rest.Config, prometheusURL string, options ...kube.ClientOptions) (*ClientSet, error) {
	cs := &ClientSet{
		Name:          name,
		prometheusURL: prometheusURL,
	}
	clientOptions := kube.ClientOptions{}
	if len(options) > 0 {
		clientOptions = options[0]
	}
	var err error
	cs.K8sClient, err = kube.NewClientWithOptions(k8sConfig, clientOptions)
	if err != nil {
		klog.Warningf("Failed to create k8s client for cluster %s: %v", name, err)
		return nil, err
	}
	if prometheusURL == "" {
		prometheusURL = discoveryPrometheusURL(cs.K8sClient)
		if prometheusURL != "" {
			cs.DiscoveredPrometheusURL = prometheusURL
			klog.Infof("Discovered Prometheus URL for cluster %s: %s", name, cs.DiscoveredPrometheusURL)
		}
	}
	if prometheusURL != "" {
		var rt = http.DefaultTransport
		var err error
		if isClusterLocalURL(prometheusURL) {
			rt, err = createK8sProxyTransport(k8sConfig, prometheusURL)
			if err != nil {
				klog.Warningf("Failed to create k8s proxy transport for cluster %s: %v, using direct connection", name, err)
			} else {
				klog.Infof("Using k8s API proxy for Prometheus in cluster %s", name)
			}
		}
		cs.PromClient, err = prometheus.NewClientWithRoundTripper(prometheusURL, rt)
		if err != nil {
			klog.Warningf("Failed to create Prometheus client for cluster %s, some features may not work as expected, err: %v", name, err)
		}
	}
	v, err := cs.K8sClient.ClientSet.Discovery().ServerVersion()
	if err != nil {
		klog.Warningf("Failed to get server version for cluster %s: %v", name, err)
	} else {
		cs.Version = v.String()
	}
	klog.Infof("Loaded K8s client for cluster: %s, version: %s", name, cs.Version)
	return cs, nil
}

func isClusterLocalURL(urlStr string) bool {
	return strings.Contains(urlStr, ".svc.cluster.local") || strings.Contains(urlStr, ".svc:")
}

func createK8sProxyTransport(k8sConfig *rest.Config, prometheusURL string) (*k8sProxyTransport, error) {
	parsedURL, err := url.Parse(prometheusURL)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(parsedURL.Host, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid cluster local URL format")
	}
	svcName := parts[0]
	namespace := parts[1]

	transport, err := rest.TransportFor(k8sConfig)
	if err != nil {
		return nil, err
	}

	transportWrapper := &k8sProxyTransport{
		transport:    transport,
		apiServerURL: k8sConfig.Host,
		namespace:    namespace,
		svcName:      svcName,
		scheme:       parsedURL.Scheme,
	}
	transportWrapper.port = parsedURL.Port()
	if transportWrapper.port == "" {
		if parsedURL.Scheme == "https" {
			transportWrapper.port = "443"
		} else {
			transportWrapper.port = "80"
		}
	}

	return transportWrapper, nil
}

type k8sProxyTransport struct {
	transport    http.RoundTripper
	apiServerURL string
	namespace    string
	svcName      string
	scheme       string
	port         string
}

func (t *k8sProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyURL, err := url.Parse(t.apiServerURL)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = proxyURL.Scheme
	req.URL.Host = proxyURL.Host

	servicePath := fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%s/proxy", t.namespace, t.svcName, t.port)
	req.URL.Path = servicePath + req.URL.Path

	return t.transport.RoundTrip(req)
}

type clusterRuntimeSnapshot struct {
	clusters       map[string]*ClientSet
	errors         map[string]string
	defaultContext string
}

func (cm *ClusterManager) snapshotRuntimeState() clusterRuntimeSnapshot {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	clusters := make(map[string]*ClientSet, len(cm.clusters))
	for name, clientSet := range cm.clusters {
		clusters[name] = clientSet
	}
	errors := make(map[string]string, len(cm.errors))
	for name, errMsg := range cm.errors {
		errors[name] = errMsg
	}
	return clusterRuntimeSnapshot{
		clusters:       clusters,
		errors:         errors,
		defaultContext: cm.defaultContext,
	}
}

func (cm *ClusterManager) GetClientSet(clusterName string) (*ClientSet, error) {
	snapshot := cm.snapshotRuntimeState()
	if len(snapshot.clusters) == 0 {
		return nil, fmt.Errorf("no clusters available")
	}
	if clusterName == "" {
		if snapshot.defaultContext == "" {
			// If no default context is set, return the first available cluster
			for _, cs := range snapshot.clusters {
				return cs, nil
			}
		}
		clusterName = snapshot.defaultContext
	}
	if cluster, ok := snapshot.clusters[clusterName]; ok {
		return cluster, nil
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterName)
}

func (cm *ClusterManager) ResolveClientSetForUser(user model.User, clusterName string) (*ClientSet, error) {
	if clusterName != "" {
		cluster, ok := cm.resolveClientSetByName(clusterName)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterName)
		}
		if !rbac.CanAccessCluster(user, clusterName) {
			return nil, fmt.Errorf("%w: %s", ErrClusterAccessDenied, clusterName)
		}
		return cluster, nil
	}

	snapshot := cm.snapshotRuntimeState()
	if len(snapshot.clusters) == 0 {
		return nil, fmt.Errorf("no clusters available")
	}

	if snapshot.defaultContext != "" {
		if cluster, ok := snapshot.clusters[snapshot.defaultContext]; ok && rbac.CanAccessCluster(user, snapshot.defaultContext) {
			return cluster, nil
		}
	}

	accessible := make([]string, 0, len(snapshot.clusters))
	for name := range snapshot.clusters {
		if rbac.CanAccessCluster(user, name) {
			accessible = append(accessible, name)
		}
	}
	if len(accessible) == 0 {
		return nil, ErrNoAccessibleCluster
	}

	sort.Strings(accessible)
	return snapshot.clusters[accessible[0]], nil
}

func (cm *ClusterManager) resolveClientSetByName(clusterName string) (*ClientSet, bool) {
	if clusterName == "" {
		return nil, false
	}

	if cluster, ok := cm.getCluster(clusterName); ok {
		return cluster, true
	}

	dbCluster, err := getClusterByName(clusterName)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			klog.Warningf("Failed to query cluster %s while resolving client set: %v", clusterName, err)
		}
		return nil, false
	}
	if !dbCluster.Enable {
		return nil, false
	}

	cm.TriggerSyncForCluster(clusterName)
	if !cm.WaitForClusterRefresh(clusterName, 10*time.Second) {
		return nil, false
	}

	cluster, ok := cm.getCluster(clusterName)
	return cluster, ok
}

func (cm *ClusterManager) getCluster(name string) (*ClientSet, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	cluster, ok := cm.clusters[name]
	return cluster, ok
}

func ImportClustersFromKubeconfig(kubeconfig *clientcmdapi.Config) int64 {
	if len(kubeconfig.Contexts) == 0 {
		return 0
	}

	importedCount := 0
	for contextName, context := range kubeconfig.Contexts {
		config := clientcmdapi.NewConfig()
		config.Contexts = map[string]*clientcmdapi.Context{
			contextName: context,
		}
		config.CurrentContext = contextName
		config.Clusters = map[string]*clientcmdapi.Cluster{
			context.Cluster: kubeconfig.Clusters[context.Cluster],
		}
		config.AuthInfos = map[string]*clientcmdapi.AuthInfo{
			context.AuthInfo: kubeconfig.AuthInfos[context.AuthInfo],
		}
		configStr, err := clientcmd.Write(*config)
		if err != nil {
			continue
		}
		cluster := model.Cluster{
			Name:      contextName,
			Config:    model.SecretString(configStr),
			IsDefault: contextName == kubeconfig.CurrentContext,
		}
		if _, err := model.GetClusterByName(contextName); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := model.AddCluster(&cluster); err != nil {
					continue
				}
				importedCount++
				klog.Infof("Imported cluster success: %s", contextName)
			}
			continue
		}
	}
	return int64(importedCount)
}

var (
	syncNow = make(chan clusterSyncRequest, 1)
)

type clusterSyncRequest struct {
	ignoreFailedBuildBackoff bool
}

type clusterSyncOptions struct {
	ignoreFailedBuildBackoff bool
}

func (cm *ClusterManager) TriggerSync() {
	cm.triggerSync(clusterSyncRequest{ignoreFailedBuildBackoff: true})
}

func (cm *ClusterManager) TriggerSyncForCluster(name string) {
	cm.triggerSync(clusterSyncRequest{ignoreFailedBuildBackoff: true})
}

func (cm *ClusterManager) triggerSync(req clusterSyncRequest) {
	select {
	case syncNow <- req:
	default:
		if req.ignoreFailedBuildBackoff {
			select {
			case <-syncNow:
			default:
			}
			select {
			case syncNow <- req:
			default:
			}
		}
	}
}

func (cm *ClusterManager) WaitForCluster(name string, timeout time.Duration) bool {
	return cm.waitForCluster(name, timeout, false)
}

func (cm *ClusterManager) WaitForClusterRefresh(name string, timeout time.Duration) bool {
	return cm.waitForCluster(name, timeout, true)
}

func (cm *ClusterManager) waitForCluster(name string, timeout time.Duration, ignoreStaleError bool) bool {
	if name == "" {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cm.mu.RLock()
		_, clusterExists := cm.clusters[name]
		_, hasError := cm.errors[name]
		cm.mu.RUnlock()
		if clusterExists {
			return true
		}
		if hasError && !ignoreStaleError {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, ok := cm.getCluster(name)
	return ok
}

func clusterConfigHash(cluster *model.Cluster) string {
	if cluster == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%t:%s:%s", cluster.InCluster, cluster.Config, cluster.PrometheusURL)))
	return fmt.Sprintf("%x", sum[:])
}

func (cm *ClusterManager) shouldSkipFailedBuild(cluster *model.Cluster, now time.Time) bool {
	if cm == nil || cluster == nil {
		return false
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.shouldSkipFailedBuildLocked(cluster, now)
}

func (cm *ClusterManager) shouldSkipFailedBuildLocked(cluster *model.Cluster, now time.Time) bool {
	if cm.errorBackoff == nil {
		return false
	}
	backoff, ok := cm.errorBackoff[cluster.Name]
	return ok && backoff.configHash == clusterConfigHash(cluster) && now.Before(backoff.retryAfter)
}

func (cm *ClusterManager) recordFailedBuild(cluster *model.Cluster, now time.Time) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.recordFailedBuildLocked(cluster, now)
}

func (cm *ClusterManager) recordFailedBuildLocked(cluster *model.Cluster, now time.Time) {
	if cm.errorBackoff == nil {
		cm.errorBackoff = make(map[string]clusterBuildBackoff)
	}
	cm.errorBackoff[cluster.Name] = clusterBuildBackoff{
		configHash: clusterConfigHash(cluster),
		retryAfter: now.Add(failedClusterBuildBackoff),
	}
}

func (cm *ClusterManager) clearBuildFailure(name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clearBuildFailureLocked(name)
}

func (cm *ClusterManager) clearBuildFailureLocked(name string) {
	delete(cm.errors, name)
	delete(cm.errorBackoff, name)
}

func syncClusters(cm *ClusterManager) error {
	return syncClustersWithOptions(cm, clusterSyncOptions{})
}

// clusterBuildResult carries the outcome of a parallel cluster build.
type clusterBuildResult struct {
	cluster   *model.Cluster
	clientSet *ClientSet
	err       error
}

// runBounded runs worker over items with at most limit goroutines and
// preserves input ordering in the returned slice.
func runBounded[Input any, Output any](items []Input, limit int, worker func(Input) Output) []Output {
	if len(items) == 0 {
		return []Output{}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > len(items) {
		limit = len(items)
	}

	jobs := make(chan int)
	results := make([]Output, len(items))
	var workers sync.WaitGroup
	workers.Add(limit)
	for workerIndex := 0; workerIndex < limit; workerIndex++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				results[index] = worker(items[index])
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return results
}

func syncClustersWithOptions(cm *ClusterManager, options clusterSyncOptions) error {
	cm.syncMu.Lock()
	defer cm.syncMu.Unlock()

	clusters, err := listClusters()
	if err != nil {
		klog.Warningf("list cluster err: %v", err)
		time.Sleep(5 * time.Second)
		return err
	}
	now := nowFunc()
	dbClusterMap := make(map[string]interface{})
	var buildQueue []*model.Cluster
	for _, cluster := range clusters {
		dbClusterMap[cluster.Name] = cluster

		cm.mu.Lock()
		if cluster.IsDefault {
			cm.defaultContext = cluster.Name
		}
		current, currentExist := cm.clusters[cluster.Name]
		if !cluster.Enable {
			cm.clearBuildFailureLocked(cluster.Name)
		}
		cm.mu.Unlock()

		needsUpdate := shouldUpdateCluster(current, cluster)
		if !needsUpdate {
			continue
		}
		cm.mu.Lock()
		current, currentExist = cm.clusters[cluster.Name]
		shouldSkip := !currentExist && !options.ignoreFailedBuildBackoff && cm.shouldSkipFailedBuildLocked(cluster, now)
		if shouldSkip {
			cm.mu.Unlock()
			continue
		}
		if currentExist {
			delete(cm.clusters, cluster.Name)
		}
		if !cluster.Enable {
			cm.clearBuildFailureLocked(cluster.Name)
		}
		cm.mu.Unlock()

		if currentExist {
			stopClientSet(cluster.Name, current)
		}
		if !cluster.Enable {
			continue
		}

		buildQueue = append(buildQueue, cluster)
	}

	buildResults := runBounded(buildQueue, maxConcurrentClusterBuilds, func(cluster *model.Cluster) clusterBuildResult {
		clientSet, err := buildClientSetFunc(cluster)
		return clusterBuildResult{
			cluster:   cluster,
			clientSet: clientSet,
			err:       err,
		}
	})
	for _, result := range buildResults {
		if result.err != nil {
			klog.Errorf("Failed to build k8s client for cluster %s, in cluster: %t, err: %v", result.cluster.Name, result.cluster.InCluster, result.err)
			cm.mu.Lock()
			cm.errors[result.cluster.Name] = result.err.Error()
			cm.recordFailedBuildLocked(result.cluster, now)
			cm.mu.Unlock()
			continue
		}
		cm.mu.Lock()
		cm.clearBuildFailureLocked(result.cluster.Name)
		cm.clusters[result.cluster.Name] = result.clientSet
		cm.mu.Unlock()
	}
	stoppedClientSets := map[string]*ClientSet{}
	cm.mu.Lock()
	for name, clientSet := range cm.clusters {
		if _, ok := dbClusterMap[name]; !ok {
			delete(cm.clusters, name)
			stoppedClientSets[name] = clientSet
		}
	}
	for name := range cm.errors {
		if _, ok := dbClusterMap[name]; !ok {
			delete(cm.errors, name)
		}
	}
	for name := range cm.errorBackoff {
		if _, ok := dbClusterMap[name]; !ok {
			delete(cm.errorBackoff, name)
		}
	}
	cm.mu.Unlock()
	for name, clientSet := range stoppedClientSets {
		stopClientSet(name, clientSet)
	}

	return nil
}

func stopClientSet(name string, clientSet *ClientSet) {
	if clientSet == nil || clientSet.K8sClient == nil {
		return
	}
	clientSet.K8sClient.Stop(name)
}

// shouldUpdateCluster decides whether the cached ClientSet needs to be updated
// based on the desired state from the database.
func shouldUpdateCluster(cs *ClientSet, cluster *model.Cluster) bool {
	// enable/disable toggle
	if (cs == nil && cluster.Enable) || (cs != nil && !cluster.Enable) {
		klog.Infof("Cluster %s status changed, updating, enabled -> %v", cluster.Name, cluster.Enable)
		return true
	}
	if cs == nil && !cluster.Enable {
		return false
	}

	if cs == nil || cs.K8sClient == nil || cs.K8sClient.ClientSet == nil {
		return true
	}

	// kubeconfig change
	if cs.config != string(cluster.Config) {
		klog.Infof("Kubeconfig changed for cluster %s, updating", cluster.Name)
		return true
	}

	// prometheus URL change
	if cs.prometheusURL != cluster.PrometheusURL {
		klog.Infof("Prometheus URL changed for cluster %s, updating", cluster.Name)
		return true
	}

	// k8s version change
	// TODO: Replace direct ClientSet.Discovery() call with a small DiscoveryInterface.
	// current code depends on *kubernetes.Clientset, which is hard to mock in tests.
	version, err := cs.K8sClient.ClientSet.Discovery().ServerVersion()
	if err != nil {
		klog.Warningf("Failed to get server version for cluster %s: %v", cluster.Name, err)
	} else if version.String() != cs.Version {
		klog.Infof("Server version changed for cluster %s, updating, old: %s, new: %s", cluster.Name, cs.Version, version.String())
		return true
	}

	return false
}

func buildClientSet(cluster *model.Cluster) (*ClientSet, error) {
	if cluster.InCluster {
		return createClientSetInCluster(cluster.Name, cluster.PrometheusURL)
	}
	return createClientSetFromConfig(cluster.Name, string(cluster.Config), cluster.PrometheusURL)
}

func NewClusterManager() (*ClusterManager, error) {
	cm := new(ClusterManager)
	cm.clusters = make(map[string]*ClientSet)
	cm.errors = make(map[string]string)
	cm.errorBackoff = make(map[string]clusterBuildBackoff)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := syncClusters(cm); err != nil {
					klog.Warningf("Failed to sync clusters: %v", err)
				}
			case req := <-syncNow:
				if err := syncClustersWithOptions(cm, clusterSyncOptions{ignoreFailedBuildBackoff: req.ignoreFailedBuildBackoff}); err != nil {
					klog.Warningf("Failed to sync clusters: %v", err)
				}
			}
		}
	}()

	if err := syncClusters(cm); err != nil {
		klog.Warningf("Failed to sync clusters: %v", err)
	}
	return cm, nil
}
