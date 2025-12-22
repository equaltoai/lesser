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
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type StreamProcessorsProps struct {
	Table     awsdynamodb.Table
	Queues    map[string]QueuePair
	Functions *LambdaFunctions
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
				eventSourceID := fmt.Sprintf("%sStreamMapping%d", sanitizeStreamMappingID(spec.Name), idx)
				eventSourceProps := buildStreamEventSourceMappingProps(handler, table, trig)
				table.GrantStreamRead(handler)
				_ = liftcdk.NewLiftEventSourceMapping(scope, jsii.String(eventSourceID), eventSourceProps)
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

func buildStreamEventSourceProps(trig inventory.StreamTrigger) *awslambdaeventsources.DynamoEventSourceProps {
	startPos := awslambda.StartingPosition_LATEST
	if trig.StartingPosition == inventory.StreamStartTrimHorizon {
		startPos = awslambda.StartingPosition_TRIM_HORIZON
	}

	props := &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition: startPos,
	}

	if trig.BatchSize > 0 {
		props.BatchSize = jsii.Number(float64(trig.BatchSize))
	}
	if trig.MaxBatchingWindowSeconds > 0 {
		props.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxBatchingWindowSeconds)))
	}
	if trig.ParallelizationFactor > 0 {
		props.ParallelizationFactor = jsii.Number(float64(trig.ParallelizationFactor))
	}
	if trig.MaxRetryAttempts > 0 {
		props.RetryAttempts = jsii.Number(float64(trig.MaxRetryAttempts))
	}
	if trig.MaxRecordAgeSeconds > 0 {
		props.MaxRecordAge = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxRecordAgeSeconds)))
	}
	if trig.EnableBisectOnError {
		props.BisectBatchOnError = jsii.Bool(true)
	}
	if trig.ReportBatchItemFailures {
		props.ReportBatchItemFailures = jsii.Bool(true)
	}

	return props
}

func buildStreamEventSourceMappingProps(handler awslambda.IFunction, table awsdynamodb.Table, trig inventory.StreamTrigger) *liftcdk.LiftEventSourceMappingProps {
	startPos := awslambda.StartingPosition_LATEST
	if trig.StartingPosition == inventory.StreamStartTrimHorizon {
		startPos = awslambda.StartingPosition_TRIM_HORIZON
	}

	props := &liftcdk.LiftEventSourceMappingProps{
		TargetFunction:   handler,
		EventSourceArn:   table.TableStreamArn(),
		StartingPosition: startPos,
		ReportBatchItemFailures: func() *bool {
			if trig.ReportBatchItemFailures {
				return jsii.Bool(true)
			}
			return nil
		}(),
	}

	if trig.BatchSize > 0 {
		props.BatchSize = jsii.Number(float64(trig.BatchSize))
	}
	if trig.MaxRetryAttempts > 0 {
		props.RetryAttempts = jsii.Number(float64(trig.MaxRetryAttempts))
	}
	if trig.ParallelizationFactor > 0 {
		props.ParallelizationFactor = jsii.Number(float64(trig.ParallelizationFactor))
	}
	if trig.MaxBatchingWindowSeconds > 0 {
		props.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxBatchingWindowSeconds)))
	}
	if trig.MaxRecordAgeSeconds > 0 {
		props.MaxRecordAge = awscdk.Duration_Seconds(jsii.Number(float64(trig.MaxRecordAgeSeconds)))
	}
	if trig.EnableBisectOnError {
		props.BisectBatchOnError = jsii.Bool(true)
	}

	return props
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
