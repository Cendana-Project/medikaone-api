package hospital

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func closedWeek() []request.HospitalOpeningDay {
	days := make([]request.HospitalOpeningDay, 7)
	for i := range days {
		days[i] = request.HospitalOpeningDay{DayOfWeek: i, IsClosed: true}
	}
	return days
}

func TestDepartmentDirectoryQueryValidation(t *testing.T) {
	query := request.DepartmentDirectoryQuery{Search: "  Mata  "}
	if err := validateDepartmentDirectoryQuery(&query); err != nil {
		t.Fatal(err)
	}
	if query.Search != "Mata" || query.Limit != 100 {
		t.Fatalf("normalized department query = %#v", query)
	}
	for _, invalid := range []request.DepartmentDirectoryQuery{
		{HospitalID: "invalid"}, {Limit: 101}, {Offset: -1}, {Search: strings.Repeat("x", 121)},
	} {
		if err := validateDepartmentDirectoryQuery(&invalid); err == nil {
			t.Fatalf("invalid department query accepted: %#v", invalid)
		}
	}
}
func TestHospitalHoursAcrossMidnightAndWeekBoundary(t *testing.T) {
	days := closedWeek()
	days[6] = request.HospitalOpeningDay{DayOfWeek: 6, Periods: []request.HospitalOpeningPeriod{{Open: "22:00", Close: "02:00"}}}
	normalized, err := normalizeOpeningHours(days)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(normalized)
	for _, tc := range []struct {
		value string
		want  bool
	}{{"2026-09-19T14:59:00Z", false}, {"2026-09-19T15:00:00Z", true}, {"2026-09-19T18:59:00Z", true}, {"2026-09-19T19:00:00Z", false}} {
		now, _ := time.Parse(time.RFC3339, tc.value)
		got := hospitalIsOpen(raw, "Asia/Jakarta", now)
		if got == nil || *got != tc.want {
			t.Fatalf("is_open %s = %v, want %v", tc.value, got, tc.want)
		}
	}
	days[0] = request.HospitalOpeningDay{DayOfWeek: 0, Periods: []request.HospitalOpeningPeriod{{Open: "01:00", Close: "03:00"}}}
	if _, err = normalizeOpeningHours(days); err == nil {
		t.Fatal("week-boundary overlap accepted")
	}
	if hospitalIsOpen(json.RawMessage("[]"), "Asia/Jakarta", time.Now()) != nil {
		t.Fatal("unknown hours reported as closed")
	}
	days = closedWeek()
	days[1] = request.HospitalOpeningDay{DayOfWeek: 1, Is24Hours: true}
	normalized, err = normalizeOpeningHours(days)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(normalized)
	now, _ := time.Parse(time.RFC3339, "2026-09-21T05:00:00Z")
	if got := hospitalIsOpen(raw, "Asia/Jakarta", now); got == nil || !*got {
		t.Fatal("24-hour hospital reported closed")
	}
}
func TestHospitalHoursRejectInvalidDefinitions(t *testing.T) {
	for _, mutate := range []func([]request.HospitalOpeningDay) []request.HospitalOpeningDay{
		func(d []request.HospitalOpeningDay) []request.HospitalOpeningDay { return d[:1] },
		func(d []request.HospitalOpeningDay) []request.HospitalOpeningDay { d[6].DayOfWeek = 0; return d },
		func(d []request.HospitalOpeningDay) []request.HospitalOpeningDay { d[0].Is24Hours = true; return d },
		func(d []request.HospitalOpeningDay) []request.HospitalOpeningDay {
			d[0].IsClosed = false
			d[0].Periods = []request.HospitalOpeningPeriod{{Open: "08:00", Close: "08:00"}}
			return d
		},
		func(d []request.HospitalOpeningDay) []request.HospitalOpeningDay {
			d[0].IsClosed = false
			d[0].Periods = []request.HospitalOpeningPeriod{{Open: "25:00", Close: "08:00"}}
			return d
		},
	} {
		if _, err := normalizeOpeningHours(mutate(closedWeek())); err == nil {
			t.Fatal("invalid hours accepted")
		}
	}
}
func TestHospitalDirectoryQueryValidation(t *testing.T) {
	lat, lng, nan, radius, rating := -6.2, 106.8, math.NaN(), 5.0, 4.5
	valid := request.HospitalDirectoryQuery{Limit: 20, Latitude: &lat, Longitude: &lng, RadiusKM: &radius, MinRating: &rating, Sort: "distance"}
	if err := validateDirectoryQuery(&valid); err != nil {
		t.Fatal(err)
	}
	for _, q := range []request.HospitalDirectoryQuery{{Limit: 20, Latitude: &lat}, {Limit: 20, Sort: "distance"}, {Limit: 20, RadiusKM: &radius}, {Limit: 20, Latitude: &nan, Longitude: &lng}, {Limit: 20, MinRating: &nan}, {Limit: 20, Sort: "DROP"}, {Limit: 101}, {Limit: 20, Offset: -1}} {
		if err := validateDirectoryQuery(&q); err == nil {
			t.Fatalf("invalid query accepted: %#v", q)
		}
	}
}
func TestHospitalFacilitiesCanonicalAndLegacy(t *testing.T) {
	for _, raw := range []string{`["Parkir Mobil", "wifi"]`, `{"parking":true,"inactive":false,"beds":120}`, `[{"code":"parking","name":"Parkir","icon":"car"}]`, `null`} {
		out, err := normalizeFacilities(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		var entries []response.HospitalFacility
		if err = json.Unmarshal(out, &entries); err != nil || entries == nil {
			t.Fatalf("not canonical array: %s", out)
		}
	}
	for _, raw := range []string{`[{"code":"parking"}]`, `["wifi","wifi"]`, `[{"code":"<html>","name":"invalid"}]`, `{"object":{"x":true}}`} {
		if _, err := normalizeFacilities(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid facilities: %s", raw)
		}
	}
}
func TestHospitalMetadataValidation(t *testing.T) {
	badEmail, badWebsite, zone := "Person <person@example.test>", "javascript:alert(1)", "Mars/City"
	future := time.Now().Year() + 1
	for _, req := range []request.UpdateHospitalRequest{{Email: &badEmail}, {Website: &badWebsite}, {Timezone: &zone}, {EstablishedYear: &future}} {
		if _, err := directoryUpdateFields(req, time.Now()); err == nil {
			t.Fatal("invalid hospital metadata accepted")
		}
	}
	blank := ""
	zero := 0
	fields, err := directoryUpdateFields(request.UpdateHospitalRequest{Email: &blank, Website: &blank, EstablishedYear: &zero}, time.Now())
	if err != nil || fields["established_year"] != nil {
		t.Fatal("cannot clear optional values", err)
	}
}
func TestHospitalImageValidationChecksDecodedContent(t *testing.T) {
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	mime, ext, err := validateHospitalImage(out.Bytes(), 1024)
	if err != nil || mime != "image/png" || ext != ".png" {
		t.Fatal(mime, ext, err)
	}
	for _, content := range [][]byte{[]byte("<svg></svg>"), out.Bytes()[:20], []byte(strings.Repeat("x", 2048))} {
		if _, _, err := validateHospitalImage(content, 1024); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
}
