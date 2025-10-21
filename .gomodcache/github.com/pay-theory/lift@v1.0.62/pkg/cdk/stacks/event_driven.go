package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// EventDrivenStackProps defines properties for an event-driven stack
type EventDrivenStackProps struct {
	awscdk.StackProps
	AppName                string
	ApiCodePath            string
	EventProcessorCodePath string
	EventBusName           string
	EnableDLQ              bool
}

// NewEventDrivenStack creates an event-driven architecture stack
func NewEventDrivenStack(scope constructs.Construct, id string, props *EventDrivenStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Create or reference event bus
	var eventBus awsevents.IEventBus
	if props.EventBusName != "" && props.EventBusName != "default" {
		eventBus = awsevents.NewEventBus(stack, jsii.String("EventBus"), &awsevents.EventBusProps{
			EventBusName: jsii.String(props.EventBusName),
		})
	} else {
		eventBus = awsevents.EventBus_FromEventBusName(stack, jsii.String("DefaultBus"), jsii.String("default"))
	}

	// Create SQS queues for event processing
	dlq := awssqs.NewQueue(stack, jsii.String("DLQ"), &awssqs.QueueProps{
		QueueName:       jsii.String(props.AppName + "-dlq"),
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
	})

	eventQueue := awssqs.NewQueue(stack, jsii.String("EventQueue"), &awssqs.QueueProps{
		QueueName:         jsii.String(props.AppName + "-events"),
		VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(5)),
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			MaxReceiveCount: jsii.Number(3),
			Queue:           dlq,
		},
	})

	// Create event processor Lambda
	eventProcessor := liftconstructs.NewLiftFunction(stack, jsii.String("EventProcessor"), &liftconstructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String(props.AppName + "-event-processor"),
			Code:         awslambda.Code_FromAsset(jsii.String(props.EventProcessorCodePath), nil),
			Handler:      jsii.String("bootstrap"),
			Environment: &map[string]*string{
				"EVENT_BUS_NAME": eventBus.EventBusName(),
				"QUEUE_URL":      eventQueue.QueueUrl(),
			},
			ReservedConcurrentExecutions: jsii.Number(10),
		},
		EnableTracing: jsii.Bool(true),
	})

	// Add SQS event source
	eventProcessor.Function.AddEventSource(
		awslambdaeventsources.NewSqsEventSource(eventQueue, &awslambdaeventsources.SqsEventSourceProps{
			BatchSize:               jsii.Number(10),
			MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
			ReportBatchItemFailures: jsii.Bool(true),
		}),
	)

	// Grant permissions
	eventQueue.GrantConsumeMessages(eventProcessor.Function)
	eventBus.GrantPutEventsTo(eventProcessor.Function, nil)

	// Create API with event publishing capability
	apiEnv := map[string]*string{
		"EVENT_BUS_NAME":  eventBus.EventBusName(),
		"EVENT_QUEUE_URL": eventQueue.QueueUrl(),
	}

	apiFunction := liftconstructs.NewLiftFunction(stack, jsii.String("ApiFunction"), &liftconstructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String(props.AppName + "-api"),
			Code:         awslambda.Code_FromAsset(jsii.String(props.ApiCodePath), nil),
			Handler:      jsii.String("bootstrap"),
			Environment:  &apiEnv,
		},
		EnableTracing:     jsii.Bool(true),
		EnableMultiTenant: jsii.Bool(true),
	})

	// Grant API permissions to publish events
	eventBus.GrantPutEventsTo(apiFunction.Function, nil)
	eventQueue.GrantSendMessages(apiFunction.Function)

	// Create API Gateway
	api := liftconstructs.NewLiftAPI(stack, jsii.String("API"), &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:        jsii.String(props.AppName + "-api"),
			Description: jsii.String("Event-driven API for " + props.AppName),
			EnableCORS:  jsii.Bool(true),
		},
	})

	// Add routes
	api.AddLambdaRoute(jsii.String("/{proxy+}"), awsapigatewayv2.HttpMethod_ANY, apiFunction.Function)

	// Create EventBridge rules for common patterns
	orderRule := awsevents.NewRule(stack, jsii.String("OrderRule"), &awsevents.RuleProps{
		EventBus: eventBus,
		EventPattern: &awsevents.EventPattern{
			Source: &[]*string{jsii.String(props.AppName)},
			DetailType: &[]*string{
				jsii.String("Order Created"),
				jsii.String("Order Updated"),
				jsii.String("Order Canceled"),
			},
		},
	})
	orderRule.AddTarget(awseventstargets.NewSqsQueue(eventQueue, nil))

	// Create database table for event sourcing
	eventStore := liftconstructs.NewLiftTable(stack, jsii.String("EventStore"), &liftconstructs.LiftTableProps{
		TableName:                 jsii.String(props.AppName + "-events"),
		EnableStreams:             jsii.Bool(true),
		StreamViewType:            awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
		EnablePointInTimeRecovery: jsii.Bool(true),
	})

	// Grant access to event store
	eventStore.GrantReadWrite(apiFunction.Function)
	eventStore.GrantReadWrite(eventProcessor.Function)

	// Add event store table name to environment
	apiFunction.Function.AddEnvironment(jsii.String("EVENT_STORE_TABLE"), eventStore.Table.TableName(), nil)
	eventProcessor.Function.AddEnvironment(jsii.String("EVENT_STORE_TABLE"), eventStore.Table.TableName(), nil)

	// Outputs
	awscdk.NewCfnOutput(stack, jsii.String("ApiEndpoint"), &awscdk.CfnOutputProps{
		Value:       api.HttpAPI.ApiEndpoint(),
		Description: jsii.String("API Gateway endpoint"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("EventBusName"), &awscdk.CfnOutputProps{
		Value:       eventBus.EventBusName(),
		Description: jsii.String("EventBridge event bus name"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("EventQueueUrl"), &awscdk.CfnOutputProps{
		Value:       eventQueue.QueueUrl(),
		Description: jsii.String("SQS event queue URL"),
	})

	if props.EnableDLQ {
		awscdk.NewCfnOutput(stack, jsii.String("DeadLetterQueueUrl"), &awscdk.CfnOutputProps{
			Value:       dlq.QueueUrl(),
			Description: jsii.String("Dead letter queue URL"),
		})
	}

	return stack
}
