package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// APICommonProps contains shared properties for both REST and HTTP APIs
type APICommonProps struct {
	// Name of the API
	Name *string
	// Description of the API
	Description *string
	// Enable CORS
	EnableCORS *bool
	// CORS allowed origins (defaults to ["*"] if not specified)
	AllowOrigins *[]*string
	// Custom domain name
	DomainName *string
	// Certificate ARN for custom domain
	CertificateArn *string
	// Enable access logging
	EnableAccessLogging *bool
	// CloudWatch log group for access logs
	AccessLogGroup awslogs.ILogGroup
	// Throttle settings
	ThrottleRateLimit  *float64
	ThrottleBurstLimit *float64
	// Stage name
	StageName *string
}

// CORSHeaders returns standard CORS headers used across all API types
func CORSHeaders() *[]*string {
	return &[]*string{
		jsii.String("Content-Type"),
		jsii.String("Authorization"),
		jsii.String("X-Tenant-ID"),
		jsii.String("X-Request-ID"),
		jsii.String("X-Api-Key"),
	}
}

// CORSExposeHeaders returns standard CORS expose headers
func CORSExposeHeaders() *[]*string {
	return &[]*string{
		jsii.String("X-Request-ID"),
		jsii.String("X-Rate-Limit-Limit"),
		jsii.String("X-Rate-Limit-Remaining"),
		jsii.String("X-Rate-Limit-Reset"),
	}
}

// CORSMethods returns standard CORS methods
func CORSMethods() []string {
	return []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
}

// CreateAPILogGroup creates a CloudWatch log group for API access logs
func CreateAPILogGroup(scope constructs.Construct, apiName *string, existingLogGroup awslogs.ILogGroup) awslogs.ILogGroup {
	if existingLogGroup != nil {
		return existingLogGroup
	}

	return awslogs.NewLogGroup(scope, jsii.String("AccessLogs"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String("/aws/apigateway/" + *apiName),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})
}

// SplitPath splits a URL path into segments
func SplitPath(path string) []string {
	segments := []string{}
	current := ""

	for _, c := range path {
		if c == '/' {
			if current != "" {
				segments = append(segments, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		segments = append(segments, current)
	}

	return segments
}
