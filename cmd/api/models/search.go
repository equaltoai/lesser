package models

// SearchSuggestion represents a search suggestion result.
type SearchSuggestion struct {
	Type  string  `json:"type"`
	Value string  `json:"value"`
	Score float64 `json:"score"`
}

// StatusSearchResult represents a status search result entry returned by /api/v1/search/statuses.
type StatusSearchResult struct {
	ID              string   `json:"id"`
	Content         string   `json:"content"`
	URL             string   `json:"url"`
	AccountID       string   `json:"account_id"`
	AccountUsername string   `json:"account_username"`
	CreatedAt       string   `json:"created_at"`
	Score           float64  `json:"score"`
	Highlights      []string `json:"highlights,omitempty"`
}
