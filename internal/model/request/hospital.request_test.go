package request

import (
	"encoding/json"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/util"
)

func TestHospitalIDCannotBeBoundFromJSON(t *testing.T) {
	admin := &CreateHospitalAdminRequest{}
	if err := json.Unmarshal([]byte(`{"hospital_id":"attacker-tenant"}`), admin); err != nil {
		t.Fatal(err)
	}
	if admin.HospitalID != "" {
		t.Fatalf("admin hospital ID was populated from JSON: %q", admin.HospitalID)
	}

	staff := &CreateHospitalStaffRequest{}
	if err := json.Unmarshal([]byte(`{"hospital_id":"attacker-tenant"}`), staff); err != nil {
		t.Fatal(err)
	}
	if staff.HospitalID != "" {
		t.Fatalf("staff hospital ID was populated from JSON: %q", staff.HospitalID)
	}
}

func TestHospitalWorkerDOBIsRequired(t *testing.T) {
	dob := "2000-01-01"
	tests := []struct {
		name    string
		missing any
		valid   any
	}{
		{
			name: "admin",
			missing: CreateHospitalAdminRequest{
				Email: "admin@example.com", Username: "hospital_admin", Password: "StrongPass9!",
			},
			valid: CreateHospitalAdminRequest{
				Email: "admin@example.com", Username: "hospital_admin", Password: "StrongPass9!", DOB: &dob,
			},
		},
		{
			name: "staff",
			missing: CreateHospitalStaffRequest{
				Role: "NURSE", Email: "nurse@example.com", Username: "hospital_nurse", Password: "StrongPass9!",
			},
			valid: CreateHospitalStaffRequest{
				Role: "NURSE", Email: "nurse@example.com", Username: "hospital_nurse", Password: "StrongPass9!", DOB: &dob,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := util.ValidateStruct(test.missing); err == nil {
				t.Fatal("request without dob must be rejected")
			}
			if err := util.ValidateStruct(test.valid); err != nil {
				t.Fatalf("valid request rejected: %v", err)
			}
		})
	}
}
