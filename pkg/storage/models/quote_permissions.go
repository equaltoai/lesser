package models

import (
	"fmt"
)

// QuotePermissions represents quote permissions for a user's statuses
type QuotePermissions struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// Business fields
	Username       string   `json:"username"`
	AllowPublic    bool     `json:"allow_public"`
	AllowFollowers bool     `json:"allow_followers"`
	AllowMentioned bool     `json:"allow_mentioned"`
	BlockList      []string `json:"block_list"` // List of usernames blocked from quoting
}

// UpdateKeys updates the composite keys based on the quote permissions
func (q *QuotePermissions) UpdateKeys() error {
	// Primary key: USER#username
	q.PK = fmt.Sprintf(KeyPatternUser, q.Username)
	q.SK = "QUOTE_PERMISSIONS"
	return nil
}

// GetPK returns the partition key
func (q *QuotePermissions) GetPK() string {
	return q.PK
}

// GetSK returns the sort key
func (q *QuotePermissions) GetSK() string {
	return q.SK
}

// IsAllowed checks if a given user is allowed to quote based on permissions
func (q *QuotePermissions) IsAllowed(quoterUsername string, isFollower bool, isMentioned bool) bool {
	// Check if user is in block list
	for _, blocked := range q.BlockList {
		if blocked == quoterUsername {
			return false
		}
	}

	// Check permission levels in order of most permissive to least
	if q.AllowPublic {
		return true
	}

	if q.AllowFollowers && isFollower {
		return true
	}

	if q.AllowMentioned && isMentioned {
		return true
	}

	return false
}

// AddToBlockList adds a username to the block list
func (q *QuotePermissions) AddToBlockList(username string) {
	// Check if already in list
	for _, blocked := range q.BlockList {
		if blocked == username {
			return
		}
	}
	q.BlockList = append(q.BlockList, username)
}

// RemoveFromBlockList removes a username from the block list
func (q *QuotePermissions) RemoveFromBlockList(username string) {
	newList := make([]string, 0, len(q.BlockList))
	for _, blocked := range q.BlockList {
		if blocked != username {
			newList = append(newList, blocked)
		}
	}
	q.BlockList = newList
}

// SetDefaults sets default quote permissions for a new user
func (q *QuotePermissions) SetDefaults() {
	q.AllowPublic = true
	q.AllowFollowers = true
	q.AllowMentioned = true
	q.BlockList = []string{}
}
