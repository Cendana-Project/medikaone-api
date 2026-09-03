package constant

import (
	"regexp"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func TestSuccessMessageCatalogContract(t *testing.T) {
	codePattern := regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	if len(MessageCatalog) < 60 {
		t.Fatalf("success catalog contains %d entries; expected all API outcomes", len(MessageCatalog))
	}

	for code, detail := range MessageCatalog {
		if !codePattern.MatchString(string(code)) {
			t.Errorf("success code %q is not UPPER_SNAKE_CASE", code)
		}
		if detail.TitleEng == "" || detail.DescEng == "" || detail.TitleIdn == "" || detail.DescIdn == "" {
			t.Errorf("success code %s does not have complete bilingual detail: %#v", code, detail)
		}

		result := NewSuccessResponse(code)
		if result.Message != string(code) {
			t.Errorf("success code %s rendered as %q", code, result.Message)
		}
		if result.MessageDetail != detail {
			t.Errorf("success code %s rendered the wrong detail", code)
		}
	}
}

func TestUnknownSuccessMessageFallsBackSafely(t *testing.T) {
	result := NewSuccessResponse(MessageCode("UNKNOWN_SUCCESS"))
	if result.Message != string(MsgSuccess) {
		t.Fatalf("message = %q, want %q", result.Message, MsgSuccess)
	}
	if result.MessageDetail == (response.MessageDetail{}) {
		t.Fatal("fallback success detail is empty")
	}
}
