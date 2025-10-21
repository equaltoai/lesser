package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
)

// EventRoutingTableProps defines properties for the event routing table
type EventRoutingTableProps struct {
	// Table name
	TableName *string
	// TTL attribute name for automatic cleanup
	TimeToLiveAttribute *string
}

// EventRoutingTable is a table for managing event routing
type EventRoutingTable struct {
	construct constructs.Construct
	*LiftTable
}

// NewEventRoutingTable creates a new event routing table
//
// The table uses standard pk/sk attributes - GSIs should be defined in DynamORM models
func NewEventRoutingTable(scope constructs.Construct, id *string, props *EventRoutingTableProps) *EventRoutingTable {
	// Create the table using shared factory
	liftTable := createTypedManagementTable(scope, id, props, ManagementTableConfig{
		DefaultTableName: "event-routing",
		PermissionMethod: "GrantEventManagement",
	})

	return &EventRoutingTable{
		construct: scope,
		LiftTable: liftTable,
	}
}

// GrantEventManagement grants permissions to manage events
func (e *EventRoutingTable) GrantEventManagement(grantee awsiam.IGrantable) {
	grantManagementPermissions(e.LiftTable, grantee)
}

// Example DynamORM model for event routing:
//
// type EventRoute struct {
//     PK         string    `dynamorm:"pk"`                          // event#{event_id}
//     SK         string    `dynamorm:"sk"`                          // timestamp#{timestamp}
//
//     // Indexes for queries
//     EventSource      string `dynamorm:"index:source-index,pk"`  // event_source
//     Timestamp        string `dynamorm:"index:source-index,sk"`  // ISO timestamp
//     ProcessingStatus string `dynamorm:"index:status-index,pk"`  // processing_status
//     Date             string `dynamorm:"index:date-index,pk"`    // YYYY-MM-DD
//
//     // Event data
//     EventID    string `json:"event_id"`
//     TTL        int64  `json:"ttl"`
// }
