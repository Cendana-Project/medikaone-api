package util

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/gin-gonic/gin"
)

func customErrorCode(t *testing.T, err error) string {
	t.Helper()
	custom, ok := err.(response.CustomError)
	if !ok {
		t.Fatalf("error type = %T, want response.CustomError", err)
	}
	if custom.Detail.DescEng == "" || custom.Detail.DescIdn == "" {
		t.Fatalf("error %s has incomplete localized detail", custom.Code)
	}
	return custom.Code
}

func TestBindAndValidateMapsJSONFailuresToSpecificCodes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty body", body: "", want: "REQUEST_BODY_REQUIRED"},
		{name: "malformed JSON", body: `{`, want: "MALFORMED_JSON"},
		{name: "multiple values", body: `{"name":"one"} {"name":"two"}`, want: "MALFORMED_JSON"},
		{name: "unknown field", body: `{"name":"one","otp":"123456"}`, want: "UNKNOWN_FIELD"},
		{name: "wrong type", body: `{"name":42}`, want: "INVALID_FIELD_TYPE"},
		{name: "missing required field", body: `{}`, want: "FIELD_REQUIRED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var destination bindFixture
			err := BindAndValidate(bindContext(test.body), &destination)
			if err == nil {
				t.Fatal("expected request error")
			}
			if got := customErrorCode(t, err); got != test.want {
				t.Fatalf("error code = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHandleErrorPreservesWrappedCustomErrorContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("trace_id", "test-trace-id")

	HandleError(ctx, fmt.Errorf("service context: %w", constant.ErrHospitalNotFound))

	if recorder.Code != constant.ErrHospitalNotFound.StatusCode {
		t.Fatalf("HTTP status = %d, want %d", recorder.Code, constant.ErrHospitalNotFound.StatusCode)
	}
	var body response.BaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != constant.ErrHospitalNotFound.Code {
		t.Fatalf("message = %q, want %q", body.Message, constant.ErrHospitalNotFound.Code)
	}
	if body.TraceID != "test-trace-id" {
		t.Fatalf("trace_id = %q", body.TraceID)
	}
	if !strings.Contains(body.MessageDetail.DescIdn, "Rumah sakit") {
		t.Fatalf("Indonesian detail is not hospital-specific: %q", body.MessageDetail.DescIdn)
	}
}
