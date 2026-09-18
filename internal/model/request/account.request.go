package request

// The target account always comes from the authenticated session.
type DeleteAccountRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,max=128"`
}
