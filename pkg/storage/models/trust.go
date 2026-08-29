package models

import (
	"fmt"
	"net/url"
	"time"
)

// TrustCategory represents the category of trust
type TrustCategory string

const (
	// TrustCategoryContent represents trust category for content
	TrustCategoryContent TrustCategory = "content" // Trust in content moderation
	// TrustCategoryBehavior represents trust category for behavior
	TrustCategoryBehavior TrustCategory = "behavior" // Trust in behavior assessment
	// TrustCategoryTechnical represents trust category for technical aspects
	TrustCategoryTechnical TrustCategory = "technical" // Trust in technical decisions
	// TrustCategoryGeneral represents general trust category
	TrustCategoryGeneral TrustCategory = "general" // General trust
)

// TrustEvidence represents evidence supporting a trust relationship
type TrustEvidence struct {
	Type        string    `json:"type"`  // consensus_agreement, direct_interaction, etc.
	Score       float64   `json:"score"` // Impact on trust score
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// TableName returns the DynamoDB table backing TrustEvidence.
func (TrustEvidence) TableName() string {
	return MainTableName
}

// TrustRelationship represents a trust relationship between two actors
type TrustRelationship struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - exact patterns from legacy
	PK string `theorydb:"pk,attr:PK"` // TRUST#trusterID#category
	SK string `theorydb:"sk,attr:SK"` // TRUSTEE#trusteeID

	// GSI1 - for reverse lookups (who trusts this trustee)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"` // TRUSTED#trusteeID#category
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"` // TRUSTER#trusterID

	// GSI2 - for domain-based queries
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"` // DOMAIN#domain
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"` // TRUST#category#score

	// GSI3 - for global listing (admin visualization)
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty"` // TRUST_RELATIONSHIPS
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty"` // TRUST#trusterID#category#TRUSTEE#trusteeID

	// Business fields
	ID         string          `theorydb:"attr:id" json:"id"`
	TrusterID  string          `theorydb:"attr:trusterID" json:"truster_id"`
	TrusteeID  string          `theorydb:"attr:trusteeID" json:"trustee_id"`
	Category   TrustCategory   `theorydb:"attr:category" json:"category"`
	Score      float64         `theorydb:"attr:score" json:"score"`           // -1.0 to 1.0
	Confidence float64         `theorydb:"attr:confidence" json:"confidence"` // 0.0 to 1.0
	Evidence   []TrustEvidence `theorydb:"attr:evidence" json:"evidence,omitempty"`
	TTL        int64           `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
	Created    time.Time       `theorydb:"attr:created" json:"created"`
	Updated    time.Time       `theorydb:"attr:updated" json:"updated"`

	// Type marker for filtering
	Type string `theorydb:"attr:type" json:"type"` // Always "RELATIONSHIP"
}

// TableName returns the DynamoDB table backing TrustRelationship.
func (TrustRelationship) TableName() string {
	return MainTableName
}

// UpdateKeys sets all the DynamoDB keys based on the relationship data
func (tr *TrustRelationship) UpdateKeys() error {
	// Primary keys
	tr.PK = fmt.Sprintf("TRUST#%s#%s", tr.TrusterID, tr.Category)
	tr.SK = fmt.Sprintf("TRUSTEE#%s", tr.TrusteeID)

	// GSI1 keys for reverse lookup
	tr.GSI1PK = fmt.Sprintf("TRUSTED#%s#%s", tr.TrusteeID, tr.Category)
	tr.GSI1SK = fmt.Sprintf("TRUSTER#%s", tr.TrusterID)

	// GSI2 keys for domain queries
	domain := getDomainFromActorID(tr.TrusteeID)
	tr.GSI2PK = fmt.Sprintf("DOMAIN#%s", domain)
	tr.GSI2SK = fmt.Sprintf("TRUST#%s#%f", tr.Category, tr.Score)

	// GSI3 keys for the global trust relationship listing
	tr.GSI3PK = "TRUST_RELATIONSHIPS"
	tr.GSI3SK = fmt.Sprintf("TRUST#%s#%s#TRUSTEE#%s", tr.TrusterID, tr.Category, tr.TrusteeID)

	// Set type
	tr.Type = "RELATIONSHIP"
	return nil
}

// GetPK returns the partition key for this trust relationship
func (tr *TrustRelationship) GetPK() string {
	return tr.PK
}

// GetSK returns the sort key for this trust relationship
func (tr *TrustRelationship) GetSK() string {
	return tr.SK
}

// TrustScore represents a cached trust score for an actor
type TrustScore struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys for cached scores
	PK string `theorydb:"pk,attr:PK"` // SCORE#actorID#category
	SK string `theorydb:"sk,attr:SK"` // CURRENT

	// Business fields
	ActorID         string             `theorydb:"attr:actorID" json:"actor_id"`
	Category        TrustCategory      `theorydb:"attr:category" json:"category"`
	Score           float64            `theorydb:"attr:score" json:"score"`                      // Aggregated score
	DirectScore     float64            `theorydb:"attr:directScore" json:"direct_score"`         // Score from direct relationships
	PropagatedScore float64            `theorydb:"attr:propagatedScore" json:"propagated_score"` // Score from network propagation
	Confidence      float64            `theorydb:"attr:confidence" json:"confidence"`            // Confidence in score
	TrusterCount    int                `theorydb:"attr:trusterCount" json:"truster_count"`       // Number of direct trusters
	CategoryScores  map[string]float64 `theorydb:"attr:categoryScores" json:"category_scores"`   // Scores by category
	LastCalculated  time.Time          `theorydb:"attr:lastCalculated" json:"last_calculated"`
	CacheTTL        time.Time          `theorydb:"attr:cacheTTL" json:"cache_ttl"`
	TTL             int64              `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Type marker
	Type string `theorydb:"attr:type" json:"type"` // Always "SCORE"
}

// TableName returns the DynamoDB table backing TrustScore.
func (TrustScore) TableName() string {
	return MainTableName
}

// UpdateKeys sets all the DynamoDB keys for the trust score
func (ts *TrustScore) UpdateKeys() error {
	ts.PK = fmt.Sprintf("SCORE#%s#%s", ts.ActorID, ts.Category)
	ts.SK = SKCurrent
	ts.Type = "SCORE"

	// Set TTL to cache TTL
	if !ts.CacheTTL.IsZero() {
		ts.TTL = ts.CacheTTL.Unix()
	}
	return nil
}

// GetPK returns the partition key for this trust score
func (ts *TrustScore) GetPK() string {
	return ts.PK
}

// GetSK returns the sort key for this trust score
func (ts *TrustScore) GetSK() string {
	return ts.SK
}

// TrustUpdate represents a trust score update event
type TrustUpdate struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys for update history
	PK string `theorydb:"pk,attr:PK"` // UPDATES#actorID
	SK string `theorydb:"sk,attr:SK"` // TIME#timestamp#eventID

	// Business fields
	ActorID   string        `theorydb:"attr:actorID" json:"actor_id"`
	EventID   string        `theorydb:"attr:eventID" json:"event_id"`
	Category  TrustCategory `theorydb:"attr:category" json:"category"`
	Delta     float64       `theorydb:"attr:delta" json:"delta"`   // Change in trust score
	Reason    string        `theorydb:"attr:reason" json:"reason"` // Why the update occurred
	Timestamp time.Time     `theorydb:"attr:timestamp" json:"timestamp"`
	TTL       int64         `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Type marker
	Type string `theorydb:"attr:type" json:"type"` // Always "UPDATE"
}

// TableName returns the DynamoDB table backing TrustUpdate.
func (TrustUpdate) TableName() string {
	return MainTableName
}

// UpdateKeys sets all the DynamoDB keys for the trust update
func (tu *TrustUpdate) UpdateKeys() error {
	tu.PK = fmt.Sprintf("UPDATES#%s", tu.ActorID)
	tu.SK = fmt.Sprintf("TIME#%s#%s", tu.Timestamp.Format(time.RFC3339), tu.EventID)
	tu.Type = "UPDATE"

	// Set TTL to 30 days from timestamp
	if tu.TTL == 0 {
		tu.TTL = tu.Timestamp.Add(30 * 24 * time.Hour).Unix()
	}
	return nil
}

// GetPK returns the partition key for this trust update
func (tu *TrustUpdate) GetPK() string {
	return tu.PK
}

// GetSK returns the sort key for this trust update
func (tu *TrustUpdate) GetSK() string {
	return tu.SK
}

// Helper function to extract domain from actor ID
func getDomainFromActorID(actorID string) string {
	// First, try to parse as URL (handles https://domain.com/users/username format)
	if u, err := url.Parse(actorID); err == nil && u.Host != "" {
		return u.Host
	}

	// Look for the last @ in the actor ID (handles @user@domain.com format)
	for i := len(actorID) - 1; i >= 0; i-- {
		if actorID[i] == '@' {
			return actorID[i+1:]
		}
	}

	// Default to "local" if no domain found
	return "local"
}
