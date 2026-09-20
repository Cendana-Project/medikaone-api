package seeder

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type departmentSeed struct {
	Code, Name, RoomCode, RoomName string
}

// SeedDepartments synchronizes only departments and rooms with fixture IDs in
// seed-owned hospitals. A matching public code on another row is a conflict,
// never permission to adopt that row as a fixture.
func SeedDepartments(db *gorm.DB) error {
	for _, hospital := range hospitalSeeds() {
		hospitalIDText, err := getHospitalIDBySeedKey(db, hospital.SeedKey)
		if err != nil {
			return fmt.Errorf("find fixture hospital %s: %w", hospital.Code, err)
		}
		hospitalID, err := uuid.Parse(hospitalIDText)
		if err != nil {
			return fmt.Errorf("parse fixture hospital ID: %w", err)
		}
		for _, department := range hospital.Departments {
			departmentID := uuid.NewSHA1(hospitalID, []byte("medikaone:department:"+department.Code))
			result := db.Exec(`INSERT INTO hospital_departments (id, hospital_id, code, name)
				VALUES (?, ?, ?, ?) ON CONFLICT (id) DO UPDATE
				SET code = EXCLUDED.code, name = EXCLUDED.name, is_active = TRUE, updated_at = NOW()
				WHERE hospital_departments.hospital_id = EXCLUDED.hospital_id`,
				departmentID, hospitalID, department.Code, department.Name)
			if result.Error != nil {
				return fmt.Errorf("seed department %s/%s: %w", hospital.Code, department.Code, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("fixture department %s/%s belongs to another hospital", hospital.Code, department.Code)
			}

			roomID := uuid.NewSHA1(hospitalID, []byte("medikaone:room:"+department.RoomCode))
			result = db.Exec(`INSERT INTO hospital_rooms (id, hospital_id, department_id, code, name)
				VALUES (?, ?, ?, ?, ?) ON CONFLICT (id) DO UPDATE
				SET code = EXCLUDED.code, name = EXCLUDED.name, is_active = TRUE, updated_at = NOW()
				WHERE hospital_rooms.hospital_id = EXCLUDED.hospital_id
				  AND hospital_rooms.department_id = EXCLUDED.department_id`,
				roomID, hospitalID, departmentID, department.RoomCode, department.RoomName)
			if result.Error != nil {
				return fmt.Errorf("seed room %s/%s: %w", hospital.Code, department.RoomCode, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("fixture room %s/%s belongs to another department", hospital.Code, department.RoomCode)
			}
		}
	}
	return nil
}
