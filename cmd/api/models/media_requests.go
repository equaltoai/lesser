package models

// UpdateMediaRequest represents a request to update media metadata.
type UpdateMediaRequest struct {
	Description string `json:"description,omitempty"`
	Focus       string `json:"focus,omitempty"`
}
