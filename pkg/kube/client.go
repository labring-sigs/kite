package kube

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	metricsv1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var runtimeScheme = runtime.NewScheme()

const cacheSyncTimeout = 10 * time.Second

func init() {
	ctrllog.SetLogger(controllerRuntimeLogger(klog.NewKlogr()))
	_ = scheme.AddToScheme(runtimeScheme)
	_ = apiextensionsv1.AddToScheme(runtimeScheme)
	_ = gatewayapiv1.Install(runtimeScheme)
	_ = metricsv1.AddToScheme(runtimeScheme)
}

func controllerRuntimeLogger(logger logr.Logger) logr.Logger {
	return logr.New(controllerRuntimeLogSink{sink: logger.GetSink()})
}

type controllerRuntimeLogSink struct {
	sink logr.LogSink
}

func (l controllerRuntimeLogSink) Init(info logr.RuntimeInfo) {
	l.sink.Init(info)
}

func (l controllerRuntimeLogSink) Enabled(level int) bool {
	return klog.V(2).Enabled() && l.sink.Enabled(level)
}

func (l controllerRuntimeLogSink) Info(level int, msg string, keysAndValues ...any) {
	if !klog.V(2).Enabled() {
		return
	}
	l.sink.Info(level, msg, keysAndValues...)
}

func (l controllerRuntimeLogSink) Error(err error, msg string, keysAndValues ...any) {
	if !klog.V(2).Enabled() {
		return
	}
	l.sink.Error(err, msg, keysAndValues...)
}

func (l controllerRuntimeLogSink) WithValues(keysAndValues ...any) logr.LogSink {
	l.sink = l.sink.WithValues(keysAndValues...)
	return l
}

func (l controllerRuntimeLogSink) WithName(name string) logr.LogSink {
	l.sink = l.sink.WithName(name)
	return l
}

func (l controllerRuntimeLogSink) WithCallDepth(depth int) logr.LogSink {
	if sink, ok := l.sink.(logr.CallDepthLogSink); ok {
		l.sink = sink.WithCallDepth(depth)
	}
	return l
}

// K8sClient holds the Kubernetes client instances
type K8sClient struct {
	client.Client
	ClientSet     *kubernetes.Clientset
	Configuration *rest.Config
	HTTPClient    *http.Client // reusable HTTP client for REST endpoint access (e.g. Table summaries)
	MetricsClient *metricsclient.Clientset
	CacheEnabled  bool // true when using controller-runtime informer cache

	cancel context.CancelFunc
}

type ClientOptions struct {
	DisableCache bool
	// CacheNamespaces restricts the informer cache to the given namespaces.
	// It is used for namespace-scoped kubeconfig contexts (e.g. Sealos
	// workspaces): namespaced reads are served from the local cache instead
	// of hitting the API server on every request, while cluster-scoped types
	// (which cannot be watched per-namespace) bypass the cache and keep
	// using direct API calls. Ignored when the cache is disabled.
	CacheNamespaces []string
}

// clusterScopedCacheExclusions lists the registered cluster-scoped types that
// must bypass a namespace-scoped informer cache: a per-namespace cache cannot
// watch cluster-scoped objects, and informer creation for them would fail.
// These reads behave exactly as they did when the whole cache was disabled —
// they go straight to the API server and succeed or fail with the caller's
// RBAC permissions.
//
// If a cluster-scoped type is ever missing here, the failure mode is a
// descriptive "cannot watch cluster-scoped resource with namespace-scoped
// cache" error on that specific read, never silent data corruption.
func clusterScopedCacheExclusions() []client.Object {
	return []client.Object{
		&corev1.Node{},
		&corev1.Namespace{},
		&corev1.PersistentVolume{},
		&corev1.ComponentStatus{},
		&rbacv1.ClusterRole{},
		&rbacv1.ClusterRoleBinding{},
		&storagev1.StorageClass{},
		&storagev1.VolumeAttachment{},
		&storagev1.CSIDriver{},
		&storagev1.CSINode{},
		&networkingv1.IngressClass{},
		&schedulingv1.PriorityClass{},
		&nodev1.RuntimeClass{},
		&admissionregistrationv1.MutatingWebhookConfiguration{},
		&admissionregistrationv1.ValidatingWebhookConfiguration{},
		&admissionregistrationv1.ValidatingAdmissionPolicy{},
		&admissionregistrationv1.ValidatingAdmissionPolicyBinding{},
		&certificatesv1.CertificateSigningRequest{},
		&apiextensionsv1.CustomResourceDefinition{},
		&gatewayapiv1.GatewayClass{},
		&metricsv1.NodeMetrics{},
	}
}

// NewClient creates a K8sClient from a rest.Config
func NewClient(config *rest.Config) (*K8sClient, error) {
	return NewClientWithOptions(config, ClientOptions{})
}

func NewClientWithOptions(config *rest.Config, options ClientOptions) (*K8sClient, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	metricsClient, err := metricsclient.NewForConfig(config)
	if err != nil {
		klog.Warningf("failed to create metrics client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cacheEnabled := os.Getenv("DISABLE_CACHE") != "true" && !options.DisableCache

	var c client.Client
	if !cacheEnabled {
		c, err = client.New(config, client.Options{
			Scheme: runtimeScheme,
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create client: %w", err)
		}
	} else {
		cacheOptions := cache.Options{
			DefaultWatchErrorHandler: func(ctx context.Context, r *toolscache.Reflector, err error) {
			},
		}
		// clientOptions carries the cache bypass list; it stays empty for a
		// cluster-wide cache so behavior is identical to before.
		clientOptions := client.Options{}

		// Build the namespace restriction for namespace-scoped clusters
		// (Sealos workspaces). With DefaultNamespaces set, every namespaced
		// informer only watches the workspace namespace, and cluster-scoped
		// types are excluded from the cache via DisableFor.
		defaultNamespaces := map[string]cache.Config{}
		for _, ns := range options.CacheNamespaces {
			if ns = strings.TrimSpace(ns); ns != "" {
				defaultNamespaces[ns] = cache.Config{}
			}
		}
		if len(defaultNamespaces) > 0 {
			cacheOptions.DefaultNamespaces = defaultNamespaces
			clientOptions.Cache = &client.CacheOptions{
				DisableFor: clusterScopedCacheExclusions(),
			}
		}

		mgr, err := manager.New(config, manager.Options{
			Scheme:         runtimeScheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: "0", // Disable metrics server
			},
			Cache:  cacheOptions,
			Client: clientOptions,
		})
		if err != nil {
			cancel()
			return nil, err
		}

		// Add field indexer for Pod spec.nodeName to enable efficient querying by node
		if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(rawObj client.Object) []string {
			pod := rawObj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create field indexer for spec.nodeName: %w", err)
		}
		go func() {
			if err := mgr.Start(ctx); err != nil {
				fmt.Printf("Error starting manager: %v\n", err)
			}
		}()
		syncCtx, syncCancel := context.WithTimeout(ctx, cacheSyncTimeout)
		defer syncCancel()
		if !mgr.GetCache().WaitForCacheSync(syncCtx) {
			cancel()
			return nil, fmt.Errorf("failed to wait for cache sync")
		}
		c = mgr.GetClient()
	}

	return &K8sClient{
		Client:        c,
		ClientSet:     clientset,
		Configuration: config,
		HTTPClient:    httpClient,
		MetricsClient: metricsClient,
		CacheEnabled:  cacheEnabled,
		cancel:        cancel,
	}, nil
}

func (c *K8sClient) Stop(name string) {
	klog.Infof("Stopping K8s client for %s", name)
	c.cancel()
}

// GetScheme returns the runtime scheme used by the client
func GetScheme() *runtime.Scheme {
	return runtimeScheme
}

func WaitForResourceDeletion(ctx context.Context, client client.Client, obj client.Object, timeout time.Duration) error {
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeoutCh := time.After(timeout)
	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for resource deletion: %s", key)
		case <-ticker.C:
			if err := client.Get(ctx, key, obj); err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("failed to get resource: %w", err)
			} else if obj.GetDeletionTimestamp().IsZero() {
				// resource still exist, but deletion timestamp is not set
				// may be created again after deletion
				// we can consider it successfully deleted.
				return nil
			}
		}
	}
}
