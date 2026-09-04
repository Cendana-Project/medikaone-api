package examination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/examination"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type Repository interface {
	GetAppointmentContext(context.Context, string) (*repository.AppointmentContext, error)
	EnsureEncounter(context.Context, string, time.Time) (string, error)
	GetEncounter(context.Context, string, string, bool, time.Time) (*response.MedicalEncounter, error)
	ListHistoryForAppointment(context.Context, string, string, time.Time) ([]response.MedicalEncounterSummary, error)
	GetHistoryEncounterForAppointment(context.Context, string, string, string, time.Time) (*response.MedicalEncounter, error)
	ListHistoryForPatient(context.Context, string, string, time.Time) ([]response.MedicalEncounterSummary, error)
	GetPatientEncounter(context.Context, string, string, string, time.Time) (*response.MedicalEncounter, error)
	SaveVitalDraft(context.Context, string, string, request.VitalSignsRequest, time.Time) error
	FinalizeVitals(context.Context, string, string, time.Time) error
	CorrectVitals(context.Context, string, string, request.CorrectVitalSignsRequest, time.Time) error
	SaveConsultationDraft(context.Context, string, string, request.ConsultationNoteRequest, time.Time) error
	CompleteConsultation(context.Context, string, string, time.Time) error
	CorrectConsultation(context.Context, string, string, request.CorrectConsultationNoteRequest, time.Time) error
	AddAttachment(context.Context, repository.AttachmentInput) (*response.MedicalAttachment, error)
	GetAttachment(context.Context, string) (*repository.AttachmentRecord, error)
	LogAttachmentAccess(context.Context, *repository.AttachmentRecord, string, time.Time) error
}

type Service struct {
	repo         Repository
	storage      storageclient.Client
	maxFileSize  int64
	signedURLTTL time.Duration
	now          func() time.Time
}

type UploadedFile struct {
	Filename string
	MIMEType string
	Content  []byte
}

func NewService(repo Repository, storage storageclient.Client, maxFileSize int64, signedURLTTL time.Duration) *Service {
	if maxFileSize <= 0 || maxFileSize > 10*1024*1024 {
		maxFileSize = 10 * 1024 * 1024
	}
	if signedURLTTL <= 0 {
		signedURLTTL = 5 * time.Minute
	}
	return &Service{repo: repo, storage: storage, maxFileSize: maxFileSize, signedURLTTL: signedURLTTL, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) GetHospitalExamination(ctx context.Context, hospitalID, actorID, appointmentID string) (*response.MedicalEncounter, error) {
	appointment, err := s.authorizeHospital(ctx, hospitalID, appointmentID)
	if err != nil {
		return nil, err
	}
	if err := validateViewableState(appointment.Status); err != nil {
		return nil, err
	}
	if _, err := s.repo.EnsureEncounter(ctx, appointmentID, s.now()); err != nil {
		return nil, mapError(err)
	}
	row, err := s.repo.GetEncounter(ctx, appointmentID, actorID, true, s.now())
	hideInternalNote(row)
	return row, mapError(err)
}

func (s *Service) GetDoctorExamination(ctx context.Context, doctorID, appointmentID string) (*response.MedicalEncounter, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if err := validateViewableState(appointment.Status); err != nil {
		return nil, err
	}
	if _, err := s.repo.EnsureEncounter(ctx, appointmentID, s.now()); err != nil {
		return nil, mapError(err)
	}
	row, err := s.repo.GetEncounter(ctx, appointmentID, doctorID, true, s.now())
	return row, mapError(err)
}

func (s *Service) SaveVitalDraft(ctx context.Context, hospitalID, actorID, appointmentID string, input request.VitalSignsRequest) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	input = normalizeVitals(input)
	if err := validateBloodPressure(input.SystolicMMHG, input.DiastolicMMHG); err != nil {
		return nil, err
	}
	if err := s.repo.SaveVitalDraft(ctx, appointmentID, actorID, input, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetEncounter(ctx, appointmentID, actorID, true, s.now())
}

func (s *Service) FinalizeVitals(ctx context.Context, hospitalID, actorID, appointmentID string) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	row, err := s.repo.GetEncounter(ctx, appointmentID, actorID, true, s.now())
	if err != nil {
		return nil, mapError(err)
	}
	if row.Vitals == nil {
		return nil, constant.ErrVitalsDraftNotFound
	}
	if !hasVitalContent(*row.Vitals) {
		return nil, constant.ErrVitalsContentRequired
	}
	if err := s.repo.FinalizeVitals(ctx, appointmentID, actorID, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetEncounter(ctx, appointmentID, actorID, true, s.now())
}

func (s *Service) CorrectVitals(ctx context.Context, hospitalID, actorID, appointmentID string, input request.CorrectVitalSignsRequest) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	input.VitalSignsRequest = normalizeVitals(input.VitalSignsRequest)
	input.CorrectionReason = strings.TrimSpace(input.CorrectionReason)
	if input.CorrectionReason == "" {
		return nil, constant.NewFieldRequiredError("correction_reason")
	}
	if !hasVitalRequestContent(input.VitalSignsRequest) {
		return nil, constant.ErrVitalsContentRequired
	}
	if err := validateBloodPressure(input.SystolicMMHG, input.DiastolicMMHG); err != nil {
		return nil, err
	}
	if err := s.repo.CorrectVitals(ctx, appointmentID, actorID, input, s.now()); err != nil {
		return nil, mapError(err)
	}
	row, err := s.repo.GetEncounter(ctx, appointmentID, actorID, true, s.now())
	hideInternalNote(row)
	return row, mapError(err)
}

func (s *Service) SaveDoctorConsultationDraft(ctx context.Context, doctorID, appointmentID string, input request.ConsultationNoteRequest) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	input, err := normalizeConsultation(input, false)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveConsultationDraft(ctx, appointmentID, doctorID, input, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetEncounter(ctx, appointmentID, doctorID, true, s.now())
}

func (s *Service) CompleteDoctorExamination(ctx context.Context, doctorID, appointmentID string) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	row, err := s.repo.GetEncounter(ctx, appointmentID, doctorID, true, s.now())
	if err != nil {
		return nil, mapError(err)
	}
	if err := validateFinalConsultation(row.Consultation); err != nil {
		return nil, err
	}
	if err := s.repo.CompleteConsultation(ctx, appointmentID, doctorID, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetEncounter(ctx, appointmentID, doctorID, false, s.now())
}

func (s *Service) CorrectDoctorConsultation(ctx context.Context, doctorID, appointmentID string, input request.CorrectConsultationNoteRequest) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	return s.correctConsultation(ctx, doctorID, appointmentID, input)
}

func (s *Service) CorrectHospitalConsultation(ctx context.Context, hospitalID, actorID, appointmentID string, input request.CorrectHospitalConsultationNoteRequest) (*response.MedicalEncounter, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	current, err := s.repo.GetEncounter(ctx, appointmentID, actorID, false, s.now())
	if err != nil {
		return nil, mapError(err)
	}
	correction := request.CorrectConsultationNoteRequest{
		ConsultationNoteRequest: request.ConsultationNoteRequest{
			Subjective: input.Subjective, Objective: input.Objective,
			Assessment: input.Assessment, Plan: input.Plan, Diagnoses: input.Diagnoses,
		},
		CorrectionReason: input.CorrectionReason,
	}
	if current.Consultation != nil {
		correction.InternalNote = current.Consultation.InternalNote
	}
	row, err := s.correctConsultation(ctx, actorID, appointmentID, correction)
	hideInternalNote(row)
	return row, err
}

func (s *Service) correctConsultation(ctx context.Context, actorID, appointmentID string, input request.CorrectConsultationNoteRequest) (*response.MedicalEncounter, error) {
	normalized, err := normalizeConsultation(input.ConsultationNoteRequest, true)
	if err != nil {
		return nil, err
	}
	input.ConsultationNoteRequest = normalized
	input.CorrectionReason = strings.TrimSpace(input.CorrectionReason)
	if input.CorrectionReason == "" {
		return nil, constant.NewFieldRequiredError("correction_reason")
	}
	if err := s.repo.CorrectConsultation(ctx, appointmentID, actorID, input, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.repo.GetEncounter(ctx, appointmentID, actorID, false, s.now())
}

func (s *Service) ListDoctorMedicalHistory(ctx context.Context, doctorID, appointmentID string) ([]response.MedicalEncounterSummary, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(appointment.ConsentVersion) == "" || appointment.ConsentedAt.IsZero() {
		return nil, constant.ErrAppointmentConsentRequired
	}
	rows, err := s.repo.ListHistoryForAppointment(ctx, appointmentID, doctorID, s.now())
	return rows, mapError(err)
}

func (s *Service) GetDoctorHistoricalMedicalRecord(ctx context.Context, doctorID, appointmentID, encounterID string) (*response.MedicalEncounter, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(appointment.ConsentVersion) == "" || appointment.ConsentedAt.IsZero() {
		return nil, constant.ErrAppointmentConsentRequired
	}
	if _, err := uuid.Parse(encounterID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetHistoryEncounterForAppointment(ctx, appointmentID, encounterID, doctorID, s.now())
	hideInternalNote(row)
	return row, mapError(err)
}

func (s *Service) ListPatientMedicalHistory(ctx context.Context, patientUserID string) ([]response.MedicalEncounterSummary, error) {
	rows, err := s.repo.ListHistoryForPatient(ctx, patientUserID, patientUserID, s.now())
	return rows, mapError(err)
}

func (s *Service) GetPatientMedicalRecord(ctx context.Context, patientUserID, encounterID string) (*response.MedicalEncounter, error) {
	if _, err := uuid.Parse(encounterID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetPatientEncounter(ctx, encounterID, patientUserID, patientUserID, s.now())
	hideInternalNote(row)
	return row, mapError(err)
}

func (s *Service) UploadHospitalAttachment(ctx context.Context, hospitalID, actorID, appointmentID, documentType string, note *string, file UploadedFile) (*response.MedicalAttachment, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	return s.uploadAttachment(ctx, actorID, appointmentID, documentType, note, file)
}

func (s *Service) UploadDoctorAttachment(ctx context.Context, doctorID, appointmentID, documentType string, note *string, file UploadedFile) (*response.MedicalAttachment, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	return s.uploadAttachment(ctx, doctorID, appointmentID, documentType, note, file)
}

func (s *Service) uploadAttachment(ctx context.Context, actorID, appointmentID, documentType string, note *string, file UploadedFile) (*response.MedicalAttachment, error) {
	documentType = strings.ToUpper(strings.TrimSpace(documentType))
	if !validDocumentType(documentType) {
		return nil, constant.NewInvalidFieldValueError("document_type", "LAB_RESULT, IMAGING, CLINICAL_DOCUMENT, or OTHER", "LAB_RESULT, IMAGING, CLINICAL_DOCUMENT, atau OTHER")
	}
	file, err := s.validateAttachment(file)
	if err != nil {
		return nil, err
	}
	if note != nil {
		value := strings.TrimSpace(*note)
		if len([]rune(value)) > 1000 {
			return nil, constant.NewInvalidFieldLengthError("note", "at most 1000 characters", "maksimal 1000 karakter")
		}
		note = &value
	}
	appointment, err := s.repo.GetAppointmentContext(ctx, appointmentID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := validateViewableState(appointment.Status); err != nil {
		return nil, err
	}
	encounterID, err := s.repo.EnsureEncounter(ctx, appointmentID, s.now())
	if err != nil {
		return nil, mapError(err)
	}
	extension := strings.ToLower(filepath.Ext(file.Filename))
	objectPath := fmt.Sprintf("hospitals/%s/patients/%s/encounters/%s/%s%s", appointment.HospitalID, appointment.PatientRecordID, encounterID, uuid.NewString(), extension)
	uploaded, err := s.storage.Upload(ctx, objectPath, file.MIMEType, file.Content)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	digest := sha256.Sum256(file.Content)
	out, err := s.repo.AddAttachment(ctx, repository.AttachmentInput{
		AppointmentID: appointmentID, ActorID: actorID, DocumentType: documentType,
		Filename: file.Filename, MIMEType: file.MIMEType, Bucket: uploaded.Bucket,
		ObjectPath: uploaded.ObjectPath, FileSize: uploaded.FileSize,
		SHA256: hex.EncodeToString(digest[:]), Note: note, Now: s.now(),
	})
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.storage.Delete(cleanupCtx, uploaded.ObjectPath)
		return nil, mapError(err)
	}
	return out, nil
}

func (s *Service) GetHospitalAttachmentURL(ctx context.Context, hospitalID, actorID, appointmentID, attachmentID string) (*response.MedicalAttachmentURL, error) {
	if _, err := s.authorizeHospital(ctx, hospitalID, appointmentID); err != nil {
		return nil, err
	}
	return s.getAttachmentURL(ctx, actorID, attachmentID, func(row *repository.AttachmentRecord) bool {
		return row.HospitalID == hospitalID && row.AppointmentID == appointmentID
	})
}

func (s *Service) GetDoctorAttachmentURL(ctx context.Context, doctorID, appointmentID, attachmentID string) (*response.MedicalAttachmentURL, error) {
	if _, err := s.authorizeDoctor(ctx, doctorID, appointmentID); err != nil {
		return nil, err
	}
	return s.getAttachmentURL(ctx, doctorID, attachmentID, func(row *repository.AttachmentRecord) bool {
		return row.DoctorID == doctorID && row.AppointmentID == appointmentID
	})
}

func (s *Service) GetDoctorHistoricalAttachmentURL(ctx context.Context, doctorID, appointmentID, encounterID, attachmentID string) (*response.MedicalAttachmentURL, error) {
	appointment, err := s.authorizeDoctor(ctx, doctorID, appointmentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(appointment.ConsentVersion) == "" || appointment.ConsentedAt.IsZero() {
		return nil, constant.ErrAppointmentConsentRequired
	}
	if _, err := uuid.Parse(encounterID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := s.repo.GetHistoryEncounterForAppointment(ctx, appointmentID, encounterID, doctorID, s.now()); err != nil {
		return nil, mapError(err)
	}
	return s.getAttachmentURL(ctx, doctorID, attachmentID, func(row *repository.AttachmentRecord) bool {
		return row.EncounterID == encounterID
	})
}

func (s *Service) GetPatientAttachmentURL(ctx context.Context, patientID, attachmentID string) (*response.MedicalAttachmentURL, error) {
	return s.getAttachmentURL(ctx, patientID, attachmentID, func(row *repository.AttachmentRecord) bool {
		return row.PatientUserID != nil && *row.PatientUserID == patientID && row.EncounterStatus == "COMPLETED"
	})
}

func (s *Service) getAttachmentURL(ctx context.Context, actorID, attachmentID string, allowed func(*repository.AttachmentRecord) bool) (*response.MedicalAttachmentURL, error) {
	if _, err := uuid.Parse(attachmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetAttachment(ctx, attachmentID)
	if err != nil || row == nil || !allowed(row) {
		return nil, constant.ErrMedicalAttachmentNotFound
	}
	url, err := s.storage.CreateSignedURL(ctx, row.ObjectPath, s.signedURLTTL, row.Filename)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	if err := s.repo.LogAttachmentAccess(ctx, row, actorID, s.now()); err != nil {
		return nil, constant.ErrInternalServerError
	}
	return &response.MedicalAttachmentURL{URL: url, ExpiresAt: s.now().Add(s.signedURLTTL)}, nil
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
		return nil, constant.ErrMedicalEncounterNotFound
	}
	return row, nil
}

func (s *Service) authorizeDoctor(ctx context.Context, doctorID, appointmentID string) (*repository.AppointmentContext, error) {
	if _, err := uuid.Parse(appointmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetAppointmentContext(ctx, appointmentID)
	if err != nil || row == nil || row.DoctorID != doctorID {
		return nil, constant.ErrMedicalEncounterNotFound
	}
	return row, nil
}

func validateViewableState(status string) error {
	switch status {
	case "WAITING_VITALS", "WAITING_DOCTOR", "IN_CONSULTATION", "COMPLETED":
		return nil
	default:
		return constant.ErrExaminationInvalidState
	}
}

func normalizeVitals(input request.VitalSignsRequest) request.VitalSignsRequest {
	input.NurseNote = cleanOptional(input.NurseNote)
	input.SkippedReason = cleanOptional(input.SkippedReason)
	return input
}

func validateBloodPressure(systolic, diastolic *int) error {
	if systolic != nil && diastolic != nil && *systolic <= *diastolic {
		return constant.ErrBloodPressureInvalid
	}
	return nil
}

func hasVitalRequestContent(v request.VitalSignsRequest) bool {
	return v.HeightCM != nil || v.WeightKG != nil || v.TemperatureC != nil || v.SystolicMMHG != nil ||
		v.DiastolicMMHG != nil || v.HeartRateBPM != nil || v.RespiratoryRateBPM != nil ||
		v.OxygenSaturationPercent != nil || (v.SkippedReason != nil && *v.SkippedReason != "")
}

func hasVitalContent(v response.VitalSigns) bool {
	return v.HeightCM != nil || v.WeightKG != nil || v.TemperatureC != nil || v.SystolicMMHG != nil ||
		v.DiastolicMMHG != nil || v.HeartRateBPM != nil || v.RespiratoryRateBPM != nil ||
		v.OxygenSaturationPercent != nil || (v.SkippedReason != nil && strings.TrimSpace(*v.SkippedReason) != "")
}

func normalizeConsultation(input request.ConsultationNoteRequest, final bool) (request.ConsultationNoteRequest, error) {
	input.Subjective = cleanOptional(input.Subjective)
	input.Objective = cleanOptional(input.Objective)
	input.Assessment = cleanOptional(input.Assessment)
	input.Plan = cleanOptional(input.Plan)
	input.InternalNote = cleanOptional(input.InternalNote)
	primary := 0
	for index := range input.Diagnoses {
		input.Diagnoses[index].Type = strings.ToUpper(strings.TrimSpace(input.Diagnoses[index].Type))
		input.Diagnoses[index].Status = strings.ToUpper(strings.TrimSpace(input.Diagnoses[index].Status))
		input.Diagnoses[index].Name = strings.TrimSpace(input.Diagnoses[index].Name)
		input.Diagnoses[index].ICD10 = cleanOptional(input.Diagnoses[index].ICD10)
		input.Diagnoses[index].Note = cleanOptional(input.Diagnoses[index].Note)
		if input.Diagnoses[index].Name == "" {
			return input, constant.NewFieldRequiredError("diagnoses[].name")
		}
		if input.Diagnoses[index].Type != "PRIMARY" && input.Diagnoses[index].Type != "SECONDARY" {
			return input, constant.NewInvalidFieldValueError("diagnoses[].type", "PRIMARY or SECONDARY", "PRIMARY atau SECONDARY")
		}
		if input.Diagnoses[index].Status != "SUSPECTED" && input.Diagnoses[index].Status != "CONFIRMED" && input.Diagnoses[index].Status != "RULED_OUT" {
			return input, constant.NewInvalidFieldValueError("diagnoses[].status", "SUSPECTED, CONFIRMED, or RULED_OUT", "SUSPECTED, CONFIRMED, atau RULED_OUT")
		}
		if input.Diagnoses[index].Type == "PRIMARY" {
			primary++
		}
	}
	if primary > 1 {
		return input, constant.ErrDiagnosisRequired
	}
	if final {
		note := &response.ConsultationNote{Subjective: input.Subjective, Objective: input.Objective, Assessment: input.Assessment, Plan: input.Plan}
		for _, diagnosis := range input.Diagnoses {
			note.Diagnoses = append(note.Diagnoses, response.Diagnosis{Type: diagnosis.Type})
		}
		if err := validateFinalConsultation(note); err != nil {
			return input, err
		}
	}
	return input, nil
}

func validateFinalConsultation(note *response.ConsultationNote) error {
	if note == nil || empty(note.Subjective) || empty(note.Objective) || empty(note.Assessment) || empty(note.Plan) {
		return constant.ErrClinicalSOAPRequired
	}
	primary := 0
	for _, diagnosis := range note.Diagnoses {
		if diagnosis.Type == "PRIMARY" {
			primary++
		}
	}
	if len(note.Diagnoses) == 0 || primary != 1 {
		return constant.ErrDiagnosisRequired
	}
	return nil
}

func empty(value *string) bool { return value == nil || strings.TrimSpace(*value) == "" }

func hideInternalNote(row *response.MedicalEncounter) {
	if row != nil && row.Consultation != nil {
		row.Consultation.InternalNote = nil
	}
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	clean := strings.TrimSpace(*value)
	if clean == "" {
		return nil
	}
	return &clean
}

func validDocumentType(value string) bool {
	switch value {
	case "LAB_RESULT", "IMAGING", "CLINICAL_DOCUMENT", "OTHER":
		return true
	}
	return false
}

func (s *Service) validateAttachment(file UploadedFile) (UploadedFile, error) {
	file.Filename = strings.TrimSpace(filepath.Base(strings.ReplaceAll(file.Filename, "\\", "/")))
	if file.Filename == "" || file.Filename == "." || len([]rune(file.Filename)) > 255 || len(file.Content) == 0 || int64(len(file.Content)) > s.maxFileSize {
		return UploadedFile{}, constant.ErrMedicalAttachmentInvalid
	}
	detected := http.DetectContentType(file.Content)
	extension := strings.ToLower(filepath.Ext(file.Filename))
	if strings.HasPrefix(detected, "image/jpeg") && (extension == ".jpg" || extension == ".jpeg") {
		file.MIMEType = "image/jpeg"
	} else if strings.HasPrefix(detected, "image/png") && extension == ".png" {
		file.MIMEType = "image/png"
	} else if len(file.Content) >= 5 && string(file.Content[:5]) == "%PDF-" && extension == ".pdf" {
		file.MIMEType = "application/pdf"
	} else {
		return UploadedFile{}, constant.ErrMedicalAttachmentInvalid
	}
	return file, nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrAppointmentNotFound), errors.Is(err, repository.ErrEncounterNotFound):
		return constant.ErrMedicalEncounterNotFound
	case errors.Is(err, repository.ErrInvalidState):
		return constant.ErrExaminationInvalidState
	case errors.Is(err, repository.ErrVitalsDraftNotFound):
		return constant.ErrVitalsDraftNotFound
	case errors.Is(err, repository.ErrVitalsRevisionNotFound):
		return constant.ErrVitalsRevisionNotFound
	case errors.Is(err, repository.ErrConsultationDraftNotFound):
		return constant.ErrConsultationDraftNotFound
	case errors.Is(err, repository.ErrConsultationRevisionNotFound):
		return constant.ErrConsultationRevisionNotFound
	case errors.Is(err, repository.ErrAttachmentNotFound):
		return constant.ErrMedicalAttachmentNotFound
	default:
		return constant.ErrInternalServerError
	}
}
