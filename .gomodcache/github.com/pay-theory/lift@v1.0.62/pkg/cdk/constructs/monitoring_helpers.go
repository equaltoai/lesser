package constructs

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// AddStandardLambdaAlarms creates common Lambda alarms (errors, throttles, duration).
func AddStandardLambdaAlarms(scope constructs.Construct, namePrefix string, fn awslambda.IFunction) {
	// Function error alarm
	awscloudwatch.NewAlarm(scope, jsii.String("FunctionErrorAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-errors", namePrefix)),
		AlarmDescription: jsii.String("Lambda function errors"),
		Metric: fn.MetricErrors(&awscloudwatch.MetricOptions{
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(5),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	// Function throttles alarm
	awscloudwatch.NewAlarm(scope, jsii.String("FunctionThrottleAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-throttles", namePrefix)),
		AlarmDescription: jsii.String("Lambda function throttled"),
		Metric: fn.MetricThrottles(&awscloudwatch.MetricOptions{
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	// Function duration alarm
	awscloudwatch.NewAlarm(scope, jsii.String("FunctionDurationAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-duration", namePrefix)),
		AlarmDescription: jsii.String("Lambda function duration high"),
		Metric: fn.MetricDuration(&awscloudwatch.MetricOptions{
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_AVERAGE(),
		}),
		Threshold:          jsii.Number(30000), // 30 seconds
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
}

// addLambdaMetricAlarm creates an alarm on a specific Lambda metric for the function.
func addLambdaMetricAlarm(scope constructs.Construct, alarmID, alarmName, alarmDesc, metricName string, threshold, evalPeriods float64, comparison awscloudwatch.ComparisonOperator, stat *string, fn awslambda.IFunction) {
	metric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/Lambda"),
		MetricName: jsii.String(metricName),
		DimensionsMap: &map[string]*string{
			"FunctionName": fn.FunctionName(),
		},
		Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		Statistic: stat,
	})

	awscloudwatch.NewAlarm(scope, jsii.String(alarmID), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(alarmName),
		AlarmDescription:   jsii.String(alarmDesc),
		Metric:             metric,
		Threshold:          jsii.Number(threshold),
		EvaluationPeriods:  jsii.Number(evalPeriods),
		ComparisonOperator: comparison,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
}

// EnableS3LambdaMonitoring adds standard alarms plus concurrency alarm for S3 processors.
func EnableS3LambdaMonitoring(scope constructs.Construct, bucketName *string, fn awslambda.IFunction) {
	AddStandardLambdaAlarms(scope, fmt.Sprintf("%s-processor", *bucketName), fn)
	addLambdaMetricAlarm(
		scope,
		"FunctionConcurrencyAlarm",
		fmt.Sprintf("%s-processor-concurrency", *bucketName),
		"S3 processor high concurrent executions",
		"ConcurrentExecutions",
		900,
		2,
		awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		awscloudwatch.Stats_MAXIMUM(),
		fn,
	)
}

// EnableStreamLambdaMonitoring adds standard alarms plus iterator age for stream processors.
func EnableStreamLambdaMonitoring(scope constructs.Construct, tableName *string, fn awslambda.IFunction) {
	AddStandardLambdaAlarms(scope, fmt.Sprintf("%s-stream-processor", *tableName), fn)
	addLambdaMetricAlarm(
		scope,
		"IteratorAgeAlarm",
		fmt.Sprintf("%s-iterator-age", *tableName),
		"Stream iterator age is too high",
		"IteratorAge",
		60000,
		2,
		awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		awscloudwatch.Stats_MAXIMUM(),
		fn,
	)
}

// applyNonNilStructFields copies non-zero fields from src into dst (same struct type).
func applyNonNilStructFields(dst any, src any) {
	dv := reflect.ValueOf(dst)
	sv := reflect.ValueOf(src)
	if dv.Kind() != reflect.Ptr || sv.Kind() != reflect.Ptr || sv.IsNil() {
		return
	}
	de := dv.Elem()
	se := sv.Elem()
	if de.Kind() != reflect.Struct || se.Kind() != reflect.Struct || de.Type() != se.Type() {
		return
	}
	for i := 0; i < se.NumField(); i++ {
		sf := se.Field(i)
		if sf.IsZero() {
			continue
		}
		df := de.Field(i)
		if df.CanSet() {
			// Only set when assignable to avoid panics
			if sf.Type().AssignableTo(df.Type()) {
				df.Set(sf)
			}
		}
	}
}
