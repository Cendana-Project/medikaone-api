package request

type VitalSignsRequest struct {
	HeightCM                *float64 `json:"height_cm,omitempty" validate:"omitempty,gte=30,lte=300"`
	WeightKG                *float64 `json:"weight_kg,omitempty" validate:"omitempty,gte=1,lte=1000"`
	TemperatureC            *float64 `json:"temperature_c,omitempty" validate:"omitempty,gte=25,lte=50"`
	SystolicMMHG            *int     `json:"systolic_mmhg,omitempty" validate:"omitempty,gte=40,lte=300"`
	DiastolicMMHG           *int     `json:"diastolic_mmhg,omitempty" validate:"omitempty,gte=20,lte=200"`
	HeartRateBPM            *int     `json:"heart_rate_bpm,omitempty" validate:"omitempty,gte=20,lte=300"`
	RespiratoryRateBPM      *int     `json:"respiratory_rate_bpm,omitempty" validate:"omitempty,gte=5,lte=100"`
	OxygenSaturationPercent *float64 `json:"oxygen_saturation_percent,omitempty" validate:"omitempty,gte=1,lte=100"`
	NurseNote               *string  `json:"nurse_note,omitempty" validate:"omitempty,max=4000"`
	SkippedReason           *string  `json:"skipped_reason,omitempty" validate:"omitempty,max=1000"`
}

type CorrectVitalSignsRequest struct {
	VitalSignsRequest
	CorrectionReason string `json:"correction_reason" validate:"required,max=1000"`
}

type DiagnosisRequest struct {
	Type   string  `json:"type" validate:"required,oneof=PRIMARY SECONDARY"`
	Status string  `json:"status" validate:"required,oneof=SUSPECTED CONFIRMED RULED_OUT"`
	ICD10  *string `json:"icd10_code,omitempty" validate:"omitempty,max=16"`
	Name   string  `json:"name" validate:"required,max=500"`
	Note   *string `json:"note,omitempty" validate:"omitempty,max=4000"`
}

type ConsultationNoteRequest struct {
	Subjective   *string            `json:"subjective,omitempty" validate:"omitempty,max=10000"`
	Objective    *string            `json:"objective,omitempty" validate:"omitempty,max=10000"`
	Assessment   *string            `json:"assessment,omitempty" validate:"omitempty,max=10000"`
	Plan         *string            `json:"plan,omitempty" validate:"omitempty,max=10000"`
	InternalNote *string            `json:"internal_note,omitempty" validate:"omitempty,max=4000"`
	Diagnoses    []DiagnosisRequest `json:"diagnoses,omitempty" validate:"max=50,dive"`
}

type CorrectConsultationNoteRequest struct {
	ConsultationNoteRequest
	CorrectionReason string `json:"correction_reason" validate:"required,max=1000"`
}

// CorrectHospitalConsultationNoteRequest deliberately excludes internal_note.
// Hospital administrators may append a clinical correction, but doctor-only
// context is carried forward by the service and cannot be read or overwritten.
type CorrectHospitalConsultationNoteRequest struct {
	Subjective       *string            `json:"subjective,omitempty" validate:"omitempty,max=10000"`
	Objective        *string            `json:"objective,omitempty" validate:"omitempty,max=10000"`
	Assessment       *string            `json:"assessment,omitempty" validate:"omitempty,max=10000"`
	Plan             *string            `json:"plan,omitempty" validate:"omitempty,max=10000"`
	Diagnoses        []DiagnosisRequest `json:"diagnoses,omitempty" validate:"max=50,dive"`
	CorrectionReason string             `json:"correction_reason" validate:"required,max=1000"`
}
