package models

// QuotePermissionsResponse represents quote permissions for an account.
type QuotePermissionsResponse struct {
	AllowPublic    bool     `json:"allow_public"`
	AllowFollowers bool     `json:"allow_followers"`
	AllowMentioned bool     `json:"allow_mentioned"`
	BlockList      []string `json:"block_list"`
}

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

// QuoteStatusAccount represents the minimal account payload used by quote endpoints.
type QuoteStatusAccount struct {
	ID       string  `json:"id"`
	Username *string `json:"username,omitempty"`
}

// QuoteStatusSummary represents the minimal status payload used by quote endpoints.
type QuoteStatusSummary struct {
	ID        string           `json:"id"`
	CreatedAt string           `json:"created_at"`
	Account   QuoteStatusAccount `json:"account"`
	Content   string           `json:"content"`
}

