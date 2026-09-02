package response

import "time"

type RegisterResponse struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Status string `json:"status"`
}

type VerifyEmailResponse struct {
	Email  string `json:"email"`
	Status string `json:"status"`
}

type PasswordResetVerifyPINResponse struct {
	Status     string `json:"status"`
	ResetToken string `json:"reset_token"`
	ExpiresIn  int64  `json:"expires_in"`
}

type RoleBrief struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type LoginResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	Role                  string `json:"role"`                     // hanya slug
	AccessTokenExpiredAt  string `json:"access_token_expired_at"`  // RFC3339 UTC
	RefreshTokenExpiredAt string `json:"refresh_token_expired_at"` // RFC3339 UTC
}

// LoginHospitalResponse sekarang menyertakan waktu kadaluarsa token
type LoginHospitalResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int64  `json:"expires_in"`
	TokenType             string `json:"token_type"`
	HospitalID            string `json:"hospital_id"`
	Role                  string `json:"role"`                     // hanya slug
	AccessTokenExpiredAt  string `json:"access_token_expired_at"`  // RFC3339 UTC
	RefreshTokenExpiredAt string `json:"refresh_token_expired_at"` // RFC3339 UTC
}

type UserProfile struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Username  *string `json:"username,omitempty"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Phone     *string `json:"phone,omitempty"`
	DOB       *string `json:"dob,omitempty"` // format: YYYY-MM-DD (jika ada)
	Address   *string `json:"address,omitempty"`
	Gender    *string `json:"gender,omitempty"` // L|P
	NIK       *string `json:"nik,omitempty"`

	// Patient-only
	HeightCM       *int    `json:"height_cm,omitempty"`
	WeightKG       *int    `json:"weight_kg,omitempty"`
	Allergies      *string `json:"allergies,omitempty"`
	MedicalHistory *string `json:"medical_history,omitempty"`

	// Doctor-only
	SIPNumber *string `json:"sip_number,omitempty"`
	Specialty *string `json:"specialty,omitempty"`

	// Timestamps opsional (kalau suatu saat diperlukan)
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type SetProfileResponse struct {
	Role    string      `json:"role"`    // slug UPPERCASE yang dipilih/diassign
	Profile UserProfile `json:"profile"` // profil lengkap gabungan
}
