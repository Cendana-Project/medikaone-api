package request_test

import (
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

func TestDoctorProfileRequestSIPNumberMaximumLength(t *testing.T) {
	sixtyFourCharacters := strings.Repeat("S", 64)
	valid := request.DoctorProfileRequest{
		FirstName: "Dian",
		LastName:  "Sasmita",
		SIPNumber: &sixtyFourCharacters,
	}
	if err := util.ValidateStruct(valid); err != nil {
		t.Fatalf("64-character sip_number rejected: %v", err)
	}

	sixtyFiveCharacters := strings.Repeat("S", 65)
	invalid := request.DoctorProfileRequest{
		FirstName: "Dian",
		LastName:  "Sasmita",
		SIPNumber: &sixtyFiveCharacters,
	}
	if err := util.ValidateStruct(invalid); err == nil {
		t.Fatal("65-character sip_number must be rejected before database persistence")
	}
}
