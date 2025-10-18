// Package models contains data models used by the API Lambda function.
package models

// Conversation represents a direct message conversation
type Conversation struct {
	ID         string    `json:"id"`
	Unread     bool      `json:"unread"`
	Accounts   []Account `json:"accounts"`
	LastStatus *Status   `json:"last_status,omitempty"`
}
