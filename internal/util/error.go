package util

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// HandleError is the single HTTP error renderer. Services return a CustomError
// and this function preserves its machine code, bilingual detail, trace ID, and
// timestamp. Unknown internal errors never leak their implementation details.
func HandleError(ctx *gin.Context, err error) {
	var custom response.CustomError
	if errors.As(err, &custom) {
		writeError(ctx, custom)
		return
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		writeError(ctx, mapValidationErrors(validationErrors))
		return
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(ctx, constant.ErrRequestTooLarge)
		return
	}

	writeError(ctx, constant.ErrInternalServerError)
}

func writeError(ctx *gin.Context, custom response.CustomError) {
	resp := custom.ToResponse()
	resp.TraceID = GetTraceID(ctx)
	resp.Timestamp = time.Now().UTC()
	ctx.JSON(resp.StatusCode, resp)
}

func mapValidationErrors(validationErrors validator.ValidationErrors) response.CustomError {
	if len(validationErrors) == 0 {
		return constant.NewInvalidFieldValueError(
			"request", "valid according to the API contract", "valid sesuai kontrak API",
		)
	}

	fieldError := validationErrors[0]
	field := fieldError.Field()
	tag := strings.ToLower(fieldError.Tag())
	param := strings.TrimSpace(fieldError.Param())

	switch tag {
	case "required":
		return constant.NewFieldRequiredError(field)
	case "email":
		return constant.ErrInvalidEmail
	case "username":
		return constant.ErrInvalidUsername
	case "validate_password":
		return constant.ErrInvalidPassword
	case "uuid", "uuid4":
		return constant.ErrInvalidUUIDFormat
	case "datetime":
		return constant.ErrInvalidDateFormat
	case "unique_db":
		switch strings.ToLower(field) {
		case "email":
			return constant.ErrEmailAlreadyExists
		case "username":
			return constant.ErrUsernameAlreadyExists
		case "nik":
			return constant.ErrDuplicateNIK
		default:
			return constant.NewDuplicateFieldValueError(field)
		}
	case "numeric":
		return constant.NewInvalidFieldValueError(field, "numeric", "berupa angka")
	case "oneof", "oneof_ci":
		return constant.NewInvalidFieldValueError(
			field,
			fmt.Sprintf("one of: %s", param),
			fmt.Sprintf("salah satu dari: %s", param),
		)
	case "uppercase":
		return constant.NewInvalidFieldValueError(field, "uppercase", "menggunakan huruf besar")
	case "alphanumdash":
		return constant.NewInvalidFieldValueError(
			field,
			"composed only of uppercase letters, numbers, or hyphens",
			"hanya terdiri dari huruf besar, angka, atau tanda hubung",
		)
	case "len":
		return constant.NewInvalidFieldLengthError(
			field,
			fmt.Sprintf("exactly %s characters or items long", param),
			fmt.Sprintf("memiliki tepat %s karakter atau item", param),
		)
	case "min":
		return constant.NewInvalidFieldLengthError(
			field,
			fmt.Sprintf("at least %s characters or items long", param),
			fmt.Sprintf("memiliki minimal %s karakter atau item", param),
		)
	case "max":
		return constant.NewInvalidFieldLengthError(
			field,
			fmt.Sprintf("at most %s characters or items long", param),
			fmt.Sprintf("memiliki maksimal %s karakter atau item", param),
		)
	default:
		return constant.NewInvalidFieldValueError(
			field,
			"valid according to the API documentation",
			"valid sesuai dokumentasi API",
		)
	}
}

// MapValidationError lets services that validate nested or polymorphic
// payloads use the same public error contract as HTTP request binding.
func MapValidationError(err error) error {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return mapValidationErrors(validationErrors)
	}
	return constant.ErrInternalServerError
}
