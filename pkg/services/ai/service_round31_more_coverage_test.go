package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAIRepository struct {
	saveErr   error
	getErr    error
	queueErr  error
	statsErr  error
	byObject  map[string]*ai.AIAnalysis
	stats     *ai.AIStats
	saved     []*ai.AIAnalysis
	queued    []string
	statsArgs []string
}

func (r *fakeAIRepository) SaveAnalysis(ctx context.Context, analysis *ai.AIAnalysis) error {
	r.saved = append(r.saved, analysis)
	return r.saveErr
}

func (r *fakeAIRepository) GetAnalysis(ctx context.Context, objectID string) (*ai.AIAnalysis, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.byObject == nil {
		return nil, nil
	}
	return r.byObject[objectID], nil
}

func (r *fakeAIRepository) QueueForAnalysis(ctx context.Context, objectID string) error {
	r.queued = append(r.queued, objectID)
	return r.queueErr
}

func (r *fakeAIRepository) GetStats(ctx context.Context, period string) (*ai.AIStats, error) {
	r.statsArgs = append(r.statsArgs, period)
	if r.statsErr != nil {
		return nil, r.statsErr
	}
	if r.stats != nil {
		return r.stats, nil
	}
	return &ai.AIStats{Period: period}, nil
}

type publishCall struct {
	target string
	event  *streaming.Event
}

type fakePublisher struct {
	streamErr error
	userErr   error

	streamCalls []publishCall
	userCalls   []publishCall
}

func (p *fakePublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	p.userCalls = append(p.userCalls, publishCall{target: userID, event: event})
	return p.userErr
}

func (p *fakePublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	p.streamCalls = append(p.streamCalls, publishCall{target: streamName, event: event})
	return p.streamErr
}

func (p *fakePublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	return nil
}

func (p *fakePublisher) Close() error { return nil }

func TestService_SaveAnalysis_ReturnsErrorWhenCommandNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	_, err := svc.SaveAnalysis(context.Background(), nil)
	require.Error(t, err)
}

func TestService_SaveAnalysis_ReturnsErrorWhenRepoNotConfigured(t *testing.T) {
	t.Parallel()

	svc := &Service{logger: zap.NewNop()}
	_, err := svc.SaveAnalysis(context.Background(), &SaveAnalysisCommand{
		Analysis: &ai.AIAnalysis{ID: "analysis-1", ObjectID: "obj-1"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "repository")
}

func TestService_SaveAnalysis_SavesAndPublishesBestEffort(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{}
	publisher := &fakePublisher{
		streamErr: errors.New("stream publish failed"),
		userErr:   errors.New("user publish failed"),
	}

	analyzedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	analysis := &ai.AIAnalysis{
		ID:               "analysis-1",
		ObjectID:         "obj-1",
		ObjectType:       "status",
		ModerationAction: ai.ActionRemove,
		OverallRisk:      0.9,
		Confidence:       0.7,
		AnalyzedAt:       analyzedAt,
		Version:          "v1",
		SpamAnalysis:     &ai.SpamAnalysis{SpamScore: 0.1},
	}

	svc := &Service{
		publisher: publisher,
		logger:    zap.NewNop(),
		aiRepo:    repo,
	}

	result, err := svc.SaveAnalysis(context.Background(), &SaveAnalysisCommand{
		Analysis: analysis,
		UserID:   "user-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Len(t, result.Events, 1)
	require.Same(t, result.Events[0], publisher.streamCalls[0].event)

	require.Len(t, repo.saved, 1)
	require.Same(t, analysis, repo.saved[0])

	require.Len(t, publisher.streamCalls, 1)
	require.Equal(t, "ai_analysis", publisher.streamCalls[0].target)
	require.Equal(t, "ai_analysis_urgent", publisher.streamCalls[0].event.Stream)

	require.Len(t, publisher.userCalls, 1)
	require.Equal(t, "user-1", publisher.userCalls[0].target)
}

func TestService_SaveAnalysis_ReturnsJoinedErrorWhenSaveFails(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{saveErr: errors.New("save failed")}
	svc := &Service{
		logger: zap.NewNop(),
		aiRepo: repo,
	}

	_, err := svc.SaveAnalysis(context.Background(), &SaveAnalysisCommand{
		Analysis: &ai.AIAnalysis{ID: "analysis-1", ObjectID: "obj-1"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSaveAnalysis)
}

func TestService_GetAnalysis_ReturnsErrorWhenQueryNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	_, err := svc.GetAnalysis(context.Background(), nil)
	require.Error(t, err)
}

func TestService_GetAnalysis_ReturnsErrorWhenRepoNotConfigured(t *testing.T) {
	t.Parallel()

	svc := &Service{logger: zap.NewNop()}
	_, err := svc.GetAnalysis(context.Background(), &GetAnalysisQuery{ObjectID: "obj-1"})
	require.Error(t, err)
}

func TestService_GetAnalysis_ReturnsAnalysisOnSuccess(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{
		byObject: map[string]*ai.AIAnalysis{
			"obj-1": {ID: "analysis-1", ObjectID: "obj-1"},
		},
	}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	out, err := svc.GetAnalysis(context.Background(), &GetAnalysisQuery{ObjectID: "obj-1"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Analysis)
	require.Equal(t, "analysis-1", out.Analysis.ID)
}

func TestService_GetAnalysis_ReturnsErrorWhenRepoFails(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{getErr: errors.New("get failed")}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	_, err := svc.GetAnalysis(context.Background(), &GetAnalysisQuery{ObjectID: "obj-1"})
	require.Error(t, err)
}

func TestService_QueueForAnalysis_ReturnsErrorWhenCommandNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	_, err := svc.QueueForAnalysis(context.Background(), nil)
	require.Error(t, err)
}

func TestService_QueueForAnalysis_SkipsWhenRecentAndNotForced(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{
		byObject: map[string]*ai.AIAnalysis{
			"obj-1": {ID: "analysis-1", ObjectID: "obj-1", AnalyzedAt: time.Now()},
		},
	}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	out, err := svc.QueueForAnalysis(context.Background(), &QueueAnalysisCommand{ObjectID: "obj-1"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, out.Queued)
	require.Empty(t, repo.queued)
}

func TestService_QueueForAnalysis_QueuesWhenForced(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{
		byObject: map[string]*ai.AIAnalysis{
			"obj-1": {ID: "analysis-1", ObjectID: "obj-1", AnalyzedAt: time.Now()},
		},
	}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	out, err := svc.QueueForAnalysis(context.Background(), &QueueAnalysisCommand{ObjectID: "obj-1", Force: true})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Queued)
	require.Equal(t, []string{"obj-1"}, repo.queued)
}

func TestService_QueueForAnalysis_ReturnsErrorWhenQueueFails(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{queueErr: errors.New("queue failed")}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	_, err := svc.QueueForAnalysis(context.Background(), &QueueAnalysisCommand{ObjectID: "obj-1", Force: true})
	require.Error(t, err)
}

func TestService_GetStats_ReturnsErrorWhenQueryNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	_, err := svc.GetStats(context.Background(), nil)
	require.Error(t, err)
}

func TestService_GetStats_DefaultsToDayAndCallsRepo(t *testing.T) {
	t.Parallel()

	repo := &fakeAIRepository{}
	svc := &Service{logger: zap.NewNop(), aiRepo: repo}

	out, err := svc.GetStats(context.Background(), &GetStatsQuery{Period: ""})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, []string{"day"}, repo.statsArgs)
}

func TestService_SubscribeToAnalysisEvents_ReturnsClosedChannel(t *testing.T) {
	t.Parallel()

	svc := &Service{logger: zap.NewNop()}
	ch, err := svc.SubscribeToAnalysisEvents(context.Background(), "user-1", nil)
	require.NoError(t, err)

	select {
	case <-ch:
		// channel should be closed immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected channel to be closed")
	}
}

func TestService_ConvertToModel_SetsKeys(t *testing.T) {
	t.Parallel()

	analyzedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	analysis := &ai.AIAnalysis{
		ID:         "analysis-1",
		ObjectID:   "obj-1",
		ObjectType: "status",
		AnalyzedAt: analyzedAt,
		Version:    "v1",
	}

	svc := &Service{logger: zap.NewNop()}
	model := svc.ConvertToModel(analysis)
	require.NotNil(t, model)
	require.Equal(t, "AI#obj-1", model.PK)
	require.Equal(t, "ANALYSIS#analysis-1", model.SK)
	require.Equal(t, "AIAnalysis", model.Type)
}
