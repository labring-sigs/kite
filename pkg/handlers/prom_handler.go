package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/prometheus"
	"github.com/zxh326/kite/pkg/rbac"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PromHandler struct {
	metricsServerCache     map[string][]prometheus.UsageDataPoint
	metricsServerCacheLock sync.Mutex
}

func NewPromHandler() *PromHandler {
	h := &PromHandler{
		metricsServerCache: make(map[string][]prometheus.UsageDataPoint),
	}
	go func() {
		for {
			time.Sleep(time.Minute)
			h.metricsServerCacheLock.Lock()
			cutoff := time.Now().Add(-30 * time.Minute)
			for key, points := range h.metricsServerCache {
				var filtered []prometheus.UsageDataPoint
				for _, pt := range points {
					if pt.Timestamp.After(cutoff) {
						filtered = append(filtered, pt)
					}
				}
				if len(filtered) > 0 {
					h.metricsServerCache[key] = filtered
				} else {
					delete(h.metricsServerCache, key)
				}
			}
			h.metricsServerCacheLock.Unlock()
		}
	}()

	return h
}

func (h *PromHandler) GetResourceUsageHistory(c *gin.Context) {
	ctx := c.Request.Context()

	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	// Get query parameter for time range
	duration := c.DefaultQuery("duration", "1h")

	// Validate duration parameter
	validDurations := map[string]bool{
		"30m": true,
		"1h":  true,
		"24h": true,
	}

	if !validDurations[duration] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration. Must be one of: 30m, 1h, 24h"})
		return
	}

	// Get resource usage history if Prometheus is available
	if cs.PromClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Prometheus client not available"})
		return
	}

	instance := c.Query("instance")

	// Prometheus range queries are the most expensive part of the dashboard
	// (up to two batches of four serialized round-trips on label fallback),
	// so the serialized response is cached per cluster/user/duration/instance
	// and refreshed in the background once it goes stale.
	body, state, err := resourceUsageHistoryCache.Serve(ctx, resourceUsageHistoryCacheKey(cs, user, duration, instance), resourceUsageRefreshTimeout, func(loadCtx context.Context) ([]byte, error) {
		history, err := h.loadResourceUsageHistory(loadCtx, cs, user, instance, duration)
		if err != nil {
			return nil, err
		}
		return json.Marshal(history)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get resource usage history: %v", err)})
		return
	}
	c.Header(resourceUsageHistoryHeader, string(state))
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// loadResourceUsageHistory performs the actual Prometheus lookups behind the
// response cache: it resolves namespace quota options, tries the "instance"
// node label first and falls back to the "node" label, and degrades
// Prometheus-forbidden setups to an empty (but valid) history payload.
func (h *PromHandler) loadResourceUsageHistory(ctx context.Context, cs *cluster.ClientSet, user model.User, instance, duration string) (*prometheus.ResourceUsageHistory, error) {
	options := prometheus.ResourceUsageOptions{}
	if cs.NamespaceScoped && cs.Namespace != "" {
		options.Namespace = cs.Namespace
		cpuCapacity, memoryCapacity, hasCPUQuota, hasMemoryQuota, err := h.getNamespaceQuotaCapacities(ctx, cs, cs.Namespace)
		if err != nil {
			klog.Warningf("failed to resolve resource quota capacities for namespace %s: %v", cs.Namespace, err)
		}
		if hasCPUQuota {
			options.CPUCapacityCores = cpuCapacity
		}
		if hasMemoryQuota {
			options.MemoryCapacityByte = memoryCapacity
		}
		if !common.IsNamespaceScopeExempt(cs.Namespace) && !rbac.UserHasRole(user, model.DefaultAdminRole.Name) {
			options.DisallowClusterCapacityFallback = true
		}
	}
	resourceUsageHistory, err := cs.PromClient.GetResourceUsageHistory(ctx, instance, duration, "instance", options)
	if err != nil {
		resourceUsageHistory, err = cs.PromClient.GetResourceUsageHistory(ctx, instance, duration, "node", options)
		if err != nil {
			if prometheus.IsForbiddenError(err) {
				klog.Warningf("resource usage history forbidden by prometheus, return empty history: cluster=%s duration=%s instance=%s namespace=%s err=%v", cs.Name, duration, instance, options.Namespace, err)
				resourceUsageHistory = newEmptyResourceUsageHistory()
				applyResourceUsageHistoryMetadata(resourceUsageHistory, options)
				return resourceUsageHistory, nil
			}
			klog.Warningf("resource usage history query failed: cluster=%s duration=%s instance=%s namespace=%s err=%v", cs.Name, duration, instance, options.Namespace, err)
			return nil, err
		}
	}
	applyResourceUsageHistoryMetadata(resourceUsageHistory, options)

	return resourceUsageHistory, nil
}

func newEmptyResourceUsageHistory() *prometheus.ResourceUsageHistory {
	return &prometheus.ResourceUsageHistory{
		CPU:        []prometheus.UsageDataPoint{},
		Memory:     []prometheus.UsageDataPoint{},
		NetworkIn:  []prometheus.UsageDataPoint{},
		NetworkOut: []prometheus.UsageDataPoint{},
		DiskRead:   []prometheus.UsageDataPoint{},
		DiskWrite:  []prometheus.UsageDataPoint{},
	}
}

func applyResourceUsageHistoryMetadata(resourceUsageHistory *prometheus.ResourceUsageHistory, options prometheus.ResourceUsageOptions) {
	resourceUsageHistory.Namespace = options.Namespace
	if options.CPUCapacityCores > 0 {
		resourceUsageHistory.CPUUtilizationMode = prometheus.UtilizationModeNamespaceQuota
	} else {
		resourceUsageHistory.CPUUtilizationMode = prometheus.UtilizationModeClusterCapacity
	}
	if options.MemoryCapacityByte > 0 {
		resourceUsageHistory.MemoryUtilizationMode = prometheus.UtilizationModeNamespaceQuota
	} else {
		resourceUsageHistory.MemoryUtilizationMode = prometheus.UtilizationModeClusterCapacity
	}
}

// GetPodMetrics handles pod-specific metrics requests
func (h *PromHandler) GetPodMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)
	// Get path parameters
	namespace := c.Param("namespace")
	podName := c.Param("podName")
	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace and podName are required"})
		return
	}
	if !rbac.CanAccess(user, string(common.Pods), string(common.VerbGet), cs.Name, namespace) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": rbac.NoAccess(user.Key(), string(common.VerbGet), string(common.Pods), namespace, cs.Name),
		})
		return
	}

	// Get query parameters
	duration := c.DefaultQuery("duration", "1h")
	container := c.Query("container") // Optional container name
	labelSelector := c.Query("labelSelector")

	// Validate duration parameter
	validDurations := map[string]bool{
		"30m": true,
		"1h":  true,
		"24h": true,
	}

	if !validDurations[duration] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration. Must be one of: 30m, 1h, 24h"})
		return
	}

	// Try Prometheus first
	var podMetrics *prometheus.PodMetrics
	var err error
	if cs.PromClient != nil {
		podMetrics, err = cs.PromClient.GetPodMetrics(ctx, namespace, podName, container, duration)
		if err == nil && podMetrics != nil {
			podMetrics.Fallback = false
			c.JSON(http.StatusOK, podMetrics)
			return
		}
	}

	// Fallback: metrics-server
	podMetrics, err = h.fetchPodMetricsFromMetricsServer(c, namespace, podName, container, labelSelector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get pod metrics from both Prometheus and metrics-server: %v", err)})
		return
	}
	podMetrics.Fallback = true
	c.JSON(http.StatusOK, podMetrics)
}

func (h *PromHandler) fetchPodMetricsFromMetricsServer(c *gin.Context, namespace, podName, container, labelSelector string) (*prometheus.PodMetrics, error) {
	ctx := c.Request.Context()
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if cs.K8sClient.MetricsClient == nil {
		return nil, fmt.Errorf("metrics client not available")
	}
	h.metricsServerCacheLock.Lock()
	defer h.metricsServerCacheLock.Unlock()

	appendPoint := func(cache []prometheus.UsageDataPoint, value float64, ts time.Time) []prometheus.UsageDataPoint {
		for i := len(cache) - 1; i >= 0; i-- {
			if ts.Sub(cache[i].Timestamp) < 15*time.Second {
				cache[i].Value = value
				return cache
			}
		}
		return append(cache, prometheus.UsageDataPoint{Timestamp: ts, Value: value})
	}

	var cpuSeries, memSeries []prometheus.UsageDataPoint
	handlePodMetrics := func(podMetrics *metricsv1beta1.PodMetrics, timestamp time.Time) {
		for _, c := range podMetrics.Containers {
			key := namespace + "/" + podMetrics.Name + "/" + c.Name
			cpuUsage := float64(c.Usage.Cpu().MilliValue()) / 1000.0
			memUsage := float64(c.Usage.Memory().Value()) / 1024.0 / 1024.0
			cpuCacheKey := key + "/cpu"
			memCacheKey := key + "/mem"
			h.metricsServerCache[cpuCacheKey] = appendPoint(h.metricsServerCache[cpuCacheKey], cpuUsage, timestamp)
			h.metricsServerCache[memCacheKey] = appendPoint(h.metricsServerCache[memCacheKey], memUsage, timestamp)
			if container == "" || c.Name == container {
				cpuSeries = append(cpuSeries, h.metricsServerCache[cpuCacheKey]...)
				memSeries = append(memSeries, h.metricsServerCache[memCacheKey]...)
			}
		}
	}

	if labelSelector != "" {
		listOpts := metav1.ListOptions{LabelSelector: labelSelector}
		podMetricsList, err := cs.K8sClient.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, listOpts)
		if err != nil {
			return nil, err
		}
		if len(podMetricsList.Items) == 0 {
			return nil, fmt.Errorf("no pod metrics found")
		}
		timestamp := time.Now()
		for _, podMetrics := range podMetricsList.Items {
			handlePodMetrics(&podMetrics, timestamp)
		}
		return &prometheus.PodMetrics{
			CPU:      prometheus.NormalizeUsageDataPoints(mergeUsageDataPointsSum(cpuSeries)),
			Memory:   prometheus.NormalizeUsageDataPoints(mergeUsageDataPointsSum(memSeries)),
			Fallback: true,
		}, nil
	}

	// single pod
	podMetrics, err := cs.K8sClient.MetricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	handlePodMetrics(podMetrics, podMetrics.Timestamp.Time)
	return &prometheus.PodMetrics{
		CPU:      prometheus.NormalizeUsageDataPoints(cpuSeries),
		Memory:   prometheus.NormalizeUsageDataPoints(memSeries),
		Fallback: true,
	}, nil
}

func mergeUsageDataPointsSum(points []prometheus.UsageDataPoint) []prometheus.UsageDataPoint {
	m := make(map[int64]float64)
	for _, pt := range points {
		ts := pt.Timestamp.Unix()
		m[ts] += pt.Value
	}
	merged := []prometheus.UsageDataPoint{}
	for ts, value := range m {
		merged = append(merged, prometheus.UsageDataPoint{
			Timestamp: time.Unix(ts, 0),
			Value:     value,
		})
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Timestamp.Before(merged[j].Timestamp)
	})
	return merged
}

func (h *PromHandler) getNamespaceQuotaCapacities(ctx context.Context, cs *cluster.ClientSet, namespace string) (float64, float64, bool, bool, error) {
	var quotaList corev1.ResourceQuotaList
	if err := cs.K8sClient.List(ctx, &quotaList, client.InNamespace(namespace)); err != nil {
		return 0, 0, false, false, err
	}
	cpuCapacity, memoryCapacity, hasCPU, hasMemory := extractNamespaceQuotaCapacities(quotaList.Items)
	return cpuCapacity, memoryCapacity, hasCPU, hasMemory, nil
}

func extractNamespaceQuotaCapacities(quotas []corev1.ResourceQuota) (float64, float64, bool, bool) {
	cpuMilli, memoryBytes, hasCPU, hasMemory := extractNamespaceQuotaHard(quotas)
	return float64(cpuMilli) / 1000.0, float64(memoryBytes), hasCPU, hasMemory
}

func extractNamespaceQuotaHard(quotas []corev1.ResourceQuota) (int64, int64, bool, bool) {
	limitCPUMilli := int64(0)
	requestCPUMilli := int64(0)
	limitMemoryBytes := int64(0)
	requestMemoryBytes := int64(0)

	for _, quota := range quotas {
		hard := quota.Status.Hard
		if len(hard) == 0 {
			hard = quota.Spec.Hard
		}
		if q, ok := hard[corev1.ResourceLimitsCPU]; ok {
			limitCPUMilli += q.MilliValue()
		}
		if q, ok := hard[corev1.ResourceRequestsCPU]; ok {
			requestCPUMilli += q.MilliValue()
		}
		if q, ok := hard[corev1.ResourceLimitsMemory]; ok {
			limitMemoryBytes += q.Value()
		}
		if q, ok := hard[corev1.ResourceRequestsMemory]; ok {
			requestMemoryBytes += q.Value()
		}
	}

	cpuMilli := limitCPUMilli
	if cpuMilli == 0 {
		cpuMilli = requestCPUMilli
	}
	memoryBytes := limitMemoryBytes
	if memoryBytes == 0 {
		memoryBytes = requestMemoryBytes
	}
	return cpuMilli, memoryBytes, cpuMilli > 0, memoryBytes > 0
}
