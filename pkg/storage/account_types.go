package storage

import (
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Account represents the combined User and Actor data
type Account struct {
	User  *User              `json:"user"`
	Actor *activitypub.Actor `json:"actor,omitempty"`
}

// LoginAttempt represents a login attempt
type LoginAttempt struct {
	Username  string    `json:"username"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	IPAddress string    `json:"ip_address,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
}

// PasswordReset represents a password reset request
type PasswordReset struct {
	Username  string    `json:"username"`
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// Bookmark represents a bookmarked status
type Bookmark struct {
	Username  string    `json:"username"`
	ObjectID  string    `json:"object_id"`
	CreatedAt time.Time `json:"created_at"`
}

// AccountSuggestion represents a suggested account to follow
type AccountSuggestion struct {
	Actor           *activitypub.Actor `json:"actor"`
	Reason          string             `json:"reason"` // "popular", "friend_of_friend", etc.
	Score           float64            `json:"score"`
	MutualFollowers int                `json:"mutual_followers,omitempty"`
	FollowerCount   int                `json:"follower_count,omitempty"`
}

