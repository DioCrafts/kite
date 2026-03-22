package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm/clause"
	"k8s.io/klog/v2"
)

const (
	// managedByLabel is the value stored in ManagedBy fields to distinguish
	// declarative-config-managed resources from manually created ones.
	managedByLabel = "kite-declarative-config"
)

// Reconciler applies a KiteConfig to the database. It performs full CRUD
// reconciliation: creates missing resources, updates existing ones, and
// deletes resources that were previously managed but are no longer declared.
type Reconciler struct{}

// NewReconciler creates a new Reconciler.
func NewReconciler() *Reconciler {
	return &Reconciler{}
}

// Reconcile applies the full KiteConfig to the database.
// Each section (OAuth, Roles, GeneralSettings) is reconciled independently
// so a failure in one doesn't block the others.
func (r *Reconciler) Reconcile(cfg *KiteConfig) error {
	var errs []string

	if err := r.reconcileOAuth(cfg); err != nil {
		errs = append(errs, fmt.Sprintf("oauth: %v", err))
	}

	if err := r.reconcileRoles(cfg); err != nil {
		errs = append(errs, fmt.Sprintf("roles: %v", err))
	}

	if err := r.reconcileGeneralSettings(cfg); err != nil {
		errs = append(errs, fmt.Sprintf("generalSettings: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("reconciliation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// OAuth Providers
// ═══════════════════════════════════════════════════════════════════════════════

func (r *Reconciler) reconcileOAuth(cfg *KiteConfig) error {
	if cfg.OAuth == nil || len(cfg.OAuth.Providers) == 0 {
		// If no providers declared, clean up any previously managed ones
		return r.deleteOrphanedOAuthProviders(nil)
	}

	// Track which provider names are declared
	declaredNames := make(map[string]bool)
	var errs []string

	for i, p := range cfg.OAuth.Providers {
		if p.Name == "" {
			return fmt.Errorf("OAuth provider at index %d has an empty name; aborting reconciliation to prevent accidental orphan cleanup", i)
		}

		name := strings.ToLower(p.Name)
		declaredNames[name] = true

		enabled := true
		if p.Enabled != nil {
			enabled = *p.Enabled
		}

		// Look for existing provider by name
		existing, err := model.GetOAuthProviderByNameUnfiltered(name)
		if err != nil {
			// Not found → create
			provider := &model.OAuthProvider{
				Name:         model.LowerCaseString(name),
				ClientID:     p.ClientID,
				ClientSecret: model.SecretString(expandEnvBraced(p.ClientSecret)),
				AuthURL:      p.AuthURL,
				TokenURL:     p.TokenURL,
				UserInfoURL:  p.UserInfoURL,
				Scopes:       p.Scopes,
				Issuer:       p.IssuerURL,
				Enabled:      enabled,
				ManagedBy:    managedByLabel,
			}
			if err := model.CreateOAuthProvider(provider); err != nil {
				klog.Errorf("Failed to create OAuth provider %q: %v", name, err)
				errs = append(errs, fmt.Sprintf("create provider %q: %v", name, err))
				continue
			}
			klog.Infof("Created OAuth provider %q", name)
		} else {
			// Found → update all fields unconditionally so the declared
			// config is the sole source of truth (deduplicateProviders
			// already merged multi-fragment values at the watcher level).
			// Only client_secret stays conditional — it is commonly
			// injected via a Secret env-var rather than config files.
			updates := map[string]interface{}{
				"client_id":     p.ClientID,
				"auth_url":      p.AuthURL,
				"token_url":     p.TokenURL,
				"user_info_url": p.UserInfoURL,
				"scopes":        p.Scopes,
				"issuer":         p.IssuerURL,
				"enabled":       enabled,
				"managed_by":    managedByLabel,
			}
			if p.ClientSecret != "" {
				updates["client_secret"] = model.SecretString(expandEnvBraced(p.ClientSecret))
			}
			if err := model.UpdateOAuthProvider(&existing, updates); err != nil {
				klog.Errorf("Failed to update OAuth provider %q: %v", name, err)
				errs = append(errs, fmt.Sprintf("update provider %q: %v", name, err))
				continue
			}
			klog.V(1).Infof("Updated OAuth provider %q", name)
		}
	}

	if err := r.deleteOrphanedOAuthProviders(declaredNames); err != nil {
		errs = append(errs, fmt.Sprintf("orphan cleanup: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("OAuth reconciliation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// deleteOrphanedOAuthProviders removes providers that were managed by declarative
// config but are no longer in the declared set. Manually created providers
// (ManagedBy == "") are never touched.
func (r *Reconciler) deleteOrphanedOAuthProviders(declaredNames map[string]bool) error {
	var managed []model.OAuthProvider
	if err := model.DB.Where("managed_by = ?", managedByLabel).Find(&managed).Error; err != nil {
		return fmt.Errorf("listing managed OAuth providers: %w", err)
	}

	var errs []string
	for _, p := range managed {
		name := strings.ToLower(string(p.Name))
		if !declaredNames[name] {
			if err := model.DeleteOAuthProvider(p.ID); err != nil {
				klog.Errorf("Failed to delete orphaned OAuth provider %q (id=%d): %v", name, p.ID, err)
				errs = append(errs, fmt.Sprintf("delete provider %q: %v", name, err))
			} else {
				klog.Infof("Deleted orphaned OAuth provider %q (id=%d)", name, p.ID)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("orphan provider cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// RBAC Roles & Assignments
// ═══════════════════════════════════════════════════════════════════════════════

func (r *Reconciler) reconcileRoles(cfg *KiteConfig) error {
	declaredNames := make(map[string]bool)
	var errs []string

	for i, rc := range cfg.Roles {
		if rc.Name == "" {
			return fmt.Errorf("role at index %d has an empty name; aborting reconciliation to prevent accidental orphan cleanup", i)
		}
		declaredNames[strings.ToLower(rc.Name)] = true

		if err := r.reconcileOneRole(rc); err != nil {
			klog.Errorf("Failed to reconcile role %q: %v", rc.Name, err)
			errs = append(errs, fmt.Sprintf("role %q: %v", rc.Name, err))
		}
	}

	if err := r.deleteOrphanedRoles(declaredNames); err != nil {
		errs = append(errs, fmt.Sprintf("orphan cleanup: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("role reconciliation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (r *Reconciler) reconcileOneRole(rc RoleConfig) error {
	// Normalize to lowercase so DB lookups are consistent regardless of
	// collation and match the lowercase keys used in orphan cleanup.
	rc.Name = strings.ToLower(rc.Name)

	existing, err := model.GetRoleByName(rc.Name)
	if err != nil {
		// Not found → create
		role := model.Role{
			Name:        rc.Name,
			Description: rc.Description,
			Clusters:    coalesceSlice(rc.Clusters, []string{"*"}),
			Namespaces:  rc.Namespaces,
			Resources:   coalesceSlice(rc.Resources, []string{"*"}),
			Verbs:       rc.Verbs,
			ManagedBy:   managedByLabel,
		}
		if err := model.DB.Create(&role).Error; err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
		klog.Infof("Created role %q", rc.Name)
		existing = &role
	} else {
		// System roles (admin/viewer): only update assignments, don't redefine rules
		if !existing.IsSystem {
			existing.Description = rc.Description
			existing.Clusters = coalesceSlice(rc.Clusters, []string{"*"})
			existing.Namespaces = rc.Namespaces
			existing.Resources = coalesceSlice(rc.Resources, []string{"*"})
			existing.Verbs = rc.Verbs
			existing.ManagedBy = managedByLabel
			if err := model.DB.Save(existing).Error; err != nil {
				return fmt.Errorf("updating role: %w", err)
			}
			klog.V(1).Infof("Updated role %q", rc.Name)
		} else {
			// For system roles, just mark as managed so assignments are tracked
			if existing.ManagedBy != managedByLabel {
				if err := model.DB.Model(existing).Update("managed_by", managedByLabel).Error; err != nil {
					return fmt.Errorf("adopting system role %q: %w", rc.Name, err)
				}
			}
		}
	}

	// Reconcile assignments
	return r.reconcileAssignments(existing, rc.Assignments)
}

func (r *Reconciler) reconcileAssignments(role *model.Role, desired []AssignmentConfig) error {
	// Validate subject types before touching the DB.
	for _, a := range desired {
		if a.SubjectType != "user" && a.SubjectType != "group" {
			return fmt.Errorf("invalid subjectType %q for assignment %q in role %q (must be \"user\" or \"group\")",
				a.SubjectType, a.Subject, role.Name)
		}
	}

	var errs []string

	// Upsert desired assignments — atomic INSERT ON CONFLICT UPDATE ensures
	// creation and adoption are idempotent and safe under concurrent writers.
	// The composite unique index idx_role_assignment_uniq on
	// (role_id, subject_type, subject) guarantees no duplicate rows.
	// We also update subject/subject_type so case-only renames on
	// case-insensitive DBs converge to the declared casing.
	for _, a := range desired {
		assignment := model.RoleAssignment{
			RoleID:      role.ID,
			SubjectType: a.SubjectType,
			Subject:     a.Subject,
			ManagedBy:   managedByLabel,
		}
		if err := model.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "role_id"}, {Name: "subject_type"}, {Name: "subject"}},
			DoUpdates: clause.AssignmentColumns([]string{"managed_by", "subject_type", "subject"}),
		}).Create(&assignment).Error; err != nil {
			klog.Errorf("Failed to upsert assignment %s/%s for role %q: %v",
				a.SubjectType, a.Subject, role.Name, err)
			errs = append(errs, fmt.Sprintf("upsert %s/%s: %v", a.SubjectType, a.Subject, err))
		}
	}

	// Delete orphaned managed assignments via SQL so case comparisons use the
	// database’s own collation rules rather than Go map equality.  This avoids
	// the mismatch where a Go-lowercased key hides a case-only rename.
	//
	// To stay within SQLite's 999-parameter limit we first collect the IDs of
	// desired rows that exist in the DB (batched), then delete all managed rows
	// for this role whose ID is not in that keep-set.
	if err := deleteOrphanAssignments(role.ID, desired); err != nil {
		klog.Errorf("Failed to delete orphaned assignments for role %q: %v", role.Name, err)
		errs = append(errs, fmt.Sprintf("orphan cleanup: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("assignment errors for role %q: %s", role.Name, strings.Join(errs, "; "))
	}
	return nil
}

// sqlParamBatchSize is the maximum number of bind parameters per SQL statement.
// SQLite's default SQLITE_MAX_VARIABLE_NUMBER is 999; we stay well below that.
const sqlParamBatchSize = 400

// deleteOrphanAssignments deletes managed assignments for roleID that are not
// in the desired list.  It collects the IDs of desired rows in batches (to stay
// within SQLite's bind-parameter limit), then deletes everything else.
func deleteOrphanAssignments(roleID uint, desired []AssignmentConfig) error {
	if len(desired) == 0 {
		// No desired assignments — delete ALL managed rows for this role.
		res := model.DB.Where("role_id = ? AND managed_by = ?", roleID, managedByLabel).
			Delete(&model.RoleAssignment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			klog.Infof("Deleted %d orphaned managed assignment(s) from role ID %d", res.RowsAffected, roleID)
		}
		return nil
	}

	// Step 1: collect IDs of desired rows that already exist (the keep-set).
	keepIDs := make(map[uint]bool)
	for i := 0; i < len(desired); i += sqlParamBatchSize {
		end := i + sqlParamBatchSize
		if end > len(desired) {
			end = len(desired)
		}
		batch := desired[i:end]

		var conds []string
		var args []interface{}
		args = append(args, roleID)
		for _, a := range batch {
			conds = append(conds, "(subject_type = ? AND subject = ?)")
			args = append(args, a.SubjectType, a.Subject)
		}
		query := "role_id = ? AND (" + strings.Join(conds, " OR ") + ")"

		var rows []model.RoleAssignment
		if err := model.DB.Where(query, args...).Find(&rows).Error; err != nil {
			return fmt.Errorf("querying keep-set batch: %w", err)
		}
		for _, r := range rows {
			keepIDs[r.ID] = true
		}
	}

	// Step 2: delete all managed rows for this role that are NOT in the keep-set.
	var allManaged []model.RoleAssignment
	if err := model.DB.Where("role_id = ? AND managed_by = ?", roleID, managedByLabel).
		Find(&allManaged).Error; err != nil {
		return fmt.Errorf("listing managed assignments: %w", err)
	}

	var toDelete []uint
	for _, a := range allManaged {
		if !keepIDs[a.ID] {
			toDelete = append(toDelete, a.ID)
		}
	}

	// Delete in batches of sqlParamBatchSize.
	for i := 0; i < len(toDelete); i += sqlParamBatchSize {
		end := i + sqlParamBatchSize
		if end > len(toDelete) {
			end = len(toDelete)
		}
		if res := model.DB.Where("id IN ?", toDelete[i:end]).Delete(&model.RoleAssignment{}); res.Error != nil {
			return fmt.Errorf("deleting orphan batch: %w", res.Error)
		} else if res.RowsAffected > 0 {
			klog.Infof("Deleted %d orphaned managed assignment(s) from role ID %d", res.RowsAffected, roleID)
		}
	}
	return nil
}

// deleteOrphanedRoles removes roles that were managed by declarative config
// but are no longer in the declared set. System roles are never deleted, but
// their managed assignments ARE revoked when the role leaves the desired config.
func (r *Reconciler) deleteOrphanedRoles(declaredNames map[string]bool) error {
	// Fetch ALL managed roles (including system) so we can revoke assignments
	// on system roles that are no longer in the desired set.
	var allManaged []model.Role
	if err := model.DB.Where("managed_by = ?", managedByLabel).Find(&allManaged).Error; err != nil {
		return fmt.Errorf("listing managed roles: %w", err)
	}

	var errs []string
	for _, role := range allManaged {
		if declaredNames[strings.ToLower(role.Name)] {
			continue
		}

		// Revoke managed assignments regardless of whether the role is a system role.
		revokeFailed := false
		if err := model.DB.Where("role_id = ? AND managed_by = ?", role.ID, managedByLabel).
			Delete(&model.RoleAssignment{}).Error; err != nil {
			klog.Errorf("Failed to revoke managed assignments for role %q (id=%d): %v", role.Name, role.ID, err)
			errs = append(errs, fmt.Sprintf("revoke assignments for %q: %v", role.Name, err))
			revokeFailed = true
		}

		if role.IsSystem {
			// Only clear managed_by when assignment revocation succeeded;
			// otherwise the role stays in the managed set and will be retried.
			if revokeFailed {
				continue
			}
			if err := model.DB.Model(&role).Update("managed_by", "").Error; err != nil {
				klog.Errorf("Failed to clear managed_by on system role %q: %v", role.Name, err)
				errs = append(errs, fmt.Sprintf("clear managed_by on %q: %v", role.Name, err))
			} else {
				klog.Infof("Revoked managed assignments and cleared managed_by for system role %q", role.Name)
			}
		} else {
			// Non-system roles: delete entirely (cascading via assignments already removed above).
			if err := model.DB.Delete(&role).Error; err != nil {
				klog.Errorf("Failed to delete orphaned role %q (id=%d): %v", role.Name, role.ID, err)
				errs = append(errs, fmt.Sprintf("delete role %q: %v", role.Name, err))
			} else {
				klog.Infof("Deleted orphaned role %q (id=%d)", role.Name, role.ID)
			}
		}
	}

	// Trigger RBAC in-memory refresh
	select {
	case rbac.SyncNow <- struct{}{}:
	default:
	}

	if len(errs) > 0 {
		return fmt.Errorf("orphan role cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// General Settings
// ═══════════════════════════════════════════════════════════════════════════════

func (r *Reconciler) reconcileGeneralSettings(cfg *KiteConfig) error {
	gs := cfg.GeneralSettings
	if gs == nil {
		return nil
	}

	updates := make(map[string]interface{})

	if gs.AIAgentEnabled != nil {
		updates["ai_agent_enabled"] = *gs.AIAgentEnabled
	}
	if gs.AIProvider != nil {
		updates["ai_provider"] = *gs.AIProvider
	}
	if gs.AIModel != nil {
		updates["ai_model"] = *gs.AIModel
	}
	if gs.AIBaseURL != nil {
		updates["ai_base_url"] = *gs.AIBaseURL
	}
	if gs.AIMaxTokens != nil {
		updates["ai_max_tokens"] = *gs.AIMaxTokens
	}
	if gs.KubectlEnabled != nil {
		updates["kubectl_enabled"] = *gs.KubectlEnabled
	}
	if gs.KubectlImage != nil {
		updates["kubectl_image"] = *gs.KubectlImage
	}
	if gs.NodeTerminalImage != nil {
		updates["node_terminal_image"] = *gs.NodeTerminalImage
	}
	if gs.EnableAnalytics != nil {
		updates["enable_analytics"] = *gs.EnableAnalytics
	}
	if gs.EnableVersionCheck != nil {
		updates["enable_version_check"] = *gs.EnableVersionCheck
	}

	if len(updates) == 0 {
		return nil
	}

	if _, err := model.UpdateGeneralSetting(updates); err != nil {
		return fmt.Errorf("updating general settings: %w", err)
	}

	klog.Infof("Updated general settings (%d fields)", len(updates))
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

// coalesceSlice returns val when explicitly set (non-nil), even if empty.
// Only when val is nil (field omitted in config) does it fall back to the default.
// This lets "clusters: []" mean "no clusters" rather than "all clusters".
func coalesceSlice(val, fallback []string) []string {
	if val != nil {
		return val
	}
	return fallback
}

// envBracedRe matches only ${VAR} placeholders (brace-delimited).
var envBracedRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvBraced replaces only ${VAR} placeholders with their environment
// values, leaving bare $, $VAR, and other dollar sequences untouched so
// literal secrets containing $ are not corrupted.  Unknown variables
// preserve the original placeholder instead of becoming empty strings.
func expandEnvBraced(s string) string {
	return envBracedRe.ReplaceAllStringFunc(s, func(m string) string {
		key := m[2 : len(m)-1] // strip ${ and }
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return m // preserve unknown placeholder
	})
}
