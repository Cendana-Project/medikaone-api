package infrastructure

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func TestRequestBodyLimitAllowsMaximumFileWithMultipartFraming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const maxFile = 10 << 20
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("contract", "maximum.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(make([]byte, maxFile)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, streamed := range []bool{false, true} {
		router := gin.New()
		router.Use(limitRequestBody(maxFile))
		router.POST("/", func(c *gin.Context) {
			if err := c.Request.ParseMultipartForm(maxFile); err != nil {
				util.HandleError(c, err)
				return
			}
			defer c.Request.MultipartForm.RemoveAll()
			if c.Request.MultipartForm.File["contract"][0].Size != maxFile {
				t.Fatal("file truncated")
			}
			c.Status(http.StatusNoContent)
		})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body.Bytes()))
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if streamed {
			req.ContentLength = -1
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("maximum upload rejected due to multipart overhead: %s", recorder.Body.String())
		}
	}
}
