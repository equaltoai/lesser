package models

// EmptyObject represents an empty JSON object response.
type EmptyObject struct{}

// MessageResponse represents a generic message response.
type MessageResponse struct {
	Message string `json:"message"`
}

// SuccessResponse represents a generic success response (optionally with a message).
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}
