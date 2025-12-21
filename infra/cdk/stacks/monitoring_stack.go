package stacks

import (
	"fmt"
	"sort"
	"strings"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssnssubscriptions"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type MonitoringStackProps struct {
	awscdk.StackProps
	AppName     string
	Environment string
	AlertEmail  string
}

// MonitoringStack is observability-only: do not add EventBridge rules, schedules,
// queue/DLQ provisioning, or Lambda event source mappings here.
type MonitoringStack struct {
	awscdk.Stack
	AlertTopic awssns.Topic
	Dashboard  awscloudwatch.Dashboard
}

func NewMonitoringStack(scope constructs.Construct, id string, props *MonitoringStackProps) *MonitoringStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	monitoringStack := &MonitoringStack{
		Stack: stack,
	}

	monitoringStack.AlertTopic = awssns.NewTopic(stack, jsii.String("AlertTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-%s-alerts", props.AppName, props.Environment)),
		DisplayName: jsii.String(fmt.Sprintf("%s %s Alerts", props.AppName, props.Environment)),
	})

	if props.AlertEmail != "" {
		monitoringStack.AlertTopic.AddSubscription(
			awssnssubscriptions.NewEmailSubscription(jsii.String(props.AlertEmail), nil),
		)
	}

	monitoringStack.Dashboard = awscloudwatch.NewDashboard(stack, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-%s", props.AppName, props.Environment)),
		Start:         jsii.String("-P1D"),
	})

	monitoringStack.Dashboard.AddWidgets(
		awscloudwatch.NewTextWidget(&awscloudwatch.TextWidgetProps{
			Markdown: jsii.String(fmt.Sprintf("# %s %s Dashboard\n\nInventory-driven monitoring for the Lesser serverless application.", props.AppName, props.Environment)),
			Width:    jsii.Number(24),
			Height:   jsii.Number(2),
		}),
	)

	monitoringStack.populateInventoryDrivenMonitoring(props.Environment)

	awscdk.NewCfnOutput(stack, jsii.String("AlertTopicArn"), &awscdk.CfnOutputProps{
		Value:       monitoringStack.AlertTopic.TopicArn(),
		Description: jsii.String("SNS topic ARN for alerts"),
		ExportName:  jsii.String(fmt.Sprintf("%s-%s-alert-topic-arn", props.AppName, props.Environment)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("DashboardURL"), &awscdk.CfnOutputProps{
		Value:       jsii.String(fmt.Sprintf("https://console.aws.amazon.com/cloudwatch/home?region=%s#dashboards:name=%s-%s", *stack.Region(), props.AppName, props.Environment)),
		Description: jsii.String("CloudWatch dashboard URL"),
	})

	return monitoringStack
}

func (s *MonitoringStack) populateInventoryDrivenMonitoring(environment string) {
	s.addSection("Lambdas")
	for _, spec := range inventory.LambdaInventory.Lambdas {
		s.addLambdaMetrics(environment, spec)
	}

	s.addSection("Queues")
	for _, q := range deriveInventoryQueues() {
		s.addQueueMetrics(environment, q)
	}

	s.addSection("DynamoDB")
	s.addDynamoDBMetrics(fmt.Sprintf("lesser-%s", environment))
	s.addDynamoDBMetrics(fmt.Sprintf("lesser-rate-limits-%s", environment))
}

func (s *MonitoringStack) addSection(title string) {
	s.Dashboard.AddWidgets(
		awscloudwatch.NewTextWidget(&awscloudwatch.TextWidgetProps{
			Markdown: jsii.String(fmt.Sprintf("## %s", title)),
			Width:    jsii.Number(24),
			Height:   jsii.Number(1),
		}),
	)
}

func (s *MonitoringStack) addLambdaMetrics(environment string, spec inventory.LambdaSpec) {
	functionName := lambdaPhysicalName(environment, spec.Name)

	invocationsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String("Invocations"),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	errorsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String("Errors"),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	durationMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String("Duration"),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String("Average"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	throttlesMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String("Throttles"),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	concurrentExecutionsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String("ConcurrentExecutions"),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String("Maximum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	var iteratorAgeMetric awscloudwatch.Metric
	if isStreamLambda(spec) {
		iteratorAgeMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:     jsii.String("AWS/Lambda"),
			MetricName:    jsii.String("IteratorAge"),
			DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
			Statistic:     jsii.String("Maximum"),
			Period:        awscdk.Duration_Minutes(jsii.Number(5)),
		})
	}

	s.Dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Invocations & Errors", spec.Name)),
			Left:   &[]awscloudwatch.IMetric{invocationsMetric},
			Right:  &[]awscloudwatch.IMetric{errorsMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Duration & Throttles", spec.Name)),
			Left:   &[]awscloudwatch.IMetric{durationMetric},
			Right:  &[]awscloudwatch.IMetric{throttlesMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
	)

	if iteratorAgeMetric != nil {
		s.Dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title:  jsii.String(fmt.Sprintf("%s - Iterator Age & Concurrency", spec.Name)),
				Left:   &[]awscloudwatch.IMetric{iteratorAgeMetric},
				Right:  &[]awscloudwatch.IMetric{concurrentExecutionsMetric},
				Width:  jsii.Number(24),
				Height: jsii.Number(6),
			}),
		)

		awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sIteratorAgeAlarm", sanitizeConstructID(spec.Name))), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-iterator-age", functionName)),
			Metric:             iteratorAgeMetric,
			Threshold:          jsii.Number(60000),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
	}

	thresholds := lambdaAlarmThresholdsFor(environment)

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sErrorRateAlarm", sanitizeConstructID(spec.Name))), &awscloudwatch.AlarmProps{
		AlarmName: jsii.String(fmt.Sprintf("%s-error-rate", functionName)),
		Metric: awscloudwatch.NewMathExpression(&awscloudwatch.MathExpressionProps{
			Expression: jsii.String("(errors / invocations) * 100"),
			UsingMetrics: &map[string]awscloudwatch.IMetric{
				"errors":      errorsMetric,
				"invocations": invocationsMetric,
			},
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(thresholds.ErrorRatePercent),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sDurationAlarm", sanitizeConstructID(spec.Name))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-duration", functionName)),
		Metric:             durationMetric,
		Threshold:          jsii.Number(thresholds.DurationMs),
		EvaluationPeriods:  jsii.Number(3),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sThrottleAlarm", sanitizeConstructID(spec.Name))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-throttles", functionName)),
		Metric:             throttlesMetric,
		Threshold:          jsii.Number(thresholds.ThrottleCount),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
}

type queueSpec struct {
	Logical    string
	DLQLogical string
}

func deriveInventoryQueues() []queueSpec {
	seen := map[string]queueSpec{}
	for _, lambdaSpec := range inventory.LambdaInventory.Lambdas {
		for _, trig := range lambdaSpec.SQSTriggers {
			if _, ok := seen[trig.Queue]; ok {
				continue
			}
			dlqLogical := trig.DeadLetterQueue
			if dlqLogical == "" {
				dlqLogical = fmt.Sprintf("%s-dlq", trig.Queue)
			}
			seen[trig.Queue] = queueSpec{Logical: trig.Queue, DLQLogical: dlqLogical}
		}
	}

	if _, ok := seen["scheduled-queue"]; !ok {
		seen["scheduled-queue"] = queueSpec{Logical: "scheduled-queue", DLQLogical: "scheduled-queue-dlq"}
	}

	out := make([]queueSpec, 0, len(seen))
	for _, spec := range seen {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Logical < out[j].Logical
	})
	return out
}

func (s *MonitoringStack) addQueueMetrics(environment string, spec queueSpec) {
	primaryName := queuePhysicalName(environment, spec.Logical)
	dlqName := queuePhysicalName(environment, spec.DLQLogical)

	visibleMessages := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/SQS"),
		MetricName:    jsii.String("ApproximateNumberOfMessagesVisible"),
		DimensionsMap: &map[string]*string{"QueueName": jsii.String(primaryName)},
		Statistic:     jsii.String("Average"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	ageOfOldest := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/SQS"),
		MetricName:    jsii.String("ApproximateAgeOfOldestMessage"),
		DimensionsMap: &map[string]*string{"QueueName": jsii.String(primaryName)},
		Statistic:     jsii.String("Maximum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	dlqDepth := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/SQS"),
		MetricName:    jsii.String("ApproximateNumberOfMessagesVisible"),
		DimensionsMap: &map[string]*string{"QueueName": jsii.String(dlqName)},
		Statistic:     jsii.String("Average"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	s.Dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Depth & Oldest Age", spec.Logical)),
			Left:   &[]awscloudwatch.IMetric{visibleMessages},
			Right:  &[]awscloudwatch.IMetric{ageOfOldest},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - DLQ Depth", spec.Logical)),
			Left:   &[]awscloudwatch.IMetric{dlqDepth},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
	)

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sAgeAlarm", sanitizeConstructID(spec.Logical))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-age", primaryName)),
		Metric:             ageOfOldest,
		Threshold:          jsii.Number(sqsAgeThresholdSecondsFor(environment)),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sDlqDepthAlarm", sanitizeConstructID(spec.Logical))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-depth", dlqName)),
		Metric:             dlqDepth,
		Threshold:          jsii.Number(0),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
}

func (s *MonitoringStack) addDynamoDBMetrics(tableName string) {
	readCapacityMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/DynamoDB"),
		MetricName:    jsii.String("ConsumedReadCapacityUnits"),
		DimensionsMap: &map[string]*string{"TableName": jsii.String(tableName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	writeCapacityMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/DynamoDB"),
		MetricName:    jsii.String("ConsumedWriteCapacityUnits"),
		DimensionsMap: &map[string]*string{"TableName": jsii.String(tableName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	throttledReadsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/DynamoDB"),
		MetricName:    jsii.String("UserReadThrottledRequests"),
		DimensionsMap: &map[string]*string{"TableName": jsii.String(tableName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	throttledWritesMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/DynamoDB"),
		MetricName:    jsii.String("UserWriteThrottledRequests"),
		DimensionsMap: &map[string]*string{"TableName": jsii.String(tableName)},
		Statistic:     jsii.String("Sum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})

	s.Dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Read/Write Capacity", tableName)),
			Left:   &[]awscloudwatch.IMetric{readCapacityMetric},
			Right:  &[]awscloudwatch.IMetric{writeCapacityMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Throttled Requests", tableName)),
			Left:   &[]awscloudwatch.IMetric{throttledReadsMetric},
			Right:  &[]awscloudwatch.IMetric{throttledWritesMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
	)

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sReadThrottleAlarm", sanitizeConstructID(tableName))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-read-throttles", tableName)),
		Metric:             throttledReadsMetric,
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sWriteThrottleAlarm", sanitizeConstructID(tableName))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-write-throttles", tableName)),
		Metric:             throttledWritesMetric,
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
}

type lambdaAlarmThresholds struct {
	ErrorRatePercent float64
	DurationMs       float64
	ThrottleCount    float64
}

func lambdaAlarmThresholdsFor(environment string) lambdaAlarmThresholds {
	switch strings.ToLower(environment) {
	case "production":
		return lambdaAlarmThresholds{ErrorRatePercent: 2.0, DurationMs: 15000, ThrottleCount: 1}
	case "staging":
		return lambdaAlarmThresholds{ErrorRatePercent: 5.0, DurationMs: 20000, ThrottleCount: 1}
	default:
		return lambdaAlarmThresholds{ErrorRatePercent: 10.0, DurationMs: 25000, ThrottleCount: 1}
	}
}

func sqsAgeThresholdSecondsFor(environment string) float64 {
	switch strings.ToLower(environment) {
	case "production":
		return 300
	case "staging":
		return 900
	default:
		return 1800
	}
}

func isStreamLambda(spec inventory.LambdaSpec) bool {
	if spec.Type == inventory.LambdaTypeProcessorStream {
		return true
	}
	if spec.Type == inventory.LambdaTypeHybrid && len(spec.StreamTriggers) > 0 {
		return true
	}
	return len(spec.StreamTriggers) > 0
}

func sanitizeConstructID(name string) string {
	clean := strings.ReplaceAll(name, "-", "")
	clean = strings.ReplaceAll(clean, "_", "")
	if clean == "" {
		return "Resource"
	}
	if clean[0] >= '0' && clean[0] <= '9' {
		return "R" + clean
	}
	return clean
}

func lambdaPhysicalName(environment string, lambdaName string) string {
	return fmt.Sprintf("lesser-%s-%s", environment, lambdaName)
}

func queuePhysicalName(environment string, logical string) string {
	return fmt.Sprintf("lesser-%s-%s", logical, environment)
}
