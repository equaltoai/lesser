package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Reputation represents reputation data for an actor
type Reputation struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields - EXACT pattern from legacy
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// Reputation data stored as JSON
	ReputationData string `dynamorm:"attr:reputationData" json:"reputation_data"`

	// Indexed fields for queries
	TotalScore   int    `dynamorm:"attr:totalScore" json:"total_score"`
	CalculatedAt string `dynamorm:"attr:calculatedAt" json:"calculated_at"`

	// TTL for 90-day expiration
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the PK and SK based on the reputation data
func (r *Reputation) UpdateKeys(actorID string, reputation interface{}) error {
	// Extract username from actorID using the same logic as legacy
	username := extractUsernameFromActorID(actorID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidActorIDFormat, actorID)
	}

	// Marshal reputation to JSON first to access fields
	repJSON, err := json.Marshal(reputation)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrReputationMarshalFailed, err)
	}

	// Validate JSON before unmarshaling
	if err := common.ValidateJSONField(string(repJSON), "reputation"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReputationJSON, err)
	}

	// Unmarshal to a map to access fields
	var repMap map[string]interface{}
	if err := json.Unmarshal(repJSON, &repMap); err != nil {
		return fmt.Errorf("%w: %w", ErrReputationUnmarshalFailed, err)
	}

	// Get calculatedAt and convert to time
	calculatedAtStr, ok := repMap["calculatedAt"].(string)
	if !ok {
		if alt, ok2 := repMap["calculated_at"].(string); ok2 {
			calculatedAtStr = alt
			ok = true
		}
	}
	if !ok || calculatedAtStr == "" {
		return ErrCalculatedAtFieldMissing
	}
	calculatedAt, err := time.Parse(time.RFC3339, calculatedAtStr)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCalculatedAtParseFailed, err)
	}

	// Get totalScore
	totalScore := 0
	if totalScoreFloat, ok := repMap["totalScore"].(float64); ok {
		totalScore = int(totalScoreFloat)
	} else if totalScoreInt, ok := repMap["totalScore"].(int); ok {
		totalScore = totalScoreInt
	} else if totalScoreFloat, ok := repMap["total_score"].(float64); ok {
		totalScore = int(totalScoreFloat)
	} else if totalScoreInt, ok := repMap["total_score"].(int); ok {
		totalScore = totalScoreInt
	}

	// Set keys - preserve EXACT case from legacy
	r.PK = fmt.Sprintf(KeyPatternActor, username)
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
	// Validate JSON before unmarshaling
	if err := common.ValidateJSONField(r.ReputationData, "reputation_data"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidReputationDataJSON, err)
	}

	var reputation map[string]interface{}
	if err := json.Unmarshal([]byte(r.ReputationData), &reputation); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReputationDataUnmarshalFailed, err)
	}
	return reputation, nil
}

// TableName returns the DynamoDB table backing Reputation.
func (Reputation) TableName() string {
	return MainTableName
}
