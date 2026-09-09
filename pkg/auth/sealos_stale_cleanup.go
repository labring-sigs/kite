package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

const (
	// staleCleanupInterval is how often the sweep runs. An hourly pass keeps
	// the residual list short without adding measurable database load.
	staleCleanupInterval = time.Hour
	// staleSweepBatchSize caps how many stale users one pass deletes, so a
	// huge backlog is drained gradually instead of in one blocking burst.
	staleSweepBatchSize = 50
)

// StartStaleSealosCleanup periodically removes Sealos auto-provisioned
// accounts (and the clusters/roles their login created) that have been
// inactive longer than KITE_SEALOS_STALE_TTL_DAYS. On multi-tenant Sealos
// platforms the cluster selector otherwise fills up with residual workspace
// entries whose kubeconfigs have long expired — which shows up as "Sync
// Error" noise for admins and as repeated failed client rebuilds in the
// cluster sync loop.
//
// The sweep loop runs in its own goroutine and this function returns
// immediately, matching scheduler.Start: a blocking loop in the caller's
// goroutine would prevent the HTTP server from ever starting.
func StartStaleSealosCleanup(ctx context.Context, cm *cluster.ClusterManager) {
	ttlDays := common.SealosStaleCleanupTTLDays
	if ttlDays <= 0 {
		klog.Infof("sealos stale cleanup disabled (KITE_SEALOS_STALE_TTL_DAYS=%d)", ttlDays)
		return
	}
	klog.Infof("sealos stale cleanup enabled: ttl=%dd interval=%s", ttlDays, staleCleanupInterval)

	go func() {
		ticker := time.NewTicker(staleCleanupInterval)
		defer ticker.Stop()
		for {
			removed, err := SweepStaleSealosUsers(time.Now(), ttlDays, staleSweepBatchSize)
			if err != nil {
				klog.Warningf("sealos stale cleanup sweep failed: %v", err)
			}
			if removed > 0 {
				// Torn-down clusters must drop their informer clients and RBAC
				// must forget the deleted roles/assignments.
				cm.TriggerSync()
				if err := rbac.ForceSyncRolesConfig(); err != nil {
					klog.Warningf("sealos stale cleanup: failed to reload RBAC config: %v", err)
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// SweepStaleSealosUsers deletes up to limit Sealos auto-provisioned users
// whose last login is older than ttlDays (or who never logged in and were
// created longer ago than that), together with everything their login
// created: the auto-generated role, all their role assignments, and every
// sealos-* cluster they ever owned. It returns the number of deleted users.
func SweepStaleSealosUsers(now time.Time, ttlDays, limit int) (int, error) {
	cutoff := now.AddDate(0, 0, -ttlDays)

	var staleUsers []model.User
	err := model.DB.
		Where("provider = ?", sealosProvider).
		Where(
			model.DB.Where("last_login_at < ?", cutoff).
				Or("last_login_at IS NULL AND created_at < ?", cutoff),
		).
		Limit(limit).
		Find(&staleUsers).Error
	if err != nil {
		return 0, err
	}

	// Ownership must be resolved exactly, not by raw prefix: cluster names
	// are "sealos-<userPart>-<workspacePart>" and sanitizeNamePart is NOT
	// collision-proof — a stale user "a" would otherwise also match the
	// active user "a-b"'s clusters ("sealos-a-b-c" starts with "sealos-a-").
	// A cluster is therefore assigned to the sealos user with the LONGEST
	// matching prefix, and a stale user may only delete clusters whose
	// longest-prefix owner is itself.
	var allSealosUsers []model.User
	if err := model.DB.Select("sub").Where("provider = ?", sealosProvider).Find(&allSealosUsers).Error; err != nil {
		return 0, err
	}
	ownerPrefixes := make([]string, 0, len(allSealosUsers))
	for _, su := range allSealosUsers {
		if id, ok := strings.CutPrefix(su.Sub, sealosProvider+":"); ok && strings.TrimSpace(id) != "" {
			ownerPrefixes = append(ownerPrefixes, "sealos-"+sanitizeNamePart(id)+"-")
		}
	}
	var allSealosClusters []model.Cluster
	if err := model.DB.Where("name LIKE ?", "sealos-%").Find(&allSealosClusters).Error; err != nil {
		return 0, err
	}
	// longestPrefixOwner returns the owner prefix (longest match) for a
	// cluster name, or false when no sealos user prefix matches.
	longestPrefixOwner := func(clusterName string) (string, bool) {
		best := ""
		for _, p := range ownerPrefixes {
			if strings.HasPrefix(clusterName, p) && len(p) > len(best) {
				best = p
			}
		}
		if best == "" {
			return "", false
		}
		return best, true
	}

	removed := 0
	for i := range staleUsers {
		u := &staleUsers[i]
		// Sealos users carry the platform user ID in Sub as "sealos:<id>";
		// anything else means the row was not auto-provisioned — skip it.
		userID, ok := strings.CutPrefix(u.Sub, sealosProvider+":")
		if !ok || strings.TrimSpace(userID) == "" {
			continue
		}

		clusterNames := map[string]struct{}{}

		// (a) Orphaned older workspace clusters: only those whose
		// longest-prefix owner is exactly this stale user.
		ownerPrefix := "sealos-" + sanitizeNamePart(userID) + "-"
		for _, cl := range allSealosClusters {
			owner, ok := longestPrefixOwner(cl.Name)
			if ok && owner == ownerPrefix {
				clusterNames[cl.Name] = struct{}{}
			}
		}

		// (b) The auto-generated role also references the current cluster.
		// Role rows are persisted and admin-editable, so role-derived names
		// must pass the same ownership proof as name-derived ones; otherwise
		// an edited role could make the sweep delete another user's cluster.
		roleName := buildSealosRoleName(userID)
		role, roleErr := model.GetRoleByName(roleName)
		if roleErr == nil {
			for _, name := range role.Clusters {
				if owner, ok := longestPrefixOwner(name); ok && owner == ownerPrefix {
					clusterNames[name] = struct{}{}
				}
			}
		} else if !errors.Is(roleErr, gorm.ErrRecordNotFound) {
			return removed, roleErr
		}

		for name := range clusterNames {
			cl, err := model.GetClusterByName(name)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return removed, err
			}
			// Only auto-created sealos-* clusters may be swept; anything else
			// (e.g. manually imported) is left alone.
			if !strings.HasPrefix(cl.Name, "sealos-") {
				continue
			}
			if err := model.DeleteCluster(cl); err != nil {
				return removed, err
			}
			klog.Infof("sealos stale cleanup: deleted cluster %s (owner %s inactive)", cl.Name, u.Username)
		}

		// Drop every role assignment of this user, covering the auto role and
		// any built-in admin assignment from exempt workspaces.
		if err := model.DB.
			Where("subject_type = ? AND subject = ?", model.SubjectTypeUser, u.Username).
			Delete(&model.RoleAssignment{}).Error; err != nil {
			return removed, err
		}

		if roleErr == nil {
			if err := model.DB.Delete(role).Error; err != nil {
				return removed, err
			}
		}

		// DeleteUserByID also removes the user's resource history.
		if err := model.DeleteUserByID(u.ID); err != nil {
			return removed, err
		}
		removed++
		klog.Infof("sealos stale cleanup: deleted user %s with %d cluster(s)", u.Username, len(clusterNames))
	}

	return removed, nil
}
