package seeder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	examinationrepo "github.com/Cendana-Project/medikaone-api/internal/repository/examination"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	prescriptionrepo "github.com/Cendana-Project/medikaone-api/internal/repository/prescription"
	hospitalsvc "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	prescriptionsvc "github.com/Cendana-Project/medikaone-api/internal/service/prescription"
)

type demoClinicalScenario struct {
	alias     string
	patient   string
	doctor    string
	practice  string
	dayOffset int
	hour      int
	minute    int
	status    string
	reason    string
}

// The cancelled booking precedes the session booking: both belong to patient3
// tomorrow, and the session occupies the full 09:00-12:00 occurrence.
var demoClinicalScenarios = []demoClinicalScenario{
	{"confirmed", "patient1", "doctor1", "general.jakarta", 1, 8, 0, "CONFIRMED", "Konsultasi kesehatan umum"},
	{"waiting_vitals", "patient2", "doctor1", "general.jakarta", 0, 8, 0, "WAITING_VITALS", "Pemeriksaan keluhan pusing"},
	{"waiting_doctor", "patient3", "doctor1", "general.jakarta", 0, 8, 30, "WAITING_DOCTOR", "Pemeriksaan kesehatan berkala"},
	{"in_consultation", "patient1", "doctor1", "general.jakarta", 0, 9, 0, "IN_CONSULTATION", "Demam ringan sejak kemarin"},
	{"completed_no_medication", "patient1", "doctor1", "general.jakarta", -1, 9, 0, "NO_MEDICATION", "Konsultasi pola hidup sehat"},
	{"completed_prescription", "patient2", "doctor1", "general.jakarta", -1, 10, 0, "ISSUED", "Demam dan nyeri kepala ringan"},
	{"cancelled", "patient3", "doctor1", "general.jakarta", 1, 11, 0, "CANCELLED", "Konsultasi kesehatan umum"},
	{"queue", "patient3", "doctor3", "lung.jakarta", 1, 9, 0, "CONFIRMED", "Kontrol keluhan pernapasan"},
}

func seedDemoClinical(seed *workflowSeedContext) error {
	if err := demoClinicalCatalog(seed); err != nil {
		return err
	}
	appointments := appointmentrepo.NewRepository(seed.DB)
	examinations := examinationrepo.NewRepository(seed.DB)
	prescriptions := prescriptionrepo.NewRepository(seed.DB)
	prescribing := prescriptionsvc.NewService(prescriptions, seed.MedicalStorage, prescriptionsvc.NewPDFRenderer(), 5*time.Minute, seed.BaseURL)
	for _, scenario := range demoClinicalScenarios {
		if err := demoClinicalAppointment(seed, appointments, examinations, prescriptions, prescribing, scenario); err != nil {
			return fmt.Errorf("seed clinical scenario %s: %w", scenario.alias, err)
		}
	}
	return demoClinicalReview(seed)
}

func demoClinicalCatalog(seed *workflowSeedContext) error {
	for _, medication := range []struct{ alias, code, name, strength string }{
		{"paracetamol", "DEMO-PCT-500", "Paracetamol", "500 mg"},
		{"salbutamol", "DEMO-SAL-2", "Salbutamol", "2 mg"},
	} {
		id := workflowUUID("medication." + medication.alias)
		var existing struct{ ID, HospitalID, CreatedBy string }
		result := seed.DB.WithContext(seed.Context).Raw(`
			SELECT id::text, hospital_id::text, created_by::text FROM hospital_medications WHERE id = ?`, id).Scan(&existing)
		if result.Error != nil {
			return result.Error
		}
		if existing.ID != "" {
			if existing.HospitalID != seed.Hospitals["jakarta"] || existing.CreatedBy != seed.Users["admin"] {
				return fmt.Errorf("demo medication %s collides with an unowned catalog entry", medication.alias)
			}
		} else {
			// Catalog roots have no seed_key or caller-supplied-ID repository API.
			// A deterministic ID plus immutable hospital/creator ownership avoids
			// adopting a same-code user entry. KFA is deliberately left NULL.
			if err := seed.DB.WithContext(seed.Context).Exec(`
				INSERT INTO hospital_medications (
					id, hospital_id, code, generic_name, dosage_form, strength,
					default_unit, default_route, created_by, updated_by, created_at, updated_at
				) VALUES (?, ?, ?, ?, 'Tablet', ?, 'tablet', 'ORAL', ?, ?, ?, ?)`,
				id, seed.Hospitals["jakarta"], medication.code, medication.name, medication.strength,
				seed.Users["admin"], seed.Users["admin"], seed.Anchor, seed.Anchor).Error; err != nil {
				return fmt.Errorf("create demo medication %s without adopting existing catalog codes: %w", medication.alias, err)
			}
		}
		seed.IDs["medication."+medication.alias] = id
	}
	return nil
}

func demoClinicalAppointment(seed *workflowSeedContext, appointments *appointmentrepo.Repository, examinations *examinationrepo.Repository, prescriptions *prescriptionrepo.Repository, prescribing *prescriptionsvc.Service, scenario demoClinicalScenario) error {
	date := seed.Anchor.In(seed.Location).AddDate(0, 0, scenario.dayOffset)
	start := time.Date(date.Year(), date.Month(), date.Day(), scenario.hour, scenario.minute, 0, 0, seed.Location).UTC()
	end := start.Add(30 * time.Minute)
	mode := "FIXED_SLOT"
	if scenario.alias == "queue" {
		end = start.Add(3 * time.Hour)
		mode = "SESSION_QUEUE"
	}
	dateString := date.Format("2006-01-02")
	scheduleID := seed.IDs[fmt.Sprintf("schedule.%s.%d", scenario.practice, int(date.Weekday()))]
	affiliationID := seed.IDs["affiliation."+scenario.practice]
	if scheduleID == "" || affiliationID == "" {
		return fmt.Errorf("missing canonical practice IDs for %s", scenario.practice)
	}
	key := workflowUUID("appointment." + scenario.alias)
	digest := sha256.Sum256([]byte(strings.Join([]string{
		seed.Users[scenario.patient], seed.Hospitals["jakarta"], seed.Users[scenario.doctor],
		affiliationID, scheduleID, dateString, start.Format(time.RFC3339), end.Format(time.RFC3339),
		mode, scenario.reason, "demo-workflows-v1",
	}, "\x00")))
	hash := hex.EncodeToString(digest[:])
	var prior []struct {
		ID, PatientID, PatientUserID, HospitalID, DoctorID, AffiliationID, ScheduleID string
		AppointmentDate, BookingMode, CreatedBy, Source, RequestHash                  string
		ScheduledStartAt, ScheduledEndAt                                              time.Time
	}
	if err := seed.DB.WithContext(seed.Context).Raw(`
		SELECT appointment.id::text, appointment.patient_id::text, patient.user_id::text AS patient_user_id,
		       appointment.hospital_id::text, appointment.doctor_id::text, appointment.affiliation_id::text,
		       appointment.schedule_id::text, appointment.appointment_date::text, appointment.booking_mode,
		       appointment.created_by::text, appointment.source, appointment.idempotency_request_hash AS request_hash,
		       appointment.scheduled_start_at, appointment.scheduled_end_at
		FROM appointments appointment JOIN patient_records patient ON patient.id = appointment.patient_record_id
		WHERE appointment.idempotency_key = ?`, key).Scan(&prior).Error; err != nil {
		return err
	}
	if len(prior) != 0 {
		if len(prior) != 1 {
			return fmt.Errorf("fixture idempotency key belongs to multiple appointments")
		}
		row := prior[0]
		if row.PatientID != seed.Users[scenario.patient] || row.PatientUserID != seed.Users[scenario.patient] ||
			row.HospitalID != seed.Hospitals["jakarta"] || row.DoctorID != seed.Users[scenario.doctor] ||
			row.AffiliationID != affiliationID || row.ScheduleID != scheduleID || row.AppointmentDate != dateString ||
			!row.ScheduledStartAt.Equal(start) || !row.ScheduledEndAt.Equal(end) || row.BookingMode != mode ||
			row.CreatedBy != seed.Users[scenario.patient] || row.Source != "SCHEDULED" || row.RequestHash != hash {
			return fmt.Errorf("fixture idempotency key collides with another appointment tuple")
		}
		seed.IDs["appointment."+scenario.alias] = row.ID
		// Existing bookings are the replay boundary. Never repeat check-in,
		// overwrite a draft, recreate a deleted decision, or reissue a PDF.
		return demoClinicalAliases(seed, scenario)
	}

	schedules, err := appointments.ListActiveSchedules(seed.Context, appointmentrepo.AvailabilityFilter{ScheduleID: scheduleID})
	if err != nil {
		return err
	}
	if len(schedules) != 1 || schedules[0].AffiliationID != affiliationID ||
		schedules[0].DoctorID != seed.Users[scenario.doctor] || schedules[0].HospitalID != seed.Hospitals["jakarta"] {
		return fmt.Errorf("canonical schedule is unavailable; preserving modified practice state")
	}
	bookedAt := seed.Now.UTC()
	if latestBooking := start.Add(-3 * time.Hour); bookedAt.After(latestBooking) {
		bookedAt = latestBooking
	}
	appointment, replay, err := appointments.Book(seed.Context, appointmentrepo.BookInput{
		PatientID: seed.Users[scenario.patient], Schedule: schedules[0], AppointmentDate: dateString,
		ScheduledStartAt: start, ScheduledEndAt: end, ReasonForVisit: scenario.reason,
		ConsentVersion: "demo-workflows-v1", ConsentIP: "127.0.0.1", ConsentUserAgent: "MedikaOne demo workflow seeder",
		IdempotencyKey: key, IdempotencyRequestHash: hash, Now: bookedAt,
	})
	if err != nil {
		return err
	}
	if replay {
		return fmt.Errorf("unexpected concurrent fixture replay; rerun the workflow transaction")
	}
	seed.IDs["appointment."+scenario.alias] = appointment.ID
	if scenario.status == "CANCELLED" {
		if err := appointments.Cancel(seed.Context, appointment.ID, seed.Users[scenario.patient], "Jadwal pasien berubah", bookedAt.Add(time.Minute)); err != nil {
			return err
		}
		return demoClinicalAliases(seed, scenario)
	}
	if scenario.status == "CONFIRMED" {
		return demoClinicalAliases(seed, scenario)
	}
	if err := appointments.CheckIn(seed.Context, appointment.ID, seed.Users["receptionist"], "IDENTITY", nil, start.Add(-15*time.Minute)); err != nil {
		return err
	}
	if _, err := examinations.EnsureEncounter(seed.Context, appointment.ID, start.Add(-15*time.Minute)); err != nil {
		return err
	}
	if scenario.status == "WAITING_VITALS" {
		return demoClinicalAliases(seed, scenario)
	}
	if err := examinations.SaveVitalDraft(seed.Context, appointment.ID, seed.Users["nurse"], demoClinicalVitals(), start.Add(-10*time.Minute)); err != nil {
		return err
	}
	if err := examinations.FinalizeVitals(seed.Context, appointment.ID, seed.Users["nurse"], start.Add(-5*time.Minute)); err != nil {
		return err
	}
	if scenario.status == "WAITING_DOCTOR" {
		return demoClinicalAliases(seed, scenario)
	}
	if err := appointments.Transition(seed.Context, appointment.ID, seed.Users[scenario.doctor], "IN_CONSULTATION", nil, start); err != nil {
		return err
	}
	if err := examinations.SaveConsultationDraft(seed.Context, appointment.ID, seed.Users[scenario.doctor], demoClinicalConsultation(scenario), start.Add(5*time.Minute)); err != nil {
		return err
	}
	clinicalContext, err := prescriptions.GetAppointmentContext(seed.Context, appointment.ID)
	if err != nil {
		return err
	}
	// Repository clocks describe the historical clinical work without changing
	// production service lead-time rules or its publication clock.
	number := "RX-DEMO-" + strings.ToUpper(strings.ReplaceAll(key, "-", "")[:24])
	if scenario.status == "NO_MEDICATION" {
		if err := prescriptions.MarkNoMedication(seed.Context, clinicalContext, seed.Users[scenario.doctor], number,
			"Tidak ada indikasi pemberian obat; edukasi pola hidup sehat dan pemantauan keluhan.", start.Add(8*time.Minute)); err != nil {
			return err
		}
	} else {
		medication, err := prescriptions.GetMedication(seed.Context, seed.Hospitals["jakarta"], seed.IDs["medication.paracetamol"])
		if err != nil {
			return err
		}
		input := request.PrescriptionDraftRequest{Items: []request.PrescriptionItemRequest{{
			Type: "NON_COMPOUND", MedicationID: &medication.ID, MedicationName: medication.GenericName,
			DosageForm: medication.DosageForm, Strength: medication.Strength, DoseAmount: 1, DoseUnit: "tablet", Route: "ORAL",
			DurationValue: 3, DurationUnit: "DAY", Quantity: 9, QuantityUnit: "tablet", AsNeeded: true,
			Directions:   "Minum satu tablet bila demam atau nyeri, paling cepat setiap 8 jam.",
			MaxDailyDose: new("3 tablet per hari"), IntervalHours: new(8),
		}}}
		if err := prescriptions.SaveDraft(seed.Context, clinicalContext, seed.Users[scenario.doctor], number, input, start.Add(8*time.Minute)); err != nil {
			return err
		}
	}
	if scenario.status == "IN_CONSULTATION" {
		return demoClinicalAliases(seed, scenario)
	}
	completedAt := start.Add(20 * time.Minute)
	if scenario.status == "ISSUED" {
		draft, err := prescriptions.GetByAppointment(seed.Context, appointment.ID)
		if err != nil {
			return err
		}
		if draft.CurrentRevision == nil {
			return fmt.Errorf("new prescription has no draft revision")
		}
		if _, err := prescribing.Issue(seed.Context, seed.Users[scenario.doctor], appointment.ID, request.IssuePrescriptionRequest{
			ExpectedRevisionID: draft.CurrentRevision.ID, AllergiesReviewed: true,
		}); err != nil {
			return err
		}
		// The visit is historical; its real PDF is published during this run.
		// Complete afterwards, rather than claiming that publication was backdated.
		completedAt = time.Now().UTC()
	}
	if err := examinations.CompleteConsultation(seed.Context, appointment.ID, seed.Users[scenario.doctor], completedAt); err != nil {
		return err
	}
	return demoClinicalAliases(seed, scenario)
}

func demoClinicalAliases(seed *workflowSeedContext, scenario demoClinicalScenario) error {
	var row struct {
		PatientRecordID, EncounterID, PrescriptionID string
		CurrentRevisionID, DocumentID                string
	}
	if err := seed.DB.WithContext(seed.Context).Raw(`
		SELECT appointment.patient_record_id::text, COALESCE(encounter.id::text, '') AS encounter_id,
		       COALESCE(prescription.id::text, '') AS prescription_id,
		       COALESCE(prescription.current_revision_id::text, '') AS current_revision_id,
		       COALESCE(document.id::text, '') AS document_id
		FROM appointments appointment
		LEFT JOIN medical_encounters encounter ON encounter.appointment_id = appointment.id
		LEFT JOIN prescriptions prescription ON prescription.encounter_id = encounter.id
		LEFT JOIN prescription_documents document ON document.revision_id = prescription.current_revision_id
		WHERE appointment.id = ?`, seed.IDs["appointment."+scenario.alias]).Scan(&row).Error; err != nil {
		return err
	}
	seed.IDs["patient_record."+scenario.patient] = row.PatientRecordID
	if row.EncounterID != "" {
		seed.IDs["encounter."+scenario.alias] = row.EncounterID
	}
	if scenario.alias == "in_consultation" && row.PrescriptionID != "" {
		seed.IDs["prescription.draft"] = row.PrescriptionID
	}
	if scenario.alias == "completed_no_medication" && row.PrescriptionID != "" {
		seed.IDs["prescription.no_medication"] = row.PrescriptionID
	}
	if scenario.alias == "completed_prescription" && row.PrescriptionID != "" {
		seed.IDs["prescription.issued"] = row.PrescriptionID
		if row.CurrentRevisionID != "" {
			seed.IDs["prescription.revision"] = row.CurrentRevisionID
		}
		if row.DocumentID != "" {
			seed.IDs["prescription.document"] = row.DocumentID
		}
	}
	return nil
}

func demoClinicalReview(seed *workflowSeedContext) error {
	var id string
	// Include soft-deleted reviews: a user's deletion is intentional state, not
	// an invitation for the seeder to revive their review.
	if err := seed.DB.WithContext(seed.Context).Raw(`SELECT id::text FROM hospital_reviews WHERE hospital_id = ? AND user_id = ?`,
		seed.Hospitals["jakarta"], seed.Users["patient1"]).Scan(&id).Error; err != nil {
		return err
	}
	if id == "" {
		var completed bool
		if err := seed.DB.WithContext(seed.Context).Raw(`SELECT status = 'COMPLETED' FROM appointments WHERE id = ?`,
			seed.IDs["appointment.completed_no_medication"]).Scan(&completed).Error; err != nil {
			return err
		}
		if !completed {
			return nil
		}
		service := hospitalsvc.NewService(nil, nil, hospitalrepo.NewRepository(seed.DB))
		review, err := service.PutReview(seed.Context, seed.Hospitals["jakarta"], seed.Users["patient1"], request.PutHospitalReviewRequest{
			Rating: 5, Comment: "Pendaftaran mudah, pemeriksaan jelas, dan edukasi dokter membantu.",
		})
		if err != nil {
			return err
		}
		id = review.ID
	}
	seed.IDs["review.jakarta.patient1"] = id
	return nil
}

func demoClinicalVitals() request.VitalSignsRequest {
	return request.VitalSignsRequest{
		HeightCM: new(168.0), WeightKG: new(65.0), TemperatureC: new(37.2),
		SystolicMMHG: new(118), DiastolicMMHG: new(76), HeartRateBPM: new(80),
		RespiratoryRateBPM: new(18), OxygenSaturationPercent: new(98.0),
		NurseNote: new("Pasien sadar, kooperatif, dan dapat berjalan mandiri."),
	}
}

func demoClinicalConsultation(scenario demoClinicalScenario) request.ConsultationNoteRequest {
	if scenario.status == "NO_MEDICATION" {
		return request.ConsultationNoteRequest{
			Subjective: new("Pasien ingin berkonsultasi tentang pola makan dan aktivitas fisik. Tidak ada keluhan akut."),
			Objective:  new("Keadaan umum baik; tanda vital stabil. Tidak ditemukan tanda kegawatan."),
			Assessment: new("Kebutuhan konseling dan pemantauan kesehatan umum, tanpa indikasi terapi obat."),
			Plan:       new("Edukasi pola makan seimbang, aktivitas fisik teratur, istirahat cukup, dan kontrol bila muncul keluhan."),
			Diagnoses:  []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", ICD10: new("Z71.3"), Name: "Konseling dan pengawasan diet"}},
		}
	}
	return request.ConsultationNoteRequest{
		Subjective: new("Demam ringan dan nyeri kepala sejak kemarin. Tidak ada sesak napas atau penurunan kesadaran."),
		Objective:  new("Keadaan umum baik, hemodinamik stabil, dan tidak ditemukan tanda bahaya."),
		Assessment: new("Demam ringan tanpa tanda kegawatan; evaluasi ulang bila keluhan menetap."),
		Plan:       new("Istirahat, cukup cairan, terapi simptomatik bila perlu; kembali bila demam menetap atau muncul tanda bahaya."),
		Diagnoses:  []request.DiagnosisRequest{{Type: "PRIMARY", Status: "CONFIRMED", ICD10: new("R50.9"), Name: "Demam, tidak spesifik"}},
	}
}
