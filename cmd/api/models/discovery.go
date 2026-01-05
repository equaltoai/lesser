package models

// SuggestionV1 represents a v1 suggestion item returned by GET /api/v1/suggestions.
type SuggestionV1 struct {
	Account Account `json:"account"`
}

// SuggestionV2 represents a v2 suggestion item returned by GET /api/v2/suggestions.
type SuggestionV2 struct {
	Source  string  `json:"source"`
	Account Account `json:"account"`
}
