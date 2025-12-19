package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
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

	// Create SNS topic for alerts
	monitoringStack.AlertTopic = awssns.NewTopic(stack, jsii.String("AlertTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-%s-alerts", props.AppName, props.Environment)),
		DisplayName: jsii.String(fmt.Sprintf("%s %s Alerts", props.AppName, props.Environment)),
	})

	// Add email subscription if provided
	if props.AlertEmail != "" {
		monitoringStack.AlertTopic.AddSubscription(
			awssnssubscriptions.NewEmailSubscription(jsii.String(props.AlertEmail), nil),
		)
	}

	// Create CloudWatch dashboard
	monitoringStack.Dashboard = awscloudwatch.NewDashboard(stack, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-%s", props.AppName, props.Environment)),
		Start:         jsii.String("-P1D"), // Last 24 hours
	})

	// Add dashboard widgets (placeholder - will be populated by other stacks)
	monitoringStack.Dashboard.AddWidgets(
		awscloudwatch.NewTextWidget(&awscloudwatch.TextWidgetProps{
			Markdown: jsii.String(fmt.Sprintf("# %s %s Dashboard\n\nMonitoring metrics for the Lesser serverless application.", props.AppName, props.Environment)),
			Width:    jsii.Number(24),
			Height:   jsii.Number(2),
		}),
	)

	// Create outputs
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

// Helper method to add comprehensive Lambda metrics to the dashboard
func (s *MonitoringStack) AddLambdaMetrics(functionName string, functionArn string) {
	// Core Lambda metrics
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

	// For stream processors, add iterator age metric
	var iteratorAgeMetric awscloudwatch.Metric
	if functionName == "activity-processor" || functionName == "notification-processor" || functionName == "stream-router" {
		iteratorAgeMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:     jsii.String("AWS/Lambda"),
			MetricName:    jsii.String("IteratorAge"),
			DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
			Statistic:     jsii.String("Maximum"),
			Period:        awscdk.Duration_Minutes(jsii.Number(5)),
		})
	}

	// Add widgets to dashboard
	s.Dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Invocations & Errors", functionName)),
			Left:   &[]awscloudwatch.IMetric{invocationsMetric},
			Right:  &[]awscloudwatch.IMetric{errorsMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String(fmt.Sprintf("%s - Duration & Throttles", functionName)),
			Left:   &[]awscloudwatch.IMetric{durationMetric},
			Right:  &[]awscloudwatch.IMetric{throttlesMetric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}),
	)

	// Add iterator age widget for stream processors
	if iteratorAgeMetric != nil {
		s.Dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title:  jsii.String(fmt.Sprintf("%s - Iterator Age & Concurrency", functionName)),
				Left:   &[]awscloudwatch.IMetric{iteratorAgeMetric},
				Right:  &[]awscloudwatch.IMetric{concurrentExecutionsMetric},
				Width:  jsii.Number(24),
				Height: jsii.Number(6),
			}),
		)

		// Create alarm for iterator age (critical for stream processing)
		awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sIteratorAgeAlarm", functionName)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-iterator-age", functionName)),
			Metric:             iteratorAgeMetric,
			Threshold:          jsii.Number(60000), // 1 minute in milliseconds
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
	}

	// Create production-ready alarms
	// Error rate alarm (more than 5% error rate)
	errorRateAlarm := awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sErrorRateAlarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName: jsii.String(fmt.Sprintf("%s-error-rate", functionName)),
		Metric: awscloudwatch.NewMathExpression(&awscloudwatch.MathExpressionProps{
			Expression: jsii.String("(errors / invocations) * 100"),
			UsingMetrics: &map[string]awscloudwatch.IMetric{
				"errors":      errorsMetric,
				"invocations": invocationsMetric,
			},
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(5), // 5% error rate
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	errorRateAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	// Duration alarm (average duration > 10 seconds)
	durationAlarm := awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sDurationAlarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-duration", functionName)),
		Metric:             durationMetric,
		Threshold:          jsii.Number(10000), // 10 seconds in milliseconds
		EvaluationPeriods:  jsii.Number(3),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	durationAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	// Throttle alarm
	throttleAlarm := awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sThrottleAlarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-throttles", functionName)),
		Metric:             throttlesMetric,
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	throttleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
}

// Helper method to add API Gateway metrics
func (s *MonitoringStack) AddAPIGatewayMetrics(apiName string, apiId string) {
	count4xx := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/ApiGateway"),
		MetricName: jsii.String("4XXError"),
		DimensionsMap: &map[string]*string{
			"ApiName": jsii.String(apiName),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(5)),
	})

	count5xx := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/ApiGateway"),
		MetricName: jsii.String("5XXError"),
		DimensionsMap: &map[string]*string{
			"ApiName": jsii.String(apiName),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(5)),
	})

	// Add widgets to dashboard
	s.Dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  jsii.String("API Gateway Errors"),
			Left:   &[]awscloudwatch.IMetric{count4xx},
			Right:  &[]awscloudwatch.IMetric{count5xx},
			Width:  jsii.Number(24),
			Height: jsii.Number(6),
		}),
	)

	// Create alarm for 5XX errors
	awscloudwatch.NewAlarm(s.Stack, jsii.String("APIGateway5XXAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:         jsii.String(fmt.Sprintf("%s-5xx-errors", apiName)),
		Metric:            count5xx,
		Threshold:         jsii.Number(10),
		EvaluationPeriods: jsii.Number(2),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))
}

// CreateEventBridgeRules creates the EventBridge rules for cost and trend aggregation
func (s *MonitoringStack) CreateEventBridgeRules(costAggregatorFunction awslambda.Function, trendAggregatorFunction awslambda.Function, environment string) {
	// Cost aggregation rule - triggers every hour
	costAggregationRule := awsevents.NewRule(s.Stack, jsii.String("CostAggregationRule"), &awsevents.RuleProps{
		RuleName:    jsii.String(fmt.Sprintf("lesser-%s-cost-aggregation", environment)),
		Description: jsii.String("Trigger cost aggregation every hour"),
		Schedule:    awsevents.Schedule_Rate(awscdk.Duration_Hours(jsii.Number(1))),
		Enabled:     jsii.Bool(true),
	})

	// Add target for cost aggregation
	costAggregationRule.AddTarget(awseventstargets.NewLambdaFunction(costAggregatorFunction, &awseventstargets.LambdaFunctionProps{
		RetryAttempts: jsii.Number(2),
		MaxEventAge:   awscdk.Duration_Hours(jsii.Number(1)),
	}))

	// Trend aggregation rule - triggers every 15 minutes
	trendAggregationRule := awsevents.NewRule(s.Stack, jsii.String("TrendAggregationRule"), &awsevents.RuleProps{
		RuleName:    jsii.String(fmt.Sprintf("lesser-%s-trend-aggregation", environment)),
		Description: jsii.String("Trigger trend aggregation every 15 minutes"),
		Schedule:    awsevents.Schedule_Rate(awscdk.Duration_Minutes(jsii.Number(15))),
		Enabled:     jsii.Bool(true),
	})

	// Add target for trend aggregation
	trendAggregationRule.AddTarget(awseventstargets.NewLambdaFunction(trendAggregatorFunction, &awseventstargets.LambdaFunctionProps{
		RetryAttempts: jsii.Number(2),
		MaxEventAge:   awscdk.Duration_Minutes(jsii.Number(30)),
	}))
}

// CreateLogGroups creates CloudWatch log groups for API Gateway and Lambda functions
func (s *MonitoringStack) CreateLogGroups(appName string, environment string) map[string]awslogs.LogGroup {
	logGroups := make(map[string]awslogs.LogGroup)

	// API Gateway log group
	logGroups["api-gateway"] = awslogs.NewLogGroup(s.Stack, jsii.String("APIGatewayLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/apigateway/%s-%s", appName, environment)),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// Lambda function log groups with configurable retention
	lambdaFunctions := []string{
		"api", "graphql", "inbox", "outbox", "webfinger",
		"streaming", "stream-router", "activity-processor", "notification-processor",
		"moderation-processor", "cost-aggregator", "trend-aggregator", "media-processor",
		"export-generator", "import-processor", "federation-delivery", "push-notification",
		"report-trust-updater", "federation-tracker",
	}

	retention := awslogs.RetentionDays_ONE_WEEK
	if environment == "prod" {
		retention = awslogs.RetentionDays_ONE_MONTH
	}

	for _, functionName := range lambdaFunctions {
		logGroups[functionName] = awslogs.NewLogGroup(s.Stack, jsii.String(fmt.Sprintf("%sLogGroup", functionName)), &awslogs.LogGroupProps{
			LogGroupName:  jsii.String(fmt.Sprintf("/aws/lambda/%s-%s-%s", appName, environment, functionName)),
			Retention:     retention,
			RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		})
	}

	return logGroups
}

// AddDynamoDBMetrics adds comprehensive DynamoDB monitoring
func (s *MonitoringStack) AddDynamoDBMetrics(table awsdynamodb.Table, tableName string) {
	// Table-level metrics
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

	// Add dashboard widgets
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

	// Create alarms for throttling
	readThrottleAlarm := awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sReadThrottleAlarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-read-throttles", tableName)),
		Metric:             throttledReadsMetric,
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	readThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	writeThrottleAlarm := awscloudwatch.NewAlarm(s.Stack, jsii.String(fmt.Sprintf("%sWriteThrottleAlarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-write-throttles", tableName)),
		Metric:             throttledWritesMetric,
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	writeThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(s.AlertTopic))

	// GSI metrics for the Lesser table (has 9 GSIs)
	gsiNames := []string{
		"gsi1", "gsi2", "gsi3", "gsi4", "gsi5", "gsi6", "gsi7", "gsi8", "gsi9",
	}

	for _, gsiName := range gsiNames {
		gsiReadCapacity := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("ConsumedReadCapacityUnits"),
			DimensionsMap: &map[string]*string{
				"TableName":                jsii.String(tableName),
				"GlobalSecondaryIndexName": jsii.String(gsiName),
			},
			Statistic: jsii.String("Sum"),
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		})

		_ = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("ConsumedWriteCapacityUnits"),
			DimensionsMap: &map[string]*string{
				"TableName":                jsii.String(tableName),
				"GlobalSecondaryIndexName": jsii.String(gsiName),
			},
			Statistic: jsii.String("Sum"),
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		})

		// Add GSI widget every 2 GSIs to keep dashboard organized
		if gsiName == "gsi1" || gsiName == "gsi3" || gsiName == "gsi5" || gsiName == "gsi7" {
			nextGSI := fmt.Sprintf("gsi%d",
				func() int {
					if gsiName == "gsi1" {
						return 2
					}
					if gsiName == "gsi3" {
						return 4
					}
					if gsiName == "gsi5" {
						return 6
					}
					return 8
				}())

			gsiReadCapacity2 := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
				Namespace:  jsii.String("AWS/DynamoDB"),
				MetricName: jsii.String("ConsumedReadCapacityUnits"),
				DimensionsMap: &map[string]*string{
					"TableName":                jsii.String(tableName),
					"GlobalSecondaryIndexName": jsii.String(nextGSI),
				},
				Statistic: jsii.String("Sum"),
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			})

			s.Dashboard.AddWidgets(
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title:  jsii.String(fmt.Sprintf("%s - %s & %s Read Capacity", tableName, gsiName, nextGSI)),
					Left:   &[]awscloudwatch.IMetric{gsiReadCapacity},
					Right:  &[]awscloudwatch.IMetric{gsiReadCapacity2},
					Width:  jsii.Number(24),
					Height: jsii.Number(6),
				}),
			)
		}

		// If there's an unpaired GSI (e.g., gsi9), add a single-widget chart
		if gsiName == "gsi9" {
			s.Dashboard.AddWidgets(
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title:  jsii.String(fmt.Sprintf("%s - %s Read Capacity", tableName, gsiName)),
					Left:   &[]awscloudwatch.IMetric{gsiReadCapacity},
					Width:  jsii.Number(24),
					Height: jsii.Number(6),
				}),
			)
		}
	}
}
