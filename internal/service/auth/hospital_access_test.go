package auth

import (
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

func TestResolveHospitalSessionRole(t *testing.T) {
	tests := []struct {
		name       string
		isSuper    bool
		isMember   bool
		tenantRole string
		wantRole   string
		wantErr    error
	}{
		{name: "global superadmin without membership", isSuper: true, wantRole: constant.RoleSuperAdmin},
		{name: "global superadmin ignores stale tenant role", isSuper: true, isMember: true, tenantRole: constant.RoleAdmin, wantRole: constant.RoleSuperAdmin},
		{name: "non-member", tenantRole: constant.RoleAdmin, wantErr: constant.ErrUserNotLinkedToHospital},
		{name: "member without tenant role", isMember: true, wantErr: constant.ErrHospitalMembershipRoleRequired},
		{name: "member with normalized tenant role", isMember: true, tenantRole: " doctor ", wantRole: constant.RoleDoctor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveHospitalSessionRole(test.isSuper, test.isMember, test.tenantRole)
			if err != test.wantErr {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if got != test.wantRole {
				t.Fatalf("role = %q, want %q", got, test.wantRole)
			}
		})
	}
}
