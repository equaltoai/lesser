package trust

import (
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
)

// Type aliases to storage types - this breaks the import cycle
type (
	TrustCategory     = storage.TrustCategory
	TrustEvidence     = storage.TrustEvidence
	TrustRelationship = storage.TrustRelationship
	TrustScore        = storage.TrustScore
	TrustUpdate       = storage.TrustUpdate
)

// Re-export constants from storage
const (
	TrustCategoryContent   = storage.TrustCategoryContent
	TrustCategoryBehavior  = storage.TrustCategoryBehavior
	TrustCategoryTechnical = storage.TrustCategoryTechnical
	TrustCategoryGeneral   = storage.TrustCategoryGeneral
)

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
