package seeder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	rolerepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	appointmentsvc "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
)

type demoFailingMedicalStorage struct{ *demoPracticeRecordingStorage }

func (s *demoFailingMedicalStorage) Upload(context.Context, string, string, []byte) (*storage.UploadedObject, error) {
	return nil, errors.New("integration medical storage unavailable")
}

func assertDemoWorkflowOrchestration(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	ctx := context.Background()
	now := time.Now()
	var invitationsBefore int64
	if err := tx.Table("doctor_hospital_invitations").Count(&invitationsBefore).Error; err != nil {
		t.Fatal(err)
	}
	contracts := &demoPracticeRecordingStorage{objects: map[string][]byte{"unrelated-original.pdf": []byte("keep-existing-object")}}
	unavailableMedical := &demoFailingMedicalStorage{&demoPracticeRecordingStorage{}}
	_, err := RunWithWorkflows(ctx, tx, contracts, unavailableMedical, now, "http://localhost:8080")
	if err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("failed real prescription upload must abort workflow seed: %v", err)
	}
	var invitationsAfter int64
	if err := tx.Table("doctor_hospital_invitations").Count(&invitationsAfter).Error; err != nil || invitationsAfter != invitationsBefore {
		t.Fatalf("failed workflow committed partial invitations: before=%d after=%d err=%v", invitationsBefore, invitationsAfter, err)
	}
	if len(contracts.objects) != 1 || string(contracts.objects["unrelated-original.pdf"]) != "keep-existing-object" {
		t.Fatalf("rollback cleanup removed an existing object or retained newly uploaded contracts: %v", contracts.objects)
	}

	medical := &demoPracticeRecordingStorage{}
	manifest, err := RunWithWorkflows(ctx, tx, contracts, medical, now, "http://localhost:8080/")
	if err != nil {
		t.Fatal(err)
	}
	roles := rolerepo.NewRepository(tx)
	for _, email := range []string{"doctor001@medikaone.id", "doctor002@medikaone.id", "doctor003@medikaone.id"} {
		doctorID, err := getUserIDBySeedKey(tx, demoUserSeedKey(email))
		if err != nil {
			t.Fatal(err)
		}
		permissions, err := roles.ListPermissionsByUser(ctx, doctorID)
		if err != nil {
			t.Fatal(err)
		}
		allowed := make(map[string]bool, len(permissions))
		for _, permission := range permissions {
			allowed[permission.Slug] = true
		}
		for _, required := range []string{constant.PermissionDoctorScheduleView, constant.PermissionExaminationConsultationWrite, constant.PermissionPrescriptionWrite} {
			if !allowed[required] {
				t.Fatalf("%s cannot exercise mobile doctor workflow: missing %s", email, required)
			}
		}
	}
	for _, email := range []string{"patient001@medikaone.id", "admin001@medikaone.id", "nurse001@medikaone.id", "receptionist001@medikaone.id"} {
		id, err := getUserIDBySeedKey(tx, demoUserSeedKey(email))
		if err != nil {
			t.Fatal(err)
		}
		hasDoctorRole, err := roles.UserHasRole(ctx, id, constant.RoleDoctor)
		if err != nil || hasDoctorRole {
			t.Fatalf("workflow granted mobile doctor access to %s: role=%v err=%v", email, hasDoctorRole, err)
		}
	}
	output := filepath.Join(t.TempDir(), "nested", "demo.local.yml")
	if err := manifest.WriteBrunoEnvironment(output); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var exported DemoManifest
	if err := yaml.Unmarshal(encoded, &exported); err != nil {
		t.Fatal(err)
	}
	variables := make(map[string]string, len(exported.Variables))
	for _, variable := range exported.Variables {
		variables[variable.Name] = variable.Value
	}
	if variables["demoBookingAvailable"] != "true" {
		t.Fatal("fresh workflow must supply a genuinely bookable choice")
	}
	for _, secretName := range []string{"DATABASE_DSN", "DATABASE_ADMIN_DSN", "REDIS_CACHE_DSN", "JWT_SECRET", "SUPABASE_SECRET_KEY", "mobileAccessToken", "websiteAccessToken"} {
		if _, exported := variables[secretName]; exported {
			t.Fatalf("environment must not export server credentials or issued tokens: %s", secretName)
		}
	}
	var patientID string
	if err := tx.Raw(`SELECT id FROM users WHERE seed_key=?`, demoUserSeedKey("patient001@medikaone.id")).Scan(&patientID).Error; err != nil {
		t.Fatal(err)
	}
	// Exercise the exported opaque choice as a real consumer would; display
	// metadata alone does not prove the environment can create an appointment.
	created, _, err := appointmentsvc.NewService(appointmentrepo.NewRepository(tx), nil, "integration-demo-secret").CreateAppointment(ctx, patientID, uuid.NewString(), "127.0.0.1", "demo-environment-integration", request.CreateAppointmentRequest{
		SlotID:         variables["slotId"],
		ReasonForVisit: "Verify exported demo availability", ConsentAccepted: true, ConsentVersion: "demo-integration-v1",
	})
	if err != nil {
		t.Fatalf("exported booking variables are not usable by consumer: %v", err)
	}
	if created.PatientID == nil || *created.PatientID != patientID || created.HospitalID != variables["hospitalId"] || created.DoctorID != variables["doctorId"] {
		t.Fatalf("exported selection booked another actor or tenant: %#v", created)
	}
	fixtureID := variables["confirmedAppointmentId"]
	if err := appointmentrepo.NewRepository(tx).Cancel(ctx, fixtureID, patientID, "Tester progressed this scenario", now); err != nil {
		t.Fatal(err)
	}
	beforeUploads := contracts.uploads + medical.uploads
	if _, err := RunWithWorkflows(ctx, tx, contracts, medical, now.Add(time.Minute), "http://localhost:8080"); err != nil {
		t.Fatal(err)
	}
	if contracts.uploads+medical.uploads != beforeUploads {
		t.Fatal("workflow rerun duplicated already persisted document uploads")
	}
	var status string
	if err := tx.Raw(`SELECT status FROM appointments WHERE id=?`, fixtureID).Scan(&status).Error; err != nil || status != "CANCELLED" {
		t.Fatalf("workflow rerun rewound tested appointment: status=%s err=%v", status, err)
	}
	var manualBookingID string
	if err := tx.Raw(`SELECT id FROM appointments WHERE id=?`, created.ID).Scan(&manualBookingID).Error; err != nil || manualBookingID != created.ID {
		t.Fatalf("workflow rerun changed manually created booking: %v", err)
	}
	if err := ResetDemoAndSeed(tx); err == nil || !strings.Contains(err.Error(), "refusing demo-only reset") {
		t.Fatalf("demo-user-only reset must fail closed with clinical history: %v", err)
	}
	if err := tx.Raw(`SELECT status FROM appointments WHERE id=?`, fixtureID).Scan(&status).Error; err != nil || status != "CANCELLED" {
		t.Fatalf("failed demo-only reset damaged retained clinical graph: %v", err)
	}
}
