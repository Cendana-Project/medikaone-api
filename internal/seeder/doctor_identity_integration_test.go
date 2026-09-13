package seeder

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	doctorhospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
)

// Called by the existing isolated, explicitly gated PostgreSQL integration suite.
func testDoctorIdentityIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Run("doctor identity remains unique immutable and public", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		ctx := context.Background()
		var doctorID string
		if err := tx.Raw(`SELECT id::text FROM users WHERE email = 'doctor001@medikaone.id'`).Scan(&doctorID).Error; err != nil || doctorID == "" {
			t.Fatalf("find seeded doctor: id=%q err=%v", doctorID, err)
		}
		users := userrepo.NewRepository(tx)
		profile, err := users.GetDoctorProfile(ctx, doctorID)
		if err != nil || profile == nil || !regexp.MustCompile(`^MDO-[0-9A-F]{16}$`).MatchString(profile.DoctorMedikaOneID) {
			t.Fatalf("seeded doctor profile public identity = %#v, %v", profile, err)
		}
		originalID := profile.DoctorMedikaOneID
		if err := users.UpsertDoctorProfile(ctx, map[string]any{"user_id": doctorID, "sip_number": profile.SIPNumber, "specialty": "Integration Specialty"}); err != nil {
			t.Fatal(err)
		}
		profile, err = users.GetDoctorProfile(ctx, doctorID)
		if err != nil || profile.DoctorMedikaOneID != originalID {
			t.Fatalf("profile upsert changed identity: %#v, %v", profile, err)
		}
		if err := tx.Exec("SAVEPOINT doctor_identity_immutable").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`UPDATE doctor_profiles SET medikaone_id = 'MDO-0000000000000000' WHERE user_id = ?`, doctorID).Error; err == nil {
			t.Fatal("database accepted a doctor identity change")
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT doctor_identity_immutable").Error; err != nil {
			t.Fatal(err)
		}
		newUserID := uuid.NewString()
		if err := tx.Exec(`INSERT INTO users (id, email, password_hash, status) VALUES (?, ?, 'test-hash', 'active')`, newUserID, newUserID+"@example.test").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("SAVEPOINT doctor_identity_unique").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO doctor_profiles (user_id, medikaone_id) VALUES (?, ?)`, newUserID, originalID).Error; err == nil {
			t.Fatal("database accepted duplicate doctor public identity")
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT doctor_identity_unique").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO doctor_profiles (user_id) VALUES (?)`, newUserID).Error; err != nil {
			t.Fatal(err)
		}
		newProfile, err := users.GetDoctorProfile(ctx, newUserID)
		if err != nil || newProfile == nil || newProfile.DoctorMedikaOneID == "" || newProfile.DoctorMedikaOneID == originalID {
			t.Fatalf("new doctor ID default = %#v, %v", newProfile, err)
		}
		if completed, err := users.ExistsDoctorProfile(ctx, newUserID); err != nil || completed {
			t.Fatalf("ID-only profile must remain eligible for initial completion: completed=%v err=%v", completed, err)
		}
		assignedUserID := uuid.NewString()
		if err := tx.Exec(`INSERT INTO users (id, email, password_hash, status) VALUES (?, ?, 'test-hash', 'active')`, assignedUserID, assignedUserID+"@example.test").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE UPPER(slug) = 'DOCTOR' AND active AND deleted_at IS NULL`, assignedUserID).Error; err != nil {
			t.Fatal(err)
		}
		assignedProfile, err := users.GetDoctorProfile(ctx, assignedUserID)
		if err != nil || assignedProfile == nil || assignedProfile.DoctorMedikaOneID == "" {
			t.Fatalf("doctor role assignment must create identity: %#v, %v", assignedProfile, err)
		}
		if completed, err := users.ExistsDoctorProfile(ctx, assignedUserID); err != nil || completed {
			t.Fatalf("new role assignment must still allow profile completion: completed=%v err=%v", completed, err)
		}
		directory := doctorrepo.NewRepository(tx)
		byUUID, err := directory.GetDoctor(ctx, doctorID)
		if err != nil || byUUID.DoctorMedikaOneID != originalID {
			t.Fatalf("directory UUID lookup = %#v, %v", byUUID, err)
		}
		byPublicID, err := directory.GetDoctor(ctx, originalID)
		if err != nil || byPublicID.DoctorID != doctorID {
			t.Fatalf("directory public ID lookup = %#v, %v", byPublicID, err)
		}
		page, err := directory.ListDoctors(ctx, doctorrepo.Filter{Query: originalID, Page: 1, Limit: 20})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].DoctorMedikaOneID != originalID {
			t.Fatalf("directory identity search = %#v, %v", page, err)
		}
		eligible, err := doctorhospitalrepo.NewRepository(tx).SearchEligibleDoctors(ctx, originalID, 20)
		if err != nil || len(eligible) != 1 || eligible[0].DoctorMedikaOneID != originalID {
			t.Fatalf("hospital invitation doctor search = %#v, %v", eligible, err)
		}
		testDoctorDirectoryAffiliation(t, tx, doctorID, originalID)
		if err := tx.Exec(`UPDATE users SET deleted_at = NOW() WHERE id = ?`, doctorID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := directory.GetDoctor(ctx, originalID); err != gorm.ErrRecordNotFound {
			t.Fatalf("directory must hide deleted accounts, got %v", err)
		}
		page, err = directory.ListDoctors(ctx, doctorrepo.Filter{Query: originalID, Page: 1, Limit: 20})
		if err != nil || page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("directory list must hide deleted accounts: %#v, %v", page, err)
		}
	})
}

func testDoctorDirectoryAffiliation(t *testing.T, tx *gorm.DB, doctorID, publicID string) {
	t.Helper()
	ctx := context.Background()
	var hospitalID, adminID string
	if err := tx.Raw(`SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001'`).Scan(&hospitalID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw(`SELECT id::text FROM users WHERE email = 'admin001@medikaone.id'`).Scan(&adminID).Error; err != nil {
		t.Fatal(err)
	}
	departmentID, invitationID, affiliationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	if err := tx.Exec(`INSERT INTO hospital_departments (id, hospital_id, code, name, created_at, updated_at)
		VALUES (?, ?, 'IT-DIRECTORY', 'Directory Department', ?, ?)`, departmentID, hospitalID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO doctor_hospital_invitations (id, hospital_id, doctor_id, department_id, invited_by, status, expires_at, responded_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'ACCEPTED', ?, ?, ?, ?)`, invitationID, hospitalID, doctorID, departmentID, adminID, now.Add(24*time.Hour), now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO doctor_hospital_affiliations (id, hospital_id, doctor_id, department_id, invitation_id, status, joined_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`, affiliationID, hospitalID, doctorID, departmentID, invitationID, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO doctor_hospital_schedules (affiliation_id, day_of_week, start_time, end_time, timezone, booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at)
		VALUES (?, 1, '08:00', '09:00', 'Asia/Jakarta', 'FIXED_SLOT', 30, 1, TRUE, ?, ?)`, affiliationID, now, now).Error; err != nil {
		t.Fatal(err)
	}
	date := now.Add(72 * time.Hour)
	if err := tx.Exec(`INSERT INTO doctor_hospital_schedules (affiliation_id, day_of_week, schedule_date, start_time, end_time, timezone, booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at)
		VALUES (?, ?, ?::date, '10:00', '11:00', 'Asia/Jakarta', 'FIXED_SLOT', 30, 1, TRUE, ?, ?)`, affiliationID, int(date.Weekday()), date.Format("2006-01-02"), now, now).Error; err != nil {
		t.Fatal(err)
	}
	directory := doctorrepo.NewRepository(tx)
	detail, err := directory.GetDoctor(ctx, publicID)
	if err != nil || len(detail.Affiliations) != 1 || len(detail.Affiliations[0].Schedules) != 2 {
		t.Fatalf("public directory affiliation and schedule projection = %#v, %v", detail, err)
	}
	schedules, err := appointmentrepo.NewRepository(tx).ListActiveSchedules(ctx, appointmentrepo.AvailabilityFilter{DoctorID: doctorID})
	if err != nil || len(schedules) != 2 || schedules[0].DoctorMedikaOneID != publicID || schedules[1].DoctorMedikaOneID != publicID {
		t.Fatalf("availability doctor identity = %#v, %v", schedules, err)
	}
	page, err := directory.ListDoctors(ctx, doctorrepo.Filter{HospitalID: hospitalID, Query: publicID, Page: 1, Limit: 20})
	if err != nil || page.Total != 1 {
		t.Fatalf("directory hospital filter = %#v, %v", page, err)
	}
	hospitalRepo := doctorhospitalrepo.NewRepository(tx)
	doctors, err := hospitalRepo.ListHospitalDoctors(ctx, hospitalID, "ACTIVE")
	if err != nil || len(doctors) != 1 || doctors[0].DoctorMedikaOneID != publicID {
		t.Fatalf("hospital doctor identity = %#v, %v", doctors, err)
	}
	data, err := json.Marshal(map[string]string{"doctor_id": doctorID})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO notifications (user_id, type, title, body, data, created_at) VALUES (?, 'TEST_DOCTOR_IDENTITY', 'Test', 'Test', ?::jsonb, ?)`, adminID, string(data), now).Error; err != nil {
		t.Fatal(err)
	}
	notifications, err := hospitalRepo.ListNotifications(ctx, adminID, false)
	if err != nil || len(notifications) == 0 {
		t.Fatalf("read doctor notification: %#v, %v", notifications, err)
	}
	var notificationIdentity map[string]string
	if err := json.Unmarshal(notifications[0].Data, &notificationIdentity); err != nil || notificationIdentity["doctor_medikaone_id"] != publicID {
		t.Fatalf("notification doctor identity = %#v, %v", notificationIdentity, err)
	}
	if err := tx.SavePoint("doctor_account_availability").Error; err != nil {
		t.Fatal(err)
	}
	if err := userrepo.NewRepository(tx).DeleteAccount(ctx, doctorID, func(*entity.User) error { return nil }, func() error { return nil }); err != nil {
		t.Fatalf("delete doctor account without live appointments: %v", err)
	}
	if _, err := directory.GetDoctor(ctx, publicID); err != gorm.ErrRecordNotFound {
		t.Fatalf("deleted doctor remains in public directory: %v", err)
	}
	schedules, err = appointmentrepo.NewRepository(tx).ListActiveSchedules(ctx, appointmentrepo.AvailabilityFilter{DoctorID: doctorID})
	if err != nil || len(schedules) != 0 {
		t.Fatalf("deleted doctor must have no available schedules: %#v, %v", schedules, err)
	}
	var preservedSchedules int64
	if err := tx.Table("doctor_hospital_schedules").Where("affiliation_id = ? AND is_active = TRUE", affiliationID).Count(&preservedSchedules).Error; err != nil || preservedSchedules != 2 {
		t.Fatalf("account deletion must preserve schedule records: count=%d, error=%v", preservedSchedules, err)
	}
	if err := tx.RollbackTo("doctor_account_availability").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE doctor_hospital_affiliations SET deleted_at = NOW() WHERE id = ?`, affiliationID).Error; err != nil {
		t.Fatal(err)
	}
	detail, err = directory.GetDoctor(ctx, publicID)
	if err != nil || len(detail.Affiliations) != 0 {
		t.Fatalf("directory must hide deleted affiliation: %#v, %v", detail, err)
	}
	page, err = directory.ListDoctors(ctx, doctorrepo.Filter{HospitalID: hospitalID, Query: publicID, Page: 1, Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("hospital directory must hide deleted affiliation: %#v, %v", page, err)
	}
}
