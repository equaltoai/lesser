package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// PKSoulBindingIdempotencyPrefix scopes soul-binding idempotency receipts by authenticated caller.
	PKSoulBindingIdempotencyPrefix = "SOUL_BINDING_IDEMPOTENCY#"
	// SKSoulBindingIdempotencyKeyPrefix scopes one idempotency key receipt under a caller partition.
	SKSoulBindingIdempotencyKeyPrefix = "KEY#"
)

// InstanceSoulBindingIdempotencyReceipt stores TTL-scoped replay evidence for
// server-side soul-binding requests. It is control-plane evidence only; the
// terminal binding truth remains the SOUL_BODY_BINDING rows.
type InstanceSoulBindingIdempotencyReceipt struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	CallerID           string    `theorydb:"attr:callerId" json:"caller_id"`
	IdempotencyKeyHash string    `theorydb:"attr:idempotencyKeyHash" json:"idempotency_key_hash"`
	PayloadHash        string    `theorydb:"attr:payloadHash" json:"payload_hash"`
	AgentID            string    `theorydb:"attr:agentId" json:"agent_id"`
	ActorUsername      string    `theorydb:"attr:actorUsername" json:"actor_username"`
	BodyActorID        string    `theorydb:"attr:bodyActorId,omitempty" json:"body_actor_id,omitempty"`
	Status             string    `theorydb:"attr:status" json:"status"`
	BindingState       string    `theorydb:"attr:bindingState,omitempty" json:"binding_state,omitempty"`
	CreatedAt          time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt          time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	TTL                int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
	Version            int       `theorydb:"version,attr:version" json:"version"`
}

// TableName returns the DynamoDB table backing InstanceSoulBindingIdempotencyReceipt.
func (InstanceSoulBindingIdempotencyReceipt) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (r *InstanceSoulBindingIdempotencyReceipt) UpdateKeys() error {
	if r == nil {
		return fmt.Errorf("soul binding idempotency receipt is nil")
	}

	r.CallerID = normalizeSoulBindingReceiptCaller(r.CallerID)
	r.IdempotencyKeyHash = normalizeSoulBindingReceiptHash(r.IdempotencyKeyHash)
	r.PayloadHash = strings.TrimSpace(r.PayloadHash)
	r.AgentID = strings.ToLower(strings.TrimSpace(r.AgentID))
	r.ActorUsername = strings.TrimSpace(r.ActorUsername)
	r.BodyActorID = strings.TrimSpace(r.BodyActorID)
	r.Status = strings.TrimSpace(r.Status)
	r.BindingState = strings.TrimSpace(r.BindingState)

	if r.CallerID == "" {
		return fmt.Errorf("caller id is required")
	}
	if r.IdempotencyKeyHash == "" {
		return fmt.Errorf("idempotency key hash is required")
	}
	if r.PayloadHash == "" {
		return fmt.Errorf("payload hash is required")
	}
	if r.AgentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if r.ActorUsername == "" {
		return fmt.Errorf("actor username is required")
	}
	if r.Status == "" {
		r.Status = "received"
	}

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}

	r.PK = SoulBindingIdempotencyPartitionKey(r.CallerID)
	r.SK = SoulBindingIdempotencySortKeyFromHash(r.IdempotencyKeyHash)
	return nil
}

// GetPK returns the partition key.
func (r *InstanceSoulBindingIdempotencyReceipt) GetPK() string {
	return r.PK
}

// GetSK returns the sort key.
func (r *InstanceSoulBindingIdempotencyReceipt) GetSK() string {
	return r.SK
}

// NewInstanceSoulBindingIdempotencyReceipt creates a new TTL-scoped soul-binding receipt.
func NewInstanceSoulBindingIdempotencyReceipt(callerID string, idempotencyKey string, payloadHash string, agentID string, actorUsername string, ttl time.Time) *InstanceSoulBindingIdempotencyReceipt {
	now := time.Now().UTC()
	receipt := &InstanceSoulBindingIdempotencyReceipt{
		CallerID:           callerID,
		IdempotencyKeyHash: SoulBindingIdempotencyKeyHash(idempotencyKey),
		PayloadHash:        strings.TrimSpace(payloadHash),
		AgentID:            strings.ToLower(strings.TrimSpace(agentID)),
		ActorUsername:      strings.TrimSpace(actorUsername),
		Status:             "received",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if !ttl.IsZero() {
		receipt.TTL = ttl.UTC().Unix()
	}
	_ = receipt.UpdateKeys()
	return receipt
}

// SoulBindingIdempotencyPartitionKey returns the caller-scoped receipt partition.
func SoulBindingIdempotencyPartitionKey(callerID string) string {
	return PKSoulBindingIdempotencyPrefix + hashSoulBindingReceiptValue(normalizeSoulBindingReceiptCaller(callerID))
}

// SoulBindingIdempotencySortKey returns the key-scoped receipt sort key.
func SoulBindingIdempotencySortKey(idempotencyKey string) string {
	return SoulBindingIdempotencySortKeyFromHash(SoulBindingIdempotencyKeyHash(idempotencyKey))
}

// SoulBindingIdempotencySortKeyFromHash returns the key-scoped receipt sort key from a precomputed hash.
func SoulBindingIdempotencySortKeyFromHash(idempotencyKeyHash string) string {
	return SKSoulBindingIdempotencyKeyPrefix + normalizeSoulBindingReceiptHash(idempotencyKeyHash)
}

// SoulBindingIdempotencyKeyHash hashes the caller-supplied idempotency key before storage.
func SoulBindingIdempotencyKeyHash(idempotencyKey string) string {
	return hashSoulBindingReceiptValue(strings.TrimSpace(idempotencyKey))
}

func normalizeSoulBindingReceiptCaller(callerID string) string {
	return strings.ToLower(strings.TrimSpace(callerID))
}

func normalizeSoulBindingReceiptHash(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func hashSoulBindingReceiptValue(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
