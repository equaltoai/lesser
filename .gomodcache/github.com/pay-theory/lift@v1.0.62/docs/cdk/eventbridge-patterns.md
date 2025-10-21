# EventBridge CDK Patterns

This guide covers how to use the `EventBridgeHandler` construct for building event-driven architectures with AWS CDK and Lift.

## Overview

The `EventBridgeHandler` construct provides a type-safe, production-ready way to process EventBridge events with Lambda functions. It automatically configures:

- EventBridge rules with event patterns or schedules
- Lambda functions optimized for Lift
- Dead letter queues for error handling
- IAM permissions and environment variables
- CloudWatch monitoring integration

## Basic Usage

### Simple Event Handler

```go
import "github.com/pay-theory/lift/pkg/cdk/constructs"

// Handle all events from a specific source
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("OrderHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("order-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Scheduled Event Handler

```go
// Process events on a schedule
scheduler, err := constructs.NewEventBridgeHandler(stack, jsii.String("DailyReport"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("daily-report-generator"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    ScheduleExpression: jsii.String("rate(1 day)"),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Event Pattern Examples

### Filter by Event Type

```go
EventPattern: &awsevents.EventPattern{
    Source:     &[]*string{jsii.String("myapp.orders")},
    DetailType: &[]*string{jsii.String("Order Placed"), jsii.String("Order Updated")},
}
```

### Filter by Event Details

```go
EventPattern: &awsevents.EventPattern{
    Source:     &[]*string{jsii.String("myapp.orders")},
    DetailType: &[]*string{jsii.String("Order Placed")},
    Detail: &map[string]interface{}{
        "state": &[]*string{jsii.String("pending"), jsii.String("processing")},
        "amount": map[string]interface{}{
            "numeric": &[]*string{jsii.String(">"), jsii.String("100")},
        },
    },
}
```

### Multiple Sources

```go
EventPattern: &awsevents.EventPattern{
    Source: &[]*string{
        jsii.String("myapp.orders"),
        jsii.String("myapp.payments"),
        jsii.String("myapp.inventory"),
    },
}
```

## Schedule Expressions

### Rate-based Schedules

```go
// Every 5 minutes
ScheduleExpression: jsii.String("rate(5 minutes)")

// Every hour
ScheduleExpression: jsii.String("rate(1 hour)")

// Every day
ScheduleExpression: jsii.String("rate(1 day)")
```

### Cron-based Schedules

```go
// Every weekday at 9 AM UTC
ScheduleExpression: jsii.String("cron(0 9 ? * MON-FRI *)")

// First day of every month at midnight
ScheduleExpression: jsii.String("cron(0 0 1 * ? *)")

// Every 15 minutes during business hours
ScheduleExpression: jsii.String("cron(*/15 9-17 ? * MON-FRI *)")
```

## Custom Event Bus

### Creating a Custom Event Bus

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("CustomEventHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("custom-event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventBusProps: &awsevents.EventBusProps{
        EventBusName: jsii.String("my-custom-event-bus"),
        Description:  jsii.String("Custom event bus for microservice communication"),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("microservice.user")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Using an Existing Event Bus

```go
existingBus := awsevents.EventBus_FromEventBusName(stack, jsii.String("ExistingBus"), jsii.String("existing-event-bus"))

handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("ExistingBusHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("existing-bus-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    ExistingEventBus: existingBus,
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("external.system")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Cross-Account Event Processing

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("CrossAccountHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("cross-account-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    CrossAccountEventBusArn: jsii.String("arn:aws:events:us-east-1:123456789012:event-bus/shared-bus"),
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("shared.events")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Error Handling and Dead Letter Queues

### Default DLQ Configuration

```go
// DLQ is enabled by default with sensible defaults
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("EventHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}

// Access DLQ for monitoring or processing
dlqUrl := handler.DeadLetterQueue.QueueUrl()
```

### Custom DLQ Configuration

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("EventHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
    DeadLetterQueueProps: &awssqs.QueueProps{
        QueueName:       jsii.String("custom-eventbridge-dlq"),
        RetentionPeriod: awscdk.Duration_Days(jsii.Number(7)),
        VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(1)),
    },
    MaxEventAge:   awscdk.Duration_Hours(jsii.Number(2)),
    RetryAttempts: jsii.Number(5),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Disable DLQ

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("EventHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
    EnableDeadLetterQueue: jsii.Bool(false),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Environment Variables

The construct automatically injects these environment variables into your Lambda function:

- `EVENT_BUS_NAME`: Name of the event bus (always set)
- `EVENT_BUS_ARN`: ARN of the event bus (always set)
- `EVENTBRIDGE_DLQ_URL`: URL of the dead letter queue (only set when DLQ is enabled)

```go
// In your Lambda function
eventBusName := os.Getenv("EVENT_BUS_NAME")
eventBusArn := os.Getenv("EVENT_BUS_ARN")
dlqUrl := os.Getenv("EVENTBRIDGE_DLQ_URL") // May be empty if DLQ is disabled
```

## Lift Integration Features

### Multi-Tenant Support

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("MultiTenantHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("tenant-event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("tenant.events")},
        Detail: &map[string]interface{}{
            "tenantId": &[]*string{jsii.String("*")},
        },
    },
    EnableMultiTenant: jsii.Bool(true),
    EnableTracing:     jsii.Bool(true),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Monitoring and Observability

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("MonitoredHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("monitored-event-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
    EnableTracing:    jsii.Bool(true),
    EnableMonitoring: jsii.Bool(true),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

When `EnableMonitoring` is enabled, the construct automatically creates CloudWatch alarms for:

- **Function Error Rate:** Alerts when Lambda function errors exceed 3 errors in 2 evaluation periods
- **Function Duration:** Alerts when Lambda function duration exceeds 30 seconds in 3 evaluation periods  
- **Rule Failures:** Alerts when EventBridge rule invocation failures exceed 5 failures in 2 evaluation periods
- **DLQ Messages:** Alerts when messages in the dead letter queue exceed 10 messages (if DLQ is enabled)

All alarms use 5-minute evaluation periods and are configured with appropriate thresholds for production workloads.

## Advanced Patterns

### Fan-out Pattern

```go
// Create multiple handlers for the same event
orderHandler, err := constructs.NewEventBridgeHandler(stack, jsii.String("OrderProcessor"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("order-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source:     &[]*string{jsii.String("myapp.orders")},
        DetailType: &[]*string{jsii.String("Order Placed")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}

inventoryHandler, err := constructs.NewEventBridgeHandler(stack, jsii.String("InventoryUpdater"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("inventory-updater"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source:     &[]*string{jsii.String("myapp.orders")},
        DetailType: &[]*string{jsii.String("Order Placed")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Event Transformation

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("TransformHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("transform-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
    InputTransformation: awsevents.RuleTargetInput_FromObject(&map[string]interface{}{
        "orderId":    awsevents.EventField_FromPath(jsii.String("$.detail.orderId")),
        "customerId": awsevents.EventField_FromPath(jsii.String("$.detail.customerId")),
        "timestamp":  awsevents.EventField_FromPath(jsii.String("$.time")),
        "eventType":  awsevents.EventField_FromPath(jsii.String("$.detail-type")),
    }),
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Helper Methods

### Grant Permissions

```go
// Grant other functions permission to put events to the bus
producer, err := constructs.NewLiftFunction(stack, jsii.String("EventProducer"), &constructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("event-producer"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create LiftFunction: %v", err))
}

handler.GrantPutEvents(producer.Function)
```

### Add Environment Variables

```go
handler.AddEnvironmentVariable("CUSTOM_CONFIG", "production")
```

### Access Construct Properties

```go
eventBusName := handler.GetEventBusName()
eventBusArn := handler.GetEventBusArn()
ruleName := handler.GetRuleName()
ruleArn := handler.GetRuleArn()
```

## Additional Properties

### Using Existing Rules

```go
// Create an existing rule
existingRule := awsevents.NewRule(stack, jsii.String("ExistingRule"), &awsevents.RuleProps{
    RuleName: jsii.String("existing-rule"),
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("existing.app")},
    },
})

// Use the existing rule with EventBridgeHandler
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("ExistingRuleHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("existing-rule-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    ExistingRule: existingRule,
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

### Custom Rule Properties

```go
handler, err := constructs.NewEventBridgeHandler(stack, jsii.String("CustomRuleHandler"), &constructs.EventBridgeHandlerProps{
    FunctionProps: awslambda.FunctionProps{
        FunctionName: jsii.String("custom-rule-processor"),
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
    },
    RuleProps: &awsevents.RuleProps{
        RuleName:    jsii.String("custom-rule-name"),
        Description: jsii.String("Custom rule for processing events"),
        Enabled:     jsii.Bool(true),
    },
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("myapp.orders")},
    },
})
if err != nil {
    panic(fmt.Sprintf("Failed to create EventBridgeHandler: %v", err))
}
```

## Deprecated Methods

⚠️ **Warning:** The following methods are deprecated and should not be used in new code:

```go
// DEPRECATED: Event patterns cannot be modified after rule creation
err := handler.AddEventPattern(&awsevents.EventPattern{
    Source: &[]*string{jsii.String("new.source")},
})
// This will return an error

// DEPRECATED: Rule state cannot be changed after CDK deployment
err = handler.EnableRule()
// This will return an error with instructions to use AWS CLI

err = handler.DisableRule()
// This will return an error with instructions to use AWS CLI
```

Instead of using deprecated methods:
- **For pattern changes:** Create a new EventBridgeHandler with the desired pattern
- **For rule state changes:** Use AWS CLI: `aws events enable-rule --name <rule-name>` or `aws events disable-rule --name <rule-name>`

## Best Practices

### Event Pattern Design

1. **Be Specific**: Use detailed event patterns to reduce unnecessary invocations
2. **Use Consistent Naming**: Follow consistent source and detail-type naming conventions
3. **Include Metadata**: Add tenant IDs, correlation IDs, and other metadata to event details

### Error Handling

1. **Enable DLQ**: Always use dead letter queues for production workloads
2. **Set Appropriate Retries**: Configure retry attempts based on your use case
3. **Monitor DLQ**: Set up alarms for messages in the dead letter queue

### Performance

1. **Right-size Functions**: Use appropriate memory and timeout settings
2. **Enable ARM64**: Use ARM64 architecture for better price/performance
3. **Use Tracing**: Enable X-Ray tracing for debugging and performance analysis

### Security

1. **Least Privilege**: Grant only necessary permissions
2. **Cross-Account**: Use resource-based policies for cross-account access
3. **Encryption**: Enable encryption for sensitive event data

## Common Patterns

### Microservice Communication

```go
// Service A publishes events
publisherHandler := constructs.NewEventBridgeHandler(stack, jsii.String("ServiceAPublisher"), &constructs.EventBridgeHandlerProps{
    // ... configuration
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("service.a")},
    },
})

// Service B consumes events from Service A
consumerHandler := constructs.NewEventBridgeHandler(stack, jsii.String("ServiceBConsumer"), &constructs.EventBridgeHandlerProps{
    // ... configuration
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("service.a")},
        DetailType: &[]*string{jsii.String("User Created")},
    },
})
```

### Data Processing Pipeline

```go
// Ingest events
ingestHandler := constructs.NewEventBridgeHandler(stack, jsii.String("DataIngest"), &constructs.EventBridgeHandlerProps{
    // ... configuration
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("data.source")},
    },
})

// Transform events
transformHandler := constructs.NewEventBridgeHandler(stack, jsii.String("DataTransform"), &constructs.EventBridgeHandlerProps{
    // ... configuration
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("data.ingest")},
        DetailType: &[]*string{jsii.String("Data Ingested")},
    },
})

// Load events
loadHandler := constructs.NewEventBridgeHandler(stack, jsii.String("DataLoad"), &constructs.EventBridgeHandlerProps{
    // ... configuration
    EventPattern: &awsevents.EventPattern{
        Source: &[]*string{jsii.String("data.transform")},
        DetailType: &[]*string{jsii.String("Data Transformed")},
    },
})
```

## Troubleshooting

### Common Issues

1. **Rule Not Triggering**: Check event pattern syntax and ensure events match exactly
2. **Permission Denied**: Verify IAM permissions for EventBridge and Lambda
3. **DLQ Messages**: Check Lambda function logs and DLQ for failed events
4. **Schedule Not Working**: Verify cron/rate expression syntax

### Debugging Tips

1. **Use CloudWatch Logs**: Enable detailed logging in your Lambda functions
2. **Test Event Patterns**: Use EventBridge console to test event patterns
3. **Monitor Metrics**: Watch EventBridge and Lambda CloudWatch metrics
4. **Enable Tracing**: Use X-Ray for distributed tracing across services