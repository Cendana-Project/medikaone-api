package seeder

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"

	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	appointmentsvc "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
)

// DemoManifest is a native Bruno environment. It contains public demo account
// defaults and fixture IDs, never database/storage credentials or issued tokens.
type DemoManifest struct {
	Name      string         `yaml:"name"`
	Variables []DemoVariable `yaml:"variables"`
}

type DemoVariable struct {
	Name        string `yaml:"name"`
	Value       string `yaml:"value"`
	Description string `yaml:"description,omitempty"`
}

// WriteBrunoEnvironment overwrites only the explicitly requested output file.
// Keep generated environments local: they identify a particular demo database.
func (manifest *DemoManifest) WriteBrunoEnvironment(path string) error {
	encoded, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode demo Bruno environment: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create demo environment directory: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		return fmt.Errorf("write demo Bruno environment: %w", err)
	}
	return nil
}

func buildDemoManifest(state *workflowSeedContext) (*DemoManifest, error) {
	manifest := &DemoManifest{Name: "MedikaOne Demo", Variables: make([]DemoVariable, 0, 60)}
	put := func(name, value, description string) {
		manifest.Variables = append(manifest.Variables, DemoVariable{Name: name, Value: value, Description: description})
	}
	put("baseUrl", state.BaseURL, "API deployment matching this seeded database; without trailing slash")
	put("mobileIdentity", "patient001@medikaone.id", "Switch to doctorIdentity for doctor actions; login again after switching")
	put("mobilePassword", "Password123", "Public non-production fixture password")
	put("websiteIdentifier", "admin001@medikaone.id", "Switch to nurseIdentity/receptionistIdentity for their website actions")
	put("websitePassword", "Password123", "Public non-production fixture password")
	put("doctorIdentity", "doctor001@medikaone.id", "Doctor for the default clinical scenarios")
	put("eyeDoctorIdentity", "doctor002@medikaone.id", "Doctor with pending schedule proposal")
	put("patientIdentity", "patient001@medikaone.id", "Patient for default booking and completed no-medication history")
	put("completedPrescriptionPatientIdentity", "patient002@medikaone.id", "Patient who owns the completed issued-prescription scenario")
	put("nurseIdentity", "nurse001@medikaone.id", "Website nurse at HSP-MO-001")
	put("receptionistIdentity", "receptionist001@medikaone.id", "Website receptionist at HSP-MO-001")
	put("superadminIdentity", "superadmin@medikaone.id", "Password123 unless SUPERADMIN_PASSWORD was explicitly overridden; no configured secret is exported")
	put("hospitalCode", "HSP-MO-001", "Default website tenant")
	put("hospitalId", state.Hospitals["jakarta"], "Actual Jakarta hospital UUID")
	put("alternateHospitalCode", "HSP-MO-002", "Bandung cross-hospital practice")
	put("alternateHospitalId", state.Hospitals["bandung"], "Actual Bandung hospital UUID")
	put("doctorId", state.Users["doctor1"], "Default fixed-slot doctor")
	put("eyeDoctorId", state.Users["doctor2"], "Mata doctor and pending schedule change")
	put("queueDoctorId", state.Users["doctor3"], "Session-queue doctor")
	put("patientMedikaOneId", state.Users["patient1"], "Registered patient account UUID for walk-in lookup")
	var doctor struct {
		MedikaoneID string
		SpecialtyID string
		Code        string
	}
	if err := state.DB.Raw(`SELECT profile.medikaone_id, profile.specialty_id, master.code
		FROM doctor_profiles profile JOIN master_specialties master ON master.id=profile.specialty_id
		WHERE profile.user_id=?`, state.Users["doctor1"]).Scan(&doctor).Error; err != nil {
		return nil, fmt.Errorf("read demo doctor identifiers: %w", err)
	}
	put("doctorMedikaOneId", doctor.MedikaoneID, "Immutable public identity, not a credential")
	put("specialtyId", doctor.SpecialtyID, "Actual specialty master UUID")
	put("specialtyCode", doctor.Code, "Local MedikaOne specialty code")
	var patientNIK string
	if err := state.DB.Raw(`SELECT nik FROM users WHERE id=?`, state.Users["patient1"]).Scan(&patientNIK).Error; err != nil {
		return nil, err
	}
	put("patientIdentityNumber", patientNIK, "Fictitious NIK belonging to patient001 fixture")
	put("demoAnchorDate", state.Anchor.In(state.Location).Format("2006-01-02"), "Original scenario day; reruns preserve this day and all clinical progress")
	put("demoTodayDate", state.Now.In(state.Location).Format("2006-01-02"), "Current local day; use demoAnchorDate for seeded day-specific queues")

	for _, item := range []struct{ variable, alias, description string }{
		{"affiliationId", "affiliation.general.jakarta", "Doctor001 Jakarta placement"},
		{"alternateAffiliationId", "affiliation.general.bandung", "Doctor001 Bandung placement"},
		{"invitationId", "invitation.pending", "Doctor003 invitation at Bandung; current state is preserved on rerun"},
		{"specificScheduleId", "schedule.specific", "Doctor001 one-off schedule"},
		{"scheduleChangeId", "schedule_change.pending", "Doctor002 counterpart ADD proposal; current state is preserved"},
		{"appointmentId", "appointment.confirmed", "Default patient001 fixture, independent of variables for a new booking"},
		{"confirmedAppointmentId", "appointment.confirmed", "Initially CONFIRMED on the day after demoAnchorDate"},
		{"waitingVitalsAppointmentId", "appointment.waiting_vitals", "Initially WAITING_VITALS for patient002"},
		{"waitingDoctorAppointmentId", "appointment.waiting_doctor", "Initially WAITING_DOCTOR for patient003"},
		{"inConsultationAppointmentId", "appointment.in_consultation", "Initially IN_CONSULTATION for patient001 with a draft prescription"},
		{"completedAppointmentId", "appointment.completed_no_medication", "Patient001 completed encounter with NO_MEDICATION"},
		{"completedPrescriptionAppointmentId", "appointment.completed_prescription", "Patient002 completed encounter with issued PDF prescription"},
		{"cancelledAppointmentId", "appointment.cancelled", "Initially CANCELLED patient003 appointment"},
		{"queueAppointmentId", "appointment.queue", "Patient003 SESSION_QUEUE appointment"},
		{"medicalEncounterId", "encounter.completed_no_medication", "Patient001 completed medical history"},
		{"completedPrescriptionEncounterId", "encounter.completed_prescription", "Patient002 medical history with issued prescription"},
		{"prescriptionId", "prescription.issued", "Genuine PDF-backed issued prescription"},
		{"prescriptionRevisionId", "prescription.revision", "Current issued prescription revision"},
		{"draftPrescriptionId", "prescription.draft", "Prescription for the active consultation; later API changes are preserved"},
		{"medicationId", "medication.paracetamol", "Local demo medication; not an external KFA identifier"},
	} {
		id := state.IDs[item.alias]
		if id == "" {
			return nil, fmt.Errorf("demo workflow omitted required fixture identity %s", item.alias)
		}
		put(item.variable, id, item.description)
	}
	put("issuedPrescriptionId", state.IDs["prescription.issued"], "Stable issued fixture selection; unlike prescriptionId, request scripts do not overwrite this variable")
	put("pendingInvitationDoctorIdentity", "doctor003@medikaone.id", "Login mobile with this actor to accept/reject invitationId")
	put("pendingInvitationDoctorId", state.Users["doctor3"], "Doctor invited to Bandung")
	put("pendingInvitationHospitalId", state.Hospitals["bandung"], "Use SUPER_ADMIN hospital selection for website management")
	var placement struct{ DepartmentID, RoomID string }
	if err := state.DB.Raw(`SELECT department_id, COALESCE(room_id::text,'') AS room_id FROM doctor_hospital_affiliations WHERE id=?`, state.IDs["affiliation.general.jakarta"]).Scan(&placement).Error; err != nil {
		return nil, err
	}
	put("departmentId", placement.DepartmentID, "Actual Jakarta POLI-UMUM placement, not master department UUID")
	put("roomId", placement.RoomID, "Actual Jakarta UMUM-01 room")
	var specificDate string
	if err := state.DB.Raw(`SELECT schedule_date::text FROM doctor_hospital_schedules WHERE id=?`, state.IDs["schedule.specific"]).Scan(&specificDate).Error; err != nil {
		return nil, err
	}
	put("specificScheduleDate", specificDate, "Original specific schedule date; not rolled forward by seeding")
	var appointment struct{ AppointmentNumber, PatientRecordID string }
	if err := state.DB.Raw(`SELECT appointment_number, patient_record_id FROM appointments WHERE id=?`, state.IDs["appointment.confirmed"]).Scan(&appointment).Error; err != nil {
		return nil, err
	}
	put("appointmentNumber", appointment.AppointmentNumber, "Read patient appointment detail to obtain its runtime verification/QR data")
	put("patientRecordId", appointment.PatientRecordID, "Patient001 longitudinal record")
	var notificationID string
	if err := state.DB.Raw(`SELECT id FROM notifications WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, state.Users["doctor3"]).Scan(&notificationID).Error; err != nil {
		return nil, err
	}
	put("notificationId", notificationID, "Actual latest doctor003 notification, if still present")

	// New-booking variables must reflect real current availability, not a stale
	// fixture date or an assumed first slot. A consumed/deactivated seed is not reset.
	date := state.Now.In(state.Location).AddDate(0, 0, 1).Format("2006-01-02")
	availability, err := appointmentsvc.NewService(appointmentrepo.NewRepository(state.DB), nil, "").ListAvailability(state.Context, state.Hospitals["jakarta"], state.Users["doctor1"], date, date)
	if err != nil {
		return nil, fmt.Errorf("resolve current demo booking choice: %w", err)
	}
	var slotID, scheduleID, startTime, bookingDate string
	for _, session := range availability {
		if len(session.Slots) == 0 {
			continue
		}
		slotID, scheduleID, bookingDate = session.Slots[0].SlotID, session.ScheduleID, session.Date
		startTime = session.Slots[0].StartAt.In(state.Location).Format("15:04")
		break
	}
	put("demoBookingAvailable", strconv.FormatBool(slotID != ""), "False means no current seed slot is bookable; inspect availability rather than reusing old variables")
	put("slotId", slotID, "Opaque current availability choice for a NEW scheduled booking; refresh inquiry before reuse")
	put("scheduleId", scheduleID, "Schedule metadata for the current choice; also used by separate walk-in requests")
	put("appointmentDate", bookingDate, "Display date for the current choice, not the date of appointmentId")
	put("appointmentStartTime", startTime, "Display local start time of actual available slot; Asia/Jakarta")
	return manifest, nil
}
