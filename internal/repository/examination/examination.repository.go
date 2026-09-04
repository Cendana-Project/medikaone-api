package examination

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var (
	ErrAppointmentNotFound          = errors.New("examination appointment not found")
	ErrEncounterNotFound            = errors.New("medical encounter not found")
	ErrInvalidState                 = errors.New("invalid examination state")
	ErrVitalsDraftNotFound          = errors.New("vital signs draft not found")
	ErrVitalsRevisionNotFound       = errors.New("finalized vital signs revision not found")
	ErrConsultationDraftNotFound    = errors.New("consultation draft not found")
	ErrConsultationRevisionNotFound = errors.New("finalized consultation revision not found")
	ErrAttachmentNotFound           = errors.New("medical attachment not found")
	ErrPrescriptionDecisionRequired = errors.New("prescription decision required")
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type AppointmentContext struct {
	AppointmentID   string
	PatientRecordID string
	PatientUserID   *string
	HospitalID      string
	DoctorID        string
	DepartmentID    string
	Status          string
	ConsentVersion  string
	ConsentedAt     time.Time
}

type AttachmentRecord struct {
	ID              string
	EncounterID     string
	AppointmentID   string
	PatientRecordID string
	HospitalID      string
	DoctorID        string
	PatientUserID   *string
	EncounterStatus string
	Filename        string
	MIMEType        string
	Bucket          string
	ObjectPath      string
	FileSize        int64
	SHA256          string
	Note            *string
	UploadedBy      string
	CreatedAt       time.Time
}

type AttachmentInput struct {
	AppointmentID string
	ActorID       string
	DocumentType  string
	Filename      string
	MIMEType      string
	Bucket        string
	ObjectPath    string
	FileSize      int64
	SHA256        string
	Note          *string
	Now           time.Time
}

func (r *Repository) GetAppointmentContext(ctx context.Context, appointmentID string) (*AppointmentContext, error) {
	return getAppointmentContext(ctx, r.db, appointmentID, false)
}

func getAppointmentContext(ctx context.Context, db *gorm.DB, appointmentID string, lock bool) (*AppointmentContext, error) {
	query := `
		SELECT appointment.id::text AS appointment_id,
		       appointment.patient_record_id::text AS patient_record_id,
		       patient.user_id::text AS patient_user_id,
		       appointment.hospital_id::text AS hospital_id,
		       appointment.doctor_id::text AS doctor_id,
		       appointment.department_id::text AS department_id,
		       appointment.status, appointment.consent_version, appointment.consented_at
		FROM appointments appointment
		JOIN patient_records patient ON patient.id = appointment.patient_record_id
		WHERE appointment.id = ?`
	if lock {
		query += ` FOR UPDATE OF appointment`
	}
	var out AppointmentContext
	result := db.WithContext(ctx).Raw(query, appointmentID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrAppointmentNotFound
	}
	return &out, nil
}

func ensureEncounter(ctx context.Context, tx *gorm.DB, appointment *AppointmentContext, now time.Time) (string, error) {
	var encounterID string
	result := tx.WithContext(ctx).Raw(`
		INSERT INTO medical_encounters (
			appointment_id, patient_record_id, hospital_id, doctor_id, department_id,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'OPEN', ?, ?)
		ON CONFLICT (appointment_id) DO UPDATE SET updated_at = medical_encounters.updated_at
		RETURNING id::text
	`, appointment.AppointmentID, appointment.PatientRecordID, appointment.HospitalID,
		appointment.DoctorID, appointment.DepartmentID, now, now).Scan(&encounterID)
	if result.Error != nil {
		return "", result.Error
	}
	return encounterID, nil
}

func (r *Repository) EnsureEncounter(ctx context.Context, appointmentID string, now time.Time) (string, error) {
	appointment, err := r.GetAppointmentContext(ctx, appointmentID)
	if err != nil {
		return "", err
	}
	return ensureEncounter(ctx, r.db, appointment, now)
}

func (r *Repository) SaveVitalDraft(ctx context.Context, appointmentID, actorID string, input request.VitalSignsRequest, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status != "WAITING_VITALS" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		result := tx.Exec(`
			UPDATE vital_sign_revisions SET
				height_cm = ?, weight_kg = ?, temperature_c = ?, systolic_mmhg = ?,
				diastolic_mmhg = ?, heart_rate_bpm = ?, respiratory_rate_bpm = ?,
				oxygen_saturation_percent = ?, nurse_note = ?, skipped_reason = ?,
				recorded_by = ?, updated_at = ?
			WHERE encounter_id = ? AND status = 'DRAFT'
		`, input.HeightCM, input.WeightKG, input.TemperatureC, input.SystolicMMHG,
			input.DiastolicMMHG, input.HeartRateBPM, input.RespiratoryRateBPM,
			input.OxygenSaturationPercent, input.NurseNote, input.SkippedReason,
			actorID, now, encounterID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Exec(`
				INSERT INTO vital_sign_revisions (
					encounter_id, version, status, height_cm, weight_kg, temperature_c,
					systolic_mmhg, diastolic_mmhg, heart_rate_bpm, respiratory_rate_bpm,
					oxygen_saturation_percent, nurse_note, skipped_reason, recorded_by,
					created_at, updated_at
				) SELECT ?, COALESCE(MAX(version), 0) + 1, 'DRAFT', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
				FROM vital_sign_revisions WHERE encounter_id = ?
			`, encounterID, input.HeightCM, input.WeightKG, input.TemperatureC,
				input.SystolicMMHG, input.DiastolicMMHG, input.HeartRateBPM,
				input.RespiratoryRateBPM, input.OxygenSaturationPercent, input.NurseNote,
				input.SkippedReason, actorID, now, now, encounterID).Error; err != nil {
				return err
			}
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "VITALS_DRAFT_SAVED", nil, now)
	})
}

func (r *Repository) FinalizeVitals(ctx context.Context, appointmentID, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status != "WAITING_VITALS" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		result := tx.Exec(`
			UPDATE vital_sign_revisions SET status = 'FINALIZED', finalized_by = ?,
				finalized_at = ?, updated_at = ?
			WHERE encounter_id = ? AND status = 'DRAFT'
		`, actorID, now, now, encounterID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVitalsDraftNotFound
		}
		result = tx.Exec(`
			UPDATE appointments SET status = 'WAITING_DOCTOR', updated_at = ?
			WHERE id = ? AND status = 'WAITING_VITALS'
		`, now, appointmentID)
		if result.Error != nil || result.RowsAffected == 0 {
			if result.Error != nil {
				return result.Error
			}
			return ErrInvalidState
		}
		if err := tx.Exec(`
			INSERT INTO appointment_status_events
				(appointment_id, actor_id, event_type, from_status, to_status, created_at)
			VALUES (?, ?, 'WAITING_DOCTOR', 'WAITING_VITALS', 'WAITING_DOCTOR', ?)
		`, appointmentID, actorID, now).Error; err != nil {
			return err
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "VITALS_FINALIZED", nil, now)
	})
}

func (r *Repository) CorrectVitals(ctx context.Context, appointmentID, actorID string, input request.CorrectVitalSignsRequest, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status == "CONFIRMED" || appointment.Status == "WAITING_VITALS" || appointment.Status == "CANCELLED" || appointment.Status == "NO_SHOW" || appointment.Status == "RESCHEDULED" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		var previousID string
		var nextVersion int
		result := tx.Raw(`
			SELECT id::text, version + 1 FROM vital_sign_revisions
			WHERE encounter_id = ? AND status = 'FINALIZED'
			ORDER BY version DESC LIMIT 1 FOR UPDATE
		`, encounterID).Row()
		if err := result.Scan(&previousID, &nextVersion); err != nil {
			return ErrVitalsRevisionNotFound
		}
		if err := tx.Exec(`
			INSERT INTO vital_sign_revisions (
				encounter_id, version, status, height_cm, weight_kg, temperature_c,
				systolic_mmhg, diastolic_mmhg, heart_rate_bpm, respiratory_rate_bpm,
				oxygen_saturation_percent, nurse_note, skipped_reason, recorded_by,
				supersedes_revision_id, correction_reason,
				created_at, updated_at
			) VALUES (?, ?, 'DRAFT', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, encounterID, nextVersion, input.HeightCM, input.WeightKG, input.TemperatureC,
			input.SystolicMMHG, input.DiastolicMMHG, input.HeartRateBPM,
			input.RespiratoryRateBPM, input.OxygenSaturationPercent, input.NurseNote,
			input.SkippedReason, actorID, previousID, input.CorrectionReason,
			now, now).Error; err != nil {
			return err
		}
		updateResult := tx.Exec(`
			UPDATE vital_sign_revisions
			SET status = 'FINALIZED', finalized_by = ?, finalized_at = ?, updated_at = ?
			WHERE encounter_id = ? AND version = ? AND status = 'DRAFT'
		`, actorID, now, now, encounterID, nextVersion)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return ErrVitalsDraftNotFound
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "VITALS_CORRECTED", map[string]any{"reason": input.CorrectionReason, "supersedes_revision_id": previousID}, now)
	})
}

func (r *Repository) SaveConsultationDraft(ctx context.Context, appointmentID, actorID string, input request.ConsultationNoteRequest, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status != "IN_CONSULTATION" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		var revisionID string
		result := tx.Raw(`SELECT id::text FROM consultation_note_revisions WHERE encounter_id = ? AND status = 'DRAFT' FOR UPDATE`, encounterID).Scan(&revisionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Raw(`
				INSERT INTO consultation_note_revisions (
					encounter_id, version, status, subjective, objective, assessment, plan,
					internal_note, authored_by, created_at, updated_at
				) SELECT ?, COALESCE(MAX(version), 0) + 1, 'DRAFT', ?, ?, ?, ?, ?, ?, ?, ?
				FROM consultation_note_revisions WHERE encounter_id = ? RETURNING id::text
			`, encounterID, input.Subjective, input.Objective, input.Assessment, input.Plan,
				input.InternalNote, actorID, now, now, encounterID).Scan(&revisionID).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Exec(`
				UPDATE consultation_note_revisions SET subjective = ?, objective = ?, assessment = ?,
					plan = ?, internal_note = ?, authored_by = ?, updated_at = ? WHERE id = ?
			`, input.Subjective, input.Objective, input.Assessment, input.Plan,
				input.InternalNote, actorID, now, revisionID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DELETE FROM encounter_diagnoses WHERE consultation_revision_id = ?`, revisionID).Error; err != nil {
				return err
			}
		}
		if err := insertDiagnoses(tx, revisionID, actorID, input.Diagnoses, now); err != nil {
			return err
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "CONSULTATION_DRAFT_SAVED", nil, now)
	})
}

func (r *Repository) CompleteConsultation(ctx context.Context, appointmentID, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status != "IN_CONSULTATION" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		var prescriptionStatus string
		prescriptionResult := tx.Raw(`
			SELECT status FROM prescriptions WHERE encounter_id = ? FOR UPDATE
		`, encounterID).Scan(&prescriptionStatus)
		if prescriptionResult.Error != nil {
			return prescriptionResult.Error
		}
		if prescriptionResult.RowsAffected == 0 || (prescriptionStatus != "ISSUED" && prescriptionStatus != "NO_MEDICATION") {
			return ErrPrescriptionDecisionRequired
		}
		result := tx.Exec(`
			UPDATE consultation_note_revisions SET status = 'FINALIZED', finalized_by = ?,
				finalized_at = ?, updated_at = ?
			WHERE encounter_id = ? AND status = 'DRAFT'
			  AND NULLIF(BTRIM(subjective), '') IS NOT NULL
			  AND NULLIF(BTRIM(objective), '') IS NOT NULL
			  AND NULLIF(BTRIM(assessment), '') IS NOT NULL
			  AND NULLIF(BTRIM(plan), '') IS NOT NULL
		  AND EXISTS (SELECT 1 FROM encounter_diagnoses diagnosis WHERE diagnosis.consultation_revision_id = consultation_note_revisions.id)
		  AND EXISTS (SELECT 1 FROM encounter_diagnoses diagnosis WHERE diagnosis.consultation_revision_id = consultation_note_revisions.id AND diagnosis.diagnosis_type = 'PRIMARY')
		`, actorID, now, now, encounterID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConsultationDraftNotFound
		}
		if err := tx.Exec(`UPDATE medical_encounters SET status = 'COMPLETED', completed_at = ?, updated_at = ? WHERE id = ? AND status = 'OPEN'`, now, now, encounterID).Error; err != nil {
			return err
		}
		result = tx.Exec(`UPDATE appointments SET status = 'COMPLETED', completed_at = ?, updated_at = ? WHERE id = ? AND status = 'IN_CONSULTATION'`, now, now, appointmentID)
		if result.Error != nil || result.RowsAffected == 0 {
			if result.Error != nil {
				return result.Error
			}
			return ErrInvalidState
		}
		if err := tx.Exec(`
			INSERT INTO appointment_status_events
				(appointment_id, actor_id, event_type, from_status, to_status, created_at)
			VALUES (?, ?, 'COMPLETED', 'IN_CONSULTATION', 'COMPLETED', ?)
		`, appointmentID, actorID, now).Error; err != nil {
			return err
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "CONSULTATION_COMPLETED", nil, now)
	})
}

func (r *Repository) CorrectConsultation(ctx context.Context, appointmentID, actorID string, input request.CorrectConsultationNoteRequest, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, appointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status != "COMPLETED" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, now)
		if err != nil {
			return err
		}
		var previousID string
		var nextVersion int
		if err := tx.Raw(`
			SELECT id::text, version + 1 FROM consultation_note_revisions
			WHERE encounter_id = ? AND status = 'FINALIZED'
			ORDER BY version DESC LIMIT 1 FOR UPDATE
		`, encounterID).Row().Scan(&previousID, &nextVersion); err != nil {
			return ErrConsultationRevisionNotFound
		}
		var revisionID string
		if err := tx.Raw(`
			INSERT INTO consultation_note_revisions (
				encounter_id, version, status, subjective, objective, assessment, plan,
				internal_note, authored_by,
				supersedes_revision_id, correction_reason, created_at, updated_at
			) VALUES (?, ?, 'DRAFT', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id::text
		`, encounterID, nextVersion, input.Subjective, input.Objective, input.Assessment,
			input.Plan, input.InternalNote, actorID, previousID,
			input.CorrectionReason, now, now).Scan(&revisionID).Error; err != nil {
			return err
		}
		if err := insertDiagnoses(tx, revisionID, actorID, input.Diagnoses, now); err != nil {
			return err
		}
		result := tx.Exec(`
			UPDATE consultation_note_revisions
			SET status = 'FINALIZED', finalized_by = ?, finalized_at = ?, updated_at = ?
			WHERE id = ? AND status = 'DRAFT'
		`, actorID, now, now, revisionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConsultationDraftNotFound
		}
		return insertAudit(ctx, tx, encounterID, appointment, actorID, "CONSULTATION_CORRECTED", map[string]any{"reason": input.CorrectionReason, "supersedes_revision_id": previousID}, now)
	})
}

func insertDiagnoses(tx *gorm.DB, revisionID, actorID string, diagnoses []request.DiagnosisRequest, now time.Time) error {
	for _, diagnosis := range diagnoses {
		if err := tx.Exec(`
			INSERT INTO encounter_diagnoses (
				consultation_revision_id, diagnosis_type, diagnosis_status, icd10_code,
				diagnosis_name, note, created_by, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, revisionID, diagnosis.Type, diagnosis.Status, diagnosis.ICD10,
			diagnosis.Name, diagnosis.Note, actorID, now).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) AddAttachment(ctx context.Context, input AttachmentInput) (*response.MedicalAttachment, error) {
	var out response.MedicalAttachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		appointment, err := getAppointmentContext(ctx, tx, input.AppointmentID, true)
		if err != nil {
			return err
		}
		if appointment.Status == "CANCELLED" || appointment.Status == "NO_SHOW" || appointment.Status == "RESCHEDULED" || appointment.Status == "CONFIRMED" {
			return ErrInvalidState
		}
		encounterID, err := ensureEncounter(ctx, tx, appointment, input.Now)
		if err != nil {
			return err
		}
		result := tx.Raw(`
			INSERT INTO medical_record_attachments (
				encounter_id, document_type, filename, mime_type, bucket, object_path,
				file_size, sha256, note, uploaded_by, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id::text AS id, document_type, filename, mime_type, file_size,
			          sha256, note, uploaded_by::text AS uploaded_by, created_at
		`, encounterID, input.DocumentType, input.Filename, input.MIMEType, input.Bucket,
			input.ObjectPath, input.FileSize, input.SHA256, input.Note, input.ActorID,
			input.Now).Scan(&out)
		if result.Error != nil {
			return result.Error
		}
		return insertAudit(ctx, tx, encounterID, appointment, input.ActorID, "ATTACHMENT_UPLOADED", map[string]any{"attachment_id": out.ID, "document_type": input.DocumentType}, input.Now)
	})
	return &out, err
}

func (r *Repository) GetAttachment(ctx context.Context, attachmentID string) (*AttachmentRecord, error) {
	var out AttachmentRecord
	result := r.db.WithContext(ctx).Raw(`
		SELECT attachment.id::text, attachment.encounter_id::text,
		       encounter.appointment_id::text, encounter.patient_record_id::text,
		       encounter.hospital_id::text, encounter.doctor_id::text,
		       patient.user_id::text AS patient_user_id, encounter.status AS encounter_status,
		       attachment.filename, attachment.mime_type, attachment.bucket,
		       attachment.object_path, attachment.file_size, attachment.sha256,
		       attachment.note, attachment.uploaded_by::text, attachment.created_at
		FROM medical_record_attachments attachment
		JOIN medical_encounters encounter ON encounter.id = attachment.encounter_id
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		WHERE attachment.id = ?
	`, attachmentID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrAttachmentNotFound
	}
	return &out, nil
}

func (r *Repository) LogAttachmentAccess(ctx context.Context, attachment *AttachmentRecord, actorID string, now time.Time) error {
	appointment, err := r.GetAppointmentContext(ctx, attachment.AppointmentID)
	if err != nil {
		return err
	}
	return insertAudit(ctx, r.db, attachment.EncounterID, appointment, actorID, "ATTACHMENT_URL_CREATED", map[string]any{"attachment_id": attachment.ID}, now)
}

func (r *Repository) GetEncounter(ctx context.Context, appointmentID, actorID string, includeDraft bool, now time.Time) (*response.MedicalEncounter, error) {
	var out response.MedicalEncounter
	result := r.db.WithContext(ctx).Raw(`
		SELECT encounter.id::text, encounter.appointment_id::text,
		       encounter.patient_record_id::text,
		       patient.user_id::text AS patient_medika_one_id,
		       CONCAT_WS(' ', patient.first_name, NULLIF(patient.last_name, '')) AS patient_name,
		       patient.dob::text AS patient_date_of_birth, patient.gender AS patient_gender,
		       profile.allergies AS patient_allergies,
		       profile.medical_hist AS patient_medical_history,
		       encounter.hospital_id::text, hospital.name AS hospital_name,
		       encounter.doctor_id::text,
		       CONCAT_WS(' ', doctor.first_name, NULLIF(doctor.last_name, '')) AS doctor_name,
		       encounter.department_id::text, department.name AS department_name,
		       appointment.appointment_date::text, appointment.reason_for_visit,
		       encounter.status, encounter.completed_at, encounter.created_at, encounter.updated_at
		FROM medical_encounters encounter
		JOIN appointments appointment ON appointment.id = encounter.appointment_id
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		LEFT JOIN patient_profiles profile ON profile.user_id = patient.user_id
		JOIN hospitals hospital ON hospital.id = encounter.hospital_id
		JOIN users doctor ON doctor.id = encounter.doctor_id
		JOIN hospital_departments department ON department.id = encounter.department_id
		WHERE encounter.appointment_id = ?
	`, appointmentID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrEncounterNotFound
	}

	statusWhere := ""
	if !includeDraft {
		statusWhere = " AND status = 'FINALIZED'"
	}
	var vital response.VitalSigns
	result = r.db.WithContext(ctx).Raw(`
		SELECT id::text, version, status, height_cm, weight_kg, bmi, temperature_c,
		       systolic_mmhg, diastolic_mmhg, heart_rate_bpm, respiratory_rate_bpm,
		       oxygen_saturation_percent, nurse_note, skipped_reason,
		       recorded_by::text, finalized_by::text, finalized_at,
		       supersedes_revision_id::text, correction_reason, created_at, updated_at
		FROM vital_sign_revisions WHERE encounter_id = ?`+statusWhere+`
		ORDER BY version DESC LIMIT 1
	`, out.ID).Scan(&vital)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		out.Vitals = &vital
	}

	var note response.ConsultationNote
	result = r.db.WithContext(ctx).Raw(`
		SELECT id::text, version, status, subjective, objective, assessment, plan,
		       internal_note, authored_by::text, finalized_by::text, finalized_at,
		       supersedes_revision_id::text, correction_reason, created_at, updated_at
		FROM consultation_note_revisions WHERE encounter_id = ?`+statusWhere+`
		ORDER BY version DESC LIMIT 1
	`, out.ID).Scan(&note)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		note.Diagnoses = make([]response.Diagnosis, 0)
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id::text, diagnosis_type AS type, diagnosis_status AS status,
			       icd10_code AS icd10, diagnosis_name AS name, note,
			       created_by::text, created_at
			FROM encounter_diagnoses WHERE consultation_revision_id = ?
			ORDER BY CASE diagnosis_type WHEN 'PRIMARY' THEN 0 ELSE 1 END, created_at
		`, note.ID).Scan(&note.Diagnoses).Error; err != nil {
			return nil, err
		}
		out.Consultation = &note
	}
	out.Attachments = make([]response.MedicalAttachment, 0)
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id::text, document_type, filename, mime_type, file_size, sha256,
		       note, uploaded_by::text, created_at
		FROM medical_record_attachments WHERE encounter_id = ? ORDER BY created_at
	`, out.ID).Scan(&out.Attachments).Error; err != nil {
		return nil, err
	}

	appointment, err := r.GetAppointmentContext(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, r.db, out.ID, appointment, actorID, "MEDICAL_RECORD_VIEWED", nil, now); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Repository) ListHistoryForAppointment(ctx context.Context, appointmentID, actorID string, now time.Time) ([]response.MedicalEncounterSummary, error) {
	appointment, err := r.GetAppointmentContext(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	rows, err := r.listHistory(ctx, "encounter.patient_record_id = ?", appointment.PatientRecordID)
	if err != nil {
		return nil, err
	}
	encounterID, err := r.EnsureEncounter(ctx, appointmentID, now)
	if err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, r.db, encounterID, appointment, actorID, "MEDICAL_HISTORY_VIEWED", map[string]any{"record_count": len(rows)}, now); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetHistoryEncounterForAppointment(ctx context.Context, appointmentID, encounterID, actorID string, now time.Time) (*response.MedicalEncounter, error) {
	var historicalAppointmentID string
	result := r.db.WithContext(ctx).Raw(`
		SELECT historical.appointment_id::text
		FROM medical_encounters historical
		JOIN appointments current_appointment ON current_appointment.id = ?
		WHERE historical.id = ?
		  AND historical.status = 'COMPLETED'
		  AND historical.patient_record_id = current_appointment.patient_record_id
	`, appointmentID, encounterID).Scan(&historicalAppointmentID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrEncounterNotFound
	}
	return r.GetEncounter(ctx, historicalAppointmentID, actorID, false, now)
}

func (r *Repository) ListHistoryForPatient(ctx context.Context, patientUserID, actorID string, now time.Time) ([]response.MedicalEncounterSummary, error) {
	rows, err := r.listHistory(ctx, "patient.user_id = ?", patientUserID)
	if err != nil {
		return nil, err
	}
	// Self-history can span multiple encounters. Audit every returned encounter so
	// access remains attributable even when the response is a collection.
	for _, row := range rows {
		appointment, contextErr := r.GetAppointmentContext(ctx, row.AppointmentID)
		if contextErr != nil {
			return nil, contextErr
		}
		if err := insertAudit(ctx, r.db, row.ID, appointment, actorID, "PATIENT_MEDICAL_HISTORY_VIEWED", nil, now); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r *Repository) listHistory(ctx context.Context, where string, arg any) ([]response.MedicalEncounterSummary, error) {
	rows := make([]response.MedicalEncounterSummary, 0)
	query := `
		SELECT encounter.id::text, encounter.appointment_id::text,
		       encounter.hospital_id::text, hospital.name AS hospital_name,
		       encounter.doctor_id::text,
		       CONCAT_WS(' ', doctor.first_name, NULLIF(doctor.last_name, '')) AS doctor_name,
		       department.name AS department_name, appointment.appointment_date::text,
		       encounter.completed_at
		FROM medical_encounters encounter
		JOIN appointments appointment ON appointment.id = encounter.appointment_id
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		JOIN hospitals hospital ON hospital.id = encounter.hospital_id
		JOIN users doctor ON doctor.id = encounter.doctor_id
		JOIN hospital_departments department ON department.id = encounter.department_id
		WHERE encounter.status = 'COMPLETED' AND ` + where + `
		ORDER BY appointment.appointment_date DESC, encounter.completed_at DESC
		LIMIT 100`
	if err := r.db.WithContext(ctx).Raw(query, arg).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		var diagnosis response.Diagnosis
		result := r.db.WithContext(ctx).Raw(`
			SELECT diagnosis.id::text, diagnosis.diagnosis_type AS type,
			       diagnosis.diagnosis_status AS status, diagnosis.icd10_code AS icd10,
			       diagnosis.diagnosis_name AS name, diagnosis.note,
			       diagnosis.created_by::text, diagnosis.created_at
			FROM consultation_note_revisions revision
			JOIN encounter_diagnoses diagnosis ON diagnosis.consultation_revision_id = revision.id
			WHERE revision.encounter_id = ? AND revision.status = 'FINALIZED'
			  AND diagnosis.diagnosis_type = 'PRIMARY'
			ORDER BY revision.version DESC LIMIT 1
		`, rows[i].ID).Scan(&diagnosis)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected > 0 {
			rows[i].PrimaryDiagnosis = &diagnosis
		}
	}
	return rows, nil
}

func (r *Repository) GetPatientEncounter(ctx context.Context, encounterID, patientUserID, actorID string, now time.Time) (*response.MedicalEncounter, error) {
	var appointmentID string
	result := r.db.WithContext(ctx).Raw(`
		SELECT encounter.appointment_id::text
		FROM medical_encounters encounter
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		WHERE encounter.id = ? AND encounter.status = 'COMPLETED' AND patient.user_id = ?
	`, encounterID, patientUserID).Scan(&appointmentID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrEncounterNotFound
	}
	return r.GetEncounter(ctx, appointmentID, actorID, false, now)
}

func insertAudit(ctx context.Context, tx *gorm.DB, encounterID string, appointment *AppointmentContext, actorID, action string, metadata any, now time.Time) error {
	payload := []byte("{}")
	if metadata != nil {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		payload = encoded
	}
	return tx.WithContext(ctx).Exec(`
		INSERT INTO medical_record_audit_events (
			encounter_id, appointment_id, patient_record_id, hospital_id,
			actor_id, action, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?)
	`, encounterID, appointment.AppointmentID, appointment.PatientRecordID,
		appointment.HospitalID, actorID, action, string(payload), now).Error
}
