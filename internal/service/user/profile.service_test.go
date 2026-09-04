package user

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
)

type fakeRepository struct {
	user           *entity.User
	usernameExists bool
	nikExists      bool
	updates        map[string]any
	image          *repository.ProfileImageRecord
}

func (f *fakeRepository) GetByID(context.Context, string) (*entity.User, error) { return f.user, nil }
func (f *fakeRepository) ExistsUsernameExcludingUser(context.Context, string, string) (bool, error) {
	return f.usernameExists, nil
}
func (f *fakeRepository) ExistsNIKExcludingUser(context.Context, string, string) (bool, error) {
	return f.nikExists, nil
}
func (f *fakeRepository) UpdateByID(_ context.Context, _ string, updates map[string]any) error {
	f.updates = updates
	return nil
}
func (f *fakeRepository) ReplaceProfileImage(_ context.Context, _ string, image repository.ProfileImageRecord) (*repository.ProfileImageRecord, error) {
	previous := f.image
	f.image = &image
	return previous, nil
}
func (f *fakeRepository) ClearProfileImage(context.Context, string, time.Time) (*repository.ProfileImageRecord, error) {
	previous := f.image
	f.image = nil
	return previous, nil
}

type fakeStorage struct {
	uploadedPath string
	deletedPaths []string
}

func (f *fakeStorage) Upload(_ context.Context, objectPath, _ string, content []byte) (*storage.UploadedObject, error) {
	f.uploadedPath = objectPath
	return &storage.UploadedObject{Bucket: "profile-images", ObjectPath: objectPath, FileSize: int64(len(content))}, nil
}
func (f *fakeStorage) Delete(_ context.Context, objectPath string) error {
	f.deletedPaths = append(f.deletedPaths, objectPath)
	return nil
}
func (*fakeStorage) CreateSignedURL(context.Context, string, time.Duration, string) (string, error) {
	return "https://storage.example.test/signed", nil
}

func TestUpdateNormalizesProfileFields(t *testing.T) {
	repo := &fakeRepository{user: &entity.User{ID: "user-1"}}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	service.now = func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) }
	username, firstName, phone := "New.User", " Budi ", " 0812345678 "
	dob, gender, nik := "1990-01-15", "p", "3201010101010001"

	err := service.Update(context.Background(), "user-1", request.UpdateUserProfileRequest{
		Username: &username, FirstName: &firstName, Phone: &phone,
		DOB: &dob, Gender: &gender, NIK: &nik,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repo.updates["username"] != "new.user" || repo.updates["first_name"] != "Budi" ||
		repo.updates["phone"] != "0812345678" || repo.updates["gender"] != "P" ||
		repo.updates["nik"] != nik {
		t.Fatalf("unexpected normalized updates: %#v", repo.updates)
	}
	if _, ok := repo.updates["dob"].(time.Time); !ok {
		t.Fatalf("dob was not parsed: %#v", repo.updates["dob"])
	}
}

func TestUpdateRejectsEmptyPatch(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	if err := service.Update(context.Background(), "user-1", request.UpdateUserProfileRequest{}); err != constant.ErrProfileUpdateEmpty {
		t.Fatalf("Update() error = %v, want %v", err, constant.ErrProfileUpdateEmpty)
	}
}

func TestProfilePhotoLifecycle(t *testing.T) {
	old := &repository.ProfileImageRecord{ObjectPath: "users/user-1/old.png"}
	repo := &fakeRepository{user: &entity.User{ID: "user-1"}, image: old}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, maxProfilePhotoSize, 10*time.Minute)
	now := time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)
	service.now = func() time.Time { return now }

	result, err := service.UploadPhoto(context.Background(), "user-1", UploadedPhoto{Content: validPNG(t)})
	if err != nil {
		t.Fatalf("UploadPhoto() error = %v", err)
	}
	if result.ContentType != "image/png" || result.FileSize < 1 || repo.image == nil {
		t.Fatalf("unexpected upload result: %#v image=%#v", result, repo.image)
	}
	if len(objectStorage.deletedPaths) != 1 || objectStorage.deletedPaths[0] != old.ObjectPath {
		t.Fatalf("old object was not cleaned up: %v", objectStorage.deletedPaths)
	}

	path, contentType, size, updatedAt := repo.image.ObjectPath, repo.image.ContentType, repo.image.FileSize, repo.image.UpdatedAt
	repo.user.AvatarObjectPath, repo.user.AvatarContentType = &path, &contentType
	repo.user.AvatarFileSize, repo.user.AvatarUpdatedAt = &size, &updatedAt
	url, err := service.GetPhotoURL(context.Background(), "user-1")
	if err != nil || url.URL == "" || !url.ExpiresAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("GetPhotoURL() = %#v, %v", url, err)
	}
	if err := service.DeletePhoto(context.Background(), "user-1"); err != nil {
		t.Fatalf("DeletePhoto() error = %v", err)
	}
	if repo.image != nil || len(objectStorage.deletedPaths) != 2 {
		t.Fatalf("profile photo was not cleared: image=%#v deletes=%v", repo.image, objectStorage.deletedPaths)
	}
}

func TestUploadPhotoRejectsNonImage(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	if _, err := service.UploadPhoto(context.Background(), "user-1", UploadedPhoto{Content: []byte("not an image")}); err != constant.ErrProfilePhotoInvalid {
		t.Fatalf("UploadPhoto() error = %v, want %v", err, constant.ErrProfilePhotoInvalid)
	}
}

func TestProfilePhotoLimitCannotExceedTenMegabytes(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, 20*1024*1024, time.Minute)
	if service.MaxFileSize() != maxProfilePhotoSize {
		t.Fatalf("MaxFileSize() = %d, want %d", service.MaxFileSize(), maxProfilePhotoSize)
	}
}

func validPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&output, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return output.Bytes()
}
