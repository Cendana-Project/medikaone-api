package seeder

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

type sampleUserSeed struct {
	Email     string
	FirstName string
	LastName  string
	Password  string
	RoleSlug  string
	Phone     string
	Gender    string
	NIK       string
	DOB       string
	Address   string
}

func sampleUserSeeds() []sampleUserSeed {
	genNIK := func(prefix string, i int) string {
		base := fmt.Sprintf("%s%012d", prefix, i)
		if len(base) > 16 {
			return base[:16]
		}
		return base
	}

	users := []sampleUserSeed{{
		Email:     "superadmin@medikaone.id",
		FirstName: "Super",
		LastName:  "Admin",
		Password:  "Password123",
		RoleSlug:  constant.RoleSuperAdmin,
		Phone:     "081270000001",
		Gender:    "L",
		NIK:       genNIK("1001", 1),
		DOB:       "1970-01-01",
		Address:   "Jl. Pusat No. 1, Jakarta",
	}}

	for i := 1; i <= 3; i++ {
		users = append(users, sampleUserSeed{
			Email:     fmt.Sprintf("patient%03d@medikaone.id", i),
			FirstName: "Patient",
			LastName:  fmt.Sprintf("%03d", i),
			Password:  "Password123",
			RoleSlug:  constant.RolePatient,
			Phone:     fmt.Sprintf("081200000%03d", i),
			Gender:    []string{"L", "P"}[i%2],
			NIK:       genNIK("1101", i),
			DOB:       "1990-01-01",
			Address:   "Jl. Contoh No. 123, Jakarta",
		})
	}

	for i := 1; i <= 3; i++ {
		users = append(users, sampleUserSeed{
			Email:     fmt.Sprintf("doctor%03d@medikaone.id", i),
			FirstName: "Doctor",
			LastName:  fmt.Sprintf("%03d", i),
			Password:  "Password123",
			RoleSlug:  constant.RoleDoctor,
			Phone:     fmt.Sprintf("081210000%03d", i),
			Gender:    []string{"L", "P"}[i%2],
			NIK:       genNIK("1201", i),
			DOB:       "1985-02-02",
			Address:   "Jl. Sehat No. 45, Jakarta",
		})
	}

	users = append(users,
		sampleUserSeed{Email: "admin001@medikaone.id", FirstName: "Admin", LastName: "001", Password: "Password123", RoleSlug: constant.RoleAdmin, Phone: "081230000001", Gender: "L", NIK: genNIK("1301", 1), DOB: "1980-03-03", Address: "Jl. Klinik No. 1, Jakarta"},
		sampleUserSeed{Email: "nurse001@medikaone.id", FirstName: "Nurse", LastName: "001", Password: "Password123", RoleSlug: constant.RoleNurse, Phone: "081240000001", Gender: "P", NIK: genNIK("1401", 1), DOB: "1992-04-04", Address: "Jl. Perawat No. 7, Jakarta"},
		sampleUserSeed{Email: "receptionist001@medikaone.id", FirstName: "Receptionist", LastName: "001", Password: "Password123", RoleSlug: constant.RoleReceptionist, Phone: "081250000001", Gender: "P", NIK: genNIK("1501", 1), DOB: "1993-05-05", Address: "Jl. Lobi No. 2, Jakarta"},
		sampleUserSeed{Email: "bod001@medikaone.id", FirstName: "BOD", LastName: "001", Password: "Password123", RoleSlug: constant.RoleBOD, Phone: "081260000001", Gender: "L", NIK: genNIK("1601", 1), DOB: "1975-06-06", Address: "Jl. Direktur No. 9, Jakarta"},
	)

	return users
}

func demoUserEmails() []string {
	users := sampleUserSeeds()
	emails := make([]string, 0, len(users))
	for _, user := range users {
		emails = append(emails, user.Email)
	}
	return emails
}

func demoUserSeedKey(email string) string {
	return "medikaone:user:" + strings.ToLower(strings.TrimSpace(email))
}

func demoUserSeedKeys() []string {
	users := sampleUserSeeds()
	keys := make([]string, 0, len(users)+1)
	for _, user := range users {
		keys = append(keys, demoUserSeedKey(user.Email))
	}
	// A SUPERADMIN_EMAIL fixture uses one stable key even if its configured
	// address changes between deployments.
	keys = append(keys, envSuperadminSeedKey)
	return keys
}

func SeedSampleUsers(db *gorm.DB) error {
	for i, user := range sampleUserSeeds() {
		created, err := CreateDemoUserActive(
			db,
			demoUserSeedKey(user.Email),
			user.Email,
			user.FirstName,
			user.LastName,
			user.Password,
			user.RoleSlug,
		)
		if err != nil {
			return err
		}

		var dob *time.Time
		if user.DOB != "" {
			parsed, parseErr := time.Parse("2006-01-02", user.DOB)
			if parseErr != nil {
				return fmt.Errorf("parse DOB for %s: %w", user.Email, parseErr)
			}
			dob = &parsed
		}
		updates := map[string]any{
			"phone":   user.Phone,
			"address": user.Address,
			"gender":  user.Gender,
			"nik":     user.NIK,
			"dob":     dob,
		}
		if err := db.Model(&entity.User{}).Where("id = ?", created.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update demo user %d (%s): %w", i, user.Email, err)
		}
	}
	return nil
}
