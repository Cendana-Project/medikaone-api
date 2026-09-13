package response

import (
	"encoding/json"
	"time"
)

// Hospital is safe for the public website and mobile directory. Internal seed
// keys, memberships, staff accounts and deletion metadata are never exposed.
type Hospital struct {
	ID          string          `json:"id"`
	Code        *string         `json:"code"`
	Name        string          `json:"name"`
	Address     *string         `json:"address"`
	City        *string         `json:"city"`
	Province    *string         `json:"province"`
	Country     *string         `json:"country"`
	Latitude    *float64        `json:"latitude"`
	Longitude   *float64        `json:"longitude"`
	Phone       *string         `json:"phone"`
	Description *string         `json:"description"`
	Facilities  json.RawMessage `json:"facilities" gorm:"type:jsonb"`
	IsActive    bool            `json:"is_active"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
