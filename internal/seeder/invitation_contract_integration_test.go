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

func testInvitationContractReplacementIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Run("pending invitation contract replacement retains audit and tenant scope", func(t *testing.T) {
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
		var masterID string
		if err := tx.Raw(`SELECT id::text FROM master_departments WHERE code = 'POLI-ANAK'`).Scan(&masterID).Error; err != nil || masterID == "" {
			t.Fatal("missing master department")
		}
		department, err := repo.CreateDepartment(ctx, hospitalID, masterID, now)
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.NewString()
		original := doctorrepo.Document{Filename: "old.pdf", MIMEType: "application/pdf", Bucket: "test", ObjectPath: id + "/old.pdf", FileSize: 10, SHA256: strings.Repeat("a", 64)}
		_, err = repo.CreateInvitation(ctx, doctorrepo.CreateInvitationInput{InvitationID: id, HospitalID: hospitalID, DoctorID: doctorID, DepartmentID: department.ID, InvitedBy: actorID, ExpiresAt: now.Add(time.Hour), Now: now, Contract: original})
		if err != nil {
			t.Fatal(err)
		}
		replacement := doctorrepo.Document{Filename: "new.pdf", MIMEType: "application/pdf", Bucket: "test", ObjectPath: id + "/new.pdf", FileSize: 12, SHA256: strings.Repeat("b", 64)}
		input := doctorrepo.UpdateInvitationInput{HospitalID: hospitalID, InvitationID: id, ActorID: actorID, Contract: &replacement, Now: now.Add(time.Minute)}
		wrongTenant := input
		wrongTenant.HospitalID = uuid.NewString()
		if _, err := repo.UpdateInvitation(ctx, wrongTenant); !errors.Is(err, doctorrepo.ErrInvitationNotFound) {
			t.Fatalf("wrong tenant updated contract: %v", err)
		}
		updated, err := repo.UpdateInvitation(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if updated.ContractFilename != replacement.Filename || updated.DepartmentID != department.ID {
			t.Fatalf("invalid update response: %#v", updated)
		}
		contract, err := repo.GetContractForDoctor(ctx, id, doctorID, "original")
		if err != nil || contract.ObjectPath != replacement.ObjectPath || contract.SHA256 != replacement.SHA256 {
			t.Fatalf("contract URL lookup still uses old file: %#v %v", contract, err)
		}
		var audit struct {
			PreviousPath, PreviousHash string
			Replaced                   bool
		}
		if err := tx.Raw(`SELECT metadata->'previous_terms'->'contract'->>'original_object_path' AS previous_path,
			metadata->'previous_terms'->'contract'->>'original_sha256' AS previous_hash,
			(metadata->>'contract_replaced')::boolean AS replaced FROM doctor_hospital_invitation_events
			WHERE invitation_id = ? AND event_type = 'UPDATED'`, id).Scan(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if audit.PreviousPath != original.ObjectPath || audit.PreviousHash != original.SHA256 || !audit.Replaced {
			t.Fatalf("previous contract missing in audit: %#v", audit)
		}
		expired := input
		expired.Now = now.Add(2 * time.Hour)
		if _, err := repo.UpdateInvitation(ctx, expired); !errors.Is(err, doctorrepo.ErrInvitationExpired) {
			t.Fatalf("expired update: %v", err)
		}
		if err := repo.AcceptInvitation(ctx, id, doctorID, now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateInvitation(ctx, input); !errors.Is(err, doctorrepo.ErrInvalidInvitationState) {
			t.Fatalf("accepted update: %v", err)
		}
		var events int64
		if err := tx.Raw(`SELECT COUNT(*) FROM doctor_hospital_invitation_events WHERE invitation_id = ? AND event_type = 'UPDATED'`, id).Scan(&events).Error; err != nil || events != 1 {
			t.Fatalf("failed updates left events: %d %v", events, err)
		}
	})
}
