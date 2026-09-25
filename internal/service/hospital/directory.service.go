package hospital

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) WithDirectoryStorage(client storage.Client, bucket string, ttl time.Duration, maxSize int64) *Service {
	s.directoryStorage = client
	s.imageBucket = bucket
	s.imageURLTTL = ttl
	s.imageMaxSize = maxSize
	if ttl <= 0 {
		s.imageURLTTL = 5 * time.Minute
	}
	if maxSize <= 0 || maxSize > 10*1024*1024 {
		s.imageMaxSize = 10 * 1024 * 1024
	}
	return s
}

func (s *Service) ListDirectory(ctx context.Context, q request.HospitalDirectoryQuery) ([]response.Hospital, error) {
	if err := validateDirectoryQuery(&q); err != nil {
		return nil, err
	}
	rows, err := s.hospitalRepo.ListDirectory(ctx, q)
	if err != nil {
		return nil, mapLifecycleError(err)
	}
	if err = s.enrichHospitals(ctx, rows, false); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) RecommendHospitals(ctx context.Context, q request.HospitalDirectoryQuery) ([]response.Hospital, error) {
	q.Recommended = true
	if q.Limit == 0 {
		q.Limit = 10
	}
	if strings.TrimSpace(q.Sort) == "" {
		q.Sort = "rating"
	}
	return s.ListDirectory(ctx, q)
}

func (s *Service) ListDepartmentOptions(ctx context.Context, q request.DepartmentDirectoryQuery) ([]response.DepartmentOption, error) {
	if err := validateDepartmentDirectoryQuery(&q); err != nil {
		return nil, err
	}
	rows, err := s.hospitalRepo.ListDepartmentOptions(ctx, q)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}
func (s *Service) GetDirectory(ctx context.Context, id string, latitude, longitude *float64) (*response.Hospital, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	q := request.HospitalDirectoryQuery{Latitude: latitude, Longitude: longitude, Limit: 1}
	if err := validateDirectoryQuery(&q); err != nil {
		return nil, err
	}
	row, err := s.hospitalRepo.GetDirectory(ctx, id, latitude, longitude)
	if err != nil {
		return nil, mapLifecycleError(err)
	}
	rows := []response.Hospital{*row}
	if err = s.enrichHospitals(ctx, rows, true); err != nil {
		return nil, err
	}
	return &rows[0], nil
}
func (s *Service) enrichHospitals(ctx context.Context, rows []response.Hospital, detail bool) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	index := map[string]int{}
	for i := range rows {
		row := &rows[i]
		ids = append(ids, row.ID)
		index[row.ID] = i
		row.Departments = []response.HospitalDepartmentSummary{}
		facilities, err := normalizeFacilities(row.Facilities)
		if err != nil {
			// Older versions accepted arbitrary JSON. Keep that value in storage
			// for an admin to correct without taking the whole directory offline.
			util.NewDefaultLogger(ctx).Warnf("hospital facilities need normalization hospital_id=%s", row.ID)
			facilities = json.RawMessage("[]")
		}
		row.Facilities = facilities
		if len(row.OpeningHours) == 0 {
			row.OpeningHours = json.RawMessage("[]")
		}
		row.IsOpen = hospitalIsOpen(row.OpeningHours, row.Timezone, time.Now())
		if row.DistanceKM != nil {
			value := math.Round(*row.DistanceKM*100) / 100
			row.DistanceKM = &value
		}
	}
	departments, err := s.hospitalRepo.DirectoryDepartments(ctx, ids)
	if err != nil {
		return constant.ErrInternalServerError
	}
	for _, department := range departments {
		if i, ok := index[department.HospitalID]; ok {
			rows[i].Departments = append(rows[i].Departments, department)
		}
	}
	images, err := s.hospitalRepo.DirectoryImages(ctx, ids, !detail)
	if err != nil {
		return constant.ErrInternalServerError
	}
	for i := range images {
		img := &images[i]
		if err := s.signImage(ctx, img); err != nil {
			return err
		}
		n, ok := index[img.HospitalID]
		if !ok {
			continue
		}
		if img.IsCover {
			rows[n].CoverImage = img
		}
		if detail {
			rows[n].Gallery = append(rows[n].Gallery, *img)
		}
	}
	return nil
}
func (s *Service) signImage(ctx context.Context, img *response.HospitalImage) error {
	if s.directoryStorage == nil || img.Bucket != s.imageBucket {
		return constant.ErrStorageUnavailable
	}
	url, err := s.directoryStorage.CreateSignedURL(ctx, img.ObjectPath, s.imageURLTTL, "")
	if err != nil {
		return constant.ErrStorageUnavailable
	}
	img.URL = url
	img.ExpiresAt = time.Now().UTC().Add(s.imageURLTTL)
	return nil
}

func (s *Service) ListImages(ctx context.Context, hospitalID string) ([]response.HospitalImage, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := s.hospitalRepo.FindByID(ctx, hospitalID); err != nil {
		return nil, mapLifecycleError(err)
	}
	rows, err := s.hospitalRepo.DirectoryImages(ctx, []string{hospitalID}, false)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	for i := range rows {
		if err = s.signImage(ctx, &rows[i]); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
func validateHospitalImage(content []byte, maxSize int64) (string, string, error) {
	if maxSize <= 0 || maxSize > 10*1024*1024 {
		maxSize = 10 * 1024 * 1024
	}
	if len(content) == 0 || int64(len(content)) > maxSize {
		return "", "", constant.ErrHospitalImageInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return "", "", constant.ErrHospitalImageInvalid
	}
	if _, _, err = image.Decode(bytes.NewReader(content)); err != nil {
		return "", "", constant.ErrHospitalImageInvalid
	}
	if format == "jpeg" {
		return "image/jpeg", ".jpg", nil
	}
	return "image/png", ".png", nil
}
func (s *Service) UploadImage(ctx context.Context, hospitalID, actorID string, content []byte, caption string, sortOrder int, isCover bool) (*response.HospitalImage, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := uuid.Parse(actorID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := s.hospitalRepo.FindByID(ctx, hospitalID); err != nil {
		return nil, mapLifecycleError(err)
	}
	caption = strings.TrimSpace(caption)
	if utf8.RuneCountInString(caption) > 200 || sortOrder < 0 || sortOrder > 1000 {
		return nil, invalidDirectory("image", "caption <=200; sort_order 0-1000")
	}
	contentType, ext, err := validateHospitalImage(content, s.imageMaxSize)
	if err != nil {
		return nil, err
	}
	if s.directoryStorage == nil {
		return nil, constant.ErrStorageUnavailable
	}
	id := uuid.NewString()
	path := fmt.Sprintf("hospitals/%s/%s%s", hospitalID, id, ext)
	uploaded, err := s.directoryStorage.Upload(ctx, path, contentType, content)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	img := response.HospitalImage{ID: id, HospitalID: hospitalID, Bucket: uploaded.Bucket, ObjectPath: uploaded.ObjectPath, ContentType: contentType, FileSize: uploaded.FileSize, Caption: caption, SortOrder: sortOrder, IsCover: isCover, CreatedAt: time.Now().UTC()}
	if err = s.signImage(ctx, &img); err != nil {
		s.cleanupHospitalImage(ctx, path)
		return nil, err
	}
	row, err := s.hospitalRepo.AddImage(ctx, img, actorID)
	if err != nil {
		s.cleanupHospitalImage(ctx, path)
		if errors.Is(err, repository.ErrHospitalGalleryFull) {
			return nil, constant.ErrHospitalGalleryFull
		}
		return nil, mapLifecycleError(err)
	}
	return row, nil
}
func (s *Service) cleanupHospitalImage(ctx context.Context, path string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.directoryStorage.Delete(cleanup, path); err != nil {
		util.Errorf(ctx, "hospital image cleanup failed object=%s error_type=%T", path, err)
	}
}
func (s *Service) UpdateImage(ctx context.Context, hospitalID, imageID string, req request.UpdateHospitalImageRequest) (*response.HospitalImage, error) {
	if err := validateHospitalIDs(hospitalID, imageID); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if req.Caption != nil {
		if utf8.RuneCountInString(*req.Caption) > 200 {
			return nil, invalidDirectory("caption", "at most 200 characters")
		}
		fields["caption"] = strings.TrimSpace(*req.Caption)
	}
	if req.SortOrder != nil {
		if *req.SortOrder < 0 || *req.SortOrder > 1000 {
			return nil, invalidDirectory("sort_order", "0-1000")
		}
		fields["sort_order"] = *req.SortOrder
	}
	if req.IsCover != nil {
		if !*req.IsCover {
			return nil, invalidDirectory("is_cover", "true to select this image as cover")
		}
		fields["is_cover"] = true
	}
	if len(fields) == 0 {
		return nil, constant.NewFieldRequiredError("at least one update field")
	}
	row, err := s.hospitalRepo.UpdateImage(ctx, hospitalID, imageID, fields, time.Now().UTC())
	if err != nil {
		return nil, imageError(err)
	}
	if err = s.signImage(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}
func (s *Service) DeleteImage(ctx context.Context, hospitalID, imageID string) error {
	if err := validateHospitalIDs(hospitalID, imageID); err != nil {
		return err
	}
	row, err := s.hospitalRepo.DeleteImage(ctx, hospitalID, imageID, time.Now().UTC())
	if err != nil {
		return imageError(err)
	}
	if s.directoryStorage != nil {
		s.cleanupHospitalImage(ctx, row.ObjectPath)
	}
	return nil
}
func validateHospitalIDs(hospitalID, otherID string) error {
	for _, id := range []string{hospitalID, otherID} {
		if _, err := uuid.Parse(id); err != nil {
			return constant.ErrInvalidUUIDFormat
		}
	}
	return nil
}
func imageError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return constant.ErrHospitalImageNotFound
	}
	return mapLifecycleError(err)
}

func (s *Service) ListReviews(ctx context.Context, hospitalID string, limit, offset int, sort string) (*response.HospitalReviewPage, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, invalidDirectory("pagination", "limit 1-100; offset 0-100000")
	}
	if sort == "" {
		sort = "latest"
	}
	if sort != "latest" && sort != "highest" && sort != "lowest" {
		return nil, invalidDirectory("sort", "latest, highest, or lowest")
	}
	page, err := s.hospitalRepo.ListReviews(ctx, hospitalID, limit, offset, sort)
	return page, mapLifecycleError(err)
}
func (s *Service) GetOwnReview(ctx context.Context, hospitalID, userID string) (*response.HospitalReview, error) {
	if err := validateHospitalIDs(hospitalID, userID); err != nil {
		return nil, err
	}
	row, err := s.hospitalRepo.GetOwnReview(ctx, hospitalID, userID)
	return row, reviewError(err)
}
func (s *Service) PutReview(ctx context.Context, hospitalID, userID string, req request.PutHospitalReviewRequest) (*response.HospitalReview, error) {
	if err := validateHospitalIDs(hospitalID, userID); err != nil {
		return nil, err
	}
	req.Comment = strings.TrimSpace(req.Comment)
	if req.Rating < 1 || req.Rating > 5 || utf8.RuneCountInString(req.Comment) > 2000 {
		return nil, invalidDirectory("review", "rating 1-5; comment at most 2000 characters")
	}
	row, err := s.hospitalRepo.PutReview(ctx, hospitalID, userID, req.Rating, req.Comment, time.Now().UTC())
	return row, reviewError(err)
}
func (s *Service) DeleteOwnReview(ctx context.Context, hospitalID, userID string) error {
	if err := validateHospitalIDs(hospitalID, userID); err != nil {
		return err
	}
	return reviewError(s.hospitalRepo.DeleteOwnReview(ctx, hospitalID, userID, time.Now().UTC()))
}
func reviewError(err error) error {
	if errors.Is(err, repository.ErrReviewVisitRequired) {
		return constant.ErrHospitalReviewVisitRequired
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return constant.ErrHospitalReviewNotFound
	}
	return mapLifecycleError(err)
}
