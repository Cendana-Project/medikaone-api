package hospital

import (
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

func TestNormalizeHospitalStaffRole(t *testing.T) {
	accepted := []string{
		constant.RoleNurse,
		constant.RoleReceptionist,
		constant.RoleBOD,
	}
	for _, input := range accepted {
		got, ok := normalizeHospitalStaffRole(input)
		if !ok {
			t.Fatalf("expected %q to be accepted", input)
		}
		if got != strings.ToUpper(strings.TrimSpace(input)) {
			t.Fatalf("role %q normalized to %q", input, got)
		}
	}

	for _, input := range []string{"", constant.RoleDoctor, constant.RolePatient, constant.RoleAdmin, constant.RoleSuperAdmin, "unknown"} {
		if _, ok := normalizeHospitalStaffRole(input); ok {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestParseDOBRejectsFutureDate(t *testing.T) {
	future := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	if _, err := parseDOB(&future); err == nil {
		t.Fatal("future date of birth was accepted")
	}
}
