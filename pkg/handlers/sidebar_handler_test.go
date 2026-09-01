package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestListBuiltinSidebarCRDsReturnsExistingBuiltins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Keep the per-cluster response cache from leaking results between tests.
	sidebarCRDResponseCache.Clear()

	scheme := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "devboxes.devbox.sealos.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "devbox.sealos.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "devboxes",
				Kind:   "Devbox",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:     "Status",
							Type:     "string",
							JSONPath: ".status.phase",
						},
					},
				},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sidebar/builtin-crds", nil)
	c.Set("cluster", &cluster.ClientSet{
		K8sClient: &kube.K8sClient{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(crd).
				Build(),
		},
	})
	c.Set("user", model.User{Username: "tester"})

	ListBuiltinSidebarCRDs(c)

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Items []sidebarCRDInfo `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	assert.Equal(t, "devboxes.devbox.sealos.io", response.Items[0].Name)
	assert.Equal(t, "Devbox", response.Items[0].Kind)
	assert.Equal(t, string(apiextensionsv1.NamespaceScoped), response.Items[0].Scope)
	require.Len(t, response.Items[0].Versions, 1)
	assert.Equal(t, "Status", response.Items[0].Versions[0].AdditionalPrinterColumns[0].Name)
}

// TestListBuiltinSidebarCRDsSkipsForbiddenEntries verifies that CRDs the
// caller cannot read (e.g. the cluster-scoped CiliumEgressGatewayPolicy for
// namespace-scoped workspace users) are hidden from the sidebar instead of
// failing the whole endpoint with a 500.
func TestListBuiltinSidebarCRDsSkipsForbiddenEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Keep the per-cluster response cache from leaking results between tests.
	sidebarCRDResponseCache.Clear()

	scheme := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))

	readableCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "devboxes.devbox.sealos.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "devbox.sealos.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "devboxes",
				Kind:   "Devbox",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	// The fake client has no RBAC, so simulate a workspace credential that is
	// forbidden from reading the admin-only Cilium CRD via an interceptor.
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"},
		"ciliumegressgatewaypolicies.cilium.io",
		nil,
	)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(readableCRD).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == "ciliumegressgatewaypolicies.cilium.io" {
					return forbidden
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sidebar/builtin-crds", nil)
	c.Set("cluster", &cluster.ClientSet{
		K8sClient: &kube.K8sClient{Client: fakeClient},
	})
	c.Set("user", model.User{Username: "tester"})

	ListBuiltinSidebarCRDs(c)

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Items []sidebarCRDInfo `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// Only the readable CRD is returned; the forbidden one is skipped.
	require.Len(t, response.Items, 1)
	assert.Equal(t, "devboxes.devbox.sealos.io", response.Items[0].Name)
}

// TestListBuiltinSidebarCRDsSurfacesUnauthorized verifies that a 401 from
// the API server (broken cluster credentials, not an admin-only resource) is
// surfaced as an error instead of being silently swallowed like a 403.
func TestListBuiltinSidebarCRDsSurfacesUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sidebarCRDResponseCache.Clear()

	scheme := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// Every CRD read fails as it would with expired credentials.
				return apierrors.NewUnauthorized("token expired")
			},
		}).
		Build()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sidebar/builtin-crds", nil)
	c.Set("cluster", &cluster.ClientSet{
		K8sClient: &kube.K8sClient{Client: fakeClient},
	})
	c.Set("user", model.User{Username: "tester"})

	ListBuiltinSidebarCRDs(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
