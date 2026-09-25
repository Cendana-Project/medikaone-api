package response

import (
	"encoding/json"
	"time"
)

// Hospital is safe for the public website and mobile directory. Internal seed
// keys, memberships, staff accounts and deletion metadata are never exposed.
type Hospital struct {
	ID              string                      `json:"id"`
	Code            *string                     `json:"code"`
	Name            string                      `json:"name"`
	Address         *string                     `json:"address"`
	City            *string                     `json:"city"`
	Province        *string                     `json:"province"`
	Country         *string                     `json:"country"`
	Latitude        *float64                    `json:"latitude"`
	Longitude       *float64                    `json:"longitude"`
	Phone           *string                     `json:"phone"`
	Description     *string                     `json:"description"`
	Facilities      json.RawMessage             `json:"facilities" gorm:"type:jsonb"`
	Email           *string                     `json:"email"`
	Website         *string                     `json:"website"`
	EstablishedYear *int                        `json:"established_year"`
	Timezone        string                      `json:"timezone"`
	OpeningHours    json.RawMessage             `json:"opening_hours" gorm:"type:jsonb"`
	IsOpen          *bool                       `json:"is_open" gorm:"-"`
	DistanceKM      *float64                    `json:"distance_km"`
	RatingAverage   *float64                    `json:"rating_average"`
	RatingCount     int64                       `json:"rating_count"`
	Departments     []HospitalDepartmentSummary `json:"departments" gorm:"-"`
	CoverImage      *HospitalImage              `json:"cover_image" gorm:"-"`
	Gallery         []HospitalImage             `json:"gallery,omitempty" gorm:"-"`
	IsActive        bool                        `json:"is_active"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

type HospitalDepartmentSummary struct {
	ID                 string  `json:"id"`
	HospitalID         string  `json:"-"`
	MasterDepartmentID *string `json:"master_department_id"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
}

type DepartmentOption struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	HospitalCount int64  `json:"hospital_count"`
	DoctorCount   int64  `json:"doctor_count"`
}
type HospitalFacility struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
}
type HospitalImage struct {
	ID          string    `json:"id"`
	HospitalID  string    `json:"hospital_id"`
	Bucket      string    `json:"-"`
	ObjectPath  string    `json:"-"`
	ContentType string    `json:"content_type"`
	FileSize    int64     `json:"file_size"`
	Caption     string    `json:"caption"`
	SortOrder   int       `json:"sort_order"`
	IsCover     bool      `json:"is_cover"`
	CreatedAt   time.Time `json:"created_at"`
	URL         string    `json:"url" gorm:"-"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"-"`
}
type HospitalReview struct {
	ID            string    `json:"id"`
	HospitalID    string    `json:"hospital_id"`
	Rating        int       `json:"rating"`
	Comment       string    `json:"comment"`
	ReviewerName  string    `json:"reviewer_name"`
	VerifiedVisit bool      `json:"verified_visit"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type HospitalReviewPage struct {
	Items              []HospitalReview `json:"items"`
	Total              int64            `json:"total"`
	Limit              int              `json:"limit"`
	Offset             int              `json:"offset"`
	RatingAverage      *float64         `json:"rating_average"`
	RatingDistribution map[string]int64 `json:"rating_distribution"`
}
