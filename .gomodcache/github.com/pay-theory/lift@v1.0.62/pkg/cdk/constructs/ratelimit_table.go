package constructs

import (
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// RateLimitTableProps defines properties for creating a rate limit table
type RateLimitTableProps struct {
	// Table name
	TableName *string
	// TTL attribute name for automatic cleanup
	TimeToLiveAttribute *string
}

// NewRateLimitTable creates a DynamoDB table for rate limiting
//
// The table uses standard pk/sk attributes - GSIs should be defined in DynamORM models
func NewRateLimitTable(scope constructs.Construct, id *string, props *RateLimitTableProps) *LiftTable {
	// Set defaults
	if props == nil {
		props = &RateLimitTableProps{}
	}

	// Set default table name
	if props.TableName == nil {
		props.TableName = jsii.String("rate-limits")
	}

	// Set default TTL attribute for automatic cleanup
	if props.TimeToLiveAttribute == nil {
		props.TimeToLiveAttribute = jsii.String("expires_at")
	}

	// Create table with field names from RateLimit struct
	return NewLiftTable(scope, id, &LiftTableProps{
		TableName:           props.TableName,
		PartitionKeyName:    jsii.String("PK"),
		SortKeyName:         jsii.String("SK"),
		TimeToLiveAttribute: props.TimeToLiveAttribute,
	})
}

// Example DynamORM model for rate limiting:
//
// type RateLimit struct {
//     PK         string    `dynamorm:"pk"`                      // ratelimit#{identifier}#{window}
//     SK         string    `dynamorm:"sk"`                      // ratelimit#{identifier}#{window}
//
//     // Indexes for different rate limit strategies
//     IPAddress  string    `dynamorm:"index:ip-index,pk"`       // ip_address
//     UserID     string    `dynamorm:"index:user-index,pk"`     // user_id
//     TenantID   string    `dynamorm:"index:tenant-index,pk"`   // tenant_id
//     BucketKey  string    `dynamorm:"index:bucket-index,pk"`   // bucket_key (for Limited library)
//
//     // Rate limit data
//     Identifier string    `json:"identifier"`
//     WindowTime string    `json:"window_time"`
//     Count      int       `json:"count"`
//     ExpiresAt  int64     `json:"expires_at"`                  // TTL
// }
