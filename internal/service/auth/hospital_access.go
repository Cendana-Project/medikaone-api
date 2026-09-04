package auth

import (
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// ResolveHospitalSessionRole applies the shared authorization rule used by
// hospital login and tenant profile bootstrap. A global SUPER_ADMIN may select
// any active hospital; every other account needs an active membership and an
// active hospital-scoped role.
func ResolveHospitalSessionRole(isGlobalSuperAdmin, isMember bool, hospitalRole string) (string, error) {
	if isGlobalSuperAdmin {
		return constant.RoleSuperAdmin, nil
	}
	if !isMember {
		return "", constant.ErrUserNotLinkedToHospital
	}
	role := strings.ToUpper(strings.TrimSpace(hospitalRole))
	if role == "" {
		return "", constant.ErrHospitalMembershipRoleRequired
	}
	return role, nil
}
