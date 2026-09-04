package prescription

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var (
	ErrNotFound           = errors.New("prescription not found")
	ErrMedicationNotFound = errors.New("medication not found")
	ErrInvalidState       = errors.New("prescription state does not allow this operation")
	ErrConcurrentUpdate   = errors.New("prescription was changed concurrently")
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type AppointmentContext struct {
	AppointmentID      string
	EncounterID        string
	PatientRecordID    string
	PatientUserID      *string
	PatientName        string
	PatientDateOfBirth string
	PatientGender      string
	PatientAllergies   *string
	HospitalID         string
	HospitalName       string
	HospitalAddress    *string
	HospitalPhone      *string
	DoctorID           string
	DoctorName         string
	DoctorSIPNumber    *string
	AppointmentDate    string
	AppointmentStatus  string
}

type DocumentInput struct {
	Bucket     string
	ObjectPath string
	Filename   string
	FileSize   int64
	SHA256     string
}

type ClinicalSnapshot struct {
	PatientName        string
	PatientDateOfBirth string
	PatientGender      string
	PatientAllergies   *string
	HospitalName       string
	HospitalAddress    *string
	HospitalPhone      *string
	DoctorName         string
	DoctorSIPNumber    string
}

type DocumentRecord struct {
	ID             string
	PrescriptionID string
	RevisionID     string
	Bucket         string
	ObjectPath     string
	Filename       string
	MIMEType       string
	FileSize       int64
	SHA256         string
	GeneratedAt    time.Time
}

func (r *Repository) GetAppointmentContext(ctx context.Context, appointmentID string) (*AppointmentContext, error) {
	var out AppointmentContext
	result := r.db.WithContext(ctx).Raw(`
		SELECT appointment.id::text AS appointment_id, encounter.id::text AS encounter_id,
		       appointment.patient_record_id::text, patient.user_id::text AS patient_user_id,
		       CONCAT_WS(' ', patient.first_name, NULLIF(patient.last_name, '')) AS patient_name,
		       patient.dob::text AS patient_date_of_birth, patient.gender AS patient_gender,
		       profile.allergies AS patient_allergies,
		       appointment.hospital_id::text, hospital.name AS hospital_name,
		       hospital.address AS hospital_address, hospital.phone AS hospital_phone,
		       appointment.doctor_id::text,
		       CONCAT_WS(' ', doctor.first_name, NULLIF(doctor.last_name, '')) AS doctor_name,
		       doctor_profile.sip_number AS doctor_sip_number,
		       appointment.appointment_date::text, appointment.status AS appointment_status
		FROM appointments appointment
		JOIN medical_encounters encounter ON encounter.appointment_id = appointment.id
		JOIN patient_records patient ON patient.id = appointment.patient_record_id
		LEFT JOIN patient_profiles profile ON profile.user_id = patient.user_id
		JOIN hospitals hospital ON hospital.id = appointment.hospital_id
		JOIN users doctor ON doctor.id = appointment.doctor_id
		LEFT JOIN doctor_profiles doctor_profile ON doctor_profile.user_id = doctor.id
		WHERE appointment.id = ?
	`, appointmentID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &out, nil
}

func (r *Repository) HasPrimaryDiagnosis(ctx context.Context, encounterID string) (bool, error) {
	var found bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM consultation_note_revisions revision
			JOIN encounter_diagnoses diagnosis ON diagnosis.consultation_revision_id = revision.id
			WHERE revision.encounter_id = ? AND diagnosis.diagnosis_type = 'PRIMARY'
			ORDER BY revision.version DESC LIMIT 1
		) AS found
	`, encounterID).Scan(&found).Error
	return found, err
}

func (r *Repository) CreateMedication(ctx context.Context, hospitalID, actorID string, input request.MedicationCatalogRequest, now time.Time) (*response.MedicationCatalog, error) {
	var out response.MedicationCatalog
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	result := r.db.WithContext(ctx).Raw(`
		INSERT INTO hospital_medications (
			hospital_id, code, kfa_code, generic_name, brand_name, dosage_form,
			strength, default_unit, default_route, controlled_category, is_active,
			created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'NONE', ?, ?, ?, ?, ?)
		RETURNING id::text, hospital_id::text, code, kfa_code, generic_name,
		          brand_name, dosage_form, strength, default_unit, default_route,
		          controlled_category, is_active, created_at, updated_at
	`, hospitalID, input.Code, input.KFACode, input.GenericName, input.BrandName,
		input.DosageForm, input.Strength, input.DefaultUnit, input.DefaultRoute,
		active, actorID, actorID, now, now).Scan(&out)
	return &out, result.Error
}

func (r *Repository) UpdateMedication(ctx context.Context, hospitalID, medicationID, actorID string, input request.MedicationCatalogRequest, now time.Time) (*response.MedicationCatalog, error) {
	var out response.MedicationCatalog
	result := r.db.WithContext(ctx).Raw(`
		UPDATE hospital_medications SET code = ?, kfa_code = ?, generic_name = ?,
		       brand_name = ?, dosage_form = ?, strength = ?, default_unit = ?,
		       default_route = ?, is_active = COALESCE(?, is_active), updated_by = ?, updated_at = ?
		WHERE id = ? AND hospital_id = ?
		RETURNING id::text, hospital_id::text, code, kfa_code, generic_name,
		          brand_name, dosage_form, strength, default_unit, default_route,
		          controlled_category, is_active, created_at, updated_at
	`, input.Code, input.KFACode, input.GenericName, input.BrandName, input.DosageForm,
		input.Strength, input.DefaultUnit, input.DefaultRoute, input.IsActive, actorID, now,
		medicationID, hospitalID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrMedicationNotFound
	}
	return &out, nil
}

func (r *Repository) ListMedications(ctx context.Context, hospitalID, search string, includeInactive bool) ([]response.MedicationCatalog, error) {
	rows := make([]response.MedicationCatalog, 0)
	whereActive := " AND is_active = TRUE"
	if includeInactive {
		whereActive = ""
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(search)) + "%"
	err := r.db.WithContext(ctx).Raw(`
		SELECT id::text, hospital_id::text, code, kfa_code, generic_name, brand_name,
		       dosage_form, strength, default_unit, default_route,
		       controlled_category, is_active, created_at, updated_at
		FROM hospital_medications
		WHERE hospital_id = ?`+whereActive+`
		  AND (? = '%%' OR LOWER(CONCAT_WS(' ', code, kfa_code, generic_name, brand_name, dosage_form, strength)) LIKE ?)
		ORDER BY generic_name, brand_name NULLS FIRST LIMIT 100
	`, hospitalID, pattern, pattern).Scan(&rows).Error
	return rows, err
}

func (r *Repository) GetMedication(ctx context.Context, hospitalID, medicationID string) (*response.MedicationCatalog, error) {
	var out response.MedicationCatalog
	result := r.db.WithContext(ctx).Raw(`
		SELECT id::text, hospital_id::text, code, kfa_code, generic_name, brand_name,
		       dosage_form, strength, default_unit, default_route,
		       controlled_category, is_active, created_at, updated_at
		FROM hospital_medications WHERE id = ? AND hospital_id = ? AND is_active = TRUE
	`, medicationID, hospitalID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrMedicationNotFound
	}
	return &out, nil
}

func (r *Repository) SaveDraft(ctx context.Context, appointment *AppointmentContext, doctorID, prescriptionNumber string, input request.PrescriptionDraftRequest, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAppointmentForPrescription(tx, appointment.AppointmentID, doctorID); err != nil {
			return err
		}
		var current struct {
			ID                string
			Status            string
			CurrentRevisionID *string
		}
		result := tx.Raw(`SELECT id::text, status, current_revision_id::text FROM prescriptions WHERE encounter_id = ? FOR UPDATE`, appointment.EncounterID).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Raw(`
				INSERT INTO prescriptions (encounter_id, prescription_number, status, created_at, updated_at)
				VALUES (?, ?, 'DRAFT', ?, ?) RETURNING id::text
			`, appointment.EncounterID, prescriptionNumber, now, now).Scan(&current.ID).Error; err != nil {
				return err
			}
			current.Status = "DRAFT"
		}
		if current.Status == "ISSUED" || current.Status == "CANCELLED" {
			return ErrInvalidState
		}

		var revisionID string
		if current.Status == "DRAFT" && current.CurrentRevisionID != nil {
			if err := tx.Exec(`UPDATE prescriptions SET current_revision_id = NULL WHERE id = ?`, current.ID).Error; err != nil {
				return err
			}
			if err := deleteDraftItems(tx, *current.CurrentRevisionID); err != nil {
				return err
			}
			if err := tx.Exec(`DELETE FROM prescription_revisions WHERE id = ? AND status = 'DRAFT'`, *current.CurrentRevisionID).Error; err != nil {
				return err
			}
		}
		const version = 1
		if err := tx.Raw(`
			INSERT INTO prescription_revisions (
				prescription_id, version, status, general_note, repeats_allowed,
				authored_by, created_at, updated_at
			) VALUES (?, ?, 'DRAFT', ?, ?, ?, ?, ?) RETURNING id::text
		`, current.ID, version, input.GeneralNote, input.RepeatsAllowed, doctorID, now, now).Scan(&revisionID).Error; err != nil {
			return err
		}
		if err := insertItems(tx, revisionID, input.Items, now); err != nil {
			return err
		}
		if err := tx.Exec(`
			UPDATE prescriptions SET status = 'DRAFT', current_revision_id = ?,
			       no_medication_reason = NULL, updated_at = ? WHERE id = ?
		`, revisionID, now, current.ID).Error; err != nil {
			return err
		}
		return insertAudit(tx, current.ID, &revisionID, doctorID, "PRESCRIPTION_DRAFT_SAVED", map[string]any{"version": version}, now)
	})
}

func (r *Repository) MarkNoMedication(ctx context.Context, appointment *AppointmentContext, doctorID, prescriptionNumber, reason string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAppointmentForPrescription(tx, appointment.AppointmentID, doctorID); err != nil {
			return err
		}
		var current struct {
			ID                string
			Status            string
			CurrentRevisionID *string
		}
		result := tx.Raw(`SELECT id::text, status, current_revision_id::text FROM prescriptions WHERE encounter_id = ? FOR UPDATE`, appointment.EncounterID).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Raw(`
				INSERT INTO prescriptions (encounter_id, prescription_number, status, no_medication_reason, created_at, updated_at)
				VALUES (?, ?, 'NO_MEDICATION', ?, ?, ?) RETURNING id::text
			`, appointment.EncounterID, prescriptionNumber, reason, now, now).Scan(&current.ID).Error; err != nil {
				return err
			}
		} else {
			if current.Status == "ISSUED" || current.Status == "CANCELLED" {
				return ErrInvalidState
			}
			if current.CurrentRevisionID != nil {
				if err := tx.Exec(`UPDATE prescriptions SET current_revision_id = NULL WHERE id = ?`, current.ID).Error; err != nil {
					return err
				}
				if err := deleteDraftItems(tx, *current.CurrentRevisionID); err != nil {
					return err
				}
				if err := tx.Exec(`DELETE FROM prescription_revisions WHERE id = ? AND status = 'DRAFT'`, *current.CurrentRevisionID).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(`
				UPDATE prescriptions SET status = 'NO_MEDICATION', current_revision_id = NULL,
				       no_medication_reason = ?, updated_at = ? WHERE id = ?
			`, reason, now, current.ID).Error; err != nil {
				return err
			}
		}
		return insertAudit(tx, current.ID, nil, doctorID, "NO_MEDICATION_RECORDED", nil, now)
	})
}

func (r *Repository) Issue(ctx context.Context, prescriptionID, revisionID, doctorID, tokenHash string, snapshot ClinicalSnapshot, document DocumentInput, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status, CurrentRevisionID string }
		result := tx.Raw(`SELECT status, current_revision_id::text FROM prescriptions WHERE id = ? FOR UPDATE`, prescriptionID).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if current.Status != "DRAFT" || current.CurrentRevisionID != revisionID {
			return ErrConcurrentUpdate
		}
		result = tx.Exec(`
			UPDATE prescription_revisions SET status = 'ISSUED', allergies_reviewed = TRUE,
			       issued_by = ?, issued_at = ?, verification_token_hash = ?,
			       patient_name_snapshot = ?, patient_date_of_birth_snapshot = ?,
			       patient_gender_snapshot = ?, patient_allergies_snapshot = ?,
			       hospital_name_snapshot = ?, hospital_address_snapshot = ?,
			       hospital_phone_snapshot = ?, doctor_name_snapshot = ?,
			       doctor_sip_number_snapshot = ?, updated_at = ?
			WHERE id = ? AND status = 'DRAFT'
		`, doctorID, now, tokenHash, snapshot.PatientName, snapshot.PatientDateOfBirth,
			snapshot.PatientGender, snapshot.PatientAllergies, snapshot.HospitalName,
			snapshot.HospitalAddress, snapshot.HospitalPhone, snapshot.DoctorName,
			snapshot.DoctorSIPNumber, now, revisionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConcurrentUpdate
		}
		if err := insertDocument(tx, revisionID, document, now); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE prescriptions SET status = 'ISSUED', updated_at = ? WHERE id = ?`, now, prescriptionID).Error; err != nil {
			return err
		}
		return insertAudit(tx, prescriptionID, &revisionID, doctorID, "PRESCRIPTION_ISSUED", nil, now)
	})
}

func (r *Repository) Correct(ctx context.Context, current *response.Prescription, doctorID, tokenHash string, input request.CorrectPrescriptionRequest, snapshot ClinicalSnapshot, document DocumentInput, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked struct{ Status, CurrentRevisionID string }
		result := tx.Raw(`SELECT status, current_revision_id::text FROM prescriptions WHERE id = ? FOR UPDATE`, current.ID).Scan(&locked)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if locked.Status != "ISSUED" || current.CurrentRevision == nil || locked.CurrentRevisionID != current.CurrentRevision.ID {
			return ErrConcurrentUpdate
		}
		version := current.CurrentRevision.Version + 1
		var revisionID string
		if err := tx.Raw(`
			INSERT INTO prescription_revisions (
				prescription_id, version, status, general_note, allergies_reviewed,
				repeats_allowed, authored_by, supersedes_revision_id, correction_reason,
				created_at, updated_at
			) VALUES (?, ?, 'DRAFT', ?, TRUE, ?, ?, ?, ?, ?, ?) RETURNING id::text
		`, current.ID, version, input.GeneralNote, input.RepeatsAllowed, doctorID,
			current.CurrentRevision.ID, input.CorrectionReason, now, now).Scan(&revisionID).Error; err != nil {
			return err
		}
		if err := insertItems(tx, revisionID, input.Items, now); err != nil {
			return err
		}
		if err := tx.Exec(`
			UPDATE prescription_revisions SET status = 'ISSUED', issued_by = ?, issued_at = ?,
			       verification_token_hash = ?, patient_name_snapshot = ?,
			       patient_date_of_birth_snapshot = ?, patient_gender_snapshot = ?,
			       patient_allergies_snapshot = ?, hospital_name_snapshot = ?,
			       hospital_address_snapshot = ?, hospital_phone_snapshot = ?,
			       doctor_name_snapshot = ?, doctor_sip_number_snapshot = ?,
			       updated_at = ? WHERE id = ? AND status = 'DRAFT'
		`, doctorID, now, tokenHash, snapshot.PatientName, snapshot.PatientDateOfBirth,
			snapshot.PatientGender, snapshot.PatientAllergies, snapshot.HospitalName,
			snapshot.HospitalAddress, snapshot.HospitalPhone, snapshot.DoctorName,
			snapshot.DoctorSIPNumber, now, revisionID).Error; err != nil {
			return err
		}
		if err := insertDocument(tx, revisionID, document, now); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE prescriptions SET current_revision_id = ?, updated_at = ? WHERE id = ?`, revisionID, now, current.ID).Error; err != nil {
			return err
		}
		return insertAudit(tx, current.ID, &revisionID, doctorID, "PRESCRIPTION_CORRECTED", map[string]any{"supersedes_revision_id": current.CurrentRevision.ID}, now)
	})
}

func (r *Repository) Cancel(ctx context.Context, prescriptionID, expectedRevisionID, doctorID, reason string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status, CurrentRevisionID string }
		result := tx.Raw(`SELECT status, current_revision_id::text FROM prescriptions WHERE id = ? FOR UPDATE`, prescriptionID).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if current.Status != "ISSUED" {
			return ErrInvalidState
		}
		if current.CurrentRevisionID != expectedRevisionID {
			return ErrConcurrentUpdate
		}
		result = tx.Exec(`
			UPDATE prescriptions SET status = 'CANCELLED', cancelled_by = ?, cancelled_at = ?,
			       cancellation_reason = ?, updated_at = ?
			WHERE id = ? AND status = 'ISSUED' AND current_revision_id = ?
		`, doctorID, now, reason, now, prescriptionID, expectedRevisionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvalidState
		}
		return insertAudit(tx, prescriptionID, nil, doctorID, "PRESCRIPTION_CANCELLED", nil, now)
	})
}

func (r *Repository) GetByAppointment(ctx context.Context, appointmentID string) (*response.Prescription, error) {
	return r.get(ctx, `encounter.appointment_id = ?`, appointmentID)
}

func (r *Repository) GetForPatient(ctx context.Context, prescriptionID, patientUserID string) (*response.Prescription, error) {
	return r.get(ctx, `prescription.id = ? AND patient.user_id = ? AND encounter.status = 'COMPLETED' AND prescription.status IN ('ISSUED','CANCELLED')`, prescriptionID, patientUserID)
}

func (r *Repository) ListForPatient(ctx context.Context, patientUserID string) ([]response.PrescriptionSummary, error) {
	rows := make([]response.PrescriptionSummary, 0)
	err := r.db.WithContext(ctx).Raw(`
		SELECT prescription.id::text, encounter.id::text AS encounter_id,
		       encounter.appointment_id::text, prescription.prescription_number,
		       prescription.status,
		       COALESCE(NULLIF(revision.hospital_name_snapshot, ''), hospital.name) AS hospital_name,
		       COALESCE(NULLIF(revision.doctor_name_snapshot, ''),
		                CONCAT_WS(' ', doctor.first_name, NULLIF(doctor.last_name, ''))) AS doctor_name,
		       appointment.appointment_date::text,
		       revision.issued_at,
		       (SELECT COUNT(*) FROM prescription_items item WHERE item.revision_id = revision.id)::int AS item_count
		FROM prescriptions prescription
		JOIN medical_encounters encounter ON encounter.id = prescription.encounter_id
		JOIN appointments appointment ON appointment.id = encounter.appointment_id
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		JOIN hospitals hospital ON hospital.id = encounter.hospital_id
		JOIN users doctor ON doctor.id = encounter.doctor_id
		LEFT JOIN prescription_revisions revision ON revision.id = prescription.current_revision_id
		WHERE patient.user_id = ? AND encounter.status = 'COMPLETED'
		  AND prescription.status IN ('ISSUED','CANCELLED')
		ORDER BY COALESCE(revision.issued_at, prescription.updated_at) DESC LIMIT 100
	`, patientUserID).Scan(&rows).Error
	return rows, err
}

func (r *Repository) GetDocument(ctx context.Context, prescriptionID string) (*DocumentRecord, error) {
	var out DocumentRecord
	result := r.db.WithContext(ctx).Raw(`
		SELECT document.id::text, prescription.id::text AS prescription_id,
		       document.revision_id::text, document.bucket, document.object_path,
		       document.filename, document.mime_type, document.file_size,
		       document.sha256, document.generated_at
		FROM prescriptions prescription
		JOIN prescription_documents document ON document.revision_id = prescription.current_revision_id
		WHERE prescription.id = ?
	`, prescriptionID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &out, nil
}

func (r *Repository) Verify(ctx context.Context, tokenHash string) (*response.PrescriptionVerification, error) {
	row, err := r.get(ctx, `revision.verification_token_hash = ? AND prescription.status = 'ISSUED'`, tokenHash)
	if err != nil || row.CurrentRevision == nil || row.CurrentRevision.IssuedAt == nil {
		return nil, ErrNotFound
	}
	if err := insertAudit(r.db.WithContext(ctx), row.ID, &row.CurrentRevision.ID, "", "PRESCRIPTION_PUBLICLY_VERIFIED", nil, time.Now().UTC()); err != nil {
		return nil, err
	}
	return &response.PrescriptionVerification{
		Valid: true, PrescriptionNumber: row.PrescriptionNumber, Status: row.Status,
		PatientName: row.PatientName, HospitalName: row.HospitalName,
		DoctorName: row.DoctorName, DoctorSIPNumber: row.DoctorSIPNumber,
		IssuedAt: *row.CurrentRevision.IssuedAt, Items: row.CurrentRevision.Items,
	}, nil
}

func (r *Repository) LogAccess(ctx context.Context, prescriptionID string, revisionID *string, actorID, action string, now time.Time) error {
	return insertAudit(r.db.WithContext(ctx), prescriptionID, revisionID, actorID, action, nil, now)
}

func (r *Repository) get(ctx context.Context, where string, args ...any) (*response.Prescription, error) {
	var out response.Prescription
	query := `
		SELECT prescription.id::text, prescription.encounter_id::text,
		       encounter.appointment_id::text, prescription.prescription_number,
		       prescription.status, prescription.no_medication_reason,
		       prescription.cancellation_reason, prescription.cancelled_at,
		       encounter.patient_record_id::text, patient.user_id::text AS patient_user_id,
		       CONCAT_WS(' ', patient.first_name, NULLIF(patient.last_name, '')) AS patient_name,
		       patient.dob::text AS patient_date_of_birth, patient.gender AS patient_gender,
		       profile.allergies AS patient_allergies,
		       encounter.hospital_id::text, hospital.name AS hospital_name,
		       hospital.address AS hospital_address, hospital.phone AS hospital_phone,
		       encounter.doctor_id::text,
		       CONCAT_WS(' ', doctor.first_name, NULLIF(doctor.last_name, '')) AS doctor_name,
		       doctor_profile.sip_number AS doctor_sip_number,
		       appointment.appointment_date::text,
		       prescription.created_at, prescription.updated_at
		FROM prescriptions prescription
		JOIN medical_encounters encounter ON encounter.id = prescription.encounter_id
		JOIN appointments appointment ON appointment.id = encounter.appointment_id
		JOIN patient_records patient ON patient.id = encounter.patient_record_id
		LEFT JOIN patient_profiles profile ON profile.user_id = patient.user_id
		JOIN hospitals hospital ON hospital.id = encounter.hospital_id
		JOIN users doctor ON doctor.id = encounter.doctor_id
		LEFT JOIN doctor_profiles doctor_profile ON doctor_profile.user_id = doctor.id
		LEFT JOIN prescription_revisions revision ON revision.id = prescription.current_revision_id
		WHERE ` + where + ` LIMIT 1`
	result := r.db.WithContext(ctx).Raw(query, args...).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	var revisionID *string
	if err := r.db.WithContext(ctx).Raw(`SELECT current_revision_id::text FROM prescriptions WHERE id = ?`, out.ID).Scan(&revisionID).Error; err != nil {
		return nil, err
	}
	if revisionID != nil {
		revision, err := r.loadRevision(ctx, *revisionID)
		if err != nil {
			return nil, err
		}
		out.CurrentRevision = revision
		if revision.Status == "ISSUED" {
			out.PatientName = revision.PatientNameSnapshot
			out.PatientDateOfBirth = revision.PatientDateOfBirthSnapshot
			out.PatientGender = revision.PatientGenderSnapshot
			out.PatientAllergies = revision.PatientAllergiesSnapshot
			out.HospitalName = revision.HospitalNameSnapshot
			out.HospitalAddress = revision.HospitalAddressSnapshot
			out.HospitalPhone = revision.HospitalPhoneSnapshot
			out.DoctorName = revision.DoctorNameSnapshot
			out.DoctorSIPNumber = &revision.DoctorSIPNumberSnapshot
		}
	}
	return &out, nil
}

func (r *Repository) loadRevision(ctx context.Context, revisionID string) (*response.PrescriptionRevision, error) {
	var out response.PrescriptionRevision
	result := r.db.WithContext(ctx).Raw(`
		SELECT revision.id::text, revision.version, revision.status, revision.general_note,
		       revision.allergies_reviewed, revision.repeats_allowed,
		       revision.authored_by::text, revision.issued_by::text, revision.issued_at,
		       revision.supersedes_revision_id::text, revision.correction_reason,
		       revision.patient_name_snapshot, revision.patient_date_of_birth_snapshot::text,
		       revision.patient_gender_snapshot, revision.patient_allergies_snapshot,
		       revision.hospital_name_snapshot, revision.hospital_address_snapshot,
		       revision.hospital_phone_snapshot, revision.doctor_name_snapshot,
		       revision.doctor_sip_number_snapshot,
		       revision.created_at, revision.updated_at
		FROM prescription_revisions revision WHERE revision.id = ?
	`, revisionID).Scan(&out)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	out.Items = make([]response.PrescriptionItem, 0)
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id::text, item_order AS "order", item_type AS type, medication_id::text,
		       medication_name, dosage_form, strength, dose_amount, dose_unit, route,
		       frequency_per_day, interval_hours, timing_instructions, duration_value,
		       duration_unit, quantity, quantity_unit, directions, as_needed,
		       max_daily_dose, controlled_substance, satusehat_medication_request_id
		FROM prescription_items WHERE revision_id = ? ORDER BY item_order
	`, revisionID).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	for index := range out.Items {
		out.Items[index].Components = make([]response.PrescriptionComponent, 0)
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id::text, medication_id::text, medication_name, dosage_form,
			       strength, amount, unit, controlled_substance
			FROM prescription_item_components WHERE prescription_item_id = ? ORDER BY component_order
		`, out.Items[index].ID).Scan(&out.Items[index].Components).Error; err != nil {
			return nil, err
		}
	}
	var document response.PrescriptionDocument
	result = r.db.WithContext(ctx).Raw(`
		SELECT id::text, filename, mime_type, file_size, sha256, generated_at
		FROM prescription_documents WHERE revision_id = ?
	`, revisionID).Scan(&document)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		out.Document = &document
	}
	return &out, nil
}

func deleteDraftItems(tx *gorm.DB, revisionID string) error {
	if err := tx.Exec(`
		DELETE FROM prescription_item_components component USING prescription_items item
		WHERE component.prescription_item_id = item.id AND item.revision_id = ?
	`, revisionID).Error; err != nil {
		return err
	}
	return tx.Exec(`DELETE FROM prescription_items WHERE revision_id = ?`, revisionID).Error
}

func lockAppointmentForPrescription(tx *gorm.DB, appointmentID, doctorID string) error {
	var status string
	result := tx.Raw(`
		SELECT status FROM appointments WHERE id = ? AND doctor_id = ? FOR UPDATE
	`, appointmentID, doctorID).Scan(&status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	if status != "IN_CONSULTATION" {
		return ErrInvalidState
	}
	return nil
}

func insertItems(tx *gorm.DB, revisionID string, items []request.PrescriptionItemRequest, now time.Time) error {
	for index, item := range items {
		var itemID string
		if err := tx.Raw(`
			INSERT INTO prescription_items (
				revision_id, item_order, item_type, medication_id, medication_name,
				dosage_form, strength, dose_amount, dose_unit, route, frequency_per_day,
				interval_hours, timing_instructions, duration_value, duration_unit,
				quantity, quantity_unit, directions, as_needed, max_daily_dose,
				controlled_substance, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id::text
		`, revisionID, index+1, item.Type, item.MedicationID, item.MedicationName,
			item.DosageForm, item.Strength, item.DoseAmount, item.DoseUnit, item.Route,
			item.FrequencyPerDay, item.IntervalHours, item.TimingInstructions,
			item.DurationValue, item.DurationUnit, item.Quantity, item.QuantityUnit,
			item.Directions, item.AsNeeded, item.MaxDailyDose, item.ControlledSubstance,
			now).Scan(&itemID).Error; err != nil {
			return err
		}
		for componentIndex, component := range item.Components {
			if err := tx.Exec(`
				INSERT INTO prescription_item_components (
					prescription_item_id, component_order, medication_id, medication_name,
					dosage_form, strength, amount, unit, controlled_substance, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, itemID, componentIndex+1, component.MedicationID, component.MedicationName,
				component.DosageForm, component.Strength, component.Amount, component.Unit,
				component.ControlledSubstance, now).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func insertDocument(tx *gorm.DB, revisionID string, document DocumentInput, now time.Time) error {
	return tx.Exec(`
		INSERT INTO prescription_documents (
			revision_id, bucket, object_path, filename, mime_type, file_size,
			sha256, generated_at
		) VALUES (?, ?, ?, ?, 'application/pdf', ?, ?, ?)
	`, revisionID, document.Bucket, document.ObjectPath, document.Filename,
		document.FileSize, document.SHA256, now).Error
}

func insertAudit(tx *gorm.DB, prescriptionID string, revisionID *string, actorID, action string, metadata any, now time.Time) error {
	payload := []byte("{}")
	if metadata != nil {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		payload = encoded
	}
	var actor any
	if strings.TrimSpace(actorID) != "" {
		actor = actorID
	}
	return tx.Exec(`
		INSERT INTO prescription_audit_events (
			prescription_id, revision_id, actor_id, action, metadata, created_at
		) VALUES (?, ?, ?, ?, ?::jsonb, ?)
	`, prescriptionID, revisionID, actor, action, string(payload), now).Error
}
