package models

// FilterKeywordAttribute represents a keyword entry in the filter create/update request.
type FilterKeywordAttribute struct {
	Keyword      string   `json:"keyword"`
	WholeWord    bool     `json:"whole_word"`
	IsRegex      *bool    `json:"is_regex,omitempty"`
	MatchWeight  *float64 `json:"match_weight,omitempty"`
	ContextTypes []string `json:"context_types,omitempty"`
}

// CreateFilterRequest represents POST /api/v2/filters.
type CreateFilterRequest struct {
	Title              string                   `json:"title"`
	Context            []string                 `json:"context"`
	FilterAction       string                   `json:"filter_action"`
	Severity           string                   `json:"severity,omitempty"`
	MatchMode          string                   `json:"match_mode,omitempty"`
	CaseSensitive      bool                     `json:"case_sensitive"`
	ExpiresIn          *int                     `json:"expires_in,omitempty"`
	KeywordsAttributes []FilterKeywordAttribute `json:"keywords_attributes,omitempty"`
}

// AddFilterKeywordRequest represents POST /api/v2/filters/{filter_id}/keywords.
type AddFilterKeywordRequest struct {
	Keyword   string `json:"keyword"`
	WholeWord bool   `json:"whole_word"`
}

// AddFilterStatusRequest represents POST /api/v2/filters/{filter_id}/statuses.
type AddFilterStatusRequest struct {
	StatusID string `json:"status_id"`
}

// TestFilterRequest represents POST /api/v2/filters/test.
type TestFilterRequest struct {
	Content string   `json:"content"`
	Context []string `json:"context"`
}
