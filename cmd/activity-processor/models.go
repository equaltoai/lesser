package main

import (
	"time"
)

// ActivityRecord represents an activity as stored in DynamoDB for processing
// This is different from the storage.models.Activity which stores parsed ActivityPub objects
type ActivityRecord struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	Username   string    `dynamodbav:"Username"`
	Timestamp  string    `dynamodbav:"Timestamp"`
	ActivityID string    `dynamodbav:"ActivityID"`
	Activity   string    `dynamodbav:"Activity"` // Raw JSON string
	Direction  string    `dynamodbav:"Direction"`
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
}
