package auth

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/permissions"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
)

const (
	sealosProvider = "sealos"
)

var errSealosUserDisabled = errors.New("user disabled")

type SealosTokenClaims struct {
	WorkspaceUID string `json:"workspaceUid"`
	WorkspaceID  string `json:"workspaceId"`
	RegionUID    string `json:"regionUid"`
	UserCrUID    string `json:"userCrUid"`
	UserCrName   string `json:"userCrName"`
	UserID       string `json:"userId"`
	UserUID      string `json:"userUid"`
	jwt.RegisteredClaims
}

type SealosSessionUser struct {
	K8sUsername string `json:"k8s_username"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	NSID        string `json:"nsid"`
	NSUID       string `json:"ns_uid"`
	UserCrUID   string `json:"userCrUid"`
	UserID      string `json:"userId"`
	UserUID     string `json:"userUid"`
}

type SealosLoginRequest struct {
	Token      string            `json:"token" binding:"required"`
	Kubeconfig string            `json:"kubeconfig" binding:"required"`
	User       SealosSessionUser `json:"user"`
}

func (h *AuthHandler) validateSealosToken(tokenString string) (*SealosTokenClaims, error) {
	secret := strings.TrimSpace(common.SealosJWTSecret)
	if secret == "" {
		return nil, errors.New("SEALOS_JWT_SECRET is not configured")
	}
	claims := new(SealosTokenClaims)
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid {
		return nil, errors.New("invalid sealos token")
	}
	return claims, nil
}

func shortHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:8]
}

func sanitizeNamePart(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func buildSealosUsername(userID string) string {
	part := sanitizeNamePart(userID)
	if part == "" {
		part = "user"
	}
	name := "sealos-" + part
	if len(name) <= 50 {
		return name
	}
	hash := shortHash(userID)
	maxPartLen := 50 - len("sealos--") - len(hash)
	if maxPartLen < 1 {
		maxPartLen = 1
	}
	if len(part) > maxPartLen {
		part = part[:maxPartLen]
	}
	part = strings.Trim(part, "-")
	if part == "" {
		part = "user"
	}
	return "sealos-" + part + "-" + hash
}

func buildSealosClusterName(userID, workspaceID string) string {
	userPart := sanitizeNamePart(userID)
	if userPart == "" {
		userPart = "user"
	}
	workspacePart := sanitizeNamePart(workspaceID)
	name := "sealos-" + userPart
	if workspacePart != "" {
		name += "-" + workspacePart
	}
	if len(name) <= 100 {
		return name
	}
	hash := shortHash(userID + ":" + workspaceID)
	combined := userPart
	if workspacePart != "" {
		combined += "-" + workspacePart
	}
	maxLen := 100 - len("sealos--") - len(hash)
	if maxLen < 1 {
		maxLen = 1
	}
	if len(combined) > maxLen {
		combined = combined[:maxLen]
	}
	combined = strings.Trim(combined, "-")
	if combined == "" {
		combined = "user"
	}
	return "sealos-" + combined + "-" + hash
}

func buildSealosRoleName(userID string) string {
	part := sanitizeNamePart(userID)
	if part == "" {
		part = "user"
	}
	name := "sealos-role-" + part
	if len(name) <= 100 {
		return name
	}
	hash := shortHash(userID)
	maxPartLen := 100 - len("sealos-role--") - len(hash)
	if maxPartLen < 1 {
		maxPartLen = 1
	}
	if len(part) > maxPartLen {
		part = part[:maxPartLen]
	}
	part = strings.Trim(part, "-")
	if part == "" {
		part = "user"
	}
	return "sealos-role-" + part + "-" + hash
}

func getSealosDefaultPrometheusURL() string {
	return strings.TrimSpace(common.SealosDefaultPrometheusURL)
}

// parseSealosKubeconfig delegates to the cluster package's identity parser
// (kept as a thin wrapper so existing callers and tests stay put).
func parseSealosKubeconfig(kubeconfig string) (identity, token string) {
	return cluster.ParseSealosKubeconfig(kubeconfig)
}

// upsertSealosCluster creates or refreshes the cluster record for a Sealos
// login and reports whether the kubeconfig's identity changed. The stored
// config is still updated whenever the raw bytes differ (so a rotated token
// persists), but role/RBAC sync and client rebuilds are only needed when the
// identity itself changed — not on every token rotation.
func upsertSealosCluster(clusterName, kubeconfig, namespace string) (bool, error) {
	defaultPrometheusURL := getSealosDefaultPrometheusURL()
	description := "Managed by Sealos SSO"
	if namespace != "" {
		description = fmt.Sprintf("%s (namespace: %s)", description, namespace)
	}
	cluster, err := model.GetClusterByName(clusterName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, model.AddCluster(&model.Cluster{
				Name:          clusterName,
				Description:   description,
				Config:        model.SecretString(kubeconfig),
				PrometheusURL: defaultPrometheusURL,
				InCluster:     false,
				IsDefault:     false,
				Enable:        true,
			})
		}
		return false, err
	}

	newIdentity, _ := parseSealosKubeconfig(kubeconfig)
	storedIdentity, _ := parseSealosKubeconfig(string(cluster.Config))
	identityChanged := newIdentity == "" || newIdentity != storedIdentity

	updates := buildSealosClusterUpdates(cluster, description, kubeconfig, defaultPrometheusURL)
	if len(updates) > 0 {
		if err := model.UpdateCluster(cluster, updates); err != nil {
			return false, err
		}
	}
	return identityChanged, nil
}

func buildSealosClusterUpdates(cluster *model.Cluster, description, kubeconfig, defaultPrometheusURL string) map[string]interface{} {
	updates := map[string]interface{}{}
	if cluster.Description != description {
		updates["description"] = description
	}
	if string(cluster.Config) != kubeconfig {
		updates["config"] = model.SecretString(kubeconfig)
	}
	if cluster.InCluster {
		updates["in_cluster"] = false
	}
	if !cluster.Enable {
		updates["enable"] = true
	}
	if strings.TrimSpace(cluster.PrometheusURL) == "" && defaultPrometheusURL != "" {
		updates["prometheus_url"] = defaultPrometheusURL
	}
	return updates
}

// SyncSealosPrometheusDefaults applies default Prometheus URL to existing
// Sealos-managed clusters that do not have prometheus_url configured.
func SyncSealosPrometheusDefaults() (int64, error) {
	if !common.SealosAuthEnabled {
		return 0, nil
	}
	defaultPrometheusURL := getSealosDefaultPrometheusURL()
	if defaultPrometheusURL == "" {
		return 0, nil
	}
	result := model.DB.Model(&model.Cluster{}).
		Where("name LIKE ? AND (prometheus_url = '' OR prometheus_url IS NULL)", "sealos-%").
		Update("prometheus_url", defaultPrometheusURL)
	return result.RowsAffected, result.Error
}

func buildSealosRoleNamespaces(namespace string) []string {
	ns := strings.TrimSpace(namespace)
	if ns == "" || common.IsNamespaceScopeExempt(ns) {
		return []string{"*"}
	}
	return []string{ns}
}

func ensureSealosRole(roleName, clusterName, namespace string) (*model.Role, error) {
	namespaces := buildSealosRoleNamespaces(namespace)
	role := &model.Role{
		Name:        roleName,
		Description: "Auto generated role for Sealos SSO user",
		Clusters:    []string{clusterName},
		Namespaces:  namespaces,
		Resources:   []string{"*"},
		Verbs:       []string{"*"},
		IsSystem:    false,
	}

	var existing model.Role
	if err := model.DB.Where("name = ?", roleName).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return role, model.DB.Create(role).Error
		}
		return nil, err
	}

	existing.Description = role.Description
	existing.Clusters = role.Clusters
	existing.Namespaces = role.Namespaces
	existing.Resources = role.Resources
	existing.Verbs = role.Verbs
	existing.IsSystem = false
	if err := model.DB.Save(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

func ensureSealosRoleAssignment(roleID uint, username string) error {
	var assignment model.RoleAssignment
	err := model.DB.Where(
		"role_id = ? AND subject_type = ? AND subject = ?",
		roleID,
		model.SubjectTypeUser,
		username,
	).First(&assignment).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return model.DB.Create(&model.RoleAssignment{
		RoleID:      roleID,
		SubjectType: model.SubjectTypeUser,
		Subject:     username,
	}).Error
}

func syncSealosAdminRoleAssignment(namespace, username string) error {
	role, err := model.GetRoleByName(model.DefaultAdminRole.Name)
	if err != nil {
		return err
	}
	if !common.IsNamespaceScopeExempt(namespace) {
		return model.DB.Where(
			"role_id = ? AND subject_type = ? AND subject = ?",
			role.ID,
			model.SubjectTypeUser,
			username,
		).Delete(&model.RoleAssignment{}).Error
	}
	return ensureSealosRoleAssignment(role.ID, username)
}

func upsertSealosUser(claims *SealosTokenClaims, sessionUser SealosSessionUser) (*model.User, error) {
	userID := strings.TrimSpace(claims.UserID)
	if userID == "" {
		userID = strings.TrimSpace(sessionUser.UserID)
	}
	if userID == "" {
		return nil, errors.New("sealos userId is empty")
	}

	displayName := strings.TrimSpace(sessionUser.Name)
	if displayName == "" {
		displayName = userID
	}

	u := &model.User{
		Username:  buildSealosUsername(userID),
		Name:      displayName,
		AvatarURL: strings.TrimSpace(sessionUser.Avatar),
		Provider:  sealosProvider,
		Sub:       sealosProvider + ":" + userID,
		Enabled:   true,
	}
	if err := model.FindWithSubOrUpsertUser(u); err != nil {
		return nil, err
	}
	if u.ID == 0 {
		if err := model.DB.Where("sub = ?", u.Sub).First(u).Error; err != nil {
			return nil, err
		}
	}
	if !u.Enabled {
		return nil, errSealosUserDisabled
	}
	if err := model.LoginUser(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (h *AuthHandler) SealosLogin(c *gin.Context) {
	if !common.SealosAuthEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "sealos auth is disabled"})
		return
	}

	var req SealosLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	claims, err := h.validateSealosToken(req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid sealos token"})
		return
	}

	userID := strings.TrimSpace(claims.UserID)
	if userID == "" {
		userID = strings.TrimSpace(req.User.UserID)
	}
	workspaceID := strings.TrimSpace(claims.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(req.User.NSID)
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	clusterName := buildSealosClusterName(userID, workspaceID)
	identityChanged, err := upsertSealosCluster(clusterName, req.Kubeconfig, workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync sealos cluster"})
		return
	}

	// Always record the freshest token: Sealos desktop kubeconfigs rotate
	// their embedded token on every session refresh, and the running client
	// picks the new token up from the store instead of needing a rebuild.
	_, freshToken := parseSealosKubeconfig(req.Kubeconfig)
	cluster.SetSealosClusterToken(clusterName, freshToken)

	user, err := upsertSealosUser(claims, req.User)
	if err != nil {
		if errors.Is(err, errSealosUserDisabled) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync sealos user"})
		return
	}

	// Role/RBAC sync runs on every login where the cluster binding's identity
	// changed (new workspace or a different kubeconfig). A mere token
	// rotation does not change the identity, so the writes are skipped — the
	// cheap existence checks below keep the sync self-healing if the role or
	// assignment was deleted manually in the meantime.
	roleName := buildSealosRoleName(userID)
	needRoleSync := identityChanged
	if !needRoleSync {
		existingRole, roleErr := model.GetRoleByName(roleName)
		switch {
		case errors.Is(roleErr, gorm.ErrRecordNotFound):
			needRoleSync = true
		case roleErr != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check sealos role"})
			return
		default:
			// Existence alone is not enough: a manually edited or previously
			// stale role must also still grant THIS exact cluster and
			// workspace, otherwise the login would keep the user bound to the
			// wrong scope while skipping the resync.
			if !slices.Equal(existingRole.Clusters, []string{clusterName}) ||
				!slices.Equal(existingRole.Namespaces, buildSealosRoleNamespaces(workspaceID)) {
				needRoleSync = true
				break
			}
			var count int64
			if err := model.DB.Model(&model.RoleAssignment{}).
				Where("role_id = ? AND subject_type = ? AND subject = ?", existingRole.ID, model.SubjectTypeUser, user.Username).
				Count(&count).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check sealos role assignment"})
				return
			}
			needRoleSync = count == 0
		}
	}

	if needRoleSync {
		role, err := ensureSealosRole(roleName, clusterName, workspaceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync sealos role"})
			return
		}
		if err := ensureSealosRoleAssignment(role.ID, user.Username); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign sealos role"})
			return
		}
		if err := syncSealosAdminRoleAssignment(workspaceID, user.Username); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync sealos admin role"})
			return
		}
		if err := rbac.ForceSyncRolesConfig(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sync sealos rbac"})
			return
		}
		select {
		case rbac.SyncNow <- struct{}{}:
		default:
		}
	}

	if h.clusterManager != nil {
		// A rebuild (and the blocking wait for it) is only needed when the
		// kubeconfig identity changed or no live client exists. A token
		// rotation alone is already covered by the token store update above.
		if identityChanged || !h.clusterManager.HasCluster(clusterName) {
			h.clusterManager.TriggerSyncForCluster(clusterName)
			_ = h.clusterManager.WaitForCluster(clusterName, 5*time.Second)
		}
	}

	user.Roles = rbac.GetUserRoles(*user)
	jwtToken, err := h.manager.GenerateJWT(user, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate JWT"})
		return
	}

	setCookieSecure(c, "auth_token", jwtToken, common.CookieExpirationSeconds)
	setCookieClient(c, "x-cluster-name", clusterName, common.CookieExpirationSeconds)

	c.JSON(http.StatusOK, gin.H{
		"user": user,
		// Capabilities ride along so the frontend can render the user
		// immediately without a second /api/auth/user round-trip.
		"capabilities": permissions.BuildUserCapabilities(*user, clusterName),
		"cluster":      clusterName,
		"namespace":    workspaceID,
		"token_type":   "Bearer",
		"access_token": jwtToken,
		"expires_in":   common.JWTExpirationSeconds,
	})
}
