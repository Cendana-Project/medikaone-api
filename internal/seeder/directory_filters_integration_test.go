package seeder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	directoryrepo "github.com/Cendana-Project/medikaone-api/internal/repository/directory"
	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	appointmentservice "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func testDirectoryFiltersIntegration(t *testing.T, db *gorm.DB) {
	t.Run("experience gender and bookable discovery", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		ctx := context.Background()
		exec := func(sql string, args ...any) {
			t.Helper()
			if err := tx.Exec(sql, args...).Error; err != nil {
				t.Fatal(err)
			}
		}
		var admin, patient string
		tx.Raw("SELECT id::text FROM users WHERE email='admin001@medikaone.id'").Scan(&admin)
		tx.Raw("SELECT id::text FROM users WHERE email='patient001@medikaone.id'").Scan(&patient)
		patientRecord := uuid.NewString()
		exec(`INSERT INTO patient_records(id,user_id,first_name,phone,dob,gender,identity_type,identity_number,identity_number_normalized) VALUES(?,?,'Fixture','081234567890','1990-01-01','L','OTHER',?,?)`, patientRecord, patient, patientRecord, patientRecord)
		var now time.Time
		if err := tx.Raw("SELECT CURRENT_TIMESTAMP").Scan(&now).Error; err != nil {
			t.Fatal(err)
		}
		date := now.UTC().AddDate(0, 0, 3)
		dateText := date.Format("2006-01-02")
		type fixture struct{ doctor, hospital, department, affiliation, schedule string }
		create := func(name, gender, start, mode string, capacity int, specific bool) fixture {
			f := fixture{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
			exec(`INSERT INTO users(id,email,password_hash,first_name,status,verified_at,gender,dob) VALUES(?,?,'test',?,'active',NOW(),NULLIF(?,''),'1980-01-01')`, f.doctor, f.doctor+"@example.test", "FilterFixture "+name, gender)
			exec(`INSERT INTO user_roles(user_id,role_id) SELECT ?,id FROM roles WHERE slug='DOCTOR'`, f.doctor)
			exec(`UPDATE doctor_profiles SET sip_number=?, practice_started_on=NULLIF(?,'')::date WHERE user_id=?`, "FILTER-"+f.doctor, start, f.doctor)
			exec(`INSERT INTO hospitals(id,code,name,is_active) VALUES(?,?,?,TRUE)`, f.hospital, "FILTER-"+name, "FilterFixture "+name)
			exec(`INSERT INTO hospital_departments(id,hospital_id,master_department_id,code,name,is_active) SELECT ?,?,id,code,name,TRUE FROM master_departments WHERE code='POLI-MATA'`, f.department, f.hospital)
			invitation := uuid.NewString()
			exec(`INSERT INTO doctor_hospital_invitations(id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at) VALUES(?,?,?,?,?,'ACCEPTED',NOW()+INTERVAL '7 days',NOW())`, invitation, f.hospital, f.doctor, f.department, admin)
			exec(`INSERT INTO doctor_hospital_affiliations(id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at) VALUES(?,?,?,?,?,'ACTIVE',NOW())`, f.affiliation, f.hospital, f.doctor, f.department, invitation)
			var specificDate any
			var weekday any = int(date.Weekday())
			if specific {
				specificDate = dateText
			}
			exec(`INSERT INTO doctor_hospital_schedules(id,affiliation_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity) VALUES(?,?,?,?,'09:00','10:00','Asia/Jakarta',?,30,?)`, f.schedule, f.affiliation, weekday, specificDate, mode, capacity)
			return f
		}
		a := create("Alpha", "P", now.UTC().AddDate(-10, 0, 0).Format("2006-01-02"), "FIXED_SLOT", 1, true)
		b := create("Beta", "L", now.UTC().AddDate(-3, 0, 0).Format("2006-01-02"), "SESSION_QUEUE", 2, false)
		unknown := create("Unknown", "", "", "FIXED_SLOT", 1, false)
		exec(`UPDATE doctor_hospital_schedules SET is_active=FALSE WHERE id=?`, unknown.schedule)
		doctors := doctorrepo.NewRepository(tx)
		hospitals := hospitalrepo.NewRepository(tx)
		list := func(q doctorrepo.Filter) *response.PublicDoctorPage {
			t.Helper()
			q.Query = "FilterFixture"
			if q.Page == 0 {
				q.Page = 1
			}
			if q.Limit == 0 {
				q.Limit = 20
			}
			p, e := doctors.ListDoctors(ctx, q)
			if e != nil {
				t.Fatal(e)
			}
			return p
		}
		hs := func(q request.HospitalDirectoryQuery) []response.Hospital {
			t.Helper()
			q.Search = "FilterFixture"
			if q.Limit == 0 {
				q.Limit = 20
			}
			r, e := hospitals.ListDirectory(ctx, q)
			if e != nil {
				t.Fatal(e)
			}
			return r
		}
		integer := func(i int) *int { return &i }
		for _, recommended := range []bool{false, true} {
			p := list(doctorrepo.Filter{Recommended: recommended, Gender: "P", MinExperienceYears: integer(10), MaxExperienceYears: integer(10)})
			if p.Total != 1 || p.Items[0].DoctorID != a.doctor || p.Items[0].ExperienceYears == nil || *p.Items[0].ExperienceYears != 10 {
				t.Fatalf("experience/gender: %#v", p)
			}
			p = list(doctorrepo.Filter{Recommended: recommended, MaxExperienceYears: integer(3)})
			if p.Total != 1 || p.Items[0].DoctorID != b.doctor {
				t.Fatalf("upper bound and unknown exclusion: %#v", p)
			}
		}
		if p := list(doctorrepo.Filter{MaxExperienceYears: integer(0)}); p.Total != 0 {
			t.Fatal("unknown experience treated as zero")
		}
		exec(`UPDATE doctor_profiles SET practice_started_on=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date WHERE user_id=?`, unknown.doctor)
		if p := list(doctorrepo.Filter{MaxExperienceYears: integer(0)}); p.Total != 1 || p.Items[0].ExperienceYears == nil || *p.Items[0].ExperienceYears != 0 {
			t.Fatal("new practitioner must have known zero years")
		}
		exec(`UPDATE doctor_profiles SET practice_started_on=NULL WHERE user_id=?`, unknown.doctor)
		detail, e := doctors.GetDoctor(ctx, unknown.doctor)
		if e != nil || detail.ExperienceYears != nil || detail.PracticeStartedOn != nil || detail.Gender != nil {
			t.Fatalf("unknown metadata: %#v %v", detail, e)
		}
		users := userrepo.NewRepository(tx)
		profile, e := users.GetDoctorProfile(ctx, a.doctor)
		if e != nil || profile.ExperienceYears == nil || *profile.ExperienceYears != 10 {
			t.Fatalf("profile projection: %#v %v", profile, e)
		}
		if e := users.UpsertDoctorProfile(ctx, map[string]any{"user_id": a.doctor, "sip_number": "FILTER-" + a.doctor, "specialty": "Mata"}); e != nil {
			t.Fatal(e)
		}
		profile, e = users.GetDoctorProfile(ctx, a.doctor)
		if e != nil || profile.PracticeStartedOn == nil {
			t.Fatal("omitting start date cleared existing experience")
		}
		if e := users.ApplyUnifiedProfileUpdate(ctx, a.doctor, userrepo.UnifiedProfileUpdate{DoctorFields: map[string]any{"practice_started_on": nil}}); e != nil {
			t.Fatal(e)
		}
		profile, e = users.GetDoctorProfile(ctx, a.doctor)
		if e != nil || profile.ExperienceYears != nil {
			t.Fatal("explicit null did not clear experience")
		}

		book := func(f fixture, start, end, mode, status string, position int) string {
			id := uuid.NewString()
			exec(`INSERT INTO appointments(id,appointment_number,patient_id,patient_record_id,created_by,affiliation_id,schedule_id,hospital_id,doctor_id,department_id,appointment_date,scheduled_start_at,scheduled_end_at,booking_mode,slot_position,queue_number,status,reason_for_visit,consent_version,consented_at,idempotency_key,idempotency_request_hash)
			VALUES(?,?,?,?,?,?,?,?,?,?,?::date,(?::date+?::time) AT TIME ZONE 'Asia/Jakarta',(?::date+?::time) AT TIME ZONE 'Asia/Jakarta',?,?,?,?,'Test','v1',NOW(),?,?)`, id, "TEST-"+id, patient, patientRecord, patient, f.affiliation, f.schedule, f.hospital, f.doctor, f.department, dateText, dateText, start, dateText, end, mode, position, id[:20], status, uuid.NewString(), "test")
			return id
		}
		first := book(a, "09:00", "09:30", "FIXED_SLOT", "CONFIRMED", 1)
		book(a, "09:30", "10:00", "FIXED_SLOT", "WAITING_DOCTOR", 1)
		book(b, "09:00", "10:00", "SESSION_QUEUE", "IN_CONSULTATION", 1)
		q := doctorrepo.Filter{AvailableOn: dateText, OnlyAvailable: true, AvailableFrom: "09:00", AvailableTo: "10:00", Limit: 1}
		hq := request.HospitalDirectoryQuery{AvailableOn: dateText, OnlyAvailable: true, AvailableFrom: "09:00", AvailableTo: "10:00", Limit: 1}
		for _, recommended := range []bool{false, true} {
			q.Recommended, hq.Recommended = recommended, recommended
			p := list(q)
			if p.Total != 1 || len(p.Items) != 1 || p.Items[0].DoctorID != b.doctor {
				t.Fatalf("capacity before pagination: %#v", p)
			}
			h := hs(hq)
			if len(h) != 1 || h[0].ID != b.hospital {
				t.Fatalf("hospital capacity before pagination: %#v", h)
			}
		}
		// Directory must agree with the availability endpoint for both booking modes.
		exec(`UPDATE hospitals SET latitude=-6.2,longitude=106.8 WHERE id IN (?,?)`, a.hospital, b.hospital)
		latitude, longitude := -6.21, 106.81
		located := q
		located.Latitude, located.Longitude = &latitude, &longitude
		if p := list(located); p.Total != 1 || p.Items[0].NearestPractice == nil || p.Items[0].NearestPractice.HospitalID != b.hospital {
			t.Fatalf("location and capacity filters diverged: %#v", p)
		}
		availability := appointmentservice.NewService(appointmentrepo.NewRepository(tx), nil, "test")
		for _, f := range []fixture{a, b} {
			rows, e := availability.ListAvailability(ctx, f.hospital, f.doctor, dateText, dateText)
			if e != nil {
				t.Fatal(e)
			}
			count := 0
			for _, r := range rows {
				count += len(r.Slots)
			}
			if (f.doctor == a.doctor && count != 0) || (f.doctor == b.doctor && count != 1) {
				t.Fatalf("booking availability mismatch: %#v", rows)
			}
		}
		// A queue session must fit completely; a short overlap isn't a bookable session.
		q.AvailableTo = "09:30"
		if p := list(q); p.Total != 0 {
			t.Fatal("partial queue session matched")
		}
		q.AvailableTo = "10:00"
		q.OnlyAvailable = false
		if p := list(q); p.Total != 2 {
			t.Fatal("legacy date filter must still include full schedules")
		}
		q.OnlyAvailable = true
		exec(`UPDATE appointments SET status='CANCELLED' WHERE id=?`, first)
		q.AvailableTo = "09:30"
		p := list(q)
		if p.Total != 1 || p.Items[0].DoctorID != a.doctor {
			t.Fatal("cancellation did not release fixed slot")
		}
		q.AvailableFrom = "09:05"
		if p := list(q); p.Total != 0 {
			t.Fatal("partial fixed slot matched")
		}
		q.AvailableFrom = "09:00"
		q.AvailableTo = "10:00"
		book(b, "09:00", "10:00", "SESSION_QUEUE", "CHECKED_IN", 2)
		p = list(q)
		if p.Total != 1 || p.Items[0].DoctorID != a.doctor {
			t.Fatal("queue capacity not enforced")
		}
		// A hospital's different, empty poli cannot satisfy availability in the selected poli.
		emptyDepartment := uuid.NewString()
		exec(`INSERT INTO hospital_departments(id,hospital_id,master_department_id,code,name,is_active) SELECT ?,?,id,code,name,TRUE FROM master_departments WHERE code='POLI-ANAK'`, emptyDepartment, a.hospital)
		hq.DepartmentID = emptyDepartment
		if h := hs(hq); len(h) != 0 {
			t.Fatal("availability leaked across departments")
		}
		hq.DepartmentID = ""
		room := uuid.NewString()
		exec(`INSERT INTO hospital_rooms(id,hospital_id,department_id,code,name,is_active) VALUES(?,?,?,'FILTER-ROOM','Filter room',FALSE)`, room, a.hospital, a.department)
		exec(`UPDATE doctor_hospital_affiliations SET room_id=? WHERE id=?`, room, a.affiliation)
		if p := list(q); p.Total != 0 {
			t.Fatal("inactive room matched doctor")
		}
		if h := hs(hq); len(h) != 0 {
			t.Fatal("inactive room matched hospital")
		}
		exec(`UPDATE doctor_hospital_affiliations SET room_id=NULL WHERE id=?`, a.affiliation)
		exec(`UPDATE appointments SET status='CANCELLED' WHERE schedule_id=?`, b.schedule)
		q.AvailableOn = date.AddDate(0, 0, 98).Format("2006-01-02")
		if p := list(q); p.Total != 0 {
			t.Fatal("outside booking horizon matched")
		}
		// Start time within the lead window cannot be booked even with free capacity.
		startSoon := now.UTC().Add(time.Hour)
		endSoon := startSoon.Add(time.Hour)
		if startSoon.Day() == endSoon.Day() {
			exec(`UPDATE doctor_hospital_schedules SET schedule_date=?::date,day_of_week=?,start_time=?::time,end_time=?::time,timezone='UTC' WHERE id=?`, startSoon.Format("2006-01-02"), int(startSoon.Weekday()), startSoon.Format("15:04"), endSoon.Format("15:04"), a.schedule)
			q.AvailableOn = startSoon.Format("2006-01-02")
			q.AvailableFrom = ""
			q.AvailableTo = ""
			q.HospitalID = a.hospital
			if p := list(q); p.Total != 0 {
				t.Fatal("lead time ignored")
			}
		}

		// Unknown hours are neither open nor closed. Apply this before LIMIT/OFFSET.
		open := true
		closed := false
		local := now.In(time.FixedZone("Jakarta", 7*3600))
		hours, _ := json.Marshal([]request.HospitalOpeningDay{{DayOfWeek: int(local.Weekday()), Is24Hours: true, Periods: []request.HospitalOpeningPeriod{}}})
		exec(`UPDATE hospitals SET opening_hours=?::jsonb,timezone='Asia/Jakarta' WHERE id=?`, string(hours), b.hospital)
		exec(`UPDATE hospitals SET opening_hours='[]'::jsonb WHERE id=?`, a.hospital)
		closedHours := fmt.Sprintf(`[{"day_of_week":%d,"is_closed":true,"periods":[]}]`, local.Weekday())
		exec(`UPDATE hospitals SET opening_hours=?::jsonb,timezone='Asia/Jakarta' WHERE id=?`, closedHours, unknown.hospital)
		for _, tc := range []struct {
			flag *bool
			want string
		}{{&open, b.hospital}, {&closed, unknown.hospital}} {
			h := hs(request.HospitalDirectoryQuery{OpenNow: tc.flag, Limit: 1})
			if len(h) != 1 || h[0].ID != tc.want {
				t.Fatalf("open filter: %#v", h)
			}
		}
		// Check overnight carryover independently of the wall clock weekday/time.
		for _, tc := range []struct {
			at, hours string
			want      bool
		}{
			{"2026-10-04 01:00:00+07", `[{"day_of_week":6,"periods":[{"open":"22:00","close":"02:00"}]}]`, true},
			{"2026-10-04 02:00:00+07", `[{"day_of_week":6,"periods":[{"open":"22:00","close":"02:00"}]}]`, false},
		} {
			var got bool
			expression := strings.ReplaceAll(directoryrepo.HospitalOpenExpression, "CURRENT_TIMESTAMP", "TIMESTAMPTZ '"+tc.at+"'")
			if e := tx.Raw("SELECT "+expression+" FROM (SELECT ?::jsonb AS opening_hours, 'Asia/Jakarta' AS timezone) directory", tc.hours).Scan(&got).Error; e != nil {
				t.Fatal(e)
			}
			if got != tc.want {
				t.Fatalf("overnight at %s = %v", tc.at, got)
			}
		}
	})
}
