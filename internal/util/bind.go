package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// BindAndValidate membaca JSON secara ketat (menolak field tak dikenal),
// lalu menjalankan validator global (CustomValidator) yang sudah kamu daftarkan
// melalui util/validation.go (ValidateStruct).
func BindAndValidate(c *gin.Context, dst any) error {
	if err := DecodeStrictJSON(c.Request.Body, dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return constant.ErrRequestTooLarge
		}
		return MapJSONDecodeError(err)
	}
	if err := ValidateStruct(dst); err != nil {
		return MapValidationError(err)
	}
	return nil
}

var errMultipleJSONValues = errors.New("multiple JSON values")

// DecodeStrictJSON accepts exactly one JSON value and rejects fields that are
// not declared by dst. Keeping this in one helper ensures nested RawMessage
// payloads follow the same rules as top-level HTTP request bodies.
func DecodeStrictJSON(reader io.Reader, dst any) error {
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errMultipleJSONValues
		}
		return err
	}
	return nil
}

func UnmarshalStrictJSON(raw []byte, dst any) error {
	return DecodeStrictJSON(bytes.NewReader(raw), dst)
}

// MapJSONDecodeError converts decoder failures into stable public API errors.
func MapJSONDecodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return constant.ErrRequestTooLarge
	}
	if errors.Is(err, io.EOF) {
		return constant.ErrRequestBodyRequired
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return constant.NewInvalidFieldTypeError(typeErr.Field, typeErr.Type.String())
	}
	const unknownFieldPrefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), unknownFieldPrefix) {
		field := strings.Trim(strings.TrimPrefix(err.Error(), unknownFieldPrefix), `"`)
		return constant.NewUnknownFieldError(field)
	}
	return constant.ErrMalformedJSON
}
