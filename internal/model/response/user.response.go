package response

import "time"

// MeResponse adalah payload untuk GET /v1/me.
type MeResponse struct {
	ID         string  `json:"id"`
	Email      string  `json:"email"`
	Username   *string `json:"username,omitempty"`
	FirstName  string  `json:"first_name"`
	LastName   string  `json:"last_name"`
	Phone      *string `json:"phone,omitempty"`
	Gender     *string `json:"gender,omitempty"` // "L" | "P"
	DOB        *string `json:"dob,omitempty"`    // "YYYY-MM-DD"
	Address    *string `json:"address,omitempty"`
	Status     string  `json:"status"`
	VerifiedAt *string `json:"verified_at,omitempty"`
	Role       string  `json:"role"` // single role slug

	// Opsional per-role:
	PatientProfile *PatientProfile `json:"patient_profile,omitempty"`
	DoctorProfile  *DoctorProfile  `json:"doctor_profile,omitempty"`
	// Untuk staff/doctor yang terkait tenant, tampilkan membership hospital minimal:
	Hospitals    []HospitalBrief       `json:"hospitals,omitempty"`
	ProfilePhoto *ProfilePhotoMetadata `json:"profile_photo,omitempty"`
}

type ProfilePhotoMetadata struct {
	ContentType string    `json:"content_type"`
	FileSize    int64     `json:"file_size"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProfilePhotoURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PatientProfile untuk user berperan PATIENT.
type PatientProfile struct {
	HeightCM    *int    `json:"height_cm,omitempty"`
	WeightKG    *int    `json:"weight_kg,omitempty"`
	Allergies   *string `json:"allergies,omitempty"`
	MedicalHist *string `json:"medical_hist,omitempty"`
}

// DoctorProfile untuk user berperan DOCTOR.
type DoctorProfile struct {
	SIPNumber *string `json:"sip_number"`
	Specialty *string `json:"specialty,omitempty"`
}

// HospitalBrief untuk ringkas info keanggotaan hospital user (multi-tenant).
type HospitalBrief struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
