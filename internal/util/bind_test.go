package util

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type bindFixture struct {
	Name string `json:"name" validate:"required"`
}

func bindContext(body string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestBindAndValidateAcceptsExactlyOneJSONValue(t *testing.T) {
	var got bindFixture
	if err := BindAndValidate(bindContext(`{"name":"MedikaOne"}`), &got); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}
	if got.Name != "MedikaOne" {
		t.Fatalf("decoded name = %q", got.Name)
	}
}

func TestBindAndValidateRejectsTrailingJSONValue(t *testing.T) {
	var got bindFixture
	if err := BindAndValidate(bindContext(`{"name":"first"} {"name":"second"}`), &got); err == nil {
		t.Fatal("multiple JSON values must be rejected")
	}
}

func TestUnmarshalStrictJSONRejectsUnknownAndTrailingFields(t *testing.T) {
	for _, raw := range []string{
		`{"name":"MedikaOne","unexpected":true}`,
		`{"name":"MedikaOne"} {"name":"second"}`,
	} {
		var got bindFixture
		if err := UnmarshalStrictJSON([]byte(raw), &got); err == nil {
			t.Fatalf("strict nested decode accepted %s", raw)
		}
	}
}
