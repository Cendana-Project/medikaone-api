package prescription

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/prescription"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type fakeRepository struct {
	Repository
	appointment     *repository.AppointmentContext
	hasDiagnosis    bool
	medication      *response.MedicationCatalog
	prescription    *response.Prescription
	savedDraft      *request.PrescriptionDraftRequest
	issueErr        error
	issuedTokenHash string
	issuedSnapshot  repository.ClinicalSnapshot
	issuedDocument  repository.DocumentInput
	issuedAt        time.Time
	noMedication    bool
}

func (f *fakeRepository) GetAppointmentContext(context.Context, string) (*repository.AppointmentContext, error) {
	return f.appointment, nil
}
func (f *fakeRepository) HasPrimaryDiagnosis(context.Context, string) (bool, error) {
	return f.hasDiagnosis, nil
}
func (f *fakeRepository) GetMedication(context.Context, string, string) (*response.MedicationCatalog, error) {
	if f.medication == nil {
		return nil, repository.ErrMedicationNotFound
	}
	return f.medication, nil
}
func (f *fakeRepository) SaveDraft(_ context.Context, _ *repository.AppointmentContext, _ string, _ string, input request.PrescriptionDraftRequest, _ time.Time) error {
	f.savedDraft = &input
	return nil
}
func (f *fakeRepository) MarkNoMedication(context.Context, *repository.AppointmentContext, string, string, string, time.Time) error {
	f.noMedication = true
	return nil
}
func (f *fakeRepository) GetByAppointment(context.Context, string) (*response.Prescription, error) {
	if f.prescription == nil {
		return nil, repository.ErrNotFound
	}
	return f.prescription, nil
}
func (f *fakeRepository) Issue(_ context.Context, _, _, _ string, tokenHash string, snapshot repository.ClinicalSnapshot, document repository.DocumentInput, issuedAt time.Time) error {
	f.issuedTokenHash = tokenHash
	f.issuedSnapshot = snapshot
	f.issuedDocument = document
	f.issuedAt = issuedAt
	if f.issueErr == nil {
		f.prescription.Status = "ISSUED"
	}
	return f.issueErr
}

type fakeStorage struct {
	uploadedPath string
	deletedPath  string
}

func (f *fakeStorage) Upload(_ context.Context, objectPath, _ string, content []byte) (*storageclient.UploadedObject, error) {
	if len(content) == 0 {
		return nil, errors.New("empty")
	}
	f.uploadedPath = objectPath
	return &storageclient.UploadedObject{Bucket: "medical-records", ObjectPath: objectPath, FileSize: int64(len(content))}, nil
}
func (f *fakeStorage) Delete(_ context.Context, objectPath string) error {
	f.deletedPath = objectPath
	return nil
}
func (f *fakeStorage) CreateSignedURL(context.Context, string, time.Duration, string) (string, error) {
	return "https://storage.example.test/signed", nil
}

type fakeRenderer struct{ content []byte }

func (f fakeRenderer) Render(*response.Prescription, string) ([]byte, error) { return f.content, nil }

func prescriptionFixture() (*repository.AppointmentContext, *response.Prescription) {
	doctorID := uuid.NewString()
	sipNumber := "SIP-TEST-001"
	appointment := &repository.AppointmentContext{
		AppointmentID: uuid.NewString(), EncounterID: uuid.NewString(),
		PatientRecordID: uuid.NewString(), PatientName: "Pasien Uji",
		PatientDateOfBirth: "1990-01-01", PatientGender: "L", HospitalID: uuid.NewString(),
		HospitalName: "RS MedikaOne",
		DoctorID:     doctorID, DoctorName: "Dokter Uji", DoctorSIPNumber: &sipNumber,
		AppointmentStatus: "IN_CONSULTATION",
	}
	prescription := &response.Prescription{
		ID: uuid.NewString(), EncounterID: appointment.EncounterID,
		AppointmentID: appointment.AppointmentID, PatientRecordID: appointment.PatientRecordID,
		HospitalID: appointment.HospitalID, DoctorID: doctorID, Status: "DRAFT",
		PrescriptionNumber: "RX-20260905-ABC12345", PatientName: "Pasien Uji",
		PatientDateOfBirth: "1990-01-01", PatientGender: "L",
		HospitalName: "RS MedikaOne", DoctorName: "Dokter Uji", DoctorSIPNumber: &sipNumber,
		CurrentRevision: &response.PrescriptionRevision{
			ID: uuid.NewString(), Version: 1, Status: "DRAFT", AuthoredBy: doctorID,
			Items: []response.PrescriptionItem{{MedicationName: "Paracetamol", Strength: "500 mg", DosageForm: "tablet"}},
		},
	}
	return appointment, prescription
}

func validDraft(medicationID string) request.PrescriptionDraftRequest {
	frequency := 3
	return request.PrescriptionDraftRequest{Items: []request.PrescriptionItemRequest{{
		Type: "NON_COMPOUND", MedicationID: &medicationID, MedicationName: "ignored",
		DosageForm: "ignored", Strength: "ignored", DoseAmount: 1, DoseUnit: "tablet",
		Route: "ORAL", FrequencyPerDay: &frequency, DurationValue: 3,
		DurationUnit: "DAY", Quantity: 9, QuantityUnit: "tablet",
		Directions: "Sesudah makan",
	}}}
}

func TestSaveDraftUsesHospitalCatalogueSnapshot(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	medicationID := uuid.NewString()
	brand := "Sanmol"
	repo := &fakeRepository{
		appointment: appointment, hasDiagnosis: true, prescription: prescription,
		medication: &response.MedicationCatalog{
			ID: medicationID, HospitalID: appointment.HospitalID, GenericName: "Paracetamol",
			BrandName: &brand, DosageForm: "tablet", Strength: "500 mg",
			ControlledCategory: "NONE", IsActive: true,
		},
	}
	service := NewService(repo, &fakeStorage{}, fakeRenderer{content: []byte("pdf")}, time.Minute, "https://api.example.test")
	_, err := service.SaveDraft(context.Background(), appointment.DoctorID, appointment.AppointmentID, validDraft(medicationID))
	if err != nil {
		t.Fatal(err)
	}
	if repo.savedDraft == nil || repo.savedDraft.Items[0].MedicationName != "Paracetamol (Sanmol)" || repo.savedDraft.Items[0].Strength != "500 mg" {
		t.Fatalf("catalogue snapshot not applied: %+v", repo.savedDraft)
	}
}

func TestSaveDraftRequiresPrimaryDiagnosis(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	repo := &fakeRepository{appointment: appointment, prescription: prescription}
	service := NewService(repo, &fakeStorage{}, fakeRenderer{content: []byte("pdf")}, time.Minute, "")
	_, err := service.SaveDraft(context.Background(), appointment.DoctorID, appointment.AppointmentID, validDraft(uuid.NewString()))
	if !errors.Is(err, constant.ErrPrescriptionPrimaryDiagnosisRequired) || repo.savedDraft != nil {
		t.Fatalf("error=%v saved=%+v", err, repo.savedDraft)
	}
}

func TestSaveDraftRequiresDoctorSIP(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	appointment.DoctorSIPNumber = nil
	repo := &fakeRepository{appointment: appointment, hasDiagnosis: true, prescription: prescription}
	service := NewService(repo, &fakeStorage{}, fakeRenderer{content: []byte("pdf")}, time.Minute, "")

	_, err := service.SaveDraft(context.Background(), appointment.DoctorID, appointment.AppointmentID, validDraft(uuid.NewString()))
	if !errors.Is(err, constant.ErrPrescriptionDoctorSIPRequired) || repo.savedDraft != nil {
		t.Fatalf("error=%v saved=%+v", err, repo.savedDraft)
	}
}

func TestControlledMedicationIsRejected(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	medicationID := uuid.NewString()
	repo := &fakeRepository{
		appointment: appointment, hasDiagnosis: true, prescription: prescription,
		medication: &response.MedicationCatalog{ID: medicationID, ControlledCategory: "NARCOTIC", IsActive: true},
	}
	service := NewService(repo, &fakeStorage{}, fakeRenderer{content: []byte("pdf")}, time.Minute, "")
	_, err := service.SaveDraft(context.Background(), appointment.DoctorID, appointment.AppointmentID, validDraft(medicationID))
	if !errors.Is(err, constant.ErrControlledMedicationUnsupported) {
		t.Fatalf("error=%v", err)
	}
}

func TestIssueUploadsPDFAndStoresOnlyTokenHash(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	repo := &fakeRepository{appointment: appointment, hasDiagnosis: true, prescription: prescription}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, fakeRenderer{content: []byte("%PDF-test")}, time.Minute, "https://api.example.test")
	publishedAt := time.Date(2026, 9, 5, 9, 10, 11, 0, time.UTC)
	service.now = func() time.Time { return publishedAt }

	result, err := service.Issue(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.IssuePrescriptionRequest{
		ExpectedRevisionID: prescription.CurrentRevision.ID, AllergiesReviewed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ISSUED" || len(repo.issuedTokenHash) != 64 || strings.Contains(objectStorage.uploadedPath, repo.issuedTokenHash) {
		t.Fatalf("result=%+v token_hash=%q path=%q", result, repo.issuedTokenHash, objectStorage.uploadedPath)
	}
	if repo.issuedDocument.Bucket != "medical-records" || !strings.HasSuffix(repo.issuedDocument.Filename, ".pdf") {
		t.Fatalf("document=%+v", repo.issuedDocument)
	}
	if repo.issuedSnapshot.PatientName != prescription.PatientName || repo.issuedSnapshot.DoctorSIPNumber != "SIP-TEST-001" {
		t.Fatalf("clinical_snapshot=%+v", repo.issuedSnapshot)
	}
	if !repo.issuedAt.Equal(publishedAt) {
		t.Fatalf("issued_at=%s want %s", repo.issuedAt, publishedAt)
	}
}

func TestIssueDeletesUploadedObjectWhenDatabasePublishFails(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	repo := &fakeRepository{
		appointment: appointment, hasDiagnosis: true, prescription: prescription,
		issueErr: repository.ErrConcurrentUpdate,
	}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, fakeRenderer{content: []byte("%PDF-test")}, time.Minute, "")
	_, err := service.Issue(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.IssuePrescriptionRequest{
		ExpectedRevisionID: prescription.CurrentRevision.ID, AllergiesReviewed: true,
	})
	if !errors.Is(err, constant.ErrPrescriptionConcurrentUpdate) || objectStorage.deletedPath == "" || objectStorage.deletedPath != objectStorage.uploadedPath {
		t.Fatalf("error=%v uploaded=%q deleted=%q", err, objectStorage.uploadedPath, objectStorage.deletedPath)
	}
}

func TestIssueRejectsStaleDraftBeforeGeneratingDocument(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	repo := &fakeRepository{appointment: appointment, hasDiagnosis: true, prescription: prescription}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, fakeRenderer{content: []byte("%PDF-test")}, time.Minute, "")

	_, err := service.Issue(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.IssuePrescriptionRequest{
		ExpectedRevisionID: uuid.NewString(), AllergiesReviewed: true,
	})
	if !errors.Is(err, constant.ErrPrescriptionConcurrentUpdate) || objectStorage.uploadedPath != "" {
		t.Fatalf("error=%v uploaded=%q", err, objectStorage.uploadedPath)
	}
}

func TestIssueRetryReturnsAlreadyIssuedRevision(t *testing.T) {
	appointment, prescription := prescriptionFixture()
	prescription.Status = "ISSUED"
	prescription.CurrentRevision.Status = "ISSUED"
	repo := &fakeRepository{appointment: appointment, prescription: prescription}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, fakeRenderer{content: []byte("%PDF-test")}, time.Minute, "")

	result, err := service.Issue(context.Background(), appointment.DoctorID, appointment.AppointmentID, request.IssuePrescriptionRequest{
		ExpectedRevisionID: prescription.CurrentRevision.ID, AllergiesReviewed: true,
	})
	if err != nil || result != prescription || objectStorage.uploadedPath != "" || repo.issuedTokenHash != "" {
		t.Fatalf("result=%+v error=%v uploaded=%q token_hash=%q", result, err, objectStorage.uploadedPath, repo.issuedTokenHash)
	}
}

func TestPDFRendererProducesPDFWithPrescriptionContent(t *testing.T) {
	_, prescription := prescriptionFixture()
	issuedAt := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	prescription.CurrentRevision.IssuedAt = &issuedAt
	prescription.CurrentRevision.Items[0].DoseAmount = 1
	prescription.CurrentRevision.Items[0].DoseUnit = "tablet"
	prescription.CurrentRevision.Items[0].Route = "ORAL"
	frequency := 3
	prescription.CurrentRevision.Items[0].FrequencyPerDay = &frequency
	prescription.CurrentRevision.Items[0].DurationValue = 3
	prescription.CurrentRevision.Items[0].DurationUnit = "DAY"
	prescription.CurrentRevision.Items[0].Quantity = 9
	prescription.CurrentRevision.Items[0].QuantityUnit = "tablet"
	prescription.CurrentRevision.Items[0].Directions = "Sesudah makan"

	pdf, err := NewPDFRenderer().Render(prescription, "https://api.example.test/v1/prescriptions/verify/token")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || len(pdf) < 1000 {
		t.Fatalf("invalid PDF output: prefix=%q size=%d", pdf[:min(8, len(pdf))], len(pdf))
	}
}
