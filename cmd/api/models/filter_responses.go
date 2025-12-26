package models

import "github.com/equaltoai/lesser/pkg/moderation"

// FilterTestResponse represents the response from POST /api/v2/filters/test.
type FilterTestResponse struct {
	Content      string                  `json:"content"`
	TotalFilters int                     `json:"total_filters"`
	MatchedCount int                     `json:"matched_count"`
	Results      []*moderation.FilterResult `json:"results"`
}

