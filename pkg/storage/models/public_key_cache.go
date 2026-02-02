package models

import (
	"fmt"
	"time"
)

// PublicKeyCache represents a cached public key for ActivityPub signature verification
type PublicKeyCache struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Fields
	ActorURL     string    `theorydb:"attr:actorURL" json:"actor_url"`          // URL of the actor
	KeyID        string    `theorydb:"attr:keyID" json:"key_id"`                // Public key ID
	PublicKeyPEM string    `theorydb:"attr:publicKeyPEM" json:"public_key_pem"` // PEM-encoded public key
	Algorithm    string    `theorydb:"attr:algorithm" json:"algorithm"`         // Signature algorithm (rsa-sha256, etc.)
	FetchedAt    time.Time `theorydb:"attr:fetchedAt" json:"fetched_at"`        // When the key was fetched
	LastUsed     time.Time `theorydb:"attr:lastUsed" json:"last_used"`          // Last time this key was used
	SuccessCount int       `theorydb:"attr:successCount" json:"success_count"`  // Number of successful verifications
	FailureCount int       `theorydb:"attr:failureCount" json:"failure_count"`  // Number of failed verifications
	TTL          int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`       // Unix timestamp for DynamoDB TTL
}

// NewPublicKeyCache creates a new public key cache entry
func NewPublicKeyCache(actorURL, keyID, publicKeyPEM, algorithm string) *PublicKeyCache {
	now := time.Now()
	cache := &PublicKeyCache{
		ActorURL:     actorURL,
		KeyID:        keyID,
		PublicKeyPEM: publicKeyPEM,
		Algorithm:    algorithm,
		FetchedAt:    now,
		LastUsed:     now,
		SuccessCount: 0,
		FailureCount: 0,
		TTL:          now.Add(24 * time.Hour).Unix(), // Cache for 24 hours
	}
	if err := cache.UpdateKeys(); err != nil {
		return nil
	}
	return cache
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (p *PublicKeyCache) UpdateKeys() error {
	if p.ActorURL != "" {
		p.PK = fmt.Sprintf("PUBKEY_CACHE#%s", p.ActorURL)
		p.SK = "KEY"
	}
	return nil
}

// GetPK returns the partition key
func (p *PublicKeyCache) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *PublicKeyCache) GetSK() string {
	return p.SK
}

// TableName returns the DynamoDB table backing PublicKeyCache.
func (PublicKeyCache) TableName() string {
	return MainTableName
}

// IsValid checks if the cache entry is still valid
func (p *PublicKeyCache) IsValid() bool {
	return time.Now().Unix() < p.TTL
}

// GetPublicKeyPEM returns the cached PEM public key for parsing elsewhere
func (p *PublicKeyCache) GetPublicKeyPEM() []byte {
	return []byte(p.PublicKeyPEM)
}

// RecordSuccess increments the success count and updates last used time
func (p *PublicKeyCache) RecordSuccess() {
	p.SuccessCount++
	p.LastUsed = time.Now()
}

// RecordFailure increments the failure count
func (p *PublicKeyCache) RecordFailure() {
	p.FailureCount++
}

// ExtendTTL extends the cache entry TTL by the specified duration
func (p *PublicKeyCache) ExtendTTL(duration time.Duration) {
	p.TTL = time.Now().Add(duration).Unix()
}

// ShouldRefresh checks if the key should be refreshed based on failure rate
func (p *PublicKeyCache) ShouldRefresh() bool {
	totalAttempts := p.SuccessCount + p.FailureCount
	if totalAttempts == 0 {
		return false
	}

	// Refresh if failure rate > 50% and we have at least 5 attempts
	failureRate := float64(p.FailureCount) / float64(totalAttempts)
	return failureRate > 0.5 && totalAttempts >= 5
}
