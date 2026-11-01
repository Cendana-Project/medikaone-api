package seeder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorhospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/Cendana-Project/medikaone-api/internal/scheduleconflict"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
)

// demoPracticeRecordingStorage retains actual uploaded document bytes, so the
// database's contract metadata is checked against a retrievable PDF, not a stub.
// The production workflow always receives the configured real storage client.
type demoPracticeRecordingStorage struct {
	objects map[string][]byte
	uploads int
}

func (s *demoPracticeRecordingStorage) Upload(_ context.Context, path, contentType string, content []byte) (*storage.UploadedObject, error) {
	if contentType != "application/pdf" || !bytes.HasPrefix(content, []byte("%PDF-")) {
		return nil, fmt.Errorf("recording storage requires a real PDF")
	}
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	if _, exists := s.objects[path]; exists {
		return nil, fmt.Errorf("object already exists")
	}
	s.objects[path] = bytes.Clone(content)
	s.uploads++
	return &storage.UploadedObject{Bucket: "demo-practice-integration", ObjectPath: path, FileSize: int64(len(content))}, nil
}

func (s *demoPracticeRecordingStorage) Delete(_ context.Context, path string) error {
	delete(s.objects, path)
	return nil
}

func (s *demoPracticeRecordingStorage) CreateSignedURL(_ context.Context, path string, _ time.Duration, _ string) (string, error) {
	if _, exists := s.objects[path]; !exists {
		return "", fmt.Errorf("object not found")
	}
	return "https://demo-storage.invalid/" + path, nil
}

func TestDemoPracticeContractPDF(t *testing.T) {
	anchor := time.Date(2026, time.October, 18, 8, 0, 0, 0, time.UTC)
	for _, fixture := range demoPractices() {
		t.Run(fixture.alias, func(t *testing.T) {
			content, err := demoContractPDF(fixture, "doctor-fixture", "hospital-fixture", anchor)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte("/Type /Page")) || !bytes.Contains(content, []byte("%%EOF")) {
				t.Fatal("contract is not a complete PDF document")
			}
		})
	}
}

// assertDemoPracticeWorkflows is hooked into the existing PostgreSQL harness
// after base seed assertions. Its complete practice graph is rolled back so it
// cannot change counts or interfere with the independent clinical helper.
func assertDemoPracticeWorkflows(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	// Keep this helper independent of the machine's time of day. Dates remain
	// relative to the current date, which repository eligibility checks use.
	now := time.Now().In(location).Truncate(time.Microsecond)
	recorder := &demoPracticeRecordingStorage{}
	s := demoPracticeTestContext(t, tx, recorder, now, location)
	if err := seedDemoPractices(s); err != nil {
		t.Fatal(err)
	}
	if recorder.uploads != 5 || len(recorder.objects) != 5 {
		t.Fatalf("contracts uploaded=%d retained=%d, want five original PDFs", recorder.uploads, len(recorder.objects))
	}
	registration := doctorhospitalrepo.NewRepository(tx)
	appointments := appointmentrepo.NewRepository(tx)
	for _, fixture := range demoPractices() {
		invitationID := s.IDs["invitation."+fixture.alias]
		invitation, err := registration.GetInvitationForDoctor(s.Context, s.Users[fixture.doctor], invitationID, s.Now)
		if err != nil {
			t.Fatalf("read invitation %s: %v", fixture.alias, err)
		}
		wantStatus := "ACCEPTED"
		if fixture.pending {
			wantStatus = "PENDING"
		}
		if invitation.Status != wantStatus {
			t.Fatalf("invitation %s status=%s want=%s", fixture.alias, invitation.Status, wantStatus)
		}
		contract, err := registration.GetContractForDoctor(s.Context, invitationID, s.Users[fixture.doctor], "original")
		if err != nil {
			t.Fatal(err)
		}
		content, exists := recorder.objects[contract.ObjectPath]
		if !exists || !bytes.HasPrefix(content, []byte("%PDF-")) || !bytes.Contains(content, []byte("%%EOF")) || contract.FileSize != int64(len(content)) || contract.SHA256 != fmt.Sprintf("%x", sha256.Sum256(content)) {
			t.Fatalf("contract %s does not describe the retained original PDF", fixture.alias)
		}
		if _, err := recorder.CreateSignedURL(s.Context, contract.ObjectPath, time.Minute, contract.Filename); err != nil {
			t.Fatal(err)
		}
		if fixture.pending {
			continue
		}
		if _, err := registration.GetAffiliationForDoctor(s.Context, s.Users[fixture.doctor], s.IDs["affiliation."+fixture.alias]); err != nil {
			t.Fatalf("read genuine affiliation %s: %v", fixture.alias, err)
		}
		for day := fixture.firstDay; day <= fixture.lastDay; day++ {
			id := s.IDs[fmt.Sprintf("schedule.%s.%d", fixture.alias, day)]
			if id == "" {
				t.Fatalf("missing actual schedule ID %s day %d", fixture.alias, day)
			}
			rows, err := appointments.ListActiveSchedules(s.Context, appointmentrepo.AvailabilityFilter{ScheduleID: id})
			if err != nil || len(rows) != 1 {
				t.Fatalf("read bookable schedule %s day %d: %d rows, %v", fixture.alias, day, len(rows), err)
			}
			row := rows[0]
			if row.DayOfWeek != day || row.StartTime != fixture.start || row.EndTime != fixture.end || row.BookingMode != fixture.mode || row.Capacity != fixture.capacity || row.SlotDurationMinutes != 30 || row.ScheduleDate != nil || row.Timezone != "Asia/Jakarta" {
				t.Fatalf("unexpected actual recurring schedule: %+v", row)
			}
		}
	}
	general, err := appointments.ListActiveSchedules(s.Context, appointmentrepo.AvailabilityFilter{DoctorID: s.Users["doctor1"]})
	if err != nil {
		t.Fatal(err)
	}
	windows := make([]scheduleconflict.Schedule, 0, len(general))
	for _, row := range general {
		if row.HospitalID == s.Hospitals["bandung"] && (row.DayOfWeek == 0 || row.DayOfWeek == 6) {
			t.Fatalf("Bandung afternoon practice falls outside hospital opening hours: %+v", row)
		}
		windows = append(windows, scheduleconflict.Schedule{DayOfWeek: row.DayOfWeek, ScheduleDate: row.ScheduleDate, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone})
	}
	if overlap, err := scheduleconflict.AnyOverlap(windows, s.Now); err != nil || overlap {
		t.Fatalf("multi-RS practice overlap=%v error=%v", overlap, err)
	}
	specific, err := appointments.ListActiveSchedules(s.Context, appointmentrepo.AvailabilityFilter{ScheduleID: s.IDs["schedule.specific"]})
	if err != nil || len(specific) != 1 || specific[0].ScheduleDate == nil || *specific[0].ScheduleDate != s.Anchor.In(location).AddDate(0, 0, 3).Format("2006-01-02") {
		t.Fatalf("approved specific schedule not bookable: %+v, %v", specific, err)
	}
	proposal, err := appointments.GetScheduleChange(s.Context, s.IDs["schedule_change.pending"])
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Status != "PENDING" || proposal.Operation != "ADD" || proposal.RequestedByParty != "DOCTOR" || len(proposal.Schedules) != 1 {
		t.Fatalf("invalid counterpart proposal: %+v", proposal)
	}
	eye, err := appointments.ListActiveSchedules(s.Context, appointmentrepo.AvailabilityFilter{DoctorID: s.Users["doctor2"]})
	if err != nil || len(eye) != 7 {
		t.Fatalf("pending ADD unexpectedly changes bookable eye schedules: %d, %v", len(eye), err)
	}
	pendingPlace, err := appointments.ListActiveSchedules(s.Context, appointmentrepo.AvailabilityFilter{DoctorID: s.Users["doctor3"], HospitalID: s.Hospitals["bandung"]})
	if err != nil || len(pendingPlace) != 0 {
		t.Fatalf("pending invitation is bookable: %+v, %v", pendingPlace, err)
	}
	// Immediate rerun must neither upload nor add audit/notification/calendar rows.
	before := demoPracticeSnapshot(t, tx)
	rerun := demoPracticeTestContext(t, tx, recorder, now.Add(24*time.Hour), location)
	if err := seedDemoPractices(rerun); err != nil {
		t.Fatal(err)
	}
	if !s.Anchor.Equal(rerun.Anchor) || !reflect.DeepEqual(s.IDs, rerun.IDs) || before != demoPracticeSnapshot(t, tx) || recorder.uploads != 5 {
		t.Fatal("practice rerun changes IDs, persistent anchor, graph, or stored objects")
	}
	// Exercise real lifecycle transitions, plus an edited/deactivated calendar.
	if err := registration.RejectInvitation(s.Context, s.IDs["invitation.pending"], s.Users["doctor3"], nil, now); err != nil {
		t.Fatal(err)
	}
	rejection := "Demo counterpart declined the extra session"
	if err := appointments.ReviewScheduleChange(s.Context, s.IDs["schedule_change.pending"], s.Users["admin"], "HOSPITAL", "REJECTED", &rejection, now); err != nil {
		t.Fatal(err)
	}
	if err := registration.UpdateAffiliationStatus(s.Context, s.Hospitals["jakarta"], s.Users["doctor3"], "SUSPENDED", s.Users["admin"], now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, capacity = 4, start_time = '08:15', updated_at = ? WHERE id = ?`, now, s.IDs["schedule.general.jakarta.0"]).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, updated_at = ? WHERE id = ?`, now, s.IDs["schedule.specific"]).Error; err != nil {
		t.Fatal(err)
	}
	before = demoPracticeSnapshot(t, tx)
	rerun = demoPracticeTestContext(t, tx, recorder, now.AddDate(0, 0, 20), location)
	if err := seedDemoPractices(rerun); err != nil {
		t.Fatal(err)
	}
	if !s.Anchor.Equal(rerun.Anchor) || !reflect.DeepEqual(s.IDs, rerun.IDs) || before != demoPracticeSnapshot(t, tx) || recorder.uploads != 5 {
		t.Fatal("practice reseed overwrites progressed lifecycle/calendar state")
	}
	var preserved int
	if err := tx.Raw(`SELECT COUNT(*) FROM doctor_hospital_schedules WHERE id = ? AND is_active = FALSE AND capacity = 4 AND start_time = '08:15'`, s.IDs["schedule.general.jakarta.0"]).Scan(&preserved).Error; err != nil || preserved != 1 {
		t.Fatalf("edited calendar not preserved: %d, %v", preserved, err)
	}
	// A deterministic root ID is never sufficient to adopt another actor tuple.
	rollback := errors.New("rollback collision fixture")
	if err := tx.Transaction(func(collision *gorm.DB) error {
		if err := collision.Exec(`UPDATE doctor_hospital_invitations SET invited_by = ? WHERE id = ?`, s.Users["superadmin"], s.IDs["invitation.general.jakarta"]).Error; err != nil {
			t.Fatal(err)
		}
		candidate := demoPracticeTestContext(t, collision, recorder, now, location)
		if err := seedDemoPractices(candidate); err == nil || !strings.Contains(err.Error(), "collision") {
			t.Fatalf("actor/place collision was not rejected: %v", err)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

func demoPracticeTestContext(t *testing.T, db *gorm.DB, contracts storage.Client, now time.Time, location *time.Location) *workflowSeedContext {
	t.Helper()
	s, err := newWorkflowSeedContext(context.Background(), db, contracts, nil, now, "https://demo.medikaone.test", location)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func demoPracticeSnapshot(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var snapshot string
	// Full row snapshots include statuses, dates, audit/event rows and metadata,
	// not merely counts which could miss a destructive replay of a transition.
	if err := db.Raw(`SELECT jsonb_build_object(
		'invitations', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_invitations r),
		'contracts', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_contracts r),
		'invitation_events', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_invitation_events r),
		'affiliations', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_affiliations r),
		'affiliation_events', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_affiliation_events r),
		'schedules', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_hospital_schedules r),
		'changes', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_schedule_change_requests r),
		'items', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_schedule_change_items r),
		'change_events', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM doctor_schedule_change_events r),
		'notifications', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM notifications r)
	)::text`).Scan(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	return snapshot
}
