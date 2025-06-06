package trust

import (
	"time"
)

// TrustCategory represents the category of trust
type TrustCategory string

const (
	TrustCategoryContent   TrustCategory = "content"   // Trust in content moderation
	TrustCategoryBehavior  TrustCategory = "behavior"  // Trust in behavior assessment
	TrustCategoryTechnical TrustCategory = "technical" // Trust in technical decisions
	TrustCategoryGeneral   TrustCategory = "general"   // General trust
)

// TrustRelationship represents a trust relationship between two actors
type TrustRelationship struct {
	ID         string          `json:"id"`
	TrusterID  string          `json:"truster_id"` // Who trusts
	TrusteeID  string          `json:"trustee_id"` // Who is trusted
	Category   TrustCategory   `json:"category"`
	Score      float64         `json:"score"`      // -1.0 to 1.0 (-1 = distrust, 0 = neutral, 1 = full trust)
	Confidence float64         `json:"confidence"` // 0.0 to 1.0 (how confident in this score)
	Evidence   []TrustEvidence `json:"evidence,omitempty"`
	Created    time.Time       `json:"created"`
	Updated    time.Time       `json:"updated"`
	TTL        int64           `json:"ttl,omitempty"` // Unix timestamp for expiration
}

// TrustEvidence represents evidence supporting a trust relationship
type TrustEvidence struct {
	Type        string    `json:"type"`  // consensus_agreement, direct_interaction, etc.
	Score       float64   `json:"score"` // Impact on trust score
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// TrustScore represents a calculated trust score for an actor
type TrustScore struct {
	ActorID         string             `json:"actor_id"`
	Category        TrustCategory      `json:"category"`
	Score           float64            `json:"score"`            // Aggregated score
	DirectScore     float64            `json:"direct_score"`     // Score from direct relationships
	PropagatedScore float64            `json:"propagated_score"` // Score from network propagation
	Confidence      float64            `json:"confidence"`       // Overall confidence
	TrusterCount    int                `json:"truster_count"`    // Number of actors who trust this one
	LastCalculated  time.Time          `json:"last_calculated"`
	CacheTTL        time.Time          `json:"cache_ttl"` // When to recalculate
	CategoryScores  map[string]float64 `json:"category_scores,omitempty"`
}

// TrustUpdate represents an update to trust based on moderation outcomes
type TrustUpdate struct {
	ActorID   string        `json:"actor_id"`
	Category  TrustCategory `json:"category"`
	Delta     float64       `json:"delta"`    // Change in trust score
	Reason    string        `json:"reason"`   // Why the update occurred
	EventID   string        `json:"event_id"` // Related moderation event
	Timestamp time.Time     `json:"timestamp"`
}

// TrustNetwork represents a view of the trust network for analysis
type TrustNetwork struct {
	RootActorID   string                 `json:"root_actor_id"`
	Relationships []TrustRelationship    `json:"relationships"`
	Scores        map[string]*TrustScore `json:"scores"`
	Depth         int                    `json:"depth"`     // How many hops to include
	MinScore      float64                `json:"min_score"` // Minimum score to include
	Generated     time.Time              `json:"generated"`
}

// TrustPropagationConfig configures how trust propagates through the network
type TrustPropagationConfig struct {
	MaxDepth           int     `json:"max_depth"`            // Maximum hops for propagation
	DecayFactor        float64 `json:"decay_factor"`         // How much trust decays per hop (0.0-1.0)
	NegativeMultiplier float64 `json:"negative_multiplier"`  // Weight for negative trust signals
	MinPropagatedScore float64 `json:"min_propagated_score"` // Minimum score to propagate
	CacheDuration      int     `json:"cache_duration_hours"` // How long to cache scores
}

// DefaultPropagationConfig returns the default trust propagation configuration
func DefaultPropagationConfig() *TrustPropagationConfig {
	return &TrustPropagationConfig{
		MaxDepth:           3,
		DecayFactor:        0.5,
		NegativeMultiplier: 1.5,
		MinPropagatedScore: 0.1,
		CacheDuration:      2,
	}
}

// TrustSummary provides a summary view of an actor's trust status
type TrustSummary struct {
	ActorID         string                    `json:"actor_id"`
	OverallScore    float64                   `json:"overall_score"`
	CategoryScores  map[TrustCategory]float64 `json:"category_scores"`
	TrustedByCount  int                       `json:"trusted_by_count"`
	TrustsCount     int                       `json:"trusts_count"`
	LastActive      time.Time                 `json:"last_active"`
	ReputationLevel string                    `json:"reputation_level"` // high, medium, low, new
}

// TrustEdge represents a single edge in the trust graph (for visualization)
type TrustEdge struct {
	From       string        `json:"from"`
	To         string        `json:"to"`
	Category   TrustCategory `json:"category"`
	Score      float64       `json:"score"`
	Confidence float64       `json:"confidence"`
	Weight     float64       `json:"weight"` // Combined score * confidence
}
