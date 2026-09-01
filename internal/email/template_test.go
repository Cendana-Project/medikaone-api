package email

import (
	"strings"
	"testing"
)

func TestRenderVerifyPINEscapesUserControlledValues(t *testing.T) {
	body := RenderVerifyPIN(`<script>alert("x")</script>`, `<123456>`, 10)
	if strings.Contains(body, "<script>") || strings.Contains(body, "<123456>") {
		t.Fatalf("email template contains unescaped user-controlled HTML: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&lt;123456&gt;") {
		t.Fatalf("email template does not contain escaped values: %s", body)
	}
}
