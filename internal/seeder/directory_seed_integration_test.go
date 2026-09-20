package seeder

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	hospitalsvc "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	"gorm.io/gorm"
)

// Called with the seeded disposable database. Rollback keeps later feature tests
// independent of mutations used to exercise fixture restoration and conflicts.
func testDirectorySeedsIntegration(t *testing.T, db *gorm.DB, sqlDB *sql.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	service := hospitalsvc.NewService(nil, nil, hospitalrepo.NewRepository(tx))
	ctx := context.Background()
	for _, fixture := range hospitalSeeds() {
		id, err := getHospitalIDBySeedKey(tx, fixture.SeedKey)
		if err != nil {
			t.Fatal(err)
		}
		detail, err := service.GetDirectory(ctx, id, fixture.Latitude, fixture.Longitude)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Email == nil || *detail.Email != fixture.Email || detail.Website == nil || *detail.Website != fixture.Website ||
			detail.EstablishedYear == nil || *detail.EstablishedYear != fixture.EstablishedYear || detail.Timezone != fixture.Timezone ||
			detail.DistanceKM == nil || *detail.DistanceKM != 0 || detail.IsOpen == nil || len(detail.Departments) != len(fixture.Departments) {
			t.Fatalf("incomplete seeded directory hospital: %#v", detail)
		}
		var facilities []response.HospitalFacility
		if err := json.Unmarshal(detail.Facilities, &facilities); err != nil || len(facilities) != len(fixture.Facilities) {
			t.Fatalf("facilities not readable: %s (%v)", detail.Facilities, err)
		}
		var hours []request.HospitalOpeningDay
		if err := json.Unmarshal(detail.OpeningHours, &hours); err != nil || len(hours) != 7 {
			t.Fatalf("invalid opening hours: %s", detail.OpeningHours)
		}
		// Exercise the public write validator too, not just successful JSON parsing.
		if _, err := service.UpdateHospital(ctx, id, request.UpdateHospitalRequest{
			Email: &fixture.Email, Website: &fixture.Website, EstablishedYear: &fixture.EstablishedYear,
			Timezone: &fixture.Timezone, Facilities: detail.Facilities, OpeningHours: &hours,
		}); err != nil {
			t.Fatalf("fixture rejected by hospital API validation: %v", err)
		}
	}
	list, err := service.ListDirectory(ctx, request.HospitalDirectoryQuery{Department: "Poli Mata", Limit: 20})
	if err != nil || len(list) != 2 {
		t.Fatalf("seeded department filter: got %d hospitals, err=%v", len(list), err)
	}
	for _, item := range []struct {
		table string
		want  int
	}{{"hospital_departments", 5}, {"hospital_rooms", 5}, {"patient_profiles", 3}} {
		if got := scalarInt(t, sqlDB, "SELECT COUNT(*) FROM "+item.table); got != item.want {
			t.Fatalf("%s count=%d want=%d", item.table, got, item.want)
		}
	}

	var departmentID, roomID string
	if err := tx.Raw(`SELECT id::text FROM hospital_departments ORDER BY id LIMIT 1`).Scan(&departmentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw(`SELECT id::text FROM hospital_rooms WHERE department_id = ?`, departmentID).Scan(&roomID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE hospital_rooms SET is_active = FALSE, name = 'Edited fixture room' WHERE id = ?`, roomID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE hospital_departments SET is_active = FALSE, name = 'Edited fixture department' WHERE id = ?`, departmentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedDepartments(tx); err != nil {
		t.Fatal(err)
	}
	var restored int
	if err := tx.Raw(`SELECT COUNT(*) FROM hospital_rooms r JOIN hospital_departments d ON d.id = r.department_id
		WHERE r.id = ? AND d.id = ? AND r.is_active AND d.is_active AND r.name <> 'Edited fixture room' AND d.name <> 'Edited fixture department'`,
		roomID, departmentID).Scan(&restored).Error; err != nil || restored != 1 {
		t.Fatalf("fixture identities not restored: %d, %v", restored, err)
	}
	if err := tx.Exec(`UPDATE patient_profiles SET weight_kg = 81, medical_hist = 'History entered in demo'
		WHERE user_id = (SELECT id FROM users WHERE email = 'patient001@medikaone.id')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedSampleUsers(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM patient_profiles WHERE weight_kg = 81 AND medical_hist = 'History entered in demo'`).Scan(&restored).Error; err != nil || restored != 1 {
		t.Fatal("rerun overwrote patient-entered profile", err)
	}

	// A pre-existing department with a fixture code is not silently adopted.
	if err := tx.Exec(`DELETE FROM hospital_rooms WHERE department_id = ?`, departmentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE hospital_departments SET id = gen_random_uuid(), name = 'Unowned department' WHERE id = ?`, departmentID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Transaction(SeedDepartments); !isUniqueViolation(err) {
		t.Fatalf("expected conflict with unowned department, got %v", err)
	}
	if err := tx.Raw(`SELECT COUNT(*) FROM hospital_departments WHERE name = 'Unowned department'`).Scan(&restored).Error; err != nil || restored != 1 {
		t.Fatal("unowned department was changed", err)
	}
}
