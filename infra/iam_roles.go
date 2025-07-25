package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// FunctionPermissions defines the permissions needed by each Lambda function
type FunctionPermissions struct {
	DynamoDBTables       []string
	DynamoDBIndices      []string
	S3Buckets            []string
	SQSQueues            []string
	AllowDynamoDBStreams bool
	AllowS3Write         bool
	AllowAIServices      bool
	AllowWebSocket       bool
	AllowKMS             bool
	CustomPolicies       []string
}

// functionPermissionsMap defines the specific permissions each function needs
var functionPermissionsMap = map[string]FunctionPermissions{
	"api": {
		DynamoDBTables:  []string{"read", "write"}, // Main table access
		DynamoDBIndices: []string{"query"},         // Query GSI indices
		S3Buckets:       []string{"read"},          // Read media
		AllowKMS:        true,                      // Encrypt/decrypt actor private keys
	},
	"auth": {
		DynamoDBTables: []string{"read", "write"}, // User/session management
		AllowKMS:       true,                      // Encrypt/decrypt actor private keys
	},
	"webfinger": {
		DynamoDBTables: []string{"read"}, // Read actor info only
	},
	"actor": {
		DynamoDBTables: []string{"read", "write"}, // Actor management
		AllowKMS:       true,                      // Encrypt/decrypt actor private keys
	},
	"inbox": {
		DynamoDBTables: []string{"read", "write"}, // Store activities
		SQSQueues:      []string{"send"},          // Queue federation tasks
	},
	"outbox": {
		DynamoDBTables: []string{"read", "write"}, // Store activities
		SQSQueues:      []string{"send"},          // Queue federation tasks
	},
	"objects": {
		DynamoDBTables: []string{"read", "write"}, // Object storage
		S3Buckets:      []string{"read"},          // Read media
	},
	"collections": {
		DynamoDBTables: []string{"read", "write"}, // Collection management
	},
	"graphql": {
		DynamoDBTables:  []string{"read", "write"}, // Full table access
		DynamoDBIndices: []string{"query"},         // Query indices
		S3Buckets:       []string{"read"},          // Read media
	},
	"federation-delivery": {
		DynamoDBTables: []string{"read", "write"}, // Read activities, update delivery status
		SQSQueues:      []string{"receive"},       // Receive from queue
	},
	"push-delivery": {
		DynamoDBTables: []string{"read"},    // Read subscriptions
		SQSQueues:      []string{"receive"}, // Receive from queue
	},
	"activity-processor": {
		DynamoDBTables:       []string{"read", "write"}, // Process activities
		AllowDynamoDBStreams: true,                      // Read from streams
	},
	"cost-aggregator": {
		DynamoDBTables:       []string{"read", "write"}, // Cost history table
		AllowDynamoDBStreams: true,                      // Read from streams
	},
	"ai-processor": {
		DynamoDBTables:       []string{"read", "write"}, // Update with AI data
		AllowDynamoDBStreams: true,                      // Read from streams
		AllowAIServices:      true,                      // Bedrock & Comprehend
	},
	"note-processor": {
		DynamoDBTables:       []string{"read", "write"}, // Process notes
		AllowDynamoDBStreams: true,                      // Read from streams
	},
	"moderation-processor": {
		DynamoDBTables:       []string{"read", "write"}, // Moderation actions
		AllowDynamoDBStreams: true,                      // Read from streams
	},
	"report-trust-updater": {
		DynamoDBTables: []string{"read", "write"}, // Update trust scores
	},
	"federation-tracker": {
		DynamoDBTables: []string{"read", "write"}, // Track federation metrics
	},
	"import-processor": {
		DynamoDBTables: []string{"read", "write"}, // Import data
		S3Buckets:      []string{"read", "write"}, // Read imports, write media
		AllowS3Write:   true,
	},
	"export-generator": {
		DynamoDBTables: []string{"read"},  // Read data to export
		S3Buckets:      []string{"write"}, // Write export files
		AllowS3Write:   true,
	},
	"media-processor": {
		S3Buckets:    []string{"read", "write"}, // Process media files
		AllowS3Write: true,
	},
	"trend-aggregator": {
		DynamoDBTables: []string{"read", "write"}, // Aggregate trends
	},
	"status-indexer": {
		DynamoDBTables:       []string{"read", "write"}, // Index statuses
		AllowDynamoDBStreams: true,                      // Read from streams
	},
	"streaming-ws": {
		DynamoDBTables: []string{"read", "write"}, // WebSocket connections
		AllowWebSocket: true,                      // API Gateway management
	},
	"stream-router-ws": {
		DynamoDBTables:       []string{"read"}, // Read subscriptions
		AllowDynamoDBStreams: true,             // Read from streams
		AllowWebSocket:       true,             // Send to connections
	},
}

// CreateLambdaRole creates an IAM role for a specific Lambda function with least privilege
func CreateLambdaRole(ctx *pulumi.Context, functionName string, tableArn pulumi.StringOutput,
	costTableArn pulumi.StringOutput, bucketArn pulumi.StringOutput,
	federationQueueArn pulumi.StringOutput, pushQueueArn pulumi.StringOutput,
	wsTableArns []pulumi.StringOutput, kmsKeyArn pulumi.StringOutput,
) (*iam.Role, error) {
	// Create the base Lambda execution role
	role, err := iam.NewRole(ctx, fmt.Sprintf("lesser-%s-role", functionName), &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Action": "sts:AssumeRole",
				"Effect": "Allow",
				"Principal": {
					"Service": "lambda.amazonaws.com"
				}
			}]
		}`),
		Tags: pulumi.StringMap{
			"Function": pulumi.String(functionName),
			"Service":  pulumi.String("lesser"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Attach basic Lambda execution policy
	_, err = iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("%s-basic-execution", functionName), &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
	})
	if err != nil {
		return nil, err
	}

	// Get permissions for this function
	perms, ok := functionPermissionsMap[functionName]
	if !ok {
		// Default minimal permissions
		perms = FunctionPermissions{
			DynamoDBTables: []string{"read"},
		}
	}

	// Create DynamoDB policy if needed
	if len(perms.DynamoDBTables) > 0 {
		dynamoActions := []string{}
		for _, perm := range perms.DynamoDBTables {
			switch perm {
			case "read":
				dynamoActions = append(dynamoActions,
					"dynamodb:GetItem",
					"dynamodb:BatchGetItem",
					"dynamodb:Query",
					"dynamodb:Scan")
			case "write":
				dynamoActions = append(dynamoActions,
					"dynamodb:PutItem",
					"dynamodb:UpdateItem",
					"dynamodb:DeleteItem",
					"dynamodb:BatchWriteItem")
			}
		}

		// Build resource list based on function needs
		resourceList := pulumi.StringArray{}

		// Main table access
		if functionName != "cost-aggregator" {
			resourceList = append(resourceList, tableArn)
			if len(perms.DynamoDBIndices) > 0 {
				resourceList = append(resourceList, pulumi.Sprintf("%s/index/*", tableArn))
			}
		}

		// Cost table access (only for cost-aggregator)
		if functionName == "cost-aggregator" {
			resourceList = append(resourceList, costTableArn)
			resourceList = append(resourceList, pulumi.Sprintf("%s/index/*", costTableArn))
		}

		// WebSocket tables access
		if functionName == "streaming-ws" || functionName == "stream-router-ws" {
			for _, arn := range wsTableArns {
				resourceList = append(resourceList, arn)
			}
		}

		dynamoPolicy := pulumi.All(resourceList).ApplyT(func(args []any) (string, error) {
			resources := []string{}
			for _, r := range args {
				if str, ok := r.(string); ok && str != "" {
					resources = append(resources, str)
				}
			}

			policy := map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect":   "Allow",
						"Action":   dynamoActions,
						"Resource": resources,
					},
				},
			}

			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-dynamodb", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: dynamoPolicy,
		})
		if err != nil {
			return nil, err
		}
	}

	// Add DynamoDB Streams permissions if needed
	if perms.AllowDynamoDBStreams {
		streamsPolicy := tableArn.ApplyT(func(arn string) (string, error) {
			policy := map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect": "Allow",
						"Action": []string{
							"dynamodb:DescribeStream",
							"dynamodb:GetRecords",
							"dynamodb:GetShardIterator",
							"dynamodb:ListStreams",
						},
						"Resource": fmt.Sprintf("%s/stream/*", arn),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-streams", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: streamsPolicy,
		})
		if err != nil {
			return nil, err
		}
	}

	// Add S3 permissions if needed
	if len(perms.S3Buckets) > 0 {
		s3Actions := []string{}
		for _, perm := range perms.S3Buckets {
			switch perm {
			case "read":
				s3Actions = append(s3Actions, "s3:GetObject", "s3:GetObjectVersion")
			case "write":
				s3Actions = append(s3Actions, "s3:PutObject", "s3:PutObjectAcl", "s3:DeleteObject")
			}
		}

		s3Policy := bucketArn.ApplyT(func(arn string) (string, error) {
			policy := map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect":   "Allow",
						"Action":   s3Actions,
						"Resource": fmt.Sprintf("%s/*", arn),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-s3", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: s3Policy,
		})
		if err != nil {
			return nil, err
		}
	}

	// Add SQS permissions if needed
	if len(perms.SQSQueues) > 0 {
		sqsActions := []string{}
		for _, perm := range perms.SQSQueues {
			switch perm {
			case "send":
				sqsActions = append(sqsActions, "sqs:SendMessage", "sqs:GetQueueUrl")
			case "receive":
				sqsActions = append(sqsActions,
					"sqs:ReceiveMessage",
					"sqs:DeleteMessage",
					"sqs:GetQueueAttributes",
					"sqs:ChangeMessageVisibility")
			}
		}

		queues := pulumi.StringArray{}
		if functionName == "inbox" || functionName == "outbox" || functionName == "federation-delivery" {
			queues = append(queues, federationQueueArn)
		}
		if functionName == "push-delivery" {
			queues = append(queues, pushQueueArn)
		}

		if len(queues) > 0 {
			sqsPolicy := pulumi.All(queues).ApplyT(func(args []any) (string, error) {
				resources := []string{}
				for _, r := range args {
					if str, ok := r.(string); ok && str != "" {
						resources = append(resources, str)
					}
				}

				policy := map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{
							"Effect":   "Allow",
							"Action":   sqsActions,
							"Resource": resources,
						},
					},
				}
				policyJSON, err := json.Marshal(policy)
				return string(policyJSON), err
			})

			_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-sqs", functionName), &iam.RolePolicyArgs{
				Role:   role.Name,
				Policy: sqsPolicy,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	// Add AI services permissions if needed
	if perms.AllowAIServices {
		aiPolicy := pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"bedrock:InvokeModel",
						"bedrock:InvokeModelWithResponseStream"
					],
					"Resource": "arn:aws:bedrock:*::foundation-model/amazon.titan-embed-text-v1"
				},
				{
					"Effect": "Allow",
					"Action": [
						"comprehend:DetectDominantLanguage",
						"comprehend:DetectEntities",
						"comprehend:DetectKeyPhrases",
						"comprehend:DetectSentiment"
					],
					"Resource": "*"
				}
			]
		}`)

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-ai", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: aiPolicy,
		})
		if err != nil {
			return nil, err
		}
	}

	// Add WebSocket API Gateway management permissions if needed
	if perms.AllowWebSocket {
		wsPolicy := pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Action": [
					"execute-api:ManageConnections"
				],
				"Resource": "arn:aws:execute-api:*:*:*/@connections/*"
			}]
		}`)

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-websocket", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: wsPolicy,
		})
		if err != nil {
			return nil, err
		}
	}

	// Add KMS permissions if needed
	if perms.AllowKMS {
		kmsPolicy := kmsKeyArn.ApplyT(func(arn string) (string, error) {
			policy := map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect": "Allow",
						"Action": []string{
							"kms:Encrypt",
							"kms:Decrypt",
							"kms:GenerateDataKey",
							"kms:DescribeKey",
						},
						"Resource": arn,
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, fmt.Sprintf("%s-kms", functionName), &iam.RolePolicyArgs{
			Role:   role.Name,
			Policy: kmsPolicy,
		})
		if err != nil {
			return nil, err
		}
	}

	return role, nil
}

// CreateLambdaWithRole creates a Lambda function with its own IAM role
func CreateLambdaWithRole(ctx *pulumi.Context, name string, handler string, timeout int,
	tableArn pulumi.StringOutput, costTableArn pulumi.StringOutput,
	bucketArn pulumi.StringOutput, federationQueueArn pulumi.StringOutput,
	pushQueueArn pulumi.StringOutput, wsTableArns []pulumi.StringOutput,
	kmsKeyArn pulumi.StringOutput, lambdaEnv pulumi.StringMap,
) (*lambda.Function, error) {
	// Create role for this specific function
	role, err := CreateLambdaRole(ctx, name, tableArn, costTableArn, bucketArn,
		federationQueueArn, pushQueueArn, wsTableArns, kmsKeyArn)
	if err != nil {
		return nil, err
	}

	// Create the Lambda function with its own role
	return lambda.NewFunction(ctx, fmt.Sprintf("lesser-%s", name), &lambda.FunctionArgs{
		Runtime:       pulumi.String("provided.al2"),
		Handler:       pulumi.String("bootstrap"),
		Role:          role.Arn, // Use the specific role, not a shared one
		Timeout:       pulumi.Int(timeout),
		MemorySize:    pulumi.Int(3008),
		Architectures: pulumi.StringArray{pulumi.String("arm64")},
		Environment: &lambda.FunctionEnvironmentArgs{
			Variables: lambdaEnv,
		},
		Code: pulumi.NewFileArchive(fmt.Sprintf("../bin/%s.zip", handler)),
		Tags: pulumi.StringMap{
			"Name":     pulumi.Sprintf("Lesser %s", name),
			"Function": pulumi.String(name),
		},
	})
}
