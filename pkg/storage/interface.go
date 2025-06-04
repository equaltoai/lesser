package storage

import (
	"context"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
)

// Storage defines the interface for data storage operations
type Storage interface {
	// Actor operations
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	DeleteActor(ctx context.Context, username string) error

	// Activity operations
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
	GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// Object operations
	CreateObject(ctx context.Context, object interface{}) error
	GetObject(ctx context.Context, id string) (interface{}, error)
	UpdateObject(ctx context.Context, object interface{}) error
	DeleteObject(ctx context.Context, id string) error

	// Relationship operations
	CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error
	AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error
	RejectFollow(ctx context.Context, followerUsername, followedUsername string) error
	RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error)

	// Collection operations
	GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error)
}

// ActorRecord represents an actor stored in DynamoDB
type ActorRecord struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	Actor      *activitypub.Actor
	PrivateKey string    `dynamodbav:"PrivateKey,omitempty"`
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// ActivityRecord represents an activity stored in DynamoDB
type ActivityRecord struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	GSI1PK    string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string `dynamodbav:"GSI1SK,omitempty"`
	Activity  *activitypub.Activity
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// RelationshipRecord represents a follow relationship in DynamoDB
type RelationshipRecord struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	GSI1PK     string    `dynamodbav:"GSI1PK"`
	GSI1SK     string    `dynamodbav:"GSI1SK"`
	ActivityID string    `dynamodbav:"ActivityID"`
	State      string    `dynamodbav:"State"` // pending, accepted, rejected
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// ObjectRecord represents an object stored in DynamoDB
type ObjectRecord struct {
	PK        string      `dynamodbav:"PK"`
	SK        string      `dynamodbav:"SK"`
	Type      string      `dynamodbav:"Type"`
	Object    interface{} `dynamodbav:"Object"`
	CreatedAt time.Time   `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time   `dynamodbav:"UpdatedAt"`
}

// Constants for DynamoDB key patterns
const (
	// Actor keys
	ActorPKPrefix = "ACTOR#"
	ActorSK       = "PROFILE"

	// Activity keys
	ActivitySKPrefix  = "ACTIVITY#"
	InboxGSI1PKPrefix = "INBOX#"

	// Relationship keys
	FollowPKPrefix    = "FOLLOW#"
	FollowingSKPrefix = "FOLLOWING#"
	FollowerSKPrefix  = "FOLLOWER#"

	// Object keys
	ObjectPKPrefix  = "OBJECT#"
	VersionSKPrefix = "VERSION#"

	// Relationship states
	RelationshipPending  = "pending"
	RelationshipAccepted = "accepted"
	RelationshipRejected = "rejected"
)
