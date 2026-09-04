package util

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/gin-gonic/gin"
)

func TestHandleResponsePreservesOperationSpecificSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/hospitals", nil)

	result := constant.NewSuccessResponse(constant.MsgHospitalCreated)
	result.StatusCode = http.StatusCreated
	result.Data = gin.H{"id": "hospital-1"}
	HandleResponse(ctx, result, nil)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	var body response.BaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != string(constant.MsgHospitalCreated) {
		t.Fatalf("message = %q, want %q", body.Message, constant.MsgHospitalCreated)
	}
	if body.MessageDetail != constant.GetMessageDetail(constant.MsgHospitalCreated) {
		t.Fatalf("message detail was not preserved: %#v", body.MessageDetail)
	}
}
