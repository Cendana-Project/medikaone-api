package user

import (
	"encoding/json"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func TestToMeDTOIncludesUsernameAndNIK(t *testing.T) {
	username := "patient_example"
	nik := "3174015002950001"

	dto := toMeDTO(&entity.User{
		ID:        "11111111-1111-4111-8111-111111111111",
		Email:     "patient@example.com",
		Username:  &username,
		FirstName: "Siti",
		LastName:  "Aminah",
		NIK:       &nik,
		Status:    "active",
	}, "PATIENT")

	if dto.Username == nil || *dto.Username != username {
		t.Fatalf("username = %v, want %q", dto.Username, username)
	}
	if dto.NIK == nil || *dto.NIK != nik {
		t.Fatalf("nik = %v, want %q", dto.NIK, nik)
	}
}

func TestPatientProfileJSONIncludesAllPublicFields(t *testing.T) {
	height, weight := 160, 52
	allergies := "Udang"
	medicalHistory := "Asma ringan"
	dto := response.MeResponse{
		ID:        "11111111-1111-4111-8111-111111111111",
		Email:     "patient@example.com",
		FirstName: "Siti",
		LastName:  "Aminah",
		Status:    "active",
		Role:      "PATIENT",
		PatientProfile: &response.PatientProfile{
			HeightCM:       &height,
			WeightKG:       &weight,
			Allergies:      &allergies,
			MedicalHistory: &medicalHistory,
		},
	}

	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal profile response: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal profile response: %v", err)
	}
	patientProfile, ok := payload["patient_profile"].(map[string]any)
	if !ok {
		t.Fatalf("patient_profile = %#v, want object", payload["patient_profile"])
	}
	for _, field := range []string{"height_cm", "weight_kg", "allergies", "medical_history"} {
		if _, exists := patientProfile[field]; !exists {
			t.Errorf("patient_profile.%s is missing", field)
		}
	}
	if _, exists := patientProfile["medical_hist"]; exists {
		t.Error("legacy patient_profile.medical_hist must not be exposed")
	}
}

func TestProfileJSONKeepsRequestedNullableFields(t *testing.T) {
	dto := response.MeResponse{
		ID:             "11111111-1111-4111-8111-111111111111",
		Email:          "patient@example.com",
		FirstName:      "Siti",
		LastName:       "Aminah",
		Status:         "active",
		Role:           "PATIENT",
		PatientProfile: &response.PatientProfile{},
	}

	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal profile response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal profile response: %v", err)
	}
	for _, field := range []string{"username", "nik"} {
		value, exists := payload[field]
		if !exists {
			t.Errorf("%s is missing", field)
		} else if value != nil {
			t.Errorf("%s = %#v, want null", field, value)
		}
	}
	patientProfile, ok := payload["patient_profile"].(map[string]any)
	if !ok {
		t.Fatalf("patient_profile = %#v, want object", payload["patient_profile"])
	}
	for _, field := range []string{"height_cm", "weight_kg", "allergies", "medical_history"} {
		value, exists := patientProfile[field]
		if !exists {
			t.Errorf("patient_profile.%s is missing", field)
		} else if value != nil {
			t.Errorf("patient_profile.%s = %#v, want null", field, value)
		}
	}
}

func TestDoctorProfileJSONKeepsAllNullableFields(t *testing.T) {
	dto := response.MeResponse{
		ID:            "22222222-2222-4222-8222-222222222222",
		Email:         "doctor@example.com",
		FirstName:     "Dian",
		LastName:      "Sasmita",
		Status:        "active",
		Role:          "DOCTOR",
		DoctorProfile: &response.DoctorProfile{},
	}

	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal profile response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal profile response: %v", err)
	}
	doctorProfile, ok := payload["doctor_profile"].(map[string]any)
	if !ok {
		t.Fatalf("doctor_profile = %#v, want object", payload["doctor_profile"])
	}
	for _, field := range []string{"sip_number", "specialty"} {
		value, exists := doctorProfile[field]
		if !exists {
			t.Errorf("doctor_profile.%s is missing", field)
		} else if value != nil {
			t.Errorf("doctor_profile.%s = %#v, want null", field, value)
		}
	}
}
