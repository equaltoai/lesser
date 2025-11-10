package models

import (
	"fmt"
	"time"
)

// Follow represents a follow relationship between actors
type Follow struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - follower's perspective
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "follow#{follower_username}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "following#{followed_username}"

	// GSI1 - followed's perspective (for listing followers)
	GSI1PK string `dynamorm:"index:gsi1-index,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "follow#{followed_username}"
	GSI1SK string `dynamorm:"index:gsi1-index,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "follower#{follower_username}"

	// GSI2 - by state and timestamp (for pending follows)
	GSI2PK string `dynamorm:"index:gsi2-index,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "follow#state#{state}"
	GSI2SK string `dynamorm:"index:gsi2-index,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{timestamp}#{follower}#{followed}"

	// Relationship data
	FollowerUsername string     `dynamorm:"attr:followerUsername" json:"follower_username"`
	FollowedUsername string     `dynamorm:"attr:followedUsername" json:"followed_username"`
	ActivityID       string     `dynamorm:"attr:activityID" json:"activity_id"` // The Follow activity ID
	State            string     `dynamorm:"attr:state" json:"state"`            // "pending", "accepted", "rejected"
	CreatedAt        time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt        time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	AcceptedAt       *time.Time `dynamorm:"attr:acceptedAt" json:"accepted_at,omitempty"`
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
