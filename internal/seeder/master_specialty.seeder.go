package seeder

import (
	"fmt"

	"gorm.io/gorm"
)

type masterSpecialtySeed struct {
	Code      string
	Name      string
	Category  string
	SortOrder int
}

// Local curated clinical fields, not exhaustive and not external terminology codes.
// Keep aligned with 20261017120000_specialty_master.sql and its source notes.
func masterSpecialtySeeds() []masterSpecialtySeed {
	return []masterSpecialtySeed{
		{Code: "SP-UMUM", Name: "Dokter Umum", Category: "GENERAL", SortOrder: 10},
		{Code: "SP-GIGI-UMUM", Name: "Dokter Gigi Umum", Category: "GENERAL", SortOrder: 20},
		{Code: "SP-PENYAKIT-DALAM", Name: "Penyakit Dalam", Category: "MEDICAL_SPECIALIST", SortOrder: 30},
		{Code: "SP-ANAK", Name: "Anak", Category: "MEDICAL_SPECIALIST", SortOrder: 40},
		{Code: "SP-OBGYN", Name: "Obstetri dan Ginekologi", Category: "MEDICAL_SPECIALIST", SortOrder: 50},
		{Code: "SP-MATA", Name: "Mata", Category: "MEDICAL_SPECIALIST", SortOrder: 60},
		{Code: "SP-THT-KL", Name: "Telinga Hidung Tenggorok Bedah Kepala dan Leher", Category: "MEDICAL_SPECIALIST", SortOrder: 70},
		{Code: "SP-SARAF", Name: "Neurologi", Category: "MEDICAL_SPECIALIST", SortOrder: 80},
		{Code: "SP-JANTUNG", Name: "Jantung dan Pembuluh Darah", Category: "MEDICAL_SPECIALIST", SortOrder: 90},
		{Code: "SP-KULIT-KELAMIN", Name: "Dermatologi, Venereologi, dan Estetika", Category: "MEDICAL_SPECIALIST", SortOrder: 100},
		{Code: "SP-JIWA", Name: "Kedokteran Jiwa", Category: "MEDICAL_SPECIALIST", SortOrder: 110},
		{Code: "SP-PARU", Name: "Pulmonologi dan Kedokteran Respirasi", Category: "MEDICAL_SPECIALIST", SortOrder: 120},
		{Code: "SP-GIZI-KLINIK", Name: "Gizi Klinik", Category: "MEDICAL_SPECIALIST", SortOrder: 130},
		{Code: "SP-BEDAH-UMUM", Name: "Bedah", Category: "MEDICAL_SPECIALIST", SortOrder: 140},
		{Code: "SP-ORTHOPAEDI-TRAUMATOLOGI", Name: "Orthopaedi dan Traumatologi", Category: "MEDICAL_SPECIALIST", SortOrder: 150},
		{Code: "SP-UROLOGI", Name: "Urologi", Category: "MEDICAL_SPECIALIST", SortOrder: 160},
		{Code: "SP-BEDAH-SARAF", Name: "Bedah Saraf", Category: "MEDICAL_SPECIALIST", SortOrder: 170},
		{Code: "SP-BEDAH-PLASTIK", Name: "Bedah Plastik Rekonstruksi dan Estetika", Category: "MEDICAL_SPECIALIST", SortOrder: 180},
		{Code: "SP-BEDAH-ANAK", Name: "Bedah Anak", Category: "MEDICAL_SPECIALIST", SortOrder: 190},
		{Code: "SP-BTKV", Name: "Bedah Toraks, Kardiak, dan Vaskular", Category: "MEDICAL_SPECIALIST", SortOrder: 200},
		{Code: "SP-ANESTESIOLOGI", Name: "Anestesiologi dan Terapi Intensif", Category: "MEDICAL_SPECIALIST", SortOrder: 210},
		{Code: "SP-REHABILITASI-MEDIK", Name: "Kedokteran Fisik dan Rehabilitasi", Category: "MEDICAL_SPECIALIST", SortOrder: 220},
		{Code: "SP-RADIOLOGI", Name: "Radiologi", Category: "MEDICAL_SPECIALIST", SortOrder: 230},
		{Code: "SP-PATOLOGI-KLINIK", Name: "Patologi Klinik", Category: "MEDICAL_SPECIALIST", SortOrder: 240},
		{Code: "SP-PATOLOGI-ANATOMIK", Name: "Patologi Anatomik", Category: "MEDICAL_SPECIALIST", SortOrder: 250},
		{Code: "SP-MIKROBIOLOGI-KLINIK", Name: "Mikrobiologi Klinik", Category: "MEDICAL_SPECIALIST", SortOrder: 260},
		{Code: "SP-ONKOLOGI-RADIASI", Name: "Onkologi Radiasi", Category: "MEDICAL_SPECIALIST", SortOrder: 270},
		{Code: "SP-KEDOKTERAN-NUKLIR", Name: "Kedokteran Nuklir dan Teranostik Molekuler", Category: "MEDICAL_SPECIALIST", SortOrder: 280},
		{Code: "SP-BEDAH-MULUT", Name: "Bedah Mulut dan Maksilofasial", Category: "DENTAL_SPECIALIST", SortOrder: 290},
		{Code: "SP-KONSERVASI-GIGI", Name: "Konservasi Gigi", Category: "DENTAL_SPECIALIST", SortOrder: 300},
		{Code: "SP-ORTODONSIA", Name: "Ortodonsia", Category: "DENTAL_SPECIALIST", SortOrder: 310},
		{Code: "SP-PERIODONSIA", Name: "Periodonsia", Category: "DENTAL_SPECIALIST", SortOrder: 320},
		{Code: "SP-PROSTODONSIA", Name: "Prostodonsia", Category: "DENTAL_SPECIALIST", SortOrder: 330},
		{Code: "SP-GIGI-ANAK", Name: "Kedokteran Gigi Anak", Category: "DENTAL_SPECIALIST", SortOrder: 340},
		{Code: "SP-PENYAKIT-MULUT", Name: "Penyakit Mulut", Category: "DENTAL_SPECIALIST", SortOrder: 350},
		{Code: "SP-RADIOLOGI-GIGI", Name: "Radiologi Kedokteran Gigi", Category: "DENTAL_SPECIALIST", SortOrder: 360},
	}
}

// SeedMasterSpecialties restores owned definitions without deleting unknown rows.
// Production installs its initial catalogue through the migration, not this seed.
func SeedMasterSpecialties(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, master := range masterSpecialtySeeds() {
			if err := tx.Exec(`INSERT INTO master_specialties (code, name, category, sort_order)
				VALUES (?, ?, ?, ?) ON CONFLICT (code) DO UPDATE SET
					name = EXCLUDED.name,
					category = EXCLUDED.category,
					sort_order = EXCLUDED.sort_order,
					is_active = TRUE,
					updated_at = NOW()`, master.Code, master.Name, master.Category, master.SortOrder).Error; err != nil {
				return fmt.Errorf("seed master specialty %s: %w", master.Code, err)
			}
		}
		return nil
	})
}
