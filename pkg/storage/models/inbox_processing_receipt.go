package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	inboxProcessingReceiptPKPrefix = "INBOX_ACTIVITY#"
	inboxProcessingReceiptSKPrefix = "TARGET#"
	inboxProcessingReceiptTTL      = 30 * 24 * time.Hour
)

// InboxProcessingReceipt records that one ActivityPub activity has been routed
// to one local target actor. It is a short-lived idempotency guard for inbox
// side effects when the same remote Create/Like/Announce/Undo reaches both a
// shared inbox and an actor inbox.
type InboxProcessingReceipt struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	ActivityID    string    `theorydb:"attr:activityID" json:"activity_id"`
	TargetActorID string    `theorydb:"attr:targetActorID" json:"target_actor_id"`
	ActivityType  string    `theorydb:"attr:activityType" json:"activity_type,omitempty"`
	CreatedAt     time.Time `theorydb:"attr:createdAt" json:"created_at"`
	TTL           int64     `theorydb:"ttl,attr:ttl" json:"-"`
	Version       int       `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the DynamoDB table backing InboxProcessingReceipt.
func (InboxProcessingReceipt) TableName() string {
	return MainTableName
}

// NewInboxProcessingReceipt builds a target-scoped inbox idempotency receipt.
func NewInboxProcessingReceipt(activityID, targetActorID, activityType string, now time.Time) *InboxProcessingReceipt {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	return &InboxProcessingReceipt{
		ActivityID:    strings.TrimSpace(activityID),
		TargetActorID: strings.TrimSpace(targetActorID),
		ActivityType:  strings.TrimSpace(activityType),
		CreatedAt:     now,
		TTL:           now.Add(inboxProcessingReceiptTTL).Unix(),
	}
}

// BeforeCreate derives keys and TTL before persistence.
func (r *InboxProcessingReceipt) BeforeCreate() error {
	return r.UpdateKeys()
}

// BeforeUpdate derives keys before persistence.
func (r *InboxProcessingReceipt) BeforeUpdate() error {
	return r.UpdateKeys()
}

// GetPK returns the partition key.
func (r *InboxProcessingReceipt) GetPK() string {
	if r == nil {
		return ""
	}
	return r.PK
}

// GetSK returns the sort key.
func (r *InboxProcessingReceipt) GetSK() string {
	if r == nil {
		return ""
	}
	return r.SK
}

// UpdateKeys derives the receipt's storage keys.
func (r *InboxProcessingReceipt) UpdateKeys() error {
	if r == nil {
		return fmt.Errorf("nil inbox processing receipt")
	}

	r.ActivityID = strings.TrimSpace(r.ActivityID)
	r.TargetActorID = strings.TrimSpace(r.TargetActorID)
	r.ActivityType = strings.TrimSpace(r.ActivityType)
	if r.ActivityID == "" {
		return fmt.Errorf("activity id is required")
	}
	if r.TargetActorID == "" {
		return fmt.Errorf("target actor id is required")
	}

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	} else {
		r.CreatedAt = r.CreatedAt.UTC()
	}
	if r.TTL == 0 {
		r.TTL = r.CreatedAt.Add(inboxProcessingReceiptTTL).Unix()
	}

	r.PK = inboxProcessingReceiptPKPrefix + inboxProcessingReceiptHash(r.ActivityID)
	r.SK = inboxProcessingReceiptSKPrefix + inboxProcessingReceiptHash(r.TargetActorID)
	return nil
}

func inboxProcessingReceiptHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
