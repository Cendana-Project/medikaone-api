package entity

const (
	MedicalEncounterOpen      = "OPEN"
	MedicalEncounterCompleted = "COMPLETED"

	MedicalRevisionDraft     = "DRAFT"
	MedicalRevisionFinalized = "FINALIZED"

	DiagnosisPrimary   = "PRIMARY"
	DiagnosisSecondary = "SECONDARY"

	DiagnosisSuspected = "SUSPECTED"
	DiagnosisConfirmed = "CONFIRMED"
	DiagnosisRuledOut  = "RULED_OUT"

	MedicalDocumentLab      = "LAB_RESULT"
	MedicalDocumentImaging  = "IMAGING"
	MedicalDocumentClinical = "CLINICAL_DOCUMENT"
	MedicalDocumentOther    = "OTHER"
)
