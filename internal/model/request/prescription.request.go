package request

type MedicationCatalogRequest struct {
	Code         *string `json:"code,omitempty" validate:"omitempty,max=64"`
	KFACode      *string `json:"kfa_code,omitempty" validate:"omitempty,max=64"`
	GenericName  string  `json:"generic_name" validate:"required,max=255"`
	BrandName    *string `json:"brand_name,omitempty" validate:"omitempty,max=255"`
	DosageForm   string  `json:"dosage_form" validate:"required,max=100"`
	Strength     string  `json:"strength" validate:"required,max=100"`
	DefaultUnit  string  `json:"default_unit" validate:"required,max=50"`
	DefaultRoute *string `json:"default_route,omitempty" validate:"omitempty,max=50"`
	IsActive     *bool   `json:"is_active,omitempty"`
}

type PrescriptionComponentRequest struct {
	MedicationID        *string `json:"medication_id,omitempty" validate:"omitempty,uuid"`
	MedicationName      string  `json:"medication_name" validate:"required,max=255"`
	DosageForm          string  `json:"dosage_form" validate:"required,max=100"`
	Strength            string  `json:"strength" validate:"required,max=100"`
	Amount              float64 `json:"amount" validate:"required,gt=0,lte=1000000"`
	Unit                string  `json:"unit" validate:"required,max=50"`
	ControlledSubstance bool    `json:"controlled_substance"`
}

type PrescriptionItemRequest struct {
	Type                string                         `json:"type" validate:"required,oneof=NON_COMPOUND COMPOUND"`
	MedicationID        *string                        `json:"medication_id,omitempty" validate:"omitempty,uuid"`
	MedicationName      string                         `json:"medication_name" validate:"required,max=255"`
	DosageForm          string                         `json:"dosage_form" validate:"required,max=100"`
	Strength            string                         `json:"strength" validate:"required,max=100"`
	DoseAmount          float64                        `json:"dose_amount" validate:"required,gt=0,lte=1000000"`
	DoseUnit            string                         `json:"dose_unit" validate:"required,max=50"`
	Route               string                         `json:"route" validate:"required,max=50"`
	FrequencyPerDay     *int                           `json:"frequency_per_day,omitempty" validate:"omitempty,gte=1,lte=24"`
	IntervalHours       *int                           `json:"interval_hours,omitempty" validate:"omitempty,gte=1,lte=168"`
	TimingInstructions  *string                        `json:"timing_instructions,omitempty" validate:"omitempty,max=500"`
	DurationValue       int                            `json:"duration_value" validate:"required,gte=1,lte=3650"`
	DurationUnit        string                         `json:"duration_unit" validate:"required,oneof=DAY WEEK MONTH"`
	Quantity            float64                        `json:"quantity" validate:"required,gt=0,lte=1000000"`
	QuantityUnit        string                         `json:"quantity_unit" validate:"required,max=50"`
	Directions          string                         `json:"directions" validate:"required,max=2000"`
	AsNeeded            bool                           `json:"as_needed"`
	MaxDailyDose        *string                        `json:"max_daily_dose,omitempty" validate:"omitempty,max=100"`
	ControlledSubstance bool                           `json:"controlled_substance"`
	Components          []PrescriptionComponentRequest `json:"components,omitempty" validate:"max=50,dive"`
}

type PrescriptionDraftRequest struct {
	GeneralNote    *string                   `json:"general_note,omitempty" validate:"omitempty,max=4000"`
	RepeatsAllowed int                       `json:"repeats_allowed" validate:"gte=0,lte=12"`
	Items          []PrescriptionItemRequest `json:"items" validate:"required,min=1,max=50,dive"`
}

type IssuePrescriptionRequest struct {
	ExpectedRevisionID string `json:"expected_revision_id" validate:"required,uuid"`
	AllergiesReviewed  bool   `json:"allergies_reviewed"`
}

type NoMedicationRequest struct {
	Reason string `json:"reason" validate:"required,max=1000"`
}

type CorrectPrescriptionRequest struct {
	PrescriptionDraftRequest
	ExpectedRevisionID string `json:"expected_revision_id" validate:"required,uuid"`
	AllergiesReviewed  bool   `json:"allergies_reviewed"`
	CorrectionReason   string `json:"correction_reason" validate:"required,max=1000"`
}

type CancelPrescriptionRequest struct {
	ExpectedRevisionID string `json:"expected_revision_id" validate:"required,uuid"`
	Reason             string `json:"reason" validate:"required,max=1000"`
}
