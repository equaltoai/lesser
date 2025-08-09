package models

import (
	"github.com/equaltoai/lesser/pkg/activitypub"
)

// SearchResults combines all search result types
// This is a composite response type, not stored in DynamoDB
type SearchResults struct {
	Accounts []*activitypub.Actor   `json:"accounts"`
	Statuses []*StatusSearchResult  `json:"statuses"`
	Hashtags []*HashtagSearchResult `json:"hashtags"`
}

// NewSearchResults creates a new empty search results container
func NewSearchResults() *SearchResults {
	return &SearchResults{
		Accounts: make([]*activitypub.Actor, 0),
		Statuses: make([]*StatusSearchResult, 0),
		Hashtags: make([]*HashtagSearchResult, 0),
	}
}

// AddAccount adds an account to the search results
func (s *SearchResults) AddAccount(account *activitypub.Actor) {
	if account != nil {
		s.Accounts = append(s.Accounts, account)
	}
}

// AddStatus adds a status to the search results
func (s *SearchResults) AddStatus(status *StatusSearchResult) {
	if status != nil {
		s.Statuses = append(s.Statuses, status)
	}
}

// AddHashtag adds a hashtag to the search results
func (s *SearchResults) AddHashtag(hashtag *HashtagSearchResult) {
	if hashtag != nil {
		s.Hashtags = append(s.Hashtags, hashtag)
	}
}

// IsEmpty returns true if there are no results of any type
func (s *SearchResults) IsEmpty() bool {
	return len(s.Accounts) == 0 && len(s.Statuses) == 0 && len(s.Hashtags) == 0
}

// Count returns the total number of results across all types
func (s *SearchResults) Count() int {
	return len(s.Accounts) + len(s.Statuses) + len(s.Hashtags)
}

// Merge combines another SearchResults into this one
func (s *SearchResults) Merge(other *SearchResults) {
	if other == nil {
		return
	}
	s.Accounts = append(s.Accounts, other.Accounts...)
	s.Statuses = append(s.Statuses, other.Statuses...)
	s.Hashtags = append(s.Hashtags, other.Hashtags...)
}

// TruncateToLimit ensures no category exceeds the specified limit
func (s *SearchResults) TruncateToLimit(limit int) {
	if len(s.Accounts) > limit {
		s.Accounts = s.Accounts[:limit]
	}
	if len(s.Statuses) > limit {
		s.Statuses = s.Statuses[:limit]
	}
	if len(s.Hashtags) > limit {
		s.Hashtags = s.Hashtags[:limit]
	}
}
