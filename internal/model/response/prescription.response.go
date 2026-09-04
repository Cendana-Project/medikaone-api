package response

import "time"

type MedicationCatalog struct {
	ID                 string    `json:"id"`
	HospitalID         string    `json:"hospital_id"`
	Code               *string   `json:"code,omitempty"`
	KFACode            *string   `json:"kfa_code,omitempty"`
	GenericName        string    `json:"generic_name"`
	BrandName          *string   `json:"brand_name,omitempty"`
	DosageForm         string    `json:"dosage_form"`
	Strength           string    `json:"strength"`
	DefaultUnit        string    `json:"default_unit"`
	DefaultRoute       *string   `json:"default_route,omitempty"`
	ControlledCategory string    `json:"controlled_category"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PrescriptionComponent struct {
	ID                  string  `json:"id"`
	MedicationID        *string `json:"medication_id,omitempty"`
	MedicationName      string  `json:"medication_name"`
	DosageForm          string  `json:"dosage_form"`
	Strength            string  `json:"strength"`
	Amount              float64 `json:"amount"`
	Unit                string  `json:"unit"`
	ControlledSubstance bool    `json:"controlled_substance"`
}

type PrescriptionItem struct {
	ID                           string                  `json:"id"`
	Order                        int                     `json:"order"`
	Type                         string                  `json:"type"`
	MedicationID                 *string                 `json:"medication_id,omitempty"`
	MedicationName               string                  `json:"medication_name"`
	DosageForm                   string                  `json:"dosage_form"`
	Strength                     string                  `json:"strength"`
	DoseAmount                   float64                 `json:"dose_amount"`
	DoseUnit                     string                  `json:"dose_unit"`
	Route                        string                  `json:"route"`
	FrequencyPerDay              *int                    `json:"frequency_per_day,omitempty"`
	IntervalHours                *int                    `json:"interval_hours,omitempty"`
	TimingInstructions           *string                 `json:"timing_instructions,omitempty"`
	DurationValue                int                     `json:"duration_value"`
	DurationUnit                 string                  `json:"duration_unit"`
	Quantity                     float64                 `json:"quantity"`
	QuantityUnit                 string                  `json:"quantity_unit"`
	Directions                   string                  `json:"directions"`
	AsNeeded                     bool                    `json:"as_needed"`
	MaxDailyDose                 *string                 `json:"max_daily_dose,omitempty"`
	ControlledSubstance          bool                    `json:"controlled_substance"`
	SATUSEHATMedicationRequestID *string                 `json:"satusehat_medication_request_id,omitempty"`
	Components                   []PrescriptionComponent `json:"components"`
}

type PrescriptionDocument struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	MIMEType    string    `json:"mime_type"`
	FileSize    int64     `json:"file_size"`
	SHA256      string    `json:"sha256"`
	GeneratedAt time.Time `json:"generated_at"`
}

type PrescriptionRevision struct {
	ID                         string                `json:"id"`
	Version                    int                   `json:"version"`
	Status                     string                `json:"status"`
	GeneralNote                *string               `json:"general_note,omitempty"`
	AllergiesReviewed          bool                  `json:"allergies_reviewed"`
	RepeatsAllowed             int                   `json:"repeats_allowed"`
	AuthoredBy                 string                `json:"authored_by"`
	IssuedBy                   *string               `json:"issued_by,omitempty"`
	IssuedAt                   *time.Time            `json:"issued_at,omitempty"`
	SupersedesRevisionID       *string               `json:"supersedes_revision_id,omitempty"`
	CorrectionReason           *string               `json:"correction_reason,omitempty"`
	PatientNameSnapshot        string                `json:"-"`
	PatientDateOfBirthSnapshot string                `json:"-"`
	PatientGenderSnapshot      string                `json:"-"`
	PatientAllergiesSnapshot   *string               `json:"-"`
	HospitalNameSnapshot       string                `json:"-"`
	HospitalAddressSnapshot    *string               `json:"-"`
	HospitalPhoneSnapshot      *string               `json:"-"`
	DoctorNameSnapshot         string                `json:"-"`
	DoctorSIPNumberSnapshot    string                `json:"-"`
	Items                      []PrescriptionItem    `json:"items"`
	Document                   *PrescriptionDocument `json:"document,omitempty"`
	CreatedAt                  time.Time             `json:"created_at"`
	UpdatedAt                  time.Time             `json:"updated_at"`
}

type Prescription struct {
	ID                 string                `json:"id"`
	EncounterID        string                `json:"encounter_id"`
	AppointmentID      string                `json:"appointment_id"`
	PrescriptionNumber string                `json:"prescription_number"`
	Status             string                `json:"status"`
	NoMedicationReason *string               `json:"no_medication_reason,omitempty"`
	CancellationReason *string               `json:"cancellation_reason,omitempty"`
	CancelledAt        *time.Time            `json:"cancelled_at,omitempty"`
	PatientRecordID    string                `json:"patient_record_id"`
	PatientUserID      *string               `json:"patient_user_id,omitempty"`
	PatientName        string                `json:"patient_name"`
	PatientDateOfBirth string                `json:"patient_date_of_birth"`
	PatientGender      string                `json:"patient_gender"`
	PatientAllergies   *string               `json:"patient_allergies,omitempty"`
	HospitalID         string                `json:"hospital_id"`
	HospitalName       string                `json:"hospital_name"`
	HospitalAddress    *string               `json:"hospital_address,omitempty"`
	HospitalPhone      *string               `json:"hospital_phone,omitempty"`
	DoctorID           string                `json:"doctor_id"`
	DoctorName         string                `json:"doctor_name"`
	DoctorSIPNumber    *string               `json:"doctor_sip_number,omitempty"`
	AppointmentDate    string                `json:"appointment_date"`
	CurrentRevision    *PrescriptionRevision `json:"current_revision,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
}

type PrescriptionSummary struct {
	ID                 string     `json:"id"`
	EncounterID        string     `json:"encounter_id"`
	AppointmentID      string     `json:"appointment_id"`
	PrescriptionNumber string     `json:"prescription_number"`
	Status             string     `json:"status"`
	HospitalName       string     `json:"hospital_name"`
	DoctorName         string     `json:"doctor_name"`
	AppointmentDate    string     `json:"appointment_date"`
	IssuedAt           *time.Time `json:"issued_at,omitempty"`
	ItemCount          int        `json:"item_count"`
}

type PrescriptionDocumentURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PrescriptionVerification struct {
	Valid              bool               `json:"valid"`
	PrescriptionNumber string             `json:"prescription_number"`
	Status             string             `json:"status"`
	PatientName        string             `json:"patient_name"`
	HospitalName       string             `json:"hospital_name"`
	DoctorName         string             `json:"doctor_name"`
	DoctorSIPNumber    *string            `json:"doctor_sip_number,omitempty"`
	IssuedAt           time.Time          `json:"issued_at"`
	Items              []PrescriptionItem `json:"items"`
}
