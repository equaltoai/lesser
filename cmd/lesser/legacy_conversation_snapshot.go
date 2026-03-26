package main

import "time"

// legacyConversationSnapshot preserves the old participant-row payload shape for
// migrations and tests that still need to read or rebuild legacy snapshots.
type legacyConversationSnapshot struct {
	ID                string
	Participants      []string
	LastStatusID      string
	Unread            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	TotalMessageCount int64
	LastMessageTime   time.Time
}
