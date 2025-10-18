package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type StreamProcessorsProps struct {
	Table     awsdynamodb.Table
	Functions *LambdaFunctions
}

func CreateStreamProcessors(scope constructs.Construct, props *StreamProcessorsProps) {
	// Activity processor - handles new activities and status updates
	activityEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_TRIM_HORIZON,
		BatchSize:               jsii.Number(25),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(5),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
	})
	props.Functions.ActivityProcessor.AddEventSource(activityEventSource)

	// Notification processor - handles real-time notifications
	notificationEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(1)),
		ParallelizationFactor:   jsii.Number(2),
		RetryAttempts:           jsii.Number(2),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
	})
	props.Functions.NotificationProcessor.AddEventSource(notificationEventSource)

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
		// Remove filters completely to fix deployment issue
	})
	props.Functions.OutboxFunction.AddEventSource(outboxEventSource)

	// Timeline processor - handles timeline fanout
	timelineEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(50),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(10)),
		ParallelizationFactor:   jsii.Number(10),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
	})
	props.Functions.TimelineProcessorFunction.AddEventSource(timelineEventSource)

	// Moderation processor - handles content moderation
	moderationEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(2),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
	})
	props.Functions.ModerationProcessor.AddEventSource(moderationEventSource)

	// Search indexer - handles search index updates
	searchEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(100),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(30)),
		ParallelizationFactor:   jsii.Number(5),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
	})
	props.Functions.SearchIndexerFunction.AddEventSource(searchEventSource)

	// ML Training processor - handles ML model training job lifecycle (Phase 2.3)
	mlTrainingEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(5), // Small batch size for training jobs
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(1)),
		ParallelizationFactor:   jsii.Number(1), // Sequential processing for job lifecycle
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
		// Filter for MODEL_TRAINING_JOB records handled in Lambda code
	})
	props.Functions.MLTrainingProcessor.AddEventSource(mlTrainingEventSource)

	// Severance processor - handles federation severance detection (Phase 2.4)
	severanceEventSource := awslambdaeventsources.NewDynamoEventSource(props.Table, &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(10),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		ParallelizationFactor:   jsii.Number(2),
		RetryAttempts:           jsii.Number(3),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		// Remove filters completely to fix deployment issue
		// Filter for DOMAIN_BLOCK, FEDERATION_ISSUE, and FEDERATION_METRICS records handled in Lambda code
	})
	props.Functions.SeveranceProcessor.AddEventSource(severanceEventSource)
}
