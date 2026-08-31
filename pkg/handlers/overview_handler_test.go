package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newOverviewTestContext builds a gin context backed by a fake Kubernetes
// client with one object of every kind the overview counts.
func newOverviewTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("4"),
				v1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "app",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("500m"),
							v1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}
	namespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	serviceA := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default"}}
	serviceB := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "default"}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ing-1", Namespace: "default"}}
	pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Namespace: "default"}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	c.Set("cluster", &cluster.ClientSet{
		Name: "overview-test",
		K8sClient: &kube.K8sClient{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(node, pod, namespace, serviceA, serviceB, ingress, pvc).
				Build(),
		},
	})
	c.Set("user", model.User{
		Username: "tester",
		Roles:    []common.Role{{Name: "admin"}},
	})
	return c, w
}

func TestGetOverviewCountsResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Start from a clean cache so the test does not depend on execution order.
	overviewResponseCache.Clear()

	c, w := newOverviewTestContext(t)
	GetOverview(c)

	require.Equal(t, http.StatusOK, w.Code)

	var overview OverviewData
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &overview))

	assert.Equal(t, 1, overview.TotalNodes)
	assert.Equal(t, 1, overview.ReadyNodes)
	assert.Equal(t, 1, overview.TotalPods)
	assert.Equal(t, 1, overview.RunningPods)
	assert.Equal(t, 1, overview.TotalNamespaces)
	assert.Equal(t, 2, overview.TotalServices)
	assert.Equal(t, 1, overview.TotalIngresses)
	assert.Equal(t, 1, overview.TotalPVCs)

	// The resource summary must aggregate node allocatable and pod requests.
	assert.Equal(t, int64(4000), overview.Resource.CPU.Allocatable)
	assert.Equal(t, int64(500), overview.Resource.CPU.Requested)
	assert.Equal(t, int64(1000), overview.Resource.CPU.Limited)
	assert.Positive(t, overview.Resource.Mem.Allocatable)

	// First call computes the response; the cache header reports a miss.
	assert.Equal(t, "miss", w.Header().Get(overviewCacheHeader))
}

func TestGetOverviewServesSecondCallFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	overviewResponseCache.Clear()

	c, w := newOverviewTestContext(t)
	GetOverview(c)
	require.Equal(t, http.StatusOK, w.Code)

	c2, w2 := newOverviewTestContext(t)
	GetOverview(c2)
	require.Equal(t, http.StatusOK, w2.Code)

	// The identical second request must come from the cache.
	assert.Equal(t, "hit", w2.Header().Get(overviewCacheHeader))
	assert.JSONEq(t, w.Body.String(), w2.Body.String())
}

func TestGetOverviewForbiddenWithoutRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	overviewResponseCache.Clear()

	c, w := newOverviewTestContext(t)
	c.Set("user", model.User{Username: "no-roles"})
	GetOverview(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
