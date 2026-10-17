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

func testGroupedAvailabilityIntegration(t *testing.T, db *gorm.DB) {
	t.Run("grouped doctor availability uses active practices and real reservations", func(t *testing.T) {
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
		var adminID, patientID string
		if err := tx.Raw("SELECT id::text FROM users WHERE email='admin001@medikaone.id'").Scan(&adminID).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Raw("SELECT id::text FROM users WHERE email='patient001@medikaone.id'").Scan(&patientID).Error; err != nil {
			t.Fatal(err)
		}
		doctorID := uuid.NewString()
		exec(`INSERT INTO users(id,email,password_hash,first_name,status,verified_at,dob) VALUES(?,?,'test-only-hash','Grouped Fixture','active',NOW(),'1990-01-01')`, doctorID, doctorID+"@example.test")
		exec(`INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE slug='DOCTOR'`, doctorID)
		exec(`UPDATE doctor_profiles SET sip_number=? WHERE user_id=?`, "GROUPED-"+doctorID, doctorID)
		repo := appointmentrepo.NewRepository(tx)
		svc := appointmentservice.NewService(repo, nil, "integration-only")
		date := time.Now().UTC().AddDate(0, 0, 3)
		dateText := date.Format("2006-01-02")
		query := request.GroupedAvailabilityQuery{DoctorID: doctorID, DateFrom: dateText, DateTo: dateText}
		get := func(q request.GroupedAvailabilityQuery) *response.GroupedDoctorAvailability {
			t.Helper()
			result, err := svc.ListGroupedAvailability(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			return result
		}
		empty := get(query)
		if empty.DoctorMedikaOneID == "" || len(empty.Dates) != 1 || len(empty.Dates[0].Hospitals) != 0 {
			t.Fatalf("doctor without schedules: %#v", empty)
		}
		type placement struct{ hospital, department, room, affiliation, schedule string }
		create := func(name, start, end, mode string, specific bool) placement {
			f := placement{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
			exec(`INSERT INTO hospitals(id,code,name,is_active) VALUES(?,?,?,TRUE)`, f.hospital, "GROUPED-"+name, "Grouped "+name)
			exec(`INSERT INTO hospital_departments(id,hospital_id,master_department_id,code,name,is_active) SELECT ?,?,id,code,name,TRUE FROM master_departments WHERE code='POLI-MATA'`, f.department, f.hospital)
			exec(`INSERT INTO hospital_rooms(id,hospital_id,department_id,code,name,is_active) VALUES(?,?,?,'ROOM','Room',TRUE)`, f.room, f.hospital, f.department)
			invitationID := uuid.NewString()
			exec(`INSERT INTO doctor_hospital_invitations(id,hospital_id,doctor_id,department_id,room_id,invited_by,status,expires_at,responded_at) VALUES(?,?,?,?,?,?,'ACCEPTED',NOW()+INTERVAL '7 days',NOW())`, invitationID, f.hospital, doctorID, f.department, f.room, adminID)
			exec(`INSERT INTO doctor_hospital_affiliations(id,hospital_id,doctor_id,department_id,room_id,invitation_id,status,joined_at) VALUES(?,?,?,?,?,?,'ACTIVE',NOW())`, f.affiliation, f.hospital, doctorID, f.department, f.room, invitationID)
			var scheduleDate any
			if specific {
				scheduleDate = dateText
			}
			exec(`INSERT INTO doctor_hospital_schedules(id,affiliation_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity) VALUES(?,?,?,?,?,?,'Asia/Jakarta',?,30,1)`, f.schedule, f.affiliation, int(date.Weekday()), scheduleDate, start, end, mode)
			return f
		}
		a := create("Alpha", "09:00", "10:00", "FIXED_SLOT", false)
		b := create("Beta", "13:00", "14:00", "SESSION_QUEUE", true)
		start := "09:00"
		booked, _, err := svc.CreateAppointment(ctx, patientID, uuid.NewString(), "", "integration", request.CreateAppointmentRequest{ScheduleID: a.schedule, AppointmentDate: dateText, StartTime: &start, ReasonForVisit: "Test visit", ConsentAccepted: true, ConsentVersion: "test-v1"})
		if err != nil {
			t.Fatal(err)
		}
		result := get(query)
		if len(result.Dates[0].Hospitals) != 2 {
			t.Fatalf("expected both hospitals: %#v", result)
		}
		slots := result.Dates[0].Hospitals[0].Slots
		if len(slots) != 2 || slots[0].Status != "FULL" || slots[0].AvailableCapacity != 0 || slots[1].Status != "AVAILABLE" || slots[0].RoomID == nil || *slots[0].RoomID != a.room {
			t.Fatalf("reserved fixed slots: %#v", slots)
		}
		if result.Dates[0].Hospitals[1].Slots[0].ScheduleType != "SPECIFIC" || len(result.Dates[0].Hospitals[1].Slots) != 1 {
			t.Fatal("queue session was split into fixed slots")
		}
		if err := repo.Cancel(ctx, booked.ID, patientID, "test cancellation", time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if get(query).Dates[0].Hospitals[0].Slots[0].AvailableCapacity != 1 {
			t.Fatal("cancelled booking still consumes capacity")
		}
		query.HospitalID = b.hospital
		if result := get(query); len(result.Dates[0].Hospitals) != 1 || result.Dates[0].Hospitals[0].HospitalID != b.hospital {
			t.Fatal("hospital filter was not applied")
		}
		for _, change := range []struct {
			disable, restore string
			id               string
		}{
			{`UPDATE hospitals SET is_active=FALSE WHERE id=?`, `UPDATE hospitals SET is_active=TRUE WHERE id=?`, b.hospital},
			{`UPDATE hospital_departments SET is_active=FALSE WHERE id=?`, `UPDATE hospital_departments SET is_active=TRUE WHERE id=?`, b.department},
			{`UPDATE hospital_rooms SET is_active=FALSE WHERE id=?`, `UPDATE hospital_rooms SET is_active=TRUE WHERE id=?`, b.room},
			{`UPDATE doctor_hospital_affiliations SET status='SUSPENDED' WHERE id=?`, `UPDATE doctor_hospital_affiliations SET status='ACTIVE' WHERE id=?`, b.affiliation},
			{`UPDATE doctor_hospital_schedules SET is_active=FALSE WHERE id=?`, `UPDATE doctor_hospital_schedules SET is_active=TRUE WHERE id=?`, b.schedule},
		} {
			exec(change.disable, change.id)
			if result := get(query); len(result.Dates[0].Hospitals) != 0 || result.Dates[0].HasAvailableSlots {
				t.Fatalf("inactive resource exposed: %s", change.disable)
			}
			exec(change.restore, change.id)
		}
		query.DateFrom, query.DateTo = date.AddDate(0, 0, 7).Format("2006-01-02"), date.AddDate(0, 0, 7).Format("2006-01-02")
		if len(get(query).Dates[0].Hospitals) != 0 {
			t.Fatal("specific schedule repeated")
		}
		for _, change := range []struct{ disable, restore string }{
			{`UPDATE users SET status='inactive' WHERE id=?`, `UPDATE users SET status='active' WHERE id=?`},
			{`UPDATE users SET deleted_at=NOW() WHERE id=?`, `UPDATE users SET deleted_at=NULL WHERE id=?`},
			{`UPDATE users SET verified_at=NULL WHERE id=?`, `UPDATE users SET verified_at=NOW() WHERE id=?`},
		} {
			exec(change.disable, doctorID)
			if _, err := svc.ListGroupedAvailability(ctx, query); !errors.Is(err, constant.ErrDoctorNotFound) {
				t.Fatalf("ineligible doctor error=%v", err)
			}
			exec(change.restore, doctorID)
		}
		query.DoctorID = uuid.NewString()
		if _, err := svc.ListGroupedAvailability(ctx, query); !errors.Is(err, constant.ErrDoctorNotFound) {
			t.Fatalf("unknown doctor error=%v", err)
		}
	})
}
