package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/moderation/advanced"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setModerationProcessorTestGlobals(t *testing.T) {
	t.Helper()

	cfgValue := *config.Get()
	cfg := &cfgValue
	lambdaCtx = &common.LambdaContext{
		Config: cfg,
		Logger: zap.NewNop(),
	}
}

type fakeConsensusStorage struct {
	events          map[string]*moderation.ModerationEvent
	reviewsByEvent  map[string][]*moderation.Review
	getEventErr     error
	addReviewErr    error
	getReviewsErr   error
	createDecision  error
	decisions       []*moderation.ModerationDecision
	recordTrustErr  error
	recordedUpdates []*models.TrustUpdate
}

func newFakeConsensusStorage() *fakeConsensusStorage {
	return &fakeConsensusStorage{
		events:         make(map[string]*moderation.ModerationEvent),
		reviewsByEvent: make(map[string][]*moderation.Review),
	}
}

func (s *fakeConsensusStorage) GetModerationEvent(_ context.Context, eventID string) (*moderation.ModerationEvent, error) {
	if s.getEventErr != nil {
		return nil, s.getEventErr
	}
	if event, ok := s.events[eventID]; ok {
		return event, nil
	}
	return &moderation.ModerationEvent{
		ID:       eventID,
		ObjectID: "obj-1",
		ActorID:  "actor-1",
		Category: moderation.CategoryNSFW,
		Severity: 1,
	}, nil
}

func (s *fakeConsensusStorage) AddModerationReview(_ context.Context, review *moderation.Review) error {
	if s.addReviewErr != nil {
		return s.addReviewErr
	}
	s.reviewsByEvent[review.EventID] = append(s.reviewsByEvent[review.EventID], review)
	return nil
}

func (s *fakeConsensusStorage) GetModerationReviews(_ context.Context, eventID string) ([]*moderation.Review, error) {
	if s.getReviewsErr != nil {
		return nil, s.getReviewsErr
	}
	return append([]*moderation.Review(nil), s.reviewsByEvent[eventID]...), nil
}

func (s *fakeConsensusStorage) CreateModerationDecision(_ context.Context, decision *moderation.ModerationDecision) error {
	if s.createDecision != nil {
		return s.createDecision
	}
	s.decisions = append(s.decisions, decision)
	return nil
}

func (s *fakeConsensusStorage) GetModerationQueue(context.Context, int, string) ([]*moderation.QueueItem, string, error) {
	return []*moderation.QueueItem{}, "", nil
}

func (s *fakeConsensusStorage) GetTrustScore(context.Context, string, string) (*models.TrustScore, error) {
	return &models.TrustScore{
		Score:      1.0,
		Confidence: 1.0,
	}, nil
}

func (s *fakeConsensusStorage) RecordTrustUpdate(_ context.Context, update *models.TrustUpdate) error {
	if s.recordTrustErr != nil {
		return s.recordTrustErr
	}
	s.recordedUpdates = append(s.recordedUpdates, update)
	return nil
}

func TestModerationProcessor_NewAndRecordRouting(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	t.Run("constructor wires globals", func(t *testing.T) {
		prevDB := db
		prevModerationRepo := moderationRepo
		prevUserRepo := userRepo
		prevNotificationRepo := notificationRepo
		prevObjectRepo := objectRepo
		prevConsensus := consensusEngine
		prevAdvanced := advancedEngine
		t.Cleanup(func() {
			db = prevDB
			moderationRepo = prevModerationRepo
			userRepo = prevUserRepo
			notificationRepo = prevNotificationRepo
			objectRepo = prevObjectRepo
			consensusEngine = prevConsensus
			advancedEngine = prevAdvanced
		})

		db = new(mocks.MockDB)
		moderationRepo = &repositories.ModerationRepository{}
		userRepo = &repositories.UserRepository{}
		notificationRepo = &repositories.NotificationRepository{}
		objectRepo = &repositories.ObjectRepository{}

		storageBackend := newFakeConsensusStorage()
		consensusEngine = moderation.NewConsensusEngine(storageBackend, nil)
		advancedEngine = nil

		processor := NewModerationProcessor()
		require.NotNil(t, processor)
		require.NotNil(t, processor.logger)
		require.Equal(t, lambdaCtx.Logger, processor.logger)
	})

	mp := &ModerationProcessor{logger: zap.NewNop(), consensusEngine: moderation.NewConsensusEngine(newFakeConsensusStorage(), &moderation.ConsensusConfig{
		MinReviewers:        1,
		MinTrustWeight:      0,
		ConsensusThreshold:  0,
		CriticalThreshold:   0,
		EscalationThreshold: 1.1,
	})}

	record := events.DynamoDBEventRecord{
		EventName: "REMOVE",
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("REVIEW#evt-1"),
				"SK": events.NewStringAttribute("REVIEWER#mod-1"),
			},
		},
	}
	require.NoError(t, mp.processRecord(context.Background(), record))

	record.EventName = "INSERT"
	record.Change.Keys["PK"] = events.NewStringAttribute("")
	require.NoError(t, mp.processRecord(context.Background(), record))
}

func TestModerationProcessor_HandleNewReview_CoversDecisionAndErrorPaths(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	t.Run("consensus decision path", func(t *testing.T) {
		storageBackend := newFakeConsensusStorage()
		storageBackend.events["evt-1"] = &moderation.ModerationEvent{
			ID:       "evt-1",
			ObjectID: "obj-1",
			ActorID:  "actor-1",
			Category: moderation.CategoryNSFW,
			Severity: 5,
		}

		engine := moderation.NewConsensusEngine(storageBackend, &moderation.ConsensusConfig{
			MinReviewers:        1,
			MinTrustWeight:      0,
			ConsensusThreshold:  0,
			CriticalThreshold:   0,
			EscalationThreshold: 1.1,
		})

		mp := &ModerationProcessor{logger: zap.NewNop(), consensusEngine: engine}
		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":    events.NewStringAttribute("REVIEW"),
					"Action":  events.NewStringAttribute("remove"),
					"Weight":  events.NewNumberAttribute("2.5"),
					"Created": events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("REVIEW#evt-1"),
					"SK": events.NewStringAttribute("REVIEWER#mod-1"),
				},
			},
		}

		require.NoError(t, mp.handleNewReview(ctx, record))
	})

	t.Run("invalid PK/SK formats", func(t *testing.T) {
		mp := &ModerationProcessor{logger: zap.NewNop(), consensusEngine: moderation.NewConsensusEngine(newFakeConsensusStorage(), nil)}

		invalidPK := events.DynamoDBEventRecord{Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("REVIEW"),
				"SK": events.NewStringAttribute("REVIEWER#mod-1"),
			},
		}}
		require.Error(t, mp.handleNewReview(ctx, invalidPK))

		invalidSK := events.DynamoDBEventRecord{Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("REVIEW#evt-1"),
				"SK": events.NewStringAttribute("REVIEWER"),
			},
		}}
		require.Error(t, mp.handleNewReview(ctx, invalidSK))
	})

	t.Run("consensus engine error does not fail batch", func(t *testing.T) {
		storageBackend := newFakeConsensusStorage()
		storageBackend.getEventErr = errors.New("boom")
		engine := moderation.NewConsensusEngine(storageBackend, nil)

		mp := &ModerationProcessor{logger: zap.NewNop(), consensusEngine: engine}
		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":   events.NewStringAttribute("REVIEW"),
					"Action": events.NewStringAttribute("remove"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("REVIEW#evt-1"),
					"SK": events.NewStringAttribute("REVIEWER#mod-1"),
				},
			},
		}

		require.NoError(t, mp.handleNewReview(ctx, record))
	})
}

func TestModerationProcessor_SendModeratorNotification_AndFallbackAdmins(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	createUserRepo := func(roleUsers map[string][]models.User) *repositories.UserRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()

		currentRole := ""
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			field, _ := args.Get(0).(string)
			if field != "gsi3PK" {
				return
			}
			value, _ := args.Get(2).(string)
			currentRole = strings.TrimPrefix(value, "ROLE#")
		}).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.User)
			*dest = append([]models.User(nil), roleUsers[currentRole]...)
		}).Return(nil).Maybe()

		return repositories.NewUserRepository(mockDB, "test-table", zap.NewNop())
	}

	t.Run("selected moderators path", func(t *testing.T) {
		userRepo := createUserRepo(map[string][]models.User{
			"moderator": {
				{Username: "mod-1", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-100 * 24 * time.Hour)},
				{Username: "mod-2", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-200 * 24 * time.Hour)},
			},
			"admin": {},
		})

		mp := &ModerationProcessor{
			userRepo:         userRepo,
			moderationRepo:   nil,
			notificationRepo: &repositories.NotificationRepository{},
			logger:           zap.NewNop(),
		}

		event := &moderation.ModerationEvent{
			ID:       "evt-1",
			ObjectID: "obj-1",
			ActorID:  "actor-1",
			Category: moderation.CategoryNSFW,
			Severity: 8,
		}

		require.NoError(t, mp.sendModeratorNotification(ctx, event))
	})

	t.Run("fallback admin notifications when no moderators", func(t *testing.T) {
		userRepo := createUserRepo(map[string][]models.User{
			"moderator": {},
			"admin": {
				{Username: "admin-1", Role: adminRole, Approved: true},
			},
		})

		mp := &ModerationProcessor{
			userRepo:         userRepo,
			notificationRepo: &repositories.NotificationRepository{},
			logger:           zap.NewNop(),
		}

		event := &moderation.ModerationEvent{
			ID:       "evt-2",
			ObjectID: "obj-2",
			ActorID:  "actor-2",
			Category: moderation.CategoryNSFW,
			Severity: 9,
		}

		require.NoError(t, mp.notifyFallbackAdmins(ctx, event))
	})
}

func TestModeratorSelector_SelectByWorkload_SortsByPendingCount(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	currentModerator := ""
	currentStatus := ""

	workload := map[string]map[string]int{
		"mod-1": {
			string(storage.ReportStatusOpen):       2,
			string(storage.ReportStatusInProgress): 0,
			string(storage.FlagStatusPending):      1,
		},
		"mod-2": {
			string(storage.ReportStatusOpen):       0,
			string(storage.ReportStatusInProgress): 0,
			string(storage.FlagStatusPending):      0,
		},
		"mod-3": {
			string(storage.ReportStatusOpen):       1,
			string(storage.ReportStatusInProgress): 0,
			string(storage.FlagStatusPending):      0,
		},
	}

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		field, _ := args.Get(0).(string)
		switch field {
		case "AssignedTo":
			currentModerator, _ = args.Get(2).(string)
		case "Status":
			currentStatus, _ = args.Get(2).(string)
		}
	}).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if dest, ok := args.Get(0).(*[]models.Report); ok {
			count := workload[currentModerator][currentStatus]
			*dest = make([]models.Report, count)
		}
		if dest, ok := args.Get(0).(*[]models.Flag); ok {
			count := workload[currentModerator][currentStatus]
			*dest = make([]models.Flag, count)
		}
	}).Return(nil).Maybe()

	moderationRepo := repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	selector := NewModeratorSelector(nil, moderationRepo, zap.NewNop())

	mods := []*storage.User{
		{Username: "mod-1"},
		{Username: "mod-2"},
		{Username: "mod-3"},
	}

	selected, err := selector.selectByWorkload(ctx, mods, &moderation.ModerationEvent{ID: "evt", Severity: 7})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	require.Equal(t, "mod-2", selected[0].Username)
	require.Equal(t, "mod-3", selected[1].Username)
}

func TestModerationProcessor_TriggerAutomaticActions_HighSeverityBranches(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	newModerationRepoWithCreate := func(createErr error) *repositories.ModerationRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Create").Return(createErr).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("not used")).Maybe()
		mockQuery.On("All", mock.Anything).Return(nil).Maybe()

		return repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	}

	t.Run("add review failure surfaces", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoWithCreate(errors.New("db down")),
			consensusEngine: moderation.NewConsensusEngine(newFakeConsensusStorage(), &moderation.ConsensusConfig{
				MinReviewers: 2,
			}),
		}

		err := mp.triggerAutomaticActions(ctx, &moderation.ModerationEvent{ID: "evt", Severity: 8})
		require.Error(t, err)
	})

	t.Run("consensus engine error surfaces", func(t *testing.T) {
		storageBackend := newFakeConsensusStorage()
		storageBackend.getEventErr = errors.New("missing")

		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoWithCreate(nil),
			consensusEngine: moderation.NewConsensusEngine(storageBackend, &moderation.ConsensusConfig{
				MinReviewers: 1,
				MinTrustWeight: 0,
				ConsensusThreshold: 0,
				CriticalThreshold: 0,
				EscalationThreshold: 1.1,
			}),
		}

		err := mp.triggerAutomaticActions(ctx, &moderation.ModerationEvent{ID: "evt", Severity: 9})
		require.Error(t, err)
	})

	t.Run("decision made path", func(t *testing.T) {
		storageBackend := newFakeConsensusStorage()
		storageBackend.events["evt-3"] = &moderation.ModerationEvent{ID: "evt-3", ObjectID: "obj", ActorID: "actor"}

		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoWithCreate(nil),
			consensusEngine: moderation.NewConsensusEngine(storageBackend, &moderation.ConsensusConfig{
				MinReviewers:        1,
				MinTrustWeight:      0,
				ConsensusThreshold:  0,
				CriticalThreshold:   0,
				EscalationThreshold: 1.1,
			}),
		}

		require.NoError(t, mp.triggerAutomaticActions(ctx, &moderation.ModerationEvent{ID: "evt-3", Severity: 9}))
	})
}

func TestModerationProcessor_HandleNewEvent_AndContentRemoval(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	t.Run("handleNewEvent executes notification and automatic action paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Create").Return(nil).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("not used")).Maybe()
		mockQuery.On("All", mock.Anything).Return(nil).Maybe()

		userRepoDB := new(mocks.MockDB)
		userRepoQuery := new(mocks.MockQuery)
		userRepoDB.On("WithContext", mock.Anything).Return(userRepoDB).Maybe()
		userRepoDB.On("Model", mock.Anything).Return(userRepoQuery).Maybe()
		userRepoQuery.On("Index", mock.Anything).Return(userRepoQuery).Maybe()

		currentRole := ""
		userRepoQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			field, _ := args.Get(0).(string)
			if field != "gsi3PK" {
				return
			}
			value, _ := args.Get(2).(string)
			currentRole = strings.TrimPrefix(value, "ROLE#")
		}).Return(userRepoQuery).Maybe()

		userRepoQuery.On("All", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.User)
			if currentRole != "moderator" {
				*dest = []models.User{}
				return
			}
			*dest = []models.User{
				{Username: "mod-1", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-200 * 24 * time.Hour)},
				{Username: "mod-2", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-100 * 24 * time.Hour)},
			}
		}).Return(nil).Maybe()

		mp := &ModerationProcessor{
			logger:           zap.NewNop(),
			userRepo:         repositories.NewUserRepository(userRepoDB, "test-table", zap.NewNop()),
			moderationRepo:   repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop()),
			notificationRepo: &repositories.NotificationRepository{},
			consensusEngine: moderation.NewConsensusEngine(newFakeConsensusStorage(), &moderation.ConsensusConfig{
				MinReviewers: 2,
			}),
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":      events.NewStringAttribute("EVENT"),
					"ID":        events.NewStringAttribute("evt-1"),
					"ActorID":   events.NewStringAttribute("actor-1"),
					"EventType": events.NewStringAttribute("flagged"),
					"Category":  events.NewStringAttribute("nsfw"),
					"Severity":  events.NewNumberAttribute("8"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("EVENT#obj-1"),
					"SK": events.NewStringAttribute("METADATA"),
				},
			},
		}

		require.NoError(t, mp.handleNewEvent(ctx, record))
	})

	t.Run("enforceContentRemoval aggregates failures but proceeds", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("not found")).Maybe()
		mockQuery.On("Delete").Return(errors.New("boom")).Maybe()

		objectRepo := repositories.NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		mp := &ModerationProcessor{
			logger:    zap.NewNop(),
			objectRepo: objectRepo,
		}

		err := mp.enforceContentRemoval(ctx, "obj-1")
		require.Error(t, err)
	})

	t.Run("enforceContentRemoval validates required object IDs", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("not found")).Maybe()
		mockQuery.On("Delete").Return(nil).Maybe()

		objectRepo := repositories.NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		mp := &ModerationProcessor{
			logger:    zap.NewNop(),
			objectRepo: objectRepo,
		}

		err := mp.enforceContentRemoval(ctx, "")
		require.Error(t, err)
	})
}

func TestPatternRepositoryAdapter_AllMethods(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.ModerationPattern")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.ModerationPattern)
		dest.PatternID = "pat-1"
		dest.Pattern = "foo"
		dest.Type = "keyword"
		dest.Category = "spam"
		dest.Name = "Test"
		dest.Severity = 1.2
		dest.Description = "desc"
		dest.Active = true
		dest.Flags = []string{"f1"}
		dest.CreatedAt = time.Now()
		dest.UpdatedAt = time.Now()
		dest.HitCount = 5
		dest.LastHit = time.Now()
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*[]*models.ModerationPattern)
		if !ok {
			return
		}
		*dest = []*models.ModerationPattern{
			{
				PatternID:   "pat-2",
				Pattern:     "bar",
				Type:        "regex",
				Category:    "nsfw",
				Name:        "Other",
				Severity:    2.3,
				Description: "desc",
				Active:      true,
			},
		}
	}).Return(nil).Maybe()

	patternRepo := repositories.NewPatternRepository(mockDB, "test-table", zap.NewNop(), nil)
	adapter := &patternRepositoryAdapter{repo: patternRepo}

	pattern := &advanced.ModerationPattern{
		ID:          "pat-1",
		Pattern:     "foo",
		Type:        "keyword",
		Category:    "spam",
		Name:        "Test",
		Severity:    1.2,
		Description: "desc",
		Active:      true,
		Flags:       []string{"f1"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	require.NoError(t, adapter.CreatePattern(ctx, pattern))
	require.NoError(t, adapter.UpdatePattern(ctx, "pat-1", pattern))
	require.NoError(t, adapter.DeletePattern(ctx, "pat-1"))

	got, err := adapter.GetPattern(ctx, "pat-1")
	require.NoError(t, err)
	require.Equal(t, "pat-1", got.ID)

	activeOnly := true
	list, err := adapter.GetPatterns(ctx, advanced.PatternFilter{Category: "spam", Active: &activeOnly})
	require.NoError(t, err)
	require.NotEmpty(t, list)

	require.NoError(t, adapter.IncrementHitCount(ctx, "pat-1"))

	loaded, err := adapter.LoadActivePatterns(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, loaded)
}

func TestModerationProcessor_EnforcementAccountActions_ErrorAggregation(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mp := &ModerationProcessor{
		logger:   zap.NewNop(),
		userRepo: &repositories.UserRepository{},
	}

	err := mp.enforceAccountSilencing(ctx, "bad username", "reason")
	require.Error(t, err)

	err = mp.enforceAccountSuspension(ctx, "bad username", "reason")
	require.Error(t, err)

	// Trigger validation failures in stubbed side effects to cover aggregation branches.
	err = mp.enforceAccountSilencing(ctx, "", "reason")
	require.Error(t, err)
}

func TestModerationProcessor_HandleDecision_UnknownAndNone(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	newModerationRepoForEnforcementStatus := func(populate bool) *repositories.ModerationRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.ModerationDecisionResult")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.ModerationDecisionResult)
			if !populate {
				*dest = []*models.ModerationDecisionResult{}
				return
			}
			*dest = []*models.ModerationDecisionResult{
				{
					ID:                "dec-result-1",
					ContentID:         "obj-1",
					Action:            "none",
					Confidence:        1.0,
					DecidedAt:         time.Now(),
					EnforcementStatus: "pending",
				},
			}
		}).Return(nil).Maybe()

		return repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	}

	t.Run("unknown action returns error and logs update failure", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoForEnforcementStatus(false),
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":   events.NewStringAttribute("DECISION"),
					"ID":     events.NewStringAttribute("dec-1"),
					"EventID": events.NewStringAttribute("evt-1"),
					"Action": events.NewStringAttribute("weird"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("DECISION#obj-1"),
					"SK": events.NewStringAttribute("TIME#t"),
				},
			},
		}

		err := mp.handleDecision(ctx, record)
		require.Error(t, err)
	})

	t.Run("none action returns nil when enforcement status updates succeed", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoForEnforcementStatus(true),
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":   events.NewStringAttribute("DECISION"),
					"ID":     events.NewStringAttribute("dec-2"),
					"EventID": events.NewStringAttribute("evt-2"),
					"Action": events.NewStringAttribute("none"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("DECISION#obj-1"),
					"SK": events.NewStringAttribute("TIME#t"),
				},
			},
		}

		require.NoError(t, mp.handleDecision(ctx, record))
	})
}

func TestRepositoryStorageAdapter_BasicOperations(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mockDBModeration := new(mocks.MockDB)
	mockQueryModeration := new(mocks.MockQuery)

	mockDBModeration.On("WithContext", mock.Anything).Return(mockDBModeration).Maybe()
	mockDBModeration.On("Model", mock.Anything).Return(mockQueryModeration).Maybe()

	mockQueryModeration.On("Index", mock.Anything).Return(mockQueryModeration).Maybe()
	mockQueryModeration.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryModeration).Maybe()
	mockQueryModeration.On("Limit", mock.Anything).Return(mockQueryModeration).Maybe()
	mockQueryModeration.On("Cursor", mock.Anything).Return(mockQueryModeration).Maybe()
	mockQueryModeration.On("Create").Return(nil).Maybe()
	mockQueryModeration.On("Count").Return(int64(1), nil).Maybe()

	mockQueryModeration.On("First", mock.AnythingOfType("*models.ModerationEvent")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.ModerationEvent)
		dest.ID = "evt-1"
		dest.ObjectID = "obj-1"
		dest.ActorID = "actor-1"
		dest.Category = "nsfw"
		dest.Severity = "critical"
		dest.ConfidenceScore = 0.9
	}).Return(nil).Maybe()

	mockQueryModeration.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.ModerationReview:
			*dest = []models.ModerationReview{
				{Type: "REVIEW", ID: "rev-1", EventID: "evt-1", ReviewerID: "mod-1", Action: "remove", Severity: "high"},
			}
		case *[]models.ModerationEvent:
			*dest = []models.ModerationEvent{
				{ID: "evt-1", GSI2SK: "c1", Severity: "critical", ConfidenceScore: 0.9},
				{ID: "evt-2", GSI2SK: "c2", Severity: "high", ConfidenceScore: 0.8},
				{ID: "evt-3", GSI2SK: "c3", Severity: "medium", ConfidenceScore: 0.7},
			}
		}
	}).Return(nil).Maybe()

	moderationRepo := repositories.NewModerationRepository(mockDBModeration, "test-table", zap.NewNop())

	mockDBUsers := new(mocks.MockDB)
	mockQueryUsers := new(mocks.MockQuery)

	mockDBUsers.On("Model", mock.Anything).Return(mockQueryUsers).Maybe()
	mockQueryUsers.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryUsers).Maybe()
	mockQueryUsers.On("First", mock.AnythingOfType("*models.TrustScore")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.TrustScore)
		dest.ActorID = "actor-1"
		dest.Category = "content"
		dest.Score = 0.8
		dest.Confidence = 0.5
		dest.CacheTTL = time.Now().Add(2 * time.Hour)
	}).Return(nil).Maybe()
	mockQueryUsers.On("Create").Return(nil).Maybe()

	userRepo := repositories.NewUserRepository(mockDBUsers, "test-table", zap.NewNop())

	adapter := &repositoryStorageAdapter{
		moderationRepo: moderationRepo,
		userRepo:       userRepo,
	}

	event, err := adapter.GetModerationEvent(ctx, "evt-1")
	require.NoError(t, err)
	require.Equal(t, "evt-1", event.ID)

	require.NoError(t, adapter.AddModerationReview(ctx, &moderation.Review{
		EventID:    "evt-1",
		ReviewerID: "mod-1",
		Action:     moderation.ActionTypeRemove,
		Created:    time.Now(),
	}))

	reviews, err := adapter.GetModerationReviews(ctx, "evt-1")
	require.NoError(t, err)
	require.NotEmpty(t, reviews)

	require.NoError(t, adapter.CreateModerationDecision(ctx, &moderation.ModerationDecision{
		ID:       "dec-1",
		EventID:  "evt-1",
		ObjectID: "obj-1",
		Action:   moderation.ActionTypeNone,
		Decided:  time.Now(),
	}))

	items, nextCursor, err := adapter.GetModerationQueue(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.NotEmpty(t, nextCursor)

	score, err := adapter.GetTrustScore(ctx, "actor-1", "content")
	require.NoError(t, err)
	require.Equal(t, 0.8, score.Score)

	require.NoError(t, adapter.RecordTrustUpdate(ctx, &models.TrustUpdate{
		ActorID:  "actor-1",
		Category: models.TrustCategoryContent,
		Delta:    0.1,
		Reason:   "test",
		EventID:  "evt-1",
	}))
}

func TestModerationProcessor_ProcessRecord_RoutesDecision(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.ModerationDecisionResult")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ModerationDecisionResult)
		*dest = []*models.ModerationDecisionResult{}
	}).Return(nil).Maybe()

	moderationRepo := repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())

	mp := &ModerationProcessor{
		logger:         zap.NewNop(),
		moderationRepo: moderationRepo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Type":   events.NewStringAttribute("DECISION"),
				"ID":     events.NewStringAttribute("dec-1"),
				"EventID": events.NewStringAttribute("evt-1"),
				"Action": events.NewStringAttribute("weird"),
			},
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("DECISION#obj-1"),
				"SK": events.NewStringAttribute("TIME#t"),
			},
		},
	}

	err := mp.processRecord(ctx, record)
	require.Error(t, err)
}

func TestInitAdvancedModerationEngine_CoversBasicModeWithoutAWS(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	prevRunningUnitTests := runningUnitTests
	prevDB := db
	prevPatternRepo := patternRepo
	prevAdvancedEngine := advancedEngine

	t.Cleanup(func() {
		runningUnitTests = prevRunningUnitTests
		db = prevDB
		patternRepo = prevPatternRepo
		advancedEngine = prevAdvancedEngine
	})

	runningUnitTests = func() bool { return false }

	lambdaCtx.Config.DisableAWSModeration = true

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()

	db = mockDB
	patternRepo = repositories.NewPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	initAdvancedModerationEngine()
	require.NotNil(t, advancedEngine)
}

func TestInitAdvancedModerationEngine_CoversAWSModeBranches(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	prevRunningUnitTests := runningUnitTests
	prevDB := db
	prevPatternRepo := patternRepo
	prevAdvancedEngine := advancedEngine
	prevAWSServices := lambdaCtx.AWSServices

	t.Cleanup(func() {
		runningUnitTests = prevRunningUnitTests
		db = prevDB
		patternRepo = prevPatternRepo
		advancedEngine = prevAdvancedEngine
		lambdaCtx.AWSServices = prevAWSServices
	})

	runningUnitTests = func() bool { return false }

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()

	db = mockDB
	patternRepo = repositories.NewPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	lambdaCtx.AWSServices = &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}}

	t.Run("aws mode with services disabled", func(t *testing.T) {
		lambdaCtx.Config.DisableAWSModeration = false
		lambdaCtx.Config.ModerationMode = "aws"
		lambdaCtx.Config.DisableComprehend = true
		lambdaCtx.Config.DisableRekognition = true

		initAdvancedModerationEngine()
		require.NotNil(t, advancedEngine)
	})

	t.Run("basic mode via config with services enabled", func(t *testing.T) {
		lambdaCtx.Config.DisableAWSModeration = false
		lambdaCtx.Config.ModerationMode = "basic"
		lambdaCtx.Config.DisableComprehend = false
		lambdaCtx.Config.DisableRekognition = false

		initAdvancedModerationEngine()
		require.NotNil(t, advancedEngine)
	})
}

func TestInitialize_CoversRepositoryWiring_WithInjectedDependencies(t *testing.T) {
	prevRunningUnitTests := runningUnitTests
	prevInitializeWithDefaults := initializeWithDefaults
	prevMustInitializeLambda := mustInitializeLambda
	prevNewLambdaOptimizedClient := newLambdaOptimizedClient

	prevLambdaCtx := lambdaCtx
	prevDB := db
	prevConsensusEngine := consensusEngine
	prevAdvancedEngine := advancedEngine
	prevModerationRepo := moderationRepo
	prevUserRepo := userRepo
	prevNotificationRepo := notificationRepo
	prevObjectRepo := objectRepo
	prevPatternRepo := patternRepo
	prevCfg := cfg
	prevLogger := logger
	prevRepos := repos

	t.Cleanup(func() {
		runningUnitTests = prevRunningUnitTests
		initializeWithDefaults = prevInitializeWithDefaults
		mustInitializeLambda = prevMustInitializeLambda
		newLambdaOptimizedClient = prevNewLambdaOptimizedClient

		lambdaCtx = prevLambdaCtx
		db = prevDB
		consensusEngine = prevConsensusEngine
		advancedEngine = prevAdvancedEngine
		moderationRepo = prevModerationRepo
		userRepo = prevUserRepo
		notificationRepo = prevNotificationRepo
		objectRepo = prevObjectRepo
		patternRepo = prevPatternRepo
		cfg = prevCfg
		logger = prevLogger
		repos = prevRepos
	})

	runningUnitTests = func() bool { return false }

	initializeWithDefaults = func(_ *common.LambdaContext) error {
		return errors.New("initializeWithDefaults failed for coverage")
	}

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	newLambdaOptimizedClient = func(_ context.Context, _ string) (core.DB, error) {
		return mockDB, nil
	}

	mustInitializeLambda = func(_ common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:              "us-east-1",
				Domain:              "example.com",
				DynamoTableName:      "test-table",
				DisableAWSModeration: true,
			},
			Logger:      zap.NewNop(),
			ServiceName: "moderation-processor",
			LambdaType:  common.LambdaTypeProcessor,
			AWSServices: &awsInit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
		}
	}

	initialize()

	require.NotNil(t, lambdaCtx)
	require.Same(t, mockDB, db)
	require.NotNil(t, moderationRepo)
	require.NotNil(t, userRepo)
	require.NotNil(t, notificationRepo)
	require.NotNil(t, objectRepo)
	require.NotNil(t, patternRepo)
	require.NotNil(t, consensusEngine)
	require.NotNil(t, advancedEngine)
}

func TestMain_DynamoDBHandler_CoversBatchProcessingPaths(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	prevLambdaStart := lambdaStart
	t.Cleanup(func() {
		lambdaStart = prevLambdaStart
	})

	var capturedHandler any
	lambdaStart = func(handler any) {
		capturedHandler = handler
	}

	main()

	require.NotNil(t, capturedHandler)

	handleRequest, ok := capturedHandler.(func(context.Context, any) (any, error))
	require.True(t, ok)

	successEvent := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventSource: "aws:dynamodb",
				EventName: "REMOVE",
				EventID:   "success",
				Change: events.DynamoDBStreamRecord{
					Keys: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("IGNORED"),
						"SK": events.NewStringAttribute("IGNORED"),
					},
				},
			},
		},
	}

	successRaw, err := marshalAsMap(successEvent)
	require.NoError(t, err)

	_, err = handleRequest(context.Background(), successRaw)
	require.NoError(t, err)

	errorEvent := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventSource: "aws:dynamodb",
				EventName: "INSERT",
				EventID:   "bad-review",
				Change: events.DynamoDBStreamRecord{
					Keys: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("REVIEW#evt-1"),
						"SK": events.NewStringAttribute("REVIEWER"),
					},
				},
			},
		},
	}

	errorRaw, err := marshalAsMap(errorEvent)
	require.NoError(t, err)

	_, err = handleRequest(context.Background(), errorRaw)
	require.Error(t, err)
}

func marshalAsMap(event any) (map[string]any, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func TestModeratorSelector_hasHandledCategory_CoversAdminAndHistoryPaths(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	t.Run("admin user short-circuits to true", func(t *testing.T) {
		mockUserDB := new(mocks.MockDB)
		mockUserQuery := new(mocks.MockQuery)

		mockUserDB.On("WithContext", mock.Anything).Return(mockUserDB).Maybe()
		mockUserDB.On("Model", mock.Anything).Return(mockUserQuery).Maybe()
		mockUserQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserQuery).Maybe()
		mockUserQuery.On("First", mock.AnythingOfType("*models.User")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.User)
			dest.Username = "admin-1"
			dest.Role = adminRole
		}).Return(nil).Maybe()

		userRepo := repositories.NewUserRepository(mockUserDB, "test-table", zap.NewNop())
		ms := NewModeratorSelector(userRepo, nil, zap.NewNop())

		require.True(t, ms.hasHandledCategory("admin-1", "spam"))
	})

	t.Run("non-admin relies on moderation history", func(t *testing.T) {
		mockUserDB := new(mocks.MockDB)
		mockUserQuery := new(mocks.MockQuery)

		mockUserDB.On("WithContext", mock.Anything).Return(mockUserDB).Maybe()
		mockUserDB.On("Model", mock.Anything).Return(mockUserQuery).Maybe()
		mockUserQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserQuery).Maybe()
		mockUserQuery.On("First", mock.AnythingOfType("*models.User")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.User)
			dest.Username = "mod-1"
			dest.Role = "moderator"
		}).Return(nil).Maybe()

		mockModerationDB := new(mocks.MockDB)
		mockModerationQuery := new(mocks.MockQuery)

		mockModerationDB.On("WithContext", mock.Anything).Return(mockModerationDB).Maybe()
		mockModerationDB.On("Model", mock.Anything).Return(mockModerationQuery).Maybe()
		mockModerationQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockModerationQuery).Maybe()
		mockModerationQuery.On("Limit", mock.Anything).Return(mockModerationQuery).Maybe()
		mockModerationQuery.On("All", mock.AnythingOfType("*[]*models.ModerationReview")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.ModerationReview)
			*dest = []*models.ModerationReview{
				{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
				{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
				{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
				{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
				{Tags: []string{"spam"}, Action: "remove", Severity: "low", Confidence: 0.9},
			}
		}).Return(nil).Maybe()

		userRepo := repositories.NewUserRepository(mockUserDB, "test-table", zap.NewNop())
		moderationRepo := repositories.NewModerationRepository(mockModerationDB, "test-table", zap.NewNop())

		ms := NewModeratorSelector(userRepo, moderationRepo, zap.NewNop())
		require.True(t, ms.hasHandledCategory("mod-1", "spam"))
	})
}

func TestModeratorSelector_calculateExpertiseScore_CoversCategoryBranches(t *testing.T) {
	setModerationProcessorTestGlobals(t)

	mockUserDB := new(mocks.MockDB)
	mockUserQuery := new(mocks.MockQuery)

	mockUserDB.On("WithContext", mock.Anything).Return(mockUserDB).Maybe()
	mockUserDB.On("Model", mock.Anything).Return(mockUserQuery).Maybe()
	mockUserQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserQuery).Maybe()
	mockUserQuery.On("First", mock.AnythingOfType("*models.User")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.User)
		dest.Username = "admin-1"
		dest.Role = adminRole
	}).Return(nil).Maybe()

	userRepo := repositories.NewUserRepository(mockUserDB, "test-table", zap.NewNop())
	ms := NewModeratorSelector(userRepo, nil, zap.NewNop())

	moderator := &storage.User{
		Username:  "admin-1",
		Role:      adminRole,
		Approved:  true,
		Suspended: false,
		CreatedAt: time.Now().Add(-400 * 24 * time.Hour),
	}

	cases := []struct {
		name     string
		category moderation.Category
	}{
		{name: "spam", category: moderation.CategorySpam},
		{name: "hate_speech", category: moderation.CategoryHateSpeech},
		{name: "harassment", category: moderation.CategoryHarassment},
		{name: "misinformation", category: moderation.CategoryMisinformation},
		{name: "violence", category: moderation.CategoryViolence},
		{name: "nsfw", category: moderation.CategoryNSFW},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := ms.calculateExpertiseScore(moderator, &moderation.ModerationEvent{Category: tc.category})
			require.GreaterOrEqual(t, score, 1.0)
		})
	}
}

func TestModeratorSelector_SelectModerators_CoversStrategiesAndEmptyList(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	createUserRepo := func(roleUsers map[string][]models.User) *repositories.UserRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()

		currentRole := ""
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			field, _ := args.Get(0).(string)
			if field != "gsi3PK" {
				return
			}
			value, _ := args.Get(2).(string)
			currentRole = strings.TrimPrefix(value, "ROLE#")
		}).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.User)
			*dest = append([]models.User(nil), roleUsers[currentRole]...)
		}).Return(nil).Maybe()

		return repositories.NewUserRepository(mockDB, "test-table", zap.NewNop())
	}

	createModerationRepo := func(counts map[string]int) *repositories.ModerationRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

		currentModerator := ""
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			field, _ := args.Get(0).(string)
			if field == "AssignedTo" {
				currentModerator, _ = args.Get(2).(string)
			}
		}).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			if dest, ok := args.Get(0).(*[]models.Report); ok {
				*dest = make([]models.Report, counts[currentModerator])
			}
			if dest, ok := args.Get(0).(*[]models.Flag); ok {
				*dest = []models.Flag{}
			}
		}).Return(nil).Maybe()

		return repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	}

	t.Run("empty moderator list returns empty selection", func(t *testing.T) {
		ms := NewModeratorSelector(createUserRepo(map[string][]models.User{
			"moderator": {},
			"admin":     {},
		}), createModerationRepo(nil), zap.NewNop())

		selected, err := ms.SelectModerators(ctx, &moderation.ModerationEvent{ID: "evt"}, StrategyRoundRobin)
		require.NoError(t, err)
		require.Empty(t, selected)
	})

	ms := NewModeratorSelector(createUserRepo(map[string][]models.User{
		"moderator": {
			{Username: "m1", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-400 * 24 * time.Hour)},
			{Username: "m2", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-200 * 24 * time.Hour)},
			{Username: "m3", Role: "moderator", Approved: true, CreatedAt: time.Now().Add(-100 * 24 * time.Hour)},
		},
		"admin": {},
	}), createModerationRepo(map[string]int{"m1": 3, "m2": 0, "m3": 1}), zap.NewNop())

	event := &moderation.ModerationEvent{ID: "evt", Severity: 7, Category: moderation.CategoryNSFW}

	selected, err := ms.SelectModerators(ctx, event, StrategyRoundRobin)
	require.NoError(t, err)
	require.Len(t, selected, 2)

	selected, err = ms.SelectModerators(ctx, event, StrategyWorkloadBased)
	require.NoError(t, err)
	require.Len(t, selected, 2)

	selected, err = ms.SelectModerators(ctx, event, StrategyExpertiseBased)
	require.NoError(t, err)
	require.Len(t, selected, 2)

	selected, err = ms.SelectModerators(ctx, event, StrategyRandom)
	require.NoError(t, err)
	require.Len(t, selected, 2)

	selected, err = ms.SelectModerators(ctx, event, ModeratorSelectionStrategy("unknown"))
	require.NoError(t, err)
	require.Len(t, selected, 2)
}

func TestModerationProcessor_Enforcement_SuccessPaths(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	t.Run("enforceAccountSilencing returns nil when UpdateUser succeeds", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if dest, ok := args.Get(0).(*models.User); ok {
				dest.Username = "alice"
				dest.Role = "user"
			}
		}).Return(nil).Maybe()

		userRepo := repositories.NewUserRepository(mockDB, "test-table", zap.NewNop())
		mp := &ModerationProcessor{
			logger:   zap.NewNop(),
			userRepo: userRepo,
		}

		require.NoError(t, mp.enforceAccountSilencing(ctx, "alice", "reason"))
	})

	t.Run("enforceContentRemoval returns nil when object deletion succeeds", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Delete").Return(nil).Maybe()
		mockQuery.On("First", mock.Anything).Return(errors.New("not found")).Maybe()

		objectRepo := repositories.NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
		mp := &ModerationProcessor{
			logger:    zap.NewNop(),
			objectRepo: objectRepo,
		}

		require.NoError(t, mp.enforceContentRemoval(ctx, "obj-1"))
	})
}

func TestModeratorSelector_getModerationHistory_ErrorBranch(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Maybe()

	moderationRepo := repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	ms := &ModeratorSelector{
		moderationRepo: moderationRepo,
		logger:         zap.NewNop(),
	}

	_, err := ms.getModerationHistory(ctx, "mod-1", "spam")
	require.Error(t, err)
}

func TestModerationProcessor_AdminMethods_Coverage(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	t.Run("review queue surfaces validation errors safely", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: &repositories.ModerationRepository{},
		}

		_, err := mp.GetReviewQueueForAdmins(ctx, map[string]interface{}{"": "invalid"})
		require.Error(t, err)
	})

	t.Run("decision history and enforcement update call through", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Return(nil).Maybe()

		moderationRepo := repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: moderationRepo,
		}

		_, err := mp.GetDecisionHistoryForAdmins(ctx, "obj-1")
		require.NoError(t, err)
	})

	t.Run("admin update enforcement status success path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.ModerationDecisionResult")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.ModerationDecisionResult)
			*dest = []*models.ModerationDecisionResult{
				{
					ID:                "dec-result-1",
					ContentID:         "obj-1",
					Action:            "none",
					Confidence:        1.0,
					DecidedAt:         time.Now(),
					EnforcementStatus: "pending",
				},
			}
		}).Return(nil).Maybe()

		moderationRepo := repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: moderationRepo,
		}

		require.NoError(t, mp.UpdateEnforcementStatusForAdmins(ctx, "obj-1", "applied", "admin-1"))
	})
}

func TestModerationProcessor_HandleDecision_ActionBranches(t *testing.T) {
	setModerationProcessorTestGlobals(t)
	ctx := context.Background()

	newModerationRepoReturningStatusError := func() *repositories.ModerationRepository {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.ModerationDecisionResult")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.ModerationDecisionResult)
			*dest = []*models.ModerationDecisionResult{}
		}).Return(nil).Maybe()

		return repositories.NewModerationRepository(mockDB, "test-table", zap.NewNop())
	}

	newDecisionRecord := func(action string, objectID string) events.DynamoDBEventRecord {
		return events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"Type":   events.NewStringAttribute("DECISION"),
					"ID":     events.NewStringAttribute("dec-1"),
					"EventID": events.NewStringAttribute("evt-1"),
					"Action": events.NewStringAttribute(action),
					"Reason": events.NewStringAttribute("reason"),
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("DECISION#" + objectID),
					"SK": events.NewStringAttribute("TIME#t"),
				},
			},
		}
	}

	t.Run("silence branch executes enforcement and returns error", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoReturningStatusError(),
			userRepo:       &repositories.UserRepository{},
		}

		err := mp.handleDecision(ctx, newDecisionRecord("silence", "bad username"))
		require.Error(t, err)
	})

	t.Run("suspend branch executes enforcement and returns error", func(t *testing.T) {
		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoReturningStatusError(),
			userRepo:       &repositories.UserRepository{},
		}

		err := mp.handleDecision(ctx, newDecisionRecord("suspend", "bad username"))
		require.Error(t, err)
	})

	t.Run("remove branch executes content removal and returns error", func(t *testing.T) {
		mockObjectDB := new(mocks.MockDB)
		mockObjectQuery := new(mocks.MockQuery)

		mockObjectDB.On("WithContext", mock.Anything).Return(mockObjectDB).Maybe()
		mockObjectDB.On("Model", mock.Anything).Return(mockObjectQuery).Maybe()
		mockObjectQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockObjectQuery).Maybe()
		mockObjectQuery.On("First", mock.Anything).Return(errors.New("not found")).Maybe()
		mockObjectQuery.On("Delete").Return(errors.New("boom")).Maybe()

		mp := &ModerationProcessor{
			logger:         zap.NewNop(),
			moderationRepo: newModerationRepoReturningStatusError(),
			objectRepo:     repositories.NewObjectRepository(mockObjectDB, "test-table", "example.com", zap.NewNop()),
		}

		err := mp.handleDecision(ctx, newDecisionRecord("remove", "obj-1"))
		require.Error(t, err)
	})
}
