package hospital

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	hrepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	rrepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	urepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Service struct {
	userRepo     *urepo.Repository
	roleRepo     *rrepo.Repository
	hospitalRepo *hrepo.Repository
}

func NewService(u *urepo.Repository, r *rrepo.Repository, h *hrepo.Repository) *Service {
	return &Service{userRepo: u, roleRepo: r, hospitalRepo: h}
}

func sp(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func parseDOB(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	tm, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil, err
	}
	if tm.After(time.Now().UTC()) {
		return nil, errors.New("date of birth cannot be in the future")
	}
	return &tm, nil
}

// hashScrypt returns the password format consumed by the auth service: key:salt.
func hashScrypt(ctx context.Context, plain string) (string, error) {
	return util.HashPasswordScrypt(ctx, plain)
}

func (s *Service) CreateHospital(ctx context.Context, in *request.CreateHospitalRequest) (*entity.Hospital, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	name := strings.TrimSpace(in.Name)
	address := strings.TrimSpace(in.Address)
	city := strings.TrimSpace(in.City)
	province := strings.TrimSpace(in.Province)
	phone := strings.TrimSpace(in.Phone)
	country := strings.TrimSpace(in.Country)
	if country == "" {
		country = "Indonesia"
	}

	if code == "" || name == "" || address == "" || city == "" || province == "" || phone == "" {
		return nil, constant.NewFieldRequiredError("code, name, address, city, province, and phone")
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90) {
		return nil, constant.ErrInvalidHospitalCoordinates
	}
	if in.Longitude != nil && (*in.Longitude < -180 || *in.Longitude > 180) {
		return nil, constant.ErrInvalidHospitalCoordinates
	}
	if exists, err := s.hospitalRepo.IsCodeExists(ctx, code); err != nil {
		return nil, constant.ErrInternalServerError
	} else if exists {
		return nil, constant.ErrHospitalCodeAlreadyExists
	}
	if exists, err := s.hospitalRepo.IsNameExists(ctx, name); err != nil {
		return nil, constant.ErrInternalServerError
	} else if exists {
		return nil, constant.ErrHospitalNameAlreadyExists
	}

	var facilities datatypes.JSON
	if in.Facilities != nil {
		encoded, err := json.Marshal(in.Facilities)
		if err != nil {
			return nil, constant.ErrInvalidHospitalFacilities
		}
		facilities = datatypes.JSON(encoded)
	}

	now := time.Now().UTC()
	h := &entity.Hospital{
		Code:        sp(code),
		Name:        name,
		Address:     sp(address),
		City:        sp(city),
		Province:    sp(province),
		Country:     sp(country),
		Latitude:    in.Latitude,
		Longitude:   in.Longitude,
		Phone:       sp(phone),
		Description: sp(strings.TrimSpace(in.Description)),
		Facilities:  facilities,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.hospitalRepo.Create(ctx, h); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, constant.ErrHospitalAlreadyExists
		}
		return nil, constant.ErrInternalServerError
	}
	return h, nil
}

// CreateHospitalAdmin creates the user, primary membership, and tenant role in
// one transaction. No partial account remains when a later step fails.
func (s *Service) CreateHospitalAdmin(ctx context.Context, req request.CreateHospitalAdminRequest) (string, error) {
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" || strings.TrimSpace(req.Username) == "" {
		return "", constant.NewFieldRequiredError("email, username, and password")
	}
	dob, err := parseDOB(req.DOB)
	if err != nil {
		return "", constant.ErrInvalidDateFormat
	}

	input := userInput(
		req.Email, req.Username, req.Phone, req.FirstName, req.LastName,
		dob, req.Address, req.Gender, req.NIK, req.Password,
	)

	var uid string
	err = s.hospitalRepo.Transaction(ctx, func(tx *gorm.DB) error {
		hospitalRepo := s.hospitalRepo.WithTx(tx)
		roleRepo := s.roleRepo.WithTx(tx)
		txService := &Service{
			userRepo:     s.userRepo.WithTx(tx),
			roleRepo:     roleRepo,
			hospitalRepo: hospitalRepo,
		}

		hospitalID, resolveErr := hospitalRepo.ResolveHospitalID(ctx, req.HospitalID)
		if resolveErr != nil {
			return constant.ErrHospitalNotFound
		}
		roleID, roleErr := roleRepo.GetRoleIDBySlug(ctx, constant.RoleAdmin)
		if roleErr != nil {
			return constant.ErrRoleNotFound
		}

		uid, err = txService.ensureUserActiveFull(ctx, input)
		if err != nil {
			return err
		}
		if err = hospitalRepo.EnsureMembership(ctx, uid, hospitalID, true); err != nil {
			return constant.ErrInternalServerError
		}
		if err = hospitalRepo.AssignHospitalRole(ctx, uid, hospitalID, roleID); err != nil {
			return constant.ErrInternalServerError
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return uid, nil
}

// CreateHospitalStaff validates the tenant role before writing and commits the
// user, membership, and role assignment atomically. Doctors use the dedicated
// invitation and signed-contract workflow instead of this generic endpoint.
func (s *Service) CreateHospitalStaff(ctx context.Context, req request.CreateHospitalStaffRequest) (string, error) {
	if strings.TrimSpace(req.Email) == "" ||
		strings.TrimSpace(req.Password) == "" ||
		strings.TrimSpace(req.Role) == "" ||
		strings.TrimSpace(req.Username) == "" {
		return "", constant.NewFieldRequiredError("role, email, username, and password")
	}
	dob, err := parseDOB(req.DOB)
	if err != nil {
		return "", constant.ErrInvalidDateFormat
	}
	roleSlug, ok := normalizeHospitalStaffRole(req.Role)
	if !ok {
		return "", constant.ErrRoleNotFound
	}

	input := userInput(
		req.Email, req.Username, req.Phone, req.FirstName, req.LastName,
		dob, req.Address, req.Gender, req.NIK, req.Password,
	)

	var uid string
	err = s.hospitalRepo.Transaction(ctx, func(tx *gorm.DB) error {
		hospitalRepo := s.hospitalRepo.WithTx(tx)
		roleRepo := s.roleRepo.WithTx(tx)
		txService := &Service{
			userRepo:     s.userRepo.WithTx(tx),
			roleRepo:     roleRepo,
			hospitalRepo: hospitalRepo,
		}

		hospitalID, resolveErr := hospitalRepo.ResolveHospitalID(ctx, req.HospitalID)
		if resolveErr != nil {
			return constant.ErrHospitalNotFound
		}
		// Resolve the role before creating the user. This prevents the old
		// partial-write bug for invalid or missing roles.
		roleID, roleErr := roleRepo.GetRoleIDBySlug(ctx, roleSlug)
		if roleErr != nil {
			if errors.Is(roleErr, gorm.ErrRecordNotFound) {
				return constant.ErrRoleNotFound
			}
			return constant.ErrInternalServerError
		}

		uid, err = txService.ensureUserActiveFull(ctx, input)
		if err != nil {
			return err
		}
		if err = hospitalRepo.EnsureMembership(ctx, uid, hospitalID, true); err != nil {
			return constant.ErrInternalServerError
		}
		if err = hospitalRepo.AssignHospitalRole(ctx, uid, hospitalID, roleID); err != nil {
			return constant.ErrInternalServerError
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return uid, nil
}

func normalizeHospitalStaffRole(role string) (string, bool) {
	slug := strings.ToUpper(strings.TrimSpace(role))
	switch slug {
	case constant.RoleNurse,
		constant.RoleReceptionist,
		constant.RoleBOD:
		return slug, true
	default:
		return "", false
	}
}

func userInput(
	email, username string,
	phone, firstName, lastName *string,
	dob *time.Time,
	address, gender, nik *string,
	password string,
) urepo.InsertUserFull {
	return urepo.InsertUserFull{
		Email:         strings.ToLower(strings.TrimSpace(email)),
		Username:      sp(username),
		Phone:         phone,
		FirstName:     firstName,
		LastName:      lastName,
		DOB:           dob,
		Address:       address,
		Gender:        gender,
		NIK:           nik,
		PasswordPlain: password,
	}
}

// ensureUserActiveFull only creates a new user. It must be called through the
// transaction-scoped service in the admin/staff workflows above.
func (s *Service) ensureUserActiveFull(ctx context.Context, in urepo.InsertUserFull) (string, error) {
	if len(in.PasswordPlain) > 128 || !util.IsValidPassword(in.PasswordPlain) {
		return "", constant.ErrInvalidPassword
	}
	username := ""
	if in.Username != nil {
		username = *in.Username
	}
	if util.IsPasswordSimilarToUserInfo(in.PasswordPlain, username, in.Email) {
		return "", constant.ErrPasswordSimilarToUserInfo
	}
	if u, err := s.userRepo.FindByEmail(ctx, in.Email); err != nil {
		return "", constant.ErrInternalServerError
	} else if u != nil {
		return "", constant.ErrEmailAlreadyExists
	}

	if in.Username != nil && *in.Username != "" {
		exists, err := s.userRepo.ExistsUsername(ctx, *in.Username)
		if err != nil {
			return "", constant.ErrInternalServerError
		}
		if exists {
			return "", constant.ErrUsernameAlreadyExists
		}
	}

	if in.NIK != nil && *in.NIK != "" {
		exists, err := s.userRepo.ExistsNIK(ctx, *in.NIK)
		if err != nil {
			return "", constant.ErrInternalServerError
		}
		if exists {
			return "", constant.ErrDuplicateNIK
		}
	}

	hash, err := hashScrypt(ctx, in.PasswordPlain)
	if err != nil {
		if errors.Is(err, util.ErrPasswordWorkLimit) {
			return "", constant.ErrPasswordProcessingBusy
		}
		return "", constant.ErrInternalServerError
	}
	in.PasswordHash = hash
	in.ID = uuid.NewString()
	in.Status = "active"
	now := time.Now().UTC()
	in.VerifiedAt = &now

	if err := s.userRepo.InsertActiveFull(ctx, in); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) ||
			strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return "", constant.ErrDuplicateUsernameOrEmail
		}
		return "", constant.ErrInternalServerError
	}
	return in.ID, nil
}
