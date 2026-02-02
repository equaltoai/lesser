package moderation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConsensusStorage struct {
	mu sync.Mutex

	events       map[string]*ModerationEvent
	reviewsByEvt map[string][]*Review
	trustByKey   map[string]*models.TrustScore

	addReviewErr   error
	getEventErr    error
	getReviewsErr  error
	createDecErr   error
	getQueueErr    error
	recordTrustErr error

	decisions []*ModerationDecision
	queue     []*QueueItem

	trustUpdates []*models.TrustUpdate
}

func newFakeConsensusStorage() *fakeConsensusStorage {
	return &fakeConsensusStorage{
		events:       make(map[string]*ModerationEvent),
		reviewsByEvt: make(map[string][]*Review),
		trustByKey:   make(map[string]*models.TrustScore),
	}
}

func (f *fakeConsensusStorage) GetModerationEvent(_ context.Context, eventID string) (*ModerationEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getEventErr != nil {
		return nil, f.getEventErr
	}
	event, ok := f.events[eventID]
	if !ok {
		return nil, errors.New("not found")
	}
	return event, nil
}

func (f *fakeConsensusStorage) AddModerationReview(_ context.Context, review *Review) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.addReviewErr != nil {
		return f.addReviewErr
	}
	f.reviewsByEvt[review.EventID] = append(f.reviewsByEvt[review.EventID], review)
	return nil
}

func (f *fakeConsensusStorage) GetModerationReviews(_ context.Context, eventID string) ([]*Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getReviewsErr != nil {
		return nil, f.getReviewsErr
	}
	return append([]*Review(nil), f.reviewsByEvt[eventID]...), nil
}

func (f *fakeConsensusStorage) CreateModerationDecision(_ context.Context, decision *ModerationDecision) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createDecErr != nil {
		return f.createDecErr
	}
	f.decisions = append(f.decisions, decision)
	return nil
}

func (f *fakeConsensusStorage) GetModerationQueue(_ context.Context, _ int, _ string) ([]*QueueItem, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getQueueErr != nil {
		return nil, "", f.getQueueErr
	}
	return append([]*QueueItem(nil), f.queue...), "", nil
}

func (f *fakeConsensusStorage) GetTrustScore(_ context.Context, actorID, category string) (*models.TrustScore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if trust, ok := f.trustByKey[actorID+":"+category]; ok {
		return trust, nil
	}
	return nil, errors.New("missing trust score")
}

func (f *fakeConsensusStorage) RecordTrustUpdate(_ context.Context, update *models.TrustUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.trustUpdates = append(f.trustUpdates, update)
	if f.recordTrustErr != nil {
		return f.recordTrustErr
	}
	return nil
}

func TestDefaultConsensusConfig(t *testing.T) {
	cfg := DefaultConsensusConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, 3, cfg.MinReviewers)
	assert.Equal(t, 0.5, cfg.MinTrustWeight)
	assert.Equal(t, 0.7, cfg.ConsensusThreshold)
	assert.Equal(t, 0.9, cfg.CriticalThreshold)
	assert.Equal(t, 0.8, cfg.EscalationThreshold)
	assert.Equal(t, 24, cfg.ReviewTimeoutHours)
}

func TestConsensusEngine_CalculateConsensus_ErrorsAndSuccess(t *testing.T) {
	store := newFakeConsensusStorage()
	engine := NewConsensusEngine(store, nil)

	event := &ModerationEvent{
		ID:        "e1",
		ObjectID:  "obj1",
		Severity:  SeverityHigh,
		Created:   time.Now(),
		Updated:   time.Now(),
		Category:  CategorySpam,
		EventType: EventTypeFlagged,
	}

	// Insufficient reviewers.
	_, err := engine.CalculateConsensus(context.Background(), event, []*Review{{ReviewerID: "r1", Confidence: 1}})
	require.Error(t, err)

	// Insufficient trust weight.
	reviews := []*Review{
		{ID: "rv1", ReviewerID: "r1", Action: ActionTypeWarning, Confidence: 0.0},
		{ID: "rv2", ReviewerID: "r2", Action: ActionTypeWarning, Confidence: 0.0},
		{ID: "rv3", ReviewerID: "r3", Action: ActionTypeWarning, Confidence: 0.0},
	}
	for _, r := range []string{"r1", "r2", "r3"} {
		store.trustByKey[r+":"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: -1.0, Confidence: 0.1}
	}
	_, err = engine.CalculateConsensus(context.Background(), event, reviews)
	require.Error(t, err)

	// Critical action requires higher threshold.
	reviews = []*Review{
		{ID: "rv1", ReviewerID: "r1", Action: ActionTypeRemove, Confidence: 1.0},
		{ID: "rv2", ReviewerID: "r2", Action: ActionTypeRemove, Confidence: 1.0},
		{ID: "rv3", ReviewerID: "r3", Action: ActionTypeWarning, Confidence: 1.0},
	}
	store.trustByKey["r1:"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: 1.0, Confidence: 1.0}
	store.trustByKey["r2:"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: 1.0, Confidence: 1.0}
	store.trustByKey["r3:"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: 0.2, Confidence: 1.0} // weight 0.6
	_, err = engine.CalculateConsensus(context.Background(), event, reviews)
	require.ErrorIs(t, err, ErrInsufficientConsensus)

	// Non-critical action between consensus and escalation threshold downgrades to warning.
	reviews = []*Review{
		{ID: "rv1", ReviewerID: "r1", Action: ActionTypeSilence, Confidence: 1.0},
		{ID: "rv2", ReviewerID: "r2", Action: ActionTypeSilence, Confidence: 1.0},
		{ID: "rv3", ReviewerID: "r3", Action: ActionTypeWarning, Confidence: 1.0},
	}
	decision, err := engine.CalculateConsensus(context.Background(), event, reviews)
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, ActionTypeWarning, decision.Action)
	assert.GreaterOrEqual(t, decision.ConsensusScore, engine.config.ConsensusThreshold)
	assert.Less(t, decision.ConsensusScore, engine.config.EscalationThreshold)
}

func TestConsensusEngine_ProcessReview_AndUpdateTrust(t *testing.T) {
	store := newFakeConsensusStorage()
	engine := NewConsensusEngine(store, &ConsensusConfig{
		MinReviewers:        2,
		MinTrustWeight:      0.1,
		ConsensusThreshold:  0.5,
		CriticalThreshold:   0.9,
		EscalationThreshold: 0.8,
		ReviewTimeoutHours:  24,
	})

	store.events["e1"] = &ModerationEvent{ID: "e1", ObjectID: "obj", Severity: SeverityHigh, Category: CategorySpam, Created: time.Now()}
	store.trustByKey["r1:"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: 1, Confidence: 1}
	store.trustByKey["r2:"+string(models.TrustCategoryContent)] = &models.TrustScore{Score: 1, Confidence: 1}

	// First review not enough reviewers.
	decision, err := engine.ProcessReview(context.Background(), "e1", &Review{ID: "rv1", EventID: "e1", ReviewerID: "r1", Action: ActionTypeWarning, Confidence: 1})
	require.NoError(t, err)
	assert.Nil(t, decision)

	// Second review reaches consensus.
	decision, err = engine.ProcessReview(context.Background(), "e1", &Review{ID: "rv2", EventID: "e1", ReviewerID: "r2", Action: ActionTypeWarning, Confidence: 1})
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, ActionTypeWarning, decision.Action)

	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.trustUpdates) == 2
	}, time.Second, 10*time.Millisecond)
}

func TestConsensusEngine_CheckTimeouts_AndDefaults(t *testing.T) {
	store := newFakeConsensusStorage()
	engine := NewConsensusEngine(store, &ConsensusConfig{
		MinReviewers:        3,
		MinTrustWeight:      0.5,
		ConsensusThreshold:  0.7,
		CriticalThreshold:   0.9,
		EscalationThreshold: 0.8,
		ReviewTimeoutHours:  1,
	})

	old := time.Now().Add(-2 * time.Hour)
	store.queue = []*QueueItem{
		{Event: &ModerationEvent{ID: "e1", ObjectID: "obj1", Severity: SeverityCritical, Created: old}, ReviewCount: 0},
	}

	require.NoError(t, engine.CheckTimeouts(context.Background()))
	store.mu.Lock()
	require.Len(t, store.decisions, 1)
	assert.Equal(t, ActionTypeSilence, store.decisions[0].Action)
	store.mu.Unlock()

	assert.Equal(t, ActionTypeWarning, engine.getDefaultAction(&ModerationEvent{Severity: SeverityHigh}))
	assert.Equal(t, ActionTypeNone, engine.getDefaultAction(&ModerationEvent{Severity: SeverityLow}))

	stats, err := engine.GetConsensusStats(context.Background(), time.Time{}, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.NotNil(t, stats.ActionBreakdown)
}

func TestConsensusHelpers_generateID_generateRandomString_calculateReviewWeight(t *testing.T) {
	id := generateID("decision")
	assert.True(t, strings.HasPrefix(id, "decision_"))

	s := generateRandomString(6)
	assert.Len(t, s, 6)

	weight := calculateReviewWeight(&models.TrustScore{Score: -1.0, Confidence: 0.0}, &Review{Confidence: 0.0})
	assert.Equal(t, 0.1, weight)
}

func TestConsensusEngine_updateTrustScores_DeltasAndErrors(t *testing.T) {
	store := newFakeConsensusStorage()
	store.recordTrustErr = errors.New("boom")
	engine := NewConsensusEngine(store, nil)

	decision := &ModerationDecision{
		EventID:        "e1",
		Action:         ActionTypeRemove,
		ConsensusScore: 0.8,
	}
	reviews := []*Review{
		{ReviewerID: "r1", Action: ActionTypeRemove},
		{ReviewerID: "r2", Action: ActionTypeWarning},
	}

	engine.updateTrustScores(context.Background(), decision, reviews)

	store.mu.Lock()
	require.Len(t, store.trustUpdates, 2)
	updates := append([]*models.TrustUpdate(nil), store.trustUpdates...)
	store.mu.Unlock()

	// remove doubles deltas
	assert.InDelta(t, 0.016, updates[0].Delta, 0.0001)
	assert.InDelta(t, -0.002, updates[1].Delta, 0.0001)
}
