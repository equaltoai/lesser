package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk/v3"
)

type StreamProcessorsProps struct {
	AppName       string
	Environment   string
	RemovalPolicy awscdk.RemovalPolicy
	Table         awsdynamodb.Table
	Queues        map[string]QueuePair
	Functions     *LambdaFunctions
}

func CreateStreamProcessors(scope constructs.Construct, props *StreamProcessorsProps) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		hasStreamTriggers := len(spec.StreamTriggers) > 0
		hasSQSTriggers := len(spec.SQSTriggers) > 0
		if !hasStreamTriggers && !hasSQSTriggers {
			continue
		}

		handler := props.Functions.Must(spec.Name)

		if hasStreamTriggers {
			validateStreamCapable(spec)
			for idx, trig := range spec.StreamTriggers {
				table := resolveStreamTable(spec, trig, props.Table)
				table.GrantStreamRead(handler)
				poisonQueue := createStreamPoisonQueue(scope, props, handler, spec, trig, idx)

				startPos := awslambda.StartingPosition_LATEST
				if trig.StartingPosition == inventory.StreamStartTrimHorizon {
					startPos = awslambda.StartingPosition_TRIM_HORIZON
				}

				// AppTheoryDynamoDBStreamMapping currently has no OnFailure destination prop.
				// Keep the inventory-driven shape but use the AWS CDK DynamoEventSource so
				// poison records are captured instead of being retried until stream expiry.
				eventSourceProps := &awslambdaeventsources.DynamoEventSourceProps{
					StartingPosition: startPos,
					OnFailure:        awslambdaeventsources.NewSqsDlq(poisonQueue),
				}

				if trig.BatchSize > 0 {
					eventSourceProps.BatchSize = jsii.Number(float64(trig.BatchSize))
				}
				if trig.MaxBatchingWindowSeconds > 0 {
					eventSourceProps.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxBatchingWindowSeconds)))
				}
				if trig.ParallelizationFactor > 0 {
					eventSourceProps.ParallelizationFactor = jsii.Number(float64(trig.ParallelizationFactor))
				}
				if trig.MaxRetryAttempts > 0 {
					eventSourceProps.RetryAttempts = jsii.Number(float64(trig.MaxRetryAttempts))
				}
				if trig.MaxRecordAgeSeconds > 0 {
					eventSourceProps.MaxRecordAge = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxRecordAgeSeconds)))
				}
				if trig.EnableBisectOnError {
					eventSourceProps.BisectBatchOnError = jsii.Bool(true)
				}
				if trig.ReportBatchItemFailures {
					eventSourceProps.ReportBatchItemFailures = jsii.Bool(true)
				}

				handler.AddEventSource(awslambdaeventsources.NewDynamoEventSource(table, eventSourceProps))
			}
		}

		if hasSQSTriggers {
			validateSQSCapable(spec)
			for _, trig := range spec.SQSTriggers {
				if !trig.ConsumeDeadLetterQueue {
					// Primary SQS consumers are wired when queues are created (LiftSQSQueue).
					continue
				}
				queue := resolveQueue(spec, trig, props.Queues)
				eventSourceProps := buildSQSEventSourceProps(trig)
				handler.AddEventSource(awslambdaeventsources.NewSqsEventSource(queue, eventSourceProps))
			}
		}
	}
}

func createStreamPoisonQueue(scope constructs.Construct, props *StreamProcessorsProps, handler awslambda.Function, spec inventory.LambdaSpec, trig inventory.StreamTrigger, idx int) awssqs.IQueue {
	if strings.TrimSpace(trig.PoisonRecordQueue) == "" {
		panic(fmt.Sprintf("lambda %s stream trigger %d requires a poison record queue", spec.Name, idx))
	}

	queueName := naming.ResourceNameWithApp(props.AppName, trig.PoisonRecordQueue, props.Environment)
	removalPolicy := props.RemovalPolicy
	if removalPolicy == "" {
		removalPolicy = awscdk.RemovalPolicy_DESTROY
	}

	queue := apptheorycdk.NewAppTheoryQueue(scope, jsii.String(fmt.Sprintf("%sStreamPoisonQueue%d", sanitizeStreamMappingID(spec.Name), idx)), &apptheorycdk.AppTheoryQueueProps{
		QueueName:         jsii.String(queueName),
		EnableDlq:         jsii.Bool(false),
		RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)),
		RemovalPolicy:     removalPolicy,
		VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(2)),
	})
	poisonQueue := queue.Queue()
	appName := strings.TrimSpace(props.AppName)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	awscdk.Tags_Of(poisonQueue).Add(jsii.String("app"), jsii.String(appName), nil)
	awscdk.Tags_Of(poisonQueue).Add(jsii.String("stage"), jsii.String(string(naming.StageForEnvironment(props.Environment))), nil)
	poisonQueue.GrantSendMessages(handler)
	return poisonQueue
}

func validateStreamCapable(spec inventory.LambdaSpec) {
	if spec.Type != inventory.LambdaTypeProcessorStream && spec.Type != inventory.LambdaTypeHybrid {
		panic(fmt.Sprintf("lambda %s has stream triggers but type %s does not support streams", spec.Name, spec.Type))
	}
}

func validateSQSCapable(spec inventory.LambdaSpec) {
	if spec.Type != inventory.LambdaTypeProcessorSQS && spec.Type != inventory.LambdaTypeHybrid {
		panic(fmt.Sprintf("lambda %s has SQS triggers but type %s does not support SQS", spec.Name, spec.Type))
	}
}

func resolveStreamTable(spec inventory.LambdaSpec, trig inventory.StreamTrigger, defaultTable awsdynamodb.Table) awsdynamodb.Table {
	if trig.SourceTable == "" || trig.SourceTable == "main-table" {
		return defaultTable
	}
	panic(fmt.Sprintf("lambda %s stream trigger references unsupported table %s", spec.Name, trig.SourceTable))
}

func resolveQueue(spec inventory.LambdaSpec, trig inventory.SQSTrigger, queues map[string]QueuePair) awssqs.IQueue {
	qp, ok := queues[trig.Queue]
	if !ok {
		panic(fmt.Sprintf("lambda %s requires queue %s but it was not created (check inventory vs Phase 2 queues)", spec.Name, trig.Queue))
	}

	if trig.ConsumeDeadLetterQueue {
		if qp.DLQ == nil {
			panic(fmt.Sprintf("lambda %s requires DLQ for queue %s but it was not created", spec.Name, trig.Queue))
		}
		return qp.DLQ
	}

	if qp.Primary != nil {
		return qp.Primary
	}
	panic(fmt.Sprintf("lambda %s requires primary queue %s but it was not created", spec.Name, trig.Queue))
}

func buildSQSEventSourceProps(trig inventory.SQSTrigger) *awslambdaeventsources.SqsEventSourceProps {
	props := &awslambdaeventsources.SqsEventSourceProps{}
	if trig.BatchSize > 0 {
		props.BatchSize = jsii.Number(float64(trig.BatchSize))
	}
	if trig.MaxBatchingWindowSeconds > 0 {
		props.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxBatchingWindowSeconds)))
	}
	if trig.EnablePartialFailure {
		props.ReportBatchItemFailures = jsii.Bool(true)
	}
	return props
}

func sanitizeStreamMappingID(name string) string {
	clean := strings.ReplaceAll(name, "-", "")
	clean = strings.ReplaceAll(clean, "_", "")
	if clean == "" {
		return "Stream"
	}
	return clean
}
