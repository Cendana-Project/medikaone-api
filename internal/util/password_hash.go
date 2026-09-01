package util

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

var ErrPasswordWorkLimit = errors.New("password hashing capacity is busy")

// scrypt N=32768/r=8 uses roughly 32 MiB per invocation. This process-wide
// pool covers every service that hashes or verifies a password.
var passwordWorkSlots = make(chan struct{}, 2)

func runPasswordWork(ctx context.Context, work func() error) error {
	select {
	case passwordWorkSlots <- struct{}{}:
		defer func() { <-passwordWorkSlots }()
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrPasswordWorkLimit
	}
	return work()
}

func HashPasswordScrypt(ctx context.Context, password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	var key []byte
	err := runPasswordWork(ctx, func() error {
		var deriveErr error
		key, deriveErr = scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 64)
		return deriveErr
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key) + ":" + base64.StdEncoding.EncodeToString(salt), nil
}

func VerifyPasswordScrypt(ctx context.Context, stored, plain string) (bool, error) {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false, errors.New("invalid scrypt password encoding")
	}
	expected, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil || len(expected) == 0 {
		return false, errors.New("invalid scrypt password key")
	}
	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(salt) != 16 {
		return false, errors.New("invalid scrypt password salt")
	}
	var derived []byte
	err = runPasswordWork(ctx, func() error {
		var deriveErr error
		derived, deriveErr = scrypt.Key([]byte(plain), salt, 1<<15, 8, 1, 64)
		return deriveErr
	})
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(derived, expected) == 1, nil
}

func VerifyPasswordBcrypt(ctx context.Context, stored, plain string) (bool, error) {
	var compareErr error
	err := runPasswordWork(ctx, func() error {
		compareErr = bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain))
		return nil
	})
	if err != nil {
		return false, err
	}
	return compareErr == nil, nil
}
