package seeder

import (
	"fmt"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// ValidateDefinitions checks the in-code RBAC and fixture graph without
// touching external services. Destructive staging reset commands call this
// before clearing data so a missing definition cannot leave staging empty.
func ValidateDefinitions() error {
	roles := make(map[string]struct{}, len(roleSeeds()))
	for _, role := range roleSeeds() {
		slug := strings.TrimSpace(role.Slug)
		if slug == "" {
			return fmt.Errorf("seed role has an empty slug")
		}
		if _, duplicate := roles[slug]; duplicate {
			return fmt.Errorf("duplicate seed role: %s", slug)
		}
		roles[slug] = struct{}{}
	}

	permissions := make(map[string]struct{}, len(permissionSeeds()))
	for _, permission := range permissionSeeds() {
		slug := strings.TrimSpace(permission.Slug)
		if slug == "" {
			return fmt.Errorf("seed permission has an empty slug")
		}
		if _, duplicate := permissions[slug]; duplicate {
			return fmt.Errorf("duplicate seed permission: %s", slug)
		}
		permissions[slug] = struct{}{}
	}

	managedPermissions := make(map[string]struct{})
	for role, grants := range constant.DefaultRolePermissions {
		if _, exists := roles[role]; !exists {
			return fmt.Errorf("default permissions reference undefined role: %s", role)
		}
		seenGrants := make(map[string]struct{}, len(grants))
		for _, permission := range grants {
			if _, duplicate := seenGrants[permission]; duplicate {
				return fmt.Errorf("role %s contains duplicate permission: %s", role, permission)
			}
			seenGrants[permission] = struct{}{}
			managedPermissions[permission] = struct{}{}
			if _, exists := permissions[permission]; !exists {
				return fmt.Errorf("role %s references undefined permission: %s", role, permission)
			}
		}
	}

	for permission := range permissions {
		if _, managed := managedPermissions[permission]; !managed {
			return fmt.Errorf("seed permission is not assigned by any default role: %s", permission)
		}
	}
	for _, user := range sampleUserSeeds() {
		if _, exists := roles[user.RoleSlug]; !exists {
			return fmt.Errorf("fixture %s references undefined role: %s", user.Email, user.RoleSlug)
		}
	}
	return nil
}
