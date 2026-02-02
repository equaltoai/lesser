package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	comprehendTypes "github.com/aws/aws-sdk-go-v2/service/comprehend/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/ai"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	"go.uber.org/zap"
)

type fakeComprehendClient struct {
	detectSentiment func(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error)
	detectPII       func(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error)
}

func (f *fakeComprehendClient) DetectSentiment(ctx context.Context, params *comprehend.DetectSentimentInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
	if f.detectSentiment != nil {
		return f.detectSentiment(ctx, params, optFns...)
	}
	return &comprehend.DetectSentimentOutput{}, nil
}

func (f *fakeComprehendClient) DetectPiiEntities(ctx context.Context, params *comprehend.DetectPiiEntitiesInput, optFns ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
	if f.detectPII != nil {
		return f.detectPII(ctx, params, optFns...)
	}
	return &comprehend.DetectPiiEntitiesOutput{}, nil
}

type fakeBedrockClient struct {
	resp *ai.ReputationAnalysisResponse
	err  error
	mu   sync.Mutex
	req  ai.ReputationAnalysisRequest
}

func (f *fakeBedrockClient) AnalyzeReputation(_ context.Context, req ai.ReputationAnalysisRequest) (*ai.ReputationAnalysisResponse, error) {
	f.mu.Lock()
	f.req = req
	f.mu.Unlock()
	return f.resp, f.err
}

type recordingStreamerClient struct {
	mu        sync.Mutex
	postedTo  []string
	payloads  [][]byte
	errByConn map[string]error
}

func (r *recordingStreamerClient) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	r.mu.Lock()
	r.postedTo = append(r.postedTo, connectionID)
	r.payloads = append(r.payloads, append([]byte(nil), data...))
	err := r.errByConn[connectionID]
	r.mu.Unlock()
	return err
}

func (r *recordingStreamerClient) DeleteConnection(_ context.Context, _ string) error { return nil }
func (r *recordingStreamerClient) GetConnection(_ context.Context, _ string) (streamer.Connection, error) {
	return streamer.Connection{}, nil
}

type fakeWebSocketSubRepo struct {
	subsByType map[string][]models.WebSocketEventSubscription
	disconnect chan string
}

func (f *fakeWebSocketSubRepo) GetSubscriptionsForType(_ context.Context, subscriptionType string) ([]models.WebSocketEventSubscription, error) {
	if f.subsByType == nil {
		return nil, nil
	}
	return f.subsByType[subscriptionType], nil
}

func (f *fakeWebSocketSubRepo) HandleDisconnect(_ context.Context, connectionID string) error {
	if f.disconnect != nil {
		f.disconnect <- connectionID
	}
	return nil
}

func TestNoteProcessor_ReputationScoring_Factors(t *testing.T) {
	now := time.Now()

	userRepo := testingmocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, "trusted").Return(&storage.User{CreatedAt: now.AddDate(-2, 0, 0)}, nil)
	userRepo.On("GetUser", mock.Anything, "new").Return(&storage.User{CreatedAt: now.AddDate(0, 0, -3)}, nil)
	userRepo.On("GetUser", mock.Anything, "missing").Return((*storage.User)(nil), errors.New("nope"))

	relRepo := testingmocks.NewMockRelationshipRepository()
	relRepo.On("GetFollowerCount", mock.Anything, "low").Return(int64(5), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "low").Return(int64(1), nil)
	relRepo.On("GetFollowerCount", mock.Anything, "ratio").Return(int64(100), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "ratio").Return(int64(10), nil)

	actRepo := testingmocks.NewMockActivityRepository()
	// 100 posts in last 30 days => >3/day => 1.0
	var acts []*activitypub.Activity
	for i := 0; i < 100; i++ {
		ts := now.AddDate(0, 0, -1)
		acts = append(acts, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create", Published: &ts},
		})
	}
	actRepo.On("GetOutboxActivities", mock.Anything, "active", 1000, "").Return(acts, "", nil)
	actRepo.On("GetOutboxActivities", mock.Anything, "err", 1000, "").Return(([]*activitypub.Activity)(nil), "", errors.New("boom"))

	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("GetUserVotingHistory", mock.Anything, "voter", mock.Anything).Return([]*storage.CommunityNoteVote{
		{NoteID: "n1", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "n2", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "n3", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "n4", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "n5", Helpful: true, VoteType: "helpful", Weight: 1},
	}, nil)
	for _, id := range []string{"n1", "n2", "n3", "n4", "n5"} {
		noteRepo.On("GetCommunityNote", mock.Anything, id).Return(&storage.CommunityNote{ID: id, Status: "accepted", Score: 1.0}, nil)
	}

	modRepo := testingmocks.NewMockModerationRepository()
	modRepo.On("GetModerationEventsByObject", mock.Anything, "clean", 100, "").Return([]*storage.ModerationEvent{}, "", nil)

	np := &NoteProcessor{
		logger:            zap.NewNop(),
		userRepo:          userRepo,
		relationshipRepo:  relRepo,
		activityRepo:      actRepo,
		communityNoteRepo: noteRepo,
		moderationRepo:    modRepo,
	}

	require.Equal(t, 1.0, np.calculateAccountAgeScore(context.Background(), "trusted"))
	require.Equal(t, 0.0, np.calculateAccountAgeScore(context.Background(), "missing"))
	require.Equal(t, 0.0, np.calculateAccountAgeScore(context.Background(), "new"))

	require.Equal(t, 0.0, np.calculateSocialScore(context.Background(), "low"))
	require.Equal(t, 1.0, np.calculateSocialScore(context.Background(), "ratio"))

	require.Equal(t, 1.0, np.calculateActivityScore(context.Background(), "active"))
	require.Equal(t, 0.0, np.calculateActivityScore(context.Background(), "err"))

	require.Equal(t, 1.0, np.calculateVotingHistoryScore(context.Background(), "voter"))
	require.Equal(t, 0.0, np.calculateModerationPenalty(context.Background(), "clean"))
}

func TestNoteProcessor_AnalyzeContentWithCostTracking(t *testing.T) {
	np := &NoteProcessor{logger: zap.NewNop()}
	_, err := np.analyzeContentWithCostTracking(context.Background(), &storage.CommunityNote{Content: "x"})
	require.Error(t, err)

	positive := float32(0.6)
	neutral := float32(0.4)
	negative := float32(0.0)
	fake := &fakeComprehendClient{
		detectSentiment: func(_ context.Context, in *comprehend.DetectSentimentInput, _ ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			require.NotNil(t, in.Text)
			return &comprehend.DetectSentimentOutput{
				SentimentScore: &comprehendTypes.SentimentScore{
					Positive: &positive,
					Neutral:  &neutral,
					Negative: &negative,
				},
			}, nil
		},
		detectPII: func(_ context.Context, _ *comprehend.DetectPiiEntitiesInput, _ ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{
				Entities: []comprehendTypes.PiiEntity{{Type: comprehendTypes.PiiEntityTypeEmail}},
			}, nil
		},
	}

	np = &NoteProcessor{logger: zap.NewNop(), comprehendClient: fake}
	analysis, err := np.analyzeContentWithCostTracking(context.Background(), &storage.CommunityNote{
		Content:  "hello",
		Language: "en",
	})
	require.NoError(t, err)
	require.True(t, analysis.Sentiment > 0)
	require.True(t, analysis.Objectivity >= 0 && analysis.Objectivity <= 1)
	require.True(t, analysis.HasPII)
}

func TestNoteProcessor_UpdateAndRecalculateScore(t *testing.T) {
	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("UpdateCommunityNoteAnalysis", mock.Anything, "n1", 0.1, 0.2, 0.3).Return(nil)
	noteRepo.On("UpdateCommunityNoteAnalysis", mock.Anything, "bad", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("nope"))

	note := &storage.CommunityNote{ID: "n1"}
	np := &NoteProcessor{logger: zap.NewNop(), communityNoteRepo: noteRepo}
	require.NoError(t, np.updateNoteAnalysis(context.Background(), note, &Analysis{Sentiment: 0.1, Objectivity: 0.2}, 0.3))
	require.Error(t, np.updateNoteAnalysis(context.Background(), &storage.CommunityNote{ID: "bad"}, &Analysis{Sentiment: 0.1, Objectivity: 0.2}, 0.3))

	noteRepo2 := testingmocks.NewMockCommunityNoteRepository()
	noteRepo2.On("GetCommunityNote", mock.Anything, "n2").Return(&storage.CommunityNote{ID: "n2", Score: 0.2}, nil)
	noteRepo2.On("GetCommunityNoteVotes", mock.Anything, "n2").Return([]*storage.CommunityNoteVote{
		{VoteType: "helpful", Helpful: true, Weight: 1},
		{VoteType: "not_helpful", Helpful: false, Weight: 1},
	}, nil)
	noteRepo2.On("UpdateCommunityNoteScore", mock.Anything, "n2", mock.MatchedBy(func(v float64) bool {
		return math.Abs(v-0.38) < 0.01
	}), visibilityDisputed).Return(nil)

	np2 := &NoteProcessor{logger: zap.NewNop(), communityNoteRepo: noteRepo2}
	require.NoError(t, np2.recalculateNoteScore(context.Background(), "n2"))
}

func TestNoteProcessor_BroadcastNoteUpdate(t *testing.T) {
	np := &NoteProcessor{logger: zap.NewNop()}
	np.broadcastNoteUpdate(context.Background(), &storage.CommunityNote{ID: "n1"})

	np = &NoteProcessor{logger: zap.NewNop(), wsClient: &fakeStreamerClient{}}
	np.broadcastNoteUpdate(context.Background(), &storage.CommunityNote{ID: "n1"})

	disconnects := make(chan string, 1)
	wsRepo := &fakeWebSocketSubRepo{
		subsByType: map[string][]models.WebSocketEventSubscription{
			"timeline":        {{ConnectionID: "c1"}, {ConnectionID: "c2"}},
			"notifications":   {{ConnectionID: "c2"}},
			"community_notes": {{ConnectionID: "c3"}},
		},
		disconnect: disconnects,
	}

	stream := &recordingStreamerClient{
		errByConn: map[string]error{
			"c2": errors.New("gone"),
		},
	}

	np = &NoteProcessor{logger: zap.NewNop(), wsClient: stream, wsRepo: wsRepo}
	np.broadcastNoteUpdate(context.Background(), &storage.CommunityNote{ID: "n1", ObjectID: "o1", CreatedAt: time.Now()})

	select {
	case connID := <-disconnects:
		require.Equal(t, "c2", connID)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected stale connection cleanup")
	}

	require.Len(t, stream.postedTo, 3)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(stream.payloads[0], &envelope))
	require.Equal(t, "community_note_update", envelope["type"])
}

func TestNoteProcessor_ProcessNewNoteByID_AndProcessRecord(t *testing.T) {
	note := &storage.CommunityNote{
		ID:         "n1",
		ObjectID:   "o1",
		ObjectType: "status",
		AuthorID:   "https://example.com/actors/anon",
		Content:    "hello",
		Language:   "en",
		Sources:    []string{"https://wikipedia.org/wiki/X"},
		CreatedAt:  time.Now(),
	}

	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("GetCommunityNote", mock.Anything, "n1").Return(note, nil)
	noteRepo.On("UpdateCommunityNoteAnalysis", mock.Anything, "n1", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	noteRepo.On("UpdateCommunityNoteScore", mock.Anything, "n1", mock.Anything, mock.Anything).Return(nil)

	actRepo := testingmocks.NewMockActivityRepository()
	actRepo.On("CreateActivity", mock.Anything, mock.MatchedBy(func(a *activitypub.Activity) bool {
		return a != nil && a.Type == activitypub.CreateType && a.Actor == note.AuthorID
	})).Return(nil)

	positive := float32(1.0)
	neutral := float32(1.0)
	negative := float32(0.0)
	comprehendClient := &fakeComprehendClient{
		detectSentiment: func(_ context.Context, _ *comprehend.DetectSentimentInput, _ ...func(*comprehend.Options)) (*comprehend.DetectSentimentOutput, error) {
			return &comprehend.DetectSentimentOutput{
				SentimentScore: &comprehendTypes.SentimentScore{
					Positive: &positive,
					Neutral:  &neutral,
					Negative: &negative,
				},
			}, nil
		},
		detectPII: func(_ context.Context, _ *comprehend.DetectPiiEntitiesInput, _ ...func(*comprehend.Options)) (*comprehend.DetectPiiEntitiesOutput, error) {
			return &comprehend.DetectPiiEntitiesOutput{}, nil
		},
	}

	np := &NoteProcessor{
		logger:            zap.NewNop(),
		baseURL:           "https://example.com",
		communityNoteRepo: noteRepo,
		activityRepo:      actRepo,
		comprehendClient:  comprehendClient,
	}

	require.NoError(t, np.processNewNoteByID(context.Background(), "n1"))

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		EventID:   "evt-1",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("NOTE#n1"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}

	require.NoError(t, np.processRecord(context.Background(), "req", record))

	// Record failure is returned to AppTheory for partial batch responses.
	noteRepo2 := testingmocks.NewMockCommunityNoteRepository()
	noteRepo2.On("GetCommunityNote", mock.Anything, "n2").Return((*storage.CommunityNote)(nil), errors.New("missing"))
	np2 := &NoteProcessor{logger: zap.NewNop(), communityNoteRepo: noteRepo2}
	record2 := events.DynamoDBEventRecord{
		EventName: "INSERT",
		EventID:   "evt-2",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("NOTE#n2"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}
	err := np2.processRecord(context.Background(), "req", record2)
	require.Error(t, err)
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestNoteProcessor_PerformAIReputationAnalysis_PopulatesCost(t *testing.T) {
	fakeBedrock := &fakeBedrockClient{
		resp: &ai.ReputationAnalysisResponse{
			ReputationScore: 900,
			ConfidenceLevel: 0.9,
			RiskFactors:     []string{"none"},
			Reasoning:       "ok",
		},
	}

	np := &NoteProcessor{
		logger:        zap.NewNop(),
		bedrockClient: fakeBedrock,
	}

	note := &storage.CommunityNote{
		ID:       "n1",
		AuthorID: "alice",
		Content:  "research analysis evidence data statistics",
		Language: "en",
		Sources:  []string{"https://wikipedia.org/wiki/X", "https://example.gov/page"},
	}

	cost := &models.AICost{}
	score, factors, err := np.performAIReputationAnalysis(context.Background(), note.AuthorID, note, cost)
	require.NoError(t, err)
	require.Equal(t, 900.0, score)
	require.NotEmpty(t, factors)

	require.Greater(t, cost.InputCharacters, int64(0))
	require.Greater(t, cost.OutputCharacters, int64(0))
	require.Equal(t, "json", cost.ResponseFormat)

	fakeBedrock.mu.Lock()
	req := fakeBedrock.req
	fakeBedrock.mu.Unlock()
	require.Contains(t, req.Content, "research")
	require.NotEmpty(t, req.Sources)
}

func TestNoteProcessor_CalculateComprehensiveReputation_UsesAllSignals(t *testing.T) {
	now := time.Now()

	userRepo := testingmocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, "alice").Return(&storage.User{CreatedAt: now.AddDate(-2, 0, 0)}, nil)

	relRepo := testingmocks.NewMockRelationshipRepository()
	relRepo.On("GetFollowerCount", mock.Anything, "alice").Return(int64(100), nil)
	relRepo.On("GetFollowingCount", mock.Anything, "alice").Return(int64(0), nil)

	actRepo := testingmocks.NewMockActivityRepository()
	actRepo.On("GetOutboxActivities", mock.Anything, "alice", 1000, "").Return([]*activitypub.Activity{}, "", nil)

	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("GetUserVotingHistory", mock.Anything, "alice", mock.Anything).Return([]*storage.CommunityNoteVote{
		{NoteID: "v1", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "v2", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "v3", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "v4", Helpful: true, VoteType: "helpful", Weight: 1},
		{NoteID: "v5", Helpful: true, VoteType: "helpful", Weight: 1},
	}, nil)
	for _, id := range []string{"v1", "v2", "v3", "v4", "v5"} {
		noteRepo.On("GetCommunityNote", mock.Anything, id).Return(&storage.CommunityNote{ID: id, Status: "accepted", Score: 1.0}, nil)
	}

	modRepo := testingmocks.NewMockModerationRepository()
	modRepo.On("GetModerationEventsByObject", mock.Anything, "alice", 100, "").Return([]*storage.ModerationEvent{
		{EventType: "warn", CreatedAt: now.AddDate(0, 0, -1)},
		{EventType: "warn", CreatedAt: now.AddDate(0, 0, -120)}, // out of window
	}, "", nil)

	fakeBedrock := &fakeBedrockClient{
		resp: &ai.ReputationAnalysisResponse{ReputationScore: 900, ConfidenceLevel: 0.9, Reasoning: "ok"},
	}

	np := &NoteProcessor{
		logger:            zap.NewNop(),
		userRepo:          userRepo,
		relationshipRepo:  relRepo,
		activityRepo:      actRepo,
		communityNoteRepo: noteRepo,
		moderationRepo:    modRepo,
		bedrockClient:     fakeBedrock,
	}

	note := &storage.CommunityNote{
		ID:       "n1",
		AuthorID: "https://example.com/users/alice",
		Content:  "content",
		Language: "en",
		Sources:  []string{"https://wikipedia.org/wiki/X"},
	}

	score := np.calculateComprehensiveReputation(context.Background(), note.AuthorID, note)
	require.True(t, score >= 0.0 && score <= 1000.0)
	require.True(t, score > 700.0)

	// Invalid actor ID returns base reputation without panicking.
	require.Equal(t, ReputationBaseValue, np.calculateComprehensiveReputation(context.Background(), "not-a-user-uri", note))
}

func TestNoteProcessor_PerformBedrockReputationAnalysis_FallsBackOnError(t *testing.T) {
	np := &NoteProcessor{
		logger:        zap.NewNop(),
		bedrockClient: &fakeBedrockClient{err: errors.New("nope")},
	}

	score := np.performBedrockReputationAnalysis("content", []string{"https://wikipedia.org/wiki/X"}, []string{"multiple_sources"}, "alice")
	require.False(t, math.IsNaN(score))
	require.True(t, score >= 0.0 && score <= 1000.0)
}
