package response

import "time"

type VitalSigns struct {
	ID                      string     `json:"id"`
	Version                 int        `json:"version"`
	Status                  string     `json:"status"`
	HeightCM                *float64   `json:"height_cm,omitempty"`
	WeightKG                *float64   `json:"weight_kg,omitempty"`
	BMI                     *float64   `json:"bmi,omitempty"`
	TemperatureC            *float64   `json:"temperature_c,omitempty"`
	SystolicMMHG            *int       `json:"systolic_mmhg,omitempty"`
	DiastolicMMHG           *int       `json:"diastolic_mmhg,omitempty"`
	HeartRateBPM            *int       `json:"heart_rate_bpm,omitempty"`
	RespiratoryRateBPM      *int       `json:"respiratory_rate_bpm,omitempty"`
	OxygenSaturationPercent *float64   `json:"oxygen_saturation_percent,omitempty"`
	NurseNote               *string    `json:"nurse_note,omitempty"`
	SkippedReason           *string    `json:"skipped_reason,omitempty"`
	RecordedBy              string     `json:"recorded_by"`
	FinalizedBy             *string    `json:"finalized_by,omitempty"`
	FinalizedAt             *time.Time `json:"finalized_at,omitempty"`
	SupersedesRevisionID    *string    `json:"supersedes_revision_id,omitempty"`
	CorrectionReason        *string    `json:"correction_reason,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type Diagnosis struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	ICD10     *string   `json:"icd10_code,omitempty"`
	Name      string    `json:"name"`
	Note      *string   `json:"note,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type ConsultationNote struct {
	ID                   string      `json:"id"`
	Version              int         `json:"version"`
	Status               string      `json:"status"`
	Subjective           *string     `json:"subjective,omitempty"`
	Objective            *string     `json:"objective,omitempty"`
	Assessment           *string     `json:"assessment,omitempty"`
	Plan                 *string     `json:"plan,omitempty"`
	InternalNote         *string     `json:"internal_note,omitempty"`
	AuthoredBy           string      `json:"authored_by"`
	FinalizedBy          *string     `json:"finalized_by,omitempty"`
	FinalizedAt          *time.Time  `json:"finalized_at,omitempty"`
	SupersedesRevisionID *string     `json:"supersedes_revision_id,omitempty"`
	CorrectionReason     *string     `json:"correction_reason,omitempty"`
	Diagnoses            []Diagnosis `json:"diagnoses"`
	CreatedAt            time.Time   `json:"created_at"`
	UpdatedAt            time.Time   `json:"updated_at"`
}

type MedicalAttachment struct {
	ID           string    `json:"id"`
	DocumentType string    `json:"document_type"`
	Filename     string    `json:"filename"`
	MIMEType     string    `json:"mime_type"`
	FileSize     int64     `json:"file_size"`
	SHA256       string    `json:"sha256"`
	Note         *string   `json:"note,omitempty"`
	UploadedBy   string    `json:"uploaded_by"`
	CreatedAt    time.Time `json:"created_at"`
}

type MedicalAttachmentURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MedicalEncounter struct {
	ID                    string              `json:"id"`
	AppointmentID         string              `json:"appointment_id"`
	PatientRecordID       string              `json:"patient_record_id"`
	PatientMedikaOneID    *string             `json:"patient_medikaone_id,omitempty"`
	PatientName           string              `json:"patient_name"`
	PatientDateOfBirth    string              `json:"patient_date_of_birth"`
	PatientGender         string              `json:"patient_gender"`
	PatientAllergies      *string             `json:"patient_allergies,omitempty"`
	PatientMedicalHistory *string             `json:"patient_medical_history,omitempty"`
	HospitalID            string              `json:"hospital_id"`
	HospitalName          string              `json:"hospital_name"`
	DoctorID              string              `json:"doctor_id"`
	DoctorName            string              `json:"doctor_name"`
	DepartmentID          string              `json:"department_id"`
	DepartmentName        string              `json:"department_name"`
	AppointmentDate       string              `json:"appointment_date"`
	ReasonForVisit        string              `json:"reason_for_visit"`
	Status                string              `json:"status"`
	Vitals                *VitalSigns         `json:"vitals,omitempty"`
	Consultation          *ConsultationNote   `json:"consultation,omitempty"`
	Attachments           []MedicalAttachment `json:"attachments"`
	CompletedAt           *time.Time          `json:"completed_at,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
}

type MedicalEncounterSummary struct {
	ID               string     `json:"id"`
	AppointmentID    string     `json:"appointment_id"`
	HospitalID       string     `json:"hospital_id"`
	HospitalName     string     `json:"hospital_name"`
	DoctorID         string     `json:"doctor_id"`
	DoctorName       string     `json:"doctor_name"`
	DepartmentName   string     `json:"department_name"`
	AppointmentDate  string     `json:"appointment_date"`
	PrimaryDiagnosis *Diagnosis `json:"primary_diagnosis,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}
