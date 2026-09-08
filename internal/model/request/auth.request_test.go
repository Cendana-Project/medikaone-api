package request_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

func TestDoctorProfileRequestSIPNumberMaximumLength(t *testing.T) {
	sixtyFourCharacters := strings.Repeat("S", 64)
	valid := request.DoctorProfileRequest{
		FirstName: "Dian",
		LastName:  "Sasmita",
		SIPNumber: &sixtyFourCharacters,
	}
	if err := util.ValidateStruct(valid); err != nil {
		t.Fatalf("64-character sip_number rejected: %v", err)
	}

	sixtyFiveCharacters := strings.Repeat("S", 65)
	invalid := request.DoctorProfileRequest{
		FirstName: "Dian",
		LastName:  "Sasmita",
		SIPNumber: &sixtyFiveCharacters,
	}
	if err := util.ValidateStruct(invalid); err == nil {
		t.Fatal("65-character sip_number must be rejected before database persistence")
	}
}

func TestUpdateProfilePatchFieldsPreserveOmittedAndNull(t *testing.T) {
	var input request.UpdateUserProfileRequest
	if err := json.Unmarshal([]byte(`{
		"phone": null,
		"patient_profile": {"height_cm": null},
		"doctor_profile": {"specialty": "Cardiology"}
	}`), &input); err != nil {
		t.Fatalf("decode update profile: %v", err)
	}
	if value, present := input.Phone.Get(); !present || value != nil {
		t.Fatalf("phone presence/value = %v/%v, want present null", present, value)
	}
	if input.Address.IsSet() {
		t.Fatal("omitted address must not be marked present")
	}
	if value, present := input.PatientProfile.HeightCM.Get(); !present || value != nil {
		t.Fatalf("height presence/value = %v/%v, want present null", present, value)
	}
	if value, present := input.DoctorProfile.Specialty.Get(); !present || value == nil || *value != "Cardiology" {
		t.Fatalf("specialty presence/value = %v/%v", present, value)
	}
}

func TestUpdateProfileRecordsExplicitNullRoleObjects(t *testing.T) {
	var input request.UpdateUserProfileRequest
	if err := util.UnmarshalStrictJSON([]byte(`{"first_name":"Budi","patient_profile":null}`), &input); err != nil {
		t.Fatalf("decode update profile: %v", err)
	}
	if !input.PatientProfileIsSet() || input.PatientProfile != nil {
		t.Fatalf("patient_profile state = set %v value %#v, want explicit null", input.PatientProfileIsSet(), input.PatientProfile)
	}
	if input.DoctorProfileIsSet() {
		t.Fatal("omitted doctor_profile must not be marked present")
	}
}

func TestUpdateProfileCustomDecoderStillRejectsUnknownNestedFields(t *testing.T) {
	var input request.UpdateUserProfileRequest
	err := util.UnmarshalStrictJSON([]byte(`{"patient_profile":{"unknown":true}}`), &input)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown nested field error = %v, want rejection", err)
	}
}
