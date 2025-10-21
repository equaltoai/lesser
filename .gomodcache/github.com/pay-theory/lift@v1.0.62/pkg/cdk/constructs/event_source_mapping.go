package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftEventSourceMappingProps defines properties for event source mapping
type LiftEventSourceMappingProps struct {
	// Target Lambda function
	TargetFunction awslambda.IFunction

	// Event source ARN (for primary region with known ARN)
	EventSourceArn *string

	// Table name (for secondary region where ARN is discovered at runtime)
	TableName *string

	// Batch size for processing
	BatchSize *float64

	// Maximum retry attempts
	RetryAttempts *float64

	// Parallelization factor
	ParallelizationFactor *float64

	// Maximum batching window
	MaxBatchingWindow awscdk.Duration

	// Maximum record age
	MaxRecordAge awscdk.Duration

	// Bisect batch on error
	BisectBatchOnError *bool

	// Report batch item failures
	ReportBatchItemFailures *bool

	// Use custom resource for dynamic ARN lookup (for secondary regions)
	UseCustomResource *bool

	// Starting position for stream reading
	StartingPosition awslambda.StartingPosition
}

// LiftEventSourceMapping wraps event source mapping with automatic handling for cross-region scenarios
type LiftEventSourceMapping struct {
	// The underlying construct
	Construct constructs.Construct

	// Event source mapping (if created directly)
	EventSourceMapping awslambda.EventSourceMapping

	// Custom resource (if using dynamic ARN lookup)
	CustomResource awscdk.CustomResource

	// Custom resource handler function (if using dynamic ARN lookup)
	CustomResourceHandler awslambda.Function
}

// NewLiftEventSourceMapping creates an event source mapping with optional custom resource for dynamic ARN lookup
func NewLiftEventSourceMapping(scope constructs.Construct, id *string, props *LiftEventSourceMappingProps) *LiftEventSourceMapping {
	this := constructs.NewConstruct(scope, id)

	esm := &LiftEventSourceMapping{
		Construct: this,
	}

	// Set defaults
	if props.StartingPosition == "" {
		props.StartingPosition = awslambda.StartingPosition_LATEST
	}
	if props.BatchSize == nil {
		props.BatchSize = jsii.Number(10)
	}
	if props.RetryAttempts == nil {
		props.RetryAttempts = jsii.Number(3)
	}
	if props.ParallelizationFactor == nil {
		props.ParallelizationFactor = jsii.Number(1)
	}

	// Determine mapping strategy
	useCustomResource := props.UseCustomResource != nil && *props.UseCustomResource

	if useCustomResource {
		// Use custom resource for dynamic ARN lookup (secondary region)
		esm.createCustomResourceMapping(props)
	} else if props.EventSourceArn != nil {
		// Direct event source mapping (primary region)
		esm.createDirectMapping(props)
	} else {
		panic("Either EventSourceArn or UseCustomResource=true with TableName must be provided")
	}

	return esm
}

// createDirectMapping creates a direct event source mapping
func (esm *LiftEventSourceMapping) createDirectMapping(props *LiftEventSourceMappingProps) {
	mappingProps := &awslambda.EventSourceMappingProps{
		Target:                props.TargetFunction,
		EventSourceArn:        props.EventSourceArn,
		StartingPosition:      props.StartingPosition,
		BatchSize:             props.BatchSize,
		RetryAttempts:         props.RetryAttempts,
		ParallelizationFactor: props.ParallelizationFactor,
	}

	// Add optional properties
	if props.MaxBatchingWindow != nil {
		mappingProps.MaxBatchingWindow = props.MaxBatchingWindow
	}
	if props.BisectBatchOnError != nil {
		mappingProps.BisectBatchOnError = props.BisectBatchOnError
	}
	if props.ReportBatchItemFailures != nil {
		mappingProps.ReportBatchItemFailures = props.ReportBatchItemFailures
	}
	if props.MaxRecordAge != nil {
		mappingProps.MaxRecordAge = props.MaxRecordAge
	}

	esm.EventSourceMapping = awslambda.NewEventSourceMapping(esm.Construct, jsii.String("EventSourceMapping"), mappingProps)
}

// createCustomResourceMapping creates a custom resource that dynamically looks up stream ARN
func (esm *LiftEventSourceMapping) createCustomResourceMapping(props *LiftEventSourceMappingProps) {
	if props.TableName == nil {
		panic("TableName must be provided when using custom resource mapping")
	}

	// Create IAM role for the custom resource Lambda
	customResourceRole := awsiam.NewRole(esm.Construct, jsii.String("CustomResourceRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		},
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"StreamMappingPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Effect: awsiam.Effect_ALLOW,
						Actions: jsii.Strings(
							"dynamodb:DescribeTable",
							"lambda:CreateEventSourceMapping",
							"lambda:DeleteEventSourceMapping",
							"lambda:ListEventSourceMappings",
							"lambda:UpdateEventSourceMapping",
						),
						Resources: jsii.Strings("*"),
					}),
				},
			}),
		},
	})

	// Create custom resource handler Lambda
	esm.CustomResourceHandler = awslambda.NewFunction(esm.Construct, jsii.String("Handler"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PYTHON_3_11(),
		Handler: jsii.String("index.handler"),
		Role:    customResourceRole,
		Timeout: awscdk.Duration_Minutes(jsii.Number(5)),
		Code:    awslambda.Code_FromInline(esm.getCustomResourceCode(props)),
	})

	// Create custom resource
	esm.CustomResource = awscdk.NewCustomResource(esm.Construct, jsii.String("CustomResource"), &awscdk.CustomResourceProps{
		ServiceToken: esm.CustomResourceHandler.FunctionArn(),
		Properties: &map[string]any{
			"TableName":               *props.TableName,
			"FunctionName":            props.TargetFunction.FunctionName(),
			"StartingPosition":        string(props.StartingPosition),
			"BatchSize":               *props.BatchSize,
			"RetryAttempts":           *props.RetryAttempts,
			"ParallelizationFactor":   *props.ParallelizationFactor,
			"BisectBatchOnError":      getBoolValue(props.BisectBatchOnError, false),
			"ReportBatchItemFailures": getBoolValue(props.ReportBatchItemFailures, false),
		},
	})
}

// getCustomResourceCode returns the Python code for the custom resource handler
func (esm *LiftEventSourceMapping) getCustomResourceCode(props *LiftEventSourceMappingProps) *string {
	return jsii.String(`
import json
import boto3
import cfnresponse

dynamodb = boto3.client('dynamodb')
lambda_client = boto3.client('lambda')

def to_bool(val):
    """Convert CloudFormation string/bool to Python bool"""
    if isinstance(val, bool):
        return val
    if isinstance(val, str):
        return val.lower() in ('true', '1', 'yes')
    return bool(val)

def handler(event, context):
    print(json.dumps(event))
    request_type = event['RequestType']

    try:
        if request_type == 'Create' or request_type == 'Update':
            table_name = event['ResourceProperties']['TableName']
            function_name = event['ResourceProperties']['FunctionName']
            starting_position = event['ResourceProperties'].get('StartingPosition', 'LATEST')
            batch_size = int(event['ResourceProperties'].get('BatchSize', 10))
            retry_attempts = int(event['ResourceProperties'].get('RetryAttempts', 3))
            parallelization_factor = int(event['ResourceProperties'].get('ParallelizationFactor', 1))
            bisect_batch_on_error = to_bool(event['ResourceProperties'].get('BisectBatchOnError', False))
            report_batch_item_failures = to_bool(event['ResourceProperties'].get('ReportBatchItemFailures', False))

            # Get stream ARN from table
            response = dynamodb.describe_table(TableName=table_name)
            stream_arn = response['Table']['LatestStreamArn']

            if not stream_arn:
                raise Exception(f"Table {table_name} does not have streams enabled")

            # Check if mapping already exists
            mappings = lambda_client.list_event_source_mappings(
                FunctionName=function_name,
                EventSourceArn=stream_arn
            )

            if not mappings['EventSourceMappings']:
                # Create event source mapping
                result = lambda_client.create_event_source_mapping(
                    EventSourceArn=stream_arn,
                    FunctionName=function_name,
                    StartingPosition=starting_position,
                    BatchSize=batch_size,
                    MaximumBatchingWindowInSeconds=0,
                    ParallelizationFactor=parallelization_factor,
                    MaximumRetryAttempts=retry_attempts,
                    BisectBatchOnFunctionError=bisect_batch_on_error,
                    FunctionResponseTypes=['ReportBatchItemFailures'] if report_batch_item_failures else []
                )
                physical_id = result['UUID']
                print(f"Created event source mapping: {physical_id}")
            else:
                physical_id = mappings['EventSourceMappings'][0]['UUID']
                print(f"Event source mapping already exists: {physical_id}")

                # Update if needed
                if request_type == 'Update':
                    lambda_client.update_event_source_mapping(
                        UUID=physical_id,
                        BatchSize=batch_size,
                        MaximumBatchingWindowInSeconds=0,
                        ParallelizationFactor=parallelization_factor,
                        MaximumRetryAttempts=retry_attempts,
                        BisectBatchOnFunctionError=bisect_batch_on_error,
                        FunctionResponseTypes=['ReportBatchItemFailures'] if report_batch_item_failures else []
                    )

            cfnresponse.send(event, context, cfnresponse.SUCCESS, {}, physical_id)

        elif request_type == 'Delete':
            # Try to delete mapping if it exists
            physical_id = event.get('PhysicalResourceId', '')
            if physical_id and physical_id != 'None':
                try:
                    lambda_client.delete_event_source_mapping(UUID=physical_id)
                    print(f"Deleted event source mapping: {physical_id}")
                except Exception as e:
                    print(f"Error deleting mapping (ignoring): {e}")
                    pass  # Ignore errors on delete
            cfnresponse.send(event, context, cfnresponse.SUCCESS, {})

    except Exception as e:
        print(f"Error: {e}")
        cfnresponse.send(event, context, cfnresponse.FAILED, {'Error': str(e)}, 'None')
`)
}

// getBoolValue returns the bool value or a default
func getBoolValue(val *bool, defaultVal bool) bool {
	if val != nil {
		return *val
	}
	return defaultVal
}
