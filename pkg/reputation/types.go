package reputation

import (
	"time"
)

// Reputation represents a user's reputation score and evidence
type Reputation struct {
	// Identity
	ActorID     string `json:"@id" dynamodbav:"ActorID"`
	InstanceURL string `json:"instance" dynamodbav:"InstanceURL"`

	// Scores (0-1000 scale)
	TrustScore      int `json:"trustScore" dynamodbav:"TrustScore"`
	ActivityScore   int `json:"activityScore" dynamodbav:"ActivityScore"`
	ModerationScore int `json:"moderationScore" dynamodbav:"ModerationScore"`
	CommunityScore  int `json:"communityScore" dynamodbav:"CommunityScore"`
	TotalScore      int `json:"totalScore" dynamodbav:"TotalScore"`

	// Metadata
	CalculatedAt time.Time `json:"calculatedAt" dynamodbav:"CalculatedAt"`
	Version      string    `json:"version" dynamodbav:"Version"`

	// Evidence
	TotalPosts     int `json:"totalPosts" dynamodbav:"TotalPosts"`
	TotalFollowers int `json:"totalFollowers" dynamodbav:"TotalFollowers"`
	AccountAge     int `json:"accountAgeDays" dynamodbav:"AccountAge"`
	VouchCount     int `json:"vouchCount" dynamodbav:"VouchCount"`

	// Trust graph metrics
	TrustingActors    int     `json:"trustingActors" dynamodbav:"TrustingActors"`
	AverageTrustScore float64 `json:"averageTrustScore" dynamodbav:"AverageTrustScore"`

	// Moderation metrics
	ReportsReceived int `json:"reportsReceived" dynamodbav:"ReportsReceived"`
	ReportsUpheld   int `json:"reportsUpheld" dynamodbav:"ReportsUpheld"`
	FalseReports    int `json:"falseReports" dynamodbav:"FalseReports"`

	// Cryptographic proof
	Signature string `json:"signature,omitempty" dynamodbav:"Signature,omitempty"`
	PublicKey string `json:"publicKey,omitempty" dynamodbav:"PublicKey,omitempty"`
}

// Vouch represents one user vouching for another
type Vouch struct {
	ID          string    `json:"@id" dynamodbav:"ID"`
	From        string    `json:"from" dynamodbav:"From"` // Actor who vouched
	To          string    `json:"to" dynamodbav:"To"`     // Actor being vouched for
	InstanceURL string    `json:"instance" dynamodbav:"InstanceURL"`
	CreatedAt   time.Time `json:"createdAt" dynamodbav:"CreatedAt"`
	ExpiresAt   time.Time `json:"expiresAt" dynamodbav:"ExpiresAt"`
	Confidence  float64   `json:"confidence" dynamodbav:"Confidence"` // 0.0-1.0
	Context     string    `json:"context" dynamodbav:"Context"`       // Why vouching

	// Voucher reputation at time of vouch
	VoucherReputation int `json:"voucherReputation" dynamodbav:"VoucherReputation"`

	// Status
	Active    bool       `json:"active" dynamodbav:"Active"`
	Revoked   bool       `json:"revoked" dynamodbav:"Revoked"`
	RevokedAt *time.Time `json:"revokedAt,omitempty" dynamodbav:"RevokedAt,omitempty"`

	// Cryptographic proof
	Signature string `json:"signature" dynamodbav:"Signature"`
}

// PortableReputation is the exportable reputation document
type PortableReputation struct {
	Context    []string    `json:"@context"`
	Type       string      `json:"@type"`
	Actor      string      `json:"actor"`
	Reputation *Reputation `json:"reputation"`
	Vouches    []Vouch     `json:"vouches"`
	IssuedAt   time.Time   `json:"issuedAt"`
	ExpiresAt  time.Time   `json:"expiresAt"`

	// Instance attestation
	Issuer      string `json:"issuer"`
	IssuerProof string `json:"issuerProof"`
}

// CalculationInput contains all data needed to calculate reputation
type CalculationInput struct {
	ActorID string

	// Activity metrics
	PostCount      int
	FollowerCount  int
	FollowingCount int
	AccountCreated time.Time
	LastActive     time.Time

	// Trust graph data
	TrustRelationships []TrustRelationship

	// Moderation data
	ModerationHistory []ModerationEvent

	// Vouch data
	VouchesReceived []Vouch
	VouchesGiven    []Vouch

	// Community contributions
	CommunityNotes int
	HelpfulVotes   int
}

// TrustRelationship represents a trust connection
type TrustRelationship struct {
	FromActor  string
	ToActor    string
	TrustScore float64
	Category   string
	UpdatedAt  time.Time
}

// ModerationEvent represents a moderation action
type ModerationEvent struct {
	ID         string
	Type       string // "report", "suspension", "appeal"
	Outcome    string // "upheld", "dismissed", "pending"
	Severity   int
	OccurredAt time.Time
}

// ImportResult represents the result of importing reputation
type ImportResult struct {
	Success         bool   `json:"success"`
	ActorID         string `json:"actorId"`
	PreviousScore   int    `json:"previousScore"`
	ImportedScore   int    `json:"importedScore"`
	VouchesImported int    `json:"vouchesImported"`
	Message         string `json:"message,omitempty"`
	Error           string `json:"error,omitempty"`
}

// VerificationResult represents the result of verifying a reputation document
type VerificationResult struct {
	Valid          bool      `json:"valid"`
	ActorID        string    `json:"actorId"`
	Issuer         string    `json:"issuer"`
	IssuedAt       time.Time `json:"issuedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	SignatureValid bool      `json:"signatureValid"`
	NotExpired     bool      `json:"notExpired"`
	IssuerTrusted  bool      `json:"issuerTrusted"`
	Error          string    `json:"error,omitempty"`
}
