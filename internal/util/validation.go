package util

import (
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	usernamePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	alphanumdashPattern = regexp.MustCompile(`^[A-Z0-9-]+$`)
)

type CustomValidator struct {
	validate *validator.Validate
}

func NewCustomValidator() *CustomValidator {
	v := validator.New()
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("alphanumdash", func(field validator.FieldLevel) bool {
		return alphanumdashPattern.MatchString(field.Field().String())
	})
	_ = v.RegisterValidation("username", func(field validator.FieldLevel) bool {
		return usernamePattern.MatchString(field.Field().String())
	})
	_ = v.RegisterValidation("validate_password", func(field validator.FieldLevel) bool {
		return IsValidPassword(field.Field().String())
	})
	_ = v.RegisterValidation("oneof_ci", func(field validator.FieldLevel) bool {
		value := strings.ToUpper(strings.TrimSpace(field.Field().String()))
		if value == "" {
			return false
		}
		for _, option := range strings.Fields(field.Param()) {
			if strings.EqualFold(option, value) {
				return true
			}
		}
		return false
	})
	return &CustomValidator{validate: v}
}

func (v *CustomValidator) Validate(value any) error {
	return v.validate.Struct(value)
}

var (
	globalValidator *CustomValidator
	validatorOnce   sync.Once
)

func GetGlobalValidator() *CustomValidator {
	validatorOnce.Do(func() {
		globalValidator = NewCustomValidator()
	})
	return globalValidator
}

func ValidateStruct(value any) error {
	return GetGlobalValidator().Validate(value)
}
