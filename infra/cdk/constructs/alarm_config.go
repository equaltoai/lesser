package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/common"
)

// AlarmConfiguration defines comprehensive alarm settings for different environments
type AlarmConfiguration struct {
	Environment string
	Lambda      LambdaAlarmConfig
	DynamoDB    DynamoDBAlarmConfig
	APIGateway  APIGatewayAlarmConfig
	SQS         SQSAlarmConfig
	Cost        CostAlarmConfig
	Business    BusinessAlarmConfig
}

// LambdaAlarmConfig defines Lambda-specific alarm thresholds
type LambdaAlarmConfig struct {
	ErrorRateThreshold       float64 // Percentage (0-100)
	DurationThresholdMs      float64 // Milliseconds
	MemoryUtilizationPercent float64 // Percentage (0-100)
	ColdStartThresholdMs     float64 // Milliseconds
	ThrottleThreshold        float64 // Count
	IteratorAgeThresholdMs   float64 // Milliseconds for stream processors
	ConcurrencyThreshold     float64 // Count
	DeadLetterThreshold      float64 // Count
	EvaluationPeriods        int
	DatapointsToAlarm        int
}

// DynamoDBAlarmConfig defines DynamoDB-specific alarm thresholds
type DynamoDBAlarmConfig struct {
	ReadThrottleThreshold        float64 // Count
	WriteThrottleThreshold       float64 // Count
	GSIThrottleThreshold         float64 // Count
	TransactionConflictThreshold float64 // Count
	HotPartitionThreshold        float64 // Percentage of total capacity
	ConsumedCapacityThreshold    float64 // Percentage of provisioned
	UserErrorsThreshold          float64 // Count
	SystemErrorsThreshold        float64 // Count
	SuccessRateThreshold         float64 // Percentage (0-100)
	EvaluationPeriods            int
	DatapointsToAlarm            int
}

// APIGatewayAlarmConfig defines API Gateway-specific alarm thresholds
type APIGatewayAlarmConfig struct {
	ErrorRate4xxThreshold       float64 // Percentage (0-100)
	ErrorRate5xxThreshold       float64 // Percentage (0-100)
	LatencyP95ThresholdMs       float64 // Milliseconds
	LatencyP99ThresholdMs       float64 // Milliseconds
	LatencyAverageThreshold     float64 // Milliseconds
	IntegrationLatencyThreshold float64 // Milliseconds
	WafBlockedThreshold         float64 // Count
	CacheHitRateThreshold       float64 // Percentage (0-100)
	EvaluationPeriods           int
	DatapointsToAlarm           int
}

// SQSAlarmConfig defines SQS-specific alarm thresholds
type SQSAlarmConfig struct {
	MessageAgeThresholdSeconds    float64 // Seconds
	DeadLetterQueueDepthThreshold float64 // Count
	QueueDepthThreshold           float64 // Count
	MessageProcessingFailureRate  float64 // Percentage (0-100)
	VisibilityTimeoutBreachRate   float64 // Percentage (0-100)
	EvaluationPeriods             int
	DatapointsToAlarm             int
}

// CostAlarmConfig defines cost monitoring alarm thresholds
type CostAlarmConfig struct {
	DailyCostThresholdDollars    float64 // USD
	MonthlyCostThresholdDollars  float64 // USD
	CostAnomalyThresholdPercent  float64 // Percentage increase from baseline
	DynamoDBCostThresholdDollars float64 // USD per day
	LambdaCostThresholdDollars   float64 // USD per day
	S3CostThresholdDollars       float64 // USD per day
	EvaluationPeriods            int
}

// BusinessAlarmConfig defines business logic-specific alarm thresholds
type BusinessAlarmConfig struct {
	FederationFailureRate       float64 // Percentage (0-100)
	AuthFailureRate             float64 // Percentage (0-100)
	MediaProcessingFailureRate  float64 // Percentage (0-100)
	UserRegistrationFailureRate float64 // Percentage (0-100)
	PostCreationFailureRate     float64 // Percentage (0-100)
	SearchFailureRate           float64 // Percentage (0-100)
	StreamingConnectionFailures float64 // Count per minute
	EvaluationPeriods           int
}

// AlarmConfigBuilder creates environment-specific alarm configurations
type AlarmConfigBuilder struct {
	appName string
}

// NewAlarmConfigBuilder creates a new alarm configuration builder
func NewAlarmConfigBuilder(appName string) *AlarmConfigBuilder {
	return &AlarmConfigBuilder{
		appName: appName,
	}
}

// GetConfiguration returns alarm configuration for the specified environment
func (b *AlarmConfigBuilder) GetConfiguration(environment string) AlarmConfiguration {
	switch strings.ToLower(environment) {
	case "prod", "production":
		return b.productionConfig()
	case "staging", "stage":
		return b.stagingConfig()
	case "dev", "development":
		return b.developmentConfig()
	default:
		return b.developmentConfig()
	}
}

// productionConfig returns production-ready alarm thresholds
func (b *AlarmConfigBuilder) productionConfig() AlarmConfiguration {
	return AlarmConfiguration{
		Environment: "production",
		Lambda: LambdaAlarmConfig{
			ErrorRateThreshold:       2.0,   // 2% error rate
			DurationThresholdMs:      15000, // 15 seconds (API Gateway timeout is 30s)
			MemoryUtilizationPercent: 85.0,  // 85% memory usage
			ColdStartThresholdMs:     3000,  // 3 seconds cold start
			ThrottleThreshold:        1,     // Any throttles
			IteratorAgeThresholdMs:   30000, // 30 seconds iterator age
			ConcurrencyThreshold:     800,   // 80% of default concurrent executions
			DeadLetterThreshold:      5,     // 5 messages in DLQ
			EvaluationPeriods:        2,
			DatapointsToAlarm:        2,
		},
		DynamoDB: DynamoDBAlarmConfig{
			ReadThrottleThreshold:        1,    // Any read throttles
			WriteThrottleThreshold:       1,    // Any write throttles
			GSIThrottleThreshold:         1,    // Any GSI throttles
			TransactionConflictThreshold: 10,   // 10 transaction conflicts
			HotPartitionThreshold:        80.0, // 80% of capacity on single partition
			ConsumedCapacityThreshold:    70.0, // 70% of provisioned capacity
			UserErrorsThreshold:          10,   // 10 user errors
			SystemErrorsThreshold:        1,    // Any system errors
			SuccessRateThreshold:         95.0, // 95% success rate
			EvaluationPeriods:            2,
			DatapointsToAlarm:            2,
		},
		APIGateway: APIGatewayAlarmConfig{
			ErrorRate4xxThreshold:       10.0, // 10% 4xx error rate
			ErrorRate5xxThreshold:       1.0,  // 1% 5xx error rate
			LatencyP95ThresholdMs:       2000, // 2 second P95 latency
			LatencyP99ThresholdMs:       5000, // 5 second P99 latency
			LatencyAverageThreshold:     1000, // 1 second average latency
			IntegrationLatencyThreshold: 8000, // 8 second integration latency
			WafBlockedThreshold:         100,  // 100 WAF blocks per minute
			CacheHitRateThreshold:       60.0, // 60% cache hit rate
			EvaluationPeriods:           2,
			DatapointsToAlarm:           2,
		},
		SQS: SQSAlarmConfig{
			MessageAgeThresholdSeconds:    300,  // 5 minutes
			DeadLetterQueueDepthThreshold: 10,   // 10 messages in DLQ
			QueueDepthThreshold:           1000, // 1000 messages in queue
			MessageProcessingFailureRate:  5.0,  // 5% processing failure rate
			VisibilityTimeoutBreachRate:   10.0, // 10% visibility timeout breaches
			EvaluationPeriods:             2,
			DatapointsToAlarm:             2,
		},
		Cost: CostAlarmConfig{
			DailyCostThresholdDollars:    10.0,  // $10 per day
			MonthlyCostThresholdDollars:  200.0, // $200 per month
			CostAnomalyThresholdPercent:  50.0,  // 50% increase from baseline
			DynamoDBCostThresholdDollars: 3.0,   // $3 per day for DynamoDB
			LambdaCostThresholdDollars:   5.0,   // $5 per day for Lambda
			S3CostThresholdDollars:       2.0,   // $2 per day for S3
			EvaluationPeriods:            1,
		},
		Business: BusinessAlarmConfig{
			FederationFailureRate:       5.0,  // 5% federation failure rate
			AuthFailureRate:             10.0, // 10% auth failure rate (includes bad logins)
			MediaProcessingFailureRate:  2.0,  // 2% media processing failure rate
			UserRegistrationFailureRate: 5.0,  // 5% user registration failure rate
			PostCreationFailureRate:     1.0,  // 1% post creation failure rate
			SearchFailureRate:           5.0,  // 5% search failure rate
			StreamingConnectionFailures: 10,   // 10 streaming connection failures per minute
			EvaluationPeriods:           3,
		},
	}
}

// stagingConfig returns staging environment alarm thresholds (more lenient than production)
func (b *AlarmConfigBuilder) stagingConfig() AlarmConfiguration {
	prod := b.productionConfig()

	// Make staging thresholds more lenient
	prod.Environment = "staging"
	prod.Lambda.ErrorRateThreshold = 5.0         // 5% error rate
	prod.Lambda.DurationThresholdMs = 20000      // 20 seconds
	prod.DynamoDB.SuccessRateThreshold = 90.0    // 90% success rate
	prod.APIGateway.ErrorRate4xxThreshold = 15.0 // 15% 4xx error rate
	prod.APIGateway.ErrorRate5xxThreshold = 3.0  // 3% 5xx error rate
	prod.Cost.DailyCostThresholdDollars = 20.0   // $20 per day
	prod.Business.FederationFailureRate = 10.0   // 10% federation failure rate

	return prod
}

// developmentConfig returns development environment alarm thresholds (most lenient)
func (b *AlarmConfigBuilder) developmentConfig() AlarmConfiguration {
	staging := b.stagingConfig()

	// Make development thresholds very lenient
	staging.Environment = "development"
	staging.Lambda.ErrorRateThreshold = 10.0        // 10% error rate
	staging.Lambda.DurationThresholdMs = 25000      // 25 seconds
	staging.DynamoDB.SuccessRateThreshold = 85.0    // 85% success rate
	staging.APIGateway.ErrorRate4xxThreshold = 20.0 // 20% 4xx error rate
	staging.APIGateway.ErrorRate5xxThreshold = 5.0  // 5% 5xx error rate
	staging.Cost.DailyCostThresholdDollars = 50.0   // $50 per day
	staging.Business.FederationFailureRate = 20.0   // 20% federation failure rate

	return staging
}

// AlarmManager provides centralized alarm creation and management
type AlarmManager struct {
	scope      constructs.Construct
	config     AlarmConfiguration
	alertTopic awssns.Topic
	appName    string
}

// NewAlarmManager creates a new alarm manager
func NewAlarmManager(scope constructs.Construct, config AlarmConfiguration, alertTopic awssns.Topic, appName string) *AlarmManager {
	return &AlarmManager{
		scope:      scope,
		config:     config,
		alertTopic: alertTopic,
		appName:    appName,
	}
}

// CreateLambdaAlarms creates comprehensive Lambda function alarms
func (am *AlarmManager) CreateLambdaAlarms(functionName string, functionArn string) []awscloudwatch.Alarm {
	var alarms []awscloudwatch.Alarm

	// Base metrics
	invocationsMetric := am.createLambdaMetric(functionName, "Invocations", "Sum")
	errorsMetric := am.createLambdaMetric(functionName, "Errors", "Sum")
	durationMetric := am.createLambdaMetric(functionName, "Duration", "Average")
	throttlesMetric := am.createLambdaMetric(functionName, "Throttles", "Sum")
	concurrentExecutionsMetric := am.createLambdaMetric(functionName, "ConcurrentExecutions", "Maximum")
	deadLetterErrorsMetric := am.createLambdaMetric(functionName, "DeadLetterErrors", "Sum")

	// Error rate alarm using math expression
	errorRateAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-error-rate-alarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-%s-%s-error-rate", am.appName, am.config.Environment, functionName)),
		AlarmDescription: jsii.String(fmt.Sprintf("Error rate for %s exceeds %v%%", functionName, am.config.Lambda.ErrorRateThreshold)),
		Metric: awscloudwatch.NewMathExpression(&awscloudwatch.MathExpressionProps{
			Expression: jsii.String("(errors / invocations) * 100"),
			UsingMetrics: &map[string]awscloudwatch.IMetric{
				"errors":      errorsMetric,
				"invocations": invocationsMetric,
			},
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(am.config.Lambda.ErrorRateThreshold),
		EvaluationPeriods:  jsii.Number(am.config.Lambda.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.Lambda.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	errorRateAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, errorRateAlarm)

	// Duration alarm
	durationAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-duration-alarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-duration", am.appName, am.config.Environment, functionName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Average duration for %s exceeds %vms", functionName, am.config.Lambda.DurationThresholdMs)),
		Metric:             durationMetric,
		Threshold:          jsii.Number(am.config.Lambda.DurationThresholdMs),
		EvaluationPeriods:  jsii.Number(am.config.Lambda.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.Lambda.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	durationAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, durationAlarm)

	// Throttle alarm
	throttleAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-throttle-alarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-throttles", am.appName, am.config.Environment, functionName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Throttles detected for %s", functionName)),
		Metric:             throttlesMetric,
		Threshold:          jsii.Number(am.config.Lambda.ThrottleThreshold),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	throttleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, throttleAlarm)

	// Concurrency alarm (only for high-traffic functions)
	if am.isHighTrafficFunction(functionName) {
		concurrencyAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-concurrency-alarm", functionName)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-concurrency", am.appName, am.config.Environment, functionName)),
			AlarmDescription:   jsii.String(fmt.Sprintf("High concurrency detected for %s", functionName)),
			Metric:             concurrentExecutionsMetric,
			Threshold:          jsii.Number(am.config.Lambda.ConcurrencyThreshold),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		concurrencyAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		alarms = append(alarms, concurrencyAlarm)
	}

	// Dead letter errors alarm
	deadLetterAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-deadletter-alarm", functionName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-deadletter", am.appName, am.config.Environment, functionName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Dead letter errors detected for %s", functionName)),
		Metric:             deadLetterErrorsMetric,
		Threshold:          jsii.Number(am.config.Lambda.DeadLetterThreshold),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	deadLetterAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, deadLetterAlarm)

	// Iterator age alarm (for stream processing functions)
	if am.isStreamProcessor(functionName) {
		iteratorAgeMetric := am.createLambdaMetric(functionName, "IteratorAge", "Maximum")
		iteratorAgeAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-iterator-age-alarm", functionName)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-iterator-age", am.appName, am.config.Environment, functionName)),
			AlarmDescription:   jsii.String(fmt.Sprintf("Iterator age for %s exceeds %vms", functionName, am.config.Lambda.IteratorAgeThresholdMs)),
			Metric:             iteratorAgeMetric,
			Threshold:          jsii.Number(am.config.Lambda.IteratorAgeThresholdMs),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		iteratorAgeAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		alarms = append(alarms, iteratorAgeAlarm)
	}

	return alarms
}

// CreateDynamoDBAlarms creates comprehensive DynamoDB alarms
func (am *AlarmManager) CreateDynamoDBAlarms(tableName string, gsiNames []string) []awscloudwatch.Alarm {
	var alarms []awscloudwatch.Alarm

	// Base table metrics
	readThrottlesMetric := am.createDynamoDBMetric(tableName, "", "UserReadThrottledRequests", "Sum")
	writeThrottlesMetric := am.createDynamoDBMetric(tableName, "", "UserWriteThrottledRequests", "Sum")
	systemErrorsMetric := am.createDynamoDBMetric(tableName, "", "SystemErrors", "Sum")
	userErrorsMetric := am.createDynamoDBMetric(tableName, "", "UserErrors", "Sum")

	// Read throttle alarm
	readThrottleAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-read-throttle-alarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-read-throttles", am.appName, am.config.Environment, tableName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Read throttles detected on table %s", tableName)),
		Metric:             readThrottlesMetric,
		Threshold:          jsii.Number(am.config.DynamoDB.ReadThrottleThreshold),
		EvaluationPeriods:  jsii.Number(am.config.DynamoDB.EvaluationPeriods),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	readThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, readThrottleAlarm)

	// Write throttle alarm
	writeThrottleAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-write-throttle-alarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-write-throttles", am.appName, am.config.Environment, tableName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Write throttles detected on table %s", tableName)),
		Metric:             writeThrottlesMetric,
		Threshold:          jsii.Number(am.config.DynamoDB.WriteThrottleThreshold),
		EvaluationPeriods:  jsii.Number(am.config.DynamoDB.EvaluationPeriods),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	writeThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, writeThrottleAlarm)

	// System errors alarm
	systemErrorsAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-system-errors-alarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-system-errors", am.appName, am.config.Environment, tableName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("System errors detected on table %s", tableName)),
		Metric:             systemErrorsMetric,
		Threshold:          jsii.Number(am.config.DynamoDB.SystemErrorsThreshold),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	systemErrorsAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, systemErrorsAlarm)

	// User errors alarm (more lenient than system errors)
	userErrorsAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-user-errors-alarm", tableName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-user-errors", am.appName, am.config.Environment, tableName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("High user error rate on table %s", tableName)),
		Metric:             userErrorsMetric,
		Threshold:          jsii.Number(am.config.DynamoDB.UserErrorsThreshold),
		EvaluationPeriods:  jsii.Number(am.config.DynamoDB.EvaluationPeriods),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	userErrorsAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, userErrorsAlarm)

	// GSI throttle alarms
	for _, gsiName := range gsiNames {
		gsiReadThrottlesMetric := am.createDynamoDBMetric(tableName, gsiName, "UserReadThrottledRequests", "Sum")
		gsiWriteThrottlesMetric := am.createDynamoDBMetric(tableName, gsiName, "UserWriteThrottledRequests", "Sum")

		// GSI read throttle alarm
		gsiReadThrottleAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-%s-read-throttle-alarm", tableName, gsiName)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-%s-read-throttles", am.appName, am.config.Environment, tableName, gsiName)),
			AlarmDescription:   jsii.String(fmt.Sprintf("Read throttles detected on GSI %s of table %s", gsiName, tableName)),
			Metric:             gsiReadThrottlesMetric,
			Threshold:          jsii.Number(am.config.DynamoDB.GSIThrottleThreshold),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		gsiReadThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		alarms = append(alarms, gsiReadThrottleAlarm)

		// GSI write throttle alarm
		gsiWriteThrottleAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-%s-write-throttle-alarm", tableName, gsiName)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-%s-write-throttles", am.appName, am.config.Environment, tableName, gsiName)),
			AlarmDescription:   jsii.String(fmt.Sprintf("Write throttles detected on GSI %s of table %s", gsiName, tableName)),
			Metric:             gsiWriteThrottlesMetric,
			Threshold:          jsii.Number(am.config.DynamoDB.GSIThrottleThreshold),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		gsiWriteThrottleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		alarms = append(alarms, gsiWriteThrottleAlarm)
	}

	return alarms
}

// CreateAPIGatewayAlarms creates comprehensive API Gateway alarms
func (am *AlarmManager) CreateAPIGatewayAlarms(apiName string, apiId string) []awscloudwatch.Alarm {
	var alarms []awscloudwatch.Alarm

	// Base metrics
	count4xxMetric := am.createAPIGatewayMetric(apiName, "4XXError", "Sum")
	count5xxMetric := am.createAPIGatewayMetric(apiName, "5XXError", "Sum")
	countMetric := am.createAPIGatewayMetric(apiName, "Count", "Sum")
	latencyMetric := am.createAPIGatewayMetric(apiName, "Latency", "Average")
	integrationLatencyMetric := am.createAPIGatewayMetric(apiName, "IntegrationLatency", "Average")

	// 4XX error rate alarm
	error4xxRateAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-4xx-rate-alarm", apiName)), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-%s-%s-4xx-rate", am.appName, am.config.Environment, apiName)),
		AlarmDescription: jsii.String(fmt.Sprintf("4XX error rate for %s exceeds %v%%", apiName, am.config.APIGateway.ErrorRate4xxThreshold)),
		Metric: awscloudwatch.NewMathExpression(&awscloudwatch.MathExpressionProps{
			Expression: jsii.String("(errors4xx / requests) * 100"),
			UsingMetrics: &map[string]awscloudwatch.IMetric{
				"errors4xx": count4xxMetric,
				"requests":  countMetric,
			},
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(am.config.APIGateway.ErrorRate4xxThreshold),
		EvaluationPeriods:  jsii.Number(am.config.APIGateway.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.APIGateway.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	error4xxRateAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, error4xxRateAlarm)

	// 5XX error rate alarm
	error5xxRateAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-5xx-rate-alarm", apiName)), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-%s-%s-5xx-rate", am.appName, am.config.Environment, apiName)),
		AlarmDescription: jsii.String(fmt.Sprintf("5XX error rate for %s exceeds %v%%", apiName, am.config.APIGateway.ErrorRate5xxThreshold)),
		Metric: awscloudwatch.NewMathExpression(&awscloudwatch.MathExpressionProps{
			Expression: jsii.String("(errors5xx / requests) * 100"),
			UsingMetrics: &map[string]awscloudwatch.IMetric{
				"errors5xx": count5xxMetric,
				"requests":  countMetric,
			},
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(am.config.APIGateway.ErrorRate5xxThreshold),
		EvaluationPeriods:  jsii.Number(am.config.APIGateway.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.APIGateway.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	error5xxRateAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, error5xxRateAlarm)

	// Latency alarm
	latencyAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-latency-alarm", apiName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-latency", am.appName, am.config.Environment, apiName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Average latency for %s exceeds %vms", apiName, am.config.APIGateway.LatencyAverageThreshold)),
		Metric:             latencyMetric,
		Threshold:          jsii.Number(am.config.APIGateway.LatencyAverageThreshold),
		EvaluationPeriods:  jsii.Number(am.config.APIGateway.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.APIGateway.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	latencyAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, latencyAlarm)

	// Integration latency alarm
	integrationLatencyAlarm := awscloudwatch.NewAlarm(am.scope, jsii.String(fmt.Sprintf("%s-integration-latency-alarm", apiName)), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-%s-%s-integration-latency", am.appName, am.config.Environment, apiName)),
		AlarmDescription:   jsii.String(fmt.Sprintf("Integration latency for %s exceeds %vms", apiName, am.config.APIGateway.IntegrationLatencyThreshold)),
		Metric:             integrationLatencyMetric,
		Threshold:          jsii.Number(am.config.APIGateway.IntegrationLatencyThreshold),
		EvaluationPeriods:  jsii.Number(am.config.APIGateway.EvaluationPeriods),
		DatapointsToAlarm:  jsii.Number(am.config.APIGateway.DatapointsToAlarm),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	integrationLatencyAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
	alarms = append(alarms, integrationLatencyAlarm)

	return alarms
}

// CreateCompositeAlarms creates service-level health composite alarms
func (am *AlarmManager) CreateCompositeAlarms(serviceAlarms map[string][]awscloudwatch.Alarm) []awscloudwatch.CompositeAlarm {
	var compositeAlarms []awscloudwatch.CompositeAlarm

	// API Service Health (combines Lambda + API Gateway + DynamoDB)
	if apiAlarms, exists := serviceAlarms["api-service"]; exists && len(apiAlarms) > 0 {
		apiServiceAlarm := awscloudwatch.NewCompositeAlarm(am.scope, jsii.String("api-service-health"), &awscloudwatch.CompositeAlarmProps{
			CompositeAlarmName: jsii.String(fmt.Sprintf("%s-%s-api-service-health", am.appName, am.config.Environment)),
			AlarmDescription:   jsii.String("Overall health of API service (Lambda + API Gateway + DynamoDB)"),
			AlarmRule:          am.buildCompositeAlarmRule(apiAlarms),
		})
		apiServiceAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		compositeAlarms = append(compositeAlarms, apiServiceAlarm)
	}

	// Federation Service Health
	if federationAlarms, exists := serviceAlarms["federation-service"]; exists && len(federationAlarms) > 0 {
		federationServiceAlarm := awscloudwatch.NewCompositeAlarm(am.scope, jsii.String("federation-service-health"), &awscloudwatch.CompositeAlarmProps{
			CompositeAlarmName: jsii.String(fmt.Sprintf("%s-%s-federation-service-health", am.appName, am.config.Environment)),
			AlarmDescription:   jsii.String("Overall health of federation service (inbox/outbox + delivery)"),
			AlarmRule:          am.buildCompositeAlarmRule(federationAlarms),
		})
		federationServiceAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		compositeAlarms = append(compositeAlarms, federationServiceAlarm)
	}

	// Auth Service Health
	if authAlarms, exists := serviceAlarms["auth-service"]; exists && len(authAlarms) > 0 {
		authServiceAlarm := awscloudwatch.NewCompositeAlarm(am.scope, jsii.String("auth-service-health"), &awscloudwatch.CompositeAlarmProps{
			CompositeAlarmName: jsii.String(fmt.Sprintf("%s-%s-auth-service-health", am.appName, am.config.Environment)),
			AlarmDescription:   jsii.String("Overall health of authentication service"),
			AlarmRule:          am.buildCompositeAlarmRule(authAlarms),
		})
		authServiceAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(am.alertTopic))
		compositeAlarms = append(compositeAlarms, authServiceAlarm)
	}

	return compositeAlarms
}

// Helper methods

func (am *AlarmManager) createLambdaMetric(functionName, metricName, statistic string) awscloudwatch.Metric {
	return awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/Lambda"),
		MetricName:    jsii.String(metricName),
		DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
		Statistic:     jsii.String(statistic),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})
}

func (am *AlarmManager) createDynamoDBMetric(tableName, gsiName, metricName, statistic string) awscloudwatch.Metric {
	dimensions := map[string]*string{"TableName": jsii.String(tableName)}
	if gsiName != "" {
		dimensions["GlobalSecondaryIndexName"] = jsii.String(gsiName)
	}

	return awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/DynamoDB"),
		MetricName:    jsii.String(metricName),
		DimensionsMap: &dimensions,
		Statistic:     jsii.String(statistic),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})
}

func (am *AlarmManager) createAPIGatewayMetric(apiName, metricName, statistic string) awscloudwatch.Metric {
	return awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/ApiGateway"),
		MetricName:    jsii.String(metricName),
		DimensionsMap: &map[string]*string{"ApiName": jsii.String(apiName)},
		Statistic:     jsii.String(statistic),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})
}

func (am *AlarmManager) isHighTrafficFunction(functionName string) bool {
	highTrafficFunctions := []string{"api", "graphql", "streaming", "inbox", "outbox"}
	for _, fn := range highTrafficFunctions {
		if fn == functionName {
			return true
		}
	}
	return false
}

func (am *AlarmManager) isStreamProcessor(functionName string) bool {
	streamProcessors := []string{"activity-processor", "notification-processor", "stream-router"}
	for _, fn := range streamProcessors {
		if fn == functionName {
			return true
		}
	}
	return false
}

func (am *AlarmManager) buildCompositeAlarmRule(alarms []awscloudwatch.Alarm) awscloudwatch.IAlarmRule {
	if err := common.ValidateSliceNotEmpty("alarms", alarms); err != nil {
		return nil
	}

	// Create OR rule: if any alarm is in ALARM state, composite alarm triggers
	var alarmRules []awscloudwatch.IAlarmRule
	for _, alarm := range alarms {
		alarmRules = append(alarmRules, alarm)
	}

	return awscloudwatch.AlarmRule_AnyOf(alarmRules...)
}
