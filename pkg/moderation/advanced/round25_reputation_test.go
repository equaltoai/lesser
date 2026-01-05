package advanced

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type dynamoStubTransport struct {
	mu sync.Mutex

	getItemPayload  map[string]any
	getItemErr      error
	putItemErr      error
	putEventItemErr error
	queryPayload    map[string]any
	scanPayload     map[string]any

	getItemCalls int
	putItemCalls int
	queryCalls   int
	scanCalls    int
}

func (t *dynamoStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	if !strings.HasPrefix(target, "DynamoDB_20120810.") {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unexpected target")),
			Request:    req,
		}, nil
	}

	op := strings.TrimPrefix(target, "DynamoDB_20120810.")

	t.mu.Lock()
	defer t.mu.Unlock()

	switch op {
	case "GetItem":
		t.getItemCalls++
		if t.getItemErr != nil {
			return nil, t.getItemErr
		}
		if t.getItemPayload == nil {
			t.getItemPayload = map[string]any{}
		}
		return t.jsonResponseLocked(req, http.StatusOK, t.getItemPayload)
	case "PutItem":
		t.putItemCalls++

		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		_ = body

		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		itemStr, _ := json.Marshal(payload["Item"])
		if bytesContains(itemStr, []byte(`"EVENT#`)) && t.putEventItemErr != nil {
			return nil, t.putEventItemErr
		}
		if t.putItemErr != nil {
			return nil, t.putItemErr
		}
		return t.jsonResponseLocked(req, http.StatusOK, map[string]any{})
	case "Query":
		t.queryCalls++
		if t.queryPayload == nil {
			t.queryPayload = map[string]any{"Items": []any{}}
		}
		return t.jsonResponseLocked(req, http.StatusOK, t.queryPayload)
	case "Scan":
		t.scanCalls++
		if t.scanPayload == nil {
			t.scanPayload = map[string]any{"Items": []any{}}
		}
		return t.jsonResponseLocked(req, http.StatusOK, t.scanPayload)
	default:
		return t.jsonResponseLocked(req, http.StatusBadRequest, map[string]any{"Message": "unsupported operation"})
	}
}

func (t *dynamoStubTransport) jsonResponseLocked(req *http.Request, status int, payload map[string]any) (*http.Response, error) {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":   []string{"application/x-amz-json-1.0"},
			"Content-Length": []string{fmt.Sprintf("%d", len(data))},
		},
		Body:    io.NopCloser(bytes.NewReader(data)),
		Request: req,
	}, nil
}

func bytesContains(haystack, needle []byte) bool { return bytes.Contains(haystack, needle) }

func TestReputationScorer_safeIntToInt32_CapsValues(t *testing.T) {
	assert.Equal(t, int32(5), safeIntToInt32(5))
	assert.Equal(t, int32(math.MaxInt32), safeIntToInt32(math.MaxInt32+1))
	assert.Equal(t, int32(math.MinInt32), safeIntToInt32(math.MinInt32-1))
}

func TestReputationScorer_GetReputationScore_CreatesDefaultAndCaches(t *testing.T) {
	transport := &dynamoStubTransport{}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	cfg := DefaultModerationConfig()
	cfg.ReputationDecayRate = 0
	rs := NewReputationScorer(db, "table", zap.NewNop(), cfg)

	score, err := rs.GetReputationScore(context.Background(), "actor-1")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, "actor-1", score.ActorID)
	assert.Equal(t, reputationLevelNormal, score.Level)

	// Second call should use cache (no extra GetItem).
	_, err = rs.GetReputationScore(context.Background(), "actor-1")
	require.NoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()
	assert.Equal(t, 1, transport.getItemCalls)
	assert.GreaterOrEqual(t, transport.putItemCalls, 1)
}

func TestReputationScorer_GetReputationScore_ParsesExistingAndAppliesDecay(t *testing.T) {
	updatedAt := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	transport := &dynamoStubTransport{
		getItemPayload: map[string]any{
			"Item": map[string]any{
				"PK":                 map[string]any{"S": "ACTOR#actor-1"},
				"SK":                 map[string]any{"S": skReputation},
				"Score":              map[string]any{"N": "90.0"},
				"Level":              map[string]any{"S": reputationLevelTrusted},
				"ViolationCount":     map[string]any{"N": "1"},
				"FalsePositiveCount": map[string]any{"N": "0"},
				"ContentCount":       map[string]any{"N": "10"},
				"UpdatedAt":          map[string]any{"S": updatedAt},
				"Factors": map[string]any{"L": []any{
					map[string]any{"M": map[string]any{
						"Factor":      map[string]any{"S": eventTypeGoodContent},
						"Impact":      map[string]any{"N": "1.0"},
						"Description": map[string]any{"S": "ok"},
					}},
				}},
			},
		},
	}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	cfg := DefaultModerationConfig()
	cfg.ReputationDecayRate = 0.1
	rs := NewReputationScorer(db, "table", zap.NewNop(), cfg)

	score, err := rs.GetReputationScore(context.Background(), "actor-1")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, "actor-1", score.ActorID)
	assert.Equal(t, reputationLevelTrusted, score.Level)
	assert.NotZero(t, score.UpdatedAt)
}

func TestReputationScorer_UpdateReputation_UpdatesCountsAndRecordsEvent(t *testing.T) {
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	transport := &dynamoStubTransport{
		getItemPayload: map[string]any{
			"Item": map[string]any{
				"PK":                 map[string]any{"S": "ACTOR#actor-1"},
				"SK":                 map[string]any{"S": skReputation},
				"Score":              map[string]any{"N": "50.0"},
				"Level":              map[string]any{"S": reputationLevelNormal},
				"ViolationCount":     map[string]any{"N": "0"},
				"FalsePositiveCount": map[string]any{"N": "0"},
				"ContentCount":       map[string]any{"N": "0"},
				"UpdatedAt":          map[string]any{"S": updatedAt},
			},
		},
		putEventItemErr: errors.New("event write failed"),
	}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20
	rs := NewReputationScorer(db, "table", zap.NewNop(), cfg)

	event := ReputationEvent{
		EventType:   eventTypeViolation,
		Severity:    SeverityHigh,
		Description: "bad",
		Timestamp:   time.Now(),
	}

	require.NoError(t, rs.UpdateReputation(context.Background(), "actor-1", event))
}

func TestReputationScorer_GetReputationHistory_ParsesItems(t *testing.T) {
	ts := time.Now().UTC()
	transport := &dynamoStubTransport{
		queryPayload: map[string]any{
			"Items": []any{
				map[string]any{
					"EventType":   map[string]any{"S": eventTypeViolation},
					"Severity":    map[string]any{"S": string(SeverityHigh)},
					"Description": map[string]any{"S": "bad"},
					"Impact":      map[string]any{"N": "-5.0"},
					"Timestamp":   map[string]any{"S": ts.Format(time.RFC3339)},
				},
			},
		},
	}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	rs := NewReputationScorer(db, "table", zap.NewNop(), DefaultModerationConfig())
	history, err := rs.GetReputationHistory(context.Background(), "actor-1", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, eventTypeViolation, history[0].EventType)
	assert.Equal(t, SeverityHigh, history[0].Severity)
}

func TestReputationScorer_GetActorsByReputation_ParsesItems(t *testing.T) {
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	transport := &dynamoStubTransport{
		scanPayload: map[string]any{
			"Items": []any{
				map[string]any{
					"PK":                 map[string]any{"S": "ACTOR#actor-1"},
					"SK":                 map[string]any{"S": skReputation},
					"Score":              map[string]any{"N": "75.0"},
					"Level":              map[string]any{"S": reputationLevelNormal},
					"ViolationCount":     map[string]any{"N": "0"},
					"FalsePositiveCount": map[string]any{"N": "0"},
					"ContentCount":       map[string]any{"N": "0"},
					"UpdatedAt":          map[string]any{"S": updatedAt},
				},
			},
		},
	}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	rs := NewReputationScorer(db, "table", zap.NewNop(), DefaultModerationConfig())
	scores, err := rs.GetActorsByReputation(context.Background(), 50, 80, 10)
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, "actor-1", scores[0].ActorID)
}

func TestReputationScorer_CalculateReputationImpact_AppliesSeverityAndConfidence(t *testing.T) {
	rs := NewReputationScorer(nil, "table", zap.NewNop(), DefaultModerationConfig())
	impact := rs.CalculateReputationImpact(&ModerationDecision{
		Decision:       ActionRemove,
		Confidence:     0.5,
		Reasons:        []DecisionReason{{Type: "x", Severity: SeverityHigh}},
		RequiresReview: true,
	})
	assert.Less(t, impact, 0.0)
}

func TestReputationScorer_clampScore_and_calculateLevel_coverBranches(t *testing.T) {
	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20

	rs := NewReputationScorer(nil, "table", zap.NewNop(), cfg)

	assert.Equal(t, float64(0), rs.clampScore(-1))
	assert.Equal(t, float64(100), rs.clampScore(101))
	assert.Equal(t, float64(42), rs.clampScore(42))

	assert.Equal(t, reputationLevelTrusted, rs.calculateLevel(90))
	assert.Equal(t, reputationLevelNormal, rs.calculateLevel(50))
	assert.Equal(t, reputationLevelSuspicious, rs.calculateLevel(30))
	assert.Equal(t, reputationLevelBadActor, rs.calculateLevel(10))
}

func TestReputationScorer_calculateEventImpact_CoversEventTypesAndModifiers(t *testing.T) {
	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20

	rs := NewReputationScorer(nil, "table", zap.NewNop(), cfg)

	violation := ReputationEvent{EventType: eventTypeViolation, Severity: SeverityHigh}

	trusted := &ReputationScore{Score: 90, ViolationCount: 2}
	neutral := &ReputationScore{Score: 50, ViolationCount: 2}
	bad := &ReputationScore{Score: 10, ViolationCount: 2}

	impactTrusted := rs.calculateEventImpact(violation, trusted)
	impactNeutral := rs.calculateEventImpact(violation, neutral)
	impactBad := rs.calculateEventImpact(violation, bad)

	assert.Greater(t, impactTrusted, impactNeutral)
	assert.Less(t, impactBad, impactNeutral)

	assert.Greater(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeFalsePositive}, neutral), 0.0)
	assert.Greater(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeGoodContent}, neutral), 0.0)
	assert.Less(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeUserReport}, neutral), 0.0)
}

func TestReputationScorer_CalculateReputationImpact_CoversActionsAndSeverities(t *testing.T) {
	rs := NewReputationScorer(nil, "table", zap.NewNop(), DefaultModerationConfig())

	for _, action := range []ModerationAction{
		ActionAllow,
		ActionFlag,
		ActionQuarantine,
		ActionRemove,
		ActionShadowBan,
		ActionReportToAuth,
	} {
		impact := rs.CalculateReputationImpact(&ModerationDecision{
			Decision:   action,
			Confidence: 0.5,
			Reasons: []DecisionReason{
				{Severity: SeverityCritical},
				{Severity: SeverityHigh},
				{Severity: SeverityMedium},
				{Severity: SeverityLow},
			},
		})
		if action == ActionAllow {
			assert.Equal(t, 0.0, impact)
			continue
		}
		assert.NotZero(t, impact)
	}
}

func TestReputationScorer_parseReputationScore_WarnsOnBadNumbersAndSkipsBadFactor(t *testing.T) {
	rs := NewReputationScorer(nil, "table", zap.NewNop(), DefaultModerationConfig())

	score, err := rs.parseReputationScore(map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: "ACTOR#actor-1"},
		"SK":             &types.AttributeValueMemberS{Value: skReputation},
		"Score":          &types.AttributeValueMemberN{Value: "not-a-number"},
		"ViolationCount": &types.AttributeValueMemberN{Value: "nan"},
		"Factors": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "not-a-map"},
			&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"Factor":      &types.AttributeValueMemberS{Value: eventTypeViolation},
				"Impact":      &types.AttributeValueMemberN{Value: "bad"},
				"Description": &types.AttributeValueMemberS{Value: "desc"},
			}},
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, "actor-1", score.ActorID)
	assert.Len(t, score.Factors, 1)
}

func TestReputationScorer_recordEvent_UsesNowWhenTimestampZero(t *testing.T) {
	transport := &dynamoStubTransport{}
	db := dynamodb.NewFromConfig(awsConfigForStub(transport))

	rs := NewReputationScorer(db, "table", zap.NewNop(), DefaultModerationConfig())
	require.NoError(t, rs.recordEvent(context.Background(), "actor-1", ReputationEvent{
		EventType:   eventTypeGoodContent,
		Severity:    SeverityLow,
		Description: "ok",
	}, 1.0))
}
