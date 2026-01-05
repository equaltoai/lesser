package models

// OEmbedResponse represents the oEmbed response format.
type OEmbedResponse struct {
	Type            string `json:"type"`                       // always "rich" for statuses
	Version         string `json:"version"`                    // always "1.0"
	Title           string `json:"title,omitempty"`            // optional title
	AuthorName      string `json:"author_name"`                // account display name
	AuthorURL       string `json:"author_url"`                 // account URL
	ProviderName    string `json:"provider_name"`              // instance name
	ProviderURL     string `json:"provider_url"`               // instance URL
	CacheAge        int    `json:"cache_age"`                  // cache duration in seconds
	HTML            string `json:"html"`                       // embeddable HTML
	Width           int    `json:"width"`                      // width of embed
	Height          *int   `json:"height,omitempty"`           // height if known
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`    // thumbnail if available
	ThumbnailWidth  *int   `json:"thumbnail_width,omitempty"`  // thumbnail width
	ThumbnailHeight *int   `json:"thumbnail_height,omitempty"` // thumbnail height
}
