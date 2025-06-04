package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/acm"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudfront"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

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
		bucketPolicyDocument := mediaBucket.ID().ApplyT(func(bucketName string) (string, error) {
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []interface{}{
					map[string]interface{}{
						"Effect": "Allow",
						"Principal": map[string]interface{}{
							"AWS": originAccessIdentity.IamArn,
						},
						"Action":   "s3:GetObject",
						"Resource": fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			return string(policyJSON), err
		}).(pulumi.StringOutput)

		_, err = s3.NewBucketPolicy(ctx, "lesser-media-policy", &s3.BucketPolicyArgs{
			Bucket: mediaBucket.ID(),
			Policy: bucketPolicyDocument,
		})
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
		dynamoPolicy := table.Arn.ApplyT(func(arn string) (string, error) {
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
						"Resource": []string{arn, fmt.Sprintf("%s/index/*", arn)},
					},
					map[string]interface{}{
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
		}).(pulumi.StringOutput)

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
		}).(pulumi.StringOutput)

		_, err = iam.NewRolePolicy(ctx, "lambda-s3", &iam.RolePolicyArgs{
			Role:   lambdaRole.Name,
			Policy: s3Policy,
		})
		if err != nil {
			return err
		}

		// Lambda environment variables
		lambdaEnv := pulumi.StringMap{
			"DYNAMODB_TABLE_NAME": table.Name,
			"S3_BUCKET_NAME":      mediaBucket.Bucket,
			"CDN_DOMAIN":          pulumi.Sprintf("media.%s", domain),
			"DOMAIN":              pulumi.String(domain),
			"JWT_SECRET":          jwtSecret,
			"AWS_REGION":          pulumi.String("us-east-1"),
		}

		// Helper function to create Lambda functions
		createLambda := func(name string, handler string, timeout int) (*lambda.Function, error) {
			return lambda.NewFunction(ctx, fmt.Sprintf("lesser-%s", name), &lambda.FunctionArgs{
				Runtime:    pulumi.String("go1.x"),
				Handler:    pulumi.String("main"),
				Role:       lambdaRole.Arn,
				Timeout:    pulumi.Int(timeout),
				MemorySize: pulumi.Int(512),
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
		mediaLambda, err := createLambda("media", "media", 120)
		if err != nil {
			return err
		}
		activityProcessorLambda, err := createLambda("activity-processor", "activity-processor", 300)
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

		// Create API Gateway
		api, err := apigatewayv2.NewApi(ctx, "lesser-api", &apigatewayv2.ApiArgs{
			ProtocolType: pulumi.String("HTTP"),
			CorsConfiguration: &apigatewayv2.ApiCorsConfigurationArgs{
				AllowOrigins:  pulumi.StringArray{pulumi.String("*")},
				AllowMethods:  pulumi.StringArray{pulumi.String("GET"), pulumi.String("POST"), pulumi.String("PUT"), pulumi.String("DELETE"), pulumi.String("OPTIONS"), pulumi.String("PATCH")},
				AllowHeaders:  pulumi.StringArray{pulumi.String("*")},
				ExposeHeaders: pulumi.StringArray{pulumi.String("*")},
				MaxAge:        pulumi.Int(300),
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

			_, err = lambda.NewPermission(ctx, fmt.Sprintf("%s-%s-permission", path, method), &lambda.PermissionArgs{
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
			{"/api/v1/{proxy+}", "ANY", apiLambda},
			{"/api/v2/{proxy+}", "ANY", apiLambda},
			{"/api/v1/media", "POST", mediaLambda},
		}

		for _, route := range routes {
			if err := createRoute(route.path, route.method, route.fn); err != nil {
				return err
			}
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
		_, err = apigatewayv2.NewApiMapping(ctx, "lesser-mapping", &apigatewayv2.ApiMappingArgs{
			ApiId:      api.ID(),
			DomainName: apiDomain.ID(),
			Stage:      stage.ID(),
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
					Name:                 apiDomain.DomainNameConfiguration.TargetDomainName(),
					ZoneId:               apiDomain.DomainNameConfiguration.HostedZoneId(),
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

		// Export important values
		ctx.Export("apiUrl", pulumi.Sprintf("https://%s", domain))
		ctx.Export("mediaUrl", pulumi.Sprintf("https://media.%s", domain))
		ctx.Export("tableName", table.Name)
		ctx.Export("bucketName", mediaBucket.Bucket)
		ctx.Export("distributionId", mediaDistribution.ID())
		ctx.Export("apiId", api.ID())

		return nil
	})
}
