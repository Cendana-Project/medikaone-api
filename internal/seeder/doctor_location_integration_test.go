package seeder

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Real PostgreSQL coverage shares the existing disposable migration/seed gate.
// All fixtures roll back so demo identities and later tests remain unchanged.
func testDoctorLocationIntegration(t *testing.T, db *gorm.DB) {
	t.Run("doctor recommendations by practice location", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		ctx := context.Background()
		exec := func(query string, args ...any) {
			t.Helper()
			if err := tx.Exec(query, args...).Error; err != nil {
				t.Fatal(err)
			}
		}
		var adminID, patientID string
		if err := tx.Raw("SELECT id::text FROM users WHERE email = 'admin001@medikaone.id'").Scan(&adminID).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Raw("SELECT id::text FROM users WHERE email = 'patient001@medikaone.id'").Scan(&patientID).Error; err != nil {
			t.Fatal(err)
		}
		newDoctor := func(name string) string {
			id := uuid.NewString()
			exec(`INSERT INTO users (id, email, password_hash, first_name, status, verified_at)
				VALUES (?, ?, 'test-only-hash', ?, 'active', NOW())`, id, id+"@example.test", "LocationFixture "+name)
			exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE slug = 'DOCTOR'`, id)
			exec(`UPDATE doctor_profiles SET sip_number = ?, specialty = 'Location Specialty' WHERE user_id = ?`, "LOC-"+id, id)
			return id
		}
		type practice struct{ hospital, department, affiliation, schedule string }
		newPractice := func(doctorID, code, city, departmentCode string, longitude *float64, day int, mode string, rating int) practice {
			p := practice{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
			var latitude *float64
			if longitude != nil {
				zero := 0.0
				latitude = &zero
			}
			exec(`INSERT INTO hospitals (id, code, name, city, latitude, longitude, is_active)
				VALUES (?, ?, ?, ?, ?, ?, TRUE)`, p.hospital, code, "Location "+code, city, latitude, longitude)
			exec(`INSERT INTO hospital_departments (id, hospital_id, master_department_id, code, name, is_active)
				SELECT ?, ?, id, code, name, TRUE FROM master_departments WHERE code = ?`, p.department, p.hospital, departmentCode)
			invitation := uuid.NewString()
			exec(`INSERT INTO doctor_hospital_invitations (id, hospital_id, doctor_id, department_id, invited_by, status, expires_at, responded_at)
				VALUES (?, ?, ?, ?, ?, 'ACCEPTED', NOW()+INTERVAL '7 days', NOW())`, invitation, p.hospital, doctorID, p.department, adminID)
			exec(`INSERT INTO doctor_hospital_affiliations (id, hospital_id, doctor_id, department_id, invitation_id, status, joined_at)
				VALUES (?, ?, ?, ?, ?, 'ACTIVE', NOW())`, p.affiliation, p.hospital, doctorID, p.department, invitation)
			exec(`INSERT INTO doctor_hospital_schedules (id, affiliation_id, day_of_week, start_time, end_time, timezone, booking_mode, slot_duration_minutes, capacity)
				VALUES (?, ?, ?, '16:00', '17:00', 'Asia/Jakarta', ?, 30, 1)`, p.schedule, p.affiliation, day, mode)
			exec(`INSERT INTO hospital_reviews (hospital_id, user_id, rating) VALUES (?, ?, ?)`, p.hospital, patientID, rating)
			return p
		}
		number := func(value float64) *float64 { return &value }
		alpha, beta, gamma, delta := newDoctor("Alpha"), newDoctor("Beta"), newDoctor("Gamma"), newDoctor("Delta")
		near := newPractice(alpha, "IT-LOC-NEAR", "Near City", "POLI-ANAK", number(.01), 2, "SESSION_QUEUE", 5)
		far := newPractice(alpha, "IT-LOC-FAR", "Far City", "POLI-MATA", number(1), 1, "FIXED_SLOT", 5)
		betaPractice := newPractice(beta, "IT-LOC-BETA", "Near City", "POLI-MATA", number(.02002), 1, "FIXED_SLOT", 4)
		deltaPractice := newPractice(delta, "IT-LOC-DELTA", "Near City", "POLI-MATA", number(.02001), 1, "FIXED_SLOT", 4)
		newPractice(gamma, "IT-LOC-UNKNOWN", "Near City", "POLI-MATA", nil, 1, "FIXED_SLOT", 5)
		noSchedule := newDoctor("NoSchedule")
		inactive := newPractice(noSchedule, "IT-LOC-INACTIVE", "Near City", "POLI-MATA", number(0), 1, "FIXED_SLOT", 5)
		exec("UPDATE doctor_hospital_schedules SET is_active = FALSE WHERE id = ?", inactive.schedule)
		repo := doctorrepo.NewRepository(tx)
		base := doctorrepo.Filter{Query: "LocationFixture", Recommended: true, Page: 1, Limit: 20, Latitude: number(0), Longitude: number(0)}
		list := func(filter doctorrepo.Filter) *response.PublicDoctorPage {
			t.Helper()
			page, err := repo.ListDoctors(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			return page
		}
		assertOrder := func(page *response.PublicDoctorPage, want ...string) {
			t.Helper()
			got := make([]string, 0, len(page.Items))
			for _, doctor := range page.Items {
				got = append(got, doctor.DoctorID)
				if doctor.DoctorMedikaOneID == "" {
					t.Fatal("public doctor identity lost")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("doctor order=%v want=%v", got, want)
			}
		}
		page := list(base)
		assertOrder(page, alpha, delta, beta, gamma)
		if page.Total != 4 || page.Items[0].NearestPractice == nil || page.Items[0].NearestPractice.AffiliationID != near.affiliation ||
			page.Items[0].NearestPractice.HospitalID != near.hospital || page.Items[0].NearestPractice.DepartmentID != near.department ||
			page.Items[0].NearestPractice.HospitalName != "Location IT-LOC-NEAR" || page.Items[0].NearestPractice.DistanceKM != 1.11 ||
			page.Items[1].NearestPractice.DistanceKM != 2.23 || page.Items[2].NearestPractice.DistanceKM != 2.23 || page.Items[3].NearestPractice != nil {
			t.Fatalf("incorrect nearest practice projection: %#v", page)
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		var jsonPage struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(encoded, &jsonPage); err != nil {
			t.Fatal(err)
		}
		if _, present := jsonPage.Items[3]["nearest_practice"]; present {
			t.Fatal("unknown distance must not fabricate a practice location")
		}
		location := jsonPage.Items[0]["nearest_practice"].(map[string]any)
		if location["latitude"] != float64(0) || location["longitude"] != .01 {
			t.Fatal("response must contain practice coordinates")
		}

		t.Run("fallback pagination radius and precise distance", func(t *testing.T) {
			filter := base
			filter.Latitude, filter.Longitude = nil, nil
			fallback := list(filter)
			assertOrder(fallback, alpha, gamma, beta, delta)
			for _, doctor := range fallback.Items {
				if doctor.NearestPractice != nil {
					t.Fatal("missing patient coordinates must not fabricate distances")
				}
			}
			filter = base
			filter.Limit, filter.Page = 1, 2
			paged := list(filter)
			assertOrder(paged, delta)
			if paged.Total != 4 {
				t.Fatal("count must precede pagination")
			}
			filter = base
			filter.RadiusKM = number(2.2255)
			within := list(filter)
			assertOrder(within, alpha, delta)
			if within.Total != 2 {
				t.Fatal("radius count differs from eligible results")
			}
			filter.RadiusKM = number(1)
			empty := list(filter)
			if empty.Total != 0 || empty.Items == nil || len(empty.Items) != 0 {
				t.Fatal("empty radius result must contain items: []")
			}
			filter = base
			filter.Longitude, filter.RadiusKM = number(.01), number(.001)
			atPractice := list(filter)
			assertOrder(atPractice, alpha)
			if atPractice.Items[0].NearestPractice.DistanceKM != 0 {
				t.Fatal("zero distance is valid")
			}
			filter = base
			filter.Recommended = false
			assertOrder(list(filter), alpha, delta, beta, gamma)
		})

		date := time.Now().UTC().AddDate(0, 0, 21)
		for date.Weekday() != time.Monday {
			date = date.AddDate(0, 0, 1)
		}
		for _, test := range []struct {
			name  string
			apply func(*doctorrepo.Filter)
		}{
			{"hospital", func(f *doctorrepo.Filter) { f.HospitalID = far.hospital }},
			{"department code", func(f *doctorrepo.Filter) { f.DepartmentCode = "POLI-MATA" }},
			{"department id", func(f *doctorrepo.Filter) { f.DepartmentID = far.department }},
			{"city", func(f *doctorrepo.Filter) { f.City = "Far City" }},
			{"date", func(f *doctorrepo.Filter) { f.AvailableOn = date.Format("2006-01-02") }},
			{"booking mode", func(f *doctorrepo.Filter) { f.BookingMode = "FIXED_SLOT" }},
		} {
			t.Run("closest practice must match "+test.name, func(t *testing.T) {
				filter := base
				filter.Query = "LocationFixture Alpha"
				test.apply(&filter)
				result := list(filter)
				assertOrder(result, alpha)
				if result.Items[0].NearestPractice.AffiliationID != far.affiliation {
					t.Fatal("nearest practice borrowed eligibility from a different affiliation")
				}
				filter.RadiusKM = number(2)
				if result := list(filter); result.Total != 0 || len(result.Items) != 0 {
					t.Fatal("radius included a nearby practice that fails the selected filter")
				}
			})
		}

		for _, test := range []struct {
			name, query string
			args        []any
		}{
			{"inactive hospital", "UPDATE hospitals SET is_active = FALSE WHERE id = ?", []any{near.hospital}},
			{"deleted hospital", "UPDATE hospitals SET deleted_at = NOW() WHERE id = ?", []any{near.hospital}},
			{"inactive department", "UPDATE hospital_departments SET is_active = FALSE WHERE id = ?", []any{near.department}},
			{"suspended affiliation", "UPDATE doctor_hospital_affiliations SET status = 'SUSPENDED' WHERE id = ?", []any{near.affiliation}},
			{"deleted affiliation", "UPDATE doctor_hospital_affiliations SET deleted_at = NOW() WHERE id = ?", []any{near.affiliation}},
			{"inactive schedule", "UPDATE doctor_hospital_schedules SET is_active = FALSE WHERE id = ?", []any{near.schedule}},
			{"expired specific", "UPDATE doctor_hospital_schedules SET schedule_date = CURRENT_DATE - 2, day_of_week = EXTRACT(DOW FROM CURRENT_DATE - 2) WHERE id = ?", []any{near.schedule}},
			{"unknown coordinates", "UPDATE hospitals SET latitude = NULL, longitude = NULL WHERE id = ?", []any{near.hospital}},
		} {
			t.Run("ignore "+test.name, func(t *testing.T) {
				exec("SAVEPOINT location_case")
				defer exec("ROLLBACK TO SAVEPOINT location_case")
				exec(test.query, test.args...)
				filter := base
				filter.Query = "LocationFixture Alpha"
				result := list(filter)
				assertOrder(result, alpha)
				if result.Items[0].NearestPractice.AffiliationID != far.affiliation {
					t.Fatal("ineligible near practice affected ranking")
				}
			})
		}
		t.Run("future specific schedule and stable distance ties", func(t *testing.T) {
			exec("SAVEPOINT location_case")
			defer exec("ROLLBACK TO SAVEPOINT location_case")
			exec("UPDATE doctor_hospital_schedules SET schedule_date = ?::date, day_of_week = 1, start_time = '12:00', end_time = '13:00' WHERE id = ?", date.Format("2006-01-02"), near.schedule)
			filter := base
			filter.Query = "LocationFixture Alpha"
			filter.AvailableOn = date.Format("2006-01-02")
			if result := list(filter); result.Items[0].NearestPractice.AffiliationID != near.affiliation {
				t.Fatal("specific date not used for nearest practice")
			}
			exec("UPDATE hospitals SET longitude = .02002 WHERE id = ?", deltaPractice.hospital)
			assertOrder(list(base), alpha, beta, delta, gamma)
			// Rating remains the first tie-breaker for identical distances.
			exec("UPDATE hospital_reviews SET rating = 3 WHERE hospital_id = ?", betaPractice.hospital)
			assertOrder(list(base), alpha, delta, beta, gamma)
		})
		t.Run("inactive doctor remains hidden", func(t *testing.T) {
			exec("SAVEPOINT location_case")
			defer exec("ROLLBACK TO SAVEPOINT location_case")
			exec("UPDATE users SET deleted_at = NOW() WHERE id = ?", alpha)
			assertOrder(list(base), delta, beta, gamma)
		})
	})
}
