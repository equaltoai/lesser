package mastodon

import (
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/storage"
)

// Filter represents a filter group in the Mastodon API v2
type Filter struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Context      []string        `json:"context"`
	ExpiresAt    *string         `json:"expires_at"`
	FilterAction string          `json:"filter_action"`
	Keywords     []FilterKeyword `json:"keywords,omitempty"`
	Statuses     []FilterStatus  `json:"statuses,omitempty"`
}

// FilterKeyword represents a keyword within a filter
type FilterKeyword struct {
	ID        string `json:"id"`
	Keyword   string `json:"keyword"`
	WholeWord bool   `json:"whole_word"`
}

// FilterStatus represents a status within a filter
type FilterStatus struct {
	ID       string `json:"id"`
	StatusID string `json:"status_id"`
}

// FilterResult represents a filter match on a status
type FilterResult struct {
	Filter         Filter   `json:"filter"`
	KeywordMatches []string `json:"keyword_matches,omitempty"`
	StatusMatches  []string `json:"status_matches,omitempty"`
}

// V1Filter represents a filter in the Mastodon API v1 (deprecated)
type V1Filter struct {
	ID           string   `json:"id"`
	Phrase       string   `json:"phrase"`
	Context      []string `json:"context"`
	ExpiresAt    *string  `json:"expires_at"`
	Irreversible bool     `json:"irreversible"`
	WholeWord    bool     `json:"whole_word"`
}

// ConvertFilterToMastodon converts a storage filter to Mastodon API format
func (c *converterImpl) ConvertFilterToMastodon(filter *storage.Filter, keywords []*storage.FilterKeyword, statuses []*storage.FilterStatus) *Filter {
	result := &Filter{
		ID:           filter.ID,
		Title:        filter.Title,
		Context:      filter.Context,
		FilterAction: filter.FilterAction,
		Keywords:     make([]FilterKeyword, 0, len(keywords)),
		Statuses:     make([]FilterStatus, 0, len(statuses)),
	}

	// Convert expires_at
	if filter.ExpiresAt != nil && !filter.ExpiresAt.IsZero() {
		expiresAt := filter.ExpiresAt.Format(time.RFC3339)
		result.ExpiresAt = &expiresAt
	}

	// Convert keywords
	for _, keyword := range keywords {
		result.Keywords = append(result.Keywords, FilterKeyword{
			ID:        keyword.ID,
			Keyword:   keyword.Keyword,
			WholeWord: keyword.WholeWord,
		})
	}

	// Convert statuses
	for _, status := range statuses {
		result.Statuses = append(result.Statuses, FilterStatus{
			ID:       status.ID,
			StatusID: status.StatusID,
		})
	}

	return result
}

// ConvertFilterKeywordToV1 converts a filter keyword to v1 filter format for compatibility
func (c *converterImpl) ConvertFilterKeywordToV1(keyword *storage.FilterKeyword, filter *storage.Filter) *V1Filter {
	result := &V1Filter{
		ID:           keyword.ID,
		Phrase:       keyword.Keyword,
		Context:      filter.Context,
		Irreversible: filter.FilterAction == "hide",
		WholeWord:    keyword.WholeWord,
	}

	// Convert expires_at
	if filter.ExpiresAt != nil && !filter.ExpiresAt.IsZero() {
		expiresAt := filter.ExpiresAt.Format(time.RFC3339)
		result.ExpiresAt = &expiresAt
	}

	return result
}

// ConvertMuteToRelationship adds mute information to a relationship
func (c *converterImpl) ConvertMuteToRelationship(relationship *models.Relationship, mute *storage.Mute) {
	if mute != nil {
		relationship.Muting = true
		relationship.MutingNotifications = mute.HideNotifications
	}
}
