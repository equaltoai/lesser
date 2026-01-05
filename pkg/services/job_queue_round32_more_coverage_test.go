package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobQueueService_Round32_QueueExportJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("missing queue url skips", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)
		q.queueUrls["export-generation"] = ""

		require.NoError(t, q.QueueExportJob(ctx, ExportJobMessage{ExportID: "e1", Username: "alice", Type: "accounts"}))
		assert.Empty(t, mockClient.sendMessageInputs)
	})

	t.Run("success sets timestamp and sends message", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)

		require.NoError(t, q.QueueExportJob(ctx, ExportJobMessage{ExportID: "e2", Username: "alice", Type: "accounts", Format: "json", Timestamp: 0}))
		require.Len(t, mockClient.sendMessageInputs, 1)

		input := mockClient.sendMessageInputs[0]
		require.NotNil(t, input.QueueUrl)
		assert.Equal(t, q.queueUrls["export-generation"], aws.ToString(input.QueueUrl))
		assert.Equal(t, int32(5), input.DelaySeconds)
		assert.Equal(t, "ExportJob", aws.ToString(input.MessageAttributes["Type"].StringValue))
		assert.Equal(t, "alice", aws.ToString(input.MessageAttributes["Username"].StringValue))
		assert.Equal(t, "accounts", aws.ToString(input.MessageAttributes["ExportType"].StringValue))
		assert.Equal(t, "json", aws.ToString(input.MessageAttributes["Format"].StringValue))

		var decoded ExportJobMessage
		require.NoError(t, json.Unmarshal([]byte(aws.ToString(input.MessageBody)), &decoded))
		assert.Equal(t, "e2", decoded.ExportID)
		assert.NotZero(t, decoded.Timestamp)
	})

	t.Run("serialization failure returns expected error", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)

		err := q.QueueExportJob(ctx, ExportJobMessage{
			ExportID: "e3",
			Username: "alice",
			Type:     "accounts",
			Options:  map[string]any{"bad": func() {}},
		})
		assert.ErrorIs(t, err, ErrExportJobSerialization)
	})

	t.Run("send failure returns expected error", func(t *testing.T) {
		mockClient := &mockSQSClient{sendMessageErr: errors.New("boom")}
		q := newTestJobQueue(mockClient)

		err := q.QueueExportJob(ctx, ExportJobMessage{ExportID: "e4", Username: "alice", Type: "accounts", Format: "json"})
		assert.ErrorIs(t, err, ErrExportJobQueue)
	})
}

func TestJobQueueService_Round32_QueueMediaJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("missing queue url skips", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)
		q.queueUrls["media-processing"] = ""

		require.NoError(t, q.QueueMediaJob(ctx, MediaJobMessage{JobID: "m1", MediaID: "media-1", Username: "alice"}))
		assert.Empty(t, mockClient.sendMessageInputs)
	})

	t.Run("success sets timestamp and sends message", func(t *testing.T) {
		mockClient := &mockSQSClient{}
		q := newTestJobQueue(mockClient)

		require.NoError(t, q.QueueMediaJob(ctx, MediaJobMessage{JobID: "m2", MediaID: "media-2", Username: "alice", Timestamp: 0}))
		require.Len(t, mockClient.sendMessageInputs, 1)

		input := mockClient.sendMessageInputs[0]
		require.NotNil(t, input.QueueUrl)
		assert.Equal(t, q.queueUrls["media-processing"], aws.ToString(input.QueueUrl))
		assert.Equal(t, int32(2), input.DelaySeconds)
		assert.Equal(t, "MediaJob", aws.ToString(input.MessageAttributes["Type"].StringValue))
		assert.Equal(t, "alice", aws.ToString(input.MessageAttributes["Username"].StringValue))
		assert.Equal(t, "media-2", aws.ToString(input.MessageAttributes["MediaID"].StringValue))

		var decoded MediaJobMessage
		require.NoError(t, json.Unmarshal([]byte(aws.ToString(input.MessageBody)), &decoded))
		assert.Equal(t, "m2", decoded.JobID)
		assert.NotZero(t, decoded.Timestamp)
	})

	t.Run("send failure returns expected error", func(t *testing.T) {
		mockClient := &mockSQSClient{sendMessageErr: errors.New("boom")}
		q := newTestJobQueue(mockClient)

		err := q.QueueMediaJob(ctx, MediaJobMessage{JobID: "m4", MediaID: "media-4", Username: "alice"})
		assert.ErrorIs(t, err, ErrMediaJobQueue)
	})
}
