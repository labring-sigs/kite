package common

import (
	"os"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	JWTExpirationSeconds = 24 * 60 * 60 // 24 hours
	DefaultJWTSecret     = "kite-default-jwt-secret-key-change-in-production"

	NodeTerminalPodName    = "kite-node-terminal-agent"
	KubectlTerminalPodName = "kite-kubectl-agent"

	KubectlAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

	// db connection max idle time
	DBMaxIdleTime  = 10 * time.Minute
	DBMaxOpenConns = 100
	DBMaxIdleConns = 10
)

var (
	Port            = "8080"
	JwtSecret       = DefaultJWTSecret
	EnableAnalytics = false
	Host            = ""
	Base            = ""

	NodeTerminalImage    = "busybox:latest"
	KubectlTerminalImage = "zzde/kubectl:latest"
	DBType               = "sqlite"
	DBDSN                = "dev.db"
	DBAutoCreate         = true

	KiteEncryptKey = "kite-default-encryption-key-change-in-production"

	AllNamespaces = "_all"

	CookieExpirationSeconds = 2 * JWTExpirationSeconds // double jwt

	DisableGZIP         = true
	DisableVersionCheck = false
	EnableVersionCheck  = true

	APIKeyProvider = "api_key"

	AgentPodNamespace = "kube-system"

	HelmArtifactHubEnabled    = true
	HelmOfflineImagesEnabled  = false
	HelmOfflineImagesRegistry = ""
	HelmOfflineImagesEnforce  = true

	AuthCookieSameSite = "lax"
	AuthCookieSecure   = "auto"
	// AuthCookieSameSiteExplicit indicates AUTH_COOKIE_SAMESITE is explicitly configured.
	AuthCookieSameSiteExplicit = false
	// AuthCookieSecureExplicit indicates AUTH_COOKIE_SECURE is explicitly configured.
	AuthCookieSecureExplicit = false

	SealosAuthEnabled = false
	SealosJWTSecret   = ""
	// SealosDefaultPrometheusURL sets default Prometheus URL for Sealos-managed clusters.
	SealosDefaultPrometheusURL = ""

	// NamespaceScopeExemptNamespaces lists namespaces that should not force
	// namespace-scoped mode even when kubeconfig current-context.namespace is set.
	NamespaceScopeExemptNamespaces = map[string]struct{}{}
)

func LoadEnvs() {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		JwtSecret = secret
	}

	if port := os.Getenv("PORT"); port != "" {
		Port = port
	}

	if analytics := os.Getenv("ENABLE_ANALYTICS"); analytics == "true" {
		EnableAnalytics = true
	}

	if ns := os.Getenv("NAMESPACE"); ns != "" {
		AgentPodNamespace = ns
	}

	if nodeTerminalImage := os.Getenv("NODE_TERMINAL_IMAGE"); nodeTerminalImage != "" {
		NodeTerminalImage = nodeTerminalImage
	}

	if kubectlTerminalImage := os.Getenv("KUBECTL_TERMINAL_IMAGE"); kubectlTerminalImage != "" {
		KubectlTerminalImage = kubectlTerminalImage
	}

	if dbDSN := os.Getenv("DB_DSN"); dbDSN != "" {
		DBDSN = dbDSN
	}

	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		if dbType != "sqlite" && dbType != "mysql" && dbType != "postgres" {
			klog.Fatalf("Invalid DB_TYPE: %s, must be one of sqlite, mysql, postgres", dbType)
		}
		DBType = dbType
	}

	if v := strings.TrimSpace(os.Getenv("DB_AUTO_CREATE")); v != "" {
		switch strings.ToLower(v) {
		case "true":
			DBAutoCreate = true
		case "false":
			DBAutoCreate = false
		default:
			klog.Warningf("Invalid DB_AUTO_CREATE=%q, use true|false, fallback to true", v)
		}
	}

	if key := os.Getenv("KITE_ENCRYPT_KEY"); key != "" {
		KiteEncryptKey = key
	} else {
		klog.Warningf("KITE_ENCRYPT_KEY is not set, using default key, this is not secure for production!")
	}

	if v := os.Getenv("HOST"); v != "" {
		Host = v
	}
	if v := os.Getenv("DISABLE_GZIP"); v != "" {
		DisableGZIP = v == "true"
	}

	if v := os.Getenv("DISABLE_VERSION_CHECK"); v == "true" {
		DisableVersionCheck = true
		EnableVersionCheck = false
	}

	if v := os.Getenv("KITE_BASE"); v != "" {
		if v[0] != '/' {
			v = "/" + v
		}
		Base = strings.TrimRight(v, "/")
		klog.Infof("Using base path: %s", Base)
	}

	if v := strings.TrimSpace(os.Getenv("AUTH_COOKIE_SAMESITE")); v != "" {
		switch strings.ToLower(v) {
		case "lax", "strict", "none":
			AuthCookieSameSite = strings.ToLower(v)
			AuthCookieSameSiteExplicit = true
		default:
			klog.Warningf("Invalid AUTH_COOKIE_SAMESITE=%q, use lax|strict|none, fallback to lax", v)
		}
	}

	if v := strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")); v != "" {
		switch strings.ToLower(v) {
		case "auto", "true", "false":
			AuthCookieSecure = strings.ToLower(v)
			AuthCookieSecureExplicit = true
		default:
			klog.Warningf("Invalid AUTH_COOKIE_SECURE=%q, use auto|true|false, fallback to auto", v)
		}
	}

	if v := strings.TrimSpace(os.Getenv("SEALOS_AUTH_ENABLED")); v == "true" {
		SealosAuthEnabled = true
	}
	if v := strings.TrimSpace(os.Getenv("SEALOS_JWT_SECRET")); v != "" {
		SealosJWTSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("SEALOS_DEFAULT_PROMETHEUS_URL")); v != "" {
		SealosDefaultPrometheusURL = v
	}
	if v := strings.TrimSpace(os.Getenv("KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES")); v != "" {
		namespaces := make(map[string]struct{})
		for _, ns := range strings.Split(v, ",") {
			ns = strings.ToLower(strings.TrimSpace(ns))
			if ns == "" {
				continue
			}
			namespaces[ns] = struct{}{}
		}
		NamespaceScopeExemptNamespaces = namespaces
		if len(namespaces) > 0 {
			klog.Infof("Configured %d namespace-scope exemptions from KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES", len(namespaces))
		}
	}

	if v := strings.TrimSpace(os.Getenv("KITE_HELM_ARTIFACT_HUB_ENABLED")); v != "" {
		switch strings.ToLower(v) {
		case "true":
			HelmArtifactHubEnabled = true
		case "false":
			HelmArtifactHubEnabled = false
		default:
			klog.Warningf("Invalid KITE_HELM_ARTIFACT_HUB_ENABLED=%q, use true|false, fallback to true", v)
		}
	}
	if v := strings.TrimSpace(os.Getenv("KITE_HELM_OFFLINE_IMAGES_ENABLED")); v != "" {
		switch strings.ToLower(v) {
		case "true":
			HelmOfflineImagesEnabled = true
		case "false":
			HelmOfflineImagesEnabled = false
		default:
			klog.Warningf("Invalid KITE_HELM_OFFLINE_IMAGES_ENABLED=%q, use true|false, fallback to false", v)
		}
	}
	if v := strings.TrimSpace(os.Getenv("KITE_HELM_OFFLINE_IMAGE_REGISTRY")); v != "" {
		HelmOfflineImagesRegistry = strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://"), "/")
	}
	if v := strings.TrimSpace(os.Getenv("KITE_HELM_OFFLINE_IMAGES_ENFORCE")); v != "" {
		switch strings.ToLower(v) {
		case "true":
			HelmOfflineImagesEnforce = true
		case "false":
			HelmOfflineImagesEnforce = false
		default:
			klog.Warningf("Invalid KITE_HELM_OFFLINE_IMAGES_ENFORCE=%q, use true|false, fallback to true", v)
		}
	}
}

func IsNamespaceScopeExempt(namespace string) bool {
	ns := strings.ToLower(strings.TrimSpace(namespace))
	if ns == "" {
		return false
	}
	_, ok := NamespaceScopeExemptNamespaces[ns]
	return ok
}
