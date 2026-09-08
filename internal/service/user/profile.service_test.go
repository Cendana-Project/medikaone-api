package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"gorm.io/gorm"
)

type fakeRepository struct {
	user                     *entity.User
	usernameExists           bool
	nikExists                bool
	sipExists                bool
	usernameExistsAfterApply bool
	nikExistsAfterApply      bool
	sipExistsAfterApply      bool
	usernameChecks           []string
	nikChecks                []string
	sipChecks                []string
	globalRoles              map[string]bool
	hospitalRoles            map[string]bool
	hospitalWorker           bool
	currentSIP               *string
	profileUpdateErr         error
	profileUpdateAttempted   bool
	updates                  map[string]any
	profileUpdate            repository.UnifiedProfileUpdate
	image                    *repository.ProfileImageRecord
}

func (f *fakeRepository) GetByID(context.Context, string) (*entity.User, error) { return f.user, nil }
func (f *fakeRepository) GetDoctorProfileByUserID(context.Context, string) (*string, *string, error) {
	return f.currentSIP, nil, nil
}
func (f *fakeRepository) ExistsUsernameExcludingUser(_ context.Context, value, _ string) (bool, error) {
	f.usernameChecks = append(f.usernameChecks, value)
	if f.profileUpdateAttempted {
		return f.usernameExistsAfterApply, nil
	}
	return f.usernameExists, nil
}
func (f *fakeRepository) ExistsNIKExcludingUser(_ context.Context, value, _ string) (bool, error) {
	f.nikChecks = append(f.nikChecks, value)
	if f.profileUpdateAttempted {
		return f.nikExistsAfterApply, nil
	}
	return f.nikExists, nil
}
func (f *fakeRepository) ExistsSIPExcludingUser(_ context.Context, value, _ string) (bool, error) {
	f.sipChecks = append(f.sipChecks, value)
	if f.profileUpdateAttempted {
		return f.sipExistsAfterApply, nil
	}
	return f.sipExists, nil
}
func (f *fakeRepository) UserHasActiveGlobalRole(_ context.Context, _ string, role string) (bool, error) {
	return f.globalRoles[role], nil
}
func (f *fakeRepository) UserHasAnyActiveHospitalRole(_ context.Context, _ string, role string) (bool, error) {
	return f.hospitalRoles[role], nil
}
func (f *fakeRepository) UserIsActiveHospitalWorker(context.Context, string) (bool, error) {
	return f.hospitalWorker, nil
}
func (f *fakeRepository) ApplyUnifiedProfileUpdate(_ context.Context, _ string, update repository.UnifiedProfileUpdate) error {
	f.profileUpdate = update
	f.updates = update.UserFields
	f.profileUpdateAttempted = true
	return f.profileUpdateErr
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
		Username: request.NewPatchValue(username), FirstName: request.NewPatchValue(firstName),
		Phone: request.NewPatchValue(phone), DOB: request.NewPatchValue(dob),
		Gender: request.NewPatchValue(gender), NIK: request.NewPatchValue(nik),
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

func TestUpdateRejectsNullRoleProfileObject(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	for _, test := range []struct {
		body  string
		field string
	}{
		{body: `{"first_name":"Budi","patient_profile":null}`, field: "patient_profile"},
		{body: `{"first_name":"Budi","doctor_profile":null}`, field: "doctor_profile"},
	} {
		var input request.UpdateUserProfileRequest
		if err := json.Unmarshal([]byte(test.body), &input); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		want := constant.NewInvalidFieldTypeError(test.field, "object")
		if err := service.Update(context.Background(), "user-1", input); !errors.Is(err, want) {
			t.Fatalf("Update() error = %v, want %v", err, want)
		}
	}
}

func TestUpdateDistinguishesOmittedAndNullFields(t *testing.T) {
	repo := &fakeRepository{
		user:        &entity.User{ID: "patient-1"},
		globalRoles: map[string]bool{constant.RolePatient: true},
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	var input request.UpdateUserProfileRequest
	if err := json.Unmarshal([]byte(`{
		"phone": null,
		"patient_profile": {"height_cm": null, "allergies": null}
	}`), &input); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	if err := service.Update(context.Background(), "patient-1", input); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if value, exists := repo.profileUpdate.UserFields["phone"]; !exists || value != nil {
		t.Fatalf("explicit null phone was not persisted as NULL: %#v", repo.profileUpdate.UserFields)
	}
	if _, exists := repo.profileUpdate.UserFields["address"]; exists {
		t.Fatalf("omitted address must remain unchanged: %#v", repo.profileUpdate.UserFields)
	}
	if value, exists := repo.profileUpdate.PatientFields["height_cm"]; !exists || value != nil {
		t.Fatalf("explicit null height was not persisted as NULL: %#v", repo.profileUpdate.PatientFields)
	}
	if value, exists := repo.profileUpdate.PatientFields["allergies"]; !exists || value != nil {
		t.Fatalf("explicit null allergies was not persisted as NULL: %#v", repo.profileUpdate.PatientFields)
	}
	if _, exists := repo.profileUpdate.PatientFields["weight_kg"]; exists {
		t.Fatalf("omitted weight must remain unchanged: %#v", repo.profileUpdate.PatientFields)
	}
}

func TestUpdateRoleSpecificFieldsRequireMatchingRole(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "patient", body: `{"patient_profile":{"allergies":"dust"}}`, want: constant.ErrPatientRoleRequired},
		{name: "doctor", body: `{"doctor_profile":{"specialty":"Cardiology"}}`, want: constant.ErrDoctorRoleRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{user: &entity.User{ID: "user-1"}}
			service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
			var input request.UpdateUserProfileRequest
			if err := json.Unmarshal([]byte(test.body), &input); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			if err := service.Update(context.Background(), "user-1", input); !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestUpdateDoctorSIPCannotBeCleared(t *testing.T) {
	for _, body := range []string{
		`{"doctor_profile":{"sip_number":null}}`,
		`{"doctor_profile":{"sip_number":"  "}}`,
	} {
		repo := &fakeRepository{
			user:        &entity.User{ID: "doctor-1"},
			globalRoles: map[string]bool{constant.RoleDoctor: true},
		}
		service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
		var input request.UpdateUserProfileRequest
		if err := json.Unmarshal([]byte(body), &input); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		want := constant.NewFieldRequiredError("doctor_profile.sip_number")
		if err := service.Update(context.Background(), "doctor-1", input); !errors.Is(err, want) {
			t.Fatalf("Update() error = %v, want %v", err, want)
		}
	}
}

func TestUpdateDoctorSpecialtyRequiresExistingSIP(t *testing.T) {
	for _, name := range []string{"new doctor profile", "legacy profile with null SIP"} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepository{
				user:        &entity.User{ID: "doctor-1"},
				globalRoles: map[string]bool{constant.RoleDoctor: true},
			}
			service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
			input := request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{
				Specialty: request.NewPatchValue("Cardiology"),
			}}
			want := constant.NewFieldRequiredError("doctor_profile.sip_number")
			if err := service.Update(context.Background(), "doctor-1", input); !errors.Is(err, want) {
				t.Fatalf("Update() error = %v, want %v", err, want)
			}
		})
	}
}

func TestUpdateDoctorSpecialtyPreservesExistingSIP(t *testing.T) {
	currentSIP := "SIP-EXISTING"
	repo := &fakeRepository{
		user:        &entity.User{ID: "doctor-1"},
		currentSIP:  &currentSIP,
		globalRoles: map[string]bool{constant.RoleDoctor: true},
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	input := request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{
		Specialty: request.NewPatchValue("Cardiology"),
	}}
	if err := service.Update(context.Background(), "doctor-1", input); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := repo.profileUpdate.DoctorFields["specialty"]; got != "Cardiology" {
		t.Fatalf("specialty = %#v, want Cardiology", got)
	}
	if _, exists := repo.profileUpdate.DoctorFields["sip_number"]; exists {
		t.Fatalf("omitted existing SIP must not be rewritten: %#v", repo.profileUpdate.DoctorFields)
	}
}

func TestUpdateDoctorSIPMustRemainUnique(t *testing.T) {
	repo := &fakeRepository{
		user: &entity.User{ID: "doctor-1"}, sipExists: true,
		hospitalRoles: map[string]bool{constant.RoleDoctor: true},
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	var input request.UpdateUserProfileRequest
	if err := json.Unmarshal([]byte(`{"doctor_profile":{"sip_number":" SIP-002 "}}`), &input); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if err := service.Update(context.Background(), "doctor-1", input); !errors.Is(err, constant.ErrDoctorSIPAlreadyExists) {
		t.Fatalf("Update() error = %v, want %v", err, constant.ErrDoctorSIPAlreadyExists)
	}
}

func TestUpdateDoctorSIPMapsDatabaseRaceConflict(t *testing.T) {
	repo := &fakeRepository{
		user:             &entity.User{ID: "doctor-1"},
		globalRoles:      map[string]bool{constant.RoleDoctor: true},
		profileUpdateErr: repository.ErrDoctorSIPConflict,
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	var input request.UpdateUserProfileRequest
	if err := json.Unmarshal([]byte(`{"doctor_profile":{"sip_number":"sip-race-001"}}`), &input); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if err := service.Update(context.Background(), "doctor-1", input); !errors.Is(err, constant.ErrDoctorSIPAlreadyExists) {
		t.Fatalf("Update() error = %v, want %v", err, constant.ErrDoctorSIPAlreadyExists)
	}
}

func TestUpdateMapsTranslatedDuplicateAfterUniquenessRace(t *testing.T) {
	tests := []struct {
		name          string
		input         request.UpdateUserProfileRequest
		configureRepo func(*fakeRepository)
		want          error
		checkedValues func(*fakeRepository) []string
		wantValue     string
	}{
		{
			name:      "username",
			input:     request.UpdateUserProfileRequest{Username: request.NewPatchValue("Race.User")},
			want:      constant.ErrUsernameAlreadyExists,
			wantValue: "race.user",
			configureRepo: func(repo *fakeRepository) {
				repo.usernameExistsAfterApply = true
			},
			checkedValues: func(repo *fakeRepository) []string { return repo.usernameChecks },
		},
		{
			name:      "nik",
			input:     request.UpdateUserProfileRequest{NIK: request.NewPatchValue("3201010101010001")},
			want:      constant.ErrDuplicateNIK,
			wantValue: "3201010101010001",
			configureRepo: func(repo *fakeRepository) {
				repo.nikExistsAfterApply = true
			},
			checkedValues: func(repo *fakeRepository) []string { return repo.nikChecks },
		},
		{
			name: "sip number",
			input: request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{
				SIPNumber: request.NewPatchValue(" SIP-RACE-001 "),
			}},
			want:      constant.ErrDoctorSIPAlreadyExists,
			wantValue: "SIP-RACE-001",
			configureRepo: func(repo *fakeRepository) {
				repo.globalRoles = map[string]bool{constant.RoleDoctor: true}
				repo.sipExistsAfterApply = true
			},
			checkedValues: func(repo *fakeRepository) []string { return repo.sipChecks },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{
				user:             &entity.User{ID: "user-1"},
				profileUpdateErr: gorm.ErrDuplicatedKey,
			}
			test.configureRepo(repo)
			service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)

			if err := service.Update(context.Background(), "user-1", test.input); !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want %v", err, test.want)
			}
			checks := test.checkedValues(repo)
			if len(checks) != 2 {
				t.Fatalf("uniqueness check count = %d, want precheck and post-failure check; values=%v", len(checks), checks)
			}
			if checks[1] != test.wantValue {
				t.Fatalf("post-failure value = %q, want normalized %q", checks[1], test.wantValue)
			}
		})
	}
}

func TestUpdateKeepsGenericConflictWhenTranslatedDuplicateCannotBeClassified(t *testing.T) {
	repo := &fakeRepository{
		user:             &entity.User{ID: "user-1"},
		profileUpdateErr: gorm.ErrDuplicatedKey,
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)

	err := service.Update(context.Background(), "user-1", request.UpdateUserProfileRequest{
		FirstName: request.NewPatchValue("Budi"),
	})
	if !errors.Is(err, constant.ErrConflict) {
		t.Fatalf("Update() error = %v, want %v", err, constant.ErrConflict)
	}
}

func TestUpdateDOBRequiresHospitalWorkerOlderThanFifteen(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		worker bool
		dob    string
		want   error
	}{
		{name: "patient may be exactly fifteen", dob: "2011-09-07"},
		{name: "worker exactly fifteen rejected", worker: true, dob: "2011-09-07", want: constant.ErrHospitalWorkerMinimumAge},
		{name: "worker older than fifteen", worker: true, dob: "2011-09-06"},
		{name: "future remains invalid", dob: "2027-01-01", want: constant.ErrInvalidDateFormat},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{user: &entity.User{ID: "user-1"}, hospitalWorker: test.worker}
			service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
			service.now = func() time.Time { return now }
			input := request.UpdateUserProfileRequest{DOB: request.NewPatchValue(test.dob)}
			if err := service.Update(context.Background(), "user-1", input); !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestUpdateHospitalWorkerCannotBypassDOBByOmittingIt(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	exactlyFifteen := now.AddDate(-15, 0, 0)
	olderThanFifteen := now.AddDate(-15, 0, -1)
	tests := []struct {
		name string
		dob  *time.Time
		want error
	}{
		{name: "missing dob", want: constant.NewFieldRequiredError("dob")},
		{name: "exactly fifteen", dob: &exactlyFifteen, want: constant.ErrHospitalWorkerMinimumAge},
		{name: "older than fifteen", dob: &olderThanFifteen},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{
				user:           &entity.User{ID: "worker-1", DOB: test.dob},
				hospitalWorker: true,
			}
			service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
			service.now = func() time.Time { return now }
			input := request.UpdateUserProfileRequest{FirstName: request.NewPatchValue("Worker")}
			if err := service.Update(context.Background(), "worker-1", input); !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestUpdateNullableDoctorSpecialty(t *testing.T) {
	currentSIP := "SIP-EXISTING"
	repo := &fakeRepository{
		user:        &entity.User{ID: "doctor-1"},
		currentSIP:  &currentSIP,
		globalRoles: map[string]bool{constant.RoleDoctor: true},
	}
	service := NewService(repo, &fakeStorage{}, maxProfilePhotoSize, time.Minute)
	input := request.UpdateUserProfileRequest{DoctorProfile: &request.UpdateDoctorProfileRequest{
		Specialty: request.NewPatchNull[string](),
	}}
	if err := service.Update(context.Background(), "doctor-1", input); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if value, exists := repo.profileUpdate.DoctorFields["specialty"]; !exists || value != nil {
		t.Fatalf("specialty null was not persisted: %#v", repo.profileUpdate.DoctorFields)
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
