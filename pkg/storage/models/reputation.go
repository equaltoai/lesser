package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Reputation represents reputation data for an actor
type Reputation struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields - EXACT pattern from legacy
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// Reputation data stored as JSON
	ReputationData string `theorydb:"attr:reputationData" json:"reputation_data"`

	// Indexed fields for queries
	TotalScore   int    `theorydb:"attr:totalScore" json:"total_score"`
	CalculatedAt string `theorydb:"attr:calculatedAt" json:"calculated_at"`

	// TTL for 90-day expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the PK and SK based on the reputation data
func (r *Reputation) UpdateKeys(actorID string, reputation interface{}) error {
	pk, err := ReputationActorPartitionKeyForRecord(actorID, reputation)
	if err != nil {
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

	// Set keys. Local actor reputations keep the legacy actor partition for
	// backward compatibility; remote actor reputations use the canonical actor
	// URI so same-username actors on different domains cannot collide.
	r.PK = pk
	r.SK = fmt.Sprintf("REP#%s", calculatedAt.Format(time.RFC3339))

	// Set fields
	r.ReputationData = string(repJSON)
	r.TotalScore = totalScore
	r.CalculatedAt = calculatedAt.Format(time.RFC3339)

	// Set TTL to 90 days from now
	r.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()

	return nil
}

// ReputationActorPartitionKeyForRecord returns the partition key for a
// reputation record. Local actor reputations preserve the legacy
// ACTOR#<username> key because deployed instances already have those rows.
// Remote actor reputations are keyed by the canonical actor URI to bind the
// reputation row to the actor's domain.
func ReputationActorPartitionKeyForRecord(actorID string, reputation interface{}) (string, error) {
	actorID = strings.TrimSpace(actorID)
	canonicalActorID, err := canonicalReputationActorID(actorID)
	if err != nil {
		return "", err
	}

	instanceURL := reputationInstanceURL(reputation)
	if instanceURL != "" && reputationActorHostMatchesInstance(canonicalActorID, instanceURL) {
		username := strings.TrimSpace(strings.TrimPrefix(extractUsernameFromActorID(strings.TrimRight(actorID, "/")), "@"))
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return "", err
		}
		return fmt.Sprintf(KeyPatternActor, username), nil
	}

	return fmt.Sprintf(KeyPatternActor, canonicalActorID), nil
}

// ReputationActorPartitionKeyCandidates returns read candidates in canonical
// order. The legacy username key remains a fallback so local cached reputation
// rows can be read during the transition; callers must still verify the stored
// ActorID matches the requested actor before trusting a fallback row.
func ReputationActorPartitionKeyCandidates(actorID string) ([]string, error) {
	canonicalActorID, err := canonicalReputationActorID(actorID)
	if err != nil {
		return nil, err
	}

	candidates := []string{fmt.Sprintf(KeyPatternActor, canonicalActorID)}
	username := strings.TrimSpace(strings.TrimPrefix(extractUsernameFromActorID(strings.TrimRight(actorID, "/")), "@"))
	if username != "" {
		legacy := fmt.Sprintf(KeyPatternActor, username)
		if legacy != candidates[0] {
			candidates = append(candidates, legacy)
		}
	}
	return candidates, nil
}

// ReputationActorIDsMatch reports whether two reputation actor identifiers
// resolve to the same canonical ActivityPub actor URI.
func ReputationActorIDsMatch(left, right string) bool {
	leftCanonical, leftErr := canonicalReputationActorID(left)
	rightCanonical, rightErr := canonicalReputationActorID(right)
	return leftErr == nil && rightErr == nil && leftCanonical == rightCanonical
}

func reputationInstanceURL(reputation interface{}) string {
	repJSON, err := json.Marshal(reputation)
	if err != nil {
		return ""
	}
	var repMap map[string]interface{}
	if err := json.Unmarshal(repJSON, &repMap); err != nil {
		return ""
	}
	for _, key := range []string{"instance", "instanceURL", "instance_url"} {
		if value, ok := repMap[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func reputationActorHostMatchesInstance(actorID, instanceURL string) bool {
	actorURL, err := url.Parse(strings.TrimSpace(actorID))
	if err != nil || actorURL == nil {
		return false
	}
	instance, err := url.Parse(strings.TrimSpace(instanceURL))
	if err != nil || instance == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(actorURL.Hostname()), strings.TrimSpace(instance.Hostname()))
}

func canonicalReputationActorID(actorID string) (string, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "", fmt.Errorf("actor ID is required")
	}
	if strings.IndexFunc(actorID, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", fmt.Errorf("actor ID contains control characters")
	}
	if err := common.ValidateActivityPubURL(actorID, "actor_id"); err != nil {
		return "", err
	}

	parsed, err := url.Parse(actorID)
	if err != nil || parsed == nil {
		return "", err
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != schemeHTTP && scheme != schemeHTTPS {
		return "", fmt.Errorf("actor ID must use HTTP or HTTPS")
	}
	if parsed.User != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("actor ID must include a bare host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("actor ID must not include query or fragment")
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "" {
		return "", fmt.Errorf("actor ID must include a path")
	}
	if path == "/users" || path == "/@" {
		return "", fmt.Errorf("actor ID must include a concrete actor path")
	}

	return fmt.Sprintf("%s://%s%s", scheme, strings.ToLower(parsed.Host), path), nil
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
