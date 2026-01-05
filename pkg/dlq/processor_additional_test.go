package dlq

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeDLQRepo struct {
	createErr error
	updateErr error

	created []*models.DLQMessage
	updated []*models.DLQMessage

	reprocessMessages []*models.DLQMessage
	reprocessErr      error

	cleanupCount int
	cleanupErr   error

	analytics *repositories.DLQAnalytics
	trends    *repositories.DLQTrends

	searchResults []*models.DLQMessage
	searchCursor  string
}

func (f *fakeDLQRepo) CreateDLQMessage(_ context.Context, msg *models.DLQMessage) error {
	f.created = append(f.created, msg)
	return f.createErr
}

func (f *fakeDLQRepo) UpdateDLQMessage(_ context.Context, msg *models.DLQMessage) error {
	f.updated = append(f.updated, msg)
	return f.updateErr
}

func (f *fakeDLQRepo) GetDLQMessagesForReprocessing(_ context.Context, _ string, _ string, _ int, _ string) ([]*models.DLQMessage, string, error) {
	return f.reprocessMessages, "", f.reprocessErr
}

func (f *fakeDLQRepo) GetDLQAnalytics(_ context.Context, _ string, _ repositories.DLQTimeRange) (*repositories.DLQAnalytics, error) {
	return f.analytics, nil
}

func (f *fakeDLQRepo) GetDLQTrends(_ context.Context, _ string, _ int) (*repositories.DLQTrends, error) {
	return f.trends, nil
}

func (f *fakeDLQRepo) SearchDLQMessages(_ context.Context, _ *repositories.DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	return f.searchResults, f.searchCursor, nil
}

func (f *fakeDLQRepo) CleanupExpiredMessages(_ context.Context, _ time.Time) (int, error) {
	return f.cleanupCount, f.cleanupErr
}

type fakeCostRepo struct {
	createErr error
	records   []*models.DynamoDBCostRecord
}

func (f *fakeCostRepo) Create(_ context.Context, record *models.DynamoDBCostRecord) error {
	f.records = append(f.records, record)
	return f.createErr
}

func TestProcessor_InitializeAWSClients_Seams(t *testing.T) {
	origLoad := loadDefaultAWSConfigFunc
	origNewSQS := newSQSClientFunc
	t.Cleanup(func() {
		loadDefaultAWSConfigFunc = origLoad
		newSQSClientFunc = origNewSQS
	})

	processor := &Processor{
		logger:            zap.NewNop(),
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(zap.NewNop()),
	}

	t.Run("load config error", func(t *testing.T) {
		loadDefaultAWSConfigFunc = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, stdErrors.New("boom")
		}

		err := processor.InitializeAWSClients(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to load AWS config")
	})

	t.Run("success wires sqs client into reprocessor", func(t *testing.T) {
		loadDefaultAWSConfigFunc = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}
		mockSQS := &MockSQSClient{}
		newSQSClientFunc = func(aws.Config) SQSClient { return mockSQS }

		err := processor.InitializeAWSClients(context.Background())
		require.NoError(t, err)
		require.Same(t, mockSQS, processor.sqsClient)
		require.Same(t, mockSQS, processor.reprocessorClient.sqsClient)
	})
}

func TestProcessor_processMessage_ReprocessAndTrackCosts(t *testing.T) {
	logger := zap.NewNop()
	mockSQS := &MockSQSClient{}
	mockSQS.
		On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(in *sqs.GetQueueUrlInput) bool {
			return in != nil && in.QueueName != nil && *in.QueueName == "notification-processor-queue"
		})).
		Return(&sqs.GetQueueUrlOutput{QueueUrl: aws.String("https://queue-url")}, nil)
	mockSQS.
		On("SendMessage", mock.Anything, mock.MatchedBy(func(in *sqs.SendMessageInput) bool {
			if in == nil || in.QueueUrl == nil || *in.QueueUrl != "https://queue-url" {
				return false
			}
			if in.MessageBody == nil || *in.MessageBody == "" || in.DelaySeconds != 30 {
				return false
			}
			_, hasType := in.MessageAttributes["DLQ.ReprocessType"]
			_, hasTS := in.MessageAttributes["DLQ.ReprocessTimestamp"]
			return hasType && hasTS
		})).
		Return(&sqs.SendMessageOutput{}, nil)

	dlqRepo := &fakeDLQRepo{}
	costRepo := &fakeCostRepo{}

	processor := &Processor{
		logger:            logger,
		dlqRepo:           dlqRepo,
		costTrackingRepo:  costRepo,
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(logger),
	}
	processor.reprocessorClient.SetSQSClient(mockSQS)

	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
		MessageId:      "msg-1",
		ReceiptHandle:  "rh-1",
		Body:           `{"notification_id":"n1","user_id":"u1","channels":[]}`,
		MessageAttributes: map[string]events.SQSMessageAttribute{
			"DeadLetterQueue.SourceQueue": {StringValue: strPtr("notification-processor-queue")},
		},
		Attributes: map[string]string{
			"ApproximateReceiveCount": "1",
		},
	}

	err := processor.processMessage(context.Background(), record)
	require.NoError(t, err)

	require.Len(t, dlqRepo.created, 1)
	require.Len(t, dlqRepo.updated, 1)
	require.Equal(t, "resolved", dlqRepo.updated[0].Status)
	require.GreaterOrEqual(t, dlqRepo.updated[0].ReprocessingCount, 1)

	require.Len(t, costRepo.records, 1)
	require.Equal(t, "DLQProcessing", costRepo.records[0].OperationType)

	mockSQS.AssertExpectations(t)
}

func TestProcessor_processMessage_ErrorsDontAbort(t *testing.T) {
	logger := zap.NewNop()

	dlqRepo := &fakeDLQRepo{
		createErr: stdErrors.New("db down"),
		updateErr: stdErrors.New("update failed"),
	}
	costRepo := &fakeCostRepo{createErr: stdErrors.New("cost write failed")}

	processor := &Processor{
		logger:            logger,
		dlqRepo:           dlqRepo,
		costTrackingRepo:  costRepo,
		errorClassifier:   NewErrorClassifier(),
		reprocessorClient: NewReprocessorClient(logger),
	}

	// Invalid JSON triggers reprocessing failure before any SQS calls.
	record := events.SQSMessage{
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-notification-processor-queue-dlq",
		MessageId:      "msg-2",
		ReceiptHandle:  "rh-2",
		Body:           `{"errorMessage":"rate limit exceeded"}`,
	}

	err := processor.processMessage(context.Background(), record)
	require.NoError(t, err)
	require.Len(t, dlqRepo.created, 1)
	require.Len(t, dlqRepo.updated, 1)
	require.Equal(t, models.DeliveryStatusFailed, dlqRepo.updated[0].Status)
	require.Len(t, costRepo.records, 1)
}

func TestProcessor_trackCosts_IncludesReprocessingCost(t *testing.T) {
	costRepo := &fakeCostRepo{}
	processor := &Processor{logger: zap.NewNop(), costTrackingRepo: costRepo}

	msg := &models.DLQMessage{
		ID:                "m1",
		Service:           "svc",
		ErrorType:         "err",
		Priority:          "high",
		Status:            "failed",
		ReprocessingCount: 2,
	}

	err := processor.trackCosts(context.Background(), msg, 1500*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(25), msg.ProcessingCostMicroCents)
	require.Equal(t, int64(20), msg.ReprocessingCostMicroCents)
	require.Len(t, costRepo.records, 1)
	require.Equal(t, int64(45), costRepo.records[0].TotalCostMicroCents)
}

func TestProcessor_reprocessServiceMessages_EmptyAndAbandon(t *testing.T) {
	t.Run("repo error is returned", func(t *testing.T) {
		processor := &Processor{
			logger:  zap.NewNop(),
			dlqRepo: &fakeDLQRepo{reprocessErr: stdErrors.New("boom")},
		}

		_, _, err := processor.reprocessServiceMessages(context.Background(), "svc", "new")
		require.Error(t, err)
	})

	t.Run("empty list returns nil", func(t *testing.T) {
		repo := &fakeDLQRepo{reprocessMessages: []*models.DLQMessage{}}
		processor := &Processor{
			logger:  zap.NewNop(),
			dlqRepo: repo,
		}

		processed, errors, err := processor.reprocessServiceMessages(context.Background(), "svc", "new")
		require.NoError(t, err)
		require.Equal(t, 0, processed)
		require.Equal(t, 0, errors)
		require.Empty(t, repo.updated)
	})

	t.Run("failed reprocessing can abandon", func(t *testing.T) {
		mockSQS := &MockSQSClient{}
		mockSQS.
			On("GetQueueUrl", mock.Anything, mock.Anything).
			Return((*sqs.GetQueueUrlOutput)(nil), stdErrors.New("no queue"))

		msg := &models.DLQMessage{
			ID:                   "m1",
			Service:              "unknown-service",
			SourceQueue:          "queue",
			Status:               "new",
			MaxReprocessAttempts: 1,
			MessageBody:          "{}",
			MessageAttributes:    map[string]string{},
			OriginalMessageID:    "orig-1",
		}
		skipped := &models.DLQMessage{
			ID:     "skip",
			Status: "resolved",
		}

		repo := &fakeDLQRepo{reprocessMessages: []*models.DLQMessage{skipped, msg}}
		processor := &Processor{
			logger:            zap.NewNop(),
			dlqRepo:           repo,
			errorClassifier:   NewErrorClassifier(),
			reprocessorClient: NewReprocessorClient(zap.NewNop()),
		}
		processor.reprocessorClient.SetSQSClient(mockSQS)

		processed, errors, err := processor.reprocessServiceMessages(context.Background(), "svc", "new")
		require.NoError(t, err)
		require.Equal(t, 0, processed)
		require.Equal(t, 1, errors)
		require.Len(t, repo.updated, 1)
		require.Equal(t, "abandoned", repo.updated[0].Status)
	})
}

func TestProcessor_CleanupAndPassThrough(t *testing.T) {
	repo := &fakeDLQRepo{cleanupCount: 5}
	processor := &Processor{logger: zap.NewNop(), dlqRepo: repo}

	require.NoError(t, processor.CleanupExpiredMessages(context.Background()))

	processor.dlqRepo = &fakeDLQRepo{cleanupErr: stdErrors.New("boom")}
	require.Error(t, processor.CleanupExpiredMessages(context.Background()))

	analytics := &repositories.DLQAnalytics{}
	trends := &repositories.DLQTrends{}
	search := []*models.DLQMessage{{ID: "m1"}}

	repo2 := &fakeDLQRepo{
		analytics:         analytics,
		trends:            trends,
		searchResults:     search,
		searchCursor:      "c1",
		reprocessMessages: nil,
	}
	processor.dlqRepo = repo2

	gotAnalytics, err := processor.GetAnalytics(context.Background(), "svc", repositories.DLQTimeRange{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
	})
	require.NoError(t, err)
	require.Same(t, analytics, gotAnalytics)

	gotTrends, err := processor.GetTrends(context.Background(), "svc", 7)
	require.NoError(t, err)
	require.Same(t, trends, gotTrends)

	gotMsgs, cursor, err := processor.SearchMessages(context.Background(), &repositories.DLQSearchFilter{})
	require.NoError(t, err)
	require.Equal(t, "c1", cursor)
	require.Equal(t, search, gotMsgs)
}
