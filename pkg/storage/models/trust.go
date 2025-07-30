package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/trust"
)

// TrustRelationship represents a trust relationship between two actors
type TrustRelationship struct {
	// Primary keys - exact patterns from legacy
	PK string `dynamorm:"pk"` // TRUST#trusterID#category
	SK string `dynamorm:"sk"` // TRUSTEE#trusteeID

	// GSI1 - for reverse lookups (who trusts this trustee)
	GSI1PK string `dynamorm:"index:gsi1-index,pk"` // TRUSTED#trusteeID#category
	GSI1SK string `dynamorm:"index:gsi1-index,sk"` // TRUSTER#trusterID

	// GSI2 - for domain-based queries
	GSI2PK string `dynamorm:"index:gsi2-index,pk"` // DOMAIN#domain
	GSI2SK string `dynamorm:"index:gsi2-index,sk"` // TRUST#category#score

	// Business fields
	ID         string                `json:"id"`
	TrusterID  string                `json:"truster_id"`
	TrusteeID  string                `json:"trustee_id"`
	Category   trust.TrustCategory `json:"category"`
	Score      float64               `json:"score"`      // -1.0 to 1.0
	Confidence float64               `json:"confidence"` // 0.0 to 1.0
	Evidence   []trust.TrustEvidence `json:"evidence,omitempty"`
	TTL        int64                 `json:"ttl,omitempty" dynamorm:"ttl"`
	Created    time.Time             `json:"created"`
	Updated    time.Time             `json:"updated"`

	// Type marker for filtering
	Type string `json:"type"` // Always "RELATIONSHIP"
}

// UpdateKeys sets all the DynamoDB keys based on the relationship data
func (tr *TrustRelationship) UpdateKeys() {
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

	// Set type
	tr.Type = "RELATIONSHIP"
}

// TrustScore represents a cached trust score for an actor
type TrustScore struct {
	// Primary keys for cached scores
	PK string `dynamorm:"pk"` // SCORE#actorID#category
	SK string `dynamorm:"sk"` // CURRENT

	// Business fields
	ActorID         string                       `json:"actor_id"`
	Category        trust.TrustCategory        `json:"category"`
	Score           float64                      `json:"score"`            // Aggregated score
	DirectScore     float64                      `json:"direct_score"`     // Score from direct relationships
	PropagatedScore float64                      `json:"propagated_score"` // Score from network propagation
	Confidence      float64                      `json:"confidence"`       // Confidence in score
	TrusterCount    int                          `json:"truster_count"`    // Number of direct trusters
	CategoryScores  map[string]float64           `json:"category_scores"`  // Scores by category
	LastCalculated  time.Time                    `json:"last_calculated"`
	CacheTTL        time.Time                    `json:"cache_ttl"`
	TTL             int64                        `json:"ttl,omitempty" dynamorm:"ttl"`

	// Type marker
	Type string `json:"type"` // Always "SCORE"
}

// UpdateKeys sets all the DynamoDB keys for the trust score
func (ts *TrustScore) UpdateKeys() {
	ts.PK = fmt.Sprintf("SCORE#%s#%s", ts.ActorID, ts.Category)
	ts.SK = "CURRENT"
	ts.Type = "SCORE"
	
	// Set TTL to cache TTL
	if !ts.CacheTTL.IsZero() {
		ts.TTL = ts.CacheTTL.Unix()
	}
}

// TrustUpdate represents a trust score update event
type TrustUpdate struct {
	// Primary keys for update history
	PK string `dynamorm:"pk"` // UPDATES#actorID
	SK string `dynamorm:"sk"` // TIME#timestamp#eventID

	// Business fields
	ActorID   string                `json:"actor_id"`
	EventID   string                `json:"event_id"`
	Category  trust.TrustCategory `json:"category"`
	Delta     float64               `json:"delta"`    // Change in trust score
	Reason    string                `json:"reason"`   // Why the update occurred
	Timestamp time.Time             `json:"timestamp"`
	TTL       int64                 `json:"ttl,omitempty" dynamorm:"ttl"`

	// Type marker
	Type string `json:"type"` // Always "UPDATE"
}

// UpdateKeys sets all the DynamoDB keys for the trust update
func (tu *TrustUpdate) UpdateKeys() {
	tu.PK = fmt.Sprintf("UPDATES#%s", tu.ActorID)
	tu.SK = fmt.Sprintf("TIME#%s#%s", tu.Timestamp.Format(time.RFC3339), tu.EventID)
	tu.Type = "UPDATE"
	
	// Set TTL to 30 days from timestamp
	if tu.TTL == 0 {
		tu.TTL = tu.Timestamp.Add(30 * 24 * time.Hour).Unix()
	}
}

// Helper function to extract domain from actor ID
func getDomainFromActorID(actorID string) string {
	// Look for the last @ in the actor ID
	for i := len(actorID) - 1; i >= 0; i-- {
		if actorID[i] == '@' {
			return actorID[i+1:]
		}
	}
	// Default to "local" if no domain found
	return "local"
}