package hospital

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
)

func TestHospitalUpdateValidatesFacilitiesAndCoordinates(t *testing.T) {
	for _, facilities := range []string{`{"parking":true}`, `["parking","laboratory"]`, `null`} {
		fields, err := hospitalUpdateFields(request.UpdateHospitalRequest{Facilities: json.RawMessage(facilities)}, time.Now())
		if err != nil || fields["facilities"] == nil {
			t.Fatalf("valid facilities rejected: %s %v", facilities, err)
		}
	}
	for _, facilities := range []string{`"parking"`, `true`, `42`, `{`} {
		if _, err := hospitalUpdateFields(request.UpdateHospitalRequest{Facilities: json.RawMessage(facilities)}, time.Now()); !errors.Is(err, constant.ErrInvalidHospitalFacilities) {
			t.Fatalf("invalid facilities accepted: %s %v", facilities, err)
		}
	}
	invalidLatitude := 91.0
	if _, err := hospitalUpdateFields(request.UpdateHospitalRequest{Latitude: &invalidLatitude}, time.Now()); !errors.Is(err, constant.ErrInvalidHospitalCoordinates) {
		t.Fatal("invalid latitude accepted")
	}
	if _, err := hospitalUpdateFields(request.UpdateHospitalRequest{}, time.Now()); err == nil {
		t.Fatal("empty patch accepted")
	}
}

func TestHospitalUpdateOmitsUnchangedFields(t *testing.T) {
	name := " New Name "
	fields, err := hospitalUpdateFields(request.UpdateHospitalRequest{Name: &name}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["name"] != "New Name" {
		t.Fatalf("partial update changed omitted values: %#v", fields)
	}
	blank := "  "
	if _, err := hospitalUpdateFields(request.UpdateHospitalRequest{Name: &blank}, time.Now()); err == nil {
		t.Fatal("blank required name accepted")
	}
}
