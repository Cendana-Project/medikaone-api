package email

import (
	"strings"
	"testing"
	"time"
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

func TestRenderDoctorHospitalInvitationDescribesNoBodyResponse(t *testing.T) {
	body := RenderDoctorHospitalInvitation("Dian", "RS MedikaOne", "Penyakit Dalam", time.Now().Add(24*time.Hour))
	if strings.Contains(body, "mengunggah kontrak") {
		t.Fatalf("invitation email still asks the doctor to upload a contract: %s", body)
	}
	if !strings.Contains(body, "tidak perlu mengunggah dokumen") {
		t.Fatalf("invitation email does not explain the no-upload response: %s", body)
	}
}
