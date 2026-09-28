package seeder

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func runPendingScheduleConflictIntegration(t *testing.T, db *gorm.DB, sqlDB *sql.DB) {
	if err := ResetAllAndSeed(db); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now().UTC()
	doctorID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'doctor001@medikaone.id'`)
	adminID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'admin001@medikaone.id'`)
	hospitalIDs := []string{
		scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001'`),
		scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-002'`),
	}
	affiliationIDs := make([]string, len(hospitalIDs))
	for i, hospitalID := range hospitalIDs {
		departmentID, invitationID, affiliationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		if _, err := sqlDB.Exec(`INSERT INTO hospital_departments (id,hospital_id,code,name,created_at,updated_at)
			VALUES ($1,$2,'IT-PENDING','Pending conflict integration',$3,$3)`, departmentID, hospitalID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitations
			(id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,'ACCEPTED',$6,$7,$7,$7)`, invitationID, hospitalID, doctorID, departmentID, adminID, now.Add(24*time.Hour), now); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_affiliations
			(id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$6)`, affiliationID, hospitalID, doctorID, departmentID, invitationID, now); err != nil {
			t.Fatal(err)
		}
		affiliationIDs[i] = affiliationID
	}
	repo := appointmentrepo.NewRepository(db)
	input := func(hospital int, operation string, items ...appointmentrepo.ScheduleItem) appointmentrepo.ScheduleChangeInput {
		return appointmentrepo.ScheduleChangeInput{
			AffiliationID: affiliationIDs[hospital], HospitalID: hospitalIDs[hospital], ActorID: adminID, ActorParty: "HOSPITAL",
			Operation: operation, Schedules: items, Now: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
		}
	}
	propose := func(hospital int, operation string, items ...appointmentrepo.ScheduleItem) *response.ScheduleChangeRequest {
		t.Helper()
		row, err := repo.CreateScheduleChange(ctx, input(hospital, operation, items...))
		if err != nil {
			t.Fatalf("propose %s: %v", operation, err)
		}
		return row
	}
	reject := func(id string) {
		t.Helper()
		reason := "integration cleanup"
		if err := repo.ReviewScheduleChange(ctx, id, doctorID, "DOCTOR", "REJECTED", &reason, now); err != nil {
			t.Fatal(err)
		}
	}
	approve := func(id string) {
		t.Helper()
		if err := repo.ReviewScheduleChange(ctx, id, doctorID, "DOCTOR", "APPROVED", nil, now); err != nil {
			t.Fatalf("approve %s: %v", id, err)
		}
	}
	conflict := func(hospital int, operation string, items ...appointmentrepo.ScheduleItem) {
		t.Helper()
		if _, err := repo.CreateScheduleChange(ctx, input(hospital, operation, items...)); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
			t.Fatalf("conflicting %s proposal accepted or mapped incorrectly: %v", operation, err)
		}
	}
	markExpired := func(id string) {
		t.Helper()
		if _, err := sqlDB.Exec(`UPDATE doctor_schedule_change_requests SET expires_at=$2 WHERE id=$1`, id, now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	assertExpired := func(id string) {
		t.Helper()
		if got := scalarString(t, sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, id); got != "EXPIRED" {
			t.Fatalf("stale proposal status=%s", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_schedule_change_events WHERE change_request_id=$1 AND event_type='EXPIRED'`, id); got != 1 {
			t.Fatalf("stale proposal expiry audit count=%d", got)
		}
	}
	future := now.AddDate(0, 0, 8)
	for future.Weekday() != time.Monday {
		future = future.AddDate(0, 0, 1)
	}
	date, nextDate := future.Format("2006-01-02"), future.AddDate(0, 0, 7).Format("2006-01-02")
	item := func(date *string, start, end, timezone string) appointmentrepo.ScheduleItem {
		return appointmentrepo.ScheduleItem{DayOfWeek: 1, ScheduleDate: date, StartTime: start, EndTime: end,
			Timezone: timezone, BookingMode: "FIXED_SLOT", SlotDurationMinutes: 15, Capacity: 1}
	}
	routine := item(nil, "09:00", "11:00", "Asia/Jakarta")
	approve(propose(0, "REPLACE", routine).ID)
	// REPLACE may overlap the old routine it will replace; specific proposals
	// still have to respect that active routine until replacement is approved.
	reject(propose(0, "REPLACE", routine).ID)
	expiredRoutine := propose(0, "REPLACE", routine)
	markExpired(expiredRoutine.ID)
	reject(propose(0, "REPLACE", routine).ID)
	assertExpired(expiredRoutine.ID)
	conflict(0, "ADD", item(&date, "09:30", "10:00", "Asia/Jakarta"))
	conflict(0, "ADD", item(&date, "08:30", "09:30", "Asia/Jakarta"))
	reject(propose(0, "ADD", item(&date, "08:00", "09:00", "Asia/Jakarta")).ID)

	specific := item(&date, "13:00", "14:00", "Asia/Jakarta")
	pending := propose(0, "ADD", specific)
	conflict(0, "ADD", specific)
	conflict(0, "ADD", item(&date, "13:15", "13:45", "Asia/Jakarta"))
	conflict(0, "ADD", item(&date, "12:30", "14:30", "Asia/Jakarta"))
	conflict(1, "ADD", item(&date, "14:00", "15:00", "Asia/Makassar"))
	adjacent := propose(0, "ADD", item(&date, "14:00", "15:00", "Asia/Jakarta"))
	nextWeek := propose(0, "ADD", item(&nextDate, "13:00", "14:00", "Asia/Jakarta"))
	approve(pending.ID) // Approval ignores its own pending snapshot.
	conflict(0, "ADD", specific)
	reject(adjacent.ID)
	reject(nextWeek.ID)

	pendingRoutine := propose(0, "REPLACE", item(nil, "15:00", "16:00", "Asia/Jakarta"))
	conflict(0, "ADD", item(&date, "15:30", "16:30", "Asia/Jakarta"))
	conflict(1, "ADD", item(&date, "16:30", "17:30", "Asia/Makassar"))
	conflict(0, "ADD", item(&date, "09:30", "10:00", "Asia/Jakarta"))
	reject(pendingRoutine.ID)
	pendingSpecific := propose(0, "ADD", item(&date, "15:30", "16:30", "Asia/Jakarta"))
	conflict(0, "REPLACE", item(nil, "15:00", "16:00", "Asia/Jakarta"))
	conflict(1, "REPLACE", item(nil, "16:00", "17:00", "Asia/Makassar"))
	reject(pendingSpecific.ID)

	expired := propose(0, "ADD", item(&date, "15:00", "16:00", "Asia/Jakarta"))
	markExpired(expired.ID)
	approve(propose(0, "ADD", item(&date, "15:00", "16:00", "Asia/Jakarta")).ID)
	assertExpired(expired.ID)
	routineID := scalarString(t, sqlDB, `SELECT id::text FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND schedule_date IS NULL AND is_active`, affiliationIDs[0])
	removeInput := input(0, "REMOVE")
	removeInput.TargetScheduleID = &routineID
	remove, err := repo.CreateScheduleChange(ctx, removeInput)
	if err != nil {
		t.Fatal(err)
	}
	conflict(0, "ADD", item(&date, "09:30", "10:00", "Asia/Jakarta"))
	markExpired(remove.ID)
	newRemove, err := repo.CreateScheduleChange(ctx, removeInput)
	if err != nil {
		t.Fatal(err)
	}
	assertExpired(remove.ID)
	reject(newRemove.ID)

	// Requests race across both affiliations. A lock scoped only to affiliation
	// allows two winners, whereas the doctor-wide lock permits exactly one.
	start := make(chan struct{})
	errorsCh := make(chan error, 8)
	var workers sync.WaitGroup
	for i := 0; i < cap(errorsCh); i++ {
		workers.Add(1)
		go func(hospital int) {
			defer workers.Done()
			<-start
			_, err := repo.CreateScheduleChange(ctx, input(hospital, "ADD", item(&date, "18:00", "19:00", "Asia/Jakarta")))
			errorsCh <- err
		}(i % len(hospitalIDs))
	}
	close(start)
	workers.Wait()
	close(errorsCh)
	winners := 0
	for err := range errorsCh {
		if err == nil {
			winners++
		} else if !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
			t.Fatalf("concurrent proposal error: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent same-doctor proposals accepted=%d, want 1", winners)
	}

	// Simulate duplicates saved by an older backend. Do not auto-approve or
	// overwrite one proposal: a conflicting legacy proposal remains PENDING.
	legacySource := propose(0, "ADD", item(&date, "20:00", "21:00", "Asia/Jakarta"))
	cloneLegacy := func() string {
		t.Helper()
		id := uuid.NewString()
		if _, err := sqlDB.Exec(`INSERT INTO doctor_schedule_change_requests
			(id,affiliation_id,requested_by,requested_by_party,status,expires_at,created_at,updated_at,operation)
			SELECT $1,affiliation_id,requested_by,requested_by_party,'PENDING',expires_at,created_at,updated_at,operation
			FROM doctor_schedule_change_requests WHERE id=$2`, id, legacySource.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO doctor_schedule_change_items
			(id,change_request_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity,created_at)
			SELECT gen_random_uuid(),$1,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity,created_at
			FROM doctor_schedule_change_items WHERE change_request_id=$2`, id, legacySource.ID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	legacy := cloneLegacy()
	if err := repo.ReviewScheduleChange(ctx, legacySource.ID, doctorID, "DOCTOR", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
		t.Fatalf("approval ignored another legacy pending proposal: %v", err)
	}
	if got := scalarString(t, sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, legacySource.ID); got != "PENDING" {
		t.Fatalf("failed approval mutated status=%s", got)
	}
	reject(legacy)
	approve(legacySource.ID)
	legacy = cloneLegacy()
	if err := repo.ReviewScheduleChange(ctx, legacy, doctorID, "DOCTOR", "APPROVED", nil, now); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
		t.Fatalf("approval ignored newly active schedule: %v", err)
	}
	if got := scalarString(t, sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, legacy); got != "PENDING" {
		t.Fatalf("failed approval mutated legacy status=%s", got)
	}
}
