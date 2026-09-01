package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

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
		// mapping ke katalog error validasi kamu
		return constant.ErrValidationError
	}
	// Gunakan ValidateStruct yang sudah didefinisikan di util/validation.go
	return ValidateStruct(dst)
}

// DecodeStrictJSON accepts exactly one JSON value and rejects fields that are
// not declared by dst. Keeping this in one helper ensures nested RawMessage
// payloads follow the same rules as top-level HTTP request bodies.
func DecodeStrictJSON(reader io.Reader, dst any) error {
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func UnmarshalStrictJSON(raw []byte, dst any) error {
	return DecodeStrictJSON(bytes.NewReader(raw), dst)
}

// Opsional: helper cepat untuk mengakhiri request dengan error validasi standar
func AbortValidation(c *gin.Context) {
	res := constant.ErrValidationError.ToResponse()
	c.AbortWithStatusJSON(res.StatusCode, res)
}
