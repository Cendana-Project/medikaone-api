package seeder

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	examinationrepo "github.com/Cendana-Project/medikaone-api/internal/repository/examination"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	prescriptionrepo "github.com/Cendana-Project/medikaone-api/internal/repository/prescription"
	examinationsvc "github.com/Cendana-Project/medikaone-api/internal/service/examination"
	hospitalsvc "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	prescriptionsvc "github.com/Cendana-Project/medikaone-api/internal/service/prescription"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
	"gorm.io/gorm"
)

// This helper requires the real migrated, base-seeded PostgreSQL database. Its
// own transaction rolls back all workflows, so existing base-suite assertions
// and later cases do not inherit immutable clinical records.
func runDemoClinicalIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	ctx := context.Background()
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	contracts := demoClinicalNewStorage(t, "doctor-contracts")
	medical := demoClinicalNewStorage(t, "medical-records")
	seed, err := newWorkflowSeedContext(ctx, tx, contracts, medical, time.Now().UTC(), "https://demo.example.test", location)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedDemoPractices(seed); err != nil {
		t.Fatal(err)
	}
	if err := seedDemoClinical(seed); err != nil {
		t.Fatal(err)
	}
	appointments := appointmentrepo.NewRepository(tx)
	examinations := examinationsvc.NewService(examinationrepo.NewRepository(tx), medical, 10*1024*1024, 5*time.Minute)
	prescriptions := prescriptionsvc.NewService(prescriptionrepo.NewRepository(tx), medical, prescriptionsvc.NewPDFRenderer(), 5*time.Minute, seed.BaseURL)
	reviews := hospitalsvc.NewService(nil, nil, hospitalrepo.NewRepository(tx))

	for alias, expected := range map[string]string{
		"confirmed": "CONFIRMED", "waiting_vitals": "WAITING_VITALS", "waiting_doctor": "WAITING_DOCTOR",
		"in_consultation": "IN_CONSULTATION", "completed_no_medication": "COMPLETED",
		"completed_prescription": "COMPLETED", "cancelled": "CANCELLED", "queue": "CONFIRMED",
	} {
		appointment, err := appointments.GetAppointment(ctx, seed.IDs["appointment."+alias])
		if err != nil || appointment == nil || appointment.Status != expected {
			t.Fatalf("scenario %s is not usable in %s: %#v %v", alias, expected, appointment, err)
		}
		if alias == "queue" && (appointment.BookingMode != "SESSION_QUEUE" || appointment.ScheduledEndAt.Sub(appointment.ScheduledStartAt) != 3*time.Hour || appointment.QueueNumber == "") {
			t.Fatalf("queue booking did not reserve its complete session: %#v", appointment)
		}
		if strings.HasPrefix(alias, "waiting_") || alias == "in_consultation" || strings.HasPrefix(alias, "completed_") {
			if appointment.AttendanceStatus != "PRESENT" || appointment.CheckedInAt == nil {
				t.Fatalf("scenario %s skipped real receptionist check-in: %#v", alias, appointment)
			}
		}
	}
	waiting, err := examinations.GetHospitalExamination(ctx, seed.Hospitals["jakarta"], seed.Users["nurse"], seed.IDs["appointment.waiting_doctor"])
	if err != nil || waiting == nil || waiting.Vitals == nil || waiting.Vitals.Status != "FINALIZED" || waiting.Vitals.FinalizedBy == nil || *waiting.Vitals.FinalizedBy != seed.Users["nurse"] {
		t.Fatalf("doctor queue lacks finalized nurse measurements: %#v %v", waiting, err)
	}
	inProgress, err := examinations.GetDoctorExamination(ctx, seed.Users["doctor1"], seed.IDs["appointment.in_consultation"])
	if err != nil || inProgress == nil || inProgress.Consultation == nil || inProgress.Consultation.Status != "DRAFT" || len(inProgress.Consultation.Diagnoses) != 1 {
		t.Fatalf("consultation lacks editable SOAP and diagnosis: %#v %v", inProgress, err)
	}
	draft, err := prescriptions.GetDoctorPrescription(ctx, seed.Users["doctor1"], seed.IDs["appointment.in_consultation"])
	if err != nil || draft == nil || draft.Status != "DRAFT" || draft.CurrentRevision == nil || len(draft.CurrentRevision.Items) != 1 || draft.CurrentRevision.Items[0].MedicationID == nil || *draft.CurrentRevision.Items[0].MedicationID != seed.IDs["medication.paracetamol"] {
		t.Fatalf("consultation lacks an editable catalog-backed prescription: %#v %v", draft, err)
	}

	var completedRecord *response.MedicalEncounter
	for _, branch := range []struct{ patient, alias string }{
		{"patient1", "completed_no_medication"}, {"patient2", "completed_prescription"},
	} {
		history, err := examinations.ListPatientMedicalHistory(ctx, seed.Users[branch.patient])
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, visit := range history {
			if visit.ID == seed.IDs["encounter."+branch.alias] && visit.PrimaryDiagnosis != nil && visit.PrimaryDiagnosis.Type == "PRIMARY" && visit.CompletedAt != nil {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s has no completed, diagnosed history for %s", branch.patient, branch.alias)
		}
		record, err := examinations.GetPatientMedicalRecord(ctx, seed.Users[branch.patient], seed.IDs["encounter."+branch.alias])
		if err != nil || record == nil || record.Status != "COMPLETED" || record.Vitals == nil || record.Vitals.Status != "FINALIZED" || record.Consultation == nil || record.Consultation.Status != "FINALIZED" || record.Consultation.FinalizedAt == nil || record.Consultation.Plan == nil {
			t.Fatalf("patient history is not clinically finalized: %#v %v", record, err)
		}
		if branch.patient == "patient1" {
			completedRecord = record
		}
	}
	if _, err := examinations.GetPatientMedicalRecord(ctx, seed.Users["patient3"], seed.IDs["encounter.completed_prescription"]); err == nil {
		t.Fatal("another patient can read the completed clinical record")
	}
	patientOnePrescriptions, err := prescriptions.ListPatientPrescriptions(ctx, seed.Users["patient1"])
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range patientOnePrescriptions {
		if item.ID == seed.IDs["prescription.no_medication"] || item.ID == seed.IDs["prescription.draft"] {
			t.Fatal("no-medication decision or draft leaked into issued patient prescription history")
		}
	}
	patientTwoPrescriptions, err := prescriptions.ListPatientPrescriptions(ctx, seed.Users["patient2"])
	if err != nil {
		t.Fatal(err)
	}
	issuedListed := false
	for _, item := range patientTwoPrescriptions {
		if item.ID == seed.IDs["prescription.issued"] && item.Status == "ISSUED" && item.ItemCount == 1 {
			issuedListed = true
		}
	}
	if !issuedListed {
		t.Fatal("completed issued prescription is absent from the patient API")
	}
	issued, err := prescriptions.GetPatientPrescription(ctx, seed.Users["patient2"], seed.IDs["prescription.issued"])
	if err != nil || issued == nil || issued.CurrentRevision == nil || issued.CurrentRevision.Document == nil || !issued.CurrentRevision.AllergiesReviewed || issued.CurrentRevision.DoctorSIPNumberSnapshot == "" {
		t.Fatalf("issued prescription lacks immutable clinical snapshots or PDF: %#v %v", issued, err)
	}
	if _, err := prescriptions.GetPatientPrescription(ctx, seed.Users["patient1"], issued.ID); err == nil {
		t.Fatal("another patient can retrieve the issued prescription")
	}
	download, err := prescriptions.GetPatientDocumentURL(ctx, seed.Users["patient2"], issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	httpResponse, err := http.Get(download.URL)
	if err != nil {
		t.Fatal(err)
	}
	pdf, readErr := io.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if readErr != nil || httpResponse.StatusCode != http.StatusOK || !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatalf("patient download is not a real stored PDF: status=%d error=%v", httpResponse.StatusCode, readErr)
	}
	digest := sha256.Sum256(pdf)
	if issued.CurrentRevision.Document.FileSize != int64(len(pdf)) || issued.CurrentRevision.Document.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("persisted prescription document metadata does not describe uploaded PDF bytes")
	}
	demoClinicalVerifyPDF(t, ctx, prescriptions, pdf, issued.PrescriptionNumber)
	review, err := reviews.GetOwnReview(ctx, seed.Hospitals["jakarta"], seed.Users["patient1"])
	if err != nil || review == nil || !review.VerifiedVisit || review.ID != seed.IDs["review.jakarta.patient1"] {
		t.Fatalf("completed patient cannot retrieve verified hospital review: %#v %v", review, err)
	}
	if _, err := reviews.PutReview(ctx, seed.Hospitals["jakarta"], seed.Users["patient3"], request.PutHospitalReviewRequest{Rating: 4, Comment: "Belum berkunjung"}); !errors.Is(err, constant.ErrHospitalReviewVisitRequired) {
		t.Fatalf("uncompleted patient is unexpectedly review-eligible: %v", err)
	}

	// Exercise real user changes before replay: operational progression, catalog
	// editing, a draft edit, a finalized correction, prescription cancellation,
	// and a review edit must all survive without another notification or upload.
	if err := appointments.Cancel(ctx, seed.IDs["appointment.confirmed"], seed.Users["patient1"], "Perubahan jadwal melalui pengujian", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := prescriptions.UpdateMedication(ctx, seed.Hospitals["jakarta"], seed.IDs["medication.paracetamol"], seed.Users["admin"], request.MedicationCatalogRequest{
		Code: new("LOCAL-PCT-EDITED"), GenericName: "Paracetamol formularium uji", DosageForm: "Tablet", Strength: "500 mg", DefaultUnit: "tablet", DefaultRoute: new("ORAL"), IsActive: new(false),
	}); err != nil {
		t.Fatal(err)
	}
	changedDraft := request.ConsultationNoteRequest{
		Subjective: new("Keluhan diperbarui oleh dokter saat pengujian"), Objective: inProgress.Consultation.Objective,
		Assessment: inProgress.Consultation.Assessment, Plan: inProgress.Consultation.Plan,
		Diagnoses: []request.DiagnosisRequest{{Type: "PRIMARY", Status: "SUSPECTED", Name: "Demam dalam evaluasi", ICD10: new("R50.9")}},
	}
	if _, err := examinations.SaveDoctorConsultationDraft(ctx, seed.Users["doctor1"], seed.IDs["appointment.in_consultation"], changedDraft); err != nil {
		t.Fatal(err)
	}
	correction := request.CorrectConsultationNoteRequest{
		ConsultationNoteRequest: request.ConsultationNoteRequest{
			Subjective: new("Pasien menambahkan riwayat konseling gizi sebelumnya"), Objective: completedRecord.Consultation.Objective,
			Assessment: completedRecord.Consultation.Assessment, Plan: completedRecord.Consultation.Plan,
			Diagnoses: []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", Name: "Konseling dan pengawasan diet", ICD10: new("Z71.3")}},
		}, CorrectionReason: "Melengkapi riwayat yang dikonfirmasi pasien",
	}
	corrected, err := examinations.CorrectDoctorConsultation(ctx, seed.Users["doctor1"], seed.IDs["appointment.completed_no_medication"], correction)
	if err != nil || corrected == nil || corrected.Consultation == nil || corrected.Consultation.SupersedesRevisionID == nil || *corrected.Consultation.SupersedesRevisionID != completedRecord.Consultation.ID {
		t.Fatalf("real finalized correction failed: %#v %v", corrected, err)
	}
	if _, err := prescriptions.Cancel(ctx, seed.Users["doctor1"], seed.IDs["appointment.completed_prescription"], request.CancelPrescriptionRequest{
		ExpectedRevisionID: issued.CurrentRevision.ID, Reason: "Dibatalkan dokter setelah peninjauan ulang",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reviews.PutReview(ctx, seed.Hospitals["jakarta"], seed.Users["patient1"], request.PutHospitalReviewRequest{Rating: 4, Comment: "Ulasan diperbarui oleh pasien"}); err != nil {
		t.Fatal(err)
	}
	before := demoClinicalSnapshot(t, tx)
	contractUploads, medicalUploads := contracts.uploads, medical.uploads
	replayed, err := newWorkflowSeedContext(ctx, tx, contracts, medical, seed.Now.AddDate(0, 0, 7), seed.BaseURL, location)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedDemoPractices(replayed); err != nil {
		t.Fatal(err)
	}
	if err := seedDemoClinical(replayed); err != nil {
		t.Fatal(err)
	}
	if !replayed.Anchor.Equal(seed.Anchor) {
		t.Fatal("replay moved the original scenario anchor")
	}
	for alias, id := range seed.IDs {
		if replayed.IDs[alias] != id {
			t.Fatalf("replay changed alias %s from %s to %s", alias, id, replayed.IDs[alias])
		}
	}
	after := demoClinicalSnapshot(t, tx)
	for table, snapshot := range before {
		if after[table] != snapshot {
			t.Fatalf("replay changed persisted %s rows or duplicated effects", table)
		}
	}
	if contracts.uploads != contractUploads || medical.uploads != medicalUploads {
		t.Fatal("replay regenerated or reuploaded an existing PDF")
	}
	if err := reviews.DeleteOwnReview(ctx, seed.Hospitals["jakarta"], seed.Users["patient1"]); err != nil {
		t.Fatal(err)
	}
	if err := seedDemoClinical(replayed); err != nil {
		t.Fatal(err)
	}
	if _, err := reviews.GetOwnReview(ctx, seed.Hospitals["jakarta"], seed.Users["patient1"]); !errors.Is(err, constant.ErrHospitalReviewNotFound) {
		t.Fatalf("replay revived a review deleted by the patient: %v", err)
	}

	// A stable idempotency key alone is not ownership: reject a changed tuple
	// instead of adopting it. This corruption is local to the rollback fixture.
	if err := tx.Exec(`UPDATE appointments SET doctor_id = ? WHERE id = ?`, seed.Users["doctor2"], seed.IDs["appointment.confirmed"]).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedDemoClinical(replayed); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("seed adopted a colliding appointment key: %v", err)
	}
	if err := tx.Exec(`UPDATE hospital_medications SET created_by = ? WHERE id = ?`, seed.Users["superadmin"], seed.IDs["medication.paracetamol"]).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedDemoClinical(replayed); err == nil || !strings.Contains(err.Error(), "unowned catalog entry") {
		t.Fatalf("seed adopted a colliding catalog identity: %v", err)
	}
}

func demoClinicalSnapshot(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	snapshots := make(map[string]string)
	for _, table := range []string{
		"appointments", "appointment_status_events", "appointment_reminders", "appointment_daily_counters", "notifications",
		"patient_records", "patient_record_events", "medical_encounters", "vital_sign_revisions", "consultation_note_revisions",
		"encounter_diagnoses", "medical_record_audit_events", "hospital_medications", "prescriptions", "prescription_revisions",
		"prescription_items", "prescription_documents", "prescription_audit_events", "hospital_reviews",
	} {
		var snapshot string
		if err := db.Raw(`SELECT COALESCE(jsonb_agg(value ORDER BY value::text), '[]'::jsonb)::text
			FROM (SELECT to_jsonb(fixture) AS value FROM ` + table + ` fixture) graph`).Scan(&snapshot).Error; err != nil {
			t.Fatal(err)
		}
		snapshots[table] = snapshot
	}
	return snapshots
}

// Recover the public verification token from the real PDF's text stream, not
// from database hashes or a replacement renderer. This exercises the same
// rendered token a patient scans or copies from their downloaded prescription.
func demoClinicalVerifyPDF(t *testing.T, ctx context.Context, service *prescriptionsvc.Service, pdf []byte, number string) {
	t.Helper()
	pattern := regexp.MustCompile(`[A-Za-z0-9_-]{43}`)
	remaining := pdf
	for {
		start := bytes.Index(remaining, []byte("stream\n"))
		if start < 0 {
			break
		}
		remaining = remaining[start+len("stream\n"):]
		end := bytes.Index(remaining, []byte("endstream"))
		if end < 0 {
			break
		}
		reader, err := zlib.NewReader(bytes.NewReader(remaining[:end]))
		if err == nil {
			content, readErr := io.ReadAll(reader)
			reader.Close()
			if readErr == nil {
				for _, token := range pattern.FindAllString(string(content), -1) {
					verified, err := service.Verify(ctx, token)
					if err == nil && verified.Valid && verified.Status == "ISSUED" && verified.PrescriptionNumber == number && len(verified.Items) == 1 {
						return
					}
				}
			}
		}
		remaining = remaining[end+len("endstream"):]
	}
	t.Fatal("downloaded real PDF did not contain a working public verification token")
}

type demoClinicalStorage struct {
	mu      sync.RWMutex
	bucket  string
	objects map[string][]byte
	uploads int
	server  *httptest.Server
}

func demoClinicalNewStorage(t *testing.T, bucket string) *demoClinicalStorage {
	t.Helper()
	store := &demoClinicalStorage{bucket: bucket, objects: make(map[string][]byte)}
	store.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.mu.RLock()
		content, exists := store.objects[strings.TrimPrefix(r.URL.Path, "/files/")]
		store.mu.RUnlock()
		if !exists {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(content)
	}))
	t.Cleanup(store.server.Close)
	return store
}

func (s *demoClinicalStorage) Upload(_ context.Context, path, mime string, content []byte) (*storage.UploadedObject, error) {
	if mime != "application/pdf" || !bytes.HasPrefix(content, []byte("%PDF-")) {
		return nil, fmt.Errorf("workflow upload is not a generated PDF")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.objects[path]; exists {
		return nil, fmt.Errorf("workflow attempted to overwrite an existing PDF")
	}
	s.objects[path] = bytes.Clone(content)
	s.uploads++
	return &storage.UploadedObject{Bucket: s.bucket, ObjectPath: path, FileSize: int64(len(content))}, nil
}

func (s *demoClinicalStorage) Delete(_ context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, path)
	return nil
}

func (s *demoClinicalStorage) CreateSignedURL(_ context.Context, path string, _ time.Duration, filename string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.objects[path]; !exists {
		return "", fmt.Errorf("requested PDF object does not exist")
	}
	return s.server.URL + "/files/" + path + "?download=" + url.QueryEscape(filename), nil
}
