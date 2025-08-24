package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

// S3LifecycleConfig defines lifecycle policies for S3 buckets
type S3LifecycleConfig struct {
	Environment string
	Bucket      awss3.Bucket
	BucketType  string // "media", "logs", "backups", etc.
}

// ApplyS3LifecyclePolicies applies comprehensive lifecycle policies to S3 buckets
func ApplyS3LifecyclePolicies(config *S3LifecycleConfig) {
	switch config.BucketType {
	case "media":
		applyMediaBucketPolicies(config)
	case "logs":
		applyLogBucketPolicies(config)
	case "backups":
		applyBackupBucketPolicies(config)
	case "cloudfront-logs":
		applyCloudFrontLogPolicies(config)
	default:
		applyDefaultPolicies(config)
	}
}

// applyMediaBucketPolicies applies lifecycle policies for media storage bucket
func applyMediaBucketPolicies(config *S3LifecycleConfig) {
	// Clean up incomplete multipart uploads after 7 days
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:                                  jsii.String("delete-incomplete-uploads"),
		AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(7)),
		Enabled:                             jsii.Bool(true),
	})

	// Move infrequently accessed media to cheaper storage classes
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("optimize-storage-class"),
		Enabled: jsii.Bool(true),
		Transitions: &[]*awss3.Transition{
			{
				// Move to IA after 30 days
				StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
			},
			{
				// Move to Glacier Instant Retrieval after 90 days
				StorageClass:    awss3.StorageClass_GLACIER_INSTANT_RETRIEVAL(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
			},
			{
				// Move to Glacier Flexible Retrieval after 180 days
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(180)),
			},
		},
	})

	// Delete old temporary files in temp/ prefix
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("delete-temp-files"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("temp/"),
		Expiration: awscdk.Duration_Days(jsii.Number(1)), // Delete temp files after 1 day
	})

	// Delete old thumbnails (they can be regenerated if needed)
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("delete-old-thumbnails"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("thumbnails/"),
		Expiration: awscdk.Duration_Days(jsii.Number(365)), // Delete thumbnails after 1 year
	})

	// Handle orphaned uploads (uploads that weren't properly associated)
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("delete-orphaned-uploads"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("uploads/orphaned/"),
		Expiration: awscdk.Duration_Days(jsii.Number(30)), // Delete orphaned uploads after 30 days
	})

	// Environment-specific policies
	if config.Environment == "development" || config.Environment == "staging" {
		// More aggressive cleanup in non-production environments
		config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
			Id:         jsii.String("dev-cleanup"),
			Enabled:    jsii.Bool(true),
			Prefix:     jsii.String("test/"),
			Expiration: awscdk.Duration_Days(jsii.Number(7)), // Delete test files after 7 days
		})
	}

	// Handle deleted/archived content
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("archive-deleted-content"),
		Enabled: jsii.Bool(true),
		Prefix:  jsii.String("deleted/"),
		Transitions: &[]*awss3.Transition{
			{
				// Move deleted content to Glacier after 7 days
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(7)),
			},
		},
		// Keep deleted content for compliance/recovery for 90 days
		Expiration: awscdk.Duration_Days(jsii.Number(90)),
	})

	// Optimize avatar storage (avatars are accessed frequently initially, then rarely)
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("optimize-avatar-storage"),
		Enabled: jsii.Bool(true),
		Prefix:  jsii.String("avatars/"),
		Transitions: &[]*awss3.Transition{
			{
				// Move to IA after 60 days (avatars rarely change)
				StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(60)),
			},
		},
	})

	// Handle media from deleted accounts
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("cleanup-deleted-accounts"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("accounts/deleted/"),
		Expiration: awscdk.Duration_Days(jsii.Number(30)), // Delete after 30 days per GDPR
	})

	// Expire old versions (if versioning is enabled)
	if config.Environment == "production" {
		config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
			Id:                          jsii.String("expire-old-versions"),
			Enabled:                     jsii.Bool(true),
			NoncurrentVersionExpiration: awscdk.Duration_Days(jsii.Number(30)), // Delete old versions after 30 days
			ExpiredObjectDeleteMarker:   jsii.Bool(true),                       // Clean up delete markers
		})
	}
}

// applyLogBucketPolicies applies lifecycle policies for log storage buckets
func applyLogBucketPolicies(config *S3LifecycleConfig) {
	// Transition logs through storage classes based on age
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("optimize-log-storage"),
		Enabled: jsii.Bool(true),
		Transitions: &[]*awss3.Transition{
			{
				// Move to IA after 30 days
				StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
			},
			{
				// Move to Glacier after 60 days
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(60)),
			},
		},
	})

	// Delete old logs based on environment
	var retentionDays float64
	switch config.Environment {
	case "production":
		retentionDays = 365 // Keep production logs for 1 year
	case "staging":
		retentionDays = 90 // Keep staging logs for 90 days
	default:
		retentionDays = 30 // Keep dev logs for 30 days
	}

	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("delete-old-logs"),
		Enabled:    jsii.Bool(true),
		Expiration: awscdk.Duration_Days(jsii.Number(retentionDays)),
	})

	// Clean up incomplete multipart uploads
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:                                  jsii.String("cleanup-incomplete-uploads"),
		AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
		Enabled:                             jsii.Bool(true),
	})
}

// applyBackupBucketPolicies applies lifecycle policies for backup buckets
func applyBackupBucketPolicies(config *S3LifecycleConfig) {
	// Move backups to cheaper storage over time
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("optimize-backup-storage"),
		Enabled: jsii.Bool(true),
		Transitions: &[]*awss3.Transition{
			{
				// Move to IA after 7 days
				StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(7)),
			},
			{
				// Move to Glacier after 30 days
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
			},
			{
				// Move to Deep Archive after 90 days
				StorageClass:    awss3.StorageClass_DEEP_ARCHIVE(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
			},
		},
	})

	// Retention policy based on backup type
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("daily-backup-retention"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("daily/"),
		Expiration: awscdk.Duration_Days(jsii.Number(30)), // Keep daily backups for 30 days
	})

	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("weekly-backup-retention"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("weekly/"),
		Expiration: awscdk.Duration_Days(jsii.Number(90)), // Keep weekly backups for 90 days
	})

	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("monthly-backup-retention"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("monthly/"),
		Expiration: awscdk.Duration_Days(jsii.Number(365)), // Keep monthly backups for 1 year
	})

	// Clean up failed backups
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("cleanup-failed-backups"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("failed/"),
		Expiration: awscdk.Duration_Days(jsii.Number(7)), // Delete failed backups after 7 days
	})
}

// applyCloudFrontLogPolicies applies lifecycle policies for CloudFront log buckets
func applyCloudFrontLogPolicies(config *S3LifecycleConfig) {
	// CloudFront logs are numerous but rarely accessed after initial analysis
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("optimize-cloudfront-logs"),
		Enabled: jsii.Bool(true),
		Prefix:  jsii.String("cloudfront/"),
		Transitions: &[]*awss3.Transition{
			{
				// Move to IA after 7 days
				StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(7)),
			},
			{
				// Move to Glacier after 30 days
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
			},
		},
		// Delete after retention period
		Expiration: awscdk.Duration_Days(jsii.Number(90)),
	})

	// Clean up incomplete uploads (logs are small, shouldn't have multipart)
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:                                  jsii.String("cleanup-incomplete-log-uploads"),
		AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
		Enabled:                             jsii.Bool(true),
	})
}

// applyDefaultPolicies applies default lifecycle policies for general buckets
func applyDefaultPolicies(config *S3LifecycleConfig) {
	// Basic cleanup of incomplete uploads
	config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:                                  jsii.String("default-cleanup-incomplete"),
		AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(7)),
		Enabled:                             jsii.Bool(true),
	})

	// Move to cheaper storage for old objects
	if config.Environment == "production" {
		config.Bucket.AddLifecycleRule(&awss3.LifecycleRule{
			Id:      jsii.String("default-storage-optimization"),
			Enabled: jsii.Bool(true),
			Transitions: &[]*awss3.Transition{
				{
					StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
					TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
				},
			},
		})
	}
}

// CreateExportBucketWithLifecycle creates an S3 bucket for data exports with lifecycle policies
func CreateExportBucketWithLifecycle(scope awscdk.Stack, environment string) awss3.Bucket {
	isProd := environment == "production"

	bucket := awss3.NewBucket(scope, jsii.String("ExportBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-exports-%s", environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     getRemovalPolicyForEnv(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		Versioned:         jsii.Bool(isProd), // Enable versioning in production
	})

	// Apply export-specific lifecycle policies
	// User exports are temporary and should be cleaned up
	bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("delete-old-exports"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("exports/"),
		Expiration: awscdk.Duration_Days(jsii.Number(30)), // Delete exports after 30 days
	})

	// Clean up incomplete exports
	bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:                                  jsii.String("cleanup-incomplete-exports"),
		AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
		Enabled:                             jsii.Bool(true),
	})

	// Archive completed exports before deletion
	bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:      jsii.String("archive-exports"),
		Enabled: jsii.Bool(true),
		Prefix:  jsii.String("exports/completed/"),
		Transitions: &[]*awss3.Transition{
			{
				StorageClass:    awss3.StorageClass_GLACIER(),
				TransitionAfter: awscdk.Duration_Days(jsii.Number(7)), // Archive after 7 days
			},
		},
		Expiration: awscdk.Duration_Days(jsii.Number(30)), // Delete after 30 days
	})

	return bucket
}

// CreateCacheBucketWithLifecycle creates an S3 bucket for caching with aggressive cleanup
func CreateCacheBucketWithLifecycle(scope awscdk.Stack, environment string) awss3.Bucket {
	isProd := environment == "production"

	bucket := awss3.NewBucket(scope, jsii.String("CacheBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-cache-%s", environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     getRemovalPolicyForEnv(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
	})

	// Aggressive cache cleanup
	bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("expire-cache"),
		Enabled:    jsii.Bool(true),
		Expiration: awscdk.Duration_Days(jsii.Number(1)), // Expire all cache after 1 day
	})

	// Even more aggressive for specific cache types
	bucket.AddLifecycleRule(&awss3.LifecycleRule{
		Id:         jsii.String("expire-temp-cache"),
		Enabled:    jsii.Bool(true),
		Prefix:     jsii.String("temp/"),
		Expiration: awscdk.Duration_Hours(jsii.Number(1)), // Expire temp cache after 1 hour
	})

	return bucket
}

// getRemovalPolicyForEnv returns the appropriate removal policy based on environment
func getRemovalPolicyForEnv(isProd bool) awscdk.RemovalPolicy {
	if isProd {
		return awscdk.RemovalPolicy_RETAIN
	}
	return awscdk.RemovalPolicy_DESTROY
}

// S3LifecycleMetrics tracks cost savings from lifecycle policies
type S3LifecycleMetrics struct {
	BucketName           string
	ObjectsTransitioned  int64
	ObjectsExpired       int64
	StorageCostSaved     float64 // in dollars
	EstimatedMonthlySave float64 // in dollars
}

// CalculateLifecycleSavings estimates cost savings from lifecycle policies
func CalculateLifecycleSavings(bucket awss3.Bucket, environment string) *S3LifecycleMetrics {
	// This would integrate with CloudWatch metrics to calculate actual savings
	// For now, return estimated values based on typical patterns

	metrics := &S3LifecycleMetrics{
		BucketName: *bucket.BucketName(),
	}

	// Estimate based on environment and typical usage patterns
	switch environment {
	case "production":
		metrics.EstimatedMonthlySave = 150.00 // Typical savings from lifecycle policies
	case "staging":
		metrics.EstimatedMonthlySave = 25.00
	default:
		metrics.EstimatedMonthlySave = 5.00
	}

	return metrics
}
