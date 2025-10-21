package constructs

import (
	"github.com/aws/constructs-go/constructs/v10"
)

// RequestTrackingTableProps defines properties for the request tracking table
type RequestTrackingTableProps struct {
	// Table name
	TableName *string
	// TTL attribute name for automatic cleanup
	TimeToLiveAttribute *string
}

// RequestTrackingTable is a table for tracking API requests and their async processing
type RequestTrackingTable struct {
	construct constructs.Construct
	*LiftTable
}

// NewRequestTrackingTable creates a new request tracking table
// The table uses standard pk/sk attributes - GSIs should be defined in DynamORM models
func NewRequestTrackingTable(scope constructs.Construct, id *string, props *RequestTrackingTableProps) *RequestTrackingTable {
	// Create base props
	baseProps := &BaseManagementTableProps{
		DefaultTableName: "request-tracking",
	}

	if props != nil {
		baseProps.TableName = props.TableName
		baseProps.TimeToLiveAttribute = props.TimeToLiveAttribute
	}

	// Create the table using common function
	liftTable := createManagementTable(scope, id, baseProps)

	return &RequestTrackingTable{
		construct: scope,
		LiftTable: liftTable,
	}
}

// Example DynamORM model for request tracking:
//
// type RequestTracking struct {
//     PK         string    `dynamorm:"pk"`                             // request#{request_id}
//     SK         string    `dynamorm:"sk"`                             // request#{request_id}
//
//     // Indexes for queries
//     CorrelationID string `dynamorm:"index:correlation-index,pk"`    // correlation_id
//     Timestamp     string `dynamorm:"index:correlation-index,sk"`    // ISO timestamp
//     Status        string `dynamorm:"index:status-index,pk"`         // status
//     UserID        string `dynamorm:"index:user-index,pk"`           // user_id
//     Date          string `dynamorm:"index:timestamp-index,pk"`      // YYYY-MM-DD
//
//     // Request data
//     RequestID     string `json:"request_id"`
//     TTL           int64  `json:"ttl"`
// }
