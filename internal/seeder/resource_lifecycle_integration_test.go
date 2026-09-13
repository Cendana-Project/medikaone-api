package seeder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
)

// Uses only the disposable database in TestPostgresSeederIntegration. Every
// fixture and lifecycle mutation is rolled back before later seeder checks.
func testResourceLifecycleIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Run("resource lifecycle preserves history and tenant boundaries", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		ctx := context.Background()
		now := time.Now().UTC()
		var doctorID, actorID string
		if err := tx.Raw(`SELECT id FROM users WHERE email = 'doctor001@medikaone.id'`).Scan(&doctorID).Error; err != nil || doctorID == "" {
			t.Fatal("missing seeded doctor")
		}
		if err := tx.Raw(`SELECT id FROM users WHERE email = 'superadmin@medikaone.id'`).Scan(&actorID).Error; err != nil || actorID == "" {
			t.Fatal("missing seeded actor")
		}
		hospitals := hospitalrepo.NewRepository(tx)
		code := "LC-" + uuid.NewString()[:8]
		hospital := &entity.Hospital{ID: uuid.NewString(), Code: &code, Name: "Lifecycle " + code, Facilities: []byte(`{"parking":true}`), IsActive: true}
		if err := hospitals.Create(ctx, hospital); err != nil {
			t.Fatal(err)
		}
		public, err := hospitals.GetPublic(ctx, hospital.ID)
		if err != nil || !strings.Contains(string(public.Facilities), "parking") {
			t.Fatalf("public hospital: %#v %v", public, err)
		}
		repo := doctorrepo.NewRepository(tx)
		department, err := repo.CreateDepartment(ctx, hospital.ID, "GENERAL", "General", now)
		if err != nil {
			t.Fatal(err)
		}
		newDepartment, err := repo.CreateDepartment(ctx, hospital.ID, "SPECIAL", "Special", now)
		if err != nil {
			t.Fatal(err)
		}
		room, err := repo.CreateRoom(ctx, hospital.ID, department.ID, "ROOM1", "Room One", now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateDepartment(ctx, uuid.NewString(), department.ID, map[string]any{"name": "wrong tenant"}); !errors.Is(err, doctorrepo.ErrPlacementNotFound) {
			t.Fatalf("cross-tenant department mutation: %v", err)
		}
		specificDay := now.Add(72 * time.Hour)
		specificDate := specificDay.Format("2006-01-02")
		makeInvitation := func(departmentID string, roomID *string) string {
			t.Helper()
			id := uuid.NewString()
			created, err := repo.CreateInvitation(ctx, doctorrepo.CreateInvitationInput{
				InvitationID: id, HospitalID: hospital.ID, DoctorID: doctorID, DepartmentID: departmentID, RoomID: roomID, InvitedBy: actorID,
				ExpiresAt: now.Add(24 * time.Hour), Now: now,
				Contract:  doctorrepo.Document{Filename: "contract.pdf", MIMEType: "application/pdf", Bucket: "test", ObjectPath: "lifecycle/" + id + ".pdf", FileSize: 10, SHA256: strings.Repeat("a", 64)},
				Schedules: []doctorrepo.Schedule{{DayOfWeek: int(specificDay.Weekday()), ScheduleDate: &specificDate, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(created.Schedules) != 1 || created.Schedules[0].ScheduleDate == nil || *created.Schedules[0].ScheduleDate != specificDate || created.Schedules[0].DayOfWeek != int(specificDay.Weekday()) {
				t.Fatalf("one-off invitation schedule was not retained: %#v", created.Schedules)
			}
			return id
		}
		invitationID := makeInvitation(department.ID, &room.ID)
		if err := repo.DeleteDepartment(ctx, hospital.ID, department.ID, now); !errors.Is(err, doctorrepo.ErrResourceInUse) {
			t.Fatalf("pending invitation must protect department: %v", err)
		}
		if err := repo.DeleteInvitation(ctx, uuid.NewString(), invitationID, actorID, now); !errors.Is(err, doctorrepo.ErrInvitationNotFound) {
			t.Fatalf("cross-tenant invitation deletion: %v", err)
		}
		clearRoom, message := "", "Updated terms"
		schedules := []doctorrepo.Schedule{{DayOfWeek: 1, StartTime: "10:00", EndTime: "11:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1}}
		updated, err := repo.UpdateInvitation(ctx, doctorrepo.UpdateInvitationInput{HospitalID: hospital.ID, InvitationID: invitationID, ActorID: actorID, DepartmentID: &newDepartment.ID, RoomID: &clearRoom, Message: &message, Schedules: &schedules, Now: now})
		if err != nil || updated.DepartmentID != newDepartment.ID || updated.RoomID != nil || len(updated.Schedules) != 1 || updated.Schedules[0].ScheduleDate != nil {
			t.Fatalf("update invitation: %#v %v", updated, err)
		}
		var auditedDate string
		if err := tx.Raw(`SELECT metadata->'previous_terms'->'schedules'->0->>'schedule_date' FROM doctor_hospital_invitation_events WHERE invitation_id = ? AND event_type = 'UPDATED'`, invitationID).Scan(&auditedDate).Error; err != nil || auditedDate != specificDate {
			t.Fatalf("draft update lost the previous one-off schedule audit: date=%q err=%v", auditedDate, err)
		}
		if err := repo.DeleteDepartment(ctx, hospital.ID, department.ID, now); err != nil {
			t.Fatalf("unused department deletion: %v", err)
		}
		rooms, err := repo.ListRooms(ctx, hospital.ID, department.ID)
		if err != nil || len(rooms) != 0 {
			t.Fatalf("department deletion must hide its rooms: %#v %v", rooms, err)
		}
		if err := repo.DeleteInvitation(ctx, hospital.ID, invitationID, actorID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetInvitationForHospital(ctx, hospital.ID, invitationID, now); !errors.Is(err, doctorrepo.ErrInvitationNotFound) {
			t.Fatalf("archived invitation visible: %v", err)
		}
		if _, err := repo.GetContractForHospital(ctx, invitationID, hospital.ID, "original"); !errors.Is(err, doctorrepo.ErrInvitationNotFound) {
			t.Fatalf("archived invitation contract visible: %v", err)
		}
		var contractCount int64
		if err := tx.Table("doctor_hospital_contracts").Where("invitation_id = ?", invitationID).Count(&contractCount).Error; err != nil || contractCount != 1 {
			t.Fatal("archiving erased contract history")
		}
		acceptedID := makeInvitation(newDepartment.ID, nil)
		if err := repo.AcceptInvitation(ctx, acceptedID, doctorID, now); err != nil {
			t.Fatal(err)
		}
		var inherited struct {
			ScheduleDate string
			DayOfWeek    int
		}
		if err := tx.Raw(`SELECT schedule.schedule_date::text AS schedule_date, schedule.day_of_week
			FROM doctor_hospital_schedules schedule JOIN doctor_hospital_affiliations affiliation ON affiliation.id = schedule.affiliation_id
			WHERE affiliation.invitation_id = ? AND schedule.is_active = TRUE`, acceptedID).Scan(&inherited).Error; err != nil || inherited.ScheduleDate != specificDate || inherited.DayOfWeek != int(specificDay.Weekday()) {
			t.Fatalf("acceptance did not copy the one-off schedule: %+v err=%v", inherited, err)
		}
		if err := repo.DeleteInvitation(ctx, hospital.ID, acceptedID, actorID, now); !errors.Is(err, doctorrepo.ErrInvalidInvitationState) {
			t.Fatalf("accepted contract deletion: %v", err)
		}
		if err := repo.DeleteDepartment(ctx, hospital.ID, newDepartment.ID, now); !errors.Is(err, doctorrepo.ErrResourceInUse) {
			t.Fatalf("active affiliation must protect placement: %v", err)
		}
		if err := repo.DeleteAffiliation(ctx, hospital.ID, doctorID, actorID, now); err != nil {
			t.Fatal(err)
		}
		doctors, err := repo.ListHospitalDoctors(ctx, hospital.ID, "")
		if err != nil || len(doctors) != 0 {
			t.Fatalf("archived affiliation visible: %#v %v", doctors, err)
		}
		if err := repo.UpdateAffiliationStatus(ctx, hospital.ID, doctorID, "ACTIVE", actorID, now); !errors.Is(err, doctorrepo.ErrAffiliationNotFound) {
			t.Fatalf("archived affiliation reactivated: %v", err)
		}
		pendingID := makeInvitation(newDepartment.ID, nil) // A fresh invitation can reuse a removed placement.
		if err := tx.Exec(`UPDATE users SET status = 'blocked', deleted_at = ? WHERE id = ?`, now, doctorID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateInvitation(ctx, doctorrepo.UpdateInvitationInput{HospitalID: hospital.ID, InvitationID: pendingID, ActorID: actorID, Message: &message, Now: now}); !errors.Is(err, doctorrepo.ErrDoctorNotEligible) {
			t.Fatalf("pending invitation to a deleted doctor was editable: %v", err)
		}
		if err := repo.DeleteDepartment(ctx, hospital.ID, newDepartment.ID, now); err != nil {
			t.Fatalf("a deleted doctor's hidden invitation must not prevent placement deletion: %v", err)
		}
		if err := hospitals.DeleteHospital(ctx, hospital.ID, actorID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := hospitals.GetPublic(ctx, hospital.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("deleted hospital visible: %v", err)
		}
		var retained int64
		if err := tx.Table("doctor_hospital_invitations").Where("hospital_id = ?", hospital.ID).Count(&retained).Error; err != nil || retained != 3 {
			t.Fatalf("invitation history lost: %d %v", retained, err)
		}
	})
}
