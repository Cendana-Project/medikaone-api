package constant

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func TestAPIErrorCatalogContract(t *testing.T) {
	codePattern := regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	seen := make(map[string]struct{})

	for _, apiErr := range APIErrorCatalog() {
		if !codePattern.MatchString(apiErr.Code) {
			t.Errorf("error code %q is not UPPER_SNAKE_CASE", apiErr.Code)
		}
		if _, exists := seen[apiErr.Code]; exists {
			t.Errorf("duplicate public error code %q", apiErr.Code)
		}
		seen[apiErr.Code] = struct{}{}
		if apiErr.StatusCode < 400 || apiErr.StatusCode > 599 {
			t.Errorf("error %s has invalid HTTP status %d", apiErr.Code, apiErr.StatusCode)
		}
		if apiErr.Message == "" {
			t.Errorf("error %s has no internal message", apiErr.Code)
		}
		if apiErr.Detail.TitleEng == "" || apiErr.Detail.DescEng == "" ||
			apiErr.Detail.TitleIdn == "" || apiErr.Detail.DescIdn == "" {
			t.Errorf("error %s does not have complete bilingual detail: %#v", apiErr.Code, apiErr.Detail)
		}

		public := apiErr.ToResponse()
		if public.Message != apiErr.Code {
			t.Errorf("error %s rendered machine code as %q", apiErr.Code, public.Message)
		}
		if public.StatusCode != apiErr.StatusCode {
			t.Errorf("error %s rendered HTTP status %d, want %d", apiErr.Code, public.StatusCode, apiErr.StatusCode)
		}
	}
}

func TestValidationAliasesExposeOnePublicCode(t *testing.T) {
	if ErrValidationFailed != ErrValidationError {
		t.Fatalf("validation aliases diverged: %#v != %#v", ErrValidationFailed, ErrValidationError)
	}
	if ErrUserNotAuthenticated != ErrUnauthorized {
		t.Fatalf("authentication aliases diverged: %#v != %#v", ErrUserNotAuthenticated, ErrUnauthorized)
	}
}

func TestDynamicValidationErrorsAreFieldSpecific(t *testing.T) {
	tests := []struct {
		name  string
		field string
		err   response.CustomError
	}{
		{name: "required", field: "challenge_id", err: NewFieldRequiredError("challenge_id")},
		{name: "unknown", field: "otp", err: NewUnknownFieldError("otp")},
		{name: "type", field: "pin", err: NewInvalidFieldTypeError("pin", "string")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err.Code == "" || test.err.Error() == "" {
				t.Fatalf("dynamic error is incomplete: %#v", test.err)
			}
			if !strings.Contains(test.err.Detail.DescEng, test.field) || !strings.Contains(test.err.Detail.DescIdn, test.field) {
				t.Fatalf("dynamic error does not identify field %q: %#v", test.field, test.err.Detail)
			}
		})
	}
}
