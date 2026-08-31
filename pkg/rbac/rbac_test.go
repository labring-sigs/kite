package rbac

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestCanAccess(t *testing.T) {
	// Define test roles
	adminRole := common.Role{
		Name:        "admin",
		Description: "Administrator with full access",
		Clusters:    []string{"*"},
		Resources:   []string{"*"},
		Namespaces:  []string{"*"},
		Verbs:       []string{"*"},
	}

	viewerRole := common.Role{
		Name:        "viewer",
		Description: "Read-only access to all resources",
		Clusters:    []string{"*"},
		Resources:   []string{"*"},
		Namespaces:  []string{"*"},
		Verbs:       []string{"get"},
	}

	devRole := common.Role{
		Name:        "developer",
		Description: "Developer access to specific resources",
		Clusters:    []string{"dev-cluster"},
		Resources:   []string{"pod", "deployment"},
		Namespaces:  []string{"dev", "test"},
		Verbs:       []string{"get", "create", "update", "delete"},
	}

	regexpDevRole := common.Role{
		Name:        "developer-regexp",
		Description: "Developer access to specific resources by regexp",
		Clusters:    []string{"dev.*"},
		Resources:   []string{"pod", "deployment"},
		Namespaces:  []string{"dev.*", "test.*"},
		Verbs:       []string{"get", "create", "update", "delete"},
	}

	prodViewRole := common.Role{
		Name:        "prod-viewer",
		Description: "Read-only access to production",
		Clusters:    []string{"prod-cluster"},
		Resources:   []string{"pod", "service"},
		Namespaces:  []string{"prod"},
		Verbs:       []string{"get"},
	}

	notKubeSystemRole := common.Role{
		Name:        "not-kube-system",
		Description: "Access to all namespaces except kube-system",
		Clusters:    []string{"*"},
		Resources:   []string{"*"},
		Namespaces:  []string{"!kube-system", "*"},
		Verbs:       []string{"*"},
	}

	tests := []struct {
		name       string
		roles      []common.Role
		mappings   []common.RoleMapping
		user       string
		oidcGroups []string
		resource   string
		verb       string
		cluster    string
		namespace  string
		expected   bool
	}{
		{
			name:  "user with no permissions",
			roles: []common.Role{adminRole, viewerRole},
			mappings: []common.RoleMapping{
				{Name: "admin", Users: []string{"admin-user"}},
				{Name: "viewer", Users: []string{"viewer-user"}},
			},
			user:       "unprivileged-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "dev-cluster",
			namespace:  "default",
			expected:   false,
		},
		{
			name:  "admin user can access anything",
			roles: []common.Role{adminRole},
			mappings: []common.RoleMapping{
				{Name: "admin", Users: []string{"admin-user"}},
			},
			user:       "admin-user",
			oidcGroups: []string{},
			resource:   "any-resource",
			verb:       "any-verb",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   true,
		},
		{
			name:  "viewer can only read",
			roles: []common.Role{viewerRole},
			mappings: []common.RoleMapping{
				{Name: "viewer", Users: []string{"viewer-user"}},
			},
			user:       "viewer-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   true,
		},
		{
			name:  "viewer cannot write",
			roles: []common.Role{viewerRole},
			mappings: []common.RoleMapping{
				{Name: "viewer", Users: []string{"viewer-user"}},
			},
			user:       "viewer-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "create",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   false,
		},
		{
			name:  "developer in correct cluster/namespace/resource",
			roles: []common.Role{devRole},
			mappings: []common.RoleMapping{
				{Name: "developer", Users: []string{"dev-user"}},
			},
			user:       "dev-user",
			oidcGroups: []string{},
			resource:   "deployment",
			verb:       "update",
			cluster:    "dev-cluster",
			namespace:  "dev",
			expected:   true,
		},
		{
			name:  "developer in wrong cluster",
			roles: []common.Role{devRole},
			mappings: []common.RoleMapping{
				{Name: "developer", Users: []string{"dev-user"}},
			},
			user:       "dev-user",
			oidcGroups: []string{},
			resource:   "deployment",
			verb:       "update",
			cluster:    "prod-cluster",
			namespace:  "dev",
			expected:   false,
		},
		{
			name:  "developer in correct cluster/namespace/resource by regexp",
			roles: []common.Role{regexpDevRole},
			mappings: []common.RoleMapping{
				{Name: "developer-regexp", Users: []string{"dev-user"}},
			},
			user:       "dev-user",
			oidcGroups: []string{},
			resource:   "deployment",
			verb:       "update",
			cluster:    "dev-cluster",
			namespace:  "dev",
			expected:   true,
		},
		{
			name:  "developer in wrong cluster by regexp",
			roles: []common.Role{regexpDevRole},
			mappings: []common.RoleMapping{
				{Name: "developer-regexp", Users: []string{"dev-user"}},
			},
			user:       "dev-user",
			oidcGroups: []string{},
			resource:   "deployment",
			verb:       "update",
			cluster:    "prod-cluster",
			namespace:  "dev",
			expected:   false,
		},
		{
			name:  "user with multiple roles",
			roles: []common.Role{devRole, prodViewRole},
			mappings: []common.RoleMapping{
				{Name: "developer", Users: []string{"multi-role-user"}},
				{Name: "prod-viewer", Users: []string{"multi-role-user"}},
			},
			user:       "multi-role-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "prod-cluster",
			namespace:  "prod",
			expected:   true,
		},
		{
			name:  "user with OIDC group permissions",
			roles: []common.Role{viewerRole},
			mappings: []common.RoleMapping{
				{Name: "viewer", OIDCGroups: []string{"viewers-group"}},
			},
			user:       "group-member",
			oidcGroups: []string{"viewers-group"},
			resource:   "pod",
			verb:       "get",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   true,
		},
		{
			name:  "wildcard in user list",
			roles: []common.Role{viewerRole},
			mappings: []common.RoleMapping{
				{Name: "viewer", Users: []string{"*"}},
			},
			user:       "any-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   true,
		},
		{
			name:  "allow all-namespace but not kube-system: access",
			roles: []common.Role{notKubeSystemRole},
			mappings: []common.RoleMapping{
				{Name: "not-kube-system", Users: []string{"*"}},
			},
			user:       "any-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "any-cluster",
			namespace:  "any-namespace",
			expected:   true,
		},
		{
			name:  "allow all-namespace but not kube-system: not access",
			roles: []common.Role{notKubeSystemRole},
			mappings: []common.RoleMapping{
				{Name: "not-kube-system", Users: []string{"*"}},
			},
			user:       "any-user",
			oidcGroups: []string{},
			resource:   "pod",
			verb:       "get",
			cluster:    "any-cluster",
			namespace:  "kube-system",
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RBACConfig = &common.RolesConfig{
				Roles:       tc.roles,
				RoleMapping: tc.mappings,
			}
			result := CanAccess(model.User{Username: tc.user, OIDCGroups: tc.oidcGroups}, tc.resource, tc.verb, tc.cluster, tc.namespace)

			if result != tc.expected {
				t.Errorf("Expected CanAccess to return %v but got %v", tc.expected, result)
			}
		})
	}
}

// TestMatchPreservesSemantics locks in the behavior of match() after regex
// compilation was memoized: exact, wildcard, negation and regex rules must
// behave exactly as before.
func TestMatchPreservesSemantics(t *testing.T) {
	tests := []struct {
		name string
		list []string
		val  string
		want bool
	}{
		{name: "wildcard matches anything", list: []string{"*"}, val: "pods", want: true},
		{name: "exact match", list: []string{"pods"}, val: "pods", want: true},
		{name: "no match", list: []string{"pods"}, val: "nodes", want: false},
		{name: "negation blocks exact value", list: []string{"!secrets", "*"}, val: "secrets", want: false},
		{name: "negation keeps other values", list: []string{"!secrets", "*"}, val: "pods", want: true},
		{name: "regex pattern match", list: []string{"^team-.*"}, val: "team-a", want: true},
		{name: "regex pattern mismatch", list: []string{"^team-.*"}, val: "other", want: false},
		{name: "invalid regex never matches", list: []string{"["}, val: "anything", want: false},
		{name: "empty list never matches", list: []string{}, val: "pods", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run twice so both the compile-once path and the memoized path
			// are exercised and must agree.
			if got := match(tc.list, tc.val); got != tc.want {
				t.Fatalf("match(%v, %q) = %v, want %v", tc.list, tc.val, got, tc.want)
			}
			if got := match(tc.list, tc.val); got != tc.want {
				t.Fatalf("memoized match(%v, %q) = %v, want %v", tc.list, tc.val, got, tc.want)
			}
		})
	}
}

// TestClearCompiledPatternCache ensures memoized patterns are dropped, which
// is how role edits propagate after a config reload.
func TestClearCompiledPatternCache(t *testing.T) {
	pattern := "^cached-.*"
	if !matchPattern(pattern, "cached-value") {
		t.Fatalf("expected pattern to match before clearing")
	}
	if _, ok := compiledPatternCache.Load(pattern); !ok {
		t.Fatalf("expected pattern to be memoized")
	}

	ClearCompiledPatternCache()

	if _, ok := compiledPatternCache.Load(pattern); ok {
		t.Fatalf("expected pattern cache to be cleared")
	}
	// Matching must still work after the clear (it recompiles lazily).
	if !matchPattern(pattern, "cached-value") {
		t.Fatalf("expected pattern to match after clearing")
	}
}
