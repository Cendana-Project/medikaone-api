// Package bookingpolicy defines the booking window shared by discovery and booking.
package bookingpolicy

import "time"

const (
	Horizon         = 90 * 24 * time.Hour
	MinimumLeadTime = 2 * time.Hour
)
