package request

// SpecialtyQuery lists active local clinical fields. Code and search are AND filters.
type SpecialtyQuery struct {
	Search   string
	Code     string
	Category string
	Limit    int
	Offset   int
}
