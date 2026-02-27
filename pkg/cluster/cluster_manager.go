package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/prometheus"
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
	clusters       map[string]*ClientSet
	errors         map[string]string
	defaultContext string
}

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
	cs, err := newClientSet(name, restConfig, prometheusURL)
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

func (cs *ClientSet) applyNamespaceScope(contextNamespace string) {
	if contextNamespace == "" || cs.K8sClient == nil || cs.K8sClient.ClientSet == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := cs.K8sClient.ClientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 1})
	if err == nil {
		return
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		cs.NamespaceScoped = true
		cs.Namespace = contextNamespace
		klog.Infof("Cluster %s detected as namespace-scoped, locked namespace: %s", cs.Name, cs.Namespace)
		return
	}
	klog.Warningf("Namespace scope probe failed for cluster %s: %v", cs.Name, err)
}

func newClientSet(name string, k8sConfig *rest.Config, prometheusURL string) (*ClientSet, error) {
	cs := &ClientSet{
		Name:          name,
		prometheusURL: prometheusURL,
	}
	var err error
	cs.K8sClient, err = kube.NewClient(k8sConfig)
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

func (cm *ClusterManager) GetClientSet(clusterName string) (*ClientSet, error) {
	if len(cm.clusters) == 0 {
		return nil, fmt.Errorf("no clusters available")
	}
	if clusterName == "" {
		if cm.defaultContext == "" {
			// If no default context is set, return the first available cluster
			for _, cs := range cm.clusters {
				return cs, nil
			}
		}
		return cm.GetClientSet(cm.defaultContext)
	}
	if cluster, ok := cm.clusters[clusterName]; ok {
		return cluster, nil
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterName)
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
	syncNow = make(chan struct{}, 1)
)

func (cm *ClusterManager) TriggerSync() {
	select {
	case syncNow <- struct{}{}:
	default:
	}
}

func (cm *ClusterManager) WaitForCluster(name string, timeout time.Duration) bool {
	if name == "" {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := cm.clusters[name]; ok {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	_, ok := cm.clusters[name]
	return ok
}

func syncClusters(cm *ClusterManager) error {
	clusters, err := model.ListClusters()
	if err != nil {
		klog.Warningf("list cluster err: %v", err)
		time.Sleep(5 * time.Second)
		return err
	}
	dbClusterMap := make(map[string]interface{})
	for _, cluster := range clusters {
		dbClusterMap[cluster.Name] = cluster
		if cluster.IsDefault {
			cm.defaultContext = cluster.Name
		}
		current, currentExist := cm.clusters[cluster.Name]
		if shouldUpdateCluster(current, cluster) {
			if currentExist {
				delete(cm.clusters, cluster.Name)
				current.K8sClient.Stop(cluster.Name)
			}
			if cluster.Enable {
				clientSet, err := buildClientSet(cluster)
				if err != nil {
					klog.Errorf("Failed to build k8s client for cluster %s, in cluster: %t, err: %v", cluster.Name, cluster.InCluster, err)
					cm.errors[cluster.Name] = err.Error()
					continue
				}
				delete(cm.errors, cluster.Name)
				cm.clusters[cluster.Name] = clientSet
			} else {
				delete(cm.errors, cluster.Name)
			}
		}
	}
	for name, clientSet := range cm.clusters {
		if _, ok := dbClusterMap[name]; !ok {
			delete(cm.clusters, name)
			clientSet.K8sClient.Stop(name)
		}
	}
	for name := range cm.errors {
		if _, ok := dbClusterMap[name]; !ok {
			delete(cm.errors, name)
		}
	}

	return nil
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
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := syncClusters(cm); err != nil {
					klog.Warningf("Failed to sync clusters: %v", err)
				}
			case <-syncNow:
				if err := syncClusters(cm); err != nil {
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
