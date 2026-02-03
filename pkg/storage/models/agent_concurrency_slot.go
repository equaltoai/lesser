package models

import (
	"fmt"
	"strings"
	"time"
)

// AgentConcurrencySlot is a DynamoDB-backed semaphore slot for agent tokens.
//
// The semaphore is represented as a fixed set of slots per agent session ID. Each slot is
// claimed with a short TTL so abandoned leases can be reclaimed safely.
type AgentConcurrencySlot struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"` // AGENT_CONCURRENCY#{session_id}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // SLOT#{n}

	LeaseID   string    `theorydb:"attr:leaseID" json:"lease_id"`
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table backing AgentConcurrencySlot.
func (AgentConcurrencySlot) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the lease for persistence and derives the TTL from ExpiresAt.
func (s *AgentConcurrencySlot) BeforeCreate() error {
	if s == nil {
		return fmt.Errorf("nil model")
	}

	s.PK = strings.TrimSpace(s.PK)
	s.SK = strings.TrimSpace(s.SK)
	s.LeaseID = strings.TrimSpace(s.LeaseID)

	if s.PK == "" || s.SK == "" || s.LeaseID == "" || s.ExpiresAt.IsZero() {
		return fmt.Errorf("missing required fields")
	}

	s.TTL = s.ExpiresAt.UTC().Unix()
	return nil
}
