package seeder

import (
	"fmt"

	"gorm.io/gorm"
)

type masterDepartmentSeed struct {
	Code      string
	Name      string
	Category  string
	SortOrder int
}

// masterDepartmentSeeds is the application-owned catalogue of selectable
// Indonesian hospital services. Codes are MedikaOne codes, not external
// SATUSEHAT, KKI, or SNOMED CT terminology identifiers.
func masterDepartmentSeeds() []masterDepartmentSeed {
	return []masterDepartmentSeed{
		{Code: "POLI-UMUM", Name: "Poli Umum", Category: "GENERAL", SortOrder: 10},
		{Code: "POLI-KKLP", Name: "Poli Kedokteran Keluarga dan Layanan Primer", Category: "GENERAL", SortOrder: 20},
		{Code: "POLI-GIGI-UMUM", Name: "Poli Gigi Umum", Category: "DENTAL", SortOrder: 30},
		{Code: "POLI-PENYAKIT-DALAM", Name: "Poli Penyakit Dalam", Category: "MEDICAL_SPECIALIST", SortOrder: 40},
		{Code: "POLI-ANAK", Name: "Poli Anak", Category: "MEDICAL_SPECIALIST", SortOrder: 50},
		{Code: "POLI-OBGYN", Name: "Poli Obstetri dan Ginekologi", Category: "MEDICAL_SPECIALIST", SortOrder: 60},
		{Code: "POLI-MATA", Name: "Poli Mata", Category: "MEDICAL_SPECIALIST", SortOrder: 70},
		{Code: "POLI-THT-KL", Name: "Poli THT, Kepala, dan Leher", Category: "MEDICAL_SPECIALIST", SortOrder: 80},
		{Code: "POLI-SARAF", Name: "Poli Saraf", Category: "MEDICAL_SPECIALIST", SortOrder: 90},
		{Code: "POLI-JANTUNG", Name: "Poli Jantung dan Pembuluh Darah", Category: "MEDICAL_SPECIALIST", SortOrder: 100},
		{Code: "POLI-KULIT-KELAMIN", Name: "Poli Dermatologi, Venereologi, dan Estetika", Category: "MEDICAL_SPECIALIST", SortOrder: 110},
		{Code: "POLI-JIWA", Name: "Poli Kedokteran Jiwa", Category: "MEDICAL_SPECIALIST", SortOrder: 120},
		{Code: "POLI-PARU", Name: "Poli Pulmonologi dan Kedokteran Respirasi", Category: "MEDICAL_SPECIALIST", SortOrder: 130},
		{Code: "POLI-GIZI-KLINIK", Name: "Poli Gizi Klinik", Category: "MEDICAL_SPECIALIST", SortOrder: 140},
		{Code: "POLI-ANDROLOGI", Name: "Poli Andrologi", Category: "MEDICAL_SPECIALIST", SortOrder: 150},
		{Code: "POLI-KEDOKTERAN-OLAHRAGA", Name: "Poli Kedokteran Olahraga", Category: "MEDICAL_SPECIALIST", SortOrder: 160},
		{Code: "POLI-KEDOKTERAN-OKUPASI", Name: "Poli Kedokteran Okupasi", Category: "MEDICAL_SPECIALIST", SortOrder: 170},
		{Code: "POLI-FARMAKOLOGI-KLINIK", Name: "Poli Farmakologi Klinik", Category: "MEDICAL_SPECIALIST", SortOrder: 180},
		{Code: "POLI-AKUPUNKTUR-MEDIK", Name: "Poli Akupunktur Medik", Category: "MEDICAL_SPECIALIST", SortOrder: 190},
		{Code: "POLI-KEDARURATAN-MEDIK", Name: "Poli Kedaruratan Medik", Category: "MEDICAL_SPECIALIST", SortOrder: 200},
		{Code: "POLI-BEDAH-UMUM", Name: "Poli Bedah Umum", Category: "SURGICAL_SPECIALIST", SortOrder: 210},
		{Code: "POLI-ORTHOPAEDI-TRAUMATOLOGI", Name: "Poli Orthopaedi dan Traumatologi", Category: "SURGICAL_SPECIALIST", SortOrder: 220},
		{Code: "POLI-UROLOGI", Name: "Poli Urologi", Category: "SURGICAL_SPECIALIST", SortOrder: 230},
		{Code: "POLI-BEDAH-SARAF", Name: "Poli Bedah Saraf", Category: "SURGICAL_SPECIALIST", SortOrder: 240},
		{Code: "POLI-BEDAH-PLASTIK", Name: "Poli Bedah Plastik Rekonstruksi dan Estetika", Category: "SURGICAL_SPECIALIST", SortOrder: 250},
		{Code: "POLI-BEDAH-ANAK", Name: "Poli Bedah Anak", Category: "SURGICAL_SPECIALIST", SortOrder: 260},
		{Code: "POLI-BTKV", Name: "Poli Bedah Toraks, Kardiak, dan Vaskular", Category: "SURGICAL_SPECIALIST", SortOrder: 270},
		{Code: "POLI-BEDAH-MULUT", Name: "Poli Bedah Mulut dan Maksilofasial", Category: "DENTAL", SortOrder: 280},
		{Code: "POLI-KONSERVASI-GIGI", Name: "Poli Konservasi Gigi", Category: "DENTAL", SortOrder: 290},
		{Code: "POLI-ORTODONSIA", Name: "Poli Ortodonsia", Category: "DENTAL", SortOrder: 300},
		{Code: "POLI-PERIODONSIA", Name: "Poli Periodonsia", Category: "DENTAL", SortOrder: 310},
		{Code: "POLI-PROSTODONSIA", Name: "Poli Prostodonsia", Category: "DENTAL", SortOrder: 320},
		{Code: "POLI-GIGI-ANAK", Name: "Poli Kedokteran Gigi Anak", Category: "DENTAL", SortOrder: 330},
		{Code: "POLI-PENYAKIT-MULUT", Name: "Poli Penyakit Mulut", Category: "DENTAL", SortOrder: 340},
		{Code: "POLI-RADIOLOGI-GIGI", Name: "Poli Radiologi Kedokteran Gigi", Category: "DENTAL", SortOrder: 350},
		{Code: "POLI-ANESTESIOLOGI", Name: "Poli Anestesiologi dan Terapi Intensif", Category: "SUPPORT_SPECIALIST", SortOrder: 360},
		{Code: "POLI-REHABILITASI-MEDIK", Name: "Poli Kedokteran Fisik dan Rehabilitasi", Category: "SUPPORT_SPECIALIST", SortOrder: 370},
		{Code: "POLI-RADIOLOGI", Name: "Poli Radiologi", Category: "SUPPORT_SPECIALIST", SortOrder: 380},
		{Code: "POLI-PATOLOGI-KLINIK", Name: "Poli Patologi Klinik", Category: "SUPPORT_SPECIALIST", SortOrder: 390},
		{Code: "POLI-PATOLOGI-ANATOMIK", Name: "Poli Patologi Anatomik", Category: "SUPPORT_SPECIALIST", SortOrder: 400},
		{Code: "POLI-MIKROBIOLOGI-KLINIK", Name: "Poli Mikrobiologi Klinik", Category: "SUPPORT_SPECIALIST", SortOrder: 410},
		{Code: "POLI-PARASITOLOGI-KLINIK", Name: "Poli Parasitologi Klinik", Category: "SUPPORT_SPECIALIST", SortOrder: 420},
		{Code: "POLI-ONKOLOGI-RADIASI", Name: "Poli Onkologi Radiasi", Category: "SUPPORT_SPECIALIST", SortOrder: 430},
		{Code: "POLI-KEDOKTERAN-NUKLIR", Name: "Poli Kedokteran Nuklir dan Teranostik Molekuler", Category: "SUPPORT_SPECIALIST", SortOrder: 440},
		{Code: "POLI-FORENSIK-MEDIKOLEGAL", Name: "Poli Kedokteran Forensik dan Medikolegal", Category: "SUPPORT_SPECIALIST", SortOrder: 450},
	}
}

// SeedMasterDepartments restores application-owned definitions without
// adopting or deleting unknown rows. Migration data makes the catalogue
// available in production; this function keeps development/reset idempotent.
func SeedMasterDepartments(db *gorm.DB) error {
	for _, master := range masterDepartmentSeeds() {
		result := db.Exec(`INSERT INTO master_departments (code, name, category, sort_order)
			VALUES (?, ?, ?, ?) ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				category = EXCLUDED.category,
				sort_order = EXCLUDED.sort_order,
				is_active = TRUE,
				updated_at = NOW()`, master.Code, master.Name, master.Category, master.SortOrder)
		if result.Error != nil {
			return fmt.Errorf("seed master department %s: %w", master.Code, result.Error)
		}
	}
	if err := db.Exec(`UPDATE hospital_departments department
		SET master_department_id = master.id, code = master.code, name = master.name, updated_at = NOW()
		FROM master_departments master
		WHERE UPPER(department.code) = master.code
		  AND (department.master_department_id IS DISTINCT FROM master.id
		       OR department.code IS DISTINCT FROM master.code
		       OR department.name IS DISTINCT FROM master.name)`).Error; err != nil {
		return fmt.Errorf("synchronize hospital departments with master: %w", err)
	}
	return nil
}
