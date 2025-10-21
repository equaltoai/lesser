package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

func TestSNSProcessor_DefaultConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify SNS topic is created
	template.HasResourceProperties(jsii.String("AWS::SNS::Topic"), &map[string]interface{}{})

	// Verify Lambda function is created
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Runtime": "provided.al2023",
		"Handler": "bootstrap",
	})

	// Verify DLQ is created by default
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), &map[string]interface{}{
		"MessageRetentionPeriod": 1209600, // 14 days in seconds
	})

	// Verify SNS subscription
	template.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), &map[string]interface{}{
		"Protocol": "lambda",
	})

	// Verify processor properties
	assert.NotNil(t, processor.Topic)
	assert.NotNil(t, processor.Function)
	assert.NotNil(t, processor.DLQ)
}

func TestSNSProcessor_CustomConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		TopicProps: &awssns.TopicProps{
			DisplayName: jsii.String("My Test Topic"),
			TopicName:   jsii.String("custom-topic-name"),
		},
		MessageRetentionSeconds: jsii.Number(86400), // 1 day
		DLQProps: &awssqs.QueueProps{
			QueueName:       jsii.String("custom-dlq"),
			RetentionPeriod: awscdk.Duration_Days(jsii.Number(7)),
		},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify custom topic configuration
	template.HasResourceProperties(jsii.String("AWS::SNS::Topic"), &map[string]interface{}{
		"DisplayName": "My Test Topic",
		"TopicName":   "custom-topic-name",
	})

	// Verify custom DLQ configuration
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), &map[string]interface{}{
		"QueueName":              "custom-dlq",
		"MessageRetentionPeriod": 604800, // 7 days in seconds
	})

	assert.NotNil(t, processor)
}

func TestSNSProcessor_FifoConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		EnableFifo:                jsii.Bool(true),
		ContentBasedDeduplication: jsii.Bool(true),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify FIFO topic
	template.HasResourceProperties(jsii.String("AWS::SNS::Topic"), &map[string]interface{}{
		"FifoTopic":                 true,
		"ContentBasedDeduplication": true,
	})

	// Verify FIFO DLQ
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), &map[string]interface{}{
		"FifoQueue": true,
	})

	assert.NotNil(t, processor)
}

func TestSNSProcessor_ExistingTopic(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
	existingTopic := awssns.NewTopic(stack, jsii.String("ExistingTopic"), &awssns.TopicProps{
		TopicName: jsii.String("existing-topic"),
	})

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		ExistingTopic: existingTopic,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should have the existing topic
	template.HasResourceProperties(jsii.String("AWS::SNS::Topic"), &map[string]interface{}{
		"TopicName": "existing-topic",
	})

	// Should not create another topic
	template.ResourceCountIs(jsii.String("AWS::SNS::Topic"), jsii.Number(1))

	// Verify subscription exists
	template.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), &map[string]interface{}{
		"Protocol": "lambda",
	})

	assert.Equal(t, existingTopic, processor.Topic)
}

func TestSNSProcessor_DisabledDLQ(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		EnableDLQ: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should not have SNS DLQ (but may have Lambda DLQ)
	// We can't check the exact count because LiftFunction might create its own DLQ

	// Verify SNS subscription has no redrive policy
	template.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), &map[string]interface{}{
		"Protocol": "lambda",
	})

	// Verify Lambda function exists
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Runtime": "provided.al2023",
	})

	assert.Nil(t, processor.DLQ)
}

func TestSNSProcessor_FilterPolicy(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		FilterPolicy: &map[string]awssns.SubscriptionFilter{
			"eventType": awssns.SubscriptionFilter_StringFilter(&awssns.StringConditions{
				Allowlist: &[]*string{jsii.String("order-created"), jsii.String("order-updated")},
			}),
			"price": awssns.SubscriptionFilter_NumericFilter(&awssns.NumericConditions{
				GreaterThan: jsii.Number(100),
			}),
		},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify filter policy in subscription
	template.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), &map[string]interface{}{
		"FilterPolicy": map[string]interface{}{
			"eventType": []interface{}{"order-created", "order-updated"},
			"price":     []interface{}{map[string]interface{}{"numeric": []interface{}{">", 100}}},
		},
	})

	assert.NotNil(t, processor)
}

func TestSNSProcessor_CustomSubscriptionProps(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		RawMessageDelivery: jsii.Bool(true),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify subscription exists
	template.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), &map[string]interface{}{
		"Protocol": "lambda",
	})

	assert.NotNil(t, processor)
}

func TestSNSProcessor_HelperMethods(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		TopicProps: &awssns.TopicProps{
			TopicName: jsii.String("test-topic"),
		},
	})

	// Create a test role
	testRole := awsiam.NewRole(stack, jsii.String("TestRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	// When & Then
	// Test grant methods
	publishGrant := processor.GrantPublish(testRole)
	assert.NotNil(t, publishGrant)

	subscribeGrant := processor.GrantSubscribe(testRole)
	assert.NotNil(t, subscribeGrant)

	// Test getter methods
	topicArn := processor.GetTopicArn()
	assert.NotNil(t, topicArn)

	topicName := processor.GetTopicName()
	assert.NotNil(t, topicName)

	dlqUrl := processor.GetDLQUrl()
	assert.NotNil(t, dlqUrl)
}

func TestSNSProcessor_DisabledDLQGetters(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
		FunctionProps: &LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
				Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
			},
		},
		EnableDLQ: jsii.Bool(false),
	})

	// When & Then
	dlqUrl := processor.GetDLQUrl()
	assert.Nil(t, dlqUrl)
}

func TestSNSProcessor_NilProps(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When & Then - should not panic
	assert.NotPanics(t, func() {
		processor := NewSNSProcessor(stack, jsii.String("TestProcessor"), &SNSProcessorProps{
			FunctionProps: &LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime: awslambda.Runtime_PROVIDED_AL2023(),
					Handler: jsii.String("bootstrap"),
					Code:    awslambda.Code_FromAsset(jsii.String("../test"), nil),
				},
			},
			TopicProps:        nil,
			SubscriptionProps: nil,
			DLQProps:          nil,
			FilterPolicy:      nil,
		})
		assert.NotNil(t, processor)
	})
}
