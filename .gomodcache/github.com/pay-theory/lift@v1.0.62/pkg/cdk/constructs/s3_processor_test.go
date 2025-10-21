package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

func TestS3Processor_DefaultConfiguration(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-s3-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	// Test that all components are created
	if processor.Function == nil {
		t.Error("Function should be created")
	}
	if processor.Bucket == nil {
		t.Error("Bucket should be created")
	}
	if processor.DeadLetterQueue == nil {
		t.Error("Dead letter queue should be created by default")
	}
	if processor.EventSource == nil {
		t.Error("Event source should be created")
	}

	// Synthesize to verify template
	template := synthesizeTemplate(t, stack)

	// Verify S3 bucket exists
	assertResourceExists(t, template, "AWS::S3::Bucket")

	// Verify dead letter queue exists
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify Lambda function
	assertResourceExists(t, template, "AWS::Lambda::Function")

	// Verify bucket notification configuration
	bucketResources := findResourcesByType(template, "AWS::S3::Bucket")
	if len(bucketResources) == 0 {
		t.Error("Should have S3 bucket")
	}

	// Verify bucket has security configurations
	for _, bucket := range bucketResources {
		propsVal := bucket["Properties"]
		props, ok := propsVal.(map[string]interface{})
		if !ok {
			t.Error("Bucket should have Properties")
			continue
		}
		if publicAccess, ok := props["PublicAccessBlockConfiguration"]; ok {
			config, ok2 := publicAccess.(map[string]interface{})
			ok = ok2
			if !ok {
				t.Error("PublicAccessBlockConfiguration should be a map")
				continue
			}
			if config["BlockPublicAcls"] != true {
				t.Error("Bucket should block public ACLs")
			}
		}
	}
}

func TestS3Processor_CustomConfiguration(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("custom-s3-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		BucketProps: &awss3.BucketProps{
			BucketName: jsii.String("custom-bucket-name"),
			Versioned:  jsii.Bool(true),
		},
		EventTypes: &[]awss3.EventType{
			awss3.EventType_OBJECT_CREATED,
			awss3.EventType_OBJECT_REMOVED,
		},
		KeyPrefix:         jsii.String("uploads/"),
		KeySuffix:         jsii.String(".jpg"),
		EnableVersioning:  jsii.Bool(true),
		EnableTracing:     jsii.Bool(true),
		EnableMultiTenant: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify custom configuration is applied
	assertResourceExists(t, template, "AWS::S3::Bucket")

	// Verify Lambda function has environment variables
	functionResources := findResourcesByType(template, "AWS::Lambda::Function")
	if len(functionResources) == 0 {
		t.Error("Should have Lambda function")
	}

	for _, function := range functionResources {
		props, ok := function["Properties"].(map[string]interface{})
		if !ok {
			t.Error("Function should have Properties")
			continue
		}
		if env, ok := props["Environment"]; ok {
			envProps, ok := env.(map[string]interface{})
			if !ok {
				t.Error("Environment should be a map")
				continue
			}
			if variables, ok := envProps["Variables"]; ok {
				vars, ok := variables.(map[string]interface{})
				if !ok {
					t.Error("Variables should be a map")
					continue
				}
				if vars["S3_BUCKET_NAME"] == nil {
					t.Error("Function should have S3_BUCKET_NAME environment variable")
				}
				if vars["S3_BUCKET_ARN"] == nil {
					t.Error("Function should have S3_BUCKET_ARN environment variable")
				}
			}
		}
	}
}

func TestS3Processor_ExistingBucket(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create existing bucket
	existingBucket := awss3.NewBucket(stack, jsii.String("ExistingBucket"), &awss3.BucketProps{
		BucketName: jsii.String("existing-bucket"),
	})

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("existing-bucket-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		ExistingBucket: existingBucket,
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	if processor.Bucket != existingBucket {
		t.Error("Processor should use existing bucket")
	}

	template := synthesizeTemplate(t, stack)

	// Should have exactly one bucket (the existing one)
	bucketResources := findResourcesByType(template, "AWS::S3::Bucket")
	if len(bucketResources) != 1 {
		t.Errorf("Should have exactly 1 bucket, found %d", len(bucketResources))
	}
}

func TestS3Processor_DisabledDeadLetterQueue(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("no-dlq-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableDeadLetterQueue: jsii.Bool(false),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	if processor.DeadLetterQueue != nil {
		t.Error("Dead letter queue should not be created when disabled")
	}

	template := synthesizeTemplate(t, stack)

	// Verify no SQS queues are created
	assertResourceCount(t, template, "AWS::SQS::Queue", 0)
}

func TestS3Processor_LifecycleRules(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("lifecycle-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableLifecycleRules: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify bucket has lifecycle configuration
	bucketResources := findResourcesByType(template, "AWS::S3::Bucket")
	if len(bucketResources) == 0 {
		t.Error("Should have S3 bucket")
	}

	for _, bucket := range bucketResources {
		props, ok := bucket["Properties"].(map[string]interface{})
		if !ok {
			t.Error("Bucket should have Properties")
			continue
		}
		if _, ok := props["LifecycleConfiguration"]; !ok {
			t.Error("Bucket should have lifecycle configuration when enabled")
		}
	}
}

func TestS3Processor_CustomEventSourceProps(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	customFilters := []*awss3.NotificationKeyFilter{
		{Prefix: jsii.String("images/")},
		{Suffix: jsii.String(".png")},
	}

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("custom-events-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EventSourceProps: &awslambdaeventsources.S3EventSourceProps{
			Events: &[]awss3.EventType{
				awss3.EventType_OBJECT_CREATED_PUT,
			},
			Filters: &customFilters,
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	// Test that event source was configured
	if processor.EventSource == nil {
		t.Error("Event source should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify bucket notification is configured
	bucketResources := findResourcesByType(template, "AWS::S3::Bucket")
	if len(bucketResources) == 0 {
		t.Error("Should have S3 bucket")
	}
}

func TestS3Processor_EnvironmentVariables(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("env-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
			Environment: &map[string]*string{
				"CUSTOM_VAR": jsii.String("custom_value"),
			},
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify Lambda function has both custom and S3-specific environment variables
	functionResources := findResourcesByType(template, "AWS::Lambda::Function")
	if len(functionResources) == 0 {
		t.Error("Should have Lambda function")
	}

	for _, function := range functionResources {
		props, ok := function["Properties"].(map[string]interface{})
		if !ok {
			t.Error("Function should have Properties")
			continue
		}
		if env, ok := props["Environment"]; ok {
			envProps, ok := env.(map[string]interface{})
			if !ok {
				t.Error("Environment should be a map")
				continue
			}
			if variables, ok := envProps["Variables"]; ok {
				vars, ok := variables.(map[string]interface{})
				if !ok {
					t.Error("Variables should be a map")
					continue
				}

				// Check custom environment variable is preserved
				if vars["CUSTOM_VAR"] != "custom_value" {
					t.Error("Custom environment variable should be preserved")
				}

				// Check S3-specific environment variables are added
				if vars["S3_BUCKET_NAME"] == nil {
					t.Error("Function should have S3_BUCKET_NAME environment variable")
				}
				if vars["S3_BUCKET_ARN"] == nil {
					t.Error("Function should have S3_BUCKET_ARN environment variable")
				}
				if vars["S3_DLQ_URL"] == nil {
					t.Error("Function should have S3_DLQ_URL environment variable")
				}
			}
		}
	}
}

func TestS3Processor_PermissionGrants(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("permissions-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	// Test helper methods
	if processor.GetBucketName() == nil {
		t.Error("GetBucketName should return bucket name")
	}

	if processor.GetBucketArn() == nil {
		t.Error("GetBucketArn should return bucket ARN")
	}

	if processor.GetBucketDomainName() == nil {
		t.Error("GetBucketDomainName should return bucket domain name")
	}

	// Test that we can add environment variables
	processor.AddEnvironmentVariable("TEST_VAR", "test_value")

	template := synthesizeTemplate(t, stack)

	// Verify IAM policies are created for S3 and SQS permissions
	policyResources := findResourcesByType(template, "AWS::IAM::Policy")
	if len(policyResources) == 0 {
		t.Error("Should have IAM policies for S3 and SQS permissions")
	}
}

func TestS3Processor_MonitoringEnabled(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("MonitoredS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("monitored-s3-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableMonitoring: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Should create CloudWatch alarms and a dashboard
	assertResourceExists(t, template, "AWS::CloudWatch::Alarm")
	assertResourceExists(t, template, "AWS::CloudWatch::Dashboard")
}

func TestS3Processor_ErrorHandling(t *testing.T) {
	// Test creation with minimal required props
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("error-handling-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created with minimal props")
	}

	// Test that defaults are applied
	if processor.Function == nil {
		t.Error("Function should be created with defaults")
	}
	if processor.Bucket == nil {
		t.Error("Bucket should be created with defaults")
	}
}

func TestS3Processor_HelperMethods(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a test function for permission testing
	testFunction := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("test-function"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	processor := NewS3Processor(stack, jsii.String("TestS3Processor"), &S3ProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("helper-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	// Test permission granting methods (these should not panic)
	processor.GrantRead(testFunction)
	processor.GrantWrite(testFunction)
	processor.GrantReadWrite(testFunction)
	processor.GrantDelete(testFunction)

	// Test adding environment variables
	processor.AddEnvironmentVariable("HELPER_TEST", "value")

	template := synthesizeTemplate(t, stack)

	// Verify additional IAM policies are created
	policyResources := findResourcesByType(template, "AWS::IAM::Policy")
	if len(policyResources) == 0 {
		t.Error("Should have IAM policies for granted permissions")
	}
}
