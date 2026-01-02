package advanced

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeThreatRepo struct {
	shareThreatFn            func(context.Context, *repositories.ThreatIntel) error
	getSharedThreatsFn       func(context.Context, time.Time) ([]*repositories.ThreatIntel, error)
	getThreatsByTypeFn       func(context.Context, string, int) ([]*repositories.ThreatIntel, error)
	updateThreatConfidenceFn func(context.Context, string, float64) error
	incrementHitCountFn      func(context.Context, string) error
	loadActiveThreatsFn      func(context.Context) ([]*repositories.ThreatIntel, error)
	getThreatByIDFn          func(context.Context, string) (*repositories.ThreatIntel, error)
	getIndicatorThreatFn     func(context.Context, string) (string, error)
}

func (f *fakeThreatRepo) ShareThreat(ctx context.Context, threat *repositories.ThreatIntel) error {
	if f.shareThreatFn != nil {
		return f.shareThreatFn(ctx, threat)
	}
	return nil
}

func (f *fakeThreatRepo) GetSharedThreats(ctx context.Context, since time.Time) ([]*repositories.ThreatIntel, error) {
	if f.getSharedThreatsFn != nil {
		return f.getSharedThreatsFn(ctx, since)
	}
	return nil, nil
}

func (f *fakeThreatRepo) GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*repositories.ThreatIntel, error) {
	if f.getThreatsByTypeFn != nil {
		return f.getThreatsByTypeFn(ctx, threatType, limit)
	}
	return nil, nil
}

func (f *fakeThreatRepo) UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error {
	if f.updateThreatConfidenceFn != nil {
		return f.updateThreatConfidenceFn(ctx, threatID, newConfidence)
	}
	return nil
}

func (f *fakeThreatRepo) IncrementHitCount(ctx context.Context, threatID string) error {
	if f.incrementHitCountFn != nil {
		return f.incrementHitCountFn(ctx, threatID)
	}
	return nil
}

func (f *fakeThreatRepo) LoadActiveThreats(ctx context.Context) ([]*repositories.ThreatIntel, error) {
	if f.loadActiveThreatsFn != nil {
		return f.loadActiveThreatsFn(ctx)
	}
	return nil, nil
}

func (f *fakeThreatRepo) GetThreatByID(ctx context.Context, threatID string) (*repositories.ThreatIntel, error) {
	if f.getThreatByIDFn != nil {
		return f.getThreatByIDFn(ctx, threatID)
	}
	return nil, nil
}

func (f *fakeThreatRepo) GetIndicatorThreat(ctx context.Context, indicator string) (string, error) {
	if f.getIndicatorThreatFn != nil {
		return f.getIndicatorThreatFn(ctx, indicator)
	}
	return "", nil
}

func TestThreatIntelligence_ShareThreat_ValidatesDefaultsAndCaches(t *testing.T) {
	ctx := context.Background()

	var stored *repositories.ThreatIntel
	repo := &fakeThreatRepo{
		shareThreatFn: func(_ context.Context, threat *repositories.ThreatIntel) error {
			stored = threat
			return nil
		},
	}

	ti := NewThreatIntelligence(repo, zap.NewNop())

	threat := &ThreatIntel{
		ThreatType:   "malicious_url",
		Indicators:   []string{"malicious.com"},
		Description:  "known bad domain",
		SourceDomain: "example",
	}

	require.NoError(t, ti.ShareThreat(ctx, threat))

	require.NotEmpty(t, threat.ID)
	assert.Equal(t, SeverityMedium, threat.Severity)
	assert.Equal(t, 0.7, threat.Confidence)
	assert.Equal(t, 7*24*time.Hour, threat.TTL)
	assert.False(t, threat.FirstSeen.IsZero())
	assert.False(t, threat.LastSeen.IsZero())

	require.NotNil(t, stored)
	assert.Equal(t, threat.ID, stored.ID)
	assert.Equal(t, threat.ThreatType, stored.ThreatType)
	assert.Equal(t, string(threat.Severity), stored.Severity)
	assert.Equal(t, threat.Indicators, stored.Indicators)
}

func TestThreatIntelligence_ShareThreat_RejectsInvalidThreat(t *testing.T) {
	ti := NewThreatIntelligence(&fakeThreatRepo{}, zap.NewNop())

	err := ti.ShareThreat(context.Background(), &ThreatIntel{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid threat")
}

func TestThreatIntelligence_GetSharedThreats_ConvertsResults(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	repo := &fakeThreatRepo{
		getSharedThreatsFn: func(context.Context, time.Time) ([]*repositories.ThreatIntel, error) {
			return []*repositories.ThreatIntel{
				{
					ID:           "t1",
					ThreatType:   "malware",
					Indicators:   []string{"hash1"},
					Severity:     string(SeverityHigh),
					Description:  "bad file",
					SourceDomain: "example",
					FirstSeen:    now,
					LastSeen:     now,
					Confidence:   0.9,
					TTL:          24 * time.Hour,
				},
			}, nil
		},
	}

	ti := NewThreatIntelligence(repo, zap.NewNop())
	threats, err := ti.GetSharedThreats(context.Background(), time.Now().Add(-2*time.Hour))
	require.NoError(t, err)
	require.Len(t, threats, 1)
	assert.Equal(t, "t1", threats[0].ID)
	assert.Equal(t, SeverityHigh, threats[0].Severity)
}

func TestThreatIntelligence_CheckContent_MatchesIndicatorsURLAndHash(t *testing.T) {
	ctx := context.Background()

	hitCh := make(chan string, 1)
	repo := &fakeThreatRepo{
		getIndicatorThreatFn: func(context.Context, string) (string, error) {
			return "hash-threat", nil
		},
		incrementHitCountFn: func(context.Context, string) error {
			hitCh <- "hit"
			return nil
		},
	}

	ti := NewThreatIntelligence(repo, zap.NewNop())
	ti.threatCache.Store("t1", &ThreatIntel{
		ID:         "t1",
		ThreatType: "keyword",
		Indicators: []string{"scam"},
		Confidence: 0.8,
	})

	matches, err := ti.CheckContent(ctx, "this is a scam", ContentMetadata{
		URLs:        []string{"https://malicious.com/path"},
		Hashtags:    []string{"Scam"},
		ContentType: ContentTypeImage,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(matches), 3)

	select {
	case <-hitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hit count increment")
	}
}

func TestThreatIntelligence_checkHashThreat_WarnsAndReturnsEmptyOnError(t *testing.T) {
	repo := &fakeThreatRepo{
		getIndicatorThreatFn: func(context.Context, string) (string, error) {
			return "", errors.New("db down")
		},
	}

	ti := NewThreatIntelligence(repo, zap.NewNop())
	assert.Empty(t, ti.checkHashThreat("hash"))
}

func TestThreatIntelligence_RefreshThreats_LoadsAndReportsErrors(t *testing.T) {
	ctx := context.Background()

	repoErr := &fakeThreatRepo{
		loadActiveThreatsFn: func(context.Context) ([]*repositories.ThreatIntel, error) {
			return nil, errors.New("boom")
		},
	}
	tiErr := NewThreatIntelligence(repoErr, zap.NewNop())
	require.Error(t, tiErr.RefreshThreats(ctx))

	repoOK := &fakeThreatRepo{
		loadActiveThreatsFn: func(context.Context) ([]*repositories.ThreatIntel, error) {
			return []*repositories.ThreatIntel{{ID: "t2", ThreatType: "x", Severity: string(SeverityLow)}}, nil
		},
	}
	tiOK := NewThreatIntelligence(repoOK, zap.NewNop())
	require.NoError(t, tiOK.RefreshThreats(ctx))
}

func TestThreatIntelligence_generateThreatID_IsStableAndShort(t *testing.T) {
	ti := NewThreatIntelligence(&fakeThreatRepo{}, zap.NewNop())
	threat := &ThreatIntel{ThreatType: "malware", Indicators: []string{"a", "b"}}

	id1 := ti.generateThreatID(threat)
	id2 := ti.generateThreatID(threat)
	assert.Len(t, id1, 16)
	assert.Equal(t, id1, id2)
}

func TestThreatIntelligence_UpdateThreatConfidence_DelegatesToRepo(t *testing.T) {
	var (
		mu       sync.Mutex
		calledID string
		value    float64
	)
	repo := &fakeThreatRepo{
		updateThreatConfidenceFn: func(_ context.Context, threatID string, newConfidence float64) error {
			mu.Lock()
			defer mu.Unlock()
			calledID = threatID
			value = newConfidence
			return nil
		},
	}

	ti := NewThreatIntelligence(repo, zap.NewNop())
	require.NoError(t, ti.UpdateThreatConfidence(context.Background(), "t1", 0.42))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "t1", calledID)
	assert.Equal(t, 0.42, value)
}
