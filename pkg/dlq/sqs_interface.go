package dlq

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSClient defines the interface for SQS operations used by the DLQ package.
// This interface allows for easy mocking in unit tests.
type SQSClient interface {
	GetQueueUrl(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	SendMessageBatch(ctx context.Context, params *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
}

// Ensure *sqs.Client implements SQSClient
var _ SQSClient = (*sqs.Client)(nil)
