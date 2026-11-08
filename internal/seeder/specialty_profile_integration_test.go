package seeder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	rolerepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	authservice "github.com/Cendana-Project/medikaone-api/internal/service/auth"
	userservice "github.com/Cendana-Project/medikaone-api/internal/service/user"
)

// Called by the explicitly gated disposable PostgreSQL suite after migrations and role seeding.
func assertSpecialtyProfileIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousConfig := config.Env
	if config.Env == nil {
		config.Env = &config.EnvConfig{}
	}
	defer func() { config.Env = previousConfig }()
	for _, selection := range []string{"active", "missing", "inactive", "null", "omitted"} {
		t.Run("initial specialty "+selection, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			ctx := context.Background()
			userID, specialtyID := uuid.NewString(), uuid.NewString()
			specialtyCode := "SP-TEST-" + strings.ToUpper(specialtyID)
			if err := tx.Exec(`INSERT INTO users (id, email, first_name, last_name, password_hash, status) VALUES (?, ?, 'Before', 'Profile', 'test-hash', 'active')`, userID, userID+"@example.test").Error; err != nil {
				t.Fatal(err)
			}
			if selection != "missing" {
				if err := tx.Exec(`INSERT INTO master_specialties (id, code, name, category, sort_order, is_active) VALUES (?, ?, 'Canonical Field', 'MEDICAL_SPECIALIST', 900, ?)`, specialtyID, specialtyCode, selection != "inactive").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Exec(`INSERT INTO doctor_profiles (user_id, specialty_legacy) VALUES (?, 'Private legacy label')`, userID).Error; err != nil {
				t.Fatal(err)
			}
			users := userrepo.NewRepository(tx)
			service := authservice.NewService(users, rolerepo.NewRepository(tx), nil, nil, nil)
			body := map[string]any{"first_name": "After", "last_name": "Profile", "dob": "1980-01-01", "sip_number": "SIP-" + userID}
			if selection != "omitted" {
				body["specialty_id"] = nil
				if selection != "null" {
					body["specialty_id"] = specialtyID
				}
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			raw := json.RawMessage(encoded)
			result, err := service.SetProfile(ctx, userID, constant.RoleDoctor, &raw)
			if selection == "missing" || selection == "inactive" {
				if !errors.Is(err, constant.ErrMasterSpecialtyNotFound) {
					t.Fatalf("selection error = %v", err)
				}
				var state struct {
					FirstName string
					Legacy    *string
					Roles     int64
				}
				if err := tx.Raw(`SELECT u.first_name, dp.specialty_legacy AS legacy, (SELECT COUNT(*) FROM user_roles WHERE user_id = u.id) AS roles FROM users u JOIN doctor_profiles dp ON dp.user_id = u.id WHERE u.id = ?`, userID).Scan(&state).Error; err != nil {
					t.Fatal(err)
				}
				if state.FirstName != "Before" || state.Legacy == nil || *state.Legacy != "Private legacy label" || state.Roles != 0 {
					t.Fatalf("invalid selection partially persisted: %+v", state)
				}
				return
			}
			if err != nil || result == nil {
				t.Fatalf("set profile result=%#v err=%v", result, err)
			}
			profile, err := users.GetDoctorProfile(ctx, userID)
			if err != nil || profile == nil {
				t.Fatalf("read profile=%#v err=%v", profile, err)
			}
			if selection != "active" {
				if profile.SpecialtyID != nil || profile.SpecialtyCode != nil || profile.Specialty != nil || result.Profile.Specialty != nil {
					t.Fatalf("no selection must never expose legacy text: %#v / %#v", profile, result.Profile)
				}
				legacy := ""
				if selection == "omitted" {
					legacy = "Private legacy label"
				}
				assertSpecialtyArchive(t, tx, userID, "", legacy)
				return
			}
			if profile.SpecialtyID == nil || *profile.SpecialtyID != specialtyID || profile.SpecialtyCode == nil || *profile.SpecialtyCode != specialtyCode || profile.Specialty == nil || *profile.Specialty != "Canonical Field" || result.Profile.SpecialtyID == nil || *result.Profile.SpecialtyID != specialtyID || result.Profile.SpecialtyCode == nil || *result.Profile.SpecialtyCode != *profile.SpecialtyCode || result.Profile.Specialty == nil || *result.Profile.Specialty != *profile.Specialty {
				t.Fatalf("canonical selection output mismatch: %#v / %#v", profile, result.Profile)
			}
			assertSpecialtyArchive(t, tx, userID, specialtyID, "")
			patches := userservice.NewService(users, nil, 0, time.Minute)
			if err := tx.Exec(`UPDATE master_specialties SET name = 'Renamed Canonical Field', is_active = FALSE WHERE id = ?`, specialtyID).Error; err != nil {
				t.Fatal(err)
			}
			profile, err = users.GetDoctorProfile(ctx, userID)
			if err != nil || profile == nil || profile.Specialty == nil || *profile.Specialty != "Renamed Canonical Field" {
				t.Fatalf("existing inactive reference must retain current canonical display: %#v, %v", profile, err)
			}
			if err := tx.Exec(`UPDATE doctor_profiles SET specialty_legacy = 'Retained archive' WHERE user_id = ?`, userID).Error; err != nil {
				t.Fatal(err)
			}
			if err := service.CompleteDoctorProfile(ctx, userID, &request.DoctorProfileRequest{FirstName: "After", LastName: "Profile", SIPNumber: profile.SIPNumber}); err != nil {
				t.Fatalf("omitted replacement specialty must preserve inactive reference: %v", err)
			}
			assertSpecialtyArchive(t, tx, userID, specialtyID, "Retained archive")
			if err := patches.Update(ctx, userID, request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{PracticeStartedOn: request.NewPatchNull[string]()}}); err != nil {
				t.Fatalf("unrelated update must preserve inactive selection: %v", err)
			}
			assertSpecialtyArchive(t, tx, userID, specialtyID, "Retained archive")
			for _, invalidID := range []string{specialtyID, uuid.NewString()} {
				err := patches.Update(ctx, userID, request.UpdateUserProfileRequest{FirstName: request.NewPatchValue("Rejected"), DoctorProfile: &request.UpdateDoctorProfileRequest{SpecialtyID: request.NewPatchValue(invalidID)}})
				if !errors.Is(err, constant.ErrMasterSpecialtyNotFound) {
					t.Fatalf("inactive/missing PATCH error = %v", err)
				}
				assertSpecialtyArchive(t, tx, userID, specialtyID, "Retained archive")
				user, err := users.GetByID(ctx, userID)
				if err != nil || user.FirstName != "After" {
					t.Fatalf("failed PATCH changed user: %#v, %v", user, err)
				}
			}
			if err := tx.Exec(`UPDATE master_specialties SET is_active = TRUE WHERE id = ?`, specialtyID).Error; err != nil {
				t.Fatal(err)
			}
			if err := patches.Update(ctx, userID, request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{SpecialtyID: request.NewPatchValue(specialtyID)}}); err != nil {
				t.Fatal(err)
			}
			assertSpecialtyArchive(t, tx, userID, specialtyID, "")
			if err := tx.Exec(`UPDATE doctor_profiles SET specialty_legacy = 'Clear me' WHERE user_id = ?`, userID).Error; err != nil {
				t.Fatal(err)
			}
			if err := patches.Update(ctx, userID, request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{SpecialtyID: request.NewPatchNull[string]()}}); err != nil {
				t.Fatal(err)
			}
			assertSpecialtyArchive(t, tx, userID, "", "")
			profile, err = users.GetDoctorProfile(ctx, userID)
			if err != nil || profile.Specialty != nil || profile.SpecialtyID != nil || profile.SpecialtyCode != nil || profile.SIPNumber == nil || *profile.SIPNumber != "SIP-"+userID {
				t.Fatalf("clear output or SIP preservation failed: %#v, %v", profile, err)
			}
		})
	}
}

func assertSpecialtyArchive(t *testing.T, db *gorm.DB, userID, specialtyID, legacy string) {
	t.Helper()
	var state struct {
		SpecialtyID *string
		Legacy      *string
	}
	if err := db.Raw(`SELECT specialty_id, specialty_legacy AS legacy FROM doctor_profiles WHERE user_id = ?`, userID).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if (specialtyID == "" && state.SpecialtyID != nil) || (specialtyID != "" && (state.SpecialtyID == nil || *state.SpecialtyID != specialtyID)) || (legacy == "" && state.Legacy != nil) || (legacy != "" && (state.Legacy == nil || *state.Legacy != legacy)) {
		t.Fatalf("persisted specialty/archive = %+v, want %q/%q", state, specialtyID, legacy)
	}
}
