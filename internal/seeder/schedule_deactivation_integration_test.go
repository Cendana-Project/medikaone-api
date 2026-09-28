package seeder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorhospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Runs only inside TestPostgresSeederIntegration, after that harness has verified
// the disposable database and applied every migration from an empty schema.
func runScheduleDeactivationIntegration(t *testing.T, db *gorm.DB, sqlDB *sql.DB) {
	t.Run("all requires counterpart review and preserves schedule history", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		proposal := f.propose(f.deactivate("ALL", nil))
		f.assertActive(6)
		if proposal.Operation != "DEACTIVATE" || proposal.Status != "PENDING" || proposal.DeactivationScope != "ALL" || len(proposal.Schedules) != 6 {
			t.Fatalf("deactivation proposal=%+v", proposal)
		}
		f.assertSnapshot(proposal.Schedules, 6)
		listed, err := f.repo.ListScheduleChanges(f.ctx, "", f.doctorID, "PENDING")
		if err != nil || len(listed) != 1 || listed[0].DeactivationScope != "ALL" {
			t.Fatalf("list omitted deactivation scope: %+v %v", listed, err)
		}
		f.assertSnapshot(listed[0].Schedules, 6)
		affiliations, err := doctorhospitalrepo.NewRepository(db).ListDoctorAffiliations(f.ctx, f.doctorID, "ACTIVE")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, affiliation := range affiliations {
			if affiliation.AffiliationID != f.affiliationID {
				continue
			}
			found = true
			if len(affiliation.Schedules) != 6 || len(affiliation.PendingScheduleChanges) != 1 {
				t.Fatalf("pending deactivation changed active projection: %+v", affiliation)
			}
			pending := affiliation.PendingScheduleChanges[0]
			if pending.ID != proposal.ID || pending.DeactivationScope != "ALL" {
				t.Fatalf("nested proposal scope missing: %+v", pending)
			}
			f.assertSnapshot(pending.Schedules, 6)
			encoded, err := json.Marshal(pending)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				ScheduleGroups []response.ScheduleGroup `json:"schedule_groups"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil || len(payload.ScheduleGroups) == 0 {
				t.Fatalf("pending deactivation groups missing: %s %v", encoded, err)
			}
			for _, group := range payload.ScheduleGroups {
				if group.Status != "PENDING" {
					t.Fatalf("pending group status=%s", group.Status)
				}
			}
		}
		if !found {
			t.Fatal("deactivation affiliation missing")
		}
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.doctorID, "DOCTOR", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeOwnApproval) {
			t.Fatalf("own approval=%v", err)
		}
		f.approve(proposal.ID)
		f.assertActive(0)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1`, f.affiliationID); got != 7 {
			t.Fatalf("deactivation rewrote history: schedule count=%d, want 7", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active`, f.otherAffiliationID); got != 1 {
			t.Fatalf("other hospital affected: active=%d", got)
		}
		if got := scalarString(t, sqlDB, `SELECT status FROM doctor_hospital_affiliations WHERE id=$1`, f.affiliationID); got != "ACTIVE" {
			t.Fatalf("schedule deactivation changed affiliation status=%s", got)
		}
		for _, event := range []string{"CREATED", "APPROVED"} {
			if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_schedule_change_events WHERE change_request_id=$1 AND event_type=$2`, proposal.ID, event); got != 1 {
				t.Fatalf("%s audit count=%d", event, got)
			}
		}
		if _, err := f.repo.CreateScheduleChange(f.ctx, f.deactivate("ALL", nil)); !errors.Is(err, appointmentrepo.ErrScheduleNotFound) {
			t.Fatalf("empty deactivation=%v", err)
		}
	})

	t.Run("weekday deactivates every routine session and preserves specific dates", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		monday := 1
		proposal := f.propose(f.deactivate("RECURRING_DAY", &monday))
		if proposal.DeactivationDayOfWeek == nil || *proposal.DeactivationDayOfWeek != monday || len(proposal.Schedules) != 2 {
			t.Fatalf("weekday target snapshot=%+v", proposal)
		}
		f.assertSnapshot(proposal.Schedules, 2)
		f.approve(proposal.ID)
		f.assertActive(4)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active AND schedule_date IS NULL AND day_of_week=1`, f.affiliationID); got != 0 {
			t.Fatalf("Monday sessions still active=%d", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active AND schedule_date IS NOT NULL`, f.affiliationID); got != 2 {
			t.Fatalf("specific Monday dates were deactivated=%d", got)
		}
		// Sunday is zero and must survive pointer/omitempty handling.
		sunday := 0
		sundayProposal := f.propose(f.deactivate("RECURRING_DAY", &sunday))
		if sundayProposal.DeactivationDayOfWeek == nil || *sundayProposal.DeactivationDayOfWeek != 0 {
			t.Fatal("Sunday selector lost")
		}
		encoded, err := json.Marshal(sundayProposal)
		if err != nil || !strings.Contains(string(encoded), `"deactivation_day_of_week":0`) {
			t.Fatalf("Sunday missing from response: %s %v", encoded, err)
		}
		f.approve(sundayProposal.ID)
		f.assertActive(3)
	})

	t.Run("rejection and expiration leave all schedules active", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		proposal := f.propose(f.deactivate("ALL", nil))
		f.reject(proposal.ID)
		f.assertActive(6)
		proposal = f.propose(f.deactivate("ALL", nil))
		if _, err := sqlDB.Exec(`UPDATE doctor_schedule_change_requests SET expires_at=$2 WHERE id=$1`, proposal.ID, f.now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		newProposal := f.propose(f.deactivate("ALL", nil))
		if status := scalarString(t, sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, proposal.ID); status != "EXPIRED" {
			t.Fatalf("expired deactivation status=%s", status)
		}
		f.assertActive(6)
		f.reject(newProposal.ID)
	})

	t.Run("locked expired proposal cannot deadlock a replacement request", func(t *testing.T) {
		for _, scope := range []string{"ALL", "RECURRING_DAY"} {
			t.Run(scope, func(t *testing.T) {
				f := newDeactivationFixture(t, db, sqlDB)
				input := f.deactivate(scope, nil)
				if scope == "RECURRING_DAY" {
					monday := 1
					input.DeactivationDayOfWeek = &monday
				}
				old := f.propose(input)
				if _, err := sqlDB.Exec(`UPDATE doctor_schedule_change_requests SET expires_at=$2 WHERE id=$1`, old.ID, f.now.Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
				// A concurrent reviewer owns the proposal lock before obtaining
				// the affiliation lock. Creation must not wait on its unique index
				// while retaining that affiliation lock and complete a lock cycle.
				holder, err := sqlDB.BeginTx(f.ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = holder.Rollback() })
				var lockedID string
				if err := holder.QueryRowContext(f.ctx, `SELECT id::text FROM doctor_schedule_change_requests WHERE id=$1 FOR UPDATE`, old.ID).Scan(&lockedID); err != nil {
					t.Fatal(err)
				}
				attemptCtx, cancel := context.WithTimeout(f.ctx, 2*time.Second)
				_, createErr := f.repo.CreateScheduleChange(attemptCtx, input)
				cancel()
				if !errors.Is(createErr, appointmentrepo.ErrScheduleChangeExists) {
					t.Fatalf("replacement waited on locked expired %s proposal or bypassed it: %v", scope, createErr)
				}
				if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_schedule_change_requests WHERE affiliation_id=$1`, f.affiliationID); got != 1 {
					t.Fatalf("locked expiry created another proposal: count=%d", got)
				}
				if err := holder.Rollback(); err != nil {
					t.Fatal(err)
				}
				f.propose(input)
				if got := scalarString(t, sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, old.ID); got != "EXPIRED" {
					t.Fatalf("unlocked expired proposal status=%s", got)
				}
				f.assertActive(6)
			})
		}
	})

	t.Run("active target appointments block entire approval", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		proposal := f.propose(f.deactivate("ALL", nil))
		// Pending deactivation does not remove booking availability.
		booked := f.book(f.schedules["specific"], f.monday, "13:00")
		for _, status := range []string{"CONFIRMED", "CHECKED_IN", "WAITING_VITALS", "WAITING_DOCTOR", "IN_CONSULTATION"} {
			if _, err := sqlDB.Exec(`UPDATE appointments SET status=$2 WHERE id=$1`, booked.ID, status); err != nil {
				t.Fatal(err)
			}
			if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.adminID, "HOSPITAL", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeAppointments) {
				t.Fatalf("%s appointment did not block ALL: %v", status, err)
			}
			f.assertActive(6)
			f.assertPending(proposal.ID)
		}
		if _, err := sqlDB.Exec(`UPDATE appointments SET status='CONFIRMED' WHERE id=$1`, booked.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.repo.Cancel(f.ctx, booked.ID, f.patientID, "integration cancellation", f.now); err != nil {
			t.Fatal(err)
		}
		f.approve(proposal.ID)
		f.assertActive(0)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM appointments WHERE id=$1 AND schedule_id=$2 AND status='CANCELLED'`, booked.ID, f.schedules["specific"]); got != 1 {
			t.Fatal("deactivation changed appointment history")
		}
	})

	t.Run("weekday ignores unrelated appointments but blocks selected sessions", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		monday := 1
		proposal := f.propose(f.deactivate("RECURRING_DAY", &monday))
		unrelated := f.book(f.schedules["tuesday"], f.monday.AddDate(0, 0, 1), "08:00")
		target := f.book(f.schedules["monday-late"], f.monday, "10:00")
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.adminID, "HOSPITAL", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeAppointments) {
			t.Fatalf("target appointment did not block weekday deactivation: %v", err)
		}
		f.assertActive(6)
		if err := f.repo.Cancel(f.ctx, target.ID, f.patientID, "integration cancellation", f.now); err != nil {
			t.Fatal(err)
		}
		f.approve(proposal.ID)
		f.assertActive(4)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM appointments WHERE id=$1 AND status='CONFIRMED'`, unrelated.ID); got != 1 {
			t.Fatal("weekday deactivation changed unrelated appointment")
		}
	})

	t.Run("stale snapshot cannot partially deactivate remaining targets", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		proposal := f.propose(f.deactivate("ALL", nil))
		if _, err := sqlDB.Exec(`UPDATE doctor_hospital_schedules SET is_active=FALSE WHERE id=$1`, f.schedules["monday"]); err != nil {
			t.Fatal(err)
		}
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.adminID, "HOSPITAL", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrInvalidScheduleChangeState) {
			t.Fatalf("stale frozen target accepted: %v", err)
		}
		f.assertActive(5)
		f.assertPending(proposal.ID)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM doctor_schedule_change_events WHERE change_request_id=$1 AND event_type='APPROVED'`, proposal.ID); got != 0 {
			t.Fatal("failed deactivation emitted approval event")
		}
	})

	t.Run("pending mutation compatibility is symmetric", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		monday, tuesday := 1, 2
		all := f.deactivate("ALL", nil)
		day := f.deactivate("RECURRING_DAY", &monday)
		nextDay := f.deactivate("RECURRING_DAY", &tuesday)
		add := f.input("ADD")
		date := f.monday.Format("2006-01-02")
		add.Schedules = []appointmentrepo.ScheduleItem{{DayOfWeek: 1, ScheduleDate: &date, StartTime: "18:00", EndTime: "19:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}}
		replace := f.input("REPLACE")
		replace.Schedules = []appointmentrepo.ScheduleItem{{DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}}
		remove := f.input("REMOVE")
		target := f.schedules["monday"]
		remove.TargetScheduleID = &target
		for _, pair := range [][2]appointmentrepo.ScheduleChangeInput{{all, add}, {all, replace}, {all, remove}, {all, day}, {all, all}, {day, replace}, {day, remove}, {day, day}} {
			for reverse := 0; reverse < 2; reverse++ {
				first, second := pair[reverse], pair[1-reverse]
				pending := f.propose(first)
				if _, err := f.repo.CreateScheduleChange(f.ctx, second); !errors.Is(err, appointmentrepo.ErrScheduleChangeExists) {
					t.Fatalf("pending %s/%s allowed %s/%s: %v", first.Operation, first.DeactivationScope, second.Operation, second.DeactivationScope, err)
				}
				f.reject(pending.ID)
			}
		}
		// Independent days and ADD can stay pending together. Pending deactivation
		// still reserves the active routine's time until counterpart approval.
		p1, p2, p3 := f.propose(day), f.propose(nextDay), f.propose(add)
		overlap := add
		overlap.Schedules = append([]appointmentrepo.ScheduleItem(nil), add.Schedules...)
		overlap.Schedules[0].StartTime, overlap.Schedules[0].EndTime = "08:00", "09:00"
		if _, err := f.repo.CreateScheduleChange(f.ctx, overlap); !errors.Is(err, appointmentrepo.ErrDoctorScheduleConflict) {
			t.Fatalf("pending deactivation freed routine time: %v", err)
		}
		for _, proposal := range []*response.ScheduleChangeRequest{p1, p2, p3} {
			f.reject(proposal.ID)
		}
	})

	t.Run("doctor and hospital scope cannot target another affiliation", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		wrongDoctor := f.deactivate("ALL", nil)
		wrongDoctor.ActorID = scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='doctor002@medikaone.id'`)
		if _, err := f.repo.CreateScheduleChange(f.ctx, wrongDoctor); !errors.Is(err, appointmentrepo.ErrAffiliationNotFound) {
			t.Fatalf("other doctor deactivation=%v", err)
		}
		wrongHospital := f.deactivate("ALL", nil)
		wrongHospital.ActorID, wrongHospital.ActorParty, wrongHospital.HospitalID = f.adminID, "HOSPITAL", f.otherHospitalID
		if _, err := f.repo.CreateScheduleChange(f.ctx, wrongHospital); !errors.Is(err, appointmentrepo.ErrAffiliationNotFound) {
			t.Fatalf("other tenant deactivation=%v", err)
		}
		f.assertActive(6)
	})

	t.Run("hospital proposal requires tenant membership and owning doctor review", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		hospitalInput := f.deactivate("ALL", nil)
		hospitalInput.ActorID, hospitalInput.ActorParty = f.adminID, "HOSPITAL"
		// The actor is administrator of hospital 1 only. Even a correctly
		// matching hospital/affiliation pair cannot grant access to hospital 2.
		hospitalInput.HospitalID, hospitalInput.AffiliationID = f.otherHospitalID, f.otherAffiliationID
		if _, err := f.repo.CreateScheduleChange(f.ctx, hospitalInput); !errors.Is(err, appointmentrepo.ErrAffiliationNotFound) {
			t.Fatalf("unaffiliated hospital administrator created proposal: %v", err)
		}
		doctorInput := f.deactivate("ALL", nil)
		doctorInput.AffiliationID = f.otherAffiliationID
		otherProposal := f.propose(doctorInput)
		reason := "unauthorized rejection"
		for _, decision := range []string{"APPROVED", "REJECTED"} {
			if err := f.repo.ReviewScheduleChange(f.ctx, otherProposal.ID, f.adminID, "HOSPITAL", decision, &reason, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeNotFound) {
				t.Fatalf("unaffiliated hospital administrator reviewed proposal as %s: %v", decision, err)
			}
		}
		f.assertPending(otherProposal.ID)
		hospitalInput.HospitalID, hospitalInput.AffiliationID = f.hospitalID, f.affiliationID
		proposal := f.propose(hospitalInput)
		if proposal.RequestedByParty != "HOSPITAL" || proposal.RequestedBy != f.adminID {
			t.Fatalf("hospital requester identity lost: %+v", proposal)
		}
		wrongDoctor := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='doctor002@medikaone.id'`)
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, wrongDoctor, "DOCTOR", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeNotFound) {
			t.Fatalf("another doctor approved hospital proposal: %v", err)
		}
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.adminID, "HOSPITAL", "APPROVED", nil, f.now); !errors.Is(err, appointmentrepo.ErrScheduleChangeOwnApproval) {
			t.Fatalf("hospital approved its own proposal: %v", err)
		}
		f.assertPending(proposal.ID)
		f.assertActive(6)
		if err := f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.doctorID, "DOCTOR", "APPROVED", nil, f.now); err != nil {
			t.Fatalf("owning doctor could not approve hospital proposal: %v", err)
		}
		f.assertActive(0)
		f.assertPending(otherProposal.ID)
	})

	t.Run("concurrent all and weekday proposals have exactly one winner", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		start := make(chan struct{})
		errorsCh := make(chan error, 8)
		var workers sync.WaitGroup
		for i := 0; i < cap(errorsCh); i++ {
			input := f.deactivate("ALL", nil)
			if i%2 == 1 {
				monday := 1
				input = f.deactivate("RECURRING_DAY", &monday)
			}
			workers.Add(1)
			go func(input appointmentrepo.ScheduleChangeInput) {
				defer workers.Done()
				<-start
				_, err := f.repo.CreateScheduleChange(f.ctx, input)
				errorsCh <- err
			}(input)
		}
		close(start)
		workers.Wait()
		close(errorsCh)
		winners := 0
		for err := range errorsCh {
			if err == nil {
				winners++
			} else if !errors.Is(err, appointmentrepo.ErrScheduleChangeExists) {
				t.Fatalf("concurrent deactivation error=%v", err)
			}
		}
		if winners != 1 {
			t.Fatalf("concurrent deactivation winners=%d, want 1", winners)
		}
		f.assertActive(6)
	})

	t.Run("booking and deactivation approval cannot both succeed", func(t *testing.T) {
		f := newDeactivationFixture(t, db, sqlDB)
		proposal := f.propose(f.deactivate("ALL", nil))
		input := f.bookingInput(f.schedules["monday"], f.monday, "08:00")
		start := make(chan struct{})
		bookingResult, approvalResult := make(chan error, 1), make(chan error, 1)
		go func() {
			<-start
			_, _, err := f.repo.Book(f.ctx, input)
			bookingResult <- err
		}()
		go func() {
			<-start
			approvalResult <- f.repo.ReviewScheduleChange(f.ctx, proposal.ID, f.adminID, "HOSPITAL", "APPROVED", nil, f.now)
		}()
		close(start)
		bookingErr, approvalErr := <-bookingResult, <-approvalResult
		switch {
		case bookingErr == nil && errors.Is(approvalErr, appointmentrepo.ErrScheduleChangeAppointments):
			f.assertActive(6)
			f.assertPending(proposal.ID)
		case approvalErr == nil && errors.Is(bookingErr, appointmentrepo.ErrScheduleNotFound):
			f.assertActive(0)
		default:
			t.Fatalf("unsafe concurrent booking/deactivation: booking=%v approval=%v", bookingErr, approvalErr)
		}
	})
}

type deactivationFixture struct {
	t                  *testing.T
	sqlDB              *sql.DB
	repo               *appointmentrepo.Repository
	ctx                context.Context
	now                time.Time
	monday             time.Time
	doctorID           string
	adminID            string
	patientID          string
	hospitalID         string
	otherHospitalID    string
	affiliationID      string
	otherAffiliationID string
	schedules          map[string]string
}

func newDeactivationFixture(t *testing.T, db *gorm.DB, sqlDB *sql.DB) *deactivationFixture {
	t.Helper()
	if err := ResetAllAndSeed(db); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	f := &deactivationFixture{t: t, sqlDB: sqlDB, repo: appointmentrepo.NewRepository(db), ctx: ctx, now: time.Now().UTC(), schedules: map[string]string{}}
	f.doctorID = scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='doctor001@medikaone.id'`)
	f.adminID = scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='admin001@medikaone.id'`)
	f.patientID = scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='patient001@medikaone.id'`)
	f.hospitalID = scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code='HSP-MO-001'`)
	f.otherHospitalID = scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code='HSP-MO-002'`)
	for i, hospitalID := range []string{f.hospitalID, f.otherHospitalID} {
		departmentID, invitationID, affiliationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		if _, err := sqlDB.Exec(`INSERT INTO hospital_departments (id,hospital_id,code,name,created_at,updated_at) VALUES ($1,$2,'IT-DEACTIVATE','Deactivation integration',$3,$3)`, departmentID, hospitalID, f.now); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_invitations (id,hospital_id,doctor_id,department_id,invited_by,status,expires_at,responded_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'ACCEPTED',$6,$7,$7,$7)`, invitationID, hospitalID, f.doctorID, departmentID, f.adminID, f.now.Add(24*time.Hour), f.now); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_affiliations (id,hospital_id,doctor_id,department_id,invitation_id,status,joined_at,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$6)`, affiliationID, hospitalID, f.doctorID, departmentID, invitationID, f.now); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.affiliationID = affiliationID
		} else {
			f.otherAffiliationID = affiliationID
		}
	}
	f.monday = f.now.AddDate(0, 0, 8)
	for f.monday.Weekday() != time.Monday {
		f.monday = f.monday.AddDate(0, 0, 1)
	}
	date, past := f.monday.Format("2006-01-02"), f.monday.AddDate(0, 0, -28).Format("2006-01-02")
	for _, row := range []struct {
		name        string
		day         int
		date        *string
		start, end  string
		active      bool
		affiliation string
	}{
		{"monday", 1, nil, "08:00", "09:00", true, f.affiliationID},
		{"monday-late", 1, nil, "10:00", "11:00", true, f.affiliationID},
		{"tuesday", 2, nil, "08:00", "09:00", true, f.affiliationID},
		{"sunday", 0, nil, "08:00", "09:00", true, f.affiliationID},
		{"specific", 1, &date, "13:00", "14:00", true, f.affiliationID},
		{"past-specific", 1, &past, "15:00", "16:00", true, f.affiliationID},
		{"history", 1, nil, "17:00", "18:00", false, f.affiliationID},
		{"other-hospital", 3, nil, "08:00", "09:00", true, f.otherAffiliationID},
	} {
		id := uuid.NewString()
		if _, err := sqlDB.Exec(`INSERT INTO doctor_hospital_schedules (id,affiliation_id,day_of_week,schedule_date,start_time,end_time,timezone,booking_mode,slot_duration_minutes,capacity,is_active,created_at,updated_at) VALUES ($1,$2,$3,$4::date,$5::time,$6::time,'Asia/Jakarta','FIXED_SLOT',30,1,$7,$8,$8)`, id, row.affiliation, row.day, row.date, row.start, row.end, row.active, f.now); err != nil {
			t.Fatal(err)
		}
		f.schedules[row.name] = id
	}
	return f
}

func (f *deactivationFixture) input(operation string) appointmentrepo.ScheduleChangeInput {
	return appointmentrepo.ScheduleChangeInput{Operation: operation, AffiliationID: f.affiliationID, ActorID: f.doctorID, ActorParty: "DOCTOR", Now: f.now, ExpiresAt: f.now.Add(7 * 24 * time.Hour)}
}

func (f *deactivationFixture) deactivate(scope string, day *int) appointmentrepo.ScheduleChangeInput {
	input := f.input("DEACTIVATE")
	input.DeactivationScope, input.DeactivationDayOfWeek = scope, day
	return input
}

func (f *deactivationFixture) propose(input appointmentrepo.ScheduleChangeInput) *response.ScheduleChangeRequest {
	f.t.Helper()
	row, err := f.repo.CreateScheduleChange(f.ctx, input)
	if err != nil {
		f.t.Fatalf("propose %s/%s: %v", input.Operation, input.DeactivationScope, err)
	}
	return row
}

func (f *deactivationFixture) approve(id string) {
	f.t.Helper()
	if err := f.repo.ReviewScheduleChange(f.ctx, id, f.adminID, "HOSPITAL", "APPROVED", nil, f.now); err != nil {
		f.t.Fatalf("approve deactivation: %v", err)
	}
}

func (f *deactivationFixture) reject(id string) {
	f.t.Helper()
	reason := "integration rejection"
	if err := f.repo.ReviewScheduleChange(f.ctx, id, f.adminID, "HOSPITAL", "REJECTED", &reason, f.now); err != nil {
		f.t.Fatalf("reject proposal: %v", err)
	}
}

func (f *deactivationFixture) assertActive(want int) {
	f.t.Helper()
	if got := scalarInt(f.t, f.sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE affiliation_id=$1 AND is_active`, f.affiliationID); got != want {
		f.t.Fatalf("active schedule count=%d, want %d", got, want)
	}
}

func (f *deactivationFixture) assertPending(id string) {
	f.t.Helper()
	if got := scalarString(f.t, f.sqlDB, `SELECT status FROM doctor_schedule_change_requests WHERE id=$1`, id); got != "PENDING" {
		f.t.Fatalf("failed approval status=%s", got)
	}
}

func (f *deactivationFixture) assertSnapshot(schedules []response.DoctorHospitalSchedule, want int) {
	f.t.Helper()
	seen := map[string]bool{}
	for _, item := range schedules {
		if item.TargetScheduleID == nil || *item.TargetScheduleID == "" || *item.TargetScheduleID == item.ID || seen[*item.TargetScheduleID] {
			f.t.Fatalf("snapshot target identity invalid: %+v", item)
		}
		seen[*item.TargetScheduleID] = true
		if got := scalarInt(f.t, f.sqlDB, `SELECT COUNT(*) FROM doctor_hospital_schedules WHERE id=$1 AND affiliation_id=$2 AND is_active`, *item.TargetScheduleID, f.affiliationID); got != 1 {
			f.t.Fatalf("snapshot target is not original active schedule: %+v", item)
		}
	}
	if len(seen) != want {
		f.t.Fatalf("snapshot targets=%d, want %d", len(seen), want)
	}
}

func (f *deactivationFixture) bookingInput(scheduleID string, date time.Time, clock string) appointmentrepo.BookInput {
	f.t.Helper()
	schedule, err := f.repo.GetActiveSchedule(f.ctx, scheduleID)
	if err != nil {
		f.t.Fatal(err)
	}
	day := date.Format("2006-01-02")
	start, err := time.Parse(time.RFC3339, day+"T"+clock+":00+07:00")
	if err != nil {
		f.t.Fatal(err)
	}
	return appointmentrepo.BookInput{PatientID: f.patientID, Schedule: *schedule, AppointmentDate: day, ScheduledStartAt: start, ScheduledEndAt: start.Add(30 * time.Minute), ReasonForVisit: "deactivation integration", ConsentVersion: "it-v1", IdempotencyKey: uuid.NewString(), IdempotencyRequestHash: strings.Repeat("d", 64), Now: f.now}
}

func (f *deactivationFixture) book(scheduleID string, date time.Time, clock string) *response.Appointment {
	f.t.Helper()
	row, _, err := f.repo.Book(f.ctx, f.bookingInput(scheduleID, date, clock))
	if err != nil {
		f.t.Fatal(err)
	}
	return row
}
