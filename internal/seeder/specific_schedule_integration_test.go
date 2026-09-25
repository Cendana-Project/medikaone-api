package seeder

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorhospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func runSpecificScheduleIntegration(t *testing.T, db *gorm.DB, sqlDB *sql.DB) {
	if err := ResetAllAndSeed(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	hospitalID := scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001'`)
	doctorID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'doctor001@medikaone.id'`)
	adminID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'admin001@medikaone.id'`)
	patientID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'patient001@medikaone.id'`)
	departmentID, invitationID, affiliationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := sqlDB.Exec(`INSERT INTO hospital_departments (id, hospital_id, code, name, created_at, updated_at) VALUES ($1,$2,'IT-SCHEDULE','Integration Schedule',$3,$3)`, departmentID, hospitalID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitations (id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'ACCEPTED',$6,$7,$7,$7)`, invitationID, hospitalID, doctorID, departmentID, adminID, now.Add(24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_affiliations (id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$6)`, affiliationID, hospitalID, doctorID, departmentID, invitationID, now); err != nil {
		t.Fatal(err)
	}
	repo := appointmentrepo.NewRepository(db)
	propose := func(operation string, items []appointmentrepo.ScheduleItem, target *string) *response.ScheduleChangeRequest {
		t.Helper()
		row, err := repo.CreateScheduleChange(ctx, appointmentrepo.ScheduleChangeInput{AffiliationID: affiliationID, ActorID: adminID, ActorParty: "HOSPITAL", HospitalID: hospitalID, Operation: operation, TargetScheduleID: target, Schedules: items, Now: now, ExpiresAt: now.Add(7 * 24 * time.Hour)})
		if err != nil {
			t.Fatalf("propose %s: %v", operation, err)
		}
		if row.Operation != operation || row.Status != "PENDING" || row.DoctorMedikaOneID == "" {
			t.Fatalf("proposal DTO=%+v", row)
		}
		return row
	}
	approve := func(changeID string) {
		t.Helper()
		if err := repo.ReviewScheduleChange(ctx, changeID, doctorID, "DOCTOR", "APPROVED", nil, now); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}
	recurring := appointmentrepo.ScheduleItem{DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 2}
	initial := propose("REPLACE", []appointmentrepo.ScheduleItem{recurring}, nil)
	if err := repo.ReviewScheduleChange(ctx, initial.ID, adminID, "HOSPITAL", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrScheduleChangeOwnApproval) {
		t.Fatalf("own approval=%v", err)
	}
	approve(initial.ID)
	var dstRecurringConflict bool
	if err := sqlDB.QueryRow(`SELECT public.medikaone_schedule_windows_overlap(
		1, NULL, '08:00'::time, '09:00'::time, 'America/New_York',
		3, NULL, '18:00'::time, '19:00'::time, 'Europe/London')`).Scan(&dstRecurringConflict); err != nil {
		t.Fatal(err)
	}
	if !dstRecurringConflict {
		t.Fatal("database guard must fail closed for recurring schedules across DST timezones")
	}

	// The same doctor can belong to another hospital whose local clock uses a
	// different fixed offset. Monday 09:00 WITA is Monday 08:00 WIB, so the
	// approval must fail even though the raw wall-clock values do not overlap.
	secondHospitalID := scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-002'`)
	secondDepartmentID := scalarString(t, sqlDB, `SELECT department.id::text FROM hospital_departments department
		JOIN hospitals hospital ON hospital.id = department.hospital_id
		WHERE hospital.code = 'HSP-MO-002' AND department.code = 'POLI-UMUM'`)
	registrationRepo := doctorhospitalrepo.NewRepository(db)
	conflicts, err := registrationRepo.HasActiveScheduleConflict(ctx, doctorID, []doctorhospitalrepo.Schedule{{
		DayOfWeek: 1, StartTime: "09:00", EndTime: "10:00", Timezone: "Asia/Makassar",
	}})
	if err != nil || !conflicts {
		t.Fatalf("invitation preflight missed WIB/WITA conflict: conflict=%v err=%v", conflicts, err)
	}
	secondMataDepartmentID := scalarString(t, sqlDB, `SELECT department.id::text FROM hospital_departments department
		JOIN hospitals hospital ON hospital.id = department.hospital_id
		WHERE hospital.code = 'HSP-MO-002' AND department.code = 'POLI-MATA'`)
	conflictingInvitationID := uuid.NewString()
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitations
		(id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,'PENDING',$6,$7,$7)`, conflictingInvitationID, secondHospitalID, doctorID, secondMataDepartmentID, adminID, now.Add(24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitation_schedules
		(invitation_id,day_of_week,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity,created_at)
		VALUES ($1,1,'10:00','11:00','Asia/Jayapura','FIXED_SLOT',30,1,$2)`, conflictingInvitationID, now); err != nil {
		t.Fatal(err)
	}
	if err := registrationRepo.AcceptInvitation(ctx, conflictingInvitationID, doctorID, now); !errors.Is(err, doctorhospitalrepo.ErrScheduleConflict) {
		t.Fatalf("invitation acceptance missed WIB/WIT conflict: %v", err)
	}
	secondInvitationID, secondAffiliationID := uuid.NewString(), uuid.NewString()
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitations
		(id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,'ACCEPTED',$6,$7,$7,$7)`, secondInvitationID, secondHospitalID, doctorID, secondDepartmentID, adminID, now.Add(24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_affiliations
		(id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$6)`, secondAffiliationID, secondHospitalID, doctorID, secondDepartmentID, secondInvitationID, now); err != nil {
		t.Fatal(err)
	}
	proposeSecondHospital := func(startTime, endTime string) *response.ScheduleChangeRequest {
		t.Helper()
		row, err := repo.CreateScheduleChange(ctx, appointmentrepo.ScheduleChangeInput{
			AffiliationID: secondAffiliationID, ActorID: adminID, ActorParty: "HOSPITAL", HospitalID: secondHospitalID,
			Operation: "ADD", Now: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
			Schedules: []appointmentrepo.ScheduleItem{{DayOfWeek: 1, StartTime: startTime, EndTime: endTime,
				Timezone: "Asia/Makassar", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	crossTimezoneConflict := proposeSecondHospital("09:00", "10:00")
	if err := repo.ReviewScheduleChange(ctx, crossTimezoneConflict.ID, doctorID, "DOCTOR", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
		t.Fatalf("WIB/WITA cross-hospital conflict=%v", err)
	}
	rejectionReason := "same absolute practice time"
	if err := repo.ReviewScheduleChange(ctx, crossTimezoneConflict.ID, doctorID, "DOCTOR", "REJECTED", &rejectionReason, now); err != nil {
		t.Fatal(err)
	}
	adjacentTimezoneSchedule := proposeSecondHospital("10:00", "11:00")
	if err := repo.ReviewScheduleChange(ctx, adjacentTimezoneSchedule.ID, doctorID, "DOCTOR", "APPROVED", nil, now); err != nil {
		t.Fatalf("adjacent WIB/WITA schedule rejected: %v", err)
	}
	if err := registrationRepo.UpdateAffiliationStatus(ctx, secondHospitalID, doctorID, "SUSPENDED", adminID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`UPDATE doctor_hospital_schedules SET start_time='09:00', end_time='10:00'
		WHERE affiliation_id=$1 AND is_active=TRUE`, secondAffiliationID); err != nil {
		t.Fatal(err)
	}
	if err := registrationRepo.UpdateAffiliationStatus(ctx, secondHospitalID, doctorID, "ACTIVE", adminID, now); !errors.Is(err, doctorhospitalrepo.ErrScheduleConflict) {
		t.Fatalf("affiliation reactivation bypassed WIB/WITA conflict: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE doctor_hospital_schedules SET start_time='10:00', end_time='11:00'
		WHERE affiliation_id=$1 AND is_active=TRUE`, secondAffiliationID); err != nil {
		t.Fatal(err)
	}
	if err := registrationRepo.UpdateAffiliationStatus(ctx, secondHospitalID, doctorID, "ACTIVE", adminID, now); err != nil {
		t.Fatalf("safe affiliation reactivation rejected: %v", err)
	}
	_, directConflictErr := sqlDB.Exec(`UPDATE doctor_hospital_schedules SET start_time='09:00', end_time='10:00'
		WHERE affiliation_id=$1 AND is_active=TRUE`, secondAffiliationID)
	var sqlState interface{ SQLState() string }
	if !errors.As(directConflictErr, &sqlState) || sqlState.SQLState() != "23P01" {
		t.Fatalf("database trigger allowed direct cross-hospital conflict: %v", directConflictErr)
	}
	if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules
		WHERE affiliation_id=$1 AND start_time='10:00' AND end_time='11:00' AND is_active=TRUE`, secondAffiliationID); got != 1 {
		t.Fatal("failed direct schedule update changed the active schedule")
	}
	if err := registrationRepo.UpdateAffiliationStatus(ctx, secondHospitalID, doctorID, "SUSPENDED", adminID, now); err != nil {
		t.Fatal(err)
	}
	overlappingInactiveScheduleID := uuid.NewString()
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_schedules
		(id, affiliation_id, day_of_week, start_time, end_time, timezone, booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at)
		VALUES ($1, $2, 1, '10:30', '11:30', 'Asia/Makassar', 'FIXED_SLOT', 30, 1, TRUE, $3, $3)`,
		overlappingInactiveScheduleID, secondAffiliationID, now); err != nil {
		t.Fatal(err)
	}
	_, directReactivationErr := sqlDB.Exec(`UPDATE doctor_hospital_affiliations SET status='ACTIVE', updated_at=$2 WHERE id=$1`, secondAffiliationID, now)
	if !errors.As(directReactivationErr, &sqlState) || sqlState.SQLState() != "23P01" {
		t.Fatalf("database trigger allowed reactivation with internally overlapping schedules: %v", directReactivationErr)
	}
	if _, err := sqlDB.Exec(`DELETE FROM doctor_hospital_schedules WHERE id=$1`, overlappingInactiveScheduleID); err != nil {
		t.Fatal(err)
	}
	if err := registrationRepo.UpdateAffiliationStatus(ctx, secondHospitalID, doctorID, "ACTIVE", adminID, now); err != nil {
		t.Fatalf("cleaned affiliation could not be reactivated: %v", err)
	}

	future := now.AddDate(0, 0, 8)
	for future.Weekday() != time.Monday {
		future = future.AddDate(0, 0, 1)
	}
	date, nextDate := future.Format("2006-01-02"), future.AddDate(0, 0, 7).Format("2006-01-02")
	specific := recurring
	specific.ScheduleDate = &date
	specific.StartTime = "13:00"
	specific.EndTime = "14:00"
	added := propose("ADD", []appointmentrepo.ScheduleItem{specific}, nil)
	approve(added.ID)
	specificNext := specific
	specificNext.ScheduleDate = &nextDate
	addedNext := propose("ADD", []appointmentrepo.ScheduleItem{specificNext}, nil)
	approve(addedNext.ID)
	if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active`, affiliationID); got != 3 {
		t.Fatalf("ADD must preserve all schedules: %d", got)
	}
	conflict := specific
	conflict.StartTime = "08:00"
	conflict.EndTime = "09:00"
	conflicting := propose("ADD", []appointmentrepo.ScheduleItem{conflict}, nil)
	if err := repo.ReviewScheduleChange(ctx, conflicting.ID, doctorID, "DOCTOR", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
		t.Fatalf("specific vs recurring conflict=%v", err)
	}
	reason := "conflicting hours"
	if err := repo.ReviewScheduleChange(ctx, conflicting.ID, doctorID, "DOCTOR", "REJECTED", &reason, now); err != nil {
		t.Fatal(err)
	}
	scheduleID := scalarString(t, sqlDB, `SELECT id::text FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND schedule_date=$2::date AND is_active`, affiliationID, date)
	schedule, err := repo.GetActiveSchedule(ctx, scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ScheduleDate == nil || *schedule.ScheduleDate != date || schedule.DoctorMedikaOneID == "" {
		t.Fatalf("specific schedule projection=%+v", schedule)
	}
	start, err := time.Parse(time.RFC3339, date+"T13:00:00+07:00")
	if err != nil {
		t.Fatal(err)
	}
	input := appointmentrepo.BookInput{PatientID: patientID, Schedule: *schedule, AppointmentDate: date, ScheduledStartAt: start, ScheduledEndAt: start.Add(30 * time.Minute), ReasonForVisit: "specific integration", ConsentVersion: "it-v1", IdempotencyKey: uuid.NewString(), IdempotencyRequestHash: strings.Repeat("a", 64), Now: now}
	wrong := input
	wrong.AppointmentDate = nextDate
	wrong.ScheduledStartAt = start.AddDate(0, 0, 7)
	wrong.ScheduledEndAt = wrong.ScheduledStartAt.Add(30 * time.Minute)
	wrong.IdempotencyKey = uuid.NewString()
	if _, _, err := repo.Book(ctx, wrong); !errors.Is(err, appointmentrepo.ErrSlotUnavailable) {
		t.Fatalf("one-off repeated on next Monday: %v", err)
	}
	booked, _, err := repo.Book(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if booked.DoctorMedikaOneID == "" {
		t.Fatal("appointment missing doctor MedikaOne ID")
	}
	remove := propose("REMOVE", nil, &scheduleID)
	if err := repo.ReviewScheduleChange(ctx, remove.ID, doctorID, "DOCTOR", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrScheduleChangeAppointments) {
		t.Fatalf("delete booked schedule=%v", err)
	}
	if err := repo.Cancel(ctx, booked.ID, patientID, "integration cancellation", now); err != nil {
		t.Fatal(err)
	}
	approve(remove.ID)
	if _, err := repo.GetActiveSchedule(ctx, scheduleID); !errors.Is(err, appointmentrepo.ErrScheduleNotFound) {
		t.Fatalf("removed schedule remains active: %v", err)
	}
	if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE id=$1`, scheduleID); got != 1 {
		t.Fatal("removed schedule history was erased")
	}
	readd := propose("ADD", []appointmentrepo.ScheduleItem{specific}, nil)
	approve(readd.ID)
	newID := scalarString(t, sqlDB, `SELECT id::text FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND schedule_date=$2::date AND is_active`, affiliationID, date)
	if newID == scheduleID {
		t.Fatal("re-add mutated historical schedule identity")
	}
	replacement := propose("REPLACE", []appointmentrepo.ScheduleItem{recurring, specific}, nil)
	approve(replacement.ID)
	if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active`, affiliationID); got != 2 {
		t.Fatalf("replace active count=%d", got)
	}
	if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM appointments WHERE id=$1 AND schedule_id=$2 AND status='CANCELLED'`, booked.ID, scheduleID); got != 1 {
		t.Fatal("replacement changed appointment history")
	}
	past := now.AddDate(0, 0, -8)
	for past.Weekday() != time.Monday {
		past = past.AddDate(0, 0, -1)
	}
	if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_schedules (id,affiliation_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity,is_active,created_at,updated_at)
		VALUES ($1,$2,1,$3::date,'15:00','16:00','Asia/Jakarta','FIXED_SLOT',30,1,TRUE,$4,$4)`, uuid.NewString(), affiliationID, past.Format("2006-01-02"), now); err != nil {
		t.Fatal(err)
	}
	conflicts, err = doctorhospitalrepo.NewRepository(db).HasActiveScheduleConflict(ctx, doctorID, []doctorhospitalrepo.Schedule{{DayOfWeek: 1, StartTime: "15:00", EndTime: "16:00"}})
	if err != nil || conflicts {
		t.Fatalf("expired one-off blocks future recurring practice: %v %v", conflicts, err)
	}
}
