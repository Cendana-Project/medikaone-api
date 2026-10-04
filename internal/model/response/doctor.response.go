package response

// PublicDoctor exposes professional identity without private account data.
type PublicDoctor struct {
	DoctorID          string                  `json:"doctor_id"`
	DoctorMedikaOneID string                  `json:"doctor_medikaone_id" gorm:"column:doctor_medikaone_id"`
	FirstName         string                  `json:"first_name"`
	LastName          string                  `json:"last_name"`
	FullName          string                  `json:"full_name"`
	SIPNumber         string                  `json:"sip_number" gorm:"column:sip_number"`
	Specialty         string                  `json:"specialty"`
	NearestPractice   *DoctorPracticeLocation `json:"nearest_practice,omitempty" gorm:"-"`
}

// DoctorPracticeLocation identifies the closest eligible practice for the
// coordinates and filters of this request. It never contains patient location.
type DoctorPracticeLocation struct {
	AffiliationID  string  `json:"affiliation_id"`
	HospitalID     string  `json:"hospital_id"`
	HospitalName   string  `json:"hospital_name"`
	DepartmentID   string  `json:"department_id"`
	DepartmentName string  `json:"department_name"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	DistanceKM     float64 `json:"distance_km"`
}

type PublicDoctorAffiliation struct {
	AffiliationID  string                   `json:"affiliation_id"`
	HospitalID     string                   `json:"hospital_id"`
	HospitalCode   string                   `json:"hospital_code"`
	HospitalName   string                   `json:"hospital_name"`
	DepartmentID   string                   `json:"department_id"`
	DepartmentName string                   `json:"department_name"`
	RoomID         *string                  `json:"room_id,omitempty"`
	RoomName       *string                  `json:"room_name,omitempty"`
	Schedules      []DoctorHospitalSchedule `json:"-" gorm:"-"`
}

type PublicDoctorDetail struct {
	PublicDoctor
	Affiliations []PublicDoctorAffiliation `json:"affiliations"`
}

type PublicDoctorPage struct {
	Items []PublicDoctor `json:"items"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
	Total int64          `json:"total"`
}
