package hospital

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/google/uuid"
)

func invalidDirectory(field, requirement string) error {
	return constant.NewInvalidFieldValueError(field, requirement, requirement)
}

func validateDirectoryQuery(q *request.HospitalDirectoryQuery) error {
	q.Search, q.City, q.Department = strings.TrimSpace(q.Search), strings.TrimSpace(q.City), strings.TrimSpace(q.Department)
	q.DepartmentCode = strings.ToUpper(strings.TrimSpace(q.DepartmentCode))
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 100000 {
		return invalidDirectory("pagination", "limit 1-100; offset 0-100000")
	}
	if utf8.RuneCountInString(q.Search) > 160 || utf8.RuneCountInString(q.City) > 100 || utf8.RuneCountInString(q.Department) > 120 || utf8.RuneCountInString(q.DepartmentCode) > 40 {
		return invalidDirectory("search", "search <=160; city <=100; department <=120; department_code <=40 characters")
	}
	if q.DepartmentID != "" {
		if _, err := uuid.Parse(q.DepartmentID); err != nil {
			return constant.ErrInvalidUUIDFormat
		}
	}
	if (q.Latitude == nil) != (q.Longitude == nil) {
		return invalidDirectory("coordinates", "latitude and longitude must be supplied together")
	}
	if q.Latitude != nil && (!finite(*q.Latitude) || !finite(*q.Longitude) || *q.Latitude < -90 || *q.Latitude > 90 || *q.Longitude < -180 || *q.Longitude > 180) {
		return constant.ErrInvalidHospitalCoordinates
	}
	if q.MinRating != nil && (!finite(*q.MinRating) || *q.MinRating < 0 || *q.MinRating > 5) {
		return invalidDirectory("min_rating", "0-5")
	}
	if q.RadiusKM != nil && (!finite(*q.RadiusKM) || *q.RadiusKM <= 0 || *q.RadiusKM > 5000 || q.Latitude == nil) {
		return invalidDirectory("radius_km", "greater than 0 through 5000, with latitude and longitude")
	}
	q.Sort = strings.ToLower(strings.TrimSpace(q.Sort))
	if q.Sort == "" {
		q.Sort = "name"
	}
	if q.Sort != "name" && q.Sort != "rating" && q.Sort != "distance" {
		return invalidDirectory("sort", "name, rating, or distance")
	}
	if q.Sort == "distance" && q.Latitude == nil {
		return invalidDirectory("sort", "distance requires latitude and longitude")
	}
	return nil
}

func validateDepartmentDirectoryQuery(q *request.DepartmentDirectoryQuery) error {
	q.Search = strings.TrimSpace(q.Search)
	q.HospitalID = strings.TrimSpace(q.HospitalID)
	if q.Limit == 0 {
		q.Limit = 100
	}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 100000 {
		return invalidDirectory("pagination", "limit 1-100; offset 0-100000")
	}
	if utf8.RuneCountInString(q.Search) > 120 {
		return invalidDirectory("q", "at most 120 characters")
	}
	if q.HospitalID != "" {
		id, err := uuid.Parse(q.HospitalID)
		if err != nil {
			return constant.ErrInvalidUUIDFormat
		}
		q.HospitalID = id.String()
	}
	return nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

var facilityCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Accept legacy arrays of labels and boolean maps, but always emit typed entries.
func normalizeFacilities(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, constant.ErrInvalidHospitalFacilities
		}
	}
	items := make([]response.HospitalFacility, 0)
	label := func(name string) response.HospitalFacility {
		name = strings.TrimSpace(name)
		code := strings.ToLower(name)
		if !facilityCodePattern.MatchString(code) {
			sum := sha256.Sum256([]byte(name))
			code = fmt.Sprintf("facility_%x", sum[:8])
		}
		return response.HospitalFacility{Code: code, Name: name}
	}
	switch v := value.(type) {
	case nil:
	case []any:
		for _, entry := range v {
			switch item := entry.(type) {
			case string:
				items = append(items, label(item))
			case map[string]any:
				code, ok1 := item["code"].(string)
				name, ok2 := item["name"].(string)
				if !ok1 || !ok2 {
					return nil, constant.ErrInvalidHospitalFacilities
				}
				icon := ""
				if field, ok := item["icon"]; ok {
					var valid bool
					icon, valid = field.(string)
					if !valid {
						return nil, constant.ErrInvalidHospitalFacilities
					}
				}
				for key := range item {
					if key != "code" && key != "name" && key != "icon" {
						return nil, constant.ErrInvalidHospitalFacilities
					}
				}
				items = append(items, response.HospitalFacility{Code: strings.TrimSpace(code), Name: strings.TrimSpace(name), Icon: strings.TrimSpace(icon)})
			default:
				return nil, constant.ErrInvalidHospitalFacilities
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			switch item := v[key].(type) {
			case bool:
				if item {
					items = append(items, label(key))
				}
			case string:
				entry := label(item)
				if facilityCodePattern.MatchString(key) {
					entry.Code = key
				}
				items = append(items, entry)
			case float64:
				entry := label(fmt.Sprintf("%s: %g", key, item))
				if facilityCodePattern.MatchString(key) {
					entry.Code = key
				}
				items = append(items, entry)
			case nil:
			default:
				return nil, constant.ErrInvalidHospitalFacilities
			}
		}
	default:
		return nil, constant.ErrInvalidHospitalFacilities
	}
	if len(items) > 50 {
		return nil, constant.ErrInvalidHospitalFacilities
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !facilityCodePattern.MatchString(item.Code) || item.Name == "" || utf8.RuneCountInString(item.Name) > 120 || seen[item.Code] || (item.Icon != "" && !facilityCodePattern.MatchString(item.Icon)) {
			return nil, constant.ErrInvalidHospitalFacilities
		}
		seen[item.Code] = true
	}
	out, err := json.Marshal(items)
	return out, err
}

func directoryUpdateFields(req request.UpdateHospitalRequest, now time.Time) (map[string]any, error) {
	fields := map[string]any{}
	if req.Email != nil {
		value := strings.TrimSpace(*req.Email)
		if value != "" {
			address, err := mail.ParseAddress(value)
			if err != nil || address.Address != value || len(value) > 190 {
				return nil, invalidDirectory("email", "valid email, or empty to clear")
			}
		}
		fields["email"] = sp(value)
	}
	if req.Website != nil {
		value := strings.TrimSpace(*req.Website)
		if value != "" {
			u, err := url.Parse(value)
			if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || len(value) > 2048 {
				return nil, invalidDirectory("website", "HTTP(S) URL, or empty to clear")
			}
		}
		fields["website"] = sp(value)
	}
	if req.EstablishedYear != nil {
		if *req.EstablishedYear == 0 {
			fields["established_year"] = nil
		} else if *req.EstablishedYear < 1800 || *req.EstablishedYear > now.Year() {
			return nil, invalidDirectory("established_year", "1800 through current year; 0 clears value")
		} else {
			fields["established_year"] = *req.EstablishedYear
		}
	}
	if req.Timezone != nil {
		zone := strings.TrimSpace(*req.Timezone)
		if zone == "" || zone == "Local" || len(zone) > 64 {
			return nil, invalidDirectory("timezone", "IANA timezone")
		}
		if _, err := time.LoadLocation(zone); err != nil {
			return nil, invalidDirectory("timezone", "IANA timezone")
		}
		fields["timezone"] = zone
	}
	if req.OpeningHours != nil {
		hours, err := normalizeOpeningHours(*req.OpeningHours)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(hours)
		if err != nil {
			return nil, err
		}
		fields["opening_hours"] = string(raw)
	}
	return fields, nil
}

func normalizeOpeningHours(input []request.HospitalOpeningDay) ([]request.HospitalOpeningDay, error) {
	result := make([]request.HospitalOpeningDay, len(input))
	copy(result, input)
	if len(result) != 0 && len(result) != 7 {
		return nil, invalidDirectory("opening_hours", "[] or all 7 unique weekdays (0 Sunday through 6 Saturday)")
	}
	seen := map[int]bool{}
	type interval struct{ start, end int }
	var intervals []interval
	for i := range result {
		day := &result[i]
		if day.DayOfWeek < 0 || day.DayOfWeek > 6 || seen[day.DayOfWeek] {
			return nil, invalidDirectory("opening_hours.day_of_week", "all 7 unique weekdays 0-6")
		}
		seen[day.DayOfWeek] = true
		if day.IsClosed && day.Is24Hours || (day.IsClosed || day.Is24Hours) && len(day.Periods) > 0 || !day.IsClosed && !day.Is24Hours && (len(day.Periods) == 0 || len(day.Periods) > 4) {
			return nil, invalidDirectory("opening_hours", "closed or 24 hours without periods; otherwise 1-4 periods")
		}
		if day.Periods == nil {
			day.Periods = []request.HospitalOpeningPeriod{}
		}
		if day.Is24Hours {
			intervals = append(intervals, interval{day.DayOfWeek * 1440, (day.DayOfWeek + 1) * 1440})
		}
		for _, period := range day.Periods {
			start, err1 := minuteOfDay(period.Open)
			end, err2 := minuteOfDay(period.Close)
			if err1 != nil || err2 != nil || start == end {
				return nil, invalidDirectory("opening_hours.periods", "HH:mm open and close; equal times require is_24_hours")
			}
			if end < start {
				end += 1440
			}
			intervals = append(intervals, interval{day.DayOfWeek*1440 + start, day.DayOfWeek*1440 + end})
		}
	}
	// Compare across the week boundary as well as between adjacent days.
	for i, a := range intervals {
		for j, b := range intervals {
			if i == j {
				continue
			}
			for _, shift := range []int{-10080, 0, 10080} {
				if a.start < b.end+shift && b.start+shift < a.end {
					return nil, invalidDirectory("opening_hours", "periods must not overlap, including overnight periods")
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DayOfWeek < result[j].DayOfWeek })
	return result, nil
}
func minuteOfDay(value string) (int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil || len(value) != 5 {
		return 0, fmt.Errorf("invalid time")
	}
	return t.Hour()*60 + t.Minute(), nil
}

func hospitalIsOpen(raw json.RawMessage, zone string, now time.Time) *bool {
	var days []request.HospitalOpeningDay
	if json.Unmarshal(raw, &days) != nil || len(days) == 0 {
		return nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil
	}
	local := now.In(location)
	weekday := int(local.Weekday())
	minute := local.Hour()*60 + local.Minute()
	open := false
	for _, day := range days {
		if day.IsClosed {
			continue
		}
		if day.Is24Hours && day.DayOfWeek == weekday {
			open = true
			break
		}
		for _, period := range day.Periods {
			start, e1 := minuteOfDay(period.Open)
			end, e2 := minuteOfDay(period.Close)
			if e1 != nil || e2 != nil {
				continue
			}
			if end > start && day.DayOfWeek == weekday && minute >= start && minute < end || end < start && (day.DayOfWeek == weekday && minute >= start || (day.DayOfWeek+1)%7 == weekday && minute < end) {
				open = true
			}
		}
	}
	return &open
}
