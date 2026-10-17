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
		"doctor_profile": {"specialty_id": "11111111-1111-4111-8111-111111111111"}
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
	if value, present := input.DoctorProfile.SpecialtyID.Get(); !present || value == nil || *value != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("specialty_id presence/value = %v/%v", present, value)
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

func TestUpdateProfileRejectsLegacySpecialtyInput(t *testing.T) {
	var input request.UpdateUserProfileRequest
	err := util.UnmarshalStrictJSON([]byte(`{"doctor_profile":{"specialty":"Mata"}}`), &input)
	if err == nil || !strings.Contains(err.Error(), "specialty") {
		t.Fatalf("legacy specialty input must be rejected, got %v", err)
	}
}

func TestInitialDoctorProfileTracksSpecialtySelectionPresence(t *testing.T) {
	for _, test := range []struct {
		body    string
		present bool
	}{
		{body: `{}`, present: false},
		{body: `{"specialty_id":null}`, present: true},
		{body: `{"specialty_id":"11111111-1111-4111-8111-111111111111"}`, present: true},
	} {
		var input request.DoctorProfileRequest
		if err := util.UnmarshalStrictJSON([]byte(test.body), &input); err != nil {
			t.Fatal(err)
		}
		if input.SpecialtyIDIsSet() != test.present {
			t.Fatalf("%s selection presence = %v", test.body, input.SpecialtyIDIsSet())
		}
	}
}
