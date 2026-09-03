package doctorhospital

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCreateInvitationRejectsRequestLargerThanTenMegabytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("contract", "contract.pdf")
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, maxMultipartRequestBytes)
	copy(payload, []byte("%PDF-"))
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if int64(body.Len()) <= maxMultipartRequestBytes {
		t.Fatalf("test body must exceed %d bytes", maxMultipartRequestBytes)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request

	controller := &Controller{}
	controller.CreateInvitation(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 for oversized request, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
