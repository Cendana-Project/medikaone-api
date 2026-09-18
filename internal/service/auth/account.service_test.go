package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"golang.org/x/crypto/bcrypt"
)

func TestDeleteAccountRequiresAuthenticatedAccountAndCurrentPassword(t *testing.T) {
	service := &Service{}
	if err := service.DeleteAccount(context.Background(), "", request.DeleteAccountRequest{CurrentPassword: "password"}); !errors.Is(err, constant.ErrUnauthorized) {
		t.Fatalf("missing account: %v", err)
	}
	if err := service.DeleteAccount(context.Background(), "user-1", request.DeleteAccountRequest{}); err == nil {
		t.Fatal("missing current password was accepted")
	}
}

func TestAccountDeletionVerifiesCurrentPasswordFormats(t *testing.T) {
	ctx := context.Background()
	password := "Correct-password-for-test-42!"
	scryptHash, err := util.HashPasswordScrypt(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{scryptHash, string(bcryptHash)} {
		if err := verifyDeletionPassword(ctx, hash, password); err != nil {
			t.Fatalf("correct password rejected: %v", err)
		}
		if err := verifyDeletionPassword(ctx, hash, "incorrect"); !errors.Is(err, constant.ErrPasswordNotMatch) {
			t.Fatalf("incorrect password accepted: %v", err)
		}
	}
}
