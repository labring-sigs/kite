package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OverviewData struct {
	TotalNodes      int                   `json:"totalNodes"`
	ReadyNodes      int                   `json:"readyNodes"`
	TotalPods       int                   `json:"totalPods"`
	RunningPods     int                   `json:"runningPods"`
	TotalNamespaces int                   `json:"totalNamespaces"`
	TotalIngresses  int                   `json:"totalIngresses"`
	TotalPVCs       int                   `json:"totalPVCs"`
	TotalServices   int                   `json:"totalServices"`
	PromEnabled     bool                  `json:"prometheusEnabled"`
	Resource        common.ResourceMetric `json:"resource"`
}

type overviewResourceSummary struct {
	cpuAllocatable resource.Quantity
	memAllocatable resource.Quantity
	cpuRequested   resource.Quantity
	memRequested   resource.Quantity
	cpuLimited     resource.Quantity
	memLimited     resource.Quantity
	cpuBasis       string
	memoryBasis    string
}

func newOverviewResourceSummary() overviewResourceSummary {
	return overviewResourceSummary{
		cpuBasis:    common.ResourceBasisClusterAllocatable,
		memoryBasis: common.ResourceBasisClusterAllocatable,
	}
}

func (s *overviewResourceSummary) collectNodeStats(nodes []v1.Node) int {
	readyNodes := 0
	for _, node := range nodes {
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			s.cpuAllocatable.Add(*cpu)
		}
		if memory := node.Status.Allocatable.Memory(); memory != nil {
			s.memAllocatable.Add(*memory)
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}
	return readyNodes
}

func (s *overviewResourceSummary) collectPodStats(pods []v1.Pod) int {
	runningPods := 0
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			if cpuRequest := container.Resources.Requests.Cpu(); cpuRequest != nil {
				s.cpuRequested.Add(*cpuRequest)
			}
			if memoryRequest := container.Resources.Requests.Memory(); memoryRequest != nil {
				s.memRequested.Add(*memoryRequest)
			}
			if container.Resources.Limits != nil {
				if cpuLimit := container.Resources.Limits.Cpu(); cpuLimit != nil {
					s.cpuLimited.Add(*cpuLimit)
				}
				if memoryLimit := container.Resources.Limits.Memory(); memoryLimit != nil {
					s.memLimited.Add(*memoryLimit)
				}
			}
		}
		if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
			runningPods++
		}
	}
	return runningPods
}

func (s *overviewResourceSummary) applyNamespaceQuota(cpuQuotaMilli, memoryQuotaBytes int64, hasCPUQuota, hasMemoryQuota bool) {
	if hasCPUQuota {
		s.cpuAllocatable = *resource.NewMilliQuantity(cpuQuotaMilli, resource.DecimalSI)
		s.cpuBasis = common.ResourceBasisNamespaceQuota
	} else {
		s.cpuBasis = common.ResourceBasisNamespaceNoQuota
	}
	if hasMemoryQuota {
		s.memAllocatable = *resource.NewQuantity(memoryQuotaBytes, resource.BinarySI)
		s.memoryBasis = common.ResourceBasisNamespaceQuota
	} else {
		s.memoryBasis = common.ResourceBasisNamespaceNoQuota
	}
}

func (s *overviewResourceSummary) toMetric() common.ResourceMetric {
	return common.ResourceMetric{
		CPU: common.Resource{
			Allocatable: s.cpuAllocatable.MilliValue(),
			Requested:   s.cpuRequested.MilliValue(),
			Limited:     s.cpuLimited.MilliValue(),
			Basis:       s.cpuBasis,
		},
		Mem: common.Resource{
			Allocatable: s.memAllocatable.MilliValue(),
			Requested:   s.memRequested.MilliValue(),
			Limited:     s.memLimited.MilliValue(),
			Basis:       s.memoryBasis,
		},
	}
}

func listOptionsForScopedNamespace(cs *cluster.ClientSet) *client.ListOptions {
	listOptions := &client.ListOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		listOptions.Namespace = cs.Namespace
	}
	return listOptions
}

func isPermissionDeniedError(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}

func listOverviewNodes(ctx context.Context, cs *cluster.ClientSet) (*v1.NodeList, error) {
	nodes := &v1.NodeList{}
	if err := cs.K8sClient.List(ctx, nodes, &client.ListOptions{}); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip nodes for cluster %s due to permission: %v", cs.Name, err)
			return &v1.NodeList{}, nil
		}
		return nil, err
	}
	return nodes, nil
}

func listOverviewPods(ctx context.Context, cs *cluster.ClientSet) (*v1.PodList, error) {
	pods := &v1.PodList{}
	if err := cs.K8sClient.List(ctx, pods, listOptionsForScopedNamespace(cs)); err != nil {
		return nil, err
	}
	return pods, nil
}

// fetchOverviewNamespaceQuota reads the aggregated ResourceQuota hard limits
// for a namespace-scoped cluster. It only fetches values; applying them to
// the summary happens after all parallel list calls have finished so the
// quota overwrite cannot race the allocatable accumulation.
func fetchOverviewNamespaceQuota(ctx context.Context, cs *cluster.ClientSet) (cpuQuotaMilli, memoryQuotaBytes int64, hasCPUQuota, hasMemoryQuota bool) {
	if !cs.NamespaceScoped || cs.Namespace == "" {
		return 0, 0, false, false
	}

	var quotaList v1.ResourceQuotaList
	if err := cs.K8sClient.List(ctx, &quotaList, client.InNamespace(cs.Namespace)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip resourcequotas for namespace %s due to permission: %v", cs.Namespace, err)
		} else {
			klog.Warningf("overview: failed to list resourcequotas for namespace %s: %v", cs.Namespace, err)
		}
		return 0, 0, false, false
	}

	return extractNamespaceQuotaHard(quotaList.Items)
}

// newMetadataList builds a PartialObjectMetadataList for the given list kind.
// Overview only counts namespaces/services/ingresses/PVCs, so asking for
// metadata-only objects avoids transferring and deep-copying full specs; the
// informer cache also keeps a much cheaper metadata-only informer for these.
func newMetadataList(gvk schema.GroupVersionKind) *metav1.PartialObjectMetadataList {
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvk)
	return list
}

func listOverviewNamespaces(ctx context.Context, cs *cluster.ClientSet) (*metav1.PartialObjectMetadataList, error) {
	namespaces := newMetadataList(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "NamespaceList"})
	if cs.NamespaceScoped && cs.Namespace != "" {
		// A namespace-scoped view always sees exactly its own namespace; the
		// empty metadata item keeps the count at 1 without a cluster-scoped
		// list the caller usually cannot perform.
		namespaces.Items = append(namespaces.Items, metav1.PartialObjectMetadata{})
		return namespaces, nil
	}
	if err := cs.K8sClient.List(ctx, namespaces, &client.ListOptions{}); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip namespaces for cluster %s due to permission: %v", cs.Name, err)
			return namespaces, nil
		}
		return nil, err
	}
	return namespaces, nil
}

func listOverviewServices(ctx context.Context, cs *cluster.ClientSet) (*metav1.PartialObjectMetadataList, error) {
	services := newMetadataList(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceList"})
	if err := cs.K8sClient.List(ctx, services, listOptionsForScopedNamespace(cs)); err != nil {
		return nil, err
	}
	return services, nil
}

func listOverviewIngresses(ctx context.Context, cs *cluster.ClientSet) (*metav1.PartialObjectMetadataList, error) {
	ingresses := newMetadataList(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "IngressList"})
	if err := cs.K8sClient.List(ctx, ingresses, listOptionsForScopedNamespace(cs)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip ingresses for cluster %s due to permission: %v", cs.Name, err)
			return newMetadataList(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "IngressList"}), nil
		}
		return nil, err
	}
	return ingresses, nil
}

func listOverviewPVCs(ctx context.Context, cs *cluster.ClientSet) (*metav1.PartialObjectMetadataList, error) {
	pvcs := newMetadataList(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaimList"})
	if err := cs.K8sClient.List(ctx, pvcs, listOptionsForScopedNamespace(cs)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip persistentvolumeclaims for cluster %s due to permission: %v", cs.Name, err)
			return newMetadataList(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaimList"}), nil
		}
		return nil, err
	}
	return pvcs, nil
}

func GetOverview(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if len(user.Roles) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// The dashboard polls this endpoint every 30s and every browser mount or
	// window focus fires it again, so the serialized response is cached
	// per cluster+user and refreshed in the background once it goes stale.
	body, state, err := overviewResponseCache.Serve(c.Request.Context(), overviewCacheKey(cs, user), overviewRefreshTimeout, func(ctx context.Context) ([]byte, error) {
		overview, err := buildOverview(ctx, cs)
		if err != nil {
			return nil, err
		}
		return json.Marshal(overview)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header(overviewCacheHeader, string(state))
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// buildOverview collects every overview input in parallel. Previously the
// seven list calls ran sequentially, so a cold informer or a slow API server
// multiplied the page-load latency; the lists are independent and safe to run
// concurrently.
func buildOverview(ctx context.Context, cs *cluster.ClientSet) (*OverviewData, error) {
	var (
		nodes      *v1.NodeList
		pods       *v1.PodList
		namespaces *metav1.PartialObjectMetadataList
		services   *metav1.PartialObjectMetadataList
		ingresses  *metav1.PartialObjectMetadataList
		pvcs       *metav1.PartialObjectMetadataList

		// Namespace quota values are fetched concurrently but only applied
		// after Wait, because applying them overwrites the allocatable totals
		// that collectNodeStats accumulates.
		quotaCPUMilli, quotaMemoryBytes int64
		quotaHasCPU, quotaHasMemory     bool
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		nodes, err = listOverviewNodes(gctx, cs)
		return err
	})
	g.Go(func() error {
		var err error
		pods, err = listOverviewPods(gctx, cs)
		return err
	})
	g.Go(func() error {
		var err error
		namespaces, err = listOverviewNamespaces(gctx, cs)
		return err
	})
	g.Go(func() error {
		var err error
		services, err = listOverviewServices(gctx, cs)
		return err
	})
	g.Go(func() error {
		var err error
		ingresses, err = listOverviewIngresses(gctx, cs)
		return err
	})
	g.Go(func() error {
		var err error
		pvcs, err = listOverviewPVCs(gctx, cs)
		return err
	})
	g.Go(func() error {
		// Quota lookup never fails the overview; permission or API problems
		// are logged inside the fetch and degrade to the cluster-wide basis.
		quotaCPUMilli, quotaMemoryBytes, quotaHasCPU, quotaHasMemory = fetchOverviewNamespaceQuota(gctx, cs)
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	resourceSummary := newOverviewResourceSummary()
	readyNodes := resourceSummary.collectNodeStats(nodes.Items)
	runningPods := resourceSummary.collectPodStats(pods.Items)
	if cs.NamespaceScoped && cs.Namespace != "" {
		resourceSummary.applyNamespaceQuota(quotaCPUMilli, quotaMemoryBytes, quotaHasCPU, quotaHasMemory)
	}

	return &OverviewData{
		TotalNodes:      len(nodes.Items),
		ReadyNodes:      readyNodes,
		TotalPods:       len(pods.Items),
		RunningPods:     runningPods,
		TotalNamespaces: len(namespaces.Items),
		TotalIngresses:  len(ingresses.Items),
		TotalPVCs:       len(pvcs.Items),
		TotalServices:   len(services.Items),
		PromEnabled:     cs.PromClient != nil,
		Resource:        resourceSummary.toMetric(),
	}, nil
}
