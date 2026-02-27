package common

import (
	"os"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	JWTExpirationSeconds = 24 * 60 * 60 // 24 hours

	NodeTerminalPodName = "kite-node-terminal-agent"

	KubectlAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

	// db connection max idle time
	DBMaxIdleTime  = 10 * time.Minute
	DBMaxOpenConns = 100
	DBMaxIdleConns = 10
)

var (
	Port            = "8080"
	JwtSecret       = "kite-default-jwt-secret-key-change-in-production"
	EnableAnalytics = false
	Host            = ""
	Base            = ""

	NodeTerminalImage = "busybox:latest"
	DBType            = "sqlite"
	DBDSN             = "dev.db"

	KiteEncryptKey = "kite-default-encryption-key-change-in-production"

	AnonymousUserEnabled = false

	CookieExpirationSeconds = 2 * JWTExpirationSeconds // double jwt

	DisableGZIP         = true
	DisableVersionCheck = false

	APIKeyProvider = "api_key"

	AuthCookieSameSite = "lax"
	AuthCookieSecure   = "auto"

	SealosAuthEnabled = false
	SealosJWTSecret   = ""
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

	if nodeTerminalImage := os.Getenv("NODE_TERMINAL_IMAGE"); nodeTerminalImage != "" {
		NodeTerminalImage = nodeTerminalImage
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

	if key := os.Getenv("KITE_ENCRYPT_KEY"); key != "" {
		KiteEncryptKey = key
	} else {
		klog.Warningf("KITE_ENCRYPT_KEY is not set, using default key, this is not secure for production!")
	}

	if v := os.Getenv("ANONYMOUS_USER_ENABLED"); v == "true" {
		AnonymousUserEnabled = true
		klog.Warningf("Anonymous user is enabled, this is not secure for production!")
	}
	if v := os.Getenv("HOST"); v != "" {
		Host = v
	}
	if v := os.Getenv("DISABLE_GZIP"); v != "" {
		DisableGZIP = v == "true"
	}

	if v := os.Getenv("DISABLE_VERSION_CHECK"); v == "true" {
		DisableVersionCheck = true
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
		default:
			klog.Warningf("Invalid AUTH_COOKIE_SAMESITE=%q, use lax|strict|none, fallback to lax", v)
		}
	}

	if v := strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")); v != "" {
		switch strings.ToLower(v) {
		case "auto", "true", "false":
			AuthCookieSecure = strings.ToLower(v)
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
}
