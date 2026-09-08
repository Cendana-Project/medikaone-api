package request

import (
	"bytes"
	"encoding/json"
	"strings"
)

// RegisterLiteRequest starts a short-lived registration challenge. The user is
// only persisted after the challenge PIN is verified.
type RegisterLiteRequest struct {
	Email    string `json:"email" validate:"required,email,max=190"`
	Username string `json:"username" validate:"required,min=3,max=64,username"`
	Phone    string `json:"phone" validate:"required,min=8,max=32"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type VerifyPINRequest struct {
	ChallengeID string `json:"challenge_id" validate:"required,max=64"`
	Email       string `json:"email" validate:"required,email,max=190"`
	PIN         string `json:"pin" validate:"required,len=6,numeric"`
}

type ResendPINRequest struct {
	ChallengeID string `json:"challenge_id" validate:"required,max=64"`
	Email       string `json:"email" validate:"required,email,max=190"`
}

type LoginRequest struct {
	Identity string `json:"identity" validate:"required,max=190"`
	Password string `json:"password" validate:"required,max=128"`
}

type RefreshTokenRequest struct {
	RefreshToken   string `json:"refresh_token" validate:"required,max=4096"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,uuid4"`
}

type LoginHospitalRequest struct {
	Identifier   string  `json:"identifier" validate:"required,max=190"`
	Password     string  `json:"password" validate:"required,max=128"`
	HospitalID   *string `json:"hospital_id" validate:"omitempty,uuid"`
	HospitalCode *string `json:"hospital_code" validate:"omitempty,max=64"`
}

// ChooseRoleRequest supports public patient and doctor identities. Privileged
// hospital staff roles must still be assigned through an admin workflow.
type ChooseRoleRequest struct {
	Role string `json:"role" validate:"required,oneof_ci=PATIENT DOCTOR"`
}

type PasswordForgotRequest struct {
	Email string `json:"email" validate:"required,email,max=190"`
}

type PasswordResetVerifyPINRequest struct {
	ChallengeID string `json:"challenge_id" validate:"required,max=64"`
	Email       string `json:"email" validate:"required,email,max=190"`
	PIN         string `json:"pin" validate:"required,len=6,numeric"`
}

type PasswordResetRequest struct {
	ChallengeID string `json:"challenge_id" validate:"required,max=64"`
	ResetToken  string `json:"reset_token" validate:"required,len=43"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=128"`
}

type PasswordChangeRequest struct {
	OldPassword string `json:"old_password" validate:"required,max=128"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=128"`
}

type SetProfileRequest struct {
	Role    string           `json:"role" validate:"required,oneof_ci=PATIENT DOCTOR"`
	Profile *json.RawMessage `json:"profile" validate:"required"`
}

type PatientProfileRequest struct {
	FirstName      string  `json:"first_name" validate:"required,max=100"`
	LastName       string  `json:"last_name" validate:"required,max=100"`
	NIK            *string `json:"nik,omitempty" validate:"omitempty,len=16,numeric"`
	DOB            *string `json:"dob,omitempty"` // YYYY-MM-DD
	Address        *string `json:"address,omitempty" validate:"omitempty,max=1000"`
	Gender         *string `json:"gender,omitempty" validate:"omitempty,oneof=L P"`
	HeightCM       *int    `json:"height_cm,omitempty" validate:"omitempty,gte=1,lte=300"`
	WeightKG       *int    `json:"weight_kg,omitempty" validate:"omitempty,gte=1,lte=1000"`
	Allergies      *string `json:"allergies,omitempty" validate:"omitempty,max=2000"`
	MedicalHistory *string `json:"medical_history,omitempty" validate:"omitempty,max=10000"`
}

type DoctorProfileRequest struct {
	FirstName string  `json:"first_name" validate:"required,max=100"`
	LastName  string  `json:"last_name" validate:"required,max=100"`
	DOB       *string `json:"dob,omitempty"`
	Address   *string `json:"address,omitempty" validate:"omitempty,max=1000"`
	Gender    *string `json:"gender,omitempty" validate:"omitempty,oneof=L P"`
	SIPNumber *string `json:"sip_number,omitempty" validate:"omitempty,max=64"`
	Specialty *string `json:"specialty,omitempty" validate:"omitempty,max=100"`
}

// PatchField preserves the difference between an omitted JSON property and a
// property explicitly set to null. That distinction is required by PATCH:
// omitted values stay unchanged, while null clears nullable profile values.
type PatchField[T any] struct {
	present bool
	value   *T
}

func (field *PatchField[T]) UnmarshalJSON(data []byte) error {
	field.present = true
	field.value = nil
	if strings.TrimSpace(string(data)) == "null" {
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	field.value = &value
	return nil
}

func (field PatchField[T]) IsSet() bool { return field.present }

func (field PatchField[T]) Get() (*T, bool) { return field.value, field.present }

func NewPatchValue[T any](value T) PatchField[T] {
	return PatchField[T]{present: true, value: &value}
}

func NewPatchNull[T any]() PatchField[T] { return PatchField[T]{present: true} }

// UpdateUserProfileRequest is the unified PATCH /v1/profile payload. Email is
// intentionally excluded because changing it requires a separate verification
// flow. Role-specific fields are accepted through their respective nested
// objects and authorized by the service.
type UpdateUserProfileRequest struct {
	Username       PatchField[string]           `json:"username,omitempty"`
	FirstName      PatchField[string]           `json:"first_name,omitempty"`
	LastName       PatchField[string]           `json:"last_name,omitempty"`
	Phone          PatchField[string]           `json:"phone,omitempty"`
	DOB            PatchField[string]           `json:"dob,omitempty"`
	Address        PatchField[string]           `json:"address,omitempty"`
	Gender         PatchField[string]           `json:"gender,omitempty"`
	NIK            PatchField[string]           `json:"nik,omitempty"`
	PatientProfile *UpdatePatientProfileRequest `json:"patient_profile,omitempty" validate:"omitempty"`
	DoctorProfile  *UpdateDoctorProfileRequest  `json:"doctor_profile,omitempty" validate:"omitempty"`

	patientProfilePresent bool
	doctorProfilePresent  bool
}

// UnmarshalJSON keeps strict nested decoding while recording whether a
// role-specific object was explicitly supplied as null. A null object cannot
// mean "delete the whole profile", so the service can reject it explicitly
// instead of silently treating it as omitted.
func (request *UpdateUserProfileRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateUserProfileRequest
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, patientPresent := fields["patient_profile"]
	_, doctorPresent := fields["doctor_profile"]

	*request = UpdateUserProfileRequest(decoded)
	request.patientProfilePresent = patientPresent
	request.doctorProfilePresent = doctorPresent
	return nil
}

func (request UpdateUserProfileRequest) PatientProfileIsSet() bool {
	return request.patientProfilePresent || request.PatientProfile != nil
}

func (request UpdateUserProfileRequest) DoctorProfileIsSet() bool {
	return request.doctorProfilePresent || request.DoctorProfile != nil
}

// UpdatePatientProfileRequest contains patient-only fields accepted by the
// unified PATCH /v1/profile endpoint. Every field is optional because PATCH
// only changes values explicitly supplied by the client.
type UpdatePatientProfileRequest struct {
	HeightCM       PatchField[int]    `json:"height_cm,omitempty"`
	WeightKG       PatchField[int]    `json:"weight_kg,omitempty"`
	Allergies      PatchField[string] `json:"allergies,omitempty"`
	MedicalHistory PatchField[string] `json:"medical_history,omitempty"`
}

// UpdateDoctorProfileRequest contains doctor-only fields accepted by the
// unified PATCH /v1/profile endpoint. An omitted SIP is left unchanged; an
// explicitly empty SIP is rejected by the service so an active doctor cannot
// clear their professional identifier.
type UpdateDoctorProfileRequest struct {
	SIPNumber PatchField[string] `json:"sip_number,omitempty"`
	Specialty PatchField[string] `json:"specialty,omitempty"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"omitempty,max=4096"`
}
