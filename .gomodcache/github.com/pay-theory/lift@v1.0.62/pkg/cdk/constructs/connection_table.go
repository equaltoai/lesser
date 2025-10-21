// Package constructs provides AWS CDK constructs for Lift applications.
//
// This package contains high-level CDK constructs that implement Lift's best practices
// for AWS infrastructure. The constructs include optimized configurations for API
// Gateway, Lambda functions, DynamoDB tables, and other AWS services.
//
// # Connection Table
//
// The connection_table.go file provides constructs for managing WebSocket connections
// in Lift applications. It includes functionality for creating and managing DynamoDB
// tables that store WebSocket connection information and related metadata.
package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
)

// ConnectionTableProps defines properties for the WebSocket connection table.
//
// This struct contains properties for creating a DynamoDB table to manage WebSocket
// connections, including table name and TTL attribute for automatic cleanup.
type ConnectionTableProps struct {
	// Table name
	TableName *string
	// Enable TTL for automatic connection cleanup
	TimeToLiveAttribute *string
}

// ConnectionTable is a table for managing WebSocket connections.
//
// This struct represents a DynamoDB table specifically designed for storing and
// managing WebSocket connection information, including connection IDs, endpoints,
// and other metadata.
type ConnectionTable struct {
	construct constructs.Construct
	*LiftTable
}

// NewConnectionTable creates a new connection management table.
//
// This function creates a DynamoDB table specifically designed for managing WebSocket
// connections. The table uses a primary key (PK) and sort key (SK) for storing
// connection IDs and metadata. Global Secondary Indexes (GSIs) should be defined
// in your DynamORM model structs for querying connections by different attributes.
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Properties for the connection table
//
// Returns:
//   - A new ConnectionTable instance
func NewConnectionTable(scope constructs.Construct, id *string, props *ConnectionTableProps) *ConnectionTable {
	// Create the table using shared factory
	liftTable := createTypedManagementTable(scope, id, props, ManagementTableConfig{
		DefaultTableName: "websocket-connections",
		PermissionMethod: "GrantConnectionManagement",
	})

	return &ConnectionTable{
		construct: scope,
		LiftTable: liftTable,
	}
}

// GrantConnectionManagement grants permissions to manage WebSocket connections.
//
// This method grants read and write permissions on the connection table to the
// specified grantee, which is typically a Lambda function or other AWS service
// that needs to manage WebSocket connections.
//
// Parameters:
//   - grantee: The IAM principal to grant permissions to
func (c *ConnectionTable) GrantConnectionManagement(grantee awsiam.IGrantable) {
	grantManagementPermissions(c.LiftTable, grantee)
}

// Example DynamORM model for connections:
//
// type Connection struct {
//     PK         string    `dynamorm:"pk"`                       // connection#{connection_id}
//     SK         string    `dynamorm:"sk"`                       // connection#{connection_id}
//
//     // Indexes for queries
//     UserID     string    `dynamorm:"index:user-index,pk"`      // user_id
//     CreatedAt  string    `dynamorm:"index:user-index,sk"`      // ISO timestamp
//     TenantID   string    `dynamorm:"index:tenant-index,pk"`    // tenant_id (if multi-tenant)
//
//     // Connection data
//     ConnectionID string  `json:"connection_id"`
//     Endpoint     string  `json:"endpoint"`
//     TTL          int64   `json:"ttl"`
// }
