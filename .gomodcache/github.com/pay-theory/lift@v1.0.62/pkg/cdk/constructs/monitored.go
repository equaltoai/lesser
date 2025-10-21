package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// AlarmConfig defines configuration for CloudWatch alarms
//
// This struct contains all configurable properties for CloudWatch alarms
// including error rate, latency, throttling, and concurrent execution alarms.
// It also includes configuration for SNS topic notifications.
type AlarmConfig struct {
	// Enable error rate alarm
	EnableErrorAlarm *bool
	// Error rate threshold (percentage)
	ErrorRateThreshold *float64
	// Enable latency alarm
	EnableLatencyAlarm *bool
	// Latency threshold in milliseconds
	LatencyThreshold *float64
	// Enable throttle alarm
	EnableThrottleAlarm *bool
	// Throttle count threshold
	ThrottleThreshold *float64
	// Enable concurrent executions alarm
	EnableConcurrentAlarm *bool
	// Concurrent executions threshold
	ConcurrentThreshold *float64
	// SNS topic for alarm notifications
	AlarmTopic awssns.ITopic
}

// MonitoredFunctionProps extends LiftFunctionProps with monitoring configuration
//
// This struct contains all configurable properties for creating a monitored
// Lambda function. It extends LiftFunctionProps with additional monitoring
// configuration like CloudWatch dashboard, alarms, Lambda Insights,
// and Log Insights queries.
type MonitoredFunctionProps struct {
	LiftFunctionProps
	// Enable CloudWatch dashboard
	EnableDashboard *bool
	// Dashboard name (optional - will generate if not provided)
	DashboardName *string
	// Alarm configuration
	AlarmConfig *AlarmConfig
	// Custom metrics namespace
	MetricsNamespace *string
	// Enable enhanced monitoring (Lambda Insights)
	EnableLambdaInsights *bool
	// Log level (ERROR, WARN, INFO, DEBUG)
	LogLevel *string
	// Enable CloudWatch Logs Insights queries
	EnableLogInsightsQueries *bool
}

// MonitoredFunction is a Lambda function with comprehensive monitoring
//
// This construct creates a Lambda function with comprehensive monitoring features
// including CloudWatch dashboard, alarms, Lambda Insights, and Log Insights queries.
// It provides methods to add custom metrics and log queries.
type MonitoredFunction struct {
	constructs.Construct
	Function  *LiftFunction
	Dashboard awscloudwatch.Dashboard
	Alarms    map[string]awscloudwatch.Alarm
}

// NewMonitoredFunction creates a Lambda function with comprehensive monitoring
//
// This function creates a Lambda function with all monitoring features configured:
//
// - Creates a CloudWatch dashboard with default widgets
// - Configures CloudWatch alarms for errors, latency, throttling, and concurrency
// - Enables Lambda Insights if requested
// - Sets up environment variables for monitoring
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new MonitoredFunction instance
func NewMonitoredFunction(scope constructs.Construct, id *string, props *MonitoredFunctionProps) *MonitoredFunction {
	builder := newMonitoredFunctionBuilder(scope, id, props)
	return builder.build()
}

// monitoredFunctionBuilder builds monitored Lambda functions with comprehensive observability
type monitoredFunctionBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *MonitoredFunctionProps
	construct constructs.Construct
	function  *LiftFunction
	alarms    map[string]awscloudwatch.Alarm
}

// newMonitoredFunctionBuilder creates a new monitored function builder
func newMonitoredFunctionBuilder(scope constructs.Construct, id *string, props *MonitoredFunctionProps) *monitoredFunctionBuilder {
	return &monitoredFunctionBuilder{
		scope:  scope,
		id:     id,
		props:  props,
		alarms: make(map[string]awscloudwatch.Alarm),
	}
}

// build constructs the complete monitored function
func (b *monitoredFunctionBuilder) build() *MonitoredFunction {
	b.construct = constructs.NewConstruct(b.scope, b.id)

	b.setDefaults()
	b.configureLambdaInsights()
	b.configureEnvironment()
	b.createFunction()

	dashboard := b.createDashboard()
	b.createAlarms()

	monitored := &MonitoredFunction{
		Construct: b.construct,
		Function:  b.function,
		Dashboard: dashboard,
		Alarms:    b.alarms,
	}

	b.setupLogInsights(monitored)
	return monitored
}

// setDefaults applies default configuration values
func (b *monitoredFunctionBuilder) setDefaults() {
	if b.props.EnableDashboard == nil {
		b.props.EnableDashboard = jsii.Bool(true)
	}
	if b.props.EnableLambdaInsights == nil {
		b.props.EnableLambdaInsights = jsii.Bool(true)
	}
	if b.props.LogLevel == nil {
		b.props.LogLevel = jsii.String("INFO")
	}
	if b.props.MetricsNamespace == nil {
		b.props.MetricsNamespace = jsii.String("Lift/Functions")
	}

	b.setAlarmDefaults()
}

// setAlarmDefaults applies default alarm configuration
func (b *monitoredFunctionBuilder) setAlarmDefaults() {
	if b.props.AlarmConfig == nil {
		b.props.AlarmConfig = &AlarmConfig{}
	}
	if b.props.AlarmConfig.EnableErrorAlarm == nil {
		b.props.AlarmConfig.EnableErrorAlarm = jsii.Bool(true)
	}
	if b.props.AlarmConfig.ErrorRateThreshold == nil {
		b.props.AlarmConfig.ErrorRateThreshold = jsii.Number(1) // 1% error rate
	}
	if b.props.AlarmConfig.EnableLatencyAlarm == nil {
		b.props.AlarmConfig.EnableLatencyAlarm = jsii.Bool(true)
	}
	if b.props.AlarmConfig.LatencyThreshold == nil {
		b.props.AlarmConfig.LatencyThreshold = jsii.Number(3000) // 3 seconds
	}
	if b.props.AlarmConfig.EnableThrottleAlarm == nil {
		b.props.AlarmConfig.EnableThrottleAlarm = jsii.Bool(true)
	}
	if b.props.AlarmConfig.ThrottleThreshold == nil {
		b.props.AlarmConfig.ThrottleThreshold = jsii.Number(5) // 5 throttles
	}
}

// configureLambdaInsights enables Lambda Insights if requested
func (b *monitoredFunctionBuilder) configureLambdaInsights() {
	if *b.props.EnableLambdaInsights {
		b.props.InsightsVersion = awslambda.LambdaInsightsVersion_VERSION_1_0_229_0()
	}
}

// configureEnvironment sets up monitoring environment variables
func (b *monitoredFunctionBuilder) configureEnvironment() {
	if b.props.Environment == nil {
		b.props.Environment = &map[string]*string{}
	}
	env := *b.props.Environment
	env["LOG_LEVEL"] = b.props.LogLevel
	env["METRICS_NAMESPACE"] = b.props.MetricsNamespace
	env["MONITORING_ENABLED"] = jsii.String("true")
	b.props.Environment = &env
}

// createFunction creates the base Lift function
func (b *monitoredFunctionBuilder) createFunction() {
	b.function = NewLiftFunction(b.construct, jsii.String("Function"), &b.props.LiftFunctionProps)
}

// createDashboard creates CloudWatch dashboard if enabled
func (b *monitoredFunctionBuilder) createDashboard() awscloudwatch.Dashboard {
	if !*b.props.EnableDashboard {
		return nil
	}

	dashboardName := b.props.DashboardName
	if dashboardName == nil {
		dashboardName = jsii.String(fmt.Sprintf("%s-dashboard", *b.id))
	}

	dashboard := awscloudwatch.NewDashboard(b.construct, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
		DashboardName: dashboardName,
	})

	// Add widgets to dashboard
	dashboard.AddWidgets(
		createInvocationsWidget(b.function.Function),
		createErrorsWidget(b.function.Function),
		createLatencyWidget(b.function.Function),
		createConcurrentExecutionsWidget(b.function.Function),
	)

	return dashboard
}

// createAlarms creates CloudWatch alarms for the function
func (b *monitoredFunctionBuilder) createAlarms() {
	b.createErrorAlarm()
	b.createLatencyAlarm()
	b.createThrottleAlarm()
	b.createConcurrentAlarm()
}

// createErrorAlarm creates error rate alarm
func (b *monitoredFunctionBuilder) createErrorAlarm() {
	if !*b.props.AlarmConfig.EnableErrorAlarm {
		return
	}

	metric := b.function.Function.MetricErrors(&awscloudwatch.MetricOptions{
		Period: awscdk.Duration_Minutes(jsii.Number(5)),
	})

	alarm := b.createAlarm(metric, "ErrorAlarm", "errors", "Lambda function error rate too high",
		b.props.AlarmConfig.ErrorRateThreshold, 2)
	b.alarms["errors"] = alarm
}

// createLatencyAlarm creates latency alarm
func (b *monitoredFunctionBuilder) createLatencyAlarm() {
	if !*b.props.AlarmConfig.EnableLatencyAlarm {
		return
	}

	metric := b.function.Function.MetricDuration(&awscloudwatch.MetricOptions{
		Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		Statistic: jsii.String("Average"),
	})

	alarm := b.createAlarm(metric, "LatencyAlarm", "latency", "Lambda function latency too high",
		b.props.AlarmConfig.LatencyThreshold, 2)
	b.alarms["latency"] = alarm
}

// createThrottleAlarm creates throttle alarm
func (b *monitoredFunctionBuilder) createThrottleAlarm() {
	if !*b.props.AlarmConfig.EnableThrottleAlarm {
		return
	}

	metric := b.function.Function.MetricThrottles(&awscloudwatch.MetricOptions{
		Period: awscdk.Duration_Minutes(jsii.Number(5)),
	})

	alarm := b.createAlarm(metric, "ThrottleAlarm", "throttles", "Lambda function throttling detected",
		b.props.AlarmConfig.ThrottleThreshold, 1)
	b.alarms["throttles"] = alarm
}

// createConcurrentAlarm creates concurrent executions alarm
func (b *monitoredFunctionBuilder) createConcurrentAlarm() {
	if b.props.AlarmConfig.EnableConcurrentAlarm == nil || !*b.props.AlarmConfig.EnableConcurrentAlarm {
		return
	}

	concurrentMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/Lambda"),
		MetricName: jsii.String("ConcurrentExecutions"),
		DimensionsMap: &map[string]*string{
			"FunctionName": b.function.Function.FunctionName(),
		},
		Period: awscdk.Duration_Minutes(jsii.Number(5)),
	})

	alarm := concurrentMetric.CreateAlarm(b.construct, jsii.String("ConcurrentAlarm"), &awscloudwatch.CreateAlarmOptions{
		AlarmName:         jsii.String(fmt.Sprintf("%s-concurrent", *b.function.Function.FunctionName())),
		AlarmDescription:  jsii.String("Lambda function concurrent executions too high"),
		Threshold:         b.props.AlarmConfig.ConcurrentThreshold,
		EvaluationPeriods: jsii.Number(2),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	if b.props.AlarmConfig.AlarmTopic != nil {
		alarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(b.props.AlarmConfig.AlarmTopic))
	}

	b.alarms["concurrent"] = alarm
}

// createAlarm helper method to create and configure alarms
func (b *monitoredFunctionBuilder) createAlarm(metric awscloudwatch.Metric, alarmId, nameSuffix, description string, threshold *float64, evaluationPeriods float64) awscloudwatch.Alarm {
	alarm := metric.CreateAlarm(b.construct, jsii.String(alarmId), &awscloudwatch.CreateAlarmOptions{
		AlarmName:         jsii.String(fmt.Sprintf("%s-%s", *b.function.Function.FunctionName(), nameSuffix)),
		AlarmDescription:  jsii.String(description),
		Threshold:         threshold,
		EvaluationPeriods: jsii.Number(evaluationPeriods),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	if b.props.AlarmConfig.AlarmTopic != nil {
		alarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(b.props.AlarmConfig.AlarmTopic))
	}

	return alarm
}

// setupLogInsights configures log insights queries if enabled
func (b *monitoredFunctionBuilder) setupLogInsights(monitored *MonitoredFunction) {
	if *b.props.EnableDashboard && b.props.EnableLogInsightsQueries != nil && *b.props.EnableLogInsightsQueries {
		monitored.AddCommonLogInsightsQueries()
	}
}

// GetFunction returns the underlying Lambda function

// This method returns the underlying Lambda function that was created with
// the monitoring enhancements. This is useful when you need to access the
// standard Lambda function properties and methods.
func (f *MonitoredFunction) GetFunction() awslambda.Function {
	return f.Function.Function
}

// GetDashboard returns the CloudWatch dashboard

// This method returns the CloudWatch dashboard that was created for monitoring
// the Lambda function. This is useful when you need to add additional widgets
// or customize the dashboard.
func (f *MonitoredFunction) GetDashboard() awscloudwatch.Dashboard {
	return f.Dashboard
}

// GetAlarm returns a specific alarm by name
//
// This method returns a specific CloudWatch alarm by name. The available alarms
// include "errors", "latency", "throttles", and "concurrent".
//
// Parameters:
//   - name: The name of the alarm to retrieve
//
// Returns:
//   - The CloudWatch alarm
func (f *MonitoredFunction) GetAlarm(name string) awscloudwatch.Alarm {
	return f.Alarms[name]
}

// AddCustomMetric adds a custom metric to the dashboard
//
// This method adds a custom CloudWatch metric to the dashboard. It creates a
// graph widget with the specified metric.
//
// Parameters:
//   - metricName: The name of the metric
//   - namespace: The CloudWatch namespace
//   - dimensions: The metric dimensions
//
// Returns:
//   - The created CloudWatch metric
func (f *MonitoredFunction) AddCustomMetric(metricName *string, namespace *string, dimensions *map[string]*string) awscloudwatch.Metric {
	metric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		MetricName:    metricName,
		Namespace:     namespace,
		DimensionsMap: dimensions,
	})

	if f.Dashboard != nil {
		f.Dashboard.AddWidgets(awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title:  metricName,
			Left:   &[]awscloudwatch.IMetric{metric},
			Width:  jsii.Number(12),
			Height: jsii.Number(6),
		}))
	}

	return metric
}

// AddLogInsightsQuery adds a CloudWatch Logs Insights query to the dashboard
//
// This method adds a CloudWatch Logs Insights query widget to the dashboard.
// It allows you to create custom log queries for analyzing Lambda function logs.
//
// Parameters:
//   - queryName: The name of the query
//   - queryString: The Logs Insights query string
func (f *MonitoredFunction) AddLogInsightsQuery(queryName *string, queryString *string) {
	if f.Dashboard == nil {
		return
	}

	// Lambda automatically creates log group with name /aws/lambda/{function-name}
	logGroupName := jsii.String(fmt.Sprintf("/aws/lambda/%s", *f.Function.Function.FunctionName()))

	// Create a Logs Insights widget
	logsWidget := awscloudwatch.NewLogQueryWidget(&awscloudwatch.LogQueryWidgetProps{
		Title:         queryName,
		LogGroupNames: &[]*string{logGroupName},
		QueryString:   queryString,
		Width:         jsii.Number(24),
		Height:        jsii.Number(6),
	})

	f.Dashboard.AddWidgets(logsWidget)
}

// AddCommonLogInsightsQueries adds common CloudWatch Logs Insights queries

// This method adds a set of common CloudWatch Logs Insights queries to the dashboard.
// The queries include:
//
// - Recent errors
// - Performance metrics
// - Cold start analysis
// - Memory usage
// - Request patterns
// - Slow requests
// - Error rate by status code
// - Tenant activity (for multi-tenant apps)
func (f *MonitoredFunction) AddCommonLogInsightsQueries() {
	if f.Dashboard == nil {
		return
	}

	// Error analysis query
	errorQuery := `fields @timestamp, @message
| filter @message like /ERROR/
| sort @timestamp desc
| limit 100`
	f.AddLogInsightsQuery(jsii.String("Recent Errors"), jsii.String(errorQuery))

	// Performance analysis query
	performanceQuery := `filter @type = "REPORT"
| stats avg(@duration), max(@duration), min(@duration),
        pct(@duration, 50) as p50,
        pct(@duration, 95) as p95,
        pct(@duration, 99) as p99
by bin(5m)`
	f.AddLogInsightsQuery(jsii.String("Performance Metrics"), jsii.String(performanceQuery))

	// Cold start analysis query
	coldStartQuery := `filter @type = "REPORT"
| filter @message like /Init Duration/
| parse @message /Init Duration: (?<initDuration>[\d.]+) ms/
| stats count() as coldStarts, avg(initDuration) as avgInitDuration, max(initDuration) as maxInitDuration
by bin(5m)`
	f.AddLogInsightsQuery(jsii.String("Cold Start Analysis"), jsii.String(coldStartQuery))

	// Memory usage query
	memoryQuery := `filter @type = "REPORT"
| parse @message /Memory Size: (?<memSize>\d+) MB\s+Max Memory Used: (?<memUsed>\d+) MB/
| stats avg(memUsed), max(memUsed), avg(memUsed/memSize*100) as avgMemoryUtilization
by bin(5m)`
	f.AddLogInsightsQuery(jsii.String("Memory Usage"), jsii.String(memoryQuery))

	// Request patterns query
	requestPatternsQuery := `fields @timestamp, @message
| parse @message /\[(?<logLevel>\w+)\].*path=(?<path>[^\s]+).*method=(?<method>\w+)/
| filter ispresent(path)
| stats count() by path, method
| sort count() desc
| limit 20`
	f.AddLogInsightsQuery(jsii.String("Top Request Patterns"), jsii.String(requestPatternsQuery))

	// Slow requests query
	slowRequestsQuery := `filter @type = "REPORT"
| filter @duration > 3000
| fields @timestamp, @requestId, @duration
| sort @duration desc
| limit 50`
	f.AddLogInsightsQuery(jsii.String("Slow Requests"), jsii.String(slowRequestsQuery))

	// Error rate by status code
	errorRateQuery := `fields @timestamp, @message
| parse @message /status=(?<statusCode>\d+)/
| filter ispresent(statusCode)
| stats count() by statusCode
| sort statusCode asc`
	f.AddLogInsightsQuery(jsii.String("Response Status Codes"), jsii.String(errorRateQuery))

	// Tenant activity (for multi-tenant apps)
	tenantActivityQuery := `fields @timestamp, @message
| parse @message /tenant=(?<tenantId>[^\s]+)/
| filter ispresent(tenantId)
| stats count() as requests by tenantId
| sort requests desc
| limit 20`
	f.AddLogInsightsQuery(jsii.String("Tenant Activity"), jsii.String(tenantActivityQuery))
}

// Helper functions for creating dashboard widgets
func createInvocationsWidget(fn awslambda.Function) awscloudwatch.GraphWidget {
	return awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
		Title: jsii.String("Invocations"),
		Left: &[]awscloudwatch.IMetric{
			fn.MetricInvocations(nil),
		},
		Width:  jsii.Number(12),
		Height: jsii.Number(6),
	})
}

func createErrorsWidget(fn awslambda.Function) awscloudwatch.GraphWidget {
	return awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
		Title: jsii.String("Errors"),
		Left: &[]awscloudwatch.IMetric{
			fn.MetricErrors(nil),
		},
		Width:  jsii.Number(12),
		Height: jsii.Number(6),
	})
}

func createLatencyWidget(fn awslambda.Function) awscloudwatch.GraphWidget {
	return awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
		Title: jsii.String("Duration"),
		Left: &[]awscloudwatch.IMetric{
			fn.MetricDuration(&awscloudwatch.MetricOptions{Statistic: jsii.String("Average")}),
			fn.MetricDuration(&awscloudwatch.MetricOptions{Statistic: jsii.String("p99")}),
		},
		Width:  jsii.Number(12),
		Height: jsii.Number(6),
	})
}

func createConcurrentExecutionsWidget(fn awslambda.Function) awscloudwatch.GraphWidget {
	// Create custom metric for concurrent executions
	concurrentMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/Lambda"),
		MetricName: jsii.String("ConcurrentExecutions"),
		DimensionsMap: &map[string]*string{
			"FunctionName": fn.FunctionName(),
		},
	})

	return awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
		Title: jsii.String("Concurrent Executions"),
		Left: &[]awscloudwatch.IMetric{
			concurrentMetric,
		},
		Width:  jsii.Number(12),
		Height: jsii.Number(6),
	})
}
