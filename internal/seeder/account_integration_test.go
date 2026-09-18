package seeder

import (
	"context"
	"errors"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Called only by the existing explicitly gated disposable database suite.
func testAccountDeletionIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, scenario := range []string{"delete preserves profile", "verification failure rolls back", "revocation failure rolls back", "last administrator remains active"} {
		t.Run(scenario, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			id := uuid.NewString()
			if err := tx.Exec(`INSERT INTO users (id, email, first_name, password_hash, status) VALUES (?, ?, 'Lifecycle test', 'not-a-login-hash', 'active')`, id, id+"@example.test").Error; err != nil {
				t.Fatal(err)
			}
			if err := tx.Exec(`INSERT INTO patient_profiles (user_id, allergies) VALUES (?, 'retained test history')`, id).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "last administrator remains active" {
				if err := tx.Exec(`DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE UPPER(slug) = 'SUPER_ADMIN')`).Error; err != nil {
					t.Fatal(err)
				}
				if err := tx.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE UPPER(slug) = 'SUPER_ADMIN' AND active AND deleted_at IS NULL`, id).Error; err != nil {
					t.Fatal(err)
				}
			}
			verifyErr := errors.New("verification failed")
			revokeErr := errors.New("revocation failed")
			revoked := false
			err := userrepo.NewRepository(tx).DeleteAccount(context.Background(), id, func(account *entity.User) error {
				if account.ID != id {
					t.Fatal("wrong account locked")
				}
				if scenario == "verification failure rolls back" {
					return verifyErr
				}
				return nil
			}, func() error {
				revoked = true
				if scenario == "revocation failure rolls back" {
					return revokeErr
				}
				return nil
			})
			var state struct {
				Status  string
				Deleted bool
			}
			if queryErr := tx.Raw(`SELECT status, deleted_at IS NOT NULL AS deleted FROM users WHERE id = ?`, id).Scan(&state).Error; queryErr != nil {
				t.Fatal(queryErr)
			}
			if scenario == "delete preserves profile" {
				if err != nil || !revoked || state.Status != "blocked" || !state.Deleted {
					t.Fatalf("delete result: %v state=%+v revoked=%v", err, state, revoked)
				}
				var count int64
				if queryErr := tx.Table("patient_profiles").Where("user_id = ?", id).Count(&count).Error; queryErr != nil || count != 1 {
					t.Fatalf("clinical profile was removed: %v count=%d", queryErr, count)
				}
			} else {
				if err == nil || state.Status != "active" || state.Deleted {
					t.Fatalf("failed deletion changed account: %v state=%+v", err, state)
				}
				if scenario == "last administrator remains active" && (!errors.Is(err, userrepo.ErrLastAccountAdministrator) || revoked) {
					t.Fatalf("last administrator guard failed: %v", err)
				}
			}
		})
	}
	testTenantAdministratorDeletionIntegration(t, db)
}

func testTenantAdministratorDeletionIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, scenario := range []struct {
		name           string
		replacement    string
		secondHospital bool
		wantDelete     bool
	}{
		{name: "last tenant administrator remains active"},
		{name: "active tenant replacement permits deletion", replacement: "active", wantDelete: true},
		{name: "inactive membership cannot replace administrator", replacement: "inactive"},
		{name: "deleted membership cannot replace administrator", replacement: "deleted"},
		{name: "blocked account cannot replace administrator", replacement: "blocked"},
		{name: "administrator in another hospital cannot replace administrator", replacement: "other-hospital"},
		{name: "replacement is required in every hospital", replacement: "active", secondHospital: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			targetID, replacementID := uuid.NewString(), uuid.NewString()
			firstHospital, secondHospital := uuid.NewString(), uuid.NewString()
			for _, id := range []string{targetID, replacementID} {
				if err := tx.Exec(`INSERT INTO users (id, email, first_name, password_hash, status)
					VALUES (?, ?, 'Tenant lifecycle test', 'not-a-login-hash', 'active')`, id, id+"@example.test").Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{firstHospital, secondHospital} {
				if err := tx.Exec(`INSERT INTO hospitals (id, code, name, is_active) VALUES (?, ?, ?, TRUE)`, id, id, "Lifecycle hospital "+id).Error; err != nil {
					t.Fatal(err)
				}
			}
			addAdmin := func(userID, hospitalID string) {
				t.Helper()
				if err := tx.Exec(`INSERT INTO user_hospitals (user_id, hospital_id, is_active) VALUES (?, ?, TRUE)`, userID, hospitalID).Error; err != nil {
					t.Fatal(err)
				}
				result := tx.Exec(`INSERT INTO hospital_user_roles (user_id, hospital_id, role_id)
					SELECT ?, ?, id FROM roles WHERE UPPER(slug) = 'ADMIN' AND active AND deleted_at IS NULL`, userID, hospitalID)
				if result.Error != nil || result.RowsAffected != 1 {
					t.Fatalf("assign tenant administrator: %v, rows=%d", result.Error, result.RowsAffected)
				}
			}
			addAdmin(targetID, firstHospital)
			if scenario.secondHospital {
				addAdmin(targetID, secondHospital)
			}
			if scenario.replacement != "" {
				hospitalID := firstHospital
				if scenario.replacement == "other-hospital" {
					hospitalID = secondHospital
				}
				addAdmin(replacementID, hospitalID)
			}
			switch scenario.replacement {
			case "inactive":
				if err := tx.Exec(`UPDATE user_hospitals SET is_active = FALSE WHERE user_id = ?`, replacementID).Error; err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := tx.Exec(`UPDATE user_hospitals SET deleted_at = NOW() WHERE user_id = ?`, replacementID).Error; err != nil {
					t.Fatal(err)
				}
			case "blocked":
				if err := tx.Exec(`UPDATE users SET status = 'blocked' WHERE id = ?`, replacementID).Error; err != nil {
					t.Fatal(err)
				}
			}
			revoked := false
			err := userrepo.NewRepository(tx).DeleteAccount(context.Background(), targetID, func(*entity.User) error { return nil }, func() error { revoked = true; return nil })
			var state struct {
				Deleted           bool
				ActiveMemberships int64
			}
			if queryErr := tx.Raw(`SELECT deleted_at IS NOT NULL AS deleted,
				(SELECT COUNT(*) FROM user_hospitals WHERE user_id = ? AND is_active AND deleted_at IS NULL) AS active_memberships
				FROM users WHERE id = ?`, targetID, targetID).Scan(&state).Error; queryErr != nil {
				t.Fatal(queryErr)
			}
			if scenario.wantDelete {
				if err != nil || !revoked || !state.Deleted || state.ActiveMemberships != 0 {
					t.Fatalf("replacement should permit deletion: err=%v state=%+v revoked=%v", err, state, revoked)
				}
			} else if !errors.Is(err, userrepo.ErrLastAccountAdministrator) || revoked || state.Deleted || state.ActiveMemberships == 0 {
				t.Fatalf("last tenant administrator guard failed: err=%v state=%+v revoked=%v", err, state, revoked)
			}
		})
	}
}
