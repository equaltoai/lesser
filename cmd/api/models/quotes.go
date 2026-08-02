package models

// UpdateQuotePermissionsRequest represents a request to update quote permissions.
type UpdateQuotePermissionsRequest struct {
	AllowPublic    *bool    `json:"allow_public,omitempty"`
	AllowFollowers *bool    `json:"allow_followers,omitempty"`
	AllowMentioned *bool    `json:"allow_mentioned,omitempty"`
	BlockList      []string `json:"block_list,omitempty"`
}

// CreateQuotePostRequest represents POST /api/v1/statuses/{id}/quote.
type CreateQuotePostRequest struct {
	Status      string `json:"status,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	SpoilerText string `json:"spoiler_text,omitempty"`
	Sensitive   bool   `json:"sensitive"`
	Language    string `json:"language,omitempty"`
}
