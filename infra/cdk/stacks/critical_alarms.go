package stacks

import (
	"fmt"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type CriticalAlarmsNestedStackProps struct {
	awscdk.NestedStackProps
	AppName     string
	Environment string
}

type CriticalAlarmsNestedStack struct {
	awscdk.NestedStack
	AppName     string
	Environment string
}

func (s *LesserApiStack) createCriticalAlarms() {
	NewCriticalAlarmsNestedStack(s.Stack, "CriticalAlarms", &CriticalAlarmsNestedStackProps{
		AppName:     s.AppName,
		Environment: s.Environment,
	})
}

func NewCriticalAlarmsNestedStack(scope constructs.Construct, id string, props *CriticalAlarmsNestedStackProps) *CriticalAlarmsNestedStack {
	nested := awscdk.NewNestedStack(scope, jsii.String(id), &props.NestedStackProps)
	stack := &CriticalAlarmsNestedStack{
		NestedStack: nested,
		AppName:     props.AppName,
		Environment: props.Environment,
	}

	alertTopic := awssns.NewTopic(nested, jsii.String("CriticalAlertTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(naming.ResourceNameWithApp(stack.AppName, "critical-alerts", stack.Environment)),
		DisplayName: jsii.String(fmt.Sprintf("%s %s critical processor alerts", stack.AppName, naming.StageForEnvironment(stack.Environment))),
	})

	stack.addCriticalLambdaAlarms(alertTopic)
	stack.addCriticalQueueAlarms(alertTopic)
	return stack
}

func (s *CriticalAlarmsNestedStack) addCriticalLambdaAlarms(alertTopic awssns.ITopic) {
	thresholds := lambdaAlarmThresholdsFor(s.Environment)
	for _, spec := range inventory.LambdaInventory.Lambdas {
		functionName := lambdaPhysicalName(s.AppName, s.Environment, spec.Name)
		constructPrefix := fmt.Sprintf("%sCritical", sanitizeConstructID(spec.Name))

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

		awscloudwatch.NewAlarm(s.NestedStack, jsii.String(fmt.Sprintf("%sErrorRateAlarm", constructPrefix)), &awscloudwatch.AlarmProps{
			AlarmName: jsii.String(fmt.Sprintf("%s-critical-error-rate", functionName)),
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
		}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))

		awscloudwatch.NewAlarm(s.NestedStack, jsii.String(fmt.Sprintf("%sErrorsAlarm", constructPrefix)), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-critical-errors", functionName)),
			Metric:             errorsMetric,
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))

		if isStreamLambda(spec) {
			iteratorAgeMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
				Namespace:     jsii.String("AWS/Lambda"),
				MetricName:    jsii.String("IteratorAge"),
				DimensionsMap: &map[string]*string{"FunctionName": jsii.String(functionName)},
				Statistic:     jsii.String("Maximum"),
				Period:        awscdk.Duration_Minutes(jsii.Number(5)),
			})
			awscloudwatch.NewAlarm(s.NestedStack, jsii.String(fmt.Sprintf("%sIteratorAgeAlarm", constructPrefix)), &awscloudwatch.AlarmProps{
				AlarmName:          jsii.String(fmt.Sprintf("%s-critical-iterator-age", functionName)),
				Metric:             iteratorAgeMetric,
				Threshold:          jsii.Number(60000),
				EvaluationPeriods:  jsii.Number(2),
				ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
				TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
			}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
		}

		if len(spec.ScheduleTriggers) > 0 {
			awscloudwatch.NewAlarm(s.NestedStack, jsii.String(fmt.Sprintf("%sScheduledErrorsAlarm", constructPrefix)), &awscloudwatch.AlarmProps{
				AlarmName:          jsii.String(fmt.Sprintf("%s-critical-scheduled-errors", functionName)),
				Metric:             errorsMetric,
				Threshold:          jsii.Number(1),
				EvaluationPeriods:  jsii.Number(1),
				ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
				TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
			}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
		}
	}
}

func (s *CriticalAlarmsNestedStack) addCriticalQueueAlarms(alertTopic awssns.ITopic) {
	for _, q := range deriveInventoryQueues() {
		primaryName := queuePhysicalName(s.AppName, s.Environment, q.Logical)
		addCriticalQueueAgeAlarm(s.NestedStack, alertTopic, s.Environment, primaryName, q.Logical)
		addCriticalQueueDepthAlarm(s.NestedStack, alertTopic, primaryName, fmt.Sprintf("%sPrimary", q.Logical), sqsDepthThresholdFor(s.Environment))

		dlqName := queuePhysicalName(s.AppName, s.Environment, q.DLQLogical)
		addCriticalQueueDepthAlarm(s.NestedStack, alertTopic, dlqName, fmt.Sprintf("%sDLQ", q.Logical), 0)
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		for idx, trig := range spec.StreamTriggers {
			poisonName := queuePhysicalName(s.AppName, s.Environment, trig.PoisonRecordQueue)
			addCriticalQueueDepthAlarm(s.NestedStack, alertTopic, poisonName, fmt.Sprintf("%sStreamPoison%d", spec.Name, idx), 0)
		}
		for idx := range spec.ScheduleTriggers {
			scheduleDLQ := queuePhysicalName(s.AppName, s.Environment, fmt.Sprintf("%s-schedule-%d-dlq", spec.Name, idx))
			addCriticalQueueDepthAlarm(s.NestedStack, alertTopic, scheduleDLQ, fmt.Sprintf("%sScheduleDLQ%d", spec.Name, idx), 0)
		}
	}
}

func addCriticalQueueAgeAlarm(scope constructs.Construct, alertTopic awssns.ITopic, environment string, queueName string, logicalID string) {
	ageOfOldest := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/SQS"),
		MetricName:    jsii.String("ApproximateAgeOfOldestMessage"),
		DimensionsMap: &map[string]*string{"QueueName": jsii.String(queueName)},
		Statistic:     jsii.String("Maximum"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})
	awscloudwatch.NewAlarm(scope, jsii.String(fmt.Sprintf("%sCriticalAgeAlarm", sanitizeConstructID(logicalID))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-critical-age", queueName)),
		Metric:             ageOfOldest,
		Threshold:          jsii.Number(sqsAgeThresholdSecondsFor(environment)),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
}

func addCriticalQueueDepthAlarm(scope constructs.Construct, alertTopic awssns.ITopic, queueName string, logicalID string, threshold float64) {
	visibleMessages := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:     jsii.String("AWS/SQS"),
		MetricName:    jsii.String("ApproximateNumberOfMessagesVisible"),
		DimensionsMap: &map[string]*string{"QueueName": jsii.String(queueName)},
		Statistic:     jsii.String("Average"),
		Period:        awscdk.Duration_Minutes(jsii.Number(5)),
	})
	awscloudwatch.NewAlarm(scope, jsii.String(fmt.Sprintf("%sCriticalDepthAlarm", sanitizeConstructID(logicalID))), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-critical-depth", queueName)),
		Metric:             visibleMessages,
		Threshold:          jsii.Number(threshold),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	}).AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
}

func sqsDepthThresholdFor(environment string) float64 {
	switch naming.StageForEnvironment(environment) {
	case naming.StageLive:
		return 100
	case naming.StageStaging:
		return 250
	default:
		return 500
	}
}
