package request

type HospitalDirectoryQuery struct {
	Search       string   `form:"search"`
	City         string   `form:"city"`
	Department   string   `form:"department"`
	DepartmentID string   `form:"department_id"`
	Latitude     *float64 `form:"latitude"`
	Longitude    *float64 `form:"longitude"`
	RadiusKM     *float64 `form:"radius_km"`
	MinRating    *float64 `form:"min_rating"`
	Sort         string   `form:"sort"`
	Limit        int      `form:"limit"`
	Offset       int      `form:"offset"`
}

type HospitalOpeningPeriod struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}
type HospitalOpeningDay struct {
	DayOfWeek int                     `json:"day_of_week"`
	IsClosed  bool                    `json:"is_closed"`
	Is24Hours bool                    `json:"is_24_hours"`
	Periods   []HospitalOpeningPeriod `json:"periods"`
}
type UpdateHospitalImageRequest struct {
	Caption   *string `json:"caption" validate:"omitempty,max=200"`
	SortOrder *int    `json:"sort_order" validate:"omitempty,gte=0,lte=1000"`
	IsCover   *bool   `json:"is_cover"`
}
type PutHospitalReviewRequest struct {
	Rating  int    `json:"rating" validate:"required,min=1,max=5"`
	Comment string `json:"comment" validate:"max=2000"`
}
