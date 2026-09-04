package examination

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/examination"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type fakeRepository struct {
	appointment *repository.AppointmentContext
	encounter   *response.MedicalEncounter
	completed   bool
	vitalSaved  bool
	correction  *request.CorrectConsultationNoteRequest
}

func (f *fakeRepository) GetAppointmentContext(context.Context, string) (*repository.AppointmentContext, error) {
	if f.appointment == nil {
		return nil, repository.ErrAppointmentNotFound
	}
	return f.appointment, nil
}
func (f *fakeRepository) EnsureEncounter(context.Context, string, time.Time) (string, error) {
	return uuid.NewString(), nil
}
func (f *fakeRepository) GetEncounter(context.Context, string, string, bool, time.Time) (*response.MedicalEncounter, error) {
	if f.encounter == nil {
		return nil, repository.ErrEncounterNotFound
	}
	return f.encounter, nil
}
func (f *fakeRepository) ListHistoryForAppointment(context.Context, string, string, time.Time) ([]response.MedicalEncounterSummary, error) {
	return []response.MedicalEncounterSummary{}, nil
}
func (f *fakeRepository) GetHistoryEncounterForAppointment(context.Context, string, string, string, time.Time) (*response.MedicalEncounter, error) {
	return f.encounter, nil
}
func (f *fakeRepository) ListHistoryForPatient(context.Context, string, string, time.Time) ([]response.MedicalEncounterSummary, error) {
	return []response.MedicalEncounterSummary{}, nil
}
func (f *fakeRepository) GetPatientEncounter(context.Context, string, string, string, time.Time) (*response.MedicalEncounter, error) {
	return f.encounter, nil
}
func (f *fakeRepository) SaveVitalDraft(context.Context, string, string, request.VitalSignsRequest, time.Time) error {
	f.vitalSaved = true
	return nil
}
func (f *fakeRepository) FinalizeVitals(context.Context, string, string, time.Time) error { return nil }
func (f *fakeRepository) CorrectVitals(context.Context, string, string, request.CorrectVitalSignsRequest, time.Time) error {
	return nil
}
func (f *fakeRepository) SaveConsultationDraft(context.Context, string, string, request.ConsultationNoteRequest, time.Time) error {
	return nil
}
func (f *fakeRepository) CompleteConsultation(context.Context, string, string, time.Time) error {
	f.completed = true
	return nil
}

func (f *fakeRepository) CorrectConsultation(_ context.Context, _, _ string, input request.CorrectConsultationNoteRequest, _ time.Time) error {
	f.correction = &input
	return nil
}
func (f *fakeRepository) AddAttachment(context.Context, repository.AttachmentInput) (*response.MedicalAttachment, error) {
	return &response.MedicalAttachment{}, nil
}
func (f *fakeRepository) GetAttachment(context.Context, string) (*repository.AttachmentRecord, error) {
	return nil, repository.ErrAttachmentNotFound
}
func (f *fakeRepository) LogAttachmentAccess(context.Context, *repository.AttachmentRecord, string, time.Time) error {
	return nil
}

type fakeStorage struct{}

func (*fakeStorage) Upload(context.Context, string, string, []byte) (*storageclient.UploadedObject, error) {
	return &storageclient.UploadedObject{}, nil
}
func (*fakeStorage) Delete(context.Context, string) error { return nil }
func (*fakeStorage) CreateSignedURL(context.Context, string, time.Duration, string) (string, error) {
	return "https://example.test/signed", nil
}

func appointmentFixture() *repository.AppointmentContext {
	return &repository.AppointmentContext{
		AppointmentID: uuid.NewString(), PatientRecordID: uuid.NewString(),
		HospitalID: uuid.NewString(), DoctorID: uuid.NewString(), DepartmentID: uuid.NewString(),
		Status: "IN_CONSULTATION", ConsentVersion: "v1", ConsentedAt: time.Now(),
	}
}

func TestSaveVitalDraftRejectsInvalidBloodPressure(t *testing.T) {
	appointment := appointmentFixture()
	appointment.Status = "WAITING_VITALS"
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	systolic, diastolic := 80, 90
	_, err := service.SaveVitalDraft(context.Background(), appointment.HospitalID, uuid.NewString(), appointment.AppointmentID, request.VitalSignsRequest{SystolicMMHG: &systolic, DiastolicMMHG: &diastolic})
	if !errors.Is(err, constant.ErrBloodPressureInvalid) || repo.vitalSaved {
		t.Fatalf("error=%v saved=%v", err, repo.vitalSaved)
	}
}

func TestFinalizeVitalsRequiresMeasurementOrSkipReason(t *testing.T) {
	appointment := appointmentFixture()
	appointment.Status = "WAITING_VITALS"
	repo := &fakeRepository{appointment: appointment, encounter: &response.MedicalEncounter{Vitals: &response.VitalSigns{Status: "DRAFT"}}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	_, err := service.FinalizeVitals(context.Background(), appointment.HospitalID, uuid.NewString(), appointment.AppointmentID)
	if !errors.Is(err, constant.ErrVitalsContentRequired) {
		t.Fatalf("error=%v", err)
	}
}

func TestCompleteExaminationRequiresSOAPAndPrimaryDiagnosis(t *testing.T) {
	appointment := appointmentFixture()
	repo := &fakeRepository{appointment: appointment, encounter: &response.MedicalEncounter{Consultation: &response.ConsultationNote{}}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	_, err := service.CompleteDoctorExamination(context.Background(), appointment.DoctorID, appointment.AppointmentID)
	if !errors.Is(err, constant.ErrClinicalSOAPRequired) || repo.completed {
		t.Fatalf("error=%v completed=%v", err, repo.completed)
	}

	value := "completed"
	repo.encounter.Consultation = &response.ConsultationNote{
		Subjective: &value, Objective: &value, Assessment: &value, Plan: &value,
		Diagnoses: []response.Diagnosis{{Type: "PRIMARY", Name: "Influenza"}},
	}
	_, err = service.CompleteDoctorExamination(context.Background(), appointment.DoctorID, appointment.AppointmentID)
	if err != nil || !repo.completed {
		t.Fatalf("error=%v completed=%v", err, repo.completed)
	}
}

func TestDoctorCannotReadAnotherDoctorsEncounter(t *testing.T) {
	appointment := appointmentFixture()
	repo := &fakeRepository{appointment: appointment, encounter: &response.MedicalEncounter{}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	_, err := service.GetDoctorExamination(context.Background(), uuid.NewString(), appointment.AppointmentID)
	if !errors.Is(err, constant.ErrMedicalEncounterNotFound) {
		t.Fatalf("error=%v", err)
	}
}

func TestPatientRecordHidesInternalDoctorNote(t *testing.T) {
	internal := "not patient-visible"
	repo := &fakeRepository{encounter: &response.MedicalEncounter{Consultation: &response.ConsultationNote{InternalNote: &internal}}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	row, err := service.GetPatientMedicalRecord(context.Background(), uuid.NewString(), uuid.NewString())
	if err != nil || row.Consultation.InternalNote != nil {
		t.Fatalf("row=%+v error=%v", row, err)
	}
}

func TestDoctorHistoricalRecordHidesPreviousInternalNote(t *testing.T) {
	appointment := appointmentFixture()
	internal := "previous doctor's private note"
	repo := &fakeRepository{
		appointment: appointment,
		encounter:   &response.MedicalEncounter{Consultation: &response.ConsultationNote{InternalNote: &internal}},
	}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)

	row, err := service.GetDoctorHistoricalMedicalRecord(context.Background(), appointment.DoctorID, appointment.AppointmentID, uuid.NewString())
	if err != nil || row.Consultation.InternalNote != nil {
		t.Fatalf("row=%+v error=%v", row, err)
	}
}

func TestHospitalCorrectionPreservesButHidesDoctorInternalNote(t *testing.T) {
	appointment := appointmentFixture()
	appointment.Status = "COMPLETED"
	internal := "doctor-only context"
	value := "completed"
	repo := &fakeRepository{
		appointment: appointment,
		encounter:   &response.MedicalEncounter{Consultation: &response.ConsultationNote{InternalNote: &internal}},
	}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)

	row, err := service.CorrectHospitalConsultation(context.Background(), appointment.HospitalID, uuid.NewString(), appointment.AppointmentID, request.CorrectHospitalConsultationNoteRequest{
		Subjective: &value, Objective: &value, Assessment: &value, Plan: &value,
		Diagnoses:        []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", Name: "Influenza"}},
		CorrectionReason: "Correction by hospital admin",
	})
	if err != nil || repo.correction == nil || repo.correction.InternalNote == nil || *repo.correction.InternalNote != internal {
		t.Fatalf("row=%+v correction=%+v error=%v", row, repo.correction, err)
	}
	if row.Consultation.InternalNote != nil {
		t.Fatal("hospital response exposed the doctor's internal note")
	}
}

func TestCorrectionsRejectWhitespaceReason(t *testing.T) {
	appointment := appointmentFixture()
	repo := &fakeRepository{appointment: appointment, encounter: &response.MedicalEncounter{}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)
	skipped := "alat sedang dikalibrasi"

	_, err := service.CorrectVitals(context.Background(), appointment.HospitalID, uuid.NewString(), appointment.AppointmentID, request.CorrectVitalSignsRequest{
		VitalSignsRequest: request.VitalSignsRequest{SkippedReason: &skipped},
		CorrectionReason:  "   ",
	})
	assertCustomErrorCode(t, err, "FIELD_REQUIRED")

	value := "completed"
	_, err = service.CorrectDoctorConsultation(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.CorrectConsultationNoteRequest{
		ConsultationNoteRequest: request.ConsultationNoteRequest{
			Subjective: &value, Objective: &value, Assessment: &value, Plan: &value,
			Diagnoses: []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", Name: "Influenza"}},
		},
		CorrectionReason: "\t",
	})
	assertCustomErrorCode(t, err, "FIELD_REQUIRED")
}

func TestConsultationRejectsWhitespaceDiagnosisName(t *testing.T) {
	appointment := appointmentFixture()
	repo := &fakeRepository{appointment: appointment, encounter: &response.MedicalEncounter{}}
	service := NewService(repo, &fakeStorage{}, 10*1024*1024, time.Minute)

	_, err := service.SaveDoctorConsultationDraft(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.ConsultationNoteRequest{
		Diagnoses: []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", Name: "  "}},
	})
	assertCustomErrorCode(t, err, "FIELD_REQUIRED")
}

func TestValidateAttachmentSanitizesBrowserPathAndChecksContent(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, 10*1024*1024, time.Minute)
	file, err := service.validateAttachment(UploadedFile{Filename: `C:\fakepath\lab-result.pdf`, Content: []byte("%PDF-test")})
	if err != nil || file.Filename != "lab-result.pdf" || file.MIMEType != "application/pdf" {
		t.Fatalf("file=%+v error=%v", file, err)
	}
	if _, err := service.validateAttachment(UploadedFile{Filename: "lab-result.png", Content: []byte("%PDF-test")}); !errors.Is(err, constant.ErrMedicalAttachmentInvalid) {
		t.Fatalf("mismatched extension error=%v", err)
	}
}

func assertCustomErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	custom, ok := err.(response.CustomError)
	if !ok || custom.Code != want {
		t.Fatalf("error=%#v code=%q want=%q", err, custom.Code, want)
	}
}
