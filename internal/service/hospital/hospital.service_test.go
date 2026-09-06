package hospital

import (
	"errors"
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

func TestValidateHospitalWorkerDOB(t *testing.T) {
	now := time.Date(2026, 9, 7, 18, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value *string
		want  error
	}{
		{name: "missing", value: nil, want: constant.NewFieldRequiredError("dob")},
		{name: "invalid date", value: strPtr("not-a-date"), want: constant.ErrInvalidDateFormat},
		{name: "future", value: strPtr("2027-09-07"), want: constant.ErrInvalidDateFormat},
		{name: "younger than fifteen", value: strPtr("2012-09-07"), want: constant.ErrHospitalWorkerMinimumAge},
		{name: "exactly fifteen", value: strPtr("2011-09-07"), want: constant.ErrHospitalWorkerMinimumAge},
		{name: "older than fifteen", value: strPtr("2011-09-06")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateHospitalWorkerDOB(test.value, now)
			if test.want == nil && err != nil {
				t.Fatalf("valid date of birth rejected: %v", err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func strPtr(value string) *string { return &value }
