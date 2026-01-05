package models

import "time"

// ReputationKeysResponse represents GET /.well-known/reputation-keys.
type ReputationKeysResponse struct {
	PublicKey string `json:"publicKey"`
}

// ReputationEvidence represents supporting evidence for a reputation score.
type ReputationEvidence struct {
	TotalPosts     int `json:"total_posts"`
	TotalFollowers int `json:"total_followers"`
	AccountAge     int `json:"account_age"`
	VouchCount     int `json:"vouch_count"`
}

// ReputationResponse represents GET /api/v1/reputation/{actor_id}.
type ReputationResponse struct {
	ID              string             `json:"id"`
	Instance        string             `json:"instance"`
	TotalScore      int                `json:"total_score"`
	TrustScore      int                `json:"trust_score"`
	ActivityScore   int                `json:"activity_score"`
	ModerationScore int                `json:"moderation_score"`
	CommunityScore  int                `json:"community_score"`
	CalculatedAt    time.Time          `json:"calculated_at"`
	Version         string             `json:"version"`
	Evidence        ReputationEvidence `json:"evidence"`
}

// ReputationDocumentRequest represents a request containing a portable reputation document.
type ReputationDocumentRequest struct {
	Document string `json:"document"`
}

// CreateVouchRequest represents POST /api/v1/vouches.
type CreateVouchRequest struct {
	To         string  `json:"to"`
	Confidence float64 `json:"confidence"`
	Context    string  `json:"context"`
}

// VouchResponse represents a vouch in API responses.
type VouchResponse struct {
	ID                string     `json:"id"`
	From              string     `json:"from"`
	To                string     `json:"to"`
	Confidence        float64    `json:"confidence"`
	Context           string     `json:"context"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	VoucherReputation int        `json:"voucher_reputation"`
	Active            bool       `json:"active"`
	Revoked           bool       `json:"revoked"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
}
