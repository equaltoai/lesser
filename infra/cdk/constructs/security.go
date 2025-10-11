package constructs

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/jsii-runtime-go"
)

type SecurityProps struct {
	Environment     string
	Table           awsdynamodb.Table
	MediaBucket     awss3.Bucket
	FederationQueue awssqs.Queue
	FederationDLQ   awssqs.Queue
	PushQueue       awssqs.Queue
}

type SecurityConstructs struct {
	LambdaRole     awsiam.Role
	DynamoDBPolicy awsiam.Policy
	S3Policy       awsiam.Policy
	SQSPolicy      awsiam.Policy
	BedrockPolicy  awsiam.Policy
	KMSPolicy      awsiam.Policy
	KMSKey         awskms.IKey
}

func CreateSecurityConstructs(stack awscdk.Stack, props *SecurityProps) *SecurityConstructs {
	security := &SecurityConstructs{}

	// Create Lambda execution role
	security.LambdaRole = awsiam.NewRole(stack, jsii.String("LesserLambdaRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	// Attach basic execution policy
	security.LambdaRole.AddManagedPolicy(awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")))

	// Create DynamoDB policy matching Pulumi exactly (lines 434-476)
	security.DynamoDBPolicy = createDynamoDBPolicy(stack, props.Table)
	security.LambdaRole.AttachInlinePolicy(security.DynamoDBPolicy)

	// Create S3 policy matching Pulumi exactly (lines 487-513)
	security.S3Policy = createS3Policy(stack, props.MediaBucket)
	security.LambdaRole.AttachInlinePolicy(security.S3Policy)

	// Create SQS policy matching Pulumi exactly (lines 516-551)
	security.SQSPolicy = createSQSPolicy(stack, props.FederationQueue, props.FederationDLQ, props.PushQueue)
	security.LambdaRole.AttachInlinePolicy(security.SQSPolicy)

	// Create Bedrock policy matching Pulumi exactly (lines 556-586)
	security.BedrockPolicy = createBedrockPolicy(stack)
	security.LambdaRole.AttachInlinePolicy(security.BedrockPolicy)

	// Create KMS policy using ARN pattern (no lookup required)
	security.KMSPolicy = createKMSPolicyByPattern(stack, props.Environment)
	security.LambdaRole.AttachInlinePolicy(security.KMSPolicy)

	// Add Secrets Manager permissions for JWT secret and actor private key
	security.LambdaRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("secretsmanager:GetSecretValue"),
			jsii.String("secretsmanager:DescribeSecret"),
		},
		Resources: &[]*string{
			jsii.String("arn:aws:secretsmanager:*:*:secret:lesser/jwt-secret-*"),
			jsii.String("arn:aws:secretsmanager:*:*:secret:lesser/actor-private-key-*"),
		},
	}))

	return security
}

func createDynamoDBPolicy(stack awscdk.Stack, table awsdynamodb.Table) awsiam.Policy {
	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"dynamodb:GetItem",
					"dynamodb:PutItem",
					"dynamodb:UpdateItem",
					"dynamodb:DeleteItem",
					"dynamodb:Query",
					"dynamodb:Scan",
					"dynamodb:BatchGetItem",
					"dynamodb:BatchWriteItem",
				},
				"Resource": []*string{
					table.TableArn(),
					jsii.String(*table.TableArn() + "/index/*"),
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"dynamodb:DescribeStream",
					"dynamodb:GetRecords",
					"dynamodb:GetShardIterator",
					"dynamodb:ListStreams",
				},
				"Resource": []*string{
					jsii.String(*table.TableArn() + "/stream/*"),
				},
			},
		},
	}

	policyJSON, _ := json.Marshal(policyDoc)
	var jsonData interface{}
	_ = json.Unmarshal(policyJSON, &jsonData)
	document := awsiam.PolicyDocument_FromJson(jsonData)

	return awsiam.NewPolicy(stack, jsii.String("LambdaDynamoDBPolicy"), &awsiam.PolicyProps{
		Document: document,
	})
}

func createS3Policy(stack awscdk.Stack, bucket awss3.Bucket) awsiam.Policy {
	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"s3:GetObject",
					"s3:PutObject",
					"s3:DeleteObject",
					"s3:PutObjectAcl",
				},
				"Resource": jsii.String(*bucket.BucketArn() + "/*"),
			},
		},
	}

	policyJSON, _ := json.Marshal(policyDoc)
	var jsonData interface{}
	_ = json.Unmarshal(policyJSON, &jsonData)
	document := awsiam.PolicyDocument_FromJson(jsonData)

	return awsiam.NewPolicy(stack, jsii.String("LambdaS3Policy"), &awsiam.PolicyProps{
		Document: document,
	})
}

func createSQSPolicy(stack awscdk.Stack, federationQueue, federationDLQ, pushQueue awssqs.Queue) awsiam.Policy {
	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"sqs:SendMessage",
					"sqs:ReceiveMessage",
					"sqs:DeleteMessage",
					"sqs:GetQueueAttributes",
					"sqs:ChangeMessageVisibility",
					"sqs:GetQueueUrl",
				},
				"Resource": []*string{
					federationQueue.QueueArn(),
					federationDLQ.QueueArn(),
					pushQueue.QueueArn(),
				},
			},
		},
	}

	policyJSON, _ := json.Marshal(policyDoc)
	var jsonData interface{}
	_ = json.Unmarshal(policyJSON, &jsonData)
	document := awsiam.PolicyDocument_FromJson(jsonData)

	return awsiam.NewPolicy(stack, jsii.String("LambdaSQSPolicy"), &awsiam.PolicyProps{
		Document: document,
	})
}

func createBedrockPolicy(stack awscdk.Stack) awsiam.Policy {
	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"bedrock:InvokeModel",
					"bedrock:InvokeModelWithResponseStream",
				},
				"Resource": "arn:aws:bedrock:*::foundation-model/amazon.titan-embed-text-v1",
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"comprehend:DetectDominantLanguage",
					"comprehend:DetectEntities",
					"comprehend:DetectKeyPhrases",
					"comprehend:DetectSentiment",
				},
				"Resource": "*",
			},
		},
	}

	policyJSON, _ := json.Marshal(policyDoc)
	var jsonData interface{}
	_ = json.Unmarshal(policyJSON, &jsonData)
	document := awsiam.PolicyDocument_FromJson(jsonData)

	return awsiam.NewPolicy(stack, jsii.String("LambdaAIPolicy"), &awsiam.PolicyProps{
		Document: document,
	})
}

// createKMSPolicyByPattern creates KMS policy using ARN pattern (no key lookup needed)
func createKMSPolicyByPattern(stack awscdk.Stack, environment string) awsiam.Policy {
	// Construct KMS key ARN pattern - works without looking up the actual key
	// Format: arn:aws:kms:region:account:alias/lesser-encryption
	kmsArnPattern := fmt.Sprintf("arn:aws:kms:*:*:alias/lesser-encryption")

	policyDoc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"kms:Encrypt",
					"kms:Decrypt",
					"kms:GenerateDataKey",
					"kms:DescribeKey",
				},
				"Resource": kmsArnPattern,
			},
		},
	}

	policyJSON, _ := json.Marshal(policyDoc)
	var jsonData interface{}
	_ = json.Unmarshal(policyJSON, &jsonData)
	document := awsiam.PolicyDocument_FromJson(jsonData)

	return awsiam.NewPolicy(stack, jsii.String("LambdaKMSPolicy"), &awsiam.PolicyProps{
		Document: document,
	})
}
