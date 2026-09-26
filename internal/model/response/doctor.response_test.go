package response

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestPublicDoctorJSONContainsOnlyProfessionalFields(t *testing.T) {
	data, err := json.Marshal(PublicDoctorDetail{PublicDoctor: PublicDoctor{
		DoctorID: "22222222-2222-4222-8222-222222222222", DoctorMedikaOneID: "MDO-0123456789ABCDEF",
	}, Affiliations: []PublicDoctorAffiliation{}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, privateField := range []string{"email", "phone", "nik", "dob", "address", "username", "password_hash"} {
		if _, exists := payload[privateField]; exists {
			t.Errorf("public directory exposes private field %s", privateField)
		}
	}
	if payload["doctor_medikaone_id"] != "MDO-0123456789ABCDEF" || payload["doctor_id"] == "" {
		t.Fatalf("public doctor identity missing: %s", data)
	}
	if _, ok := payload["affiliations"].([]any); !ok {
		t.Fatalf("affiliations must serialize as an array: %s", data)
	}
}

func TestAffiliationDetailGroupsHospitalAndOmitsOfferMessage(t *testing.T) {
	detail := DoctorHospitalAffiliationDetail{
		HospitalDoctor: HospitalDoctor{AffiliationID: "affiliation-id"},
		Hospital:       &HospitalInformation{ID: "hospital-id", Name: "RS MedikaOne", Facilities: json.RawMessage("[]"), OpeningHours: json.RawMessage("[]")},
		Invitation:     AffiliationInvitation{ID: "invitation-id", ContractFilename: "contract.pdf"},
	}
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	hospital, ok := payload["hospital"].(map[string]any)
	if !ok || hospital["id"] != "hospital-id" {
		t.Fatalf("hospital must be one populated object: %s", data)
	}
	invitation, ok := payload["invitation"].(map[string]any)
	if !ok || invitation["id"] != "invitation-id" {
		t.Fatalf("invitation summary missing: %s", data)
	}
	if _, exists := invitation["message"]; exists {
		t.Fatalf("affiliation offer must not expose invitation message: %s", data)
	}
}

func TestDoctorResponsesPreservePublicIdentityColumnMapping(t *testing.T) {
	// Explicit aliases are required: GORM's default for MedikaOne is medika_one.
	for _, value := range []any{
		PublicDoctor{}, DoctorSearchResult{}, DoctorHospitalInvitation{}, HospitalDoctor{},
		DoctorScheduleAvailability{}, DoctorTodaySchedule{}, Appointment{}, ScheduleChangeRequest{},
		MedicalEncounter{}, MedicalEncounterSummary{}, Prescription{}, PrescriptionSummary{}, PrescriptionVerification{},
	} {
		typeOf := reflect.TypeOf(value)
		field, ok := typeOf.FieldByName("DoctorMedikaOneID")
		if !ok || field.Tag.Get("json") != "doctor_medikaone_id" || field.Tag.Get("gorm") != "column:doctor_medikaone_id" {
			t.Errorf("%s must preserve the doctor_medikaone_id SQL and JSON name", typeOf.Name())
		}
	}
}

func TestDoctorProfileSIPSQLColumnMapping(t *testing.T) {
	for _, model := range []any{&DoctorProfile{}, &PublicDoctor{}, &DoctorSearchResult{}, &DoctorHospitalInvitation{}, &HospitalDoctor{}, &ScheduleChangeRequest{}, &MedicalEncounter{}, &MedicalEncounterSummary{}, &ConsultationNote{}, &Prescription{}, &PrescriptionRevision{}, &PrescriptionItem{}, &PrescriptionVerification{}} {
		parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse %T: %v", model, err)
		}
		if field, ok := parsed.FieldsByName["SIPNumber"]; ok && field.DBName != "sip_number" {
			t.Fatalf("%T SIPNumber SQL column = %s, want sip_number", model, field.DBName)
		}
		if field, ok := parsed.FieldsByName["DoctorSIPNumber"]; ok && field.DBName != "doctor_sip_number" {
			t.Fatalf("%T doctor SIP SQL column = %s, want doctor_sip_number", model, field.DBName)
		}
	}
}
