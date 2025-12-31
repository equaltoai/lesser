package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockSQSClient struct {
	sendMessageInputs      []*sqs.SendMessageInput
	sendMessageErr         error
	sendMessageBatchInputs []*sqs.SendMessageBatchInput
	sendMessageBatchErr    error
	sendMessageBatchOutput *sqs.SendMessageBatchOutput
	getQueueAttrInputs     []*sqs.GetQueueAttributesInput
	getQueueAttrErr        error
	getQueueAttrOutput     *sqs.GetQueueAttributesOutput
}

func (m *mockSQSClient) SendMessage(_ context.Context, params *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	m.sendMessageInputs = append(m.sendMessageInputs, params)
	if m.sendMessageErr != nil {
		return nil, m.sendMessageErr
	}
	return &sqs.SendMessageOutput{}, nil
}

func (m *mockSQSClient) SendMessageBatch(_ context.Context, params *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	m.sendMessageBatchInputs = append(m.sendMessageBatchInputs, params)
	if m.sendMessageBatchErr != nil {
		return nil, m.sendMessageBatchErr
	}
	if m.sendMessageBatchOutput != nil {
		return m.sendMessageBatchOutput, nil
	}
	return &sqs.SendMessageBatchOutput{}, nil
}

func (m *mockSQSClient) GetQueueAttributes(_ context.Context, params *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	m.getQueueAttrInputs = append(m.getQueueAttrInputs, params)
	if m.getQueueAttrErr != nil {
		return nil, m.getQueueAttrErr
	}
	if m.getQueueAttrOutput != nil {
		return m.getQueueAttrOutput, nil
	}
	return &sqs.GetQueueAttributesOutput{Attributes: map[string]string{}}, nil
}

func newTestJobQueue(client *mockSQSClient) *JobQueueService {
	return &JobQueueService{
		sqsClient: client,
		queueUrls: map[string]string{
			"import-processing":    "https://sqs.example.com/import",
			"export-generation":    "https://sqs.example.com/export",
			"media-processing":     "https://sqs.example.com/media",
			"scheduled-publishing": "https://sqs.example.com/scheduled",
			"federation-delivery":  "https://sqs.example.com/federation",
		},
		logger: zap.NewNop(),
	}
}

func TestJobQueueService_Round25_QueueImportJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("missing queue url skips", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)
		q.queueUrls["import-processing"] = ""

		err := q.QueueImportJob(ctx, ImportJobMessage{ImportID: "i1", Username: "alice", Type: "mastodon"})
		require.NoError(t, err)
		assert.Empty(t, mockClient.sendMessageInputs)
	})

	t.Run("success sets timestamp and sends message", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)

		err := q.QueueImportJob(ctx, ImportJobMessage{ImportID: "i2", Username: "alice", Type: "mastodon", Timestamp: 0})
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageInputs, 1)

		input := mockClient.sendMessageInputs[0]
		require.NotNil(t, input.QueueUrl)
		assert.Equal(t, q.queueUrls["import-processing"], aws.ToString(input.QueueUrl))
		assert.Equal(t, int32(5), input.DelaySeconds)
		assert.Equal(t, "ImportJob", aws.ToString(input.MessageAttributes["Type"].StringValue))
		assert.Equal(t, "alice", aws.ToString(input.MessageAttributes["Username"].StringValue))

		var decoded ImportJobMessage
		require.NoError(t, json.Unmarshal([]byte(aws.ToString(input.MessageBody)), &decoded))
		assert.Equal(t, "i2", decoded.ImportID)
		assert.NotZero(t, decoded.Timestamp)
	})

	t.Run("serialization failure returns expected error", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)

		err := q.QueueImportJob(ctx, ImportJobMessage{
			ImportID: "i3",
			Username: "alice",
			Type:     "mastodon",
			Options:  map[string]any{"bad": func() {}},
		})
		assert.ErrorIs(t, err, ErrImportJobSerialization)
	})
}

func TestJobQueueService_Round25_QueueScheduledJob_delayBounds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := &mockSQSClient{}
	q := newTestJobQueue(mockClient)

	t.Run("past scheduled time uses minimum delay", func(t *testing.T) {
		mockClient.sendMessageInputs = nil
		err := q.QueueScheduledJob(ctx, ScheduledJobMessage{ScheduledStatusID: "s1", Username: "alice", ScheduledAt: time.Now().Add(-1 * time.Hour)})
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageInputs, 1)
		assert.Equal(t, int32(5), mockClient.sendMessageInputs[0].DelaySeconds)
	})

	t.Run("far future caps at 900 seconds", func(t *testing.T) {
		mockClient.sendMessageInputs = nil
		err := q.QueueScheduledJob(ctx, ScheduledJobMessage{ScheduledStatusID: "s2", Username: "alice", ScheduledAt: time.Now().Add(48 * time.Hour)})
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageInputs, 1)
		assert.Equal(t, int32(900), mockClient.sendMessageInputs[0].DelaySeconds)
	})
}

func TestJobQueueService_Round25_QueueActivityJob_priorityDelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := &mockSQSClient{}
	q := newTestJobQueue(mockClient)

	tests := []struct {
		name     string
		priority string
		want     int32
	}{
		{name: "high", priority: priorityHigh, want: 0},
		{name: "normal", priority: priorityNormal, want: 5},
		{name: "low", priority: "low", want: 30},
		{name: "default", priority: "", want: 5},
		{name: "unknown", priority: "weird", want: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient.sendMessageInputs = nil
			err := q.QueueActivityJob(ctx, ActivityJobMessage{
				ActivityID: "a1",
				ActorID:    "actor-1",
				Priority:   tc.priority,
				Recipients: []string{"r1", "r2"},
			})
			require.NoError(t, err)
			require.Len(t, mockClient.sendMessageInputs, 1)
			assert.Equal(t, tc.want, mockClient.sendMessageInputs[0].DelaySeconds)
		})
	}
}

func TestJobQueueService_Round25_QueueDelayedJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := &mockSQSClient{}
	q := newTestJobQueue(mockClient)

	t.Run("missing queue url returns expected error", func(t *testing.T) {
		err := q.QueueDelayedJob(ctx, "missing", map[string]any{"k": "v"}, 0)
		assert.ErrorIs(t, err, ErrQueueURLNotConfigured)
	})

	t.Run("serialization failure returns expected error", func(t *testing.T) {
		err := q.QueueDelayedJob(ctx, "import-processing", func() {}, 0)
		assert.ErrorIs(t, err, ErrMessageSerialization)
	})

	t.Run("send failure returns expected error", func(t *testing.T) {
		mockClient.sendMessageErr = errors.New("boom")
		err := q.QueueDelayedJob(ctx, "import-processing", map[string]any{"k": "v"}, 7)
		assert.ErrorIs(t, err, ErrDelayedJobQueue)
		mockClient.sendMessageErr = nil
	})
}

func TestJobQueueService_Round25_SendBatchMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := &mockSQSClient{}
	q := newTestJobQueue(mockClient)

	t.Run("missing queue url returns expected error", func(t *testing.T) {
		err := q.SendBatchMessages(ctx, "missing", []interface{}{"a"})
		assert.ErrorIs(t, err, ErrQueueURLNotConfigured)
	})

	t.Run("serialization failure returns expected error", func(t *testing.T) {
		err := q.SendBatchMessages(ctx, "import-processing", []interface{}{func() {}})
		assert.ErrorIs(t, err, ErrMessageSerialization)
	})

	t.Run("splits into multiple batches of up to 10", func(t *testing.T) {
		mockClient.sendMessageBatchInputs = nil
		msgs := make([]interface{}, 0, 11)
		for i := 0; i < 11; i++ {
			msgs = append(msgs, map[string]any{"i": i})
		}
		err := q.SendBatchMessages(ctx, "import-processing", msgs)
		require.NoError(t, err)
		require.Len(t, mockClient.sendMessageBatchInputs, 2)
		assert.Len(t, mockClient.sendMessageBatchInputs[0].Entries, 10)
		assert.Len(t, mockClient.sendMessageBatchInputs[1].Entries, 1)
	})

	t.Run("batch send error returns expected error", func(t *testing.T) {
		mockClient.sendMessageBatchErr = errors.New("boom")
		err := q.SendBatchMessages(ctx, "import-processing", []interface{}{map[string]any{"k": "v"}})
		assert.ErrorIs(t, err, ErrBatchMessageSend)
		mockClient.sendMessageBatchErr = nil
	})

	t.Run("batch operation with failed items returns expected error", func(t *testing.T) {
		mockClient.sendMessageBatchOutput = &sqs.SendMessageBatchOutput{
			Failed: []types.BatchResultErrorEntry{
				{
					Id:      aws.String("msg-0"),
					Code:    aws.String("AccessDenied"),
					Message: aws.String("nope"),
				},
			},
		}
		err := q.SendBatchMessages(ctx, "import-processing", []interface{}{map[string]any{"k": "v"}})
		assert.ErrorIs(t, err, ErrBatchOperation)
		mockClient.sendMessageBatchOutput = nil
	})
}

func TestJobQueueService_Round25_GetQueueAttributes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockClient := &mockSQSClient{}
	q := newTestJobQueue(mockClient)

	t.Run("missing queue url returns expected error", func(t *testing.T) {
		_, err := q.GetQueueAttributes(ctx, "missing")
		assert.ErrorIs(t, err, ErrQueueURLNotConfigured)
	})

	t.Run("client error returns expected error", func(t *testing.T) {
		mockClient.getQueueAttrErr = errors.New("boom")
		_, err := q.GetQueueAttributes(ctx, "import-processing")
		assert.ErrorIs(t, err, ErrQueueAttributeQuery)
		mockClient.getQueueAttrErr = nil
	})

	t.Run("success returns attributes", func(t *testing.T) {
		mockClient.getQueueAttrOutput = &sqs.GetQueueAttributesOutput{
			Attributes: map[string]string{
				string(types.QueueAttributeNameApproximateNumberOfMessages): "3",
			},
		}
		attrs, err := q.GetQueueAttributes(ctx, "import-processing")
		require.NoError(t, err)
		assert.Equal(t, "3", attrs[string(types.QueueAttributeNameApproximateNumberOfMessages)])
		mockClient.getQueueAttrOutput = nil
	})
}

