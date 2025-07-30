package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// Reputation represents reputation data for an actor
type Reputation struct {
	// Primary key fields - EXACT pattern from legacy
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// Reputation data stored as JSON
	ReputationData string `json:"reputation_data"`

	// Indexed fields for queries
	TotalScore   int    `json:"total_score"`
	CalculatedAt string `json:"calculated_at"`

	// TTL for 90-day expiration
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys updates the PK and SK based on the reputation data
func (r *Reputation) UpdateKeys(actorID string, reputation interface{}) error {
	// Extract username from actorID using the same logic as legacy
	username := extractUsernameFromActorID(actorID)
	if username == "" {
		return fmt.Errorf("invalid actorID format: %s", actorID)
	}

	// Marshal reputation to JSON first to access fields
	repJSON, err := json.Marshal(reputation)
	if err != nil {
		return fmt.Errorf("failed to marshal reputation: %w", err)
	}

	// Unmarshal to a map to access fields
	var repMap map[string]interface{}
	if err := json.Unmarshal(repJSON, &repMap); err != nil {
		return fmt.Errorf("failed to unmarshal reputation to map: %w", err)
	}

	// Get calculatedAt and convert to time
	calculatedAtStr, ok := repMap["calculatedAt"].(string)
	if !ok {
		return fmt.Errorf("calculatedAt field not found or not a string")
	}
	calculatedAt, err := time.Parse(time.RFC3339, calculatedAtStr)
	if err != nil {
		return fmt.Errorf("failed to parse calculatedAt: %w", err)
	}

	// Get totalScore
	totalScore := 0
	if totalScoreFloat, ok := repMap["totalScore"].(float64); ok {
		totalScore = int(totalScoreFloat)
	} else if totalScoreInt, ok := repMap["totalScore"].(int); ok {
		totalScore = totalScoreInt
	}

	// Set keys - preserve EXACT case from legacy
	r.PK = fmt.Sprintf("ACTOR#%s", username)
	r.SK = fmt.Sprintf("REP#%s", calculatedAt.Format(time.RFC3339))

	// Set fields
	r.ReputationData = string(repJSON)
	r.TotalScore = totalScore
	r.CalculatedAt = calculatedAt.Format(time.RFC3339)

	// Set TTL to 90 days from now
	r.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()

	return nil
}

// ToStorageReputation converts the model back to a map that can be used as storage.Reputation
func (r *Reputation) ToStorageReputation() (interface{}, error) {
	var reputation map[string]interface{}
	if err := json.Unmarshal([]byte(r.ReputationData), &reputation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reputation data: %w", err)
	}
	return reputation, nil
}

