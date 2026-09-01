package seeder

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type hospitalSeed struct {
	SeedKey     string
	Code        string
	Name        string
	Address     string
	City        string
	Province    string
	Country     string
	Latitude    *float64
	Longitude   *float64
	Phone       string
	Description string
}

func SeedHospitals(db *gorm.DB) error {
	now := time.Now()

	lat1, lon1 := -6.200000, 106.816666 // Jakarta
	lat2, lon2 := -6.914744, 107.609810 // Bandung

	items := []hospitalSeed{
		{
			SeedKey:     "medikaone:hospital:general-jakarta",
			Code:        "HSP-MO-001",
			Name:        "MedikaOne General Hospital",
			Address:     "Jl. Kesehatan No. 1",
			City:        "Jakarta",
			Province:    "DKI Jakarta",
			Country:     "Indonesia",
			Latitude:    &lat1,
			Longitude:   &lon1,
			Phone:       "+62211234567",
			Description: "Rumah sakit umum MedikaOne",
		},
		{
			SeedKey:     "medikaone:hospital:clinic-bandung",
			Code:        "HSP-MO-002",
			Name:        "MedikaOne Clinic Bandung",
			Address:     "Jl. Sehat No. 2",
			City:        "Bandung",
			Province:    "Jawa Barat",
			Country:     "Indonesia",
			Latitude:    &lat2,
			Longitude:   &lon2,
			Phone:       "+622287654321",
			Description: "Klinik MedikaOne Bandung",
		},
	}

	for _, h := range items {
		// Fixture ownership is tracked by an internal immutable seed key. Never
		// adopt a row merely because it currently owns the canonical code.
		updated := db.Exec(`
			UPDATE hospitals
			SET seed_key = ?,
				code = ?,
				name = ?,
				address = ?,
				city = ?,
				province = ?,
				country = ?,
				latitude = ?,
				longitude = ?,
				phone = ?,
				description = ?,
				is_active = TRUE,
				updated_at = ?,
				deleted_at = NULL
			WHERE seed_key = ?
		`, h.SeedKey, h.Code, h.Name, h.Address, h.City, h.Province, h.Country, h.Latitude, h.Longitude,
			h.Phone, h.Description, now, h.SeedKey)
		if updated.Error != nil {
			return fmt.Errorf("restore hospital %s: %w", h.Code, updated.Error)
		}
		if updated.RowsAffected > 0 {
			continue
		}

		if err := db.Exec(`
			INSERT INTO hospitals (id, seed_key, code, name, address, city, province, country, latitude, longitude, phone, description, is_active, created_at, updated_at)
			VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, true, ?, ?)
		`, h.SeedKey, h.Code, h.Name, h.Address, h.City, h.Province, h.Country, h.Latitude, h.Longitude, h.Phone, h.Description, now, now).Error; err != nil {
			return fmt.Errorf("upsert hospital %s: %w", h.Code, err)
		}
	}
	return nil
}

func demoHospitalCodes() []string {
	return []string{"hsp-mo-001", "hsp-mo-002"}
}
