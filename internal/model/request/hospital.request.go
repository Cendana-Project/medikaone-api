package request

type CreateHospitalRequest struct {
	Code        string   `json:"code" validate:"required,uppercase,alphanumdash,min=3,max=40"`
	Name        string   `json:"name" validate:"required,max=160"`
	Address     string   `json:"address" validate:"required,max=1000"`
	City        string   `json:"city" validate:"required,max=100"`
	Province    string   `json:"province" validate:"required,max=100"`
	Country     string   `json:"country" validate:"omitempty,max=100"`
	Latitude    *float64 `json:"latitude" validate:"omitempty"`
	Longitude   *float64 `json:"longitude" validate:"omitempty"`
	Phone       string   `json:"phone" validate:"required,max=50"`
	Description string   `json:"description" validate:"omitempty,max=200"`
	Facilities  any      `json:"facilities" validate:"omitempty"` // JSON (obj/array)
}

type CreateHospitalAdminRequest struct {
	// HospitalID is populated from the authorized tenant context, never JSON.
	HospitalID string  `json:"-" validate:"-"`
	Email      string  `json:"email"       validate:"required,email,max=190"`
	Username   string  `json:"username"    validate:"required,min=3,max=64,username"`
	Phone      *string `json:"phone"       validate:"omitempty,max=32"`
	Password   string  `json:"password"    validate:"required,max=128,validate_password"`
	FirstName  *string `json:"first_name"  validate:"omitempty,max=100"`
	LastName   *string `json:"last_name"   validate:"omitempty,max=100"`
	DOB        *string `json:"dob"         validate:"omitempty,datetime=2006-01-02"`
	Address    *string `json:"address"     validate:"omitempty,max=1000"`
	Gender     *string `json:"gender"      validate:"omitempty,oneof=L P"`
	NIK        *string `json:"nik"         validate:"omitempty,len=16,numeric"`
}

type CreateHospitalStaffRequest struct {
	// HospitalID is populated from the authorized tenant context, never JSON.
	HospitalID string  `json:"-" validate:"-"`
	Role       string  `json:"role" validate:"required,oneof_ci=DOCTOR NURSE RECEPTIONIST BOD"`
	Email      string  `json:"email"      validate:"required,email,max=190"`
	Username   string  `json:"username"   validate:"required,min=3,max=64,username"`
	Phone      *string `json:"phone"      validate:"omitempty,max=32"`
	Password   string  `json:"password"   validate:"required,max=128,validate_password"`
	FirstName  *string `json:"first_name" validate:"omitempty,max=100"`
	LastName   *string `json:"last_name"  validate:"omitempty,max=100"`
	DOB        *string `json:"dob"        validate:"omitempty,datetime=2006-01-02"`
	Address    *string `json:"address"    validate:"omitempty,max=1000"`
	Gender     *string `json:"gender"     validate:"omitempty,oneof=L P"`
	NIK        *string `json:"nik"        validate:"omitempty,len=16,numeric"`
}
