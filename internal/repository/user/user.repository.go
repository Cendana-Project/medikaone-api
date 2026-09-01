package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) WithTx(tx *gorm.DB) *Repository { return &Repository{db: tx} }

func (r *Repository) Begin(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Begin()
}

func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	var u entity.User
	err := r.db.WithContext(ctx).Where("LOWER(email)=LOWER(?)", strings.ToLower(email)).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) FindByUsername(ctx context.Context, uname string) (*entity.User, error) {
	var u entity.User
	err := r.db.WithContext(ctx).Where("LOWER(username)=LOWER(?)", strings.ToLower(uname)).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

// FindByIdentityForAuth holds a shared row lock for the surrounding
// transaction. Password changes/resets take an exclusive lock, making login
// credential verification and session issuance serializable with them.
func (r *Repository) FindByIdentityForAuth(ctx context.Context, identity string) (*entity.User, error) {
	var u entity.User
	query := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"})
	if strings.Contains(identity, "@") {
		query = query.Where("LOWER(email)=LOWER(?)", strings.ToLower(identity))
	} else {
		query = query.Where("LOWER(username)=LOWER(?)", strings.ToLower(identity))
	}
	err := query.First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) FindByEmailForUpdate(ctx context.Context, email string) (*entity.User, error) {
	var u entity.User
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("LOWER(email)=LOWER(?)", strings.ToLower(email)).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) GetByIDForUpdate(ctx context.Context, id string) (*entity.User, error) {
	var u entity.User
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&u, "id = ? AND deleted_at IS NULL", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) ExistsUsername(ctx context.Context, uname string) (bool, error) {
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("LOWER(username)=LOWER(?)", strings.ToLower(uname)).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (r *Repository) Create(ctx context.Context, u *entity.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *Repository) MarkVerified(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":      "active",
			"verified_at": now,
			"updated_at":  now,
		}).Error
}

func (r *Repository) UpdateByID(ctx context.Context, id string, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Updates(fields).Error
}

// ActivatePendingRegistration updates only an account that is still pending.
// The status predicate is evaluated by PostgreSQL after acquiring the row lock,
// so an administrator blocking the account concurrently cannot be overwritten.
func (r *Repository) ActivatePendingRegistration(ctx context.Context, id string, fields map[string]any) (bool, error) {
	fields["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", id, "pending").
		Updates(fields)
	return result.RowsAffected == 1, result.Error
}

func (r *Repository) UpdateActivePassword(ctx context.Context, id, passwordHash string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", id, "active").
		Updates(map[string]any{"password_hash": passwordHash, "updated_at": time.Now()})
	return result.RowsAffected == 1, result.Error
}

// Profiles (UPSERT)
func (r *Repository) UpsertPatientProfile(ctx context.Context, p map[string]any) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO patient_profiles (user_id, height_cm, weight_kg, allergies, medical_hist, created_at, updated_at)
		VALUES (@user_id, @height_cm, @weight_kg, @allergies, @medical_hist, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
		  height_cm   = EXCLUDED.height_cm,
		  weight_kg   = EXCLUDED.weight_kg,
		  allergies   = EXCLUDED.allergies,
		  medical_hist= EXCLUDED.medical_hist,
		  updated_at  = NOW();
	`, p).Error
}

func (r *Repository) UpsertDoctorProfile(ctx context.Context, p map[string]any) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO doctor_profiles (user_id, sip_number, specialty, created_at, updated_at)
		VALUES (@user_id, @sip_number, @specialty, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
		  sip_number = EXCLUDED.sip_number,
		  specialty  = EXCLUDED.specialty,
		  updated_at = NOW();
	`, p).Error
}

func (r *Repository) GetByID(ctx context.Context, id string) (*entity.User, error) {
	var u entity.User
	if err := r.db.WithContext(ctx).First(&u, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*entity.User, error) {
	var u entity.User
	err := r.db.WithContext(ctx).First(&u, "email = ? AND deleted_at IS NULL", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *Repository) Update(ctx context.Context, u *entity.User) error {
	return r.db.WithContext(ctx).Save(u).Error
}

type InsertUser struct {
	ID           string
	Email        string
	Username     string
	Phone        string
	FirstName    string
	LastName     string
	PasswordHash string
	VerifiedAt   time.Time
}

func (r *Repository) InsertActive(ctx context.Context, in InsertUser) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO users (id, email, username, phone, first_name, last_name, password_hash, status, verified_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, NOW())
	`, in.ID, in.Email, in.Username, in.Phone, in.FirstName, in.LastName, in.PasswordHash, in.VerifiedAt).Error
}
func (r *Repository) ExistsNIK(ctx context.Context, nik string) (bool, error) {
	if strings.TrimSpace(nik) == "" {
		return false, nil
	}
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("nik = ? AND deleted_at IS NULL", nik).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (r *Repository) ExistsNIKExcludingUser(ctx context.Context, nik, userID string) (bool, error) {
	if strings.TrimSpace(nik) == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("nik = ? AND id <> ? AND deleted_at IS NULL", nik, userID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type InsertUserFull struct {
	ID            string
	Email         string
	Username      *string
	Phone         *string
	FirstName     *string
	LastName      *string
	DOB           *time.Time
	Address       *string
	Gender        *string // "L" | "P"
	NIK           *string
	PasswordPlain string // input (akan di-hash di service)
	PasswordHash  string // hasil scrypt "hash:salt"
	Status        string // "active"
	VerifiedAt    *time.Time
}

// InsertActiveFull: membuat user aktif dengan field lengkap (tanpa verifikasi)
func (r *Repository) InsertActiveFull(ctx context.Context, in InsertUserFull) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO users (
			id, email, username, first_name, last_name, phone, dob, address, gender, nik,
			password_hash, status, verified_at, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, NOW())
	`, in.ID, in.Email, in.Username, in.FirstName, in.LastName, in.Phone, in.DOB, in.Address, in.Gender, in.NIK,
		in.PasswordHash, in.VerifiedAt).Error
}

func (r *Repository) GetPatientProfileByUserID(ctx context.Context, userID string) (heightCM, weightKG *int, allergies, medicalHist *string, err error) {
	type row struct {
		HeightCM    *int
		WeightKG    *int
		Allergies   *string
		MedicalHist *string
	}
	var out row
	err = r.db.WithContext(ctx).Raw(`
		SELECT height_cm, weight_kg, allergies, medical_hist
		FROM patient_profiles
		WHERE user_id = ?
		LIMIT 1
	`, userID).Scan(&out).Error
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return out.HeightCM, out.WeightKG, out.Allergies, out.MedicalHist, nil
}

// GetDoctorProfileByUserID mengambil data doctor_profiles untuk user tertentu.
// GetDoctorProfileByUserID mengambil data doctor_profiles untuk user tertentu.
func (r *Repository) GetDoctorProfileByUserID(ctx context.Context, userID string) (sipNumber, specialty *string, err error) {
	type row struct {
		SipNumber *string `gorm:"column:sip_number"`
		Specialty *string `gorm:"column:specialty"`
	}
	var out row
	err = r.db.WithContext(ctx).Raw(`
		SELECT sip_number, specialty
		FROM doctor_profiles
		WHERE user_id = ?
		LIMIT 1
	`, userID).Scan(&out).Error
	if err != nil {
		return nil, nil, err
	}
	return out.SipNumber, out.Specialty, nil
}

func (r *Repository) ExistsPatientProfile(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM patient_profiles WHERE user_id = ?)`, userID).
		Scan(&exists).Error
	return exists, err
}

func (r *Repository) ExistsDoctorProfile(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM doctor_profiles WHERE user_id = ?)`, userID).
		Scan(&exists).Error
	return exists, err
}

// ====== Untuk /v1/me (global) ======

func (r *Repository) GetUserRoleSlug(ctx context.Context, userID string) (string, error) {
	var slug string
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.slug
		FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = ?
		  AND u.status = 'active' AND u.deleted_at IS NULL
		  AND r.active = TRUE AND r.deleted_at IS NULL
		ORDER BY CASE UPPER(r.slug)
		  WHEN 'SUPER_ADMIN' THEN 1
		  WHEN 'ADMIN' THEN 2
		  WHEN 'DOCTOR' THEN 3
		  WHEN 'NURSE' THEN 4
		  WHEN 'RECEPTIONIST' THEN 5
		  WHEN 'BOD' THEN 6
		  WHEN 'PATIENT' THEN 7
		  ELSE 100 END, UPPER(r.slug)
		LIMIT 1
	`, userID).Scan(&slug).Error
	if err != nil {
		return "", err
	}
	return strings.ToUpper(slug), nil
}

type HospitalBrief struct {
	ID   string
	Code string
	Name string
}

func (r *Repository) ListHospitalsByUserID(ctx context.Context, userID string) ([]HospitalBrief, error) {
	var rows []HospitalBrief
	err := r.db.WithContext(ctx).Raw(`
		SELECT h.id, h.code, h.name
		FROM user_hospitals uh
		JOIN users u ON u.id = uh.user_id
		JOIN hospitals h ON h.id = uh.hospital_id
		WHERE uh.user_id = ?
		  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
		  AND u.status = 'active' AND u.deleted_at IS NULL
		  AND h.is_active = TRUE AND h.deleted_at IS NULL
		ORDER BY uh.is_primary DESC, h.name
	`, userID).Scan(&rows).Error
	return rows, err
}

// ====== Untuk /v1/tenant/me (scoped) ======

// ResolveHospitalHint: hint bisa berupa UUID (id) atau code (string).
func (r *Repository) ResolveHospitalHint(ctx context.Context, hint string) (*HospitalBrief, error) {
	type row struct{ ID, Code, Name string }
	var out row
	// coba cocokkan ID (UUID)
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, code, name FROM hospitals
		WHERE id::text = ? AND is_active = TRUE AND deleted_at IS NULL
		LIMIT 1
	`, hint).Scan(&out).Error
	if err == nil && out.ID != "" {
		return &HospitalBrief{ID: out.ID, Code: out.Code, Name: out.Name}, nil
	}
	// fallback ke CODE
	out = row{}
	err = r.db.WithContext(ctx).Raw(`
		SELECT id, code, name FROM hospitals
		WHERE LOWER(code) = LOWER(?) AND is_active = TRUE AND deleted_at IS NULL
		LIMIT 1
	`, hint).Scan(&out).Error
	if err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &HospitalBrief{ID: out.ID, Code: out.Code, Name: out.Name}, nil
}

func (r *Repository) IsMemberOfHospital(ctx context.Context, userID, hospitalID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1
			FROM user_hospitals uh
			JOIN users u ON u.id = uh.user_id
			JOIN hospitals h ON h.id = uh.hospital_id
			WHERE uh.user_id = ? AND uh.hospital_id = ?
			  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
			  AND u.status = 'active' AND u.deleted_at IS NULL
			  AND h.is_active = TRUE AND h.deleted_at IS NULL
		)
	`, userID, hospitalID).Scan(&exists).Error
	return exists, err
}

// Role user di hospital tertentu (dari hospital_user_roles).
func (r *Repository) GetHospitalRoleSlug(ctx context.Context, userID, hospitalID string) (string, error) {
	var slug string
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.slug
		FROM hospital_user_roles hur
		JOIN user_hospitals uh ON uh.user_id = hur.user_id AND uh.hospital_id = hur.hospital_id
		JOIN hospitals h ON h.id = hur.hospital_id
		JOIN users u ON u.id = hur.user_id
		JOIN roles r ON r.id = hur.role_id
		WHERE hur.user_id = ? AND hur.hospital_id = ?
		  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
		  AND h.is_active = TRUE AND h.deleted_at IS NULL
		  AND u.status = 'active' AND u.deleted_at IS NULL
		  AND r.active = TRUE AND r.deleted_at IS NULL
		ORDER BY CASE UPPER(r.slug)
		  WHEN 'SUPER_ADMIN' THEN 1
		  WHEN 'ADMIN' THEN 2
		  WHEN 'DOCTOR' THEN 3
		  WHEN 'NURSE' THEN 4
		  WHEN 'RECEPTIONIST' THEN 5
		  WHEN 'BOD' THEN 6
		  WHEN 'PATIENT' THEN 7
		  ELSE 100 END, UPPER(r.slug)
		LIMIT 1
	`, userID, hospitalID).Scan(&slug).Error
	if err != nil {
		return "", err
	}
	return strings.ToUpper(slug), nil
}
