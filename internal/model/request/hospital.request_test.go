package request

import (
	"encoding/json"
	"testing"
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
