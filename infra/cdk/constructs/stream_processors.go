package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type StreamProcessorsProps struct {
	Table     awsdynamodb.Table
	PushQueue awssqs.Queue
	Functions *LambdaFunctions
}

func CreateStreamProcessors(scope constructs.Construct, props *StreamProcessorsProps) {
	activityProcessor := props.Functions.Must("activity-processor")
	notificationProcessor := props.Functions.Must("push-delivery")
	outboxProcessor := props.Functions.Must("outbox")
	timelineProcessor := props.Functions.Must("trend-aggregator")
	moderationProcessor := props.Functions.Must("moderation-processor")
	searchIndexer := props.Functions.Must("status-indexer")
	mlTrainingProcessor := props.Functions.Must("ml-training-processor")
	severanceProcessor := props.Functions.Must("severance-processor")
	streamRouter := props.Functions.Must("stream-router")

	// Activity processor - handles new activities and status updates
	activityEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_TRIM_HORIZON,
		BatchSize:               jsii.Number(25),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(5),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	activityProcessor.AddEventSource(activityEventSource)

	// Notification processor - handles real-time notifications via SQS
	if props.PushQueue != nil {
		notificationEventSource := awslambdaeventsources.NewSqsEventSource(props.PushQueue, &awslambdaeventsources.SqsEventSourceProps{
			BatchSize:               jsii.Number(10),
			MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(1)),
			ReportBatchItemFailures: jsii.Bool(true),
		})
		notificationProcessor.AddEventSource(notificationEventSource)
	}

	// Federation outbox processor - handles outgoing federation
	outboxEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(2)),
		ParallelizationFactor:   jsii.Number(3),
		RetryAttempts:           jsii.Number(5),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		MaxRecordAge:            awscdk.Duration_Hours(jsii.Number(2)),
	})
	outboxProcessor.AddEventSource(outboxEventSource)

	// Timeline processor - handles timeline fanout
	timelineEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(50),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(10)),
		ParallelizationFactor:   jsii.Number(10),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	timelineProcessor.AddEventSource(timelineEventSource)

	// Moderation processor - handles content moderation
	moderationEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(2),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	moderationProcessor.AddEventSource(moderationEventSource)

	// Search indexer - handles search index updates
	searchEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(100),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(30)),
		ParallelizationFactor:   jsii.Number(5),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	searchIndexer.AddEventSource(searchEventSource)

	// ML Training processor - handles ML model training job lifecycle (Phase 2.3)
	mlTrainingEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(5), // Small batch size for training jobs
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(1)),
		ParallelizationFactor:   jsii.Number(1), // Sequential processing for job lifecycle
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	mlTrainingProcessor.AddEventSource(mlTrainingEventSource)

	// Severance processor - handles federation severance detection (Phase 2.4)
	severanceEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(2),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	severanceProcessor.AddEventSource(severanceEventSource)

	// Stream router - fan out streaming events to WebSocket subscribers
	streamRouterEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(50),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(2)),
		ParallelizationFactor:   jsii.Number(5),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	})
	streamRouter.AddEventSource(streamRouterEventSource)
}
