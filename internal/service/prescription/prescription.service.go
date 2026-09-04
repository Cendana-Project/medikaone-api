package prescription

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/prescription"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type Repository interface {
	GetAppointmentContext(context.Context, string) (*repository.AppointmentContext, error)
	HasPrimaryDiagnosis(context.Context, string) (bool, error)
	CreateMedication(context.Context, string, string, request.MedicationCatalogRequest, time.Time) (*response.MedicationCatalog, error)
	UpdateMedication(context.Context, string, string, string, request.MedicationCatalogRequest, time.Time) (*response.MedicationCatalog, error)
	ListMedications(context.Context, string, string, bool) ([]response.MedicationCatalog, error)
	GetMedication(context.Context, string, string) (*response.MedicationCatalog, error)
	SaveDraft(context.Context, *repository.AppointmentContext, string, string, request.PrescriptionDraftRequest, time.Time) error
	MarkNoMedication(context.Context, *repository.AppointmentContext, string, string, string, time.Time) error
	Issue(context.Context, string, string, string, string, repository.ClinicalSnapshot, repository.DocumentInput, time.Time) error
	Correct(context.Context, *response.Prescription, string, string, request.CorrectPrescriptionRequest, repository.ClinicalSnapshot, repository.DocumentInput, time.Time) error
	Cancel(context.Context, string, string, string, string, time.Time) error
	GetByAppointment(context.Context, string) (*response.Prescription, error)
	GetForPatient(context.Context, string, string) (*response.Prescription, error)
	ListForPatient(context.Context, string) ([]response.PrescriptionSummary, error)
	GetDocument(context.Context, string) (*repository.DocumentRecord, error)
	Verify(context.Context, string) (*response.PrescriptionVerification, error)
	LogAccess(context.Context, string, *string, string, string, time.Time) error
}

type PDFRenderer interface {
	Render(*response.Prescription, string) ([]byte, error)
}

type Service struct {
	repo         Repository
	storage      storageclient.Client
	renderer     PDFRenderer
	signedURLTTL time.Duration
	baseURL      string
	now          func() time.Time
}

func NewService(repo Repository, storage storageclient.Client, renderer PDFRenderer, signedURLTTL time.Duration, baseURL string) *Service {
	if renderer == nil {
		renderer = NewPDFRenderer()
	}
	if signedURLTTL <= 0 {
		signedURLTTL = 5 * time.Minute
	}
	return &Service{
		repo: repo, storage: storage, renderer: renderer, signedURLTTL: signedURLTTL,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) CreateMedication(ctx context.Context, hospitalID, actorID string, input request.MedicationCatalogRequest) (*response.MedicationCatalog, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	input = normalizeMedication(input)
	if err := validateMedication(input); err != nil {
		return nil, err
	}
	row, err := s.repo.CreateMedication(ctx, hospitalID, actorID, input, s.now())
	return row, mapMedicationError(err)
}

func (s *Service) UpdateMedication(ctx context.Context, hospitalID, medicationID, actorID string, input request.MedicationCatalogRequest) (*response.MedicationCatalog, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := uuid.Parse(medicationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	input = normalizeMedication(input)
	if err := validateMedication(input); err != nil {
		return nil, err
	}
	row, err := s.repo.UpdateMedication(ctx, hospitalID, medicationID, actorID, input, s.now())
	return row, mapMedicationError(err)
}

func (s *Service) ListHospitalMedications(ctx context.Context, hospitalID, search string, includeInactive bool) ([]response.MedicationCatalog, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if len([]rune(search)) > 100 {
		return nil, constant.NewInvalidFieldLengthError("q", "at most 100 characters", "maksimal 100 karakter")
	}
	rows, err := s.repo.ListMedications(ctx, hospitalID, search, includeInactive)
	return rows, mapError(err)
}

func (s *Service) ListDoctorMedications(ctx context.Context, doctorID, appointmentID, search string) ([]response.MedicationCatalog, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if len([]rune(search)) > 100 {
		return nil, constant.NewInvalidFieldLengthError("q", "at most 100 characters", "maksimal 100 karakter")
	}
	rows, err := s.repo.ListMedications(ctx, appointment.HospitalID, search, false)
	return rows, mapError(err)
}

func (s *Service) SaveDraft(ctx context.Context, doctorID, appointmentID string, input request.PrescriptionDraftRequest) (*response.Prescription, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if appointment.AppointmentStatus != "IN_CONSULTATION" {
		return nil, constant.ErrPrescriptionInvalidState
	}
	if appointment.DoctorSIPNumber == nil || strings.TrimSpace(*appointment.DoctorSIPNumber) == "" {
		return nil, constant.ErrPrescriptionDoctorSIPRequired
	}
	hasDiagnosis, err := s.repo.HasPrimaryDiagnosis(ctx, appointment.EncounterID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if !hasDiagnosis {
		return nil, constant.ErrPrescriptionPrimaryDiagnosisRequired
	}
	input, err = s.normalizeDraft(ctx, appointment.HospitalID, input)
	if err != nil {
		return nil, err
	}
	number := newPrescriptionNumber(s.now())
	if err := s.repo.SaveDraft(ctx, appointment, doctorID, number, input, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetByAppointment(ctx, appointmentID)
}

func (s *Service) MarkNoMedication(ctx context.Context, doctorID, appointmentID string, input request.NoMedicationRequest) (*response.Prescription, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if appointment.AppointmentStatus != "IN_CONSULTATION" {
		return nil, constant.ErrPrescriptionInvalidState
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, constant.NewFieldRequiredError("reason")
	}
	if err := s.repo.MarkNoMedication(ctx, appointment, doctorID, newPrescriptionNumber(s.now()), reason, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetByAppointment(ctx, appointmentID)
}

func (s *Service) Issue(ctx context.Context, doctorID, appointmentID string, input request.IssuePrescriptionRequest) (*response.Prescription, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(input.ExpectedRevisionID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetByAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if !currentRevisionMatches(row, input.ExpectedRevisionID) {
		return nil, constant.ErrPrescriptionConcurrentUpdate
	}
	if row.Status == "ISSUED" {
		return row, nil
	}
	if appointment.AppointmentStatus != "IN_CONSULTATION" {
		return nil, constant.ErrPrescriptionInvalidState
	}
	if appointment.DoctorSIPNumber == nil || strings.TrimSpace(*appointment.DoctorSIPNumber) == "" {
		return nil, constant.ErrPrescriptionDoctorSIPRequired
	}
	if !input.AllergiesReviewed {
		return nil, constant.ErrPrescriptionAllergyReviewRequired
	}
	hasDiagnosis, err := s.repo.HasPrimaryDiagnosis(ctx, appointment.EncounterID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if !hasDiagnosis {
		return nil, constant.ErrPrescriptionPrimaryDiagnosisRequired
	}
	if row.Status != "DRAFT" || row.CurrentRevision == nil || len(row.CurrentRevision.Items) == 0 {
		return nil, constant.ErrPrescriptionDraftRequired
	}
	applyAppointmentIdentity(row, appointment)
	return s.publish(ctx, row, doctorID, "", nil)
}

func (s *Service) Correct(ctx context.Context, doctorID, appointmentID string, input request.CorrectPrescriptionRequest) (*response.Prescription, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if appointment.DoctorSIPNumber == nil || strings.TrimSpace(*appointment.DoctorSIPNumber) == "" {
		return nil, constant.ErrPrescriptionDoctorSIPRequired
	}
	if _, err := uuid.Parse(input.ExpectedRevisionID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if !input.AllergiesReviewed {
		return nil, constant.ErrPrescriptionAllergyReviewRequired
	}
	input.CorrectionReason = strings.TrimSpace(input.CorrectionReason)
	if input.CorrectionReason == "" {
		return nil, constant.NewFieldRequiredError("correction_reason")
	}
	row, err := s.repo.GetByAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if row.Status != "ISSUED" || row.CurrentRevision == nil {
		return nil, constant.ErrPrescriptionCorrectionInvalidState
	}
	if !currentRevisionMatches(row, input.ExpectedRevisionID) {
		return nil, constant.ErrPrescriptionConcurrentUpdate
	}
	applyAppointmentIdentity(row, appointment)
	normalized, err := s.normalizeDraft(ctx, row.HospitalID, input.PrescriptionDraftRequest)
	if err != nil {
		return nil, err
	}
	input.PrescriptionDraftRequest = normalized
	return s.publish(ctx, row, doctorID, input.CorrectionReason, &input)
}

func (s *Service) Cancel(ctx context.Context, doctorID, appointmentID string, input request.CancelPrescriptionRequest) (*response.Prescription, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, constant.NewFieldRequiredError("reason")
	}
	if _, err := uuid.Parse(input.ExpectedRevisionID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetByAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if row.Status == "CANCELLED" && currentRevisionMatches(row, input.ExpectedRevisionID) &&
		row.CancellationReason != nil && strings.TrimSpace(*row.CancellationReason) == reason {
		return row, nil
	}
	if row.Status != "ISSUED" {
		return nil, constant.ErrPrescriptionCancellationInvalidState
	}
	if !currentRevisionMatches(row, input.ExpectedRevisionID) {
		return nil, constant.ErrPrescriptionConcurrentUpdate
	}
	if err := s.repo.Cancel(ctx, row.ID, input.ExpectedRevisionID, doctorID, reason, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetByAppointment(ctx, appointmentID)
}

func (s *Service) publish(ctx context.Context, row *response.Prescription, doctorID, correctionReason string, correction *request.CorrectPrescriptionRequest) (*response.Prescription, error) {
	token, tokenHash, err := newVerificationToken()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	publishedAt := s.now()
	renderRow := cloneForIssue(row, doctorID, publishedAt)
	version := row.CurrentRevision.Version
	if correction != nil {
		version++
		renderRow = cloneForCorrection(row, *correction, doctorID, version, publishedAt)
	}
	verificationURL := s.verificationURL(token)
	pdf, err := s.renderer.Render(renderRow, verificationURL)
	if err != nil || len(pdf) == 0 || len(pdf) > 10*1024*1024 {
		return nil, constant.ErrPrescriptionPDFGenerationFailed
	}
	objectPath := fmt.Sprintf("hospitals/%s/patients/%s/encounters/%s/prescriptions/%s/v%d-%s.pdf",
		row.HospitalID, row.PatientRecordID, row.EncounterID, row.ID, version, uuid.NewString())
	uploaded, err := s.storage.Upload(ctx, objectPath, "application/pdf", pdf)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	digest := sha256.Sum256(pdf)
	document := repository.DocumentInput{
		Bucket: uploaded.Bucket, ObjectPath: uploaded.ObjectPath,
		Filename: fmt.Sprintf("resep-%s-v%d.pdf", row.PrescriptionNumber, version),
		FileSize: int64(len(pdf)), SHA256: hex.EncodeToString(digest[:]),
	}
	snapshot := clinicalSnapshot(renderRow)
	if correction == nil {
		err = s.repo.Issue(ctx, row.ID, row.CurrentRevision.ID, doctorID, tokenHash, snapshot, document, publishedAt)
	} else {
		err = s.repo.Correct(ctx, row, doctorID, tokenHash, *correction, snapshot, document, publishedAt)
	}
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.storage.Delete(cleanupCtx, uploaded.ObjectPath)
		return nil, mapError(err)
	}
	return s.repo.GetByAppointment(ctx, row.AppointmentID)
}

func cloneForIssue(row *response.Prescription, doctorID string, now time.Time) *response.Prescription {
	clone := *row
	revision := *row.CurrentRevision
	revision.Status = "ISSUED"
	revision.AllergiesReviewed = true
	revision.IssuedBy = &doctorID
	revision.IssuedAt = &now
	clone.CurrentRevision = &revision
	return &clone
}

func clinicalSnapshot(row *response.Prescription) repository.ClinicalSnapshot {
	sipNumber := ""
	if row.DoctorSIPNumber != nil {
		sipNumber = strings.TrimSpace(*row.DoctorSIPNumber)
	}
	return repository.ClinicalSnapshot{
		PatientName: row.PatientName, PatientDateOfBirth: row.PatientDateOfBirth,
		PatientGender: row.PatientGender, PatientAllergies: row.PatientAllergies,
		HospitalName: row.HospitalName, HospitalAddress: row.HospitalAddress,
		HospitalPhone: row.HospitalPhone, DoctorName: row.DoctorName,
		DoctorSIPNumber: sipNumber,
	}
}

func applyAppointmentIdentity(row *response.Prescription, appointment *repository.AppointmentContext) {
	row.PatientName = appointment.PatientName
	row.PatientDateOfBirth = appointment.PatientDateOfBirth
	row.PatientGender = appointment.PatientGender
	row.PatientAllergies = appointment.PatientAllergies
	row.HospitalName = appointment.HospitalName
	row.HospitalAddress = appointment.HospitalAddress
	row.HospitalPhone = appointment.HospitalPhone
	row.DoctorName = appointment.DoctorName
	row.DoctorSIPNumber = appointment.DoctorSIPNumber
}

func currentRevisionMatches(row *response.Prescription, expectedRevisionID string) bool {
	return row.CurrentRevision != nil && row.CurrentRevision.ID == expectedRevisionID
}

func (s *Service) GetDoctorPrescription(ctx context.Context, doctorID, appointmentID string) (*response.Prescription, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	row, err := s.repo.GetByAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.logAccess(ctx, row, doctorID, "PRESCRIPTION_VIEWED"); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) GetHospitalPrescription(ctx context.Context, hospitalID, actorID, appointmentID string) (*response.Prescription, error) {
	appointment, err := s.authorizeHospital(ctx, hospitalID, appointmentID)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.GetByAppointment(ctx, appointment.AppointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if row.Status == "DRAFT" || row.Status == "NO_MEDICATION" {
		return nil, constant.ErrPrescriptionNotAvailable
	}
	if err := s.logAccess(ctx, row, actorID, "HOSPITAL_PRESCRIPTION_VIEWED"); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) ListPatientPrescriptions(ctx context.Context, patientID string) ([]response.PrescriptionSummary, error) {
	rows, err := s.repo.ListForPatient(ctx, patientID)
	if err != nil {
		return nil, mapError(err)
	}
	for _, row := range rows {
		if err := s.repo.LogAccess(ctx, row.ID, nil, patientID, "PATIENT_PRESCRIPTION_LISTED", s.now()); err != nil {
			return nil, constant.ErrInternalServerError
		}
	}
	return rows, nil
}

func (s *Service) GetPatientPrescription(ctx context.Context, patientID, prescriptionID string) (*response.Prescription, error) {
	if _, err := uuid.Parse(prescriptionID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetForPatient(ctx, prescriptionID, patientID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.logAccess(ctx, row, patientID, "PATIENT_PRESCRIPTION_VIEWED"); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) GetDoctorDocumentURL(ctx context.Context, doctorID, appointmentID string) (*response.PrescriptionDocumentURL, error) {
	row, err := s.GetDoctorPrescription(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	return s.documentURL(ctx, row.ID, doctorID)
}

func (s *Service) GetHospitalDocumentURL(ctx context.Context, hospitalID, actorID, appointmentID string) (*response.PrescriptionDocumentURL, error) {
	row, err := s.GetHospitalPrescription(ctx, hospitalID, actorID, appointmentID)
	if err != nil {
		return nil, err
	}
	return s.documentURL(ctx, row.ID, actorID)
}

func (s *Service) GetPatientDocumentURL(ctx context.Context, patientID, prescriptionID string) (*response.PrescriptionDocumentURL, error) {
	row, err := s.GetPatientPrescription(ctx, patientID, prescriptionID)
	if err != nil {
		return nil, err
	}
	return s.documentURL(ctx, row.ID, patientID)
}

func (s *Service) documentURL(ctx context.Context, prescriptionID, actorID string) (*response.PrescriptionDocumentURL, error) {
	document, err := s.repo.GetDocument(ctx, prescriptionID)
	if err != nil {
		return nil, mapError(err)
	}
	url, err := s.storage.CreateSignedURL(ctx, document.ObjectPath, s.signedURLTTL, filepath.Base(document.Filename))
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	if err := s.repo.LogAccess(ctx, prescriptionID, &document.RevisionID, actorID, "PRESCRIPTION_DOCUMENT_URL_CREATED", s.now()); err != nil {
		return nil, constant.ErrInternalServerError
	}
	return &response.PrescriptionDocumentURL{URL: url, ExpiresAt: s.now().Add(s.signedURLTTL)}, nil
}

func (s *Service) logAccess(ctx context.Context, row *response.Prescription, actorID, action string) error {
	var revisionID *string
	if row.CurrentRevision != nil {
		revisionID = &row.CurrentRevision.ID
	}
	if err := s.repo.LogAccess(ctx, row.ID, revisionID, actorID, action, s.now()); err != nil {
		return constant.ErrInternalServerError
	}
	return nil
}

func (s *Service) Verify(ctx context.Context, token string) (*response.PrescriptionVerification, error) {
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 128 {
		return nil, constant.ErrPrescriptionVerificationInvalid
	}
	digest := sha256.Sum256([]byte(token))
	row, err := s.repo.Verify(ctx, hex.EncodeToString(digest[:]))
	if err != nil {
		return nil, constant.ErrPrescriptionVerificationInvalid
	}
	return row, nil
}

func (s *Service) authorizeDoctor(ctx context.Context, doctorID, appointmentID string) (*repository.AppointmentContext, error) {
	if _, err := uuid.Parse(appointmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetAppointmentContext(ctx, appointmentID)
	if err != nil || row == nil || row.DoctorID != doctorID {
		return nil, constant.ErrPrescriptionNotFound
	}
	return row, nil
}

func (s *Service) authorizeHospital(ctx context.Context, hospitalID, appointmentID string) (*repository.AppointmentContext, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := uuid.Parse(appointmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetAppointmentContext(ctx, appointmentID)
	if err != nil || row == nil || row.HospitalID != hospitalID {
		return nil, constant.ErrPrescriptionNotFound
	}
	return row, nil
}

func (s *Service) normalizeDraft(ctx context.Context, hospitalID string, input request.PrescriptionDraftRequest) (request.PrescriptionDraftRequest, error) {
	input.GeneralNote = cleanOptional(input.GeneralNote)
	if len(input.Items) == 0 {
		return input, constant.ErrPrescriptionItemsRequired
	}
	for index := range input.Items {
		item := &input.Items[index]
		item.Type = strings.ToUpper(strings.TrimSpace(item.Type))
		item.MedicationName = strings.TrimSpace(item.MedicationName)
		item.DosageForm = strings.TrimSpace(item.DosageForm)
		item.Strength = strings.TrimSpace(item.Strength)
		item.DoseUnit = strings.TrimSpace(item.DoseUnit)
		item.Route = strings.TrimSpace(item.Route)
		item.DurationUnit = strings.ToUpper(strings.TrimSpace(item.DurationUnit))
		item.QuantityUnit = strings.TrimSpace(item.QuantityUnit)
		item.Directions = strings.TrimSpace(item.Directions)
		item.TimingInstructions = cleanOptional(item.TimingInstructions)
		item.MaxDailyDose = cleanOptional(item.MaxDailyDose)
		if item.ControlledSubstance {
			return input, constant.ErrControlledMedicationUnsupported
		}
		if item.FrequencyPerDay == nil && item.IntervalHours == nil && !item.AsNeeded {
			return input, constant.ErrPrescriptionScheduleRequired
		}
		if item.Type == "COMPOUND" {
			if item.MedicationID != nil || len(item.Components) == 0 {
				return input, constant.ErrPrescriptionCompoundInvalid
			}
		} else if item.Type == "NON_COMPOUND" {
			if len(item.Components) != 0 {
				return input, constant.ErrPrescriptionCompoundInvalid
			}
		} else {
			return input, constant.NewInvalidFieldValueError("items[].type", "NON_COMPOUND or COMPOUND", "NON_COMPOUND atau COMPOUND")
		}
		if item.MedicationID != nil {
			catalog, err := s.repo.GetMedication(ctx, hospitalID, *item.MedicationID)
			if err != nil {
				return input, mapError(err)
			}
			if catalog.ControlledCategory != "NONE" {
				return input, constant.ErrControlledMedicationUnsupported
			}
			item.MedicationName = catalog.GenericName
			if catalog.BrandName != nil && strings.TrimSpace(*catalog.BrandName) != "" {
				item.MedicationName += " (" + strings.TrimSpace(*catalog.BrandName) + ")"
			}
			item.DosageForm = catalog.DosageForm
			item.Strength = catalog.Strength
		}
		for componentIndex := range item.Components {
			component := &item.Components[componentIndex]
			component.MedicationName = strings.TrimSpace(component.MedicationName)
			component.DosageForm = strings.TrimSpace(component.DosageForm)
			component.Strength = strings.TrimSpace(component.Strength)
			component.Unit = strings.TrimSpace(component.Unit)
			if component.ControlledSubstance {
				return input, constant.ErrControlledMedicationUnsupported
			}
			if component.MedicationID != nil {
				catalog, err := s.repo.GetMedication(ctx, hospitalID, *component.MedicationID)
				if err != nil {
					return input, mapError(err)
				}
				if catalog.ControlledCategory != "NONE" {
					return input, constant.ErrControlledMedicationUnsupported
				}
				component.MedicationName = catalog.GenericName
				component.DosageForm = catalog.DosageForm
				component.Strength = catalog.Strength
			}
		}
	}
	return input, nil
}

func normalizeMedication(input request.MedicationCatalogRequest) request.MedicationCatalogRequest {
	input.Code = cleanOptional(input.Code)
	input.KFACode = cleanOptional(input.KFACode)
	input.GenericName = strings.TrimSpace(input.GenericName)
	input.BrandName = cleanOptional(input.BrandName)
	input.DosageForm = strings.TrimSpace(input.DosageForm)
	input.Strength = strings.TrimSpace(input.Strength)
	input.DefaultUnit = strings.TrimSpace(input.DefaultUnit)
	input.DefaultRoute = cleanOptional(input.DefaultRoute)
	return input
}

func validateMedication(input request.MedicationCatalogRequest) error {
	for field, value := range map[string]string{
		"generic_name": input.GenericName, "dosage_form": input.DosageForm,
		"strength": input.Strength, "default_unit": input.DefaultUnit,
	} {
		if value == "" {
			return constant.NewFieldRequiredError(field)
		}
	}
	return nil
}

func cloneForCorrection(row *response.Prescription, input request.CorrectPrescriptionRequest, doctorID string, version int, now time.Time) *response.Prescription {
	clone := *row
	items := make([]response.PrescriptionItem, 0, len(input.Items))
	for index, item := range input.Items {
		converted := response.PrescriptionItem{
			Order: index + 1, Type: item.Type, MedicationID: item.MedicationID,
			MedicationName: item.MedicationName, DosageForm: item.DosageForm,
			Strength: item.Strength, DoseAmount: item.DoseAmount, DoseUnit: item.DoseUnit,
			Route: item.Route, FrequencyPerDay: item.FrequencyPerDay,
			IntervalHours: item.IntervalHours, TimingInstructions: item.TimingInstructions,
			DurationValue: item.DurationValue, DurationUnit: item.DurationUnit,
			Quantity: item.Quantity, QuantityUnit: item.QuantityUnit, Directions: item.Directions,
			AsNeeded: item.AsNeeded, MaxDailyDose: item.MaxDailyDose,
			Components: make([]response.PrescriptionComponent, 0, len(item.Components)),
		}
		for _, component := range item.Components {
			converted.Components = append(converted.Components, response.PrescriptionComponent{
				MedicationID: component.MedicationID, MedicationName: component.MedicationName,
				DosageForm: component.DosageForm, Strength: component.Strength,
				Amount: component.Amount, Unit: component.Unit,
			})
		}
		items = append(items, converted)
	}
	clone.CurrentRevision = &response.PrescriptionRevision{
		Version: version, Status: "ISSUED", GeneralNote: input.GeneralNote,
		AllergiesReviewed: true, RepeatsAllowed: input.RepeatsAllowed,
		AuthoredBy: doctorID, IssuedBy: &doctorID, IssuedAt: &now,
		CorrectionReason: &input.CorrectionReason, Items: items,
	}
	return &clone
}

func newPrescriptionNumber(now time.Time) string {
	return fmt.Sprintf("RX-%s-%s", now.UTC().Format("20060102"), strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", "")))
}

func newVerificationToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(digest[:]), nil
}

func (s *Service) verificationURL(token string) string {
	if s.baseURL != "" {
		return s.baseURL + "/v1/prescriptions/verify/" + token
	}
	return "medikaone://prescriptions/verify?token=" + token
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return constant.ErrPrescriptionNotFound
	case errors.Is(err, repository.ErrMedicationNotFound):
		return constant.ErrMedicationCatalogNotFound
	case errors.Is(err, repository.ErrInvalidState):
		return constant.ErrPrescriptionInvalidState
	case errors.Is(err, repository.ErrConcurrentUpdate):
		return constant.ErrPrescriptionConcurrentUpdate
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return constant.ErrConflict
	default:
		return constant.ErrInternalServerError
	}
}

func mapMedicationError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return constant.ErrMedicationCatalogDuplicate
	}
	return mapError(err)
}
