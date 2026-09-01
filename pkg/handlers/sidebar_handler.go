package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"golang.org/x/sync/errgroup"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var builtinSidebarCRDNames = []string{
	"apps.app.sealos.io",
	"devboxes.devbox.sealos.io",
	"rabbitmqclusters.rabbitmq.com",
	"elasticsearches.elasticsearch.k8s.elastic.co",
	"clusters.apps.kubeblocks.io",
	// SealosNetworkPolicy is namespaced and intended to be visible to every
	// workspace user.
	"sealosnetworkpolicies.networking.sealos.io",
	// CiliumEgressGatewayPolicy is cluster-scoped; workspace credentials
	// usually cannot read it, so it only shows up for admin kubeconfigs (the
	// handler skips entries the caller cannot read instead of failing).
	"ciliumegressgatewaypolicies.cilium.io",
}

type sidebarCRDVersion struct {
	Name                     string                                           `json:"name"`
	Served                   bool                                             `json:"served"`
	Storage                  bool                                             `json:"storage"`
	AdditionalPrinterColumns []apiextensionsv1.CustomResourceColumnDefinition `json:"additionalPrinterColumns,omitempty"`
}

type sidebarCRDInfo struct {
	Name     string              `json:"name"`
	Kind     string              `json:"kind"`
	Group    string              `json:"group"`
	Scope    string              `json:"scope"`
	Versions []sidebarCRDVersion `json:"versions"`
}

func ListBuiltinSidebarCRDs(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	// CRD presence changes rarely, so the serialized discovery response is
	// cached per cluster+user instead of re-reading the same CRDs on every
	// page load.
	body, state, err := sidebarCRDResponseCache.Serve(c.Request.Context(), sidebarCRDCacheKey(cs, user), sidebarCRDRefreshTimeout, func(ctx context.Context) ([]byte, error) {
		items, err := listBuiltinSidebarCRDItems(ctx, cs)
		if err != nil {
			return nil, err
		}
		return json.Marshal(gin.H{"items": items})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header(sidebarCRDCacheHeader, string(state))
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// listBuiltinSidebarCRDItems resolves every builtin CRD concurrently. The
// lookups are independent; CRDs that are not installed (NotFound) or not
// readable by the caller's credentials (Forbidden, e.g. the cluster-scoped
// CiliumEgressGatewayPolicy for namespace-scoped workspace users) are
// skipped instead of failing the whole sidebar response. Unauthorized (401)
// is NOT skipped: it signals broken cluster credentials rather than an
// admin-only resource, and must surface as an error. Results keep the
// declaration order via their index slots.
func listBuiltinSidebarCRDItems(ctx context.Context, cs *cluster.ClientSet) ([]sidebarCRDInfo, error) {
	results := make([]*sidebarCRDInfo, len(builtinSidebarCRDNames))

	g, gctx := errgroup.WithContext(ctx)
	for i, crdName := range builtinSidebarCRDNames {
		g.Go(func() error {
			var crd apiextensionsv1.CustomResourceDefinition
			if err := cs.K8sClient.Get(gctx, types.NamespacedName{Name: crdName}, &crd); err != nil {
				if apierrors.IsNotFound(err) {
					// CRD not installed on this cluster: leave the slot empty.
					return nil
				}
				if apierrors.IsForbidden(err) {
					// The credentials may not see this CRD (admin-only entry);
					// hide it for this caller rather than breaking the sidebar.
					klog.V(1).Infof("sidebar: skip builtin CRD %s for cluster %s due to permissions: %v", crdName, cs.Name, err)
					return nil
				}
				return err
			}
			results[i] = buildSidebarCRDInfo(&crd)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	items := make([]sidebarCRDInfo, 0, len(results))
	for _, info := range results {
		if info != nil {
			items = append(items, *info)
		}
	}
	return items, nil
}

// buildSidebarCRDInfo converts a CRD into its sidebar representation, keeping
// only the fields the UI needs to render the entry.
func buildSidebarCRDInfo(crd *apiextensionsv1.CustomResourceDefinition) *sidebarCRDInfo {
	versions := make([]sidebarCRDVersion, 0, len(crd.Spec.Versions))
	for _, version := range crd.Spec.Versions {
		versions = append(versions, sidebarCRDVersion{
			Name:                     version.Name,
			Served:                   version.Served,
			Storage:                  version.Storage,
			AdditionalPrinterColumns: version.AdditionalPrinterColumns,
		})
	}

	return &sidebarCRDInfo{
		Name:     crd.Name,
		Kind:     crd.Spec.Names.Kind,
		Group:    crd.Spec.Group,
		Scope:    string(crd.Spec.Scope),
		Versions: versions,
	}
}
