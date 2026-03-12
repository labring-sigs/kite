package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

func applyOverviewNamespaceQuota(ctx context.Context, cs *cluster.ClientSet, summary *overviewResourceSummary) {
	if !cs.NamespaceScoped || cs.Namespace == "" {
		return
	}

	var quotaList v1.ResourceQuotaList
	if err := cs.K8sClient.List(ctx, &quotaList, client.InNamespace(cs.Namespace)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip resourcequotas for namespace %s due to permission: %v", cs.Namespace, err)
		} else {
			klog.Warningf("overview: failed to list resourcequotas for namespace %s: %v", cs.Namespace, err)
		}
		return
	}

	cpuQuotaMilli, memoryQuotaBytes, hasCPUQuota, hasMemoryQuota := extractNamespaceQuotaHard(quotaList.Items)
	summary.applyNamespaceQuota(cpuQuotaMilli, memoryQuotaBytes, hasCPUQuota, hasMemoryQuota)
}

func listOverviewNamespaces(ctx context.Context, cs *cluster.ClientSet) (*v1.NamespaceList, error) {
	namespaces := &v1.NamespaceList{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		namespaces.Items = append(namespaces.Items, v1.Namespace{})
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

func listOverviewServices(ctx context.Context, cs *cluster.ClientSet) (*v1.ServiceList, error) {
	services := &v1.ServiceList{}
	if err := cs.K8sClient.List(ctx, services, listOptionsForScopedNamespace(cs)); err != nil {
		return nil, err
	}
	return services, nil
}

func listOverviewIngresses(ctx context.Context, cs *cluster.ClientSet) (*networkingv1.IngressList, error) {
	ingresses := &networkingv1.IngressList{}
	if err := cs.K8sClient.List(ctx, ingresses, listOptionsForScopedNamespace(cs)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip ingresses for cluster %s due to permission: %v", cs.Name, err)
			return &networkingv1.IngressList{}, nil
		}
		return nil, err
	}
	return ingresses, nil
}

func listOverviewPVCs(ctx context.Context, cs *cluster.ClientSet) (*v1.PersistentVolumeClaimList, error) {
	pvcs := &v1.PersistentVolumeClaimList{}
	if err := cs.K8sClient.List(ctx, pvcs, listOptionsForScopedNamespace(cs)); err != nil {
		if isPermissionDeniedError(err) {
			klog.Warningf("overview: skip persistentvolumeclaims for cluster %s due to permission: %v", cs.Name, err)
			return &v1.PersistentVolumeClaimList{}, nil
		}
		return nil, err
	}
	return pvcs, nil
}

func GetOverview(c *gin.Context) {
	ctx := c.Request.Context()

	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	if len(user.Roles) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	nodes, err := listOverviewNodes(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pods, err := listOverviewPods(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resourceSummary := newOverviewResourceSummary()
	readyNodes := resourceSummary.collectNodeStats(nodes.Items)
	runningPods := resourceSummary.collectPodStats(pods.Items)
	applyOverviewNamespaceQuota(ctx, cs, &resourceSummary)

	namespaces, err := listOverviewNamespaces(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	services, err := listOverviewServices(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ingresses, err := listOverviewIngresses(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pvcs, err := listOverviewPVCs(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	overview := OverviewData{
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
	}

	c.JSON(http.StatusOK, overview)
}

// var (
// 	initialized bool
// )

func InitCheck(c *gin.Context) {
	// if initialized {
	// 	c.JSON(http.StatusOK, gin.H{"initialized": true})
	// 	return
	// }
	step := 0
	uc, _ := model.CountUsers()
	if uc == 0 && !common.AnonymousUserEnabled {
		c.SetCookie("auth_token", "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"initialized": false, "step": step})
		return
	}
	if uc > 0 || common.AnonymousUserEnabled {
		step++
	}
	cc, _ := model.CountClusters()
	if cc > 0 {
		step++
	}
	initialized := step == 2
	c.JSON(http.StatusOK, gin.H{"initialized": initialized, "step": step})
}
