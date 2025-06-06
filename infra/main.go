package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/acm"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudfront"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/opensearch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// sanitizePermissionName converts a route path into a valid Lambda permission statement ID
func sanitizePermissionName(path string, method string) string {
	// Replace special characters with underscores
	sanitized := strings.ReplaceAll(path, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "{", "_")
	sanitized = strings.ReplaceAll(sanitized, "}", "_")
	sanitized = strings.ReplaceAll(sanitized, "+", "plus")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, "-", "_")

	// Remove leading/trailing underscores and collapse multiple underscores
	sanitized = strings.Trim(sanitized, "_")
	for strings.Contains(sanitized, "__") {
		sanitized = strings.ReplaceAll(sanitized, "__", "_")
	}

	return fmt.Sprintf("%s_%s_permission", sanitized, method)
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Get configuration
		cfg := config.New(ctx, "lesser")
		domain := cfg.Require("domain")
		hostedZoneId := cfg.Require("hostedZoneId")
		environment := cfg.Require("environment")
		jwtSecret := cfg.RequireSecret("jwtSecret")

		// Create DynamoDB table with all required GSIs
		table, err := dynamodb.NewTable(ctx, "lesser-table", &dynamodb.TableArgs{
			Name:        pulumi.Sprintf("lesser-%s", environment),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("PK"),
			RangeKey:    pulumi.String("SK"),
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{Name: pulumi.String("PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("SK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI1PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI1SK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI2PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI2SK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI3PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI3SK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI4PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI4SK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI5PK"), Type: pulumi.String("S")},
				&dynamodb.TableAttributeArgs{Name: pulumi.String("GSI5SK"), Type: pulumi.String("S")},
			},
			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI1"),
					HashKey:        pulumi.String("GSI1PK"),
					RangeKey:       pulumi.String("GSI1SK"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI2"),
					HashKey:        pulumi.String("GSI2PK"),
					RangeKey:       pulumi.String("GSI2SK"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI3"),
					HashKey:        pulumi.String("GSI3PK"),
					RangeKey:       pulumi.String("GSI3SK"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI4"),
					HashKey:        pulumi.String("GSI4PK"),
					RangeKey:       pulumi.String("GSI4SK"),
					ProjectionType: pulumi.String("ALL"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI5"),
					HashKey:        pulumi.String("GSI5PK"),
					RangeKey:       pulumi.String("GSI5SK"),
					ProjectionType: pulumi.String("ALL"),
				},
			},
			StreamEnabled:  pulumi.Bool(true),
			StreamViewType: pulumi.String("NEW_AND_OLD_IMAGES"),
			Ttl: &dynamodb.TableTtlArgs{
				AttributeName: pulumi.String("TTL"),
				Enabled:       pulumi.Bool(true),
			},
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser ActivityPub Storage"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Export table name for other components
		ctx.Export("tableName", table.Name)

		// Cost History Table for tracking operation costs
		costHistoryTable, err := dynamodb.NewTable(ctx, "cost-history-table", &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("lesser-cost-history-%s", environment)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("PK"),
			RangeKey:    pulumi.String("SK"),

			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("PK"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("SK"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("GSI1PK"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("GSI1SK"),
					Type: pulumi.String("S"),
				},
			},

			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("GSI1"),
					HashKey:        pulumi.String("GSI1PK"),
					RangeKey:       pulumi.String("GSI1SK"),
					ProjectionType: pulumi.String("ALL"),
				},
			},

			StreamEnabled:  pulumi.Bool(true),
			StreamViewType: pulumi.String("NEW_AND_OLD_IMAGES"),

			PointInTimeRecovery: &dynamodb.TablePointInTimeRecoveryArgs{
				Enabled: pulumi.Bool(true),
			},

			Tags: pulumi.StringMap{
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Export the cost history table name
		ctx.Export("costHistoryTableName", costHistoryTable.Name)

		// Create S3 bucket for media storage
		mediaBucket, err := s3.NewBucket(ctx, "lesser-media", &s3.BucketArgs{
			Bucket: pulumi.Sprintf("lesser-media-%s", environment),
			Acl:    pulumi.String("private"),
			CorsRules: s3.BucketCorsRuleArray{
				&s3.BucketCorsRuleArgs{
					AllowedHeaders: pulumi.StringArray{pulumi.String("*")},
					AllowedMethods: pulumi.StringArray{
						pulumi.String("GET"),
						pulumi.String("PUT"),
						pulumi.String("POST"),
						pulumi.String("HEAD"),
					},
					AllowedOrigins: pulumi.StringArray{pulumi.String("*")},
					ExposeHeaders:  pulumi.StringArray{pulumi.String("ETag")},
					MaxAgeSeconds:  pulumi.Int(3000),
				},
			},
			LifecycleRules: s3.BucketLifecycleRuleArray{
				&s3.BucketLifecycleRuleArgs{
					Enabled:                            pulumi.Bool(true),
					AbortIncompleteMultipartUploadDays: pulumi.Int(7),
				},
			},
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Media Storage"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Create ACM certificate for HTTPS
		certificate, err := acm.NewCertificate(ctx, "lesser-cert", &acm.CertificateArgs{
			DomainName: pulumi.String(domain),
			SubjectAlternativeNames: pulumi.StringArray{
				pulumi.Sprintf("*.%s", domain),
				pulumi.Sprintf("www.%s", domain),
			},
			ValidationMethod: pulumi.String("DNS"),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Certificate"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Create certificate validation records
		certificateValidation, err := acm.NewCertificateValidation(ctx, "lesser-cert-validation", &acm.CertificateValidationArgs{
			CertificateArn: certificate.Arn,
		})
		if err != nil {
			return err
		}

		// Create CloudFront Origin Access Identity
		originAccessIdentity, err := cloudfront.NewOriginAccessIdentity(ctx, "lesser-oai", &cloudfront.OriginAccessIdentityArgs{
			Comment: pulumi.String("Lesser Media OAI"),
		})
		if err != nil {
			return err
		}

		// Update bucket policy to allow CloudFront access
		bucketPolicyDocument := pulumi.All(mediaBucket.ID(), originAccessIdentity.IamArn).ApplyT(func(args []interface{}) (string, error) {
			bucketName := string(args[0].(pulumi.ID))
			oaiArn := args[1].(string)
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Effect": "Allow",
						"Principal": map[string]interface{}{
							"AWS": oaiArn,
						},
						"Action":   "s3:GetObject",
						"Resource": fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = s3.NewBucketPolicy(ctx, "lesser-media-policy", &s3.BucketPolicyArgs{
			Bucket: mediaBucket.ID(),
			Policy: bucketPolicyDocument,
		})
		if err != nil {
			return err
		}

		// Create OpenSearch Serverless encryption security policy
		collectionName := pulumi.Sprintf("lesser-search-%s", environment)
		encryptionPolicy, err := opensearch.NewServerlessSecurityPolicy(ctx, "lesser-search-encryption", &opensearch.ServerlessSecurityPolicyArgs{
			Name: pulumi.String("lesser-search-encryption"),
			Type: pulumi.String("encryption"),
			Policy: collectionName.ApplyT(func(name string) (string, error) {
				policy := map[string]interface{}{
					"Rules": []interface{}{
						map[string]interface{}{
							"Resource": []string{
								fmt.Sprintf("collection/%s", name),
							},
							"ResourceType": "collection",
						},
					},
					"AWSOwnedKey": true,
				}
				policyJSON, err := json.Marshal(policy)
				return string(policyJSON), err
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// Create OpenSearch Serverless network security policy
		networkPolicy, err := opensearch.NewServerlessSecurityPolicy(ctx, "lesser-search-network", &opensearch.ServerlessSecurityPolicyArgs{
			Name: pulumi.String("lesser-search-network"),
			Type: pulumi.String("network"),
			Policy: collectionName.ApplyT(func(name string) (string, error) {
				policy := []interface{}{
					map[string]interface{}{
						"Rules": []interface{}{
							map[string]interface{}{
								"Resource": []string{
									fmt.Sprintf("collection/%s", name),
								},
								"ResourceType": "collection",
							},
						},
						"AllowFromPublic": true,
					},
				}
				policyJSON, err := json.Marshal(policy)
				return string(policyJSON), err
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// Create OpenSearch Serverless collection
		searchCollection, err := opensearch.NewServerlessCollection(ctx, "lesser-search", &opensearch.ServerlessCollectionArgs{
			Name:        collectionName,
			Type:        pulumi.String("SEARCH"),
			Description: pulumi.String("Lesser ActivityPub search collection for actors"),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Search Collection"),
				"Environment": pulumi.String(environment),
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			encryptionPolicy,
			networkPolicy,
		}))
		if err != nil {
			return err
		}

		// Create CloudFront distribution for media
		mediaDistribution, err := cloudfront.NewDistribution(ctx, "lesser-media-cdn", &cloudfront.DistributionArgs{
			Enabled:           pulumi.Bool(true),
			IsIpv6Enabled:     pulumi.Bool(true),
			Comment:           pulumi.String("Lesser Media CDN"),
			DefaultRootObject: pulumi.String("index.html"),
			Aliases:           pulumi.StringArray{pulumi.Sprintf("media.%s", domain)},
			ViewerCertificate: &cloudfront.DistributionViewerCertificateArgs{
				AcmCertificateArn:      certificateValidation.CertificateArn,
				SslSupportMethod:       pulumi.String("sni-only"),
				MinimumProtocolVersion: pulumi.String("TLSv1.2_2021"),
			},
			Origins: cloudfront.DistributionOriginArray{
				&cloudfront.DistributionOriginArgs{
					DomainName: mediaBucket.BucketRegionalDomainName,
					OriginId:   pulumi.String("S3-Media"),
					S3OriginConfig: &cloudfront.DistributionOriginS3OriginConfigArgs{
						OriginAccessIdentity: originAccessIdentity.CloudfrontAccessIdentityPath,
					},
				},
			},
			DefaultCacheBehavior: &cloudfront.DistributionDefaultCacheBehaviorArgs{
				TargetOriginId:       pulumi.String("S3-Media"),
				ViewerProtocolPolicy: pulumi.String("redirect-to-https"),
				AllowedMethods:       pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD"), pulumi.String("OPTIONS")},
				CachedMethods:        pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")},
				ForwardedValues: &cloudfront.DistributionDefaultCacheBehaviorForwardedValuesArgs{
					QueryString: pulumi.Bool(false),
					Cookies: &cloudfront.DistributionDefaultCacheBehaviorForwardedValuesCookiesArgs{
						Forward: pulumi.String("none"),
					},
				},
				MinTtl:     pulumi.Int(0),
				DefaultTtl: pulumi.Int(86400),
				MaxTtl:     pulumi.Int(31536000),
				Compress:   pulumi.Bool(true),
			},
			Restrictions: &cloudfront.DistributionRestrictionsArgs{
				GeoRestriction: &cloudfront.DistributionRestrictionsGeoRestrictionArgs{
					RestrictionType: pulumi.String("none"),
				},
			},
			PriceClass: pulumi.String("PriceClass_100"),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Media CDN"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Create IAM role for Lambda functions
		lambdaRole, err := iam.NewRole(ctx, "lesser-lambda-role", &iam.RoleArgs{
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
		})
		if err != nil {
			return err
		}

		// Attach basic execution policy
		_, err = iam.NewRolePolicyAttachment(ctx, "lambda-basic", &iam.RolePolicyAttachmentArgs{
			Role:      lambdaRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return err
		}

		// Create DynamoDB policy for Lambda
		dynamoPolicy := pulumi.All(table.Arn, costHistoryTable.Arn).ApplyT(func(args []interface{}) (string, error) {
			tableArn := args[0].(string)
			costTableArn := args[1].(string)
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
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
						"Resource": []string{
							tableArn,
							fmt.Sprintf("%s/index/*", tableArn),
							costTableArn,
							fmt.Sprintf("%s/index/*", costTableArn),
						},
					},
					map[string]interface{}{
						"Effect": "Allow",
						"Action": []string{
							"dynamodb:DescribeStream",
							"dynamodb:GetRecords",
							"dynamodb:GetShardIterator",
							"dynamodb:ListStreams",
						},
						"Resource": []string{
							fmt.Sprintf("%s/stream/*", tableArn),
							fmt.Sprintf("%s/stream/*", costTableArn),
						},
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, "lambda-dynamodb", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: dynamoPolicy,
		})
		if err != nil {
			return err
		}

		// Create S3 policy for Lambda
		s3Policy := mediaBucket.Arn.ApplyT(func(arn string) (string, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Effect": "Allow",
						"Action": []string{
							"s3:GetObject",
							"s3:PutObject",
							"s3:DeleteObject",
							"s3:PutObjectAcl",
						},
						"Resource": fmt.Sprintf("%s/*", arn),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, "lambda-s3", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: s3Policy,
		})
		if err != nil {
			return err
		}

		// Create OpenSearch policy for Lambda
		opensearchPolicy := searchCollection.Arn.ApplyT(func(arn string) (string, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Effect": "Allow",
						"Action": []string{
							"aoss:APIAccessAll",
						},
						"Resource": arn,
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, "lambda-opensearch", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: opensearchPolicy,
		})
		if err != nil {
			return err
		}

		// Create Bedrock and Comprehend policy for Lambda
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

		_, err = iam.NewRolePolicy(ctx, "lambda-ai", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: aiPolicy,
		})
		if err != nil {
			return err
		}

		// Create OpenSearch data access policy now that Lambda role exists
		_, err = opensearch.NewServerlessAccessPolicy(ctx, "lesser-search-data-access", &opensearch.ServerlessAccessPolicyArgs{
			Name: pulumi.String("lesser-search-data-access"),
			Type: pulumi.String("data"),
			Policy: pulumi.All(collectionName, lambdaRole.Arn).ApplyT(func(args []interface{}) (string, error) {
				collection := args[0].(string)
				roleArn := args[1].(string)
				policy := []interface{}{
					map[string]interface{}{
						"Rules": []interface{}{
							map[string]interface{}{
								"Resource": []string{
									fmt.Sprintf("collection/%s", collection),
								},
								"Permission": []string{
									"aoss:CreateCollectionItems",
									"aoss:UpdateCollectionItems",
									"aoss:DeleteCollectionItems",
									"aoss:DescribeCollectionItems",
								},
								"ResourceType": "collection",
							},
							map[string]interface{}{
								"Resource": []string{
									fmt.Sprintf("index/%s/*", collection),
								},
								"Permission": []string{
									"aoss:CreateIndex",
									"aoss:UpdateIndex",
									"aoss:DeleteIndex",
									"aoss:DescribeIndex",
									"aoss:ReadDocument",
									"aoss:WriteDocument",
								},
								"ResourceType": "index",
							},
						},
						"Principal": []string{roleArn},
					},
				}
				policyJSON, err := json.Marshal(policy)
				return string(policyJSON), err
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		// Lambda environment variables
		lambdaEnv := pulumi.StringMap{
			"DYNAMO_TABLE_NAME":       table.Name,
			"S3_BUCKET_NAME":          mediaBucket.Bucket,
			"CDN_DOMAIN":              pulumi.Sprintf("media.%s", domain),
			"DOMAIN":                  pulumi.String(domain),
			"JWT_SECRET":              jwtSecret,
			"OPENSEARCH_ENDPOINT":     searchCollection.CollectionEndpoint,
			"COST_HISTORY_TABLE_NAME": costHistoryTable.Name,
			// Instance configuration
			"INSTANCE_TITLE":       pulumi.String("Lesser Instance"),
			"INSTANCE_SHORT_DESC":  pulumi.String("A personal ActivityPub server"),
			"INSTANCE_DESCRIPTION": pulumi.String("A lightweight, serverless ActivityPub implementation"),
			"INSTANCE_ADMIN_EMAIL": pulumi.Sprintf("admin@%s", domain),
			"REGISTRATIONS_OPEN":   pulumi.String("false"),
			"APPROVAL_REQUIRED":    pulumi.String("true"),
			"INVITES_ENABLED":      pulumi.String("false"),
			"FEDERATION_ENABLED":   pulumi.String("true"),
		}

		// Helper function to create Lambda functions
		createLambda := func(name string, handler string, timeout int) (*lambda.Function, error) {
			return lambda.NewFunction(ctx, fmt.Sprintf("lesser-%s", name), &lambda.FunctionArgs{
				Runtime:       pulumi.String("provided.al2"),
				Handler:       pulumi.String("bootstrap"),
				Role:          lambdaRole.Arn,
				Timeout:       pulumi.Int(timeout),
				MemorySize:    pulumi.Int(512),
				Architectures: pulumi.StringArray{pulumi.String("arm64")},
				Environment: &lambda.FunctionEnvironmentArgs{
					Variables: lambdaEnv,
				},
				Code: pulumi.NewFileArchive(fmt.Sprintf("../bin/%s.zip", handler)),
				Tags: pulumi.StringMap{
					"Name":        pulumi.Sprintf("Lesser %s", name),
					"Environment": pulumi.String(environment),
				},
			})
		}

		// Create Lambda functions
		apiLambda, err := createLambda("api", "api", 60)
		if err != nil {
			return err
		}
		actorLambda, err := createLambda("actor", "actor", 30)
		if err != nil {
			return err
		}
		inboxLambda, err := createLambda("inbox", "inbox", 30)
		if err != nil {
			return err
		}
		outboxLambda, err := createLambda("outbox", "outbox", 30)
		if err != nil {
			return err
		}
		collectionsLambda, err := createLambda("collections", "collections", 30)
		if err != nil {
			return err
		}
		objectsLambda, err := createLambda("objects", "objects", 30)
		if err != nil {
			return err
		}
		webfingerLambda, err := createLambda("webfinger", "webfinger", 30)
		if err != nil {
			return err
		}
		authLambda, err := createLambda("auth", "auth", 30)
		if err != nil {
			return err
		}
		activityProcessorLambda, err := createLambda("activity-processor", "activity-processor", 300)
		if err != nil {
			return err
		}
		searchIndexerLambda, err := createLambda("search-indexer", "search-indexer", 60)
		if err != nil {
			return err
		}
		costAggregatorLambda, err := createLambda("cost-aggregator", "cost-aggregator", 60)
		if err != nil {
			return err
		}

		// WebSocket Lambda functions will be created after tables are set up
		var streamingLambda *lambda.Function
		var streamRouterLambda *lambda.Function

		// Create DynamoDB tables for WebSocket connections
		connectionsTable, err := dynamodb.NewTable(ctx, "lesser-streaming-connections", &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("lesser-streaming-connections-%s", environment)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("PK"),
			RangeKey:    pulumi.String("SK"),

			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("PK"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("SK"),
					Type: pulumi.String("S"),
				},
			},

			Ttl: &dynamodb.TableTtlArgs{
				AttributeName: pulumi.String("TTL"),
				Enabled:       pulumi.Bool(true),
			},

			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Streaming Connections"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		subscriptionsTable, err := dynamodb.NewTable(ctx, "lesser-streaming-subscriptions", &dynamodb.TableArgs{
			Name:        pulumi.String(fmt.Sprintf("lesser-streaming-subscriptions-%s", environment)),
			BillingMode: pulumi.String("PAY_PER_REQUEST"),
			HashKey:     pulumi.String("PK"),
			RangeKey:    pulumi.String("SK"),

			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("PK"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("SK"),
					Type: pulumi.String("S"),
				},
			},

			Ttl: &dynamodb.TableTtlArgs{
				AttributeName: pulumi.String("TTL"),
				Enabled:       pulumi.Bool(true),
			},

			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Streaming Subscriptions"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Add DynamoDB policy for WebSocket tables
		wsTablePolicy := pulumi.All(connectionsTable.Arn, subscriptionsTable.Arn).ApplyT(func(args []interface{}) (string, error) {
			connectionsArn := args[0].(string)
			subscriptionsArn := args[1].(string)
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
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
						"Resource": []string{
							connectionsArn,
							fmt.Sprintf("%s/index/*", connectionsArn),
							subscriptionsArn,
							fmt.Sprintf("%s/index/*", subscriptionsArn),
						},
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, "lambda-dynamodb-websocket", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: wsTablePolicy,
		})
		if err != nil {
			return err
		}

		// Create WebSocket API
		wsApi, err := apigatewayv2.NewApi(ctx, "lesser-websocket-api", &apigatewayv2.ApiArgs{
			ProtocolType:             pulumi.String("WEBSOCKET"),
			RouteSelectionExpression: pulumi.String("$request.body.type"),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser WebSocket API"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Update Lambda environment for WebSocket functions
		wsLambdaEnv := pulumi.StringMap{
			"DYNAMO_TABLE_NAME":   table.Name,
			"CONNECTIONS_TABLE":   connectionsTable.Name,
			"SUBSCRIPTIONS_TABLE": subscriptionsTable.Name,
			"DOMAIN":              pulumi.String(domain),
			"WEBSOCKET_ENDPOINT":  pulumi.Sprintf("https://%s.execute-api.us-east-1.amazonaws.com/%s", wsApi.ID(), environment),
			"JWT_SECRET":          jwtSecret,
		}

		// Create WebSocket Lambda with updated environment
		createWebSocketLambda := func(name string, handler string, timeout int) (*lambda.Function, error) {
			return lambda.NewFunction(ctx, fmt.Sprintf("lesser-%s", name), &lambda.FunctionArgs{
				Runtime:       pulumi.String("provided.al2"),
				Handler:       pulumi.String("bootstrap"),
				Role:          lambdaRole.Arn,
				Timeout:       pulumi.Int(timeout),
				MemorySize:    pulumi.Int(512),
				Architectures: pulumi.StringArray{pulumi.String("arm64")},
				Environment: &lambda.FunctionEnvironmentArgs{
					Variables: wsLambdaEnv,
				},
				Code: pulumi.NewFileArchive(fmt.Sprintf("../bin/%s.zip", handler)),
				Tags: pulumi.StringMap{
					"Name":        pulumi.Sprintf("Lesser %s", name),
					"Environment": pulumi.String(environment),
				},
			})
		}

		// Recreate streaming Lambda with WebSocket environment
		streamingLambda, err = createWebSocketLambda("streaming-ws", "streaming", 30)
		if err != nil {
			return err
		}

		// Recreate stream router Lambda with WebSocket environment
		streamRouterLambda, err = createWebSocketLambda("stream-router-ws", "stream-router", 30)
		if err != nil {
			return err
		}

		// Create WebSocket integrations
		connectIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-connect-integration", &apigatewayv2.IntegrationArgs{
			ApiId:                wsApi.ID(), // Use WebSocket API, not HTTP API
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       streamingLambda.InvokeArn,
			PayloadFormatVersion: pulumi.String("1.0"),
		}, pulumi.DependsOn([]pulumi.Resource{streamingLambda}))
		if err != nil {
			return err
		}

		disconnectIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-disconnect-integration", &apigatewayv2.IntegrationArgs{
			ApiId:                wsApi.ID(), // Use WebSocket API, not HTTP API
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       streamingLambda.InvokeArn,
			PayloadFormatVersion: pulumi.String("1.0"),
		}, pulumi.DependsOn([]pulumi.Resource{streamingLambda}))
		if err != nil {
			return err
		}

		defaultIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-default-integration", &apigatewayv2.IntegrationArgs{
			ApiId:                wsApi.ID(), // Use WebSocket API, not HTTP API
			IntegrationType:      pulumi.String("AWS_PROXY"),
			IntegrationUri:       streamingLambda.InvokeArn,
			PayloadFormatVersion: pulumi.String("1.0"),
		}, pulumi.DependsOn([]pulumi.Resource{streamingLambda}))
		if err != nil {
			return err
		}

		// Create WebSocket routes
		_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-connect-route", &apigatewayv2.RouteArgs{
			ApiId:    wsApi.ID(),
			RouteKey: pulumi.String("$connect"),
			Target:   pulumi.Sprintf("integrations/%s", connectIntegration.ID()),
		}, pulumi.DependsOn([]pulumi.Resource{connectIntegration}))
		if err != nil {
			return err
		}

		_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-disconnect-route", &apigatewayv2.RouteArgs{
			ApiId:    wsApi.ID(),
			RouteKey: pulumi.String("$disconnect"),
			Target:   pulumi.Sprintf("integrations/%s", disconnectIntegration.ID()),
		}, pulumi.DependsOn([]pulumi.Resource{disconnectIntegration}))
		if err != nil {
			return err
		}

		_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-default-route", &apigatewayv2.RouteArgs{
			ApiId:    wsApi.ID(),
			RouteKey: pulumi.String("$default"),
			Target:   pulumi.Sprintf("integrations/%s", defaultIntegration.ID()),
		}, pulumi.DependsOn([]pulumi.Resource{defaultIntegration}))
		if err != nil {
			return err
		}

		// Create WebSocket stage
		wsStage, err := apigatewayv2.NewStage(ctx, "lesser-websocket-stage", &apigatewayv2.StageArgs{
			ApiId:      wsApi.ID(),
			Name:       pulumi.String(environment),
			AutoDeploy: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser WebSocket Stage"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Grant Lambda permission for WebSocket
		_, err = lambda.NewPermission(ctx, "lesser-streaming-permission", &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  streamingLambda.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: pulumi.Sprintf("%s/*/*", wsApi.ExecutionArn),
		})
		if err != nil {
			return err
		}

		// Add API Gateway Management API permissions for stream router
		wsManagementPolicy := wsApi.ExecutionArn.ApplyT(func(executionArn string) (string, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Effect": "Allow",
						"Action": []string{
							"execute-api:ManageConnections",
						},
						"Resource": fmt.Sprintf("%s/*/*/*", executionArn),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		})

		_, err = iam.NewRolePolicy(ctx, "lambda-ws-management", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: wsManagementPolicy,
		})
		if err != nil {
			return err
		}

		// Add DynamoDB Streams trigger
		_, err = lambda.NewEventSourceMapping(ctx, "activity-stream", &lambda.EventSourceMappingArgs{
			EventSourceArn:                 table.StreamArn,
			FunctionName:                   activityProcessorLambda.Name,
			StartingPosition:               pulumi.String("LATEST"),
			MaximumBatchingWindowInSeconds: pulumi.Int(5),
			ParallelizationFactor:          pulumi.Int(10),
			MaximumRetryAttempts:           pulumi.Int(3),
		})
		if err != nil {
			return err
		}

		// Add DynamoDB Streams trigger for search indexer
		_, err = lambda.NewEventSourceMapping(ctx, "search-indexer-stream", &lambda.EventSourceMappingArgs{
			EventSourceArn:        table.StreamArn,
			FunctionName:          searchIndexerLambda.Name,
			StartingPosition:      pulumi.String("LATEST"),
			ParallelizationFactor: pulumi.Int(5),
			MaximumRetryAttempts:  pulumi.Int(3),
			FilterCriteria: &lambda.EventSourceMappingFilterCriteriaArgs{
				Filters: lambda.EventSourceMappingFilterCriteriaFilterArray{
					&lambda.EventSourceMappingFilterCriteriaFilterArgs{
						Pattern: pulumi.String(`{"eventName": ["INSERT", "MODIFY", "REMOVE"], "dynamodb": {"Keys": {"PK": {"S": [{"prefix": "ACTOR#"}]}}}}`),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		// Add DynamoDB Streams trigger for cost aggregator
		_, err = lambda.NewEventSourceMapping(ctx, "cost-aggregator-stream", &lambda.EventSourceMappingArgs{
			EventSourceArn:        costHistoryTable.StreamArn,
			FunctionName:          costAggregatorLambda.Name,
			StartingPosition:      pulumi.String("LATEST"),
			ParallelizationFactor: pulumi.Int(5),
			MaximumRetryAttempts:  pulumi.Int(3),
			FilterCriteria: &lambda.EventSourceMappingFilterCriteriaArgs{
				Filters: lambda.EventSourceMappingFilterCriteriaFilterArray{
					&lambda.EventSourceMappingFilterCriteriaFilterArgs{
						Pattern: pulumi.String(`{"eventName": ["INSERT"], "dynamodb": {"Keys": {"PK": {"S": [{"prefix": "COST#"}]}}}}`),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		// Add DynamoDB Streams trigger for stream router (WebSocket broadcasting)
		_, err = lambda.NewEventSourceMapping(ctx, "stream-router-trigger", &lambda.EventSourceMappingArgs{
			EventSourceArn:        table.StreamArn,
			FunctionName:          streamRouterLambda.Name,
			StartingPosition:      pulumi.String("LATEST"),
			ParallelizationFactor: pulumi.Int(5),
			MaximumRetryAttempts:  pulumi.Int(3),
			FilterCriteria: &lambda.EventSourceMappingFilterCriteriaArgs{
				Filters: lambda.EventSourceMappingFilterCriteriaFilterArray{
					&lambda.EventSourceMappingFilterCriteriaFilterArgs{
						Pattern: pulumi.String(`{"eventName": ["INSERT", "MODIFY"], "dynamodb": {"Keys": {"PK": {"S": [{"prefix": "STATUS#"}, {"prefix": "NOTIFICATION#"}, {"prefix": "ACTOR#"}]}}}}`),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		// Create API Gateway
		api, err := apigatewayv2.NewApi(ctx, "lesser-api", &apigatewayv2.ApiArgs{
			ProtocolType: pulumi.String("HTTP"),
			CorsConfiguration: &apigatewayv2.ApiCorsConfigurationArgs{
				AllowOrigins: pulumi.StringArray{pulumi.String("*")},
				AllowMethods: pulumi.StringArray{pulumi.String("GET"), pulumi.String("POST"), pulumi.String("PUT"), pulumi.String("DELETE"), pulumi.String("OPTIONS"), pulumi.String("PATCH"), pulumi.String("HEAD")},
				AllowHeaders: pulumi.StringArray{
					pulumi.String("*"),
					pulumi.String("Accept"),
					pulumi.String("Accept-Encoding"),
					pulumi.String("Accept-Language"),
					pulumi.String("Authorization"),
					pulumi.String("Content-Type"),
					pulumi.String("Date"),
					pulumi.String("Digest"),
					pulumi.String("Host"),
					pulumi.String("Signature"),
					pulumi.String("User-Agent"),
					pulumi.String("X-Requested-With"),
					pulumi.String("X-Forwarded-For"),
					pulumi.String("X-Forwarded-Proto"),
				},
				ExposeHeaders: pulumi.StringArray{
					pulumi.String("*"),
					pulumi.String("Date"),
					pulumi.String("ETag"),
					pulumi.String("Link"),
					pulumi.String("Location"),
					pulumi.String("X-Content-Type-Options"),
					pulumi.String("X-Frame-Options"),
					pulumi.String("X-RateLimit-Limit"),
					pulumi.String("X-RateLimit-Remaining"),
					pulumi.String("X-RateLimit-Reset"),
				},
				MaxAge: pulumi.Int(300),
			},
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser API"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Helper function to create routes
		createRoute := func(path string, method string, fn *lambda.Function) error {
			integration, err := apigatewayv2.NewIntegration(ctx, fmt.Sprintf("%s-%s-integration", path, method), &apigatewayv2.IntegrationArgs{
				ApiId:                api.ID(),
				IntegrationType:      pulumi.String("AWS_PROXY"),
				IntegrationUri:       fn.Arn,
				PayloadFormatVersion: pulumi.String("2.0"),
			})
			if err != nil {
				return err
			}

			_, err = apigatewayv2.NewRoute(ctx, fmt.Sprintf("%s-%s-route", path, method), &apigatewayv2.RouteArgs{
				ApiId:    api.ID(),
				RouteKey: pulumi.Sprintf("%s %s", method, path),
				Target:   pulumi.Sprintf("integrations/%s", integration.ID()),
			})
			if err != nil {
				return err
			}

			_, err = lambda.NewPermission(ctx, sanitizePermissionName(path, method), &lambda.PermissionArgs{
				Action:    pulumi.String("lambda:InvokeFunction"),
				Function:  fn.Name,
				Principal: pulumi.String("apigateway.amazonaws.com"),
				SourceArn: pulumi.Sprintf("%s/*/*", api.ExecutionArn),
			})
			return err
		}

		// Create all routes
		routes := []struct {
			path   string
			method string
			fn     *lambda.Function
		}{
			{"/.well-known/webfinger", "GET", webfingerLambda},
			{"/.well-known/nodeinfo", "GET", webfingerLambda},
			{"/nodeinfo/2.0", "GET", webfingerLambda},
			{"/nodeinfo/2.1", "GET", webfingerLambda},
			{"/users/{username}", "GET", actorLambda},
			{"/users/{username}/inbox", "GET", inboxLambda},
			{"/users/{username}/inbox", "POST", inboxLambda},
			{"/users/{username}/outbox", "GET", outboxLambda},
			{"/users/{username}/outbox", "POST", outboxLambda},
			{"/users/{username}/followers", "GET", collectionsLambda},
			{"/users/{username}/following", "GET", collectionsLambda},
			{"/objects/{id}", "GET", objectsLambda},
			{"/oauth/authorize", "GET", authLambda},
			{"/oauth/authorize", "POST", authLambda},
			{"/oauth/token", "POST", authLambda},
			{"/.well-known/oauth-authorization-server", "GET", authLambda},
			// Don't use ANY for the catch-all to avoid conflicts with OPTIONS
			{"/{proxy+}", "GET", apiLambda},
			{"/{proxy+}", "POST", apiLambda},
			{"/{proxy+}", "PUT", apiLambda},
			{"/{proxy+}", "DELETE", apiLambda},
			{"/{proxy+}", "PATCH", apiLambda},
			{"/{proxy+}", "HEAD", apiLambda},
			// OPTIONS will be handled separately after the loop
		}

		for _, route := range routes {
			if err := createRoute(route.path, route.method, route.fn); err != nil {
				return err
			}
		}

		// Add OPTIONS handler for all routes to properly handle CORS preflight
		if err := createRoute("/{proxy+}", "OPTIONS", apiLambda); err != nil {
			return err
		}

		// Create API Gateway stage
		stage, err := apigatewayv2.NewStage(ctx, "lesser-stage", &apigatewayv2.StageArgs{
			ApiId:      api.ID(),
			Name:       pulumi.String(environment),
			AutoDeploy: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser API Stage"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Create custom domain
		apiDomain, err := apigatewayv2.NewDomainName(ctx, "lesser-domain", &apigatewayv2.DomainNameArgs{
			DomainName: pulumi.String(domain),
			DomainNameConfiguration: &apigatewayv2.DomainNameDomainNameConfigurationArgs{
				CertificateArn: certificateValidation.CertificateArn,
				EndpointType:   pulumi.String("REGIONAL"),
				SecurityPolicy: pulumi.String("TLS_1_2"),
			},
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser API Domain"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Map API to domain
		// Note: Mastodon API has both v1 and v2 endpoints
		// v2 is used for: /instance, /search, /suggestions, /media
		// v1 is used for everything else (timelines, accounts, statuses, etc.)
		_, err = apigatewayv2.NewApiMapping(ctx, "lesser-mapping", &apigatewayv2.ApiMappingArgs{
			ApiId:         api.ID(),
			DomainName:    apiDomain.ID(),
			Stage:         stage.ID(),
			ApiMappingKey: pulumi.String("api/v2"),
		})
		if err != nil {
			return err
		}

		// v1 mapping for endpoints that don't have v2 versions
		_, err = apigatewayv2.NewApiMapping(ctx, "lesser-v1-mapping", &apigatewayv2.ApiMappingArgs{
			ApiId:         api.ID(),
			DomainName:    apiDomain.ID(),
			Stage:         stage.ID(),
			ApiMappingKey: pulumi.String("api/v1"),
		})
		if err != nil {
			return err
		}

		// Map root-level routes for OAuth and ActivityPub
		_, err = apigatewayv2.NewApiMapping(ctx, "lesser-root-mapping", &apigatewayv2.ApiMappingArgs{
			ApiId:      api.ID(),
			DomainName: apiDomain.ID(),
			Stage:      stage.ID(),
			// Empty ApiMappingKey means routes are accessible at root level
			ApiMappingKey: pulumi.String(""),
		})
		if err != nil {
			return err
		}

		// Create WebSocket domain
		wsDomain, err := apigatewayv2.NewDomainName(ctx, "lesser-ws-domain", &apigatewayv2.DomainNameArgs{
			DomainName: pulumi.Sprintf("ws.%s", domain),
			DomainNameConfiguration: &apigatewayv2.DomainNameDomainNameConfigurationArgs{
				CertificateArn: certificateValidation.CertificateArn,
				EndpointType:   pulumi.String("REGIONAL"),
				SecurityPolicy: pulumi.String("TLS_1_2"),
			},
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser WebSocket Domain"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Map WebSocket API to root of subdomain
		_, err = apigatewayv2.NewApiMapping(ctx, "lesser-websocket-mapping", &apigatewayv2.ApiMappingArgs{
			ApiId:         wsApi.ID(),
			DomainName:    wsDomain.ID(),
			Stage:         wsStage.ID(),
			ApiMappingKey: pulumi.String(""), // Empty string maps to root
		})
		if err != nil {
			return err
		}

		// Create Route53 records
		_, err = route53.NewRecord(ctx, "api-record", &route53.RecordArgs{
			ZoneId: pulumi.String(hostedZoneId),
			Name:   pulumi.String(domain),
			Type:   pulumi.String("A"),
			Aliases: route53.RecordAliasArray{
				&route53.RecordAliasArgs{
					Name:                 apiDomain.DomainNameConfiguration.TargetDomainName().Elem(),
					ZoneId:               apiDomain.DomainNameConfiguration.HostedZoneId().Elem(),
					EvaluateTargetHealth: pulumi.Bool(true),
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = route53.NewRecord(ctx, "media-record", &route53.RecordArgs{
			ZoneId: pulumi.String(hostedZoneId),
			Name:   pulumi.Sprintf("media.%s", domain),
			Type:   pulumi.String("A"),
			Aliases: route53.RecordAliasArray{
				&route53.RecordAliasArgs{
					Name:                 mediaDistribution.DomainName,
					ZoneId:               mediaDistribution.HostedZoneId,
					EvaluateTargetHealth: pulumi.Bool(false),
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = route53.NewRecord(ctx, "ws-record", &route53.RecordArgs{
			ZoneId: pulumi.String(hostedZoneId),
			Name:   pulumi.Sprintf("ws.%s", domain),
			Type:   pulumi.String("A"),
			Aliases: route53.RecordAliasArray{
				&route53.RecordAliasArgs{
					Name:                 wsDomain.DomainNameConfiguration.TargetDomainName().Elem(),
					ZoneId:               wsDomain.DomainNameConfiguration.HostedZoneId().Elem(),
					EvaluateTargetHealth: pulumi.Bool(true),
				},
			},
		})
		if err != nil {
			return err
		}

		// Create CloudWatch log group
		_, err = cloudwatch.NewLogGroup(ctx, "api-logs", &cloudwatch.LogGroupArgs{
			Name:            pulumi.String("/aws/apigateway/lesser"),
			RetentionInDays: pulumi.Int(7),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser API Logs"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Create EventBridge rule for periodic cost aggregation
		costAggregationRule, err := cloudwatch.NewEventRule(ctx, "cost-aggregation-rule", &cloudwatch.EventRuleArgs{
			Description:        pulumi.String("Trigger cost aggregation every hour"),
			ScheduleExpression: pulumi.String("rate(1 hour)"),
			State:              pulumi.String("ENABLED"),
			Tags: pulumi.StringMap{
				"Name":        pulumi.String("Lesser Cost Aggregation"),
				"Environment": pulumi.String(environment),
			},
		})
		if err != nil {
			return err
		}

		// Add Lambda permission for EventBridge
		_, err = lambda.NewPermission(ctx, "cost-aggregation-eventbridge", &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  costAggregatorLambda.Name,
			Principal: pulumi.String("events.amazonaws.com"),
			SourceArn: costAggregationRule.Arn,
		})
		if err != nil {
			return err
		}

		// Create EventBridge target
		_, err = cloudwatch.NewEventTarget(ctx, "cost-aggregation-target", &cloudwatch.EventTargetArgs{
			Rule: costAggregationRule.Name,
			Arn:  costAggregatorLambda.Arn,
			RetryPolicy: &cloudwatch.EventTargetRetryPolicyArgs{
				MaximumRetryAttempts:     pulumi.Int(2),
				MaximumEventAgeInSeconds: pulumi.Int(3600),
			},
		})
		if err != nil {
			return err
		}

		// Export important values
		ctx.Export("apiUrl", pulumi.Sprintf("https://%s", domain))
		ctx.Export("mediaUrl", pulumi.Sprintf("https://media.%s", domain))
		ctx.Export("bucketName", mediaBucket.Bucket)
		ctx.Export("distributionId", mediaDistribution.ID())
		ctx.Export("apiId", api.ID())
		ctx.Export("opensearchEndpoint", searchCollection.CollectionEndpoint)
		ctx.Export("websocketUrl", pulumi.Sprintf("wss://ws.%s", domain))

		return nil
	})
}
