package util

import (
	"net/url"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

// ValidateDiscoveryQuery prevents empty/repeated new filters from silently
// broadening discovery (Gin otherwise turns an empty boolean into false).
func ValidateDiscoveryQuery(values url.Values) error {
	for _, name := range []string{"gender", "min_experience_years", "max_experience_years", "available_on", "available_from", "available_to", "booking_mode", "only_available", "open_now"} {
		if entries, present := values[name]; present {
			if len(entries) != 1 || strings.TrimSpace(entries[0]) == "" {
				return constant.NewInvalidFieldValueError(name, "one nonempty value", "satu nilai yang tidak kosong")
			}
			if (name == "only_available" || name == "open_now") && entries[0] != "true" && entries[0] != "false" {
				return constant.NewInvalidFieldValueError(name, "true or false", "true atau false")
			}
		}
	}
	return nil
}
