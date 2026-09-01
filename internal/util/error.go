package util

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func HandleError(ctx *gin.Context, err error) {
	switch cErr := err.(type) {
	case response.CustomError:
		resp := cErr.ToResponse()
		resp.TraceID = GetTraceID(ctx)
		resp.Timestamp = time.Now().UTC()
		ctx.JSON(resp.StatusCode, resp)

	case validator.ValidationErrors:
		// Smart mapping untuk error validasi field.
		custom := mapValidationErrors(cErr)
		resp := custom.ToResponse()
		resp.TraceID = GetTraceID(ctx)
		resp.Timestamp = time.Now().UTC()
		ctx.JSON(resp.StatusCode, resp)
		// ================================================

	default:
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			resp := constant.ErrRequestTooLarge.ToResponse()
			resp.TraceID = GetTraceID(ctx)
			resp.Timestamp = time.Now().UTC()
			ctx.JSON(resp.StatusCode, resp)
			return
		}
		errStr := err.Error()
		if strings.Contains(errStr, "json") || strings.Contains(errStr, "unmarshal") {
			jsonErr := constant.ErrValidationError.ToResponse()
			detail := constant.GetMessageDetail(constant.MsgValidationError)
			if detail == (response.MessageDetail{}) {
				detail = response.MessageDetail{
					TitleEng: "Validation Error",
					TitleIdn: "format JSON tidak valid",
				}
			}
			detail.TitleEng = coalesce(detail.TitleEng, "Validation Error")
			detail.TitleIdn = "Format JSON tidak valid"

			jsonErr.MessageDetail = detail
			jsonErr.TraceID = GetTraceID(ctx)
			jsonErr.Timestamp = time.Now().UTC()
			ctx.JSON(jsonErr.StatusCode, jsonErr)
			return
		}

		if strings.Contains(errStr, "parsing time") {
			dateErr := constant.ErrInvalidDateFormat.ToResponse()
			dateErr.MessageDetail = response.MessageDetail{
				TitleEng: "Invalid Date Format",
				TitleIdn: "Gunakan format RFC3339 (contoh: 1997-12-22T00:00:00Z)",
			}
			dateErr.TraceID = GetTraceID(ctx)
			dateErr.Timestamp = time.Now().UTC()
			ctx.JSON(dateErr.StatusCode, dateErr)
			return
		}

		internalServerErr := constant.ErrInternalServerError.ToResponse()
		if internalServerErr.MessageDetail == (response.MessageDetail{}) {
			internalServerErr.MessageDetail = constant.GetMessageDetail(constant.MsgInternalServerError)
		}
		internalServerErr.TraceID = GetTraceID(ctx)
		internalServerErr.Timestamp = time.Now().UTC()
		ctx.JSON(internalServerErr.StatusCode, internalServerErr)
	}
}

// coalesce returns b if a is empty
func coalesce(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// mapValidationErrors berusaha memetakan ValidationErrors ke CustomError yang paling spesifik.
func mapValidationErrors(ve validator.ValidationErrors) response.CustomError {
	// Urutkan prioritas: duplikat -> role invalid -> tanggal -> UUID -> password -> len/format -> default
	for _, fe := range ve {
		field := strings.ToLower(fe.Field())
		tag := strings.ToLower(fe.Tag())
		param := strings.ToLower(fe.Param())

		// 1) Duplikat (custom tag unique_db) — param biasanya "users:email" / "users:username" / "users:nik"
		if tag == "unique_db" {
			if strings.Contains(param, "users:email") || field == "email" {
				return constant.ErrDuplicateUsernameOrEmail
			}
			if strings.Contains(param, "users:username") || field == "username" {
				return constant.ErrDuplicateUsernameOrEmail
			}
			if strings.Contains(param, "users:nik") || field == "nik" {
				// Pakai error khusus NIK bila ada (lihat constant). Jika belum ada, fallback ke 409 umum.
				if (constant.ErrDuplicateNIK != response.CustomError{}) {
					return constant.ErrDuplicateNIK
				}
				return constant.ErrConflict
			}
		}

		// 2) Role invalid (oneof_ci pada field role) → ROLE_NOT_FOUND
		if tag == "oneof_ci" && field == "role" {
			return constant.ErrRoleNotFound
		}

		// 3) Gender invalid (oneof_ci pada field gender) → VALIDATION_ERROR (tetap generik)
		if tag == "oneof_ci" && field == "gender" {
			return constant.ErrValidationError
		}

		// 4) DOB format salah (datetime=2006-01-02) → INVALID_DATE_FORMAT (pesan umum)
		if (tag == "datetime" || strings.Contains(fe.ActualTag(), "datetime")) && field == "dob" {
			return constant.ErrInvalidDateFormat
		}

		// 5) UUID salah (misal hospital_id dari URI) → INVALID_UUID_FORMAT
		if tag == "uuid" || tag == "uuid4" {
			return constant.ErrInvalidUUIDFormat
		}

		// 6) Password tidak memenuhi aturan (custom tag validate_password) → INVALID_PASSWORD
		if tag == "validate_password" || field == "password" && (tag == "min" || tag == "required") {
			return constant.ErrInvalidPassword
		}

		// 7) Required penting yang kosong → VALIDATION_FAILED (lebih jelas daripada error generik)
		if tag == "required" {
			return constant.ErrValidationFailed
		}

		// 8) Panjang/format NIK (len=16 + numeric) tetap VALIDATION_ERROR (biar konsisten)
		if field == "nik" && (tag == "len" || tag == "numeric") {
			return constant.ErrValidationError
		}
	}

	// Fallback: jika tidak ada yang cocok, kembalikan VALIDATION_ERROR
	return constant.ErrValidationError
}
