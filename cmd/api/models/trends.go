package models

// Trend represents a mixed trend item returned by GET /api/v1/trends and /api/v2/trends.
type Trend struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// TrendingStatusSummary represents a lightweight trending status entry.
type TrendingStatusSummary struct {
	ID        string                `json:"id"`
	URL       string                `json:"url"`
	Account   TrendingStatusAccount `json:"account"`
	Content   string                `json:"content"`
	CreatedAt string                `json:"created_at"`
}

// TrendingStatusAccount represents the account object embedded in trending status summaries.
type TrendingStatusAccount struct {
	ID string `json:"id"`
}

// PreviewCard represents a Mastodon-style link preview card.
type PreviewCard struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	AuthorName   string `json:"author_name"`
	AuthorURL    string `json:"author_url"`
	ProviderName string `json:"provider_name"`
	ProviderURL  string `json:"provider_url"`
	HTML         string `json:"html"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Image        string `json:"image"`
	EmbedURL     string `json:"embed_url"`
	Blurhash     string `json:"blurhash"`
}

// LinkTimelineEntry represents the minimal timeline entry returned by GET /api/v1/timelines/link.
type LinkTimelineEntry struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	URL     string `json:"url"`
}
