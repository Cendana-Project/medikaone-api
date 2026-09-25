package seeder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func testInvitationRejectionIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Run("rejection message is persisted without changing invitation terms", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		ctx, now := context.Background(), time.Now().UTC()
		hospitalID, err := getHospitalIDBySeedKey(tx, "medikaone:hospital:general-jakarta")
		if err != nil {
			t.Fatal(err)
		}
		doctorID, err := getUserIDBySeedKey(tx, demoUserSeedKey("doctor001@medikaone.id"))
		if err != nil {
			t.Fatal(err)
		}
		actorID, err := getUserIDBySeedKey(tx, demoUserSeedKey("admin001@medikaone.id"))
		if err != nil {
			t.Fatal(err)
		}
		repo := doctorrepo.NewRepository(tx)
		var masterDepartmentID string
		if err := tx.Raw(`SELECT id::text FROM master_departments WHERE code = 'POLI-ANAK'`).Scan(&masterDepartmentID).Error; err != nil || masterDepartmentID == "" {
			t.Fatal("missing child master department")
		}
		department, err := repo.CreateDepartment(ctx, hospitalID, masterDepartmentID, now)
		if err != nil {
			t.Fatal(err)
		}
		originalMessage, rejection := "Silakan bergabung dengan rumah sakit kami.", "Jadwal berbenturan dengan praktik lain."
		for _, message := range []*string{&rejection, nil} {
			id := uuid.NewString()
			_, err := repo.CreateInvitation(ctx, doctorrepo.CreateInvitationInput{
				InvitationID: id, HospitalID: hospitalID, DoctorID: doctorID, DepartmentID: department.ID,
				InvitedBy: actorID, Message: &originalMessage, ExpiresAt: now.Add(time.Hour), Now: now,
				Contract: doctorrepo.Document{Filename: "test.pdf", MIMEType: "application/pdf", Bucket: "test", ObjectPath: id + ".pdf", FileSize: 10, SHA256: strings.Repeat("a", 64)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.RejectInvitation(ctx, id, uuid.NewString(), message, now); !errors.Is(err, doctorrepo.ErrInvitationNotFound) {
				t.Fatalf("wrong doctor could reject: %v", err)
			}
			if err := repo.RejectInvitation(ctx, id, doctorID, message, now); err != nil {
				t.Fatal(err)
			}
			replacement := "Replacement must not overwrite first rejection"
			if err := repo.RejectInvitation(ctx, id, doctorID, &replacement, now); !errors.Is(err, doctorrepo.ErrInvalidInvitationState) {
				t.Fatalf("repeated rejection: %v", err)
			}
			assertMessage := func(label string, got *string) {
				t.Helper()
				if (got == nil) != (message == nil) || (got != nil && message != nil && *got != *message) {
					t.Fatalf("%s lost rejection message: %v", label, got)
				}
			}
			doctorDetail, err := repo.GetInvitationForDoctor(ctx, doctorID, id, now)
			if err != nil {
				t.Fatal(err)
			}
			assertMessage("doctor detail", doctorDetail.RejectionReason)
			if doctorDetail.Status != "REJECTED" || doctorDetail.RespondedAt == nil || doctorDetail.Message == nil || *doctorDetail.Message != originalMessage {
				t.Fatalf("incorrect invitation state: %#v", doctorDetail)
			}
			hospitalDetail, err := repo.GetInvitationForHospital(ctx, hospitalID, id, now)
			if err != nil {
				t.Fatal(err)
			}
			assertMessage("hospital detail", hospitalDetail.RejectionReason)
			for _, forDoctor := range []bool{true, false} {
				items, err := repo.ListInvitationsForHospital(ctx, hospitalID, "REJECTED", now)
				if forDoctor {
					items, err = repo.ListInvitationsForDoctor(ctx, doctorID, "REJECTED", now)
				}
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range items {
					if item.ID == id {
						found = true
						assertMessage("invitation list", item.RejectionReason)
					}
				}
				if !found {
					t.Fatal("rejected invitation missing from list")
				}
			}
			for _, query := range []string{
				`SELECT metadata->>'rejection_reason' AS reason FROM doctor_hospital_invitation_events WHERE invitation_id = ? AND event_type = 'REJECTED'`,
				`SELECT data->>'rejection_reason' AS reason FROM notifications WHERE data->>'invitation_id' = ? AND type = 'DOCTOR_HOSPITAL_INVITATION_REJECTED'`,
			} {
				var rows []struct{ Reason *string }
				if err := tx.Raw(query, id).Scan(&rows).Error; err != nil || len(rows) != 1 {
					t.Fatalf("expected exactly one rejection event/notification: %v (%d rows)", err, len(rows))
				}
				assertMessage("audit/notification", rows[0].Reason)
			}
		}
	})
}
