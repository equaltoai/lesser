package constructs

import (
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// IdempotencyTableProps defines properties for creating an idempotency table
type IdempotencyTableProps struct {
	// Table name
	TableName *string
	// TTL attribute name for automatic cleanup
	TimeToLiveAttribute *string
}

// NewIdempotencyTable creates a DynamoDB table for idempotency tracking
// The table uses standard pk/sk attributes - GSIs should be defined in DynamORM models
func NewIdempotencyTable(scope constructs.Construct, id *string, props *IdempotencyTableProps) *LiftTable {
	// Set defaults
	if props == nil {
		props = &IdempotencyTableProps{}
	}

	// Set default table name
	if props.TableName == nil {
		props.TableName = jsii.String("idempotency")
	}

	// Set default TTL attribute for automatic cleanup
	if props.TimeToLiveAttribute == nil {
		props.TimeToLiveAttribute = jsii.String("expires_at")
	}

	// Create table with field names from IdempotencyRecord struct
	return NewLiftTable(scope, id, &LiftTableProps{
		TableName:                 props.TableName,
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		TimeToLiveAttribute:       props.TimeToLiveAttribute,
		EnablePointInTimeRecovery: jsii.Bool(true),
	})
}

// Example DynamORM model for idempotency:
//
// type IdempotencyRecord struct {
//     PK         string    `dynamorm:"pk"`                          // idempotency#{key}
//     SK         string    `dynamorm:"sk"`                          // idempotency#{key}
//
//     // Indexes for queries
//     FunctionName string  `dynamorm:"index:function-index,pk"`     // function_name
//     Status       string  `dynamorm:"index:status-index,pk"`       // status
//     Timestamp    string  `dynamorm:"index:status-index,sk"`       // ISO timestamp
//     TenantID     string  `dynamorm:"index:tenant-index,pk"`       // tenant_id (if multi-tenant)
//
//     // Record data
//     IdempotencyKey string `json:"idempotency_key"`
//     Response       string `json:"response"`
//     ExpiresAt      int64  `json:"expires_at"`                     // TTL
// }
