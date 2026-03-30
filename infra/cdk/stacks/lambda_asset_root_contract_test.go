package stacks

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
)

func TestLesserApiStackLambdaFunctionsPropsUsesConfiguredLambdaAssetRoot(t *testing.T) {
	stack := awscdk.NewStack(awscdk.NewApp(nil), jsii.String("TestStack"), nil)

	table := awsdynamodb.NewTable(stack, jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	rateTable := awsdynamodb.NewTable(stack, jsii.String("RateLimitTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	streamEventsTable := awsdynamodb.NewTable(stack, jsii.String("StreamEventsTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	mediaBucket := awss3.NewBucket(stack, jsii.String("MediaBucket"), nil)
	streamingBucket := awss3.NewBucket(stack, jsii.String("StreamingBucket"), nil)
	trainingBucket := awss3.NewBucket(stack, jsii.String("TrainingBucket"), nil)
	privateKey := awssecretsmanager.NewSecret(stack, jsii.String("PrivateKey"), nil)
	jwtSecret := awssecretsmanager.NewSecret(stack, jsii.String("JwtSecret"), nil)
	encryptionRole := awsiam.NewRole(stack, jsii.String("EncryptionRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	basicRole := awsiam.NewRole(stack, jsii.String("BasicRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	apiStack := &LesserApiStack{
		Stack:                  stack,
		AppName:                "app",
		Environment:            "development",
		Domain:                 "dev.example.com",
		Configuration:          map[string]interface{}{"lambdaAssetRoot": " /tmp/release-assets "},
		MainTable:              table,
		RateLimitTable:         rateTable,
		StreamEventsTable:      streamEventsTable,
		MediaBucket:            mediaBucket,
		StreamingBucket:        streamingBucket,
		TrainingBucket:         trainingBucket,
		PrivateKey:             privateKey,
		JwtSecret:              jwtSecret,
		ModelMetadataTableName: "model-metadata",
		LambdaEncryptionRole:   encryptionRole,
		LambdaBasicRole:        basicRole,
		MediaConvertRole:       awsiam.NewRole(stack, jsii.String("MediaConvertRole"), &awsiam.RoleProps{AssumedBy: awsiam.NewServicePrincipal(jsii.String("mediaconvert.amazonaws.com"), nil)}),
	}

	props := apiStack.lambdaFunctionsProps()
	if got, want := props.LambdaAssetRoot, "/tmp/release-assets"; got != want {
		t.Fatalf("LambdaAssetRoot = %q, want %q", got, want)
	}
}
