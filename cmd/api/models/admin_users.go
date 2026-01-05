package models

// AdminCreateUserRequest defines the request body for creating a new user.
type AdminCreateUserRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`        // Ignored - email is disabled
	Password    string `json:"password,omitempty"`     // Ignored - passwordless auth only
	DisplayName string `json:"display_name,omitempty"` // Optional
	Role        string `json:"role,omitempty"`         // Optional (defaults to "user")
}
