//go:build example
// +build example

package main

import (
	"cdk/constructs"
	"cdk/stacks"
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

// Example of how to use S3 lifecycle policies in the Lesser infrastructure
func main() {
	app := awscdk.NewApp(nil)
	
	// Example stack showing S3 lifecycle policy usage
	stack := awscdk.NewStack(app, jsii.String("S3LifecycleExampleStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	environment := "production"

	// Example 1: Media bucket with comprehensive lifecycle policies
	mediaBucket := awss3.NewBucket(stack, jsii.String("MediaBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-media-%s", environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
		Versioning:        jsii.Bool(true),
	})

	// Apply media-specific lifecycle policies
	constructs.ApplyS3LifecyclePolicies(&constructs.S3LifecycleConfig{
		Environment: environment,
		Bucket:      mediaBucket,
		BucketType:  "media",
	})

	// Example 2: Create an export bucket with lifecycle policies
	exportBucket := constructs.CreateExportBucketWithLifecycle(stack, environment)
	
	// Example 3: Create a cache bucket with aggressive cleanup
	cacheBucket := constructs.CreateCacheBucketWithLifecycle(stack, environment)

	// Example 4: Log bucket with lifecycle policies
	logBucket := awss3.NewBucket(stack, jsii.String("LogBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-logs-%s", environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
	})

	constructs.ApplyS3LifecyclePolicies(&constructs.S3LifecycleConfig{
		Environment: environment,
		Bucket:      logBucket,
		BucketType:  "logs",
	})

	// Example 5: Backup bucket with tiered storage
	backupBucket := awss3.NewBucket(stack, jsii.String("BackupBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-backups-%s", environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
		Versioning:        jsii.Bool(true),
	})

	constructs.ApplyS3LifecyclePolicies(&constructs.S3LifecycleConfig{
		Environment: environment,
		Bucket:      backupBucket,
		BucketType:  "backups",
	})

	// Calculate and display estimated savings
	mediaMetrics := constructs.CalculateLifecycleSavings(mediaBucket, environment)
	fmt.Printf("Estimated monthly savings for media bucket: $%.2f\n", mediaMetrics.EstimatedMonthlySave)

	exportMetrics := constructs.CalculateLifecycleSavings(exportBucket, environment)
	fmt.Printf("Estimated monthly savings for export bucket: $%.2f\n", exportMetrics.EstimatedMonthlySave)

	// Create outputs
	awscdk.NewCfnOutput(stack, jsii.String("MediaBucketName"), &awscdk.CfnOutputProps{
		Value:       mediaBucket.BucketName(),
		Description: jsii.String("Media bucket with lifecycle policies"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ExportBucketName"), &awscdk.CfnOutputProps{
		Value:       exportBucket.BucketName(),
		Description: jsii.String("Export bucket with 30-day retention"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("CacheBucketName"), &awscdk.CfnOutputProps{
		Value:       cacheBucket.BucketName(),
		Description: jsii.String("Cache bucket with aggressive cleanup"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("EstimatedMonthlySavings"), &awscdk.CfnOutputProps{
		Value: jsii.String(fmt.Sprintf("$%.2f", 
			mediaMetrics.EstimatedMonthlySave + exportMetrics.EstimatedMonthlySave)),
		Description: jsii.String("Total estimated monthly savings from lifecycle policies"),
	})

	app.Synth(nil)
}

// Example showing how lifecycle policies save money:
//
// 1. Media Storage Optimization:
//    - Frequently accessed media stays in STANDARD storage
//    - After 30 days → STANDARD_IA (saves ~45%)
//    - After 90 days → GLACIER_IR (saves ~68%)
//    - After 180 days → GLACIER (saves ~95%)
//
// 2. Automatic Cleanup:
//    - Temp files deleted after 1 day
//    - Orphaned uploads deleted after 30 days
//    - Old thumbnails deleted after 1 year
//    - Incomplete multipart uploads aborted after 7 days
//
// 3. Cost Example (per TB/month):
//    - STANDARD: $23.00
//    - STANDARD_IA: $12.50
//    - GLACIER_IR: $4.00
//    - GLACIER: $1.00
//    - DEEP_ARCHIVE: $0.20
//
// 4. Environment-Specific Policies:
//    - Production: Conservative retention, focus on compliance
//    - Staging: Moderate retention, balance cost/availability
//    - Development: Aggressive cleanup, minimize costs
//
// 5. Compliance Considerations:
//    - GDPR: Deleted account media removed after 30 days
//    - Audit: Logs retained based on regulatory requirements
//    - Backups: Tiered retention (daily/weekly/monthly)