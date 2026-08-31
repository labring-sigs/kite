package prometheus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"
)

type Client struct {
	client v1.API
}

const (
	UtilizationModeClusterCapacity = "cluster_capacity"
	UtilizationModeNamespaceQuota  = "namespace_quota"
)

type ResourceMetrics struct {
	CPURequest    float64
	CPUTotal      float64
	MemoryRequest float64
	MemoryTotal   float64
}

// UsageDataPoint represents a single time point in usage metrics
type UsageDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ResourceUsageHistory contains historical usage data for a resource
type ResourceUsageHistory struct {
	CPU                   []UsageDataPoint `json:"cpu"`
	Memory                []UsageDataPoint `json:"memory"`
	NetworkIn             []UsageDataPoint `json:"networkIn"`
	NetworkOut            []UsageDataPoint `json:"networkOut"`
	DiskRead              []UsageDataPoint `json:"diskRead"`
	DiskWrite             []UsageDataPoint `json:"diskWrite"`
	Namespace             string           `json:"namespace,omitempty"`
	CPUUtilizationMode    string           `json:"cpuUtilizationMode,omitempty"`
	MemoryUtilizationMode string           `json:"memoryUtilizationMode,omitempty"`
}

// PodMetrics contains metrics for a specific pod
type PodMetrics struct {
	CPU        []UsageDataPoint `json:"cpu"`
	Memory     []UsageDataPoint `json:"memory"`
	NetworkIn  []UsageDataPoint `json:"networkIn"`
	NetworkOut []UsageDataPoint `json:"networkOut"`
	DiskRead   []UsageDataPoint `json:"diskRead"`
	DiskWrite  []UsageDataPoint `json:"diskWrite"`
	Fallback   bool             `json:"fallback"`
}

type PodCurrentMetrics struct {
	PodName   string  `json:"podName"`
	Namespace string  `json:"namespace"`
	CPU       float64 `json:"cpu"`    // CPU cores
	Memory    float64 `json:"memory"` // Memory in MB
}

type ResourceUsageOptions struct {
	Namespace                       string
	CPUCapacityCores                float64
	MemoryCapacityByte              float64
	DisallowClusterCapacityFallback bool
}

func NewClientWithRoundTripper(prometheusURL string, rt http.RoundTripper) (*Client, error) {
	if prometheusURL == "" {
		return nil, fmt.Errorf("prometheus URL cannot be empty")
	}
	client, err := api.NewClient(api.Config{
		Address:      prometheusURL,
		RoundTripper: rt,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	v1api := v1.NewAPI(client)
	return &Client{
		client: v1api,
	}, nil
}

// GetResourceUsageHistory fetches historical usage data for CPU and Memory
func (c *Client) GetResourceUsageHistory(ctx context.Context, instance string, duration string, nodeLabel string, options ResourceUsageOptions) (*ResourceUsageHistory, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 1 * time.Minute
	case "1h":
		timeRange = 1 * time.Hour
		step = 2 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 30 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	now := time.Now()
	start := now.Add(-timeRange)

	cpuQuery, memoryQuery, networkInQuery, networkOutQuery := buildResourceUsageQueries(instance, nodeLabel, options)

	// The four range queries are independent, so they run concurrently; this
	// shaves the dashboard chart latency from the sum of four Prometheus
	// round-trips down to the slowest single one. Each goroutine writes only
	// its own result/error slots, so no extra synchronization is needed
	// beyond the WaitGroup.
	var (
		cpuData, memoryData, networkInData, networkOutData []UsageDataPoint
		cpuErr, memoryErr, networkInErr, networkOutErr     error
		wg                                                 sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		if cpuQuery == "" {
			// CPU utilization may be intentionally disabled (namespace quota
			// without cluster-capacity fallback); leave the series empty.
			cpuData = []UsageDataPoint{}
			return
		}
		cpuData, cpuErr = c.queryRange(ctx, cpuQuery, start, now, step)
	}()
	go func() {
		defer wg.Done()
		if memoryQuery == "" {
			memoryData = []UsageDataPoint{}
			return
		}
		memoryData, memoryErr = c.queryRange(ctx, memoryQuery, start, now, step)
	}()
	go func() {
		defer wg.Done()
		networkInData, networkInErr = c.queryRange(ctx, networkInQuery, start, now, step)
	}()
	go func() {
		defer wg.Done()
		networkOutData, networkOutErr = c.queryRange(ctx, networkOutQuery, start, now, step)
	}()
	wg.Wait()

	// CPU and memory failures are fatal (they are the primary chart series);
	// network failures degrade to empty series exactly as before.
	if cpuErr != nil {
		return nil, fmt.Errorf("error querying CPU usage: %w", cpuErr)
	}
	if memoryErr != nil {
		return nil, fmt.Errorf("error querying Memory usage: %w", memoryErr)
	}
	if networkInErr != nil {
		klog.Warningf("network incoming query failed, fallback to empty series: query=%s err=%v", networkInQuery, networkInErr)
		networkInData = []UsageDataPoint{}
	}
	if networkOutErr != nil {
		klog.Warningf("network outgoing query failed, fallback to empty series: query=%s err=%v", networkOutQuery, networkOutErr)
		networkOutData = []UsageDataPoint{}
	}

	if len(cpuData) == 0 && len(memoryData) == 0 && len(networkInData) == 0 && len(networkOutData) == 0 {
		return nil, fmt.Errorf("metrics-server or kube-state-metrics may not be available or configured correctly")
	}

	return &ResourceUsageHistory{
		CPU:        NormalizeUsageDataPoints(cpuData),
		Memory:     NormalizeUsageDataPoints(memoryData),
		NetworkIn:  NormalizeUsageDataPoints(networkInData),
		NetworkOut: NormalizeUsageDataPoints(networkOutData),
		DiskRead:   []UsageDataPoint{},
		DiskWrite:  []UsageDataPoint{},
	}, nil
}

func NormalizeUsageDataPoints(points []UsageDataPoint) []UsageDataPoint {
	if points == nil {
		return []UsageDataPoint{}
	}
	return points
}

func buildResourceUsageQueries(instance, nodeLabel string, options ResourceUsageOptions) (string, string, string, string) {
	containerConditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if options.Namespace != "" {
		containerConditions = append(containerConditions, fmt.Sprintf(`namespace="%s"`, options.Namespace))
	}
	if instance != "" {
		containerConditions = append(containerConditions, fmt.Sprintf(`%s="%s"`, nodeLabel, instance))
	}

	cpuQuery := ""
	if options.CPUCapacityCores > 0 {
		cpuQuery = fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m])) / %s * 100`, strings.Join(containerConditions, ","), formatPromQLScalar(options.CPUCapacityCores))
	} else if options.Namespace != "" && options.DisallowClusterCapacityFallback {
		cpuQuery = ""
	} else {
		cpuConditions := []string{
			`resource="cpu"`,
		}
		if instance != "" {
			cpuConditions = append(cpuConditions, fmt.Sprintf(`node="%s"`, instance))
		}
		cpuQuery = fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m])) / sum(kube_node_status_allocatable{%s}) * 100`, strings.Join(containerConditions, ","), strings.Join(cpuConditions, ","))
	}

	memoryQuery := ""
	if options.MemoryCapacityByte > 0 {
		memoryQuery = fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / %s * 100`, strings.Join(containerConditions, ","), formatPromQLScalar(options.MemoryCapacityByte))
	} else if options.Namespace != "" && options.DisallowClusterCapacityFallback {
		memoryQuery = ""
	} else {
		memoryConditions := []string{
			`resource="memory"`,
		}
		if instance != "" {
			memoryConditions = append(memoryConditions, fmt.Sprintf(`node="%s"`, instance))
		}
		memoryQuery = fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, strings.Join(containerConditions, ","), strings.Join(memoryConditions, ","))
	}

	networkInQuery, networkOutQuery := buildNetworkUsageQueries(instance, nodeLabel, options)
	return cpuQuery, memoryQuery, networkInQuery, networkOutQuery
}

func buildNetworkUsageQueries(instance, nodeLabel string, options ResourceUsageOptions) (string, string) {
	// For cluster-wide overview (no namespace and no node), prefer node-level metrics
	// to avoid high-cardinality container-level scans.
	if options.Namespace == "" && instance == "" {
		return `sum(rate(node_network_receive_bytes_total{device!="lo"}[1m]))`,
			`sum(rate(node_network_transmit_bytes_total{device!="lo"}[1m]))`
	}

	networkConditions := []string{}
	if options.Namespace != "" {
		networkConditions = append(networkConditions, fmt.Sprintf(`namespace="%s"`, options.Namespace))
	}
	if instance != "" {
		networkConditions = append(networkConditions, fmt.Sprintf(`%s="%s"`, nodeLabel, instance))
	}

	return fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, strings.Join(networkConditions, ",")),
		fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, strings.Join(networkConditions, ","))
}

func formatPromQLScalar(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (c *Client) queryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	r := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	result, warnings, err := c.client.QueryRange(ctx, query, r)
	if err != nil {
		klog.Error("queryRange", "error", err)
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	dataPoints := []UsageDataPoint{}

	switch result.Type() {
	case model.ValMatrix:
		matrix := result.(model.Matrix)
		if len(matrix) > 0 {
			for _, sample := range matrix[0].Values {
				dataPoints = append(dataPoints, UsageDataPoint{
					Timestamp: sample.Timestamp.Time(),
					Value:     float64(sample.Value),
				})
			}
		}
	default:
		return nil, fmt.Errorf("unexpected result type: %s", result.Type())
	}

	return dataPoints, nil
}

func IsClientErrorStatus(err error, statusCode int) bool {
	if err == nil {
		return false
	}

	var promErr *v1.Error
	if errors.As(err, &promErr) {
		if promErr.Type != v1.ErrClient {
			return false
		}
		return strings.Contains(promErr.Msg, fmt.Sprintf("client error: %d", statusCode))
	}

	return false
}

func IsForbiddenError(err error) bool {
	return IsClientErrorStatus(err, http.StatusForbidden)
}

// Query executes an instant query against Prometheus.
func (c *Client) Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.Query(ctx, query, ts, opts...)
}

// QueryRange executes a range query against Prometheus.
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.QueryRange(ctx, query, r, opts...)
}

// HealthCheck verifies if Prometheus is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Config(ctx)
	return err
}

func (c *Client) GetCPUUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	// Build query conditionally based on whether pod name prefix and container are provided
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func (c *Client) GetMemoryUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	// Build query conditionally based on whether pod name prefix and container are provided
	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / 1024 / 1024`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func (c *Client) GetNetworkInUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	conditions := []string{}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func (c *Client) GetNetworkOutUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	conditions := []string{}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func (c *Client) GetDiskReadUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func (c *Client) GetDiskWriteUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) ([]UsageDataPoint, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	conditions := []string{
		`container!="POD"`, // Exclude the "POD" container
		`container!=""`,    // Exclude empty containers
	}
	if podNamePrefix != "" {
		conditions = append(conditions, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if container != "" {
		conditions = append(conditions, fmt.Sprintf(`container="%s"`, container))
	}
	if namespace != "" {
		conditions = append(conditions, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	query := fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, strings.Join(conditions, ","))
	return c.queryRange(ctx, query, start, now, step)
}

func FillMissingDataPoints(timeRange time.Duration, step time.Duration, existing []UsageDataPoint) []UsageDataPoint {
	if len(existing) == 0 {
		return existing
	}

	startTime := time.Now().Add(-timeRange)
	firstTime := existing[0].Timestamp

	if firstTime.Sub(startTime) <= step {
		return existing
	}

	result := []UsageDataPoint{}
	for t := startTime.Add(step); t.Before(firstTime); t = t.Add(step) {
		result = append(result, UsageDataPoint{
			Timestamp: t,
			Value:     0.0,
		})
	}

	return append(result, existing...)
}

// GetPodMetrics fetches metrics for a specific pod
func (c *Client) GetPodMetrics(ctx context.Context, namespace, podName, container string, duration string) (*PodMetrics, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 15 * time.Second
	case "1h":
		timeRange = 1 * time.Hour
		step = 1 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 5 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	cpuData, err := c.GetCPUUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod CPU usage: %w", err)
	}
	// Memory usage query for specific pod
	memoryData, err := c.GetMemoryUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Memory usage: %w", err)
	}

	networkInData, err := c.GetNetworkInUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network incoming usage: %w", err)
	}

	networkOutData, err := c.GetNetworkOutUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network outgoing usage: %w", err)
	}

	diskReadData, err := c.GetDiskReadUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk read usage: %w", err)
	}

	diskWriteData, err := c.GetDiskWriteUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk write usage: %w", err)
	}

	return &PodMetrics{
		CPU:        NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, cpuData)),
		Memory:     NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, memoryData)),
		NetworkIn:  NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, networkInData)),
		NetworkOut: NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, networkOutData)),
		DiskRead:   NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, diskReadData)),
		DiskWrite:  NormalizeUsageDataPoints(FillMissingDataPoints(timeRange, step, diskWriteData)),
		Fallback:   false,
	}, nil
}
