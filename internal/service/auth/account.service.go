package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"gorm.io/gorm"
)

func (s *Service) DeleteAccount(ctx context.Context, userID string, req request.DeleteAccountRequest) error {
	if userID == "" {
		return constant.ErrUnauthorized
	}
	if req.CurrentPassword == "" {
		return constant.NewFieldRequiredError("current_password")
	}
	if s.users == nil || s.redis == nil {
		return constant.ErrInternalServerError
	}
	err := s.users.DeleteAccount(ctx, userID, func(account *entity.User) error {
		return verifyDeletionPassword(ctx, account.PasswordHash, req.CurrentPassword)
	}, func() error { return s.revokeAllRefresh(ctx, userID) })
	switch {
	case err == nil:
		return nil
	case errors.Is(err, userrepo.ErrAccountHasAppointments):
		return constant.ErrResourceInUse
	case errors.Is(err, userrepo.ErrLastAccountAdministrator):
		return constant.ErrLastAdministrator
	case errors.Is(err, gorm.ErrRecordNotFound):
		return constant.ErrAccountInactive
	default:
		var public response.CustomError
		if errors.As(err, &public) {
			return public
		}
		util.Errorf(ctx, "account deletion failed error_type=%T", err)
		return constant.ErrInternalServerError
	}
}

func verifyDeletionPassword(ctx context.Context, stored, supplied string) error {
	var matched bool
	var err error
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		matched, err = util.VerifyPasswordBcrypt(ctx, stored, supplied)
	} else {
		matched, err = util.VerifyPasswordScrypt(ctx, stored, supplied)
	}
	if errors.Is(err, util.ErrPasswordWorkLimit) {
		return constant.ErrPasswordProcessingBusy
	}
	if err != nil {
		return constant.ErrInternalServerError
	}
	if !matched {
		return constant.ErrPasswordNotMatch
	}
	return nil
}
