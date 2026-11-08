package seeder

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"

	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorhospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
)

type demoPractice struct {
	alias, doctor, hospital, department, room string
	start, end, mode                          string
	firstDay, lastDay, capacity               int
	pending                                   bool
}

func demoPractices() []demoPractice {
	return []demoPractice{
		{"general.jakarta", "doctor1", "jakarta", "POLI-UMUM", "UMUM-01", "08:00", "12:00", "FIXED_SLOT", 0, 6, 1, false},
		{"general.bandung", "doctor1", "bandung", "POLI-UMUM", "UMUM-01", "15:00", "18:00", "FIXED_SLOT", 1, 5, 2, false},
		{"eye.jakarta", "doctor2", "jakarta", "POLI-MATA", "MATA-01", "13:00", "17:00", "FIXED_SLOT", 0, 6, 1, false},
		{"lung.jakarta", "doctor3", "jakarta", "POLI-PARU", "PARU-01", "09:00", "12:00", "SESSION_QUEUE", 0, 6, 3, false},
		{"pending", "doctor3", "bandung", "POLI-UMUM", "UMUM-01", "", "", "", 0, 0, 0, true},
	}
}

// seedDemoPractices is called inside the workflow transaction. Only newly
// created invitations are accepted; existing lifecycle and calendar state is
// never replayed, including expired invitations and inactive schedules.
func seedDemoPractices(s *workflowSeedContext) error {
	if s.ContractStorage == nil {
		return fmt.Errorf("demo practices require configured contract storage")
	}
	if s.IDs == nil {
		s.IDs = make(map[string]string)
	}
	var origin struct {
		CreatedAt time.Time
	}
	if err := s.DB.WithContext(s.Context).Raw(`SELECT created_at FROM doctor_hospital_invitations WHERE id = ?`, workflowUUID("invitation.general.jakarta")).Scan(&origin).Error; err != nil {
		return err
	}
	s.Anchor = origin.CreatedAt
	if s.Anchor.IsZero() {
		s.Anchor = s.Now.In(s.Location)
	}
	for _, fixture := range demoPractices() {
		if err := demoSeedPractice(s, fixture); err != nil {
			return fmt.Errorf("seed practice %s: %w", fixture.alias, err)
		}
	}
	if err := demoSeedScheduleProposal(s, "specific", "general.jakarta", "doctor1", "12:30", "13:30", true); err != nil {
		return err
	}
	return demoSeedScheduleProposal(s, "pending", "eye.jakarta", "doctor2", "18:00", "19:00", false)
}

func demoSeedPractice(s *workflowSeedContext, fixture demoPractice) error {
	db := s.DB.WithContext(s.Context)
	hospitalID, err := uuid.Parse(s.Hospitals[fixture.hospital])
	if err != nil {
		return fmt.Errorf("fixture hospital missing: %w", err)
	}
	departmentID := uuid.NewSHA1(hospitalID, []byte("medikaone:department:"+fixture.department)).String()
	roomID := uuid.NewSHA1(hospitalID, []byte("medikaone:room:"+fixture.room)).String()
	inviter := s.Users["admin"]
	if fixture.hospital == "bandung" {
		inviter = s.Users["superadmin"]
	}
	invitationID := workflowUUID("invitation." + fixture.alias)
	var existing struct {
		ID, HospitalID, DoctorID, DepartmentID, RoomID, InvitedBy string
		CreatedAt                                                 time.Time
	}
	if err := db.Raw(`SELECT id, hospital_id, doctor_id, department_id, COALESCE(room_id::text, '') AS room_id, invited_by, created_at
		FROM doctor_hospital_invitations WHERE id = ?`, invitationID).Scan(&existing).Error; err != nil {
		return err
	}
	created := existing.ID == ""
	if !created {
		if existing.HospitalID != hospitalID.String() || existing.DoctorID != s.Users[fixture.doctor] || existing.DepartmentID != departmentID || existing.RoomID != roomID || existing.InvitedBy != inviter {
			return fmt.Errorf("fixture invitation ID collision: %s", invitationID)
		}
	} else {
		content, err := demoContractPDF(fixture, s.Users[fixture.doctor], hospitalID.String(), s.Anchor)
		if err != nil {
			return err
		}
		objectPath := fmt.Sprintf("hospitals/%s/doctor-invitations/%s/original/%s.pdf", hospitalID, invitationID, uuid.NewString())
		object, err := s.ContractStorage.Upload(s.Context, objectPath, "application/pdf", content)
		if err != nil {
			return fmt.Errorf("upload original contract: %w", err)
		}
		if object == nil || object.Bucket == "" || object.ObjectPath != objectPath || object.FileSize != int64(len(content)) {
			return fmt.Errorf("contract storage returned inconsistent uploaded object metadata")
		}
		var schedules []doctorhospitalrepo.Schedule
		if !fixture.pending {
			schedules = make([]doctorhospitalrepo.Schedule, 0, fixture.lastDay-fixture.firstDay+1)
			for day := fixture.firstDay; day <= fixture.lastDay; day++ {
				schedules = append(schedules, doctorhospitalrepo.Schedule{DayOfWeek: day, StartTime: fixture.start, EndTime: fixture.end, Timezone: s.Location.String(), BookingMode: fixture.mode, SlotDurationMinutes: 30, Capacity: fixture.capacity})
			}
		}
		message := "MedikaOne demo workflow: " + fixture.alias
		_, err = doctorhospitalrepo.NewRepository(s.DB).CreateInvitation(s.Context, doctorhospitalrepo.CreateInvitationInput{
			InvitationID: invitationID, HospitalID: hospitalID.String(), DoctorID: s.Users[fixture.doctor], DepartmentID: departmentID,
			RoomID: &roomID, InvitedBy: inviter, Message: &message, ExpiresAt: s.Anchor.AddDate(0, 0, 7), Now: s.Anchor,
			Contract:  doctorhospitalrepo.Document{Filename: "demo-practice-" + fixture.alias + ".pdf", MIMEType: "application/pdf", Bucket: object.Bucket, ObjectPath: object.ObjectPath, FileSize: object.FileSize, SHA256: fmt.Sprintf("%x", sha256.Sum256(content))},
			Schedules: schedules,
		})
		if err != nil {
			return err
		}
		if !fixture.pending {
			if err := doctorhospitalrepo.NewRepository(s.DB).AcceptInvitation(s.Context, invitationID, s.Users[fixture.doctor], s.Anchor); err != nil {
				return err
			}
		}
	}
	s.IDs["invitation."+fixture.alias] = invitationID
	var contract struct {
		ID, Filename, MIMEType, Bucket, ObjectPath, SHA256 string
		FileSize                                           int64
	}
	if err := db.Raw(`SELECT id, original_filename AS filename, original_mime_type AS mime_type,
		original_bucket AS bucket, original_object_path AS object_path, original_file_size AS file_size, original_sha256 AS sha256
		FROM doctor_hospital_contracts WHERE invitation_id = ?`, invitationID).Scan(&contract).Error; err != nil {
		return err
	}
	objectPrefix := fmt.Sprintf("hospitals/%s/doctor-invitations/%s/original/", hospitalID, invitationID)
	if contract.ID == "" || contract.Filename == "" || contract.MIMEType != "application/pdf" || contract.Bucket == "" ||
		!strings.HasPrefix(contract.ObjectPath, objectPrefix) || contract.FileSize <= 0 || contract.FileSize > 10<<20 || len(contract.SHA256) != sha256.Size*2 {
		return fmt.Errorf("fixture invitation %s has missing or invalid original contract metadata", invitationID)
	}
	s.IDs["contract."+fixture.alias] = contract.ID
	if fixture.alias == "general.jakarta" {
		var origin struct{ CreatedAt time.Time }
		if err := db.Raw(`SELECT created_at FROM doctor_hospital_invitations WHERE id = ?`, invitationID).Scan(&origin).Error; err != nil {
			return err
		}
		s.Anchor = origin.CreatedAt.In(s.Location)
	}
	var affiliations []struct {
		ID, HospitalID, DoctorID, DepartmentID, RoomID string
		CreatedAt                                      time.Time
	}
	if err := db.Raw(`SELECT id, hospital_id, doctor_id, department_id, COALESCE(room_id::text, '') AS room_id, created_at
		FROM doctor_hospital_affiliations WHERE invitation_id = ?`, invitationID).Scan(&affiliations).Error; err != nil {
		return err
	}
	if len(affiliations) == 0 {
		if fixture.pending {
			return nil
		}
		return fmt.Errorf("fixture invitation %s has no affiliation; existing invitation state was preserved", invitationID)
	}
	if len(affiliations) != 1 {
		return fmt.Errorf("fixture invitation has ambiguous affiliations")
	}
	affiliation := affiliations[0]
	if affiliation.HospitalID != hospitalID.String() || affiliation.DoctorID != s.Users[fixture.doctor] || affiliation.DepartmentID != departmentID || affiliation.RoomID != roomID {
		return fmt.Errorf("fixture affiliation actor/place collision: %s", affiliation.ID)
	}
	s.IDs["affiliation."+fixture.alias] = affiliation.ID
	if fixture.pending {
		return nil // An invitation accepted by a tester remains accepted, untouched.
	}
	var schedules []struct {
		ID        string
		DayOfWeek int
	}
	// Acceptance inserts exactly one recurring schedule per day at affiliation
	// creation. Do not filter is_active: deactivation must retain fixture identity.
	if err := db.Raw(`SELECT id, day_of_week FROM doctor_hospital_schedules
		WHERE affiliation_id = ? AND created_at = ? AND schedule_date IS NULL ORDER BY day_of_week, id`, affiliation.ID, affiliation.CreatedAt).Scan(&schedules).Error; err != nil {
		return err
	}
	for _, schedule := range schedules {
		key := fmt.Sprintf("schedule.%s.%d", fixture.alias, schedule.DayOfWeek)
		if s.IDs[key] != "" && s.IDs[key] != schedule.ID {
			return fmt.Errorf("ambiguous fixture schedule %s", key)
		}
		s.IDs[key] = schedule.ID
	}
	return nil
}

func demoSeedScheduleProposal(s *workflowSeedContext, alias, practice, doctor, start, end string, approve bool) error {
	reason := "medikaone:demo:schedule-change:" + alias
	var rows []struct {
		ID, AffiliationID, RequestedBy, RequestedByParty, Operation, Status string
		ReviewedAt                                                          *time.Time
	}
	if err := s.DB.WithContext(s.Context).Raw(`SELECT id, affiliation_id, requested_by, requested_by_party, operation, status, reviewed_at
		FROM doctor_schedule_change_requests WHERE reason = ?`, reason).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) > 1 {
		return fmt.Errorf("ambiguous fixture schedule change %s", alias)
	}
	repo := appointmentrepo.NewRepository(s.DB)
	if len(rows) == 0 {
		date := s.Anchor.In(s.Location).AddDate(0, 0, 3).Format("2006-01-02")
		change, err := repo.CreateScheduleChange(s.Context, appointmentrepo.ScheduleChangeInput{
			Operation: "ADD", AffiliationID: s.IDs["affiliation."+practice], ActorID: s.Users[doctor], ActorParty: "DOCTOR", Reason: &reason,
			Now: s.Anchor, ExpiresAt: s.Anchor.AddDate(0, 0, 7),
			Schedules: []appointmentrepo.ScheduleItem{{DayOfWeek: int(s.Anchor.In(s.Location).AddDate(0, 0, 3).Weekday()), ScheduleDate: &date,
				StartTime: start, EndTime: end, Timezone: s.Location.String(), BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}},
		})
		if err != nil {
			return fmt.Errorf("create demo %s schedule proposal: %w", alias, err)
		}
		s.IDs["schedule_change."+alias] = change.ID
		if approve {
			if err := repo.ReviewScheduleChange(s.Context, change.ID, s.Users["admin"], "HOSPITAL", "APPROVED", nil, s.Anchor); err != nil {
				return fmt.Errorf("approve demo specific schedule: %w", err)
			}
		}
		// Reuse the same resolution path as reseeding; no transitions are replayed.
		return demoSeedScheduleProposal(s, alias, practice, doctor, start, end, approve)
	}
	change := rows[0]
	if change.AffiliationID != s.IDs["affiliation."+practice] || change.RequestedBy != s.Users[doctor] || change.RequestedByParty != "DOCTOR" || change.Operation != "ADD" {
		return fmt.Errorf("fixture schedule change actor/place collision: %s", change.ID)
	}
	s.IDs["schedule_change."+alias] = change.ID
	if approve && change.Status == "APPROVED" && change.ReviewedAt != nil {
		var schedules []struct{ ID string }
		if err := s.DB.WithContext(s.Context).Raw(`SELECT id FROM doctor_hospital_schedules
			WHERE affiliation_id = ? AND created_at = ? AND schedule_date IS NOT NULL`, change.AffiliationID, *change.ReviewedAt).Scan(&schedules).Error; err != nil {
			return err
		}
		if len(schedules) != 1 {
			return fmt.Errorf("approved demo specific schedule identity is missing or ambiguous")
		}
		s.IDs["schedule.specific"] = schedules[0].ID
	}
	return nil
}

func demoContractPDF(fixture demoPractice, doctorID, hospitalID string, anchor time.Time) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.MultiCell(0, 9, "MedikaOne Demonstration Practice Agreement", "", "L", false)
	pdf.Ln(5)
	pdf.SetFont("Arial", "", 11)
	lines := []string{
		"DEMONSTRATION DATA - not an executed legal agreement.",
		"Scenario: " + fixture.alias,
		"Doctor fixture ID: " + doctorID,
		"Hospital fixture ID: " + hospitalID,
		"Department: " + fixture.department + " / Room: " + fixture.room,
		"Issued: " + anchor.Format(time.RFC3339),
		"Timezone: Asia/Jakarta",
	}
	if fixture.pending {
		lines = append(lines, "Invitation awaiting doctor response. No bookable practice schedule is granted.")
	} else if fixture.mode == "SESSION_QUEUE" {
		lines = append(lines, fmt.Sprintf("Recurring practice: %s through %s, %s-%s; session queue with capacity %d for the complete session.", time.Weekday(fixture.firstDay), time.Weekday(fixture.lastDay), fixture.start, fixture.end, fixture.capacity))
	} else {
		lines = append(lines, fmt.Sprintf("Recurring practice: %s through %s, %s-%s; %s; capacity %d; 30-minute slots.", time.Weekday(fixture.firstDay), time.Weekday(fixture.lastDay), fixture.start, fixture.end, fixture.mode, fixture.capacity))
	}
	lines = append(lines, "This original PDF is retained for invitation and affiliation contract downloads in the demo environment.")
	for _, line := range lines {
		pdf.MultiCell(0, 7, line, "", "L", false)
		pdf.Ln(2)
	}
	var content bytes.Buffer
	if err := pdf.Output(&content); err != nil {
		return nil, fmt.Errorf("render demo practice contract: %w", err)
	}
	return content.Bytes(), nil
}
