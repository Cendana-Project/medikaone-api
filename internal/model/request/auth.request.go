package request

import "encoding/json"

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

// ChooseRoleRequest only supports the PATIENT self-service role. Privileged
// and clinical roles must be assigned through an authorized admin workflow.
type ChooseRoleRequest struct {
	Role string `json:"role" validate:"required,oneof_ci=PATIENT"`
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
	Role    string           `json:"role" validate:"required,oneof_ci=PATIENT"`
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
	Address   *string `json:"address,omitempty" validate:"omitempty,max=1000"`
	Gender    *string `json:"gender,omitempty" validate:"omitempty,oneof=L P"`
	SIPNumber *string `json:"sip_number,omitempty" validate:"omitempty,max=100"`
	Specialty *string `json:"specialty,omitempty" validate:"omitempty,max=100"`
}

// UpdateUserProfileRequest changes identity-neutral account details. Email is
// intentionally excluded because changing it requires a separate verification
// flow. Role-specific clinical fields remain on the patient/doctor endpoints.
type UpdateUserProfileRequest struct {
	Username  *string `json:"username,omitempty" validate:"omitempty,min=3,max=64,username"`
	FirstName *string `json:"first_name,omitempty" validate:"omitempty,max=100"`
	LastName  *string `json:"last_name,omitempty" validate:"omitempty,max=100"`
	Phone     *string `json:"phone,omitempty" validate:"omitempty,max=32"`
	DOB       *string `json:"dob,omitempty"`
	Address   *string `json:"address,omitempty" validate:"omitempty,max=1000"`
	Gender    *string `json:"gender,omitempty"`
	NIK       *string `json:"nik,omitempty"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"omitempty,max=4096"`
}
