package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeReportTrustService struct {
	event       *storage.ModerationEvent
	eventErr    error
	updateErr   error
	updateCalls int
}

func (f *fakeReportTrustService) GetModerationEvent(context.Context, string) (*storage.ModerationEvent, error) {
	if f.eventErr != nil {
		return nil, f.eventErr
	}
	return f.event, nil
}

func (f *fakeReportTrustService) UpdateReporterTrustOnDecision(context.Context, string, *moderation.ModerationDecision, string) error {
	f.updateCalls++
	return f.updateErr
}

func TestReportTrustService_UpdateReporterTrustOnDecision_Round12(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()

	var createdRel *models.TrustRelationship
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe().Run(func(args mock.Arguments) {
		if tr, ok := args.Get(0).(*models.TrustRelationship); ok {
			createdRel = tr
		}
	})

	svc := NewReportTrustService(mockDB, "test-table", zap.NewNop())

	err := svc.UpdateReporterTrustOnDecision(context.Background(), "r1", &moderation.ModerationDecision{Action: moderation.ActionTypeRemove}, "https://remote.example/users/alice")
	require.NoError(t, err)
	require.NotNil(t, createdRel)
	require.Equal(t, float64(0.6), createdRel.Score)

	for _, action := range []moderation.ActionType{
		moderation.ActionTypeSuspend,
		moderation.ActionTypeSilence,
		moderation.ActionTypeWarning,
	} {
		createdRel = nil
		err = svc.UpdateReporterTrustOnDecision(context.Background(), "r1", &moderation.ModerationDecision{Action: action}, "https://remote.example/users/alice")
		require.NoError(t, err)
		require.NotNil(t, createdRel)
		require.Equal(t, float64(0.6), createdRel.Score)
	}

	createdRel = nil
	err = svc.UpdateReporterTrustOnDecision(context.Background(), "r1", &moderation.ModerationDecision{Action: moderation.ActionTypeNone}, "https://remote.example/users/alice")
	require.NoError(t, err)
	require.NotNil(t, createdRel)
	require.Equal(t, float64(0.4), createdRel.Score)
}

func TestReportTrustService_UpdateReporterTrustOnDecision_GetReportError_Round12(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Maybe()

	svc := NewReportTrustService(mockDB, "test-table", zap.NewNop())
	err := svc.UpdateReporterTrustOnDecision(context.Background(), "r1", &moderation.ModerationDecision{Action: moderation.ActionTypeRemove}, "actor")
	require.Error(t, err)
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestReportTrustService_GetModerationEvent_Round12(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Once()

	svc := NewReportTrustService(mockDB, "test-table", zap.NewNop())
	event, err := svc.GetModerationEvent(context.Background(), "evt")
	require.NoError(t, err)
	require.NotNil(t, event)

	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = svc.GetModerationEvent(context.Background(), "evt")
	require.Error(t, err)
}

func TestNewReportTrustUpdater_Round12(t *testing.T) {
	updater := NewReportTrustUpdater(new(dynamormmocks.MockDB), "test-table", zap.NewNop())
	require.NotNil(t, updater)
}

func TestReportTrustUpdater_processModerationEvent_Round12(t *testing.T) {
	event := &storage.ModerationEvent{
		ActorID: "actor-1",
		Evidence: []any{
			map[string]any{
				"type": "user_report",
				"metadata": map[string]any{
					"report_id": "report-123",
				},
			},
		},
	}

	service := &fakeReportTrustService{event: event}
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: service}

	err := updater.processModerationEvent(context.Background(), moderation.ModerationDecision{EventID: "evt", Action: moderation.ActionTypeRemove})
	require.NoError(t, err)
	require.Equal(t, 1, service.updateCalls)

	service.updateErr = errors.New("boom")
	err = updater.processModerationEvent(context.Background(), moderation.ModerationDecision{EventID: "evt", Action: moderation.ActionTypeRemove})
	require.Error(t, err)

	service.eventErr = errors.New("boom")
	err = updater.processModerationEvent(context.Background(), moderation.ModerationDecision{EventID: "evt", Action: moderation.ActionTypeRemove})
	require.NoError(t, err)
}

func TestReportTrustUpdater_ExtractHelpers_Round12(t *testing.T) {
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: &fakeReportTrustService{}}

	require.Equal(t, "s", updater.getStringFromAttribute(events.NewStringAttribute("s")))
	require.Equal(t, "", updater.getStringFromAttribute(events.NewNumberAttribute("1")))

	require.Equal(t, 1.25, updater.getNumberFromAttribute(events.NewNumberAttribute("1.25")))
	require.Equal(t, 0.0, updater.getNumberFromAttribute(events.NewStringAttribute("x")))
	require.Equal(t, 0.0, updater.getNumberFromAttribute(events.NewNumberAttribute("not-a-number")))
}

func TestReportTrustUpdater_extractReportIDFromEvidence_Branches_Round12(t *testing.T) {
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: &fakeReportTrustService{}}

	require.Equal(t, "", updater.extractReportIDFromEvidence("not-a-map"))
	require.Equal(t, "", updater.extractReportIDFromEvidence(map[string]any{"type": 123}))
	require.Equal(t, "", updater.extractReportIDFromEvidence(map[string]any{"type": "other"}))
	require.Equal(t, "", updater.extractReportIDFromEvidence(map[string]any{"type": "user_report", "metadata": 123}))
	require.Equal(t, "", updater.extractReportIDFromEvidence(map[string]any{"type": "user_report", "metadata": map[string]any{"report_id": 123}}))
	require.Equal(t, "r1", updater.extractReportIDFromEvidence(map[string]any{"type": "user_report", "metadata": map[string]any{"report_id": "r1"}}))
}

func TestReportTrustUpdater_processRecord_Branches_Round12(t *testing.T) {
	service := &fakeReportTrustService{eventErr: errors.New("boom")}
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: service}

	// Not insert/modify.
	require.NoError(t, updater.processRecord(context.Background(), events.DynamoDBEventRecord{EventName: "REMOVE"}))

	// Missing keys.
	require.NoError(t, updater.processRecord(context.Background(), events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
	}))

	// Not a moderation decision.
	require.NoError(t, updater.processRecord(context.Background(), events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK": events.NewStringAttribute("OTHER#1"),
			"SK": events.NewStringAttribute("META"),
		}},
	}))

	// Decision missing EventID.
	require.NoError(t, updater.processRecord(context.Background(), events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK": events.NewStringAttribute("MODERATION#1"),
			"SK": events.NewStringAttribute("DECISION"),
		}},
	}))
}

func TestReportTrustUpdater_extractModerationDecision_Round12(t *testing.T) {
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: &fakeReportTrustService{}}

	record := events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"ID":             events.NewStringAttribute("id"),
			"EventID":        events.NewStringAttribute("event"),
			"ObjectID":       events.NewStringAttribute("object"),
			"Action":         events.NewStringAttribute(string(moderation.ActionTypeRemove)),
			"ConsensusScore": events.NewNumberAttribute("0.75"),
			"ReviewerCount":  events.NewNumberAttribute("3"),
		}},
	}

	decision := updater.extractModerationDecision(record)
	require.Equal(t, "id", decision.ID)
	require.Equal(t, "event", decision.EventID)
	require.Equal(t, "object", decision.ObjectID)
	require.Equal(t, moderation.ActionTypeRemove, decision.Action)
	require.Equal(t, 0.75, decision.ConsensusScore)
	require.Equal(t, 3, decision.ReviewerCount)
}

func TestReportTrustUpdater_HandleStream_ProcessesAndErrors_Round12(t *testing.T) {
	service := &fakeReportTrustService{
		event: &storage.ModerationEvent{
			ActorID: "actor-1",
			Evidence: []any{
				map[string]any{
					"type": "user_report",
					"metadata": map[string]any{
						"report_id": "report-123",
					},
				},
			},
		},
		updateErr: errors.New("boom"),
	}
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: service}

	ctx := &apptheory.EventContext{RequestID: "req"}

	records := []events.DynamoDBEventRecord{
		{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":      events.NewStringAttribute("MODERATION#1"),
				"SK":      events.NewStringAttribute("DECISION"),
				"EventID": events.NewStringAttribute("evt"),
			}},
		},
		{EventName: "REMOVE"},
	}

	for _, record := range records {
		require.NoError(t, updater.HandleDynamoDBRecord(ctx, record))
	}
	require.Equal(t, 1, service.updateCalls)
}

func TestRunReportTrustUpdater_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origGetClient := dynamormGetClientFn
	origStart := lambdaStartFn
	origNewUpdater := newReportTrustUpdaterFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		dynamormGetClientFn = origGetClient
		lambdaStartFn = origStart
		newReportTrustUpdaterFn = origNewUpdater
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				DynamoTableName: "lesser-dev-main-table",
			},
			Logger: zap.NewNop(),
		}
	}

	dynamormGetClientFn = func(context.Context) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}

	fakeSvc := &fakeReportTrustService{
		event: &storage.ModerationEvent{
			ActorID: "actor-1",
			Evidence: []any{
				map[string]any{
					"type": "user_report",
					"metadata": map[string]any{
						"report_id": "report-123",
					},
				},
			},
		},
	}
	newReportTrustUpdaterFn = func(dynamormCore.DB, string, *zap.Logger) *ReportTrustUpdater {
		return &ReportTrustUpdater{logger: zap.NewNop(), trustService: fakeSvc}
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
				Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":      events.NewStringAttribute("MODERATION#1"),
					"SK":      events.NewStringAttribute("DECISION"),
					"EventID": events.NewStringAttribute("evt"),
					"Action":  events.NewStringAttribute(string(moderation.ActionTypeRemove)),
				}},
			},
		}}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.True(t, called)
	require.GreaterOrEqual(t, fakeSvc.updateCalls, 1)
}

func TestReportTrustUpdater_HandleStream_Round12(t *testing.T) {
	service := &fakeReportTrustService{eventErr: errors.New("boom")}
	updater := &ReportTrustUpdater{logger: zap.NewNop(), trustService: service}

	require.NoError(t, updater.HandleDynamoDBRecord(&apptheory.EventContext{RequestID: "req"}, events.DynamoDBEventRecord{}))
}
