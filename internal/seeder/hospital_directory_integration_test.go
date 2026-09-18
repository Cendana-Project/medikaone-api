package seeder

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	hospitalrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	hospitalsvc "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type directoryTestStorage struct{ deleted []string }

func (*directoryTestStorage) Upload(_ context.Context, path, mime string, content []byte) (*storage.UploadedObject, error) {
	return &storage.UploadedObject{Bucket: "hospital-images", ObjectPath: path, FileSize: int64(len(content))}, nil
}
func (s *directoryTestStorage) Delete(_ context.Context, path string) error {
	s.deleted = append(s.deleted, path)
	return nil
}
func (*directoryTestStorage) CreateSignedURL(_ context.Context, path string, _ time.Duration, _ string) (string, error) {
	return "https://storage.example.test/" + path + "?token=dummy", nil
}

// Runs after the one-off schedule test has created a real booked-and-cancelled
// appointment. The enclosing disposable suite resets these fixtures afterwards.
func runHospitalDirectoryIntegration(t *testing.T, db *gorm.DB, sqlDB *sql.DB) {
	ctx := context.Background()
	repo := hospitalrepo.NewRepository(db)
	store := &directoryTestStorage{}
	service := hospitalsvc.NewService(nil, nil, repo).WithDirectoryStorage(store, "hospital-images", 5*time.Minute, 10*1024*1024)
	hospitalID := scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code='HSP-MO-001'`)
	otherHospital := scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code='HSP-MO-002'`)
	patientID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='patient001@medikaone.id'`)
	otherPatient := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='patient002@medikaone.id'`)
	adminID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email='admin001@medikaone.id'`)
	reviewReq := request.PutHospitalReviewRequest{Rating: 4, Comment: "Pelayanan baik"}
	if _, err := service.PutReview(ctx, hospitalID, patientID, reviewReq); !errors.Is(err, constant.ErrHospitalReviewVisitRequired) {
		t.Fatalf("cancelled visit can review: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE appointments SET status='COMPLETED',completed_at=NOW() WHERE hospital_id=$1 AND patient_id=$2`, hospitalID, patientID); err != nil {
		t.Fatal(err)
	}
	review, err := service.PutReview(ctx, hospitalID, patientID, reviewReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PutReview(ctx, otherHospital, patientID, reviewReq); !errors.Is(err, constant.ErrHospitalReviewVisitRequired) {
		t.Fatalf("visit qualification leaked across hospitals: %v", err)
	}
	if _, err = service.PutReview(ctx, hospitalID, otherPatient, reviewReq); !errors.Is(err, constant.ErrHospitalReviewVisitRequired) {
		t.Fatalf("non-patient can review: %v", err)
	}
	reviewReq.Rating = 5
	updated, err := service.PutReview(ctx, hospitalID, patientID, reviewReq)
	if err != nil || updated.ID != review.ID {
		t.Fatalf("review upsert: %#v %v", updated, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.PutReview(ctx, hospitalID, patientID, reviewReq)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent review failed", err)
		}
	}
	page, err := service.ListReviews(ctx, hospitalID, 1, 0, "latest")
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.RatingAverage == nil || *page.RatingAverage != 5 || page.RatingDistribution["5"] != 1 {
		t.Fatalf("review aggregation: %#v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	for _, field := range []string{`"patient_id"`, `"user_id"`, `"appointment_id"`, `"email"`} {
		if strings.Contains(string(raw), field) {
			t.Fatal("public review exposed", field)
		}
	}
	if err = service.DeleteOwnReview(ctx, hospitalID, otherPatient); !errors.Is(err, constant.ErrHospitalReviewNotFound) {
		t.Fatalf("another account deleted review: %v", err)
	}
	if err = service.DeleteOwnReview(ctx, hospitalID, patientID); err != nil {
		t.Fatal(err)
	}
	page, err = service.ListReviews(ctx, hospitalID, 20, 0, "latest")
	if err != nil || page.Total != 0 || page.RatingAverage != nil || page.Items == nil {
		t.Fatalf("deleted review remains visible: %#v %v", page, err)
	}
	updated, err = service.PutReview(ctx, hospitalID, patientID, reviewReq)
	if err != nil || updated.ID != review.ID {
		t.Fatal("review recreation duplicates identity", err)
	}

	lat, lng := -6.2, 106.8
	email, website, description := "contact@example.test", "https://hospital.example.test", strings.Repeat("Deskripsi lengkap. ", 30)
	year := 1990
	hours := make([]request.HospitalOpeningDay, 7)
	for i := range hours {
		hours[i] = request.HospitalOpeningDay{DayOfWeek: i, Is24Hours: true}
	}
	detail, err := service.UpdateHospital(ctx, hospitalID, request.UpdateHospitalRequest{Latitude: &lat, Longitude: &lng, Email: &email, Website: &website, EstablishedYear: &year, Description: &description, OpeningHours: &hours, Facilities: json.RawMessage(`[{"code":"parking","name":"Parkir","icon":"car"}]`)})
	if err != nil || detail.Email == nil || *detail.Email != email || detail.IsOpen == nil || !*detail.IsOpen || len(detail.Departments) == 0 {
		t.Fatalf("hospital enrichment: %#v %v", detail, err)
	}
	radius, minRating := 1.0, 4.0
	list, err := service.ListDirectory(ctx, request.HospitalDirectoryQuery{Latitude: &lat, Longitude: &lng, RadiusKM: &radius, MinRating: &minRating, Department: "Integration Schedule", Sort: "distance", Limit: 20})
	if err != nil || len(list) != 1 || list[0].ID != hospitalID || list[0].DistanceKM == nil || *list[0].DistanceKM > 0.01 {
		t.Fatalf("directory filters/distance: %#v %v", list, err)
	}
	detail, err = service.GetHospital(ctx, hospitalID)
	if err != nil || detail.DistanceKM != nil {
		t.Fatal("distance was persisted from another visitor", err)
	}
	legacyFacilities := `{"legacy":{"details":["parking"]}}`
	if _, err = sqlDB.Exec(`UPDATE hospitals SET facilities=$1::jsonb WHERE id=$2`, legacyFacilities, hospitalID); err != nil {
		t.Fatal(err)
	}
	detail, err = service.GetHospital(ctx, hospitalID)
	if err != nil || string(detail.Facilities) != "[]" {
		t.Fatal("legacy free-form facilities broke the directory", err)
	}
	if stored := scalarString(t, sqlDB, `SELECT facilities->'legacy'->'details'->>0 FROM hospitals WHERE id=$1`, hospitalID); stored != "parking" {
		t.Fatal("reading the directory overwrote legacy facilities")
	}
	if _, err = service.UpdateHospital(ctx, hospitalID, request.UpdateHospitalRequest{Facilities: json.RawMessage(`[{"code":"parking","name":"Parkir","icon":"car"}]`)}); err != nil {
		t.Fatal(err)
	}
	var content bytes.Buffer
	if err = png.Encode(&content, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	first, err := service.UploadImage(ctx, hospitalID, adminID, content.Bytes(), "Gedung utama", 2, false)
	if err != nil || !first.IsCover || first.URL == "" {
		t.Fatalf("first image: %#v %v", first, err)
	}
	second, err := service.UploadImage(ctx, hospitalID, adminID, content.Bytes(), "Lobby", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	cover := true
	if _, err = service.UpdateImage(ctx, otherHospital, first.ID, request.UpdateHospitalImageRequest{IsCover: &cover}); !errors.Is(err, constant.ErrHospitalImageNotFound) {
		t.Fatal("cross-tenant image mutation", err)
	}
	if err = service.DeleteImage(ctx, otherHospital, first.ID); !errors.Is(err, constant.ErrHospitalImageNotFound) {
		t.Fatal("cross-tenant image deletion", err)
	}
	if _, err = service.UpdateImage(ctx, hospitalID, second.ID, request.UpdateHospitalImageRequest{IsCover: &cover}); err != nil {
		t.Fatal(err)
	}
	detail, err = service.GetHospital(ctx, hospitalID)
	if err != nil || len(detail.Gallery) != 2 || detail.CoverImage == nil || detail.CoverImage.ID != second.ID {
		t.Fatalf("gallery detail: %#v %v", detail, err)
	}
	if err = service.DeleteImage(ctx, hospitalID, second.ID); err != nil {
		t.Fatal(err)
	}
	detail, err = service.GetHospital(ctx, hospitalID)
	if err != nil || len(detail.Gallery) != 1 || detail.CoverImage.ID != first.ID || len(store.deleted) != 1 {
		t.Fatal("cover promotion/storage cleanup failed", err)
	}
	raw, _ = json.Marshal(detail)
	for _, field := range []string{`"bucket"`, `"object_path"`, `"uploaded_by"`} {
		if strings.Contains(string(raw), field) {
			t.Fatal("image metadata exposed", field)
		}
	}
	for i := 1; i < 20; i++ {
		id := uuid.NewString()
		_, err = repo.AddImage(ctx, response.HospitalImage{ID: id, HospitalID: hospitalID, Bucket: "hospital-images", ObjectPath: "test/" + id, ContentType: "image/png", FileSize: 10, SortOrder: i, CreatedAt: time.Now()}, adminID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = service.UploadImage(ctx, hospitalID, adminID, content.Bytes(), "Excess", 0, false); !errors.Is(err, constant.ErrHospitalGalleryFull) || len(store.deleted) != 2 {
		t.Fatalf("gallery cap/cleanup: %v %v", err, store.deleted)
	}
	if _, err = sqlDB.Exec(`UPDATE users SET deleted_at=NOW(),status='blocked' WHERE id=$1`, patientID); err != nil {
		t.Fatal(err)
	}
	page, err = service.ListReviews(ctx, hospitalID, 20, 0, "latest")
	if err != nil || page.Items[0].ReviewerName != "Pasien" {
		t.Fatal("deleted reviewer not anonymized", err)
	}
	if _, err = sqlDB.Exec(`UPDATE hospitals SET is_active=FALSE WHERE id=$1`, hospitalID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ListReviews(ctx, hospitalID, 20, 0, "latest"); !errors.Is(err, constant.ErrHospitalNotFound) {
		t.Fatal("inactive hospital reviews exposed", err)
	}
	if _, err = service.ListImages(ctx, hospitalID); !errors.Is(err, constant.ErrHospitalNotFound) {
		t.Fatal("inactive hospital gallery exposed", err)
	}
}
