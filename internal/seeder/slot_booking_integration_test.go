package seeder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	appointmentservice "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
)

// Each case has its own doctor, hospital, patients and rollback boundary. None
// of these bookings can consume seed capacity or change retained demo history.
func testSlotBookingIntegration(t *testing.T, db *gorm.DB) {
	for _, tc := range []struct {
		name, mode, timezone string
		specific             bool
		capacity             int
	}{
		{"recurring fixed WIB", "FIXED_SLOT", "Asia/Jakarta", false, 1},
		{"recurring queue WITA", "SESSION_QUEUE", "Asia/Makassar", false, 2},
		{"specific fixed WIT", "FIXED_SLOT", "Asia/Jayapura", true, 1},
		{"specific queue WIB", "SESSION_QUEUE", "Asia/Jakarta", true, 2},
	} {
		t.Run("slot inquiry booking and reschedule "+tc.name, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			ctx := context.Background()
			exec := func(query string, args ...any) {
				t.Helper()
				if err := tx.Exec(query, args...).Error; err != nil {
					t.Fatal(err)
				}
			}
			location, err := time.LoadLocation(tc.timezone)
			if err != nil {
				t.Fatal(err)
			}
			date := time.Now().In(location).AddDate(0, 0, 3)
			dateText := date.Format("2006-01-02")
			targetDate := date.AddDate(0, 0, 1).Format("2006-01-02")
			doctorID, hospitalID, affiliationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			departmentID, invitationID := uuid.NewString(), uuid.NewString()
			var adminID string
			if err := tx.Raw(`SELECT id::text FROM users WHERE email='admin001@medikaone.id'`).Scan(&adminID).Error; err != nil || adminID == "" {
				t.Fatalf("resolve seed admin: %v", err)
			}
			createUser := func(role string) string {
				t.Helper()
				id := uuid.NewString()
				exec(`INSERT INTO users(id,email,password_hash,first_name,status,verified_at,dob) VALUES(?,?,'test-only-hash','Slot Fixture','active',NOW(),'1990-01-01')`, id, id+"@example.test")
				exec(`INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE slug=?`, id, role)
				return id
			}
			// The DOCTOR role trigger creates the real profile consumed by inquiry.
			exec(`INSERT INTO users(id,email,password_hash,first_name,status,verified_at,dob) VALUES(?,?,'test-only-hash','Slot Doctor','active',NOW(),'1990-01-01')`, doctorID, doctorID+"@example.test")
			exec(`INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE slug='DOCTOR'`, doctorID)
			exec(`UPDATE doctor_profiles SET sip_number=? WHERE user_id=?`, "SLOT-"+doctorID, doctorID)
			patients := make([]string, tc.capacity+2)
			for i := range patients {
				patients[i] = createUser("PATIENT")
			}
			exec(`INSERT INTO hospitals(id,code,name,is_active) VALUES(?,?, 'Slot Consumer Hospital',TRUE)`, hospitalID, "SLT-"+hospitalID)
			exec(`INSERT INTO hospital_departments(id,hospital_id,master_department_id,code,name,is_active) SELECT ?,?,id,code,name,TRUE FROM master_departments WHERE code='POLI-MATA'`, departmentID, hospitalID)
			exec(`INSERT INTO doctor_hospital_invitations(id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at) VALUES(?,?,?,?,?,'ACCEPTED',NOW()+INTERVAL '7 days',NOW())`, invitationID, hospitalID, doctorID, departmentID, adminID)
			exec(`INSERT INTO doctor_hospital_affiliations(id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at) VALUES(?,?,?,?,?,'ACTIVE',NOW())`, affiliationID, hospitalID, doctorID, departmentID, invitationID)
			createSchedule := func(day time.Time, specific bool) string {
				t.Helper()
				id := uuid.NewString()
				var scheduleDate any
				if specific {
					scheduleDate = day.Format("2006-01-02")
				}
				// The first local slot starts on the previous UTC date. This catches
				// consumers or resolution code deriving appointment_date in UTC.
				exec(`INSERT INTO doctor_hospital_schedules(id,affiliation_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity) VALUES(?,?,?,?,'00:30','02:30',?,?,30,?)`, id, affiliationID, int(day.Weekday()), scheduleDate, tc.timezone, tc.mode, tc.capacity)
				return id
			}
			originSchedule := createSchedule(date, tc.specific)
			targetSchedule := createSchedule(date.AddDate(0, 0, 1), true)
			repo := appointmentrepo.NewRepository(tx)
			svc := appointmentservice.NewService(repo, nil, "slot-integration-first-secret")
			flat := func(scheduleID, day string) response.AvailabilitySlot {
				t.Helper()
				rows, err := svc.ListAvailability(ctx, hospitalID, doctorID, day, day)
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					if row.ScheduleID == scheduleID && len(row.Slots) > 0 {
						return row.Slots[0]
					}
				}
				t.Fatalf("no bookable slot for %s on %s", scheduleID, day)
				return response.AvailabilitySlot{}
			}
			grouped := func(scheduleID, day string, start time.Time) response.GroupedAvailabilitySlot {
				t.Helper()
				result, err := svc.ListGroupedAvailability(ctx, request.GroupedAvailabilityQuery{DoctorID: doctorID, HospitalID: hospitalID, DateFrom: day, DateTo: day})
				if err != nil {
					t.Fatal(err)
				}
				for _, date := range result.Dates {
					for _, hospital := range date.Hospitals {
						for _, slot := range hospital.Slots {
							if slot.ScheduleID == scheduleID && slot.StartAt.Equal(start) {
								return slot
							}
						}
					}
				}
				t.Fatalf("no grouped slot for %s on %s", scheduleID, day)
				return response.GroupedAvailabilitySlot{}
			}
			// Counts include both rows and counter values: a rolled-back failure
			// must not consume a number, create a patient record, or notify anyone.
			snapshot := func() [9]int64 {
				t.Helper()
				var result [9]int64
				for i, table := range []string{"appointments", "appointment_status_events", "appointment_reminders", "patient_records", "patient_record_events", "notifications", "appointment_daily_counters"} {
					if err := tx.Table(table).Count(&result[i]).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := tx.Raw(`SELECT COALESCE(SUM(last_value),0) FROM appointment_daily_counters`).Scan(&result[7]).Error; err != nil {
					t.Fatal(err)
				}
				if err := tx.Raw(`SELECT COALESCE(SUM(slot_position),0) FROM appointments`).Scan(&result[8]).Error; err != nil {
					t.Fatal(err)
				}
				return result
			}
			assertUnchanged := func(before [9]int64) {
				t.Helper()
				if after := snapshot(); after != before {
					t.Fatalf("failed/replayed request leaked side effects: before=%v after=%v", before, after)
				}
			}
			book := func(patient, key string, slot response.AvailabilitySlot) (*response.Appointment, bool, error) {
				return svc.CreateAppointment(ctx, patient, key, "127.0.0.1", "slot-consumer-integration", request.CreateAppointmentRequest{SlotID: slot.SlotID, ReasonForVisit: "Slot consumer integration", ConsentAccepted: true, ConsentVersion: "integration-v1"})
			}
			origin := flat(originSchedule, dateText)
			target := flat(targetSchedule, targetDate)
			for _, selection := range []struct {
				schedule, day string
				slot          response.AvailabilitySlot
			}{{originSchedule, dateText, origin}, {targetSchedule, targetDate, target}} {
				choice := grouped(selection.schedule, selection.day, selection.slot.StartAt)
				if len(selection.slot.SlotID) != 48 || choice.SlotID != selection.slot.SlotID || choice.Status != "AVAILABLE" || !choice.EndAt.Equal(selection.slot.EndAt) {
					t.Fatalf("flat/grouped inquiry disagrees: flat=%#v grouped=%#v", selection.slot, choice)
				}
			}
			if origin.StartAt.UTC().Format("2006-01-02") == dateText || origin.StartAt.In(location).Format("2006-01-02 15:04") != dateText+" 00:30" {
				t.Fatalf("fixture must cross the UTC date boundary: %s", origin.StartAt)
			}
			if tc.mode == "SESSION_QUEUE" && origin.EndAt.Sub(origin.StartAt) != 2*time.Hour {
				t.Fatal("queue inquiry split the whole session")
			}
			exec(`UPDATE doctor_hospital_schedules SET capacity=? WHERE id=?`, tc.capacity+1, originSchedule)
			if expanded := flat(originSchedule, dateText); expanded.SlotID != origin.SlotID || expanded.AvailableCapacity != tc.capacity+1 {
				t.Fatalf("capacity update changed occurrence identity: %#v", expanded)
			}
			exec(`UPDATE doctor_hospital_schedules SET capacity=? WHERE id=?`, tc.capacity, originSchedule)
			nextDate := date.AddDate(0, 0, 7).Format("2006-01-02")
			next, err := svc.ListAvailability(ctx, hospitalID, doctorID, nextDate, nextDate)
			if err != nil {
				t.Fatal(err)
			}
			repeated := false
			for _, session := range next {
				if session.ScheduleID == originSchedule {
					repeated = true
					if len(session.Slots) == 0 || session.Slots[0].SlotID == origin.SlotID {
						t.Fatalf("recurring occurrence must get its own identity: %#v", session)
					}
				}
			}
			if repeated == tc.specific {
				t.Fatalf("specific/recurring occurrence expansion: specific=%v repeated=%v", tc.specific, repeated)
			}
			// A different service instance and secret must resolve the same choice.
			svc = appointmentservice.NewService(repo, nil, "slot-integration-rotated-secret")
			if again := flat(originSchedule, dateText); again.SlotID != origin.SlotID {
				t.Fatal("slot identity changed after service/secret replacement")
			}
			key := uuid.NewString()
			original, replay, err := book(patients[0], key, origin)
			if err != nil || replay || original == nil {
				t.Fatalf("book API-returned choice: replay=%v err=%v", replay, err)
			}
			if original.ScheduleID != originSchedule || original.AppointmentDate != dateText || original.Timezone != tc.timezone || !original.ScheduledStartAt.Equal(origin.StartAt) || !original.ScheduledEndAt.Equal(origin.EndAt) {
				t.Fatalf("booking changed selected occurrence: %#v", original)
			}
			before := snapshot()
			retried, replay, err := book(patients[0], key, origin)
			if err != nil || !replay || retried == nil || retried.ID != original.ID {
				t.Fatalf("booking retry: replay=%v err=%v result=%#v", replay, err, retried)
			}
			assertUnchanged(before)
			if _, _, err := book(patients[0], key, target); !errors.Is(err, constant.ErrIdempotencyConflict) {
				t.Fatalf("changed-slot retry error=%v", err)
			}
			assertUnchanged(before)
			for position := 1; position < tc.capacity; position++ {
				remaining := flat(originSchedule, dateText)
				if remaining.SlotID != origin.SlotID || remaining.AvailableCapacity != tc.capacity-position {
					t.Fatalf("queue capacity changed resource identity: %#v", remaining)
				}
				if _, replay, err := book(patients[position], uuid.NewString(), origin); err != nil || replay {
					t.Fatalf("book shared queue reference: replay=%v err=%v", replay, err)
				}
			}
			full := grouped(originSchedule, dateText, origin.StartAt)
			if full.SlotID != origin.SlotID || full.Status != "FULL" || full.AvailableCapacity != 0 {
				t.Fatalf("full occurrence lost its identity/capacity: %#v", full)
			}
			remaining, err := svc.ListAvailability(ctx, hospitalID, doctorID, dateText, dateText)
			if err != nil {
				t.Fatal(err)
			}
			for _, session := range remaining {
				for _, slot := range session.Slots {
					if slot.SlotID == origin.SlotID {
						t.Fatalf("flat inquiry still offers a full occurrence: %#v", slot)
					}
				}
			}
			before = snapshot()
			if _, _, err := book(patients[len(patients)-1], uuid.NewString(), origin); !errors.Is(err, constant.ErrAppointmentSlotUnavailable) {
				t.Fatalf("inquiry is not a reservation: full booking error=%v", err)
			}
			assertUnchanged(before)
			assertOriginal := func() {
				t.Helper()
				row, err := repo.GetAppointment(ctx, original.ID)
				if err != nil || row == nil || row.Status != "CONFIRMED" || row.RescheduledToID != nil || row.CancelledAt != nil || !row.ScheduledStartAt.Equal(origin.StartAt) || !row.ScheduledEndAt.Equal(origin.EndAt) {
					t.Fatalf("failed reschedule damaged original: row=%#v err=%v", row, err)
				}
			}
			reschedule := func(key string) (*response.Appointment, bool, error) {
				return svc.ReschedulePatientAppointment(ctx, patients[0], original.ID, key, "", "integration", request.RescheduleAppointmentRequest{SlotID: target.SlotID, Reason: "Move to selected slot"})
			}
			// These are genuine IDs from an earlier inquiry, not a test-only
			// encoder. Changed end/grid, window, timezone and occurrence must not
			// be normalized into a different appointment by either consumer path.
			type staleChange struct {
				name, query string
				args        []any
			}
			changes := []staleChange{
				{"window", `UPDATE doctor_hospital_schedules SET start_time='03:30',end_time='05:30' WHERE id=?`, []any{targetSchedule}},
				{"timezone", `UPDATE doctor_hospital_schedules SET timezone='UTC' WHERE id=?`, []any{targetSchedule}},
				{"specific date", `UPDATE doctor_hospital_schedules SET schedule_date=CAST(? AS date) WHERE id=?`, []any{date.AddDate(0, 0, 8).Format("2006-01-02"), targetSchedule}},
				{"inactive", `UPDATE doctor_hospital_schedules SET is_active=FALSE WHERE id=?`, []any{targetSchedule}},
			}
			if tc.mode == "FIXED_SLOT" {
				changes = append(changes, staleChange{"duration", `UPDATE doctor_hospital_schedules SET slot_duration_minutes=45 WHERE id=?`, []any{targetSchedule}})
			}
			for _, change := range changes {
				exec(change.query, change.args...)
				before := snapshot()
				_, _, bookingErr := book(patients[len(patients)-1], uuid.NewString(), target)
				_, _, rescheduleErr := reschedule(uuid.NewString())
				for _, err := range []error{bookingErr, rescheduleErr} {
					if !errors.Is(err, constant.ErrAppointmentSlotUnavailable) && !errors.Is(err, constant.ErrScheduleNotFound) {
						t.Fatalf("stale %s reference error=%v", change.name, err)
					}
				}
				assertUnchanged(before)
				assertOriginal()
				exec(`UPDATE doctor_hospital_schedules SET start_time='00:30',end_time='02:30',timezone=?,schedule_date=CAST(? AS date),slot_duration_minutes=30,is_active=TRUE WHERE id=?`, tc.timezone, targetDate, targetSchedule)
			}
			before = snapshot()
			if _, _, err := svc.ReschedulePatientAppointment(ctx, patients[1], original.ID, uuid.NewString(), "", "integration", request.RescheduleAppointmentRequest{SlotID: target.SlotID, Reason: "Foreign patient"}); !errors.Is(err, constant.ErrAppointmentNotFound) {
				t.Fatalf("foreign patient reschedule error=%v", err)
			}
			if _, _, err := svc.RescheduleHospitalAppointment(ctx, uuid.NewString(), adminID, original.ID, uuid.NewString(), "", "integration", request.RescheduleAppointmentRequest{SlotID: target.SlotID, Reason: "Foreign hospital"}); !errors.Is(err, constant.ErrAppointmentNotFound) {
				t.Fatalf("foreign hospital reschedule error=%v", err)
			}
			assertUnchanged(before)
			assertOriginal()
			var blockers []*response.Appointment
			for i := range tc.capacity {
				blocker, _, err := book(patients[i+1], uuid.NewString(), target)
				if err != nil {
					t.Fatal(err)
				}
				blockers = append(blockers, blocker)
			}
			before = snapshot()
			if _, _, err := reschedule(uuid.NewString()); !errors.Is(err, constant.ErrAppointmentSlotUnavailable) {
				t.Fatalf("full-target reschedule error=%v", err)
			}
			assertUnchanged(before)
			assertOriginal()
			if err := repo.Cancel(ctx, blockers[0].ID, patients[1], "Release target capacity", time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			if refreshed := flat(targetSchedule, targetDate); refreshed.SlotID != target.SlotID || refreshed.AvailableCapacity != 1 {
				t.Fatalf("released capacity must retain the same reference: %#v", refreshed)
			}
			rescheduleKey := uuid.NewString()
			// Exercise both public reschedule consumers across the case matrix.
			move := func() (*response.Appointment, bool, error) {
				if tc.specific {
					return svc.RescheduleHospitalAppointment(ctx, hospitalID, adminID, original.ID, rescheduleKey, "", "integration", request.RescheduleAppointmentRequest{SlotID: target.SlotID, Reason: "Move to selected slot"})
				}
				return reschedule(rescheduleKey)
			}
			moved, replay, err := move()
			if err != nil || replay || moved == nil {
				t.Fatalf("reschedule API-returned choice: replay=%v err=%v", replay, err)
			}
			if moved.ScheduleID != targetSchedule || moved.AppointmentDate != targetDate || moved.RescheduledFromID == nil || *moved.RescheduledFromID != original.ID || !moved.ScheduledStartAt.Equal(target.StartAt) || !moved.ScheduledEndAt.Equal(target.EndAt) {
				t.Fatalf("reschedule changed selected occurrence: %#v", moved)
			}
			old, err := repo.GetAppointment(ctx, original.ID)
			if err != nil || old == nil || old.Status != "RESCHEDULED" || old.RescheduledToID == nil || *old.RescheduledToID != moved.ID {
				t.Fatalf("reschedule history links: row=%#v err=%v", old, err)
			}
			before = snapshot()
			retried, replay, err = move()
			if err != nil || !replay || retried == nil || retried.ID != moved.ID {
				t.Fatalf("reschedule retry: replay=%v err=%v result=%#v", replay, err, retried)
			}
			assertUnchanged(before)
			if released := grouped(originSchedule, dateText, origin.StartAt); released.SlotID != origin.SlotID || released.AvailableCapacity != 1 || released.Status != "AVAILABLE" {
				t.Fatalf("reschedule did not release original capacity: %#v", released)
			}
		})
	}
}
