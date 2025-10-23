package models

import (
	"fmt"
	"time"
)

// Follow represents a follow relationship between actors
type Follow struct {
	// Primary key - follower's perspective
	PK string `dynamorm:"pk" json:"pk"` // Format: "follow#{follower_username}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "following#{followed_username}"

	// GSI1 - followed's perspective (for listing followers)
	GSI1PK string `dynamorm:"index:gsi1-index,pk" json:"gsi1_pk"` // Format: "follow#{followed_username}"
	GSI1SK string `dynamorm:"index:gsi1-index,sk" json:"gsi1_sk"` // Format: "follower#{follower_username}"

	// GSI2 - by state and timestamp (for pending follows)
	GSI2PK string `dynamorm:"index:gsi2-index,pk" json:"gsi2_pk"` // Format: "follow#state#{state}"
	GSI2SK string `dynamorm:"index:gsi2-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{follower}#{followed}"

	// Relationship data
	FollowerUsername string     `json:"follower_username"`
	FollowedUsername string     `json:"followed_username"`
	ActivityID       string     `json:"activity_id"` // The Follow activity ID
	State            string     `json:"state"`       // "pending", "accepted", "rejected"
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	AcceptedAt       *time.Time `json:"accepted_at,omitempty"`
}

// Follow state constants
const (
	FollowStatePending  = "pending"
	FollowStateAccepted = "accepted"
	FollowStateRejected = "rejected"
)

// NewFollow creates a new follow relationship
func NewFollow(followerUsername, followedUsername, activityID string) *Follow {
	now := time.Now()
	follow := &Follow{
		PK:               fmt.Sprintf("follow#%s", followerUsername),
		SK:               fmt.Sprintf("following#%s", followedUsername),
		GSI1PK:           fmt.Sprintf("follow#%s", followedUsername),
		GSI1SK:           fmt.Sprintf("follower#%s", followerUsername),
		FollowerUsername: followerUsername,
		FollowedUsername: followedUsername,
		ActivityID:       activityID,
		State:            FollowStatePending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	follow.updateGSI2()
	return follow
}

// Accept marks the follow as accepted
func (f *Follow) Accept() {
	now := time.Now()
	f.State = FollowStateAccepted
	f.AcceptedAt = &now
	f.UpdatedAt = now
	f.updateGSI2()
}

// Reject marks the follow as rejected
func (f *Follow) Reject() {
	f.State = FollowStateRejected
	f.UpdatedAt = time.Now()
	f.updateGSI2()
}

// updateGSI2 updates the GSI2 keys based on current state
func (f *Follow) updateGSI2() {
	f.GSI2PK = fmt.Sprintf("follow#state#%s", f.State)
	f.GSI2SK = fmt.Sprintf("%s#%s#%s",
		f.CreatedAt.Format(time.RFC3339),
		f.FollowerUsername,
		f.FollowedUsername,
	)
}

// TableName returns the DynamoDB table backing Follow.
func (Follow) TableName() string {
	return MainTableName
}
