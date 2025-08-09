package models

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// SearchHistoryEntry represents a user's search history entry
// Stored in DynamoDB with pattern:
// PK: USER#username
// SK: SEARCH_HISTORY#{timestamp}#{queryHash}
type SearchHistoryEntry struct {
	PK          string    `dynamorm:"pk" json:"-"`
	SK          string    `dynamorm:"sk" json:"-"`
	UserID      string    `json:"user_id"`
	Query       string    `json:"query"`
	ResultCount int       `json:"result_count"`
	ClickedIDs  []string  `json:"clicked_ids"` // IDs of results user clicked
	SearchedAt  time.Time `json:"searched_at"`
}

// UpdateKeys updates the DynamoDB keys based on the entry data
func (s *SearchHistoryEntry) UpdateKeys() {
	if s.UserID != "" {
		s.PK = fmt.Sprintf(KeyPatternUser, s.UserID)
	}
	if !s.SearchedAt.IsZero() && s.Query != "" {
		queryHash := hashQuery(s.Query)
		s.SK = fmt.Sprintf("SEARCH_HISTORY#%d#%s", s.SearchedAt.Unix(), queryHash)
	}
}

// hashQuery creates a short hash of the query for the sort key
func hashQuery(query string) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("%x", h[:8]) // Use first 8 bytes of hash
}

// NewSearchHistoryEntry creates a new search history entry
func NewSearchHistoryEntry(userID, query string, resultCount int) *SearchHistoryEntry {
	entry := &SearchHistoryEntry{
		UserID:      userID,
		Query:       query,
		ResultCount: resultCount,
		ClickedIDs:  make([]string, 0),
		SearchedAt:  time.Now(),
	}
	entry.UpdateKeys()
	return entry
}

// AddClickedID adds a clicked result ID to the history
func (s *SearchHistoryEntry) AddClickedID(id string) {
	if id != "" && !s.hasClickedID(id) {
		s.ClickedIDs = append(s.ClickedIDs, id)
	}
}

// hasClickedID checks if an ID has already been clicked
func (s *SearchHistoryEntry) hasClickedID(id string) bool {
	for _, clickedID := range s.ClickedIDs {
		if clickedID == id {
			return true
		}
	}
	return false
}

// GetClickRate returns the click-through rate for this search
func (s *SearchHistoryEntry) GetClickRate() float64 {
	if s.ResultCount == 0 {
		return 0
	}
	return float64(len(s.ClickedIDs)) / float64(s.ResultCount)
}

// IsRecent returns true if the search was performed within the specified duration
func (s *SearchHistoryEntry) IsRecent(duration time.Duration) bool {
	return time.Since(s.SearchedAt) < duration
}
